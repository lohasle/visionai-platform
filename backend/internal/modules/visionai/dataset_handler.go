package visionai

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type datasetRequest struct {
	Name        string `json:"name"`
	TaskType    string `json:"taskType"`
	Description string `json:"description"`
	OwnerUserID uint64 `json:"ownerUserId"`
}

type datasetVersionRequest struct {
	SourceType           string             `json:"sourceType"`
	SourceID             uint64             `json:"sourceId"`
	ParentID             uint64             `json:"parentId"`
	AnnotationRevisionID uint64             `json:"annotationRevisionId"`
	OntologyVersion      string             `json:"ontologyVersion"`
	SplitSeed            int64              `json:"splitSeed"`
	Split                map[string]float64 `json:"split"`
}

func (h *Handler) getDataset(c *gin.Context, project Project) (Dataset, bool) {
	id, _ := strconv.ParseUint(c.Param("datasetId"), 10, 64)
	var row Dataset
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "数据集不存在")
		return row, false
	}
	return row, true
}

func (h *Handler) getDatasetVersion(c *gin.Context, project Project) (DatasetVersion, bool) {
	id, _ := strconv.ParseUint(c.Param("versionId"), 10, 64)
	var row DatasetVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "数据集版本不存在")
		return row, false
	}
	return row, true
}

// DatasetPage godoc
// @Summary Page logical datasets
// @Tags VisionAI Dataset
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/datasets [get]
func (h *Handler) DatasetPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	type datasetView struct {
		Dataset
		VersionCount int64 `json:"versionCount"`
		FrozenCount  int64 `json:"frozenCount"`
	}
	var rows []datasetView
	h.db.Table("ai_dataset d").
		Select("d.*, COUNT(v.id) AS version_count, SUM(CASE WHEN v.status = 'FROZEN' THEN 1 ELSE 0 END) AS frozen_count").
		Joins("LEFT JOIN ai_dataset_version v ON v.dataset_id = d.id AND v.tenant_id = d.tenant_id").
		Where("d.tenant_id = ? AND d.project_id = ?", project.TenantID, project.ID).
		Group("d.id").Order("d.id DESC").Scan(&rows)
	httpx.OK(c, rows)
}

// DatasetCreate godoc
// @Summary Create a logical dataset
// @Tags VisionAI Dataset
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/datasets [post]
func (h *Handler) DatasetCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER", "ALGORITHM_ENGINEER") {
		return
	}
	var req datasetRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, 400, 400, "数据集名称必填")
		return
	}
	if req.OwnerUserID == 0 {
		req.OwnerUserID = c.GetUint64("user_id")
	}
	taskType := strings.ToUpper(strings.TrimSpace(req.TaskType))
	if taskType == "" {
		taskType = "CV_DETECTION"
	}
	row := Dataset{
		TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name),
		TaskType: taskType, Description: strings.TrimSpace(req.Description),
		OwnerUserID: req.OwnerUserID, CreatedBy: c.GetUint64("user_id"),
	}
	if err := h.db.Create(&row).Error; err != nil {
		httpx.Fail(c, 409, 409, "同名数据集已存在")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "DATASET_CREATED", "DATASET", row.ID, nil, row)
	httpx.OK(c, row)
}

func normalizeSplit(split map[string]float64) (map[string]float64, bool) {
	if len(split) == 0 {
		split = map[string]float64{"TRAIN": 0.8, "VAL": 0.1, "TEST": 0.1}
	}
	normalized := map[string]float64{
		"TRAIN": split["TRAIN"], "VAL": split["VAL"], "TEST": split["TEST"],
	}
	sum := normalized["TRAIN"] + normalized["VAL"] + normalized["TEST"]
	return normalized, normalized["TRAIN"] > 0 && normalized["VAL"] >= 0 && normalized["TEST"] >= 0 && sum > 0.999999 && sum < 1.000001
}

func splitFor(seed int64, assetID uint64, split map[string]float64) string {
	var value [16]byte
	binary.BigEndian.PutUint64(value[:8], uint64(seed))
	binary.BigEndian.PutUint64(value[8:], assetID)
	sum := sha256.Sum256(value[:])
	position := float64(binary.BigEndian.Uint64(sum[:8])%1000000) / 1000000
	if position < split["TRAIN"] {
		return "TRAIN"
	}
	if position < split["TRAIN"]+split["VAL"] {
		return "VAL"
	}
	return "TEST"
}

// DatasetVersionCreate godoc
// @Summary Create a reproducible dataset version
// @Tags VisionAI Dataset
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/datasets/{datasetId}/versions [post]
func (h *Handler) DatasetVersionCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	dataset, ok := h.getDataset(c, project)
	if !ok {
		return
	}
	var req datasetVersionRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "版本请求格式错误")
		return
	}
	split, valid := normalizeSplit(req.Split)
	if !valid {
		httpx.Fail(c, 400, 400, "TRAIN/VAL/TEST 划分比例必须非负且总和为 1")
		return
	}
	req.SourceType = strings.ToUpper(strings.TrimSpace(req.SourceType))
	if req.SplitSeed == 0 {
		req.SplitSeed = 20260721
	}
	var assetIDs []uint64
	switch req.SourceType {
	case "COLLECTION":
		var collection AssetCollection
		if req.SourceID == 0 || h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND frozen = ?", project.TenantID, project.ID, req.SourceID, true).First(&collection).Error != nil {
			httpx.Fail(c, 409, 409, "数据集版本只能引用已冻结资产集合")
			return
		}
		h.db.Model(&AssetCollectionItem{}).Where("tenant_id = ? AND collection_id = ?", project.TenantID, req.SourceID).Order("id").Pluck("asset_id", &assetIDs)
	case "PARENT_VERSION":
		var parent DatasetVersion
		if req.ParentID == 0 || h.db.Where("tenant_id = ? AND dataset_id = ? AND id = ? AND status IN ?", project.TenantID, dataset.ID, req.ParentID, []DatasetVersionStatus{DatasetVersionFrozen, DatasetVersionDeprecated}).First(&parent).Error != nil {
			httpx.Fail(c, 409, 409, "父版本必须属于当前数据集且已冻结")
			return
		}
		req.SourceID, req.ParentID = parent.ID, parent.ID
		h.db.Model(&DatasetVersionItem{}).Where("tenant_id = ? AND dataset_version_id = ?", project.TenantID, parent.ID).Order("id").Pluck("asset_id", &assetIDs)
	default:
		httpx.Fail(c, 400, 400, "sourceType 必须是 COLLECTION 或 PARENT_VERSION")
		return
	}
	if len(assetIDs) == 0 {
		httpx.Fail(c, 409, 409, "数据源不包含资产")
		return
	}
	if req.AnnotationRevisionID != 0 {
		var revision AnnotationRevision
		if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, req.AnnotationRevisionID).First(&revision).Error != nil {
			httpx.Fail(c, 400, 400, "标注 Revision 不存在或跨项目")
			return
		}
		if req.OntologyVersion == "" {
			var task AnnotationTask
			h.db.Where("id = ?", revision.AnnotationTaskID).First(&task)
			req.OntologyVersion = task.OntologyVersion
		}
	}
	if req.OntologyVersion == "" {
		req.OntologyVersion = "v1"
	}
	var version DatasetVersion
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var latest DatasetVersion
		tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("dataset_id = ?", dataset.ID).Order("version_no DESC").First(&latest)
		versionNo := latest.VersionNo + 1
		version = DatasetVersion{
			TenantID: project.TenantID, ProjectID: project.ID, DatasetID: dataset.ID, VersionNo: versionNo,
			SemanticVersion: fmt.Sprintf("v%d", versionNo), ParentID: req.ParentID, SourceType: req.SourceType,
			SourceID: req.SourceID, AnnotationRevisionID: req.AnnotationRevisionID, OntologyVersion: req.OntologyVersion,
			SplitSeed: req.SplitSeed, SplitConfig: jsonValue(split), Status: DatasetVersionDraft,
			ItemCount: int64(len(assetIDs)), ValidationSummary: "{}",
			CreatedBy: c.GetUint64("user_id"),
		}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		for _, assetID := range assetIDs {
			item := DatasetVersionItem{
				TenantID: project.TenantID, ProjectID: project.ID, DatasetVersionID: version.ID,
				AssetID: assetID, Split: splitFor(req.SplitSeed, assetID, split), Source: req.SourceType,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "数据集版本创建失败")
		return
	}
	var counts []struct {
		Split string
		Count int64
	}
	h.db.Model(&DatasetVersionItem{}).Select("split, COUNT(*) count").Where("dataset_version_id = ?", version.ID).Group("split").Scan(&counts)
	for _, count := range counts {
		switch count.Split {
		case "TRAIN":
			version.TrainCount = count.Count
		case "VAL":
			version.ValidationCount = count.Count
		case "TEST":
			version.TestCount = count.Count
		}
	}
	h.db.Save(&version)
	_ = appendAudit(h.db, c, project.ID, "DATASET_VERSION_CREATED", "DATASET_VERSION", version.ID, nil, version)
	httpx.OK(c, version)
}

func (h *Handler) enqueueDatasetEvent(c *gin.Context, project Project, version *DatasetVersion, eventType string, target DatasetVersionStatus) {
	before := *version
	version.Status = target
	idempotency := eventType + ":" + strconv.FormatUint(version.ID, 10)
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "DATASET",
		ResourceType: "DATASET_VERSION", ResourceID: version.ID, Status: JobQueued,
		Stage: string(target), TraceID: uuid.NewString(), Idempotency: idempotency, MaxRetries: 3, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(version).Error; err != nil {
			return err
		}
		if err := tx.Where("idempotency = ?", idempotency).FirstOrCreate(&job).Error; err != nil {
			return err
		}
		return tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: eventType,
			AggregateType: "DATASET_VERSION", AggregateID: version.ID,
			Payload: jsonValue(gin.H{"datasetVersionId": version.ID, "requestedBy": c.GetUint64("user_id")}), Status: outboxNew,
		}).Error
	})
	if err != nil {
		*version = before
		httpx.Fail(c, 500, 500, "数据集编排任务创建失败")
		return
	}
	httpx.OK(c, gin.H{"version": version, "job": job})
}

// DatasetVersionValidate godoc
// @Summary Validate a dataset version
// @Tags VisionAI Dataset
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/dataset-versions/{versionId}/validate [post]
func (h *Handler) DatasetVersionValidate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	version, ok := h.getDatasetVersion(c, project)
	if !ok || version.Status != DatasetVersionDraft {
		if ok {
			httpx.Fail(c, 409, 409, "仅 DRAFT 版本可执行验证")
		}
		return
	}
	h.enqueueDatasetEvent(c, project, &version, "dataset.validate.requested.v1", DatasetVersionValidating)
}

// DatasetVersionFreeze godoc
// @Summary Freeze an immutable validated dataset version
// @Tags VisionAI Dataset
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/dataset-versions/{versionId}/freeze [post]
func (h *Handler) DatasetVersionFreeze(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER", "APPROVER") {
		return
	}
	version, ok := h.getDatasetVersion(c, project)
	if !ok || version.Status != DatasetVersionReady {
		if ok {
			httpx.Fail(c, 409, 409, "仅验证通过的 READY 版本可冻结")
		}
		return
	}
	h.enqueueDatasetEvent(c, project, &version, "dataset.freeze.requested.v1", DatasetVersionValidating)
}

// DatasetVersionGet godoc
// @Summary Get dataset version, items, validation and usage
// @Tags VisionAI Dataset
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/dataset-versions/{versionId} [get]
func (h *Handler) DatasetVersionGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	version, ok := h.getDatasetVersion(c, project)
	if !ok {
		return
	}
	var issues []DatasetValidationIssue
	h.db.Where("tenant_id = ? AND dataset_version_id = ?", project.TenantID, version.ID).Order("severity DESC, id").Find(&issues)
	var usage []DatasetUsage
	h.db.Where("tenant_id = ? AND dataset_version_id = ?", project.TenantID, version.ID).Order("id").Find(&usage)
	httpx.OK(c, gin.H{"version": version, "issues": issues, "usage": usage})
}

// DatasetVersions godoc
// @Summary List versions of a dataset
// @Tags VisionAI Dataset
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/datasets/{datasetId}/versions [get]
func (h *Handler) DatasetVersions(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	dataset, ok := h.getDataset(c, project)
	if !ok {
		return
	}
	var rows []DatasetVersion
	h.db.Where("tenant_id = ? AND dataset_id = ?", project.TenantID, dataset.ID).Order("version_no DESC").Find(&rows)
	httpx.OK(c, rows)
}

// DatasetVersionCompare godoc
// @Summary Compare two dataset versions
// @Tags VisionAI Dataset
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/dataset-versions/compare [get]
func (h *Handler) DatasetVersionCompare(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	left, _ := strconv.ParseUint(c.Query("left"), 10, 64)
	right, _ := strconv.ParseUint(c.Query("right"), 10, 64)
	var versions []DatasetVersion
	h.db.Where("tenant_id = ? AND project_id = ? AND id IN ?", project.TenantID, project.ID, []uint64{left, right}).Find(&versions)
	if len(versions) != 2 {
		httpx.Fail(c, 400, 400, "必须选择两个同项目版本")
		return
	}
	loadSet := func(versionID uint64) map[uint64]bool {
		var ids []uint64
		h.db.Model(&DatasetVersionItem{}).Where("tenant_id = ? AND dataset_version_id = ?", project.TenantID, versionID).Pluck("asset_id", &ids)
		result := make(map[uint64]bool, len(ids))
		for _, id := range ids {
			result[id] = true
		}
		return result
	}
	leftSet, rightSet := loadSet(left), loadSet(right)
	var added, removed int
	for id := range rightSet {
		if !leftSet[id] {
			added++
		}
	}
	for id := range leftSet {
		if !rightSet[id] {
			removed++
		}
	}
	httpx.OK(c, gin.H{"left": left, "right": right, "added": added, "removed": removed, "unchanged": len(rightSet) - added})
}

type deprecateRequest struct {
	Reason string `json:"reason"`
}

// DatasetVersionDeprecate godoc
// @Summary Deprecate a frozen version without breaking history
// @Tags VisionAI Dataset
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/dataset-versions/{versionId}/deprecate [post]
func (h *Handler) DatasetVersionDeprecate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER", "APPROVER") {
		return
	}
	version, ok := h.getDatasetVersion(c, project)
	var req deprecateRequest
	if !ok || version.Status != DatasetVersionFrozen || c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Reason) == "" {
		if ok {
			httpx.Fail(c, 409, 409, "仅 FROZEN 版本可废弃且必须填写原因")
		}
		return
	}
	now := time.Now()
	version.Status, version.DeprecatedAt, version.DeprecatedBy, version.DeprecationReason = DatasetVersionDeprecated, &now, c.GetUint64("user_id"), strings.TrimSpace(req.Reason)
	h.db.Save(&version)
	_ = appendAudit(h.db, c, project.ID, "DATASET_VERSION_DEPRECATED", "DATASET_VERSION", version.ID, nil, version)
	httpx.OK(c, version)
}
