package visionai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/storage"
	"gorm.io/gorm"
)

type datasetEventPayload struct {
	DatasetVersionID uint64 `json:"datasetVersionId"`
	RequestedBy      uint64 `json:"requestedBy"`
}

func processDatasetEvent(ctx context.Context, db *gorm.DB, cfg config.Config, event DomainEventEnvelope) error {
	var payload datasetEventPayload
	if json.Unmarshal(event.Payload, &payload) != nil || payload.DatasetVersionID == 0 {
		return errors.New("invalid dataset event payload")
	}
	var version DatasetVersion
	if db.Where("tenant_id = ? AND id = ?", event.TenantID, payload.DatasetVersionID).First(&version).Error != nil {
		return nil
	}
	objectStore, err := storage.NewMinIO(cfg)
	if err != nil {
		return finishDatasetJob(db, &version, false, "STORAGE_CONFIG_INVALID", err.Error())
	}
	if err = objectStore.EnsureBucket(ctx); err != nil {
		return finishDatasetJob(db, &version, false, "STORAGE_UNAVAILABLE", err.Error())
	}
	switch event.EventType {
	case "dataset.validate.requested.v1":
		return validateDatasetVersion(ctx, db, objectStore, &version)
	case "dataset.freeze.requested.v1":
		return freezeDatasetVersion(ctx, db, objectStore, &version, payload.RequestedBy)
	}
	return nil
}

func finishDatasetJob(db *gorm.DB, version *DatasetVersion, success bool, code, message string) error {
	now := time.Now()
	jobStatus, stage := JobFailed, "FAILED"
	if success {
		jobStatus, stage = JobSucceeded, string(version.Status)
	}
	return db.Model(&PlatformJob{}).
		Where("tenant_id = ? AND resource_type = ? AND resource_id = ? AND status IN ?", version.TenantID, "DATASET_VERSION", version.ID, []JobStatus{JobQueued, JobRunning}).
		Updates(map[string]any{
			"status": jobStatus, "stage": stage, "progress": map[bool]int{true: 100, false: 0}[success],
			"error_code": code, "error_message": message, "finished_at": now,
		}).Error
}

type cvatAnnotationShape struct {
	Frame   int       `json:"frame"`
	LabelID int64     `json:"label_id"`
	Points  []float64 `json:"points"`
}

type cvatAnnotationSnapshot struct {
	Shapes []cvatAnnotationShape `json:"shapes"`
	Tags   []struct {
		Frame   int   `json:"frame"`
		LabelID int64 `json:"label_id"`
	} `json:"tags"`
	Tracks []struct {
		LabelID int64 `json:"label_id"`
		Shapes  []struct {
			Frame  int       `json:"frame"`
			Points []float64 `json:"points"`
		} `json:"shapes"`
	} `json:"tracks"`
}

func validationIssue(version *DatasetVersion, assetID uint64, severity, code, message, remediation string) DatasetValidationIssue {
	return DatasetValidationIssue{
		TenantID: version.TenantID, ProjectID: version.ProjectID, DatasetVersionID: version.ID,
		AssetID: assetID, Severity: severity, Code: code, Message: message, Remediation: remediation,
	}
}

func validateDatasetVersion(ctx context.Context, db *gorm.DB, objectStore storage.Provider, version *DatasetVersion) error {
	var items []DatasetVersionItem
	db.Where("tenant_id = ? AND dataset_version_id = ?", version.TenantID, version.ID).Order("id").Find(&items)
	assetIDs := make([]uint64, 0, len(items))
	for _, item := range items {
		assetIDs = append(assetIDs, item.AssetID)
	}
	var assets []Asset
	db.Where("tenant_id = ? AND project_id = ? AND id IN ?", version.TenantID, version.ProjectID, assetIDs).Find(&assets)
	assetByID := make(map[uint64]Asset, len(assets))
	for _, asset := range assets {
		assetByID[asset.ID] = asset
	}
	issues := make([]DatasetValidationIssue, 0)
	hashSplit := make(map[string]string)
	for _, item := range items {
		asset, exists := assetByID[item.AssetID]
		if !exists || asset.Status != AssetReady {
			issues = append(issues, validationIssue(version, item.AssetID, "ERROR", "OBJECT_MISSING", "资产不存在或不可用", "恢复或替换该资产后创建新版本"))
			continue
		}
		if _, err := objectStore.Stat(ctx, asset.ObjectKey); err != nil {
			issues = append(issues, validationIssue(version, asset.ID, "ERROR", "OBJECT_MISSING", "对象存储中找不到资产", "恢复对象后重新验证"))
		}
		if previousSplit, duplicate := hashSplit[asset.SHA256]; duplicate {
			severity, code := "WARNING", "DUPLICATE_CONTENT"
			if previousSplit != item.Split {
				severity, code = "ERROR", "SPLIT_LEAKAGE"
			}
			issues = append(issues, validationIssue(version, asset.ID, severity, code, "相同内容哈希出现在数据集多次", "去重并重新生成固定划分"))
		} else {
			hashSplit[asset.SHA256] = item.Split
		}
	}

	var revision AnnotationRevision
	var snapshot cvatAnnotationSnapshot
	if version.AnnotationRevisionID == 0 || db.Where("tenant_id = ? AND project_id = ? AND id = ?", version.TenantID, version.ProjectID, version.AnnotationRevisionID).First(&revision).Error != nil {
		issues = append(issues, validationIssue(version, 0, "ERROR", "ANNOTATION_REVISION_MISSING", "数据集版本未引用有效的标注快照", "先完成标注审核与快照导出"))
	} else {
		if version.OntologyVersionID == 0 || revision.OntologyVersionID == 0 ||
			version.OntologyVersionID != revision.OntologyVersionID ||
			version.OntologyChecksum == "" || revision.OntologyChecksum != version.OntologyChecksum {
			issues = append(issues, validationIssue(version, 0, "ERROR", "ONTOLOGY_VERSION_MISMATCH", "数据集与标注快照的类别体系版本或校验和不一致", "选择与标注快照一致的已发布类别版本创建新数据集版本"))
		}
		reader, _, err := objectStore.Get(ctx, revision.ObjectKey)
		if err != nil {
			issues = append(issues, validationIssue(version, 0, "ERROR", "ANNOTATION_OBJECT_MISSING", "标注快照对象不存在", "恢复快照对象或重新导出 Revision"))
		} else {
			raw, readErr := io.ReadAll(io.LimitReader(reader, 512<<20))
			_ = reader.Close()
			sum := sha256.Sum256(raw)
			if readErr != nil || hex.EncodeToString(sum[:]) != revision.Checksum {
				issues = append(issues, validationIssue(version, 0, "ERROR", "ANNOTATION_CHECKSUM_MISMATCH", "标注快照校验和不匹配", "隔离受损快照并重新导出"))
			} else if json.Unmarshal(raw, &snapshot) != nil {
				issues = append(issues, validationIssue(version, 0, "ERROR", "ANNOTATION_FORMAT_INVALID", "标注快照不是有效 CVAT JSON", "重新导出受支持格式"))
			}
		}
	}
	annotatedFrames := make(map[int]bool)
	validLabelIDs := map[int64]bool{}
	if revision.CategoryMapping != "" {
		var categoryMapping []struct {
			ID int64 `json:"id"`
		}
		if json.Unmarshal([]byte(revision.CategoryMapping), &categoryMapping) == nil {
			for _, category := range categoryMapping {
				if category.ID > 0 {
					validLabelIDs[category.ID] = true
				}
			}
		}
	}
	for _, shape := range snapshot.Shapes {
		annotatedFrames[shape.Frame] = true
		if shape.LabelID <= 0 || (len(validLabelIDs) > 0 && !validLabelIDs[shape.LabelID]) {
			issues = append(issues, validationIssue(version, 0, "ERROR", "CATEGORY_MISMATCH", "标注引用无效类别", "修复类别映射后重新导出"))
		}
		if shape.Frame >= 0 && shape.Frame < len(items) {
			asset := assetByID[items[shape.Frame].AssetID]
			for point := 0; point+1 < len(shape.Points); point += 2 {
				if shape.Points[point] < 0 || shape.Points[point] > float64(asset.Width) || shape.Points[point+1] < 0 || shape.Points[point+1] > float64(asset.Height) {
					issues = append(issues, validationIssue(version, asset.ID, "ERROR", "ANNOTATION_OUT_OF_BOUNDS", "检测框或点超出图像边界", "在 CVAT 修复几何标注并导出新 Revision"))
					break
				}
			}
		}
	}
	for _, tag := range snapshot.Tags {
		annotatedFrames[tag.Frame] = true
	}
	for _, track := range snapshot.Tracks {
		for _, shape := range track.Shapes {
			annotatedFrames[shape.Frame] = true
		}
	}
	for frame, item := range items {
		if !annotatedFrames[frame] {
			issues = append(issues, validationIssue(version, item.AssetID, "WARNING", "EMPTY_ANNOTATION", "样本没有标注对象", "确认其为负样本或补充标注"))
		}
	}
	var errorsCount, warningsCount int64
	for _, issue := range issues {
		if issue.Severity == "ERROR" {
			errorsCount++
		} else {
			warningsCount++
		}
	}
	version.Status = DatasetVersionReady
	if errorsCount > 0 {
		version.Status = DatasetVersionDraft
	}
	version.ValidationSummary = jsonValue(map[string]any{
		"objectsChecked": len(items), "annotations": len(snapshot.Shapes) + len(snapshot.Tags) + len(snapshot.Tracks),
		"errors": errorsCount, "warnings": warningsCount, "checkedAt": time.Now().UTC(),
	})
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND dataset_version_id = ?", version.TenantID, version.ID).Delete(&DatasetValidationIssue{}).Error; err != nil {
			return err
		}
		if len(issues) > 0 {
			if err := tx.Create(&issues).Error; err != nil {
				return err
			}
		}
		return tx.Save(version).Error
	})
	if err != nil {
		return finishDatasetJob(db, version, false, "VALIDATION_SAVE_FAILED", err.Error())
	}
	if errorsCount > 0 {
		return finishDatasetJob(db, version, false, "DATASET_VALIDATION_FAILED", fmt.Sprintf("%d blocking validation errors", errorsCount))
	}
	return finishDatasetJob(db, version, true, "", "")
}

type manifestItem struct {
	AssetID   uint64 `json:"assetId"`
	URI       string `json:"uri"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Split     string `json:"split"`
	MediaType string `json:"mediaType"`
}

func freezeDatasetVersion(ctx context.Context, db *gorm.DB, objectStore storage.Provider, version *DatasetVersion, requestedBy uint64) error {
	var issues int64
	db.Model(&DatasetValidationIssue{}).Where("tenant_id = ? AND dataset_version_id = ? AND severity = ?", version.TenantID, version.ID, "ERROR").Count(&issues)
	if issues > 0 {
		version.Status = DatasetVersionDraft
		db.Save(version)
		return finishDatasetJob(db, version, false, "DATASET_VALIDATION_FAILED", "blocking validation errors remain")
	}
	var items []DatasetVersionItem
	db.Where("tenant_id = ? AND dataset_version_id = ?", version.TenantID, version.ID).Order("asset_id").Find(&items)
	assetIDs := make([]uint64, 0, len(items))
	splitByAsset := make(map[uint64]string, len(items))
	for _, item := range items {
		assetIDs = append(assetIDs, item.AssetID)
		splitByAsset[item.AssetID] = item.Split
	}
	var assets []Asset
	db.Where("tenant_id = ? AND id IN ?", version.TenantID, assetIDs).Order("id").Find(&assets)
	manifestItems := make([]manifestItem, 0, len(assets))
	for _, asset := range assets {
		manifestItems = append(manifestItems, manifestItem{
			AssetID: asset.ID, URI: asset.URI, SHA256: asset.SHA256, Size: asset.Size,
			Width: asset.Width, Height: asset.Height, Split: splitByAsset[asset.ID], MediaType: asset.ContentType,
		})
	}
	sort.Slice(manifestItems, func(i, j int) bool { return manifestItems[i].AssetID < manifestItems[j].AssetID })
	var revision AnnotationRevision
	if db.Where("tenant_id = ? AND id = ?", version.TenantID, version.AnnotationRevisionID).First(&revision).Error != nil {
		return finishDatasetJob(db, version, false, "ANNOTATION_REVISION_MISSING", "annotation revision is required")
	}
	manifest := map[string]any{
		"schemaVersion": "visionai.dataset-manifest.v1", "datasetVersionId": version.ID,
		"semanticVersion": version.SemanticVersion, "taskType": func() string {
			var dataset Dataset
			db.Where("id = ?", version.DatasetID).First(&dataset)
			return dataset.TaskType
		}(),
		"ontologyVersionId": version.OntologyVersionID, "ontologyVersion": version.OntologyVersion,
		"ontologyChecksum": version.OntologyChecksum, "splitSeed": version.SplitSeed,
		"splitConfig": json.RawMessage(version.SplitConfig), "items": manifestItems,
		"annotationRevision": map[string]any{
			"id": revision.ID, "uri": revision.SnapshotURI, "checksum": revision.Checksum, "format": revision.Format,
		},
	}
	raw, _ := json.Marshal(manifest)
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])
	root := assetRoot(version.TenantID, version.ProjectID) + "/datasets/" + strconv.FormatUint(version.DatasetID, 10) + "/" + version.SemanticVersion
	manifestKey, cardKey := root+"/manifest.json", root+"/dataset-card.md"
	card := fmt.Sprintf("# Dataset Card — %s\n\n- Version: %s\n- Items: %d\n- Train/Val/Test: %d/%d/%d\n- Ontology: %s (#%d)\n- Ontology SHA-256: `%s`\n- Annotation Revision: %d\n- Manifest SHA-256: `%s`\n- Frozen at: %s\n",
		version.SemanticVersion, version.SemanticVersion, version.ItemCount, version.TrainCount, version.ValidationCount, version.TestCount,
		version.OntologyVersion, version.OntologyVersionID, version.OntologyChecksum,
		revision.ID, checksum, time.Now().UTC().Format(time.RFC3339))
	if err := objectStore.Put(ctx, manifestKey, bytes.NewReader(raw), int64(len(raw)), "application/json"); err != nil {
		return finishDatasetJob(db, version, false, "MANIFEST_WRITE_FAILED", err.Error())
	}
	if err := objectStore.Put(ctx, cardKey, bytes.NewReader([]byte(card)), int64(len(card)), "text/markdown"); err != nil {
		return finishDatasetJob(db, version, false, "DATASET_CARD_WRITE_FAILED", err.Error())
	}
	now := time.Now()
	version.Status, version.FrozenAt, version.FrozenBy = DatasetVersionFrozen, &now, requestedBy
	version.ManifestObjectKey, version.ManifestURI = manifestKey, objectStore.URI(manifestKey)
	version.DatasetCardObjectKey, version.DatasetCardURI, version.Checksum = cardKey, objectStore.URI(cardKey), checksum
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			ref := AssetReference{
				TenantID: version.TenantID, ProjectID: version.ProjectID, AssetID: item.AssetID,
				ResourceType: "DATASET_VERSION", ResourceID: version.ID, Frozen: true,
			}
			var existing AssetReference
			result := tx.Where("asset_id = ? AND resource_type = ? AND resource_id = ?", item.AssetID, ref.ResourceType, ref.ResourceID).First(&existing)
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				if err := tx.Create(&ref).Error; err != nil {
					return err
				}
				if err := tx.Model(&Asset{}).Where("id = ?", item.AssetID).UpdateColumn("reference_count", gorm.Expr("reference_count + 1")).Error; err != nil {
					return err
				}
			} else if result.Error != nil {
				return result.Error
			}
		}
		return tx.Save(version).Error
	})
	if err != nil {
		return finishDatasetJob(db, version, false, "DATASET_FREEZE_FAILED", err.Error())
	}
	return finishDatasetJob(db, version, true, "", "")
}
