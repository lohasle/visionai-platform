package visionai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	evaluationplatform "github.com/lohasle/nimbus-framework-go/internal/platform/evaluation"
	platforminference "github.com/lohasle/nimbus-framework-go/internal/platform/inference"
	"github.com/lohasle/nimbus-framework-go/internal/platform/storage"
	"gorm.io/gorm"
)

type evaluatedDetection struct {
	ClassIndex int
	LabelCode  string
	LabelName  string
	Confidence float64
	XYXY       []float64
}

type evaluatedSample struct {
	Row         EvaluationSample
	Width       int
	Height      int
	GroundTruth []evaluatedDetection
	Predictions []evaluatedDetection
}

type classPrediction struct {
	Confidence float64
	Matched    bool
}

type detectionAccumulator struct {
	GroundTruthByClass map[int]int
	PredictionsByClass map[int][]classPrediction
	ClassCode          map[int]string
	TP                 int
	FP                 int
	FN                 int
	IoUTotal           float64
	MatchedCount       int
	LatencyTotal       float64
}

const evaluationOperatingConfidence = 0.25

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
	var training TrainingRun
	if db.Where(
		"tenant_id = ? AND project_id = ? AND id = ? AND dataset_version_id = ? AND status = ?",
		run.TenantID, run.ProjectID, run.TrainingRunID, suite.DatasetVersionID, TrainingSucceeded,
	).First(&training).Error != nil {
		return failEvaluation(db, &run, "TRAINING_RUN_INVALID", "successful training run for the evaluation dataset is required")
	}
	var weights TrainingArtifact
	if db.Where(
		"tenant_id = ? AND project_id = ? AND training_run_id = ? AND kind = ?",
		run.TenantID, run.ProjectID, training.ID, "WEIGHTS",
	).Order("id").First(&weights).Error != nil {
		return failEvaluation(db, &run, "WEIGHTS_MISSING", "training weights artifact is missing")
	}
	var indexArtifact TrainingArtifact
	if db.Where(
		"tenant_id = ? AND project_id = ? AND training_run_id = ? AND kind = ?",
		run.TenantID, run.ProjectID, training.ID, "TRAINING_INDEX",
	).Order("id").First(&indexArtifact).Error != nil {
		return failEvaluation(db, &run, "TRAINING_INDEX_MISSING", "training detection index artifact is missing")
	}
	objectStore, err := storage.NewMinIO(cfg)
	if err != nil {
		return failEvaluation(db, &run, "STORAGE_CONFIG_INVALID", err.Error())
	}
	index, err := loadEvaluationIndex(ctx, objectStore, cfg.S3Bucket, indexArtifact)
	if err != nil {
		return failEvaluation(db, &run, "TRAINING_INDEX_INVALID", err.Error())
	}
	if index.DatasetVersionID != suite.DatasetVersionID || index.DatasetChecksum == "" {
		return failEvaluation(db, &run, "DATASET_MISMATCH", "evaluation suite does not match the immutable training input")
	}
	if len(index.Items) == 0 {
		return failEvaluation(db, &run, "DATASET_EMPTY", "evaluation dataset is empty")
	}

	now := time.Now()
	run.Status, run.StartedAt = "RUNNING", &now
	db.Save(&run)
	db.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "EVALUATION_RUN", run.ID).
		Updates(map[string]any{"status": JobRunning, "stage": "LOADING_MODEL", "progress": 10, "started_at": now})
	inferenceClient := platforminference.NewClient(cfg.InferenceAPIURL, cfg.InferenceTimeout)
	if err = inferenceClient.LoadEvaluation(ctx, run.ID, platforminference.EvaluationModel{
		TrainingRunID: training.ID, ArtifactURI: weights.URI, ArtifactSHA256: weights.SHA256,
	}); err != nil {
		return failEvaluation(db, &run, "MODEL_LOAD_FAILED", err.Error())
	}
	db.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "EVALUATION_RUN", run.ID).
		Updates(map[string]any{"stage": "EVALUATING", "progress": 20})

	assets, err := loadEvaluationAssets(db, run, index.Items)
	if err != nil {
		return failEvaluation(db, &run, "ASSET_MISSING", err.Error())
	}
	sliceByAsset := loadEvaluationSlices(db, run, suite, index.Items)
	accumulator := detectionAccumulator{
		GroundTruthByClass: make(map[int]int),
		PredictionsByClass: make(map[int][]classPrediction),
		ClassCode:          make(map[int]string),
	}
	evaluated := make([]evaluatedSample, 0, len(index.Items))
	for itemIndex, item := range index.Items {
		asset := assets[item.AssetID]
		object, _, getErr := objectStore.Get(ctx, asset.ObjectKey)
		if getErr != nil {
			return failEvaluation(db, &run, "ASSET_READ_FAILED", fmt.Sprintf("asset %d: %v", asset.ID, getErr))
		}
		prediction, predictErr := inferenceClient.PredictEvaluation(
			ctx, run.ID, training.ID, weights.SHA256, asset.Filename, object,
		)
		_ = object.Close()
		if predictErr != nil {
			return failEvaluation(db, &run, "INFERENCE_FAILED", fmt.Sprintf("asset %d: %v", asset.ID, predictErr))
		}
		groundTruth := make([]evaluatedDetection, 0, len(item.Boxes))
		for _, box := range item.Boxes {
			groundTruth = append(groundTruth, evaluatedDetection{
				ClassIndex: box.ClassIndex, LabelCode: box.LabelCode, LabelName: box.LabelName, XYXY: box.XYXY,
			})
			accumulator.GroundTruthByClass[box.ClassIndex]++
			accumulator.ClassCode[box.ClassIndex] = box.LabelCode
		}
		predictions := make([]evaluatedDetection, 0, len(prediction.Detections))
		for _, detected := range prediction.Detections {
			if len(detected.BBox) != 4 {
				continue
			}
			predictions = append(predictions, evaluatedDetection{
				ClassIndex: detected.ClassIndex, LabelCode: detected.LabelCode, LabelName: detected.Label,
				Confidence: detected.Confidence, XYXY: detected.BBox,
			})
			if detected.LabelCode != "" {
				accumulator.ClassCode[detected.ClassIndex] = detected.LabelCode
			}
		}
		row, matchedIoU := scoreEvaluationSample(run, item, sliceByAsset[item.AssetID], groundTruth, predictions, &accumulator)
		row.LatencyMS = prediction.LatencyMS
		accumulator.LatencyTotal += prediction.LatencyMS
		if matchedIoU > 0 {
			row.IoU = matchedIoU
		}
		evaluated = append(evaluated, evaluatedSample{
			Row: row, Width: asset.Width, Height: asset.Height, GroundTruth: groundTruth, Predictions: predictions,
		})
		if (itemIndex+1)%10 == 0 || itemIndex+1 == len(index.Items) {
			progress := 20 + int(float64(itemIndex+1)/float64(len(index.Items))*55)
			db.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "EVALUATION_RUN", run.ID).
				Updates(map[string]any{"progress": progress})
		}
	}
	metrics := calculateDetectionMetrics(accumulator, len(index.Items))
	decision, evidence, gateErr := evaluateGate(db, run, suite, metrics)
	if gateErr != nil {
		return failEvaluation(db, &run, "BASELINE_INVALID", gateErr.Error())
	}
	datasetName := fmt.Sprintf("tenant-%d-project-%d-evaluation-%d", run.TenantID, run.ProjectID, run.ID)
	if err = syncFiftyOne(ctx, db, cfg, run, suite, datasetName, evaluated, metrics); err != nil {
		return failEvaluation(db, &run, "FIFTYONE_SYNC_FAILED", err.Error())
	}
	if err = persistEvaluation(db, &run, evaluated, metrics, decision, evidence, datasetName); err != nil {
		return failEvaluation(db, &run, "PERSIST_FAILED", err.Error())
	}
	now = *run.FinishedAt
	return db.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "EVALUATION_RUN", run.ID).
		Updates(map[string]any{"status": JobSucceeded, "stage": "SUCCEEDED", "progress": 100, "finished_at": now}).Error
}

func loadEvaluationIndex(ctx context.Context, objectStore storage.Provider, bucket string, artifact TrainingArtifact) (detectionTrainingIndex, error) {
	var result detectionTrainingIndex
	prefix := "s3://" + bucket + "/"
	if !strings.HasPrefix(artifact.URI, prefix) {
		return result, errors.New("training index artifact URI is not in the governed object store")
	}
	reader, _, err := objectStore.Get(ctx, strings.TrimPrefix(artifact.URI, prefix))
	if err != nil {
		return result, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(reader, 128<<20))
	_ = reader.Close()
	if readErr != nil {
		return result, readErr
	}
	if digestBytes(raw) != strings.ToLower(artifact.SHA256) {
		return result, errors.New("training index artifact checksum mismatch")
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	if result.SchemaVersion != "visionai.detection-training-input.v1" {
		return result, errors.New("unsupported training index schema")
	}
	return result, nil
}

func loadEvaluationAssets(db *gorm.DB, run EvaluationRun, items []detectionTrainingItem) (map[uint64]Asset, error) {
	ids := make([]uint64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.AssetID)
	}
	var assets []Asset
	if err := db.Where(
		"tenant_id = ? AND project_id = ? AND id IN ? AND status = ?",
		run.TenantID, run.ProjectID, ids, AssetReady,
	).Find(&assets).Error; err != nil {
		return nil, err
	}
	result := make(map[uint64]Asset, len(assets))
	for _, asset := range assets {
		result[asset.ID] = asset
	}
	if len(result) != len(items) {
		return nil, fmt.Errorf("expected %d ready assets, found %d", len(items), len(result))
	}
	return result, nil
}

func loadEvaluationSlices(db *gorm.DB, run EvaluationRun, suite EvaluationSuite, items []detectionTrainingItem) map[uint64]string {
	var requested []string
	_ = json.Unmarshal([]byte(suite.Slices), &requested)
	requestedSet := make(map[string]bool, len(requested))
	for _, value := range requested {
		requestedSet[strings.ToLower(strings.TrimSpace(value))] = true
	}
	ids := make([]uint64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.AssetID)
	}
	var tags []AssetTag
	db.Where("tenant_id = ? AND project_id = ? AND asset_id IN ?", run.TenantID, run.ProjectID, ids).Order("id").Find(&tags)
	result := make(map[uint64]string, len(items))
	for _, item := range items {
		result[item.AssetID] = "all"
	}
	for _, tag := range tags {
		if requestedSet[strings.ToLower(tag.Tag)] {
			result[tag.AssetID] = tag.Tag
		}
	}
	return result
}

func scoreEvaluationSample(
	run EvaluationRun,
	item detectionTrainingItem,
	slice string,
	groundTruth []evaluatedDetection,
	predictions []evaluatedDetection,
	accumulator *detectionAccumulator,
) (EvaluationSample, float64) {
	sort.Slice(predictions, func(i, j int) bool { return predictions[i].Confidence > predictions[j].Confidence })
	usedForAP := make([]bool, len(groundTruth))
	for _, prediction := range predictions {
		bestIndex, bestIoU := -1, 0.0
		for index, truth := range groundTruth {
			if usedForAP[index] || truth.ClassIndex != prediction.ClassIndex {
				continue
			}
			if value := boxIoU(truth.XYXY, prediction.XYXY); value > bestIoU {
				bestIndex, bestIoU = index, value
			}
		}
		matched := bestIndex >= 0 && bestIoU >= 0.5
		accumulator.PredictionsByClass[prediction.ClassIndex] = append(
			accumulator.PredictionsByClass[prediction.ClassIndex],
			classPrediction{Confidence: prediction.Confidence, Matched: matched},
		)
		if matched {
			usedForAP[bestIndex] = true
		}
	}

	operatingPredictions := make([]evaluatedDetection, 0, len(predictions))
	for _, prediction := range predictions {
		if prediction.Confidence >= evaluationOperatingConfidence {
			operatingPredictions = append(operatingPredictions, prediction)
		}
	}
	used := make([]bool, len(groundTruth))
	tp, fp := 0, 0
	var iouTotal, confidenceTotal float64
	for _, prediction := range operatingPredictions {
		confidenceTotal += prediction.Confidence
		bestIndex, bestIoU := -1, 0.0
		for index, truth := range groundTruth {
			if used[index] || truth.ClassIndex != prediction.ClassIndex {
				continue
			}
			if value := boxIoU(truth.XYXY, prediction.XYXY); value > bestIoU {
				bestIndex, bestIoU = index, value
			}
		}
		if bestIndex >= 0 && bestIoU >= 0.5 {
			used[bestIndex] = true
			tp++
			iouTotal += bestIoU
			accumulator.IoUTotal += bestIoU
			accumulator.MatchedCount++
		} else {
			fp++
		}
	}
	fn := len(groundTruth) - tp
	accumulator.TP += tp
	accumulator.FP += fp
	accumulator.FN += fn
	errorType := "TP"
	switch {
	case fn > 0:
		errorType = "FN"
	case fp > 0:
		errorType = "FP"
	case len(groundTruth) == 0 && len(predictions) == 0:
		errorType = "TN"
	}
	meanConfidence := confidenceTotal / math.Max(1, float64(len(operatingPredictions)))
	meanIoU := iouTotal / math.Max(1, float64(tp))
	return EvaluationSample{
		TenantID: run.TenantID, ProjectID: run.ProjectID, EvaluationRunID: run.ID,
		AssetID: item.AssetID, Split: item.Split, Slice: slice, ErrorType: errorType,
		Confidence: meanConfidence, IoU: meanIoU, GTCount: len(groundTruth), PredictionCount: len(operatingPredictions),
	}, meanIoU
}

func boxIoU(left, right []float64) float64 {
	if len(left) != 4 || len(right) != 4 {
		return 0
	}
	x1, y1 := math.Max(left[0], right[0]), math.Max(left[1], right[1])
	x2, y2 := math.Min(left[2], right[2]), math.Min(left[3], right[3])
	intersection := math.Max(0, x2-x1) * math.Max(0, y2-y1)
	leftArea := math.Max(0, left[2]-left[0]) * math.Max(0, left[3]-left[1])
	rightArea := math.Max(0, right[2]-right[0]) * math.Max(0, right[3]-right[1])
	return intersection / math.Max(1e-12, leftArea+rightArea-intersection)
}

func averagePrecision(predictions []classPrediction, groundTruthCount int) float64 {
	if groundTruthCount == 0 {
		return 0
	}
	sort.Slice(predictions, func(i, j int) bool { return predictions[i].Confidence > predictions[j].Confidence })
	recall := make([]float64, 0, len(predictions)+2)
	precision := make([]float64, 0, len(predictions)+2)
	recall, precision = append(recall, 0), append(precision, 0)
	tp, fp := 0, 0
	for _, prediction := range predictions {
		if prediction.Matched {
			tp++
		} else {
			fp++
		}
		recall = append(recall, float64(tp)/float64(groundTruthCount))
		precision = append(precision, float64(tp)/math.Max(1, float64(tp+fp)))
	}
	recall, precision = append(recall, 1), append(precision, 0)
	for index := len(precision) - 2; index >= 0; index-- {
		precision[index] = math.Max(precision[index], precision[index+1])
	}
	ap := 0.0
	for index := 1; index < len(recall); index++ {
		if recall[index] != recall[index-1] {
			ap += (recall[index] - recall[index-1]) * precision[index]
		}
	}
	return ap
}

func calculateDetectionMetrics(accumulator detectionAccumulator, sampleCount int) map[string]float64 {
	precision := float64(accumulator.TP) / math.Max(1, float64(accumulator.TP+accumulator.FP))
	recall := float64(accumulator.TP) / math.Max(1, float64(accumulator.TP+accumulator.FN))
	metrics := map[string]float64{
		"precision":            precision,
		"recall":               recall,
		"IoU":                  accumulator.IoUTotal / math.Max(1, float64(accumulator.MatchedCount)),
		"TP":                   float64(accumulator.TP),
		"FP":                   float64(accumulator.FP),
		"FN":                   float64(accumulator.FN),
		"latency_ms":           accumulator.LatencyTotal / math.Max(1, float64(sampleCount)),
		"operating_confidence": evaluationOperatingConfidence,
	}
	mapTotal, evaluatedClasses := 0.0, 0
	for classIndex, count := range accumulator.GroundTruthByClass {
		if count == 0 {
			continue
		}
		ap := averagePrecision(accumulator.PredictionsByClass[classIndex], count)
		code := accumulator.ClassCode[classIndex]
		if code == "" {
			code = fmt.Sprintf("class-%d", classIndex)
		}
		metrics["AP/"+code] = ap
		mapTotal += ap
		evaluatedClasses++
	}
	metrics["mAP"] = mapTotal / math.Max(1, float64(evaluatedClasses))
	return metrics
}

func evaluateGate(db *gorm.DB, run EvaluationRun, suite EvaluationSuite, metrics map[string]float64) (string, map[string]any, error) {
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
		if err := db.Where(
			"tenant_id = ? AND project_id = ? AND id = ? AND status = ?",
			run.TenantID, run.ProjectID, run.BaselineRunID, "SUCCEEDED",
		).First(&baseline).Error; err != nil {
			return "", nil, errors.New("successful baseline evaluation run is required")
		}
		var baselineMetrics map[string]float64
		if err := json.Unmarshal([]byte(baseline.Summary), &baselineMetrics); err != nil {
			return "", nil, errors.New("baseline evaluation summary is invalid")
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
	return decision, evidence, nil
}

func persistEvaluation(
	db *gorm.DB,
	run *EvaluationRun,
	evaluated []evaluatedSample,
	metrics map[string]float64,
	decision string,
	evidence map[string]any,
	datasetName string,
) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("evaluation_run_id = ?", run.ID).Delete(&EvaluationMetric{}).Error; err != nil {
			return err
		}
		if err := tx.Where("evaluation_run_id = ?", run.ID).Delete(&EvaluationSample{}).Error; err != nil {
			return err
		}
		if err := tx.Where("evaluation_run_id = ?", run.ID).Delete(&EvaluationSavedSlice{}).Error; err != nil {
			return err
		}
		for name, value := range metrics {
			if err := tx.Create(&EvaluationMetric{
				TenantID: run.TenantID, ProjectID: run.ProjectID, EvaluationRunID: run.ID,
				Slice: "all", Name: name, Value: value,
			}).Error; err != nil {
				return err
			}
		}
		rows := make([]EvaluationSample, 0, len(evaluated))
		fpSamples, fnSamples, lowConfidence := 0, 0, 0
		for _, item := range evaluated {
			rows = append(rows, item.Row)
			if item.Row.ErrorType == "FP" {
				fpSamples++
			}
			if item.Row.ErrorType == "FN" {
				fnSamples++
			}
			if item.Row.Confidence < 0.5 {
				lowConfidence++
			}
		}
		if err := tx.CreateInBatches(rows, 100).Error; err != nil {
			return err
		}
		for _, saved := range []struct {
			name, filter string
			count        int
		}{
			{"False Positives", `{"errorType":"FP"}`, fpSamples},
			{"False Negatives", `{"errorType":"FN"}`, fnSamples},
			{"Low Confidence", `{"confidenceLt":0.5}`, lowConfidence},
		} {
			if err := tx.Create(&EvaluationSavedSlice{
				TenantID: run.TenantID, ProjectID: run.ProjectID, EvaluationRunID: run.ID,
				Name: saved.name, Filter: saved.filter, SampleCount: saved.count, CreatedBy: run.CreatedBy,
			}).Error; err != nil {
				return err
			}
		}
		now := time.Now()
		run.Status, run.GateDecision, run.GateEvidence = "SUCCEEDED", decision, jsonValue(evidence)
		run.Summary, run.FiftyOneDataset, run.FinishedAt = jsonValue(metrics), datasetName, &now
		return tx.Save(run).Error
	})
}

func normalizeEvaluationBox(box []float64, width, height int) []float64 {
	if len(box) != 4 || width <= 0 || height <= 0 {
		return []float64{0, 0, 0, 0}
	}
	return []float64{
		box[0] / float64(width),
		box[1] / float64(height),
		(box[2] - box[0]) / float64(width),
		(box[3] - box[1]) / float64(height),
	}
}

func syncFiftyOne(
	ctx context.Context,
	db *gorm.DB,
	cfg config.Config,
	run EvaluationRun,
	suite EvaluationSuite,
	datasetName string,
	evaluated []evaluatedSample,
	metrics map[string]float64,
) error {
	provider, err := storage.NewMinIO(cfg)
	if err != nil {
		return err
	}
	assetIDs := make([]uint64, 0, len(evaluated))
	for _, sample := range evaluated {
		assetIDs = append(assetIDs, sample.Row.AssetID)
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
			"baselineRunId": run.BaselineRunID, "evaluatorVersion": suite.EvaluatorVersion,
			"metrics": metrics, "source": "real-torchvision-inference",
		},
		Samples: make([]evaluationplatform.Sample, 0, len(evaluated)),
	}
	for _, sample := range evaluated {
		asset, ok := byID[sample.Row.AssetID]
		if !ok {
			return fmt.Errorf("asset %d is missing", sample.Row.AssetID)
		}
		signed, signErr := provider.PresignedGet(ctx, asset.ObjectKey, 15*time.Minute)
		if signErr != nil {
			return signErr
		}
		groundTruth := make([]evaluationplatform.Detection, 0, len(sample.GroundTruth))
		for _, detection := range sample.GroundTruth {
			groundTruth = append(groundTruth, evaluationplatform.Detection{
				Label: detection.LabelName, BoundingBox: normalizeEvaluationBox(detection.XYXY, sample.Width, sample.Height),
			})
		}
		predictions := make([]evaluationplatform.Detection, 0, len(sample.Predictions))
		for _, detection := range sample.Predictions {
			predictions = append(predictions, evaluationplatform.Detection{
				Label: detection.LabelName, Confidence: detection.Confidence,
				BoundingBox: normalizeEvaluationBox(detection.XYXY, sample.Width, sample.Height),
			})
		}
		payload.Samples = append(payload.Samples, evaluationplatform.Sample{
			AssetID: sample.Row.AssetID, SourceURL: signed.String(), Split: sample.Row.Split,
			Slice: sample.Row.Slice, ErrorType: sample.Row.ErrorType, IoU: sample.Row.IoU,
			LatencyMS: sample.Row.LatencyMS, GroundTruth: groundTruth, Predictions: predictions,
		})
	}
	client := evaluationplatform.NewClient(strings.TrimSpace(cfg.FiftyOneAPIURL), cfg.FiftyOneTimeout)
	result, err := client.SyncDataset(ctx, payload)
	if err != nil {
		return err
	}
	if result.SampleCount != len(evaluated) {
		return fmt.Errorf("FiftyOne sample count mismatch: want %d, got %d", len(evaluated), result.SampleCount)
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
