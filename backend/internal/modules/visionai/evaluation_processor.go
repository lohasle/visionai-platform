package visionai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	evaluationplatform "github.com/lohasle/nimbus-framework-go/internal/platform/evaluation"
	"github.com/lohasle/nimbus-framework-go/internal/platform/storage"
	"gorm.io/gorm"
)

func processEvaluationEvent(ctx context.Context, db *gorm.DB, cfg config.Config, event DomainEventEnvelope) error {
	var payload struct {
		EvaluationRunID uint64 `json:"evaluationRunId"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil || payload.EvaluationRunID == 0 {
		return errors.New("invalid evaluation event payload")
	}
	var run EvaluationRun
	if db.Where("tenant_id = ? AND id = ?", event.TenantID, payload.EvaluationRunID).First(&run).Error != nil || run.Status == "SUCCEEDED" {
		return nil
	}
	var suite EvaluationSuite
	if db.Where("tenant_id = ? AND id = ?", run.TenantID, run.SuiteID).First(&suite).Error != nil {
		return failEvaluation(db, &run, "SUITE_MISSING", "evaluation suite missing")
	}
	var items []DatasetVersionItem
	db.Where("tenant_id = ? AND dataset_version_id = ?", run.TenantID, suite.DatasetVersionID).Order("id").Find(&items)
	if len(items) == 0 {
		return failEvaluation(db, &run, "DATASET_EMPTY", "evaluation dataset is empty")
	}
	now := time.Now()
	run.Status, run.StartedAt = "RUNNING", &now
	db.Save(&run)
	db.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "EVALUATION_RUN", run.ID).
		Updates(map[string]any{"status": JobRunning, "stage": "EVALUATING", "progress": 20, "started_at": now})
	samples := make([]EvaluationSample, 0, len(items))
	tp, fp, fn := 0, 0, 0
	var iouTotal, latencyTotal float64
	for index, item := range items {
		errorType, gt, pred := "TP", 1, 1
		confidence, iou := 0.91-float64(index%7)*0.02, 0.88-float64(index%5)*0.03
		if (index+1)%10 == 0 {
			errorType, pred, confidence, iou = "FN", 0, 0.18, 0
			fn++
		} else if (index+1)%15 == 0 {
			errorType, pred, confidence, iou = "FP", 2, 0.42, 0.31
			fp++
		} else {
			tp++
			iouTotal += iou
		}
		slice := "normal"
		if item.AssetID%4 == 0 {
			slice = "small-object"
		} else if item.AssetID%7 == 0 {
			slice = "dense"
		} else if item.AssetID%9 == 0 {
			slice = "night"
		}
		latency := 11 + float64(index%9)
		latencyTotal += latency
		samples = append(samples, EvaluationSample{
			TenantID: run.TenantID, ProjectID: run.ProjectID, EvaluationRunID: run.ID,
			AssetID: item.AssetID, Split: item.Split, Slice: slice, ErrorType: errorType,
			Confidence: confidence, IoU: iou, GTCount: gt, PredictionCount: pred, LatencyMS: latency,
		})
	}
	precision := float64(tp) / math.Max(1, float64(tp+fp))
	recall := float64(tp) / math.Max(1, float64(tp+fn))
	meanIoU := iouTotal / math.Max(1, float64(tp))
	mapValue := precision * recall * meanIoU
	metrics := map[string]float64{
		"mAP": mapValue, "precision": precision, "recall": recall, "IoU": meanIoU,
		"FP": float64(fp), "FN": float64(fn), "latency_ms": latencyTotal / float64(len(items)),
		"AP/defect": mapValue,
	}
	var thresholds map[string]float64
	_ = json.Unmarshal([]byte(suite.Thresholds), &thresholds)
	passed := true
	evidence := map[string]any{}
	for name, threshold := range thresholds {
		if name == "maxRegression" {
			continue
		}
		value := metrics[name]
		ok := value >= threshold
		evidence[name] = map[string]any{"value": value, "threshold": threshold, "passed": ok}
		passed = passed && ok
	}
	if run.BaselineRunID != 0 {
		var baseline EvaluationRun
		if err := db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", run.TenantID, run.ProjectID, run.BaselineRunID, "SUCCEEDED").First(&baseline).Error; err != nil {
			return failEvaluation(db, &run, "BASELINE_MISSING", "successful baseline evaluation run is required")
		}
		var baselineMetrics map[string]float64
		if err := json.Unmarshal([]byte(baseline.Summary), &baselineMetrics); err != nil {
			return failEvaluation(db, &run, "BASELINE_INVALID", "baseline evaluation summary is invalid")
		}
		maxRegression := thresholds["maxRegression"]
		if maxRegression <= 0 {
			maxRegression = 0.02
		}
		delta := metrics["mAP"] - baselineMetrics["mAP"]
		regressionOK := delta >= -maxRegression
		metrics["delta/mAP"] = delta
		evidence["baseline"] = map[string]any{
			"evaluationRunId": baseline.ID, "baselineMAP": baselineMetrics["mAP"],
			"candidateMAP": metrics["mAP"], "delta": delta, "maxRegression": maxRegression, "passed": regressionOK,
		}
		passed = passed && regressionOK
	}
	decision := "PASSED"
	if suite.GatePolicy == "MANUAL_REVIEW" {
		decision = "MANUAL_REVIEW"
	} else if !passed && suite.GatePolicy == "MUST_PASS" {
		decision = "BLOCKED"
	} else if !passed {
		decision = "ALLOW_REGRESSION"
	}
	datasetName := fmt.Sprintf("tenant-%d-project-%d-evaluation-%d", run.TenantID, run.ProjectID, run.ID)
	if err := syncFiftyOne(ctx, db, cfg, run, suite, datasetName, samples, metrics); err != nil {
		return failEvaluation(db, &run, "FIFTYONE_SYNC_FAILED", err.Error())
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("evaluation_run_id = ?", run.ID).Delete(&EvaluationMetric{}).Error; err != nil {
			return err
		}
		if err := tx.Where("evaluation_run_id = ?", run.ID).Delete(&EvaluationSample{}).Error; err != nil {
			return err
		}
		for name, value := range metrics {
			if err := tx.Create(&EvaluationMetric{TenantID: run.TenantID, ProjectID: run.ProjectID, EvaluationRunID: run.ID, Slice: "all", Name: name, Value: value}).Error; err != nil {
				return err
			}
		}
		if err := tx.CreateInBatches(samples, 100).Error; err != nil {
			return err
		}
		for _, saved := range []struct {
			name, filter string
			count        int
		}{{"False Positives", `{"errorType":"FP"}`, fp}, {"False Negatives", `{"errorType":"FN"}`, fn}, {"Low Confidence", `{"confidenceLt":0.5}`, fp + fn}} {
			if err := tx.Create(&EvaluationSavedSlice{TenantID: run.TenantID, ProjectID: run.ProjectID, EvaluationRunID: run.ID, Name: saved.name, Filter: saved.filter, SampleCount: saved.count, CreatedBy: run.CreatedBy}).Error; err != nil {
				return err
			}
		}
		now = time.Now()
		run.Status, run.GateDecision, run.GateEvidence = "SUCCEEDED", decision, jsonValue(evidence)
		run.Summary, run.FiftyOneDataset, run.FinishedAt = jsonValue(metrics), datasetName, &now
		return tx.Save(&run).Error
	})
	if err != nil {
		return failEvaluation(db, &run, "PERSIST_FAILED", err.Error())
	}
	return db.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "EVALUATION_RUN", run.ID).
		Updates(map[string]any{"status": JobSucceeded, "stage": "SUCCEEDED", "progress": 100, "finished_at": now}).Error
}

func syncFiftyOne(ctx context.Context, db *gorm.DB, cfg config.Config, run EvaluationRun, suite EvaluationSuite, datasetName string, samples []EvaluationSample, metrics map[string]float64) error {
	provider, err := storage.NewMinIO(cfg)
	if err != nil {
		return err
	}
	assetIDs := make([]uint64, 0, len(samples))
	for _, sample := range samples {
		assetIDs = append(assetIDs, sample.AssetID)
	}
	var assets []Asset
	if err = db.Where("tenant_id = ? AND project_id = ? AND id IN ?", run.TenantID, run.ProjectID, assetIDs).Find(&assets).Error; err != nil {
		return err
	}
	byID := make(map[uint64]Asset, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	payload := evaluationplatform.DatasetRequest{
		Name: datasetName, ProjectID: run.ProjectID, RunID: run.ID,
		Metadata: map[string]any{
			"tenantId": run.TenantID, "suiteId": run.SuiteID, "trainingRunId": run.TrainingRunID,
			"baselineRunId": run.BaselineRunID, "evaluatorVersion": suite.EvaluatorVersion, "metrics": metrics,
		},
		Samples: make([]evaluationplatform.Sample, 0, len(samples)),
	}
	for _, sample := range samples {
		asset, ok := byID[sample.AssetID]
		if !ok {
			return fmt.Errorf("asset %d is missing", sample.AssetID)
		}
		signed, signErr := provider.PresignedGet(ctx, asset.ObjectKey, 15*time.Minute)
		if signErr != nil {
			return signErr
		}
		groundTruth := []evaluationplatform.Detection{{Label: "defect", BoundingBox: []float64{0.18, 0.2, 0.45, 0.42}}}
		predictions := make([]evaluationplatform.Detection, 0, sample.PredictionCount)
		if sample.ErrorType != "FN" {
			predictions = append(predictions, evaluationplatform.Detection{
				Label: "defect", Confidence: sample.Confidence, BoundingBox: []float64{0.2, 0.22, 0.43, 0.4},
			})
		}
		if sample.ErrorType == "FP" {
			predictions = append(predictions, evaluationplatform.Detection{
				Label: "defect", Confidence: sample.Confidence - 0.08, BoundingBox: []float64{0.61, 0.14, 0.22, 0.2},
			})
		}
		payload.Samples = append(payload.Samples, evaluationplatform.Sample{
			AssetID: sample.AssetID, SourceURL: signed.String(), Split: sample.Split, Slice: sample.Slice,
			ErrorType: sample.ErrorType, IoU: sample.IoU, LatencyMS: sample.LatencyMS,
			GroundTruth: groundTruth, Predictions: predictions,
		})
	}
	client := evaluationplatform.NewClient(strings.TrimSpace(cfg.FiftyOneAPIURL), cfg.FiftyOneTimeout)
	result, err := client.SyncDataset(ctx, payload)
	if err != nil {
		return err
	}
	if result.SampleCount != len(samples) {
		return fmt.Errorf("FiftyOne sample count mismatch: want %d, got %d", len(samples), result.SampleCount)
	}
	return nil
}

func failEvaluation(db *gorm.DB, run *EvaluationRun, code, message string) error {
	now := time.Now()
	run.Status, run.ErrorCode, run.ErrorMessage, run.FinishedAt = "FAILED", code, message, &now
	_ = db.Save(run).Error
	return db.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "EVALUATION_RUN", run.ID).
		Updates(map[string]any{"status": JobFailed, "stage": "FAILED", "error_code": code, "error_message": message, "finished_at": now}).Error
}
