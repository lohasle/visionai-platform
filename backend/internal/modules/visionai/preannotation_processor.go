package visionai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/annotation"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	platforminference "github.com/lohasle/nimbus-framework-go/internal/platform/inference"
	"github.com/lohasle/nimbus-framework-go/internal/platform/storage"
	"gorm.io/gorm"
)

type preannotationPayload struct {
	PreannotationRunID uint64 `json:"preannotationRunId"`
}

type cvatPreannotationSnapshot struct {
	Version int                      `json:"version"`
	Tags    []any                    `json:"tags"`
	Shapes  []cvatPreannotationShape `json:"shapes"`
	Tracks  []any                    `json:"tracks"`
}

type cvatPreannotationShape struct {
	Type     string    `json:"type"`
	Occluded bool      `json:"occluded"`
	Outside  bool      `json:"outside"`
	ZOrder   int       `json:"z_order"`
	Rotation float64   `json:"rotation"`
	Points   []float64 `json:"points"`
	Frame    int       `json:"frame"`
	LabelID  int64     `json:"label_id"`
	Source   string    `json:"source"`
}

func processPreannotationEvent(ctx context.Context, db *gorm.DB, cfg config.Config, event DomainEventEnvelope) error {
	var payload preannotationPayload
	if json.Unmarshal(event.Payload, &payload) != nil || payload.PreannotationRunID == 0 {
		return errors.New("invalid pre-annotation event payload")
	}
	var run PreannotationRun
	if db.Where("tenant_id = ? AND id = ?", event.TenantID, payload.PreannotationRunID).First(&run).Error != nil {
		return nil
	}
	if run.Status == "IMPORTED" || run.Status == "SUCCEEDED" {
		return nil
	}
	fail := func(code string, cause error) error {
		now := time.Now()
		db.Model(&run).Updates(map[string]any{
			"status": "FAILED", "error_code": code, "error_message": cause.Error(),
		})
		db.Model(&AnnotationTask{}).Where("tenant_id = ? AND id = ? AND status = ?", run.TenantID, run.AnnotationTaskID, AnnotationPreannotating).
			Updates(map[string]any{"status": AnnotationFailed, "error_code": code, "error_message": cause.Error()})
		db.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "PREANNOTATION_RUN", run.ID).
			Updates(map[string]any{"status": JobFailed, "stage": "FAILED", "error_code": code, "error_message": cause.Error(), "finished_at": now})
		return nil
	}
	var parameters preannotationParameters
	if json.Unmarshal([]byte(run.Parameters), &parameters) != nil {
		return fail("PARAMETERS_INVALID", errors.New("stored pre-annotation parameters are invalid"))
	}
	var task AnnotationTask
	if db.Where("tenant_id = ? AND project_id = ? AND id = ?", run.TenantID, run.ProjectID, run.AnnotationTaskID).First(&task).Error != nil {
		return fail("ANNOTATION_TASK_MISSING", errors.New("annotation task is missing"))
	}
	var binding ExternalResourceBinding
	if task.ExternalBindingID == 0 || db.Where("tenant_id = ? AND id = ? AND provider_type = ?", run.TenantID, task.ExternalBindingID, "CVAT").First(&binding).Error != nil {
		return fail("CVAT_BINDING_MISSING", errors.New("annotation task has no prepared CVAT binding"))
	}
	externalTaskID, err := strconv.ParseInt(binding.ExternalID, 10, 64)
	if err != nil || externalTaskID == 0 {
		return fail("CVAT_BINDING_INVALID", errors.New("CVAT task identifier is invalid"))
	}
	var model ModelVersion
	if db.Where(
		"tenant_id = ? AND project_id = ? AND id = ? AND preannotation_approved = ?",
		run.TenantID, run.ProjectID, run.ModelVersionID, true,
	).First(&model).Error != nil || !modelProductionEligible(model) || model.TrainingRunID == 0 {
		return fail("MODEL_NOT_ELIGIBLE", errors.New("approved trained model is required"))
	}
	var weights TrainingArtifact
	if db.Where(
		"tenant_id = ? AND project_id = ? AND training_run_id = ? AND kind = ?",
		run.TenantID, run.ProjectID, model.TrainingRunID, "WEIGHTS",
	).Order("id").First(&weights).Error != nil {
		return fail("WEIGHTS_MISSING", errors.New("model weights artifact is missing"))
	}
	objectStore, err := storage.NewMinIO(cfg)
	if err != nil {
		return fail("STORAGE_CONFIG_INVALID", err)
	}
	provider, err := annotation.NewCVAT(cfg.CVATBaseURL, cfg.CVATPublicURL, cfg.CVATUsername, cfg.CVATPassword, cfg.CVATTimeout)
	if err != nil {
		return fail("CVAT_CONFIG_INVALID", err)
	}
	labels, err := provider.GetTaskLabels(ctx, externalTaskID)
	if err != nil {
		return fail("CVAT_LABELS_UNAVAILABLE", err)
	}
	labelByName := make(map[string]annotation.Label, len(labels))
	for _, label := range labels {
		labelByName[strings.ToLower(strings.TrimSpace(label.Name))] = label
	}
	var items []AssetCollectionItem
	if db.Where("tenant_id = ? AND collection_id = ?", run.TenantID, task.CollectionID).Order("id").Find(&items).Error != nil || len(items) == 0 {
		return fail("COLLECTION_EMPTY", errors.New("annotation collection is empty"))
	}
	assetIDs := make([]uint64, 0, len(items))
	for _, item := range items {
		assetIDs = append(assetIDs, item.AssetID)
	}
	var assets []Asset
	if db.Where("tenant_id = ? AND project_id = ? AND id IN ? AND status = ?", run.TenantID, run.ProjectID, assetIDs, AssetReady).Find(&assets).Error != nil {
		return fail("ASSET_QUERY_FAILED", errors.New("assets could not be loaded"))
	}
	assetByID := make(map[uint64]Asset, len(assets))
	for _, asset := range assets {
		assetByID[asset.ID] = asset
	}
	now := time.Now()
	run.Status = "RUNNING"
	db.Model(&run).Updates(map[string]any{"status": "RUNNING", "error_code": "", "error_message": ""})
	db.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "PREANNOTATION_RUN", run.ID).
		Updates(map[string]any{"status": JobRunning, "stage": "LOADING_MODEL", "progress": 5, "total": len(items), "started_at": now})
	client := platforminference.NewClient(cfg.InferenceAPIURL, cfg.InferenceTimeout)
	inferenceKey := uint64(1_000_000_000) + run.ID
	if err = client.LoadEvaluation(ctx, inferenceKey, platforminference.EvaluationModel{
		TrainingRunID: model.TrainingRunID, ArtifactURI: weights.URI, ArtifactSHA256: weights.SHA256,
	}); err != nil {
		return fail("MODEL_LOAD_FAILED", err)
	}
	snapshot := cvatPreannotationSnapshot{Version: 0, Tags: []any{}, Shapes: []cvatPreannotationShape{}, Tracks: []any{}}
	byClass, byScene := map[string]int64{}, map[string]int64{}
	lowConfidenceCount := int64(0)
	for frame, item := range items {
		asset, exists := assetByID[item.AssetID]
		if !exists {
			return fail("ASSET_MISSING", fmt.Errorf("asset %d is unavailable", item.AssetID))
		}
		object, _, getErr := objectStore.Get(ctx, asset.ObjectKey)
		if getErr != nil {
			return fail("ASSET_READ_FAILED", getErr)
		}
		prediction, predictErr := client.PredictEvaluation(ctx, inferenceKey, model.TrainingRunID, weights.SHA256, asset.Filename, object)
		_ = object.Close()
		if predictErr != nil {
			return fail("INFERENCE_FAILED", fmt.Errorf("asset %d: %w", asset.ID, predictErr))
		}
		detections := filterPreannotationDetections(prediction.Detections, parameters)
		for _, detection := range detections {
			target := strings.TrimSpace(detection.Label)
			if mapped := strings.TrimSpace(parameters.ClassMapping[detection.LabelCode]); mapped != "" {
				target = mapped
			} else if mapped = strings.TrimSpace(parameters.ClassMapping[detection.Label]); mapped != "" {
				target = mapped
			}
			label, found := labelByName[strings.ToLower(target)]
			if !found || len(detection.BBox) != 4 {
				continue
			}
			if detection.Confidence < parameters.Confidence {
				lowConfidenceCount++
			}
			snapshot.Shapes = append(snapshot.Shapes, cvatPreannotationShape{
				Type: "rectangle", Occluded: false, Outside: false, ZOrder: 0, Rotation: 0,
				Points: detection.BBox, Frame: frame, LabelID: label.ID, Source: "auto",
			})
			byClass[label.Name]++
			byScene[assetScene(asset)]++
		}
		progress := 10 + int(float64(frame+1)/float64(len(items))*75)
		db.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "PREANNOTATION_RUN", run.ID).
			Updates(map[string]any{"stage": "INFERENCING", "progress": progress, "processed": frame + 1})
	}
	raw, _ := json.Marshal(snapshot)
	if err = provider.PutAnnotations(ctx, externalTaskID, raw); err != nil {
		return fail("CVAT_IMPORT_FAILED", err)
	}
	finished := time.Now()
	breakdown := jsonValue(map[string]any{
		"byClass": byClass, "byScene": byScene, "lowConfidenceCount": lowConfidenceCount,
		"modelVersionId": model.ID, "provider": "ORCHESTRATOR", "device": parameters.Device,
		"batchSize": parameters.BatchSize, "cvatTaskId": externalTaskID,
	})
	if err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&run).Updates(map[string]any{
			"status": "IMPORTED", "proposed_count": len(snapshot.Shapes), "output_snapshot": string(raw),
			"breakdown": breakdown, "imported_at": finished, "error_code": "", "error_message": "",
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&task).Updates(map[string]any{
			"status": AnnotationReady, "progress": 0, "error_code": "", "error_message": "",
		}).Error; err != nil {
			return err
		}
		return tx.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "PREANNOTATION_RUN", run.ID).
			Updates(map[string]any{"status": JobSucceeded, "stage": "IMPORTED_TO_CVAT", "progress": 100, "processed": len(items), "finished_at": finished}).Error
	}); err != nil {
		return err
	}
	return nil
}

func filterPreannotationDetections(input []platforminference.Detection, parameters preannotationParameters) []platforminference.Detection {
	floor := parameters.Confidence
	if parameters.LowConfidencePolicy == "KEEP_REVIEW" {
		floor = parameters.LowConfidenceFloor
	}
	byClass := map[string][]platforminference.Detection{}
	for _, detection := range input {
		if len(detection.BBox) != 4 || detection.Confidence < floor {
			continue
		}
		key := detection.LabelCode
		if key == "" {
			key = detection.Label
		}
		byClass[key] = append(byClass[key], detection)
	}
	result := make([]platforminference.Detection, 0, len(input))
	for _, values := range byClass {
		sort.Slice(values, func(i, j int) bool { return values[i].Confidence > values[j].Confidence })
		kept := make([]platforminference.Detection, 0, len(values))
		for _, candidate := range values {
			suppressed := false
			for _, selected := range kept {
				if boxIoU(candidate.BBox, selected.BBox) > parameters.NMSIoU {
					suppressed = true
					break
				}
			}
			if !suppressed {
				kept = append(kept, candidate)
			}
		}
		result = append(result, kept...)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Confidence > result[j].Confidence })
	return result
}

func assetScene(asset Asset) string {
	var metadata map[string]any
	if json.Unmarshal([]byte(asset.Metadata), &metadata) == nil {
		for _, key := range []string{"businessScene", "scene"} {
			if value := strings.TrimSpace(fmt.Sprint(metadata[key])); value != "" && value != "<nil>" {
				return value
			}
		}
	}
	return "未分类"
}
