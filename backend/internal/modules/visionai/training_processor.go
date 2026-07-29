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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/storage"
	platformtraining "github.com/lohasle/nimbus-framework-go/internal/platform/training"
	"gorm.io/gorm"
)

func processTrainingEvent(ctx context.Context, db *gorm.DB, cfg config.Config, event DomainEventEnvelope) error {
	if event.EventType == "training.template.smoke.requested.v1" {
		return processTrainingTemplateSmokeEvent(ctx, db, cfg, event)
	}
	var payload struct {
		TrainingRunID uint64 `json:"trainingRunId"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil || payload.TrainingRunID == 0 {
		return errors.New("invalid training event payload")
	}
	var run TrainingRun
	if db.Where("tenant_id = ? AND id = ?", event.TenantID, payload.TrainingRunID).First(&run).Error != nil {
		return nil
	}
	if run.Status == TrainingSucceeded || run.Status == TrainingCancelled {
		if run.Status == TrainingSucceeded {
			return enqueueAutomaticEvaluations(db, &run)
		}
		return nil
	}
	if run.CancelRequestedAt != nil {
		run.Status, run.FinishedAt = TrainingCancelled, run.CancelRequestedAt
		db.Save(&run)
		return finishTrainingJob(db, &run, false, "CANCELLED", "training was cancelled before allocation")
	}
	var template TrainingTemplateVersion
	var dataset DatasetVersion
	if db.Where("tenant_id = ? AND id = ? AND published = ?", run.TenantID, run.TemplateVersionID, true).First(&template).Error != nil {
		return failTrainingRun(db, &run, "CONFIGURATION", "TEMPLATE_UNAVAILABLE", "published template version no longer available")
	}
	if db.Where("tenant_id = ? AND id = ? AND status = ?", run.TenantID, run.DatasetVersionID, DatasetVersionFrozen).First(&dataset).Error != nil {
		return failTrainingRun(db, &run, "DATA", "DATASET_NOT_FROZEN", "dataset version is not FROZEN")
	}
	now := time.Now()
	run.Status, run.StartedAt, run.Progress = TrainingAllocating, &now, 5
	db.Save(&run)
	db.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "TRAINING_RUN", run.ID).
		Updates(map[string]any{"status": JobRunning, "stage": "ALLOCATING", "progress": 5, "started_at": now})
	if run.Provider == "CLEARML" {
		return processClearMLTraining(ctx, db, cfg, &run, template, dataset)
	}
	if run.Provider != "LOCAL_DOCKER" {
		return failTrainingRun(db, &run, "CONFIGURATION", "PROVIDER_UNSUPPORTED", "unsupported training provider")
	}
	workRoot, err := filepath.Abs(cfg.TrainingWorkRoot)
	if err != nil {
		return failTrainingRun(db, &run, "RESOURCE", "WORK_ROOT_INVALID", err.Error())
	}
	runDir := filepath.Join(workRoot, "runs", strconv.FormatUint(run.ID, 10))
	inputDir := filepath.Join(workRoot, "inputs", strconv.FormatUint(run.ID, 10))
	relative, err := filepath.Rel(workRoot, runDir)
	if err != nil || strings.HasPrefix(relative, "..") {
		return failTrainingRun(db, &run, "RESOURCE", "WORK_ROOT_ESCAPE", "training run directory escapes configured root")
	}
	inputRelative, err := filepath.Rel(workRoot, inputDir)
	if err != nil || strings.HasPrefix(inputRelative, "..") {
		return failTrainingRun(db, &run, "RESOURCE", "WORK_ROOT_ESCAPE", "training input directory escapes configured root")
	}
	defer cleanupTrainingWorkspace(workRoot, run.ID)
	var runtimeSpec struct {
		MemoryBytes int64   `json:"memoryBytes"`
		CPUs        float64 `json:"cpus"`
		CPU         float64 `json:"cpu"`
	}
	_ = json.Unmarshal([]byte(run.RuntimeSpec), &runtimeSpec)
	if runtimeSpec.MemoryBytes == 0 {
		runtimeSpec.MemoryBytes = 1 << 30
	}
	if runtimeSpec.CPUs == 0 {
		runtimeSpec.CPUs = runtimeSpec.CPU
	}
	if runtimeSpec.CPUs == 0 {
		runtimeSpec.CPUs = 1
	}
	objectStore, err := storage.NewMinIO(cfg)
	if err != nil {
		return failTrainingRun(db, &run, "EXTERNAL_SERVICE", "STORAGE_CONFIG_INVALID", err.Error())
	}
	if err = objectStore.EnsureBucket(ctx); err != nil {
		return failTrainingRun(db, &run, "EXTERNAL_SERVICE", "STORAGE_UNAVAILABLE", err.Error())
	}
	if err = stageTrainingDataset(ctx, db, objectStore, &run, dataset, inputDir); err != nil {
		return failTrainingRun(db, &run, "DATA", "DATASET_STAGE_FAILED", err.Error())
	}
	run.Status, run.Progress = TrainingRunning, 20
	db.Save(&run)
	db.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "TRAINING_RUN", run.ID).
		Updates(map[string]any{"stage": "RUNNING", "progress": 20})
	trainingCtx, cancel := context.WithTimeout(ctx, cfg.TrainingTimeout)
	defer cancel()
	go watchTrainingCancellation(trainingCtx, db, run.ID, cancel)
	manifest, log, runErr := (platformtraining.LocalDocker{Binary: cfg.DockerBinary, VolumesFrom: cfg.DockerVolumesFrom}).Run(trainingCtx, platformtraining.LocalDockerSpec{
		RunID: run.ID, ImageRef: template.ImageRef, Entrypoint: template.Entrypoint, OutputDir: runDir,
		InputDir:           inputDir,
		DatasetManifestURI: dataset.ManifestURI, ParametersJSON: run.Parameters, GPUCount: run.GPUCount,
		MemoryBytes: runtimeSpec.MemoryBytes, CPUs: runtimeSpec.CPUs,
	})
	if runErr != nil {
		var cancellation TrainingRun
		if db.Select("cancel_requested_at").First(&cancellation, run.ID).Error == nil && cancellation.CancelRequestedAt != nil {
			_ = storeTrainingLog(context.Background(), db, cfg, &run, log)
			run.Status, run.FinishedAt = TrainingCancelled, cancellation.CancelRequestedAt
			db.Save(&run)
			return finishTrainingJob(db, &run, false, "CANCELLED", "LocalDocker container cancelled and workspace cleaned")
		}
		category, code := classifyTrainingFailure(runErr)
		_ = storeTrainingLog(context.Background(), db, cfg, &run, log)
		return failTrainingRun(db, &run, category, code, runErr.Error())
	}
	run.Status, run.Progress = TrainingExporting, 85
	db.Save(&run)
	db.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "TRAINING_RUN", run.ID).
		Updates(map[string]any{"stage": "EXPORTING", "progress": 85})
	if err = collectTrainingResults(ctx, db, objectStore, &run, runDir, manifest, log); err != nil {
		return failTrainingRun(db, &run, "FRAMEWORK", "RESULT_MANIFEST_INVALID", err.Error())
	}
	now = time.Now()
	run.Status, run.Progress, run.FinishedAt = TrainingSucceeded, 100, &now
	run.MetricSummary = jsonValue(manifest.Summary)
	if err = db.Save(&run).Error; err != nil {
		return err
	}
	if err = enqueueAutomaticEvaluations(db, &run); err != nil {
		return err
	}
	return finishTrainingJob(db, &run, true, "", "")
}

func enqueueAutomaticEvaluations(db *gorm.DB, run *TrainingRun) error {
	if run == nil || run.Status != TrainingSucceeded {
		return nil
	}
	var suites []EvaluationSuite
	if err := db.Where(
		"tenant_id = ? AND project_id = ? AND dataset_version_id = ? AND auto_trigger = ?",
		run.TenantID, run.ProjectID, run.DatasetVersionID, true,
	).Order("id").Find(&suites).Error; err != nil {
		return err
	}
	for _, suite := range suites {
		var existing int64
		db.Model(&EvaluationRun{}).Where(
			"tenant_id = ? AND project_id = ? AND suite_id = ? AND training_run_id = ?",
			run.TenantID, run.ProjectID, suite.ID, run.ID,
		).Count(&existing)
		if existing > 0 {
			continue
		}
		var baseline EvaluationRun
		db.Where(
			"tenant_id = ? AND project_id = ? AND suite_id = ? AND status = ?",
			run.TenantID, run.ProjectID, suite.ID, "SUCCEEDED",
		).Order("id DESC").First(&baseline)
		evaluation := EvaluationRun{
			TenantID: run.TenantID, ProjectID: run.ProjectID, SuiteID: suite.ID,
			TrainingRunID: run.ID, BaselineRunID: baseline.ID, Status: "QUEUED",
			GateDecision: "PENDING", GateEvidence: "{}", Summary: "{}", CreatedBy: run.CreatedBy,
		}
		job := PlatformJob{
			TenantID: run.TenantID, ProjectID: run.ProjectID, JobType: "EVALUATION",
			ResourceType: "EVALUATION_RUN", Status: JobQueued, Stage: "QUEUED",
			TraceID: uuid.NewString(), Idempotency: fmt.Sprintf("auto-evaluation:%d:%d", run.ID, suite.ID),
			MaxRetries: 2, CreatedBy: run.CreatedBy,
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&evaluation).Error; err != nil {
				return err
			}
			job.ResourceID = evaluation.ID
			if err := tx.Create(&job).Error; err != nil {
				return err
			}
			if err := tx.Create(&DatasetUsage{
				TenantID: run.TenantID, ProjectID: run.ProjectID, DatasetVersionID: suite.DatasetVersionID,
				ResourceType: "EVALUATION_RUN", ResourceID: evaluation.ID,
			}).Error; err != nil {
				return err
			}
			return tx.Create(&OutboxEvent{
				TenantID: run.TenantID, EventID: uuid.NewString(), EventType: "evaluation.run.requested.v1",
				AggregateType: "EVALUATION_RUN", AggregateID: evaluation.ID,
				Payload: jsonValue(map[string]any{"evaluationRunId": evaluation.ID, "autoTriggered": true}),
				Status:  outboxNew,
			}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func watchTrainingCancellation(ctx context.Context, db *gorm.DB, runID uint64, cancel context.CancelFunc) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var run TrainingRun
			if db.Select("cancel_requested_at").First(&run, runID).Error == nil && run.CancelRequestedAt != nil {
				cancel()
				return
			}
		}
	}
}

func cleanupTrainingWorkspace(workRoot string, runID uint64) {
	root, err := filepath.Abs(workRoot)
	if err != nil || runID == 0 {
		return
	}
	for _, candidate := range []string{
		filepath.Join(root, "runs", strconv.FormatUint(runID, 10)),
		filepath.Join(root, "inputs", strconv.FormatUint(runID, 10)),
	} {
		resolved, resolveErr := filepath.Abs(candidate)
		relative, relativeErr := filepath.Rel(root, resolved)
		if resolveErr == nil && relativeErr == nil && relative != "." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".." {
			_ = os.RemoveAll(resolved)
		}
	}
}

func processTrainingTemplateSmokeEvent(ctx context.Context, db *gorm.DB, cfg config.Config, event DomainEventEnvelope) error {
	var payload struct {
		TemplateVersionID uint64 `json:"templateVersionId"`
		JobID             uint64 `json:"jobId"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil || payload.TemplateVersionID == 0 || payload.JobID == 0 {
		return errors.New("invalid training template smoke event payload")
	}
	var version TrainingTemplateVersion
	if db.Where("tenant_id = ? AND id = ?", event.TenantID, payload.TemplateVersionID).First(&version).Error != nil {
		return nil
	}
	if version.Published || version.SmokeStatus == "PASSED" {
		return nil
	}
	now := time.Now()
	db.Model(&PlatformJob{}).Where("tenant_id = ? AND id = ?", event.TenantID, payload.JobID).Updates(map[string]any{
		"status": JobRunning, "stage": "RUNNING", "progress": 10, "started_at": now,
	})
	var requirements struct {
		GPUMin int `json:"gpuMin"`
	}
	_ = json.Unmarshal([]byte(version.ResourceRequirements), &requirements)
	smokeCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	manifest, log, runErr := (platformtraining.LocalDocker{
		Binary:      cfg.DockerBinary,
		VolumesFrom: cfg.DockerVolumesFrom,
	}).Run(smokeCtx, platformtraining.LocalDockerSpec{
		RunID: payload.JobID, ImageRef: version.ImageRef, Entrypoint: version.Entrypoint,
		OutputDir: filepath.Join(
			cfg.TrainingWorkRoot,
			"template-smoke",
			strconv.FormatUint(version.ID, 10),
			strconv.FormatUint(payload.JobID, 10),
		),
		DatasetManifestURI: "s3://visionai-assets/smoke/dataset-manifest.json",
		ParametersJSON:     "{}",
		MemoryBytes:        4 << 30,
		CPUs:               2,
		GPUCount:           requirements.GPUMin,
	})
	finishedAt := time.Now()
	if runErr != nil {
		report := jsonValue(map[string]any{
			"jobId": payload.JobID, "runner": "visionai-orchestrator",
			"error": runErr.Error(), "log": string(log),
		})
		_ = db.Model(&version).Updates(map[string]any{"smoke_status": "FAILED", "smoke_report": report}).Error
		_ = db.Model(&PlatformJob{}).Where("tenant_id = ? AND id = ?", event.TenantID, payload.JobID).Updates(map[string]any{
			"status": JobFailed, "stage": "FAILED", "progress": 100,
			"error_code": "TEMPLATE_SMOKE_FAILED", "error_message": runErr.Error(),
			"remediation": "检查镜像、GPU 运行时、资源约束与 result-manifest 后重试",
			"finished_at": finishedAt,
		}).Error
		return nil
	}
	report := jsonValue(map[string]any{
		"jobId": payload.JobID, "runner": "visionai-orchestrator",
		"summary": manifest.Summary, "log": string(log),
	})
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&version).Updates(map[string]any{"smoke_status": "PASSED", "smoke_report": report}).Error; err != nil {
			return err
		}
		return tx.Model(&PlatformJob{}).Where("tenant_id = ? AND id = ?", event.TenantID, payload.JobID).Updates(map[string]any{
			"status": JobSucceeded, "stage": "PASSED", "progress": 100, "finished_at": finishedAt,
		}).Error
	})
}

type detectionTrainingClass struct {
	Index        int    `json:"index"`
	OntologyID   uint64 `json:"ontologyLabelId"`
	CVATLabelID  int64  `json:"cvatLabelId"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Color        string `json:"color"`
	OntologySort int    `json:"ontologySort"`
}

type detectionTrainingBox struct {
	ClassIndex  int       `json:"classIndex"`
	CVATLabelID int64     `json:"cvatLabelId"`
	LabelCode   string    `json:"labelCode"`
	LabelName   string    `json:"labelName"`
	XYXY        []float64 `json:"xyxy"`
}

type detectionTrainingItem struct {
	AssetID        uint64                 `json:"assetId"`
	Filename       string                 `json:"filename"`
	SourceFilename string                 `json:"sourceFilename"`
	Split          string                 `json:"split"`
	SHA256         string                 `json:"sha256"`
	Width          int                    `json:"width"`
	Height         int                    `json:"height"`
	ContentType    string                 `json:"contentType"`
	Boxes          []detectionTrainingBox `json:"boxes"`
}

type detectionTrainingIndex struct {
	SchemaVersion        string                   `json:"schemaVersion"`
	DatasetVersionID     uint64                   `json:"datasetVersionId"`
	DatasetChecksum      string                   `json:"datasetChecksum"`
	AnnotationRevisionID uint64                   `json:"annotationRevisionId"`
	AnnotationChecksum   string                   `json:"annotationChecksum"`
	OntologyVersionID    uint64                   `json:"ontologyVersionId"`
	OntologyChecksum     string                   `json:"ontologyChecksum"`
	Classes              []detectionTrainingClass `json:"classes"`
	Items                []detectionTrainingItem  `json:"items"`
}

type stagedCVATShape struct {
	Frame   int       `json:"frame"`
	LabelID int64     `json:"label_id"`
	Type    string    `json:"type"`
	Points  []float64 `json:"points"`
	Outside bool      `json:"outside"`
}

type stagedCVATSnapshot struct {
	Shapes []stagedCVATShape `json:"shapes"`
	Tracks []struct {
		LabelID int64             `json:"label_id"`
		Shapes  []stagedCVATShape `json:"shapes"`
	} `json:"tracks"`
}

type stagedCVATLabel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func stageTrainingDataset(ctx context.Context, db *gorm.DB, objectStore storage.Provider, run *TrainingRun, dataset DatasetVersion, inputDir string) error {
	if strings.TrimSpace(dataset.ManifestObjectKey) == "" {
		return errors.New("frozen dataset manifest object key is missing")
	}
	if err := os.MkdirAll(filepath.Join(inputDir, "assets"), 0o755); err != nil {
		return err
	}
	if err := copyTrainingObject(ctx, objectStore, dataset.ManifestObjectKey, filepath.Join(inputDir, "manifest.json")); err != nil {
		return fmt.Errorf("stage dataset manifest: %w", err)
	}
	var items []DatasetVersionItem
	if err := db.Where(
		"tenant_id = ? AND project_id = ? AND dataset_version_id = ?",
		run.TenantID, run.ProjectID, dataset.ID,
	).Order("id").Find(&items).Error; err != nil {
		return err
	}
	if len(items) == 0 {
		return errors.New("frozen dataset contains no items")
	}
	assetIDs := make([]uint64, 0, len(items))
	for _, item := range items {
		assetIDs = append(assetIDs, item.AssetID)
	}
	var assets []Asset
	if err := db.Where(
		"tenant_id = ? AND project_id = ? AND id IN ? AND status = ?",
		run.TenantID, run.ProjectID, assetIDs, AssetReady,
	).Find(&assets).Error; err != nil {
		return err
	}
	if len(assets) != len(items) {
		return fmt.Errorf("dataset staging expected %d ready assets, found %d", len(items), len(assets))
	}
	assetByID := make(map[uint64]Asset, len(assets))
	for _, asset := range assets {
		assetByID[asset.ID] = asset
	}

	var revision AnnotationRevision
	if dataset.AnnotationRevisionID == 0 || db.Where(
		"tenant_id = ? AND project_id = ? AND id = ?",
		run.TenantID, run.ProjectID, dataset.AnnotationRevisionID,
	).First(&revision).Error != nil {
		return errors.New("dataset annotation revision is missing")
	}
	if revision.OntologyVersionID == 0 || revision.OntologyVersionID != dataset.OntologyVersionID ||
		revision.OntologyChecksum == "" || revision.OntologyChecksum != dataset.OntologyChecksum {
		return errors.New("dataset and annotation revision ontology checksums do not match")
	}
	annotationReader, _, err := objectStore.Get(ctx, revision.ObjectKey)
	if err != nil {
		return fmt.Errorf("stage annotation revision: %w", err)
	}
	annotationRaw, readErr := io.ReadAll(io.LimitReader(annotationReader, 512<<20))
	closeErr := annotationReader.Close()
	if readErr != nil {
		return fmt.Errorf("read annotation revision: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close annotation revision: %w", closeErr)
	}
	annotationDigest := sha256.Sum256(annotationRaw)
	if hex.EncodeToString(annotationDigest[:]) != strings.ToLower(revision.Checksum) {
		return errors.New("annotation revision checksum mismatch")
	}
	var snapshot stagedCVATSnapshot
	if err = json.Unmarshal(annotationRaw, &snapshot); err != nil {
		return fmt.Errorf("decode CVAT annotation revision: %w", err)
	}
	var cvatLabels []stagedCVATLabel
	if err = json.Unmarshal([]byte(revision.CategoryMapping), &cvatLabels); err != nil || len(cvatLabels) == 0 {
		return errors.New("annotation category mapping is invalid or empty")
	}
	var ontologyLabels []OntologyLabel
	if err = db.Where(
		"tenant_id = ? AND project_id = ? AND ontology_version_id = ?",
		run.TenantID, run.ProjectID, dataset.OntologyVersionID,
	).Order("sort, id").Find(&ontologyLabels).Error; err != nil {
		return err
	}
	if len(ontologyLabels) == 0 {
		return errors.New("dataset ontology contains no labels")
	}
	ontologyByName := make(map[string]OntologyLabel, len(ontologyLabels))
	for _, label := range ontologyLabels {
		ontologyByName[strings.ToLower(strings.TrimSpace(label.Name))] = label
	}
	cvatByName := make(map[string]stagedCVATLabel, len(cvatLabels))
	for _, label := range cvatLabels {
		cvatByName[strings.ToLower(strings.TrimSpace(label.Name))] = label
	}
	classes := make([]detectionTrainingClass, 0, len(cvatLabels))
	classByCVATID := make(map[int64]detectionTrainingClass, len(cvatLabels))
	for _, ontologyLabel := range ontologyLabels {
		cvatLabel, exists := cvatByName[strings.ToLower(strings.TrimSpace(ontologyLabel.Name))]
		if !exists {
			return fmt.Errorf("ontology label %q is not present in the CVAT category mapping", ontologyLabel.Name)
		}
		class := detectionTrainingClass{
			Index: len(classes) + 1, OntologyID: ontologyLabel.ID, CVATLabelID: cvatLabel.ID,
			Code: ontologyLabel.Code, Name: ontologyLabel.Name, Color: ontologyLabel.Color, OntologySort: ontologyLabel.Sort,
		}
		classes = append(classes, class)
		classByCVATID[cvatLabel.ID] = class
	}
	if len(classes) != len(cvatLabels) || len(ontologyByName) != len(cvatByName) {
		return errors.New("CVAT category mapping does not exactly match the governed ontology")
	}

	staged := make([]detectionTrainingItem, 0, len(items))
	for _, item := range items {
		asset, exists := assetByID[item.AssetID]
		if !exists {
			return fmt.Errorf("dataset asset %d is missing", item.AssetID)
		}
		extension := strings.ToLower(filepath.Ext(asset.Filename))
		if extension == "" || len(extension) > 10 {
			extension = ".bin"
		}
		filename := strconv.FormatUint(asset.ID, 10) + extension
		localPath := filepath.Join(inputDir, "assets", filename)
		if err := copyTrainingObject(ctx, objectStore, asset.ObjectKey, localPath); err != nil {
			return fmt.Errorf("stage asset %d: %w", asset.ID, err)
		}
		staged = append(staged, detectionTrainingItem{
			AssetID: asset.ID, Filename: filename, SourceFilename: asset.Filename,
			Split: item.Split, SHA256: asset.SHA256, Width: asset.Width, Height: asset.Height,
			ContentType: asset.ContentType, Boxes: make([]detectionTrainingBox, 0),
		})
	}
	appendShape := func(shape stagedCVATShape, labelID int64) error {
		if shape.Outside {
			return nil
		}
		if shape.Frame < 0 || shape.Frame >= len(staged) {
			return fmt.Errorf("annotation frame %d is outside the staged dataset", shape.Frame)
		}
		if len(shape.Points) < 4 || len(shape.Points)%2 != 0 {
			return fmt.Errorf("annotation frame %d contains invalid geometry", shape.Frame)
		}
		class, exists := classByCVATID[labelID]
		if !exists {
			return fmt.Errorf("annotation references unknown CVAT label %d", labelID)
		}
		minX, maxX, minY, maxY := shape.Points[0], shape.Points[0], shape.Points[1], shape.Points[1]
		for index := 2; index+1 < len(shape.Points); index += 2 {
			minX = min(minX, shape.Points[index])
			maxX = max(maxX, shape.Points[index])
			minY = min(minY, shape.Points[index+1])
			maxY = max(maxY, shape.Points[index+1])
		}
		item := &staged[shape.Frame]
		minX, minY = max(0, minX), max(0, minY)
		maxX, maxY = min(float64(item.Width), maxX), min(float64(item.Height), maxY)
		if maxX <= minX || maxY <= minY {
			return fmt.Errorf("annotation frame %d contains a degenerate bounding box", shape.Frame)
		}
		item.Boxes = append(item.Boxes, detectionTrainingBox{
			ClassIndex: class.Index, CVATLabelID: labelID, LabelCode: class.Code,
			LabelName: class.Name, XYXY: []float64{minX, minY, maxX, maxY},
		})
		return nil
	}
	for _, shape := range snapshot.Shapes {
		if err = appendShape(shape, shape.LabelID); err != nil {
			return err
		}
	}
	for _, track := range snapshot.Tracks {
		for _, shape := range track.Shapes {
			if err = appendShape(shape, track.LabelID); err != nil {
				return err
			}
		}
	}
	indexRaw, err := json.Marshal(detectionTrainingIndex{
		SchemaVersion: "visionai.detection-training-input.v1", DatasetVersionID: dataset.ID,
		DatasetChecksum: dataset.Checksum, AnnotationRevisionID: revision.ID, AnnotationChecksum: revision.Checksum,
		OntologyVersionID: dataset.OntologyVersionID, OntologyChecksum: dataset.OntologyChecksum,
		Classes: classes, Items: staged,
	})
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(inputDir, "annotations.json"), annotationRaw, 0o444); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(inputDir, "staged-index.json"), indexRaw, 0o444); err != nil {
		return err
	}
	_ = os.Chmod(filepath.Join(inputDir, "manifest.json"), 0o444)
	_ = os.Chmod(filepath.Join(inputDir, "annotations.json"), 0o444)
	_ = os.Chmod(filepath.Join(inputDir, "assets"), 0o555)
	return os.Chmod(inputDir, 0o555)
}

func copyTrainingObject(ctx context.Context, objectStore storage.Provider, key, destination string) error {
	source, _, err := objectStore.Get(ctx, key)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o444)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func processClearMLTraining(ctx context.Context, db *gorm.DB, cfg config.Config, run *TrainingRun, template TrainingTemplateVersion, dataset DatasetVersion) error {
	client, err := platformtraining.NewClearML(cfg.ClearMLAPIURL, cfg.ClearMLWebURL, cfg.ClearMLAccessKey, cfg.ClearMLSecretKey, 30*time.Second)
	if err != nil {
		return failTrainingRun(db, run, "CONFIGURATION", "CLEARML_CONFIG_INVALID", err.Error())
	}
	if strings.TrimSpace(cfg.DockerVolumesFrom) == "" {
		return failTrainingRun(db, run, "CONFIGURATION", "CLEARML_SHARED_VOLUME_REQUIRED", "ClearML Agent requires a configured shared training volume")
	}
	workRoot, err := filepath.Abs(cfg.TrainingWorkRoot)
	if err != nil {
		return failTrainingRun(db, run, "RESOURCE", "WORK_ROOT_INVALID", err.Error())
	}
	runDir := filepath.Join(workRoot, "runs", strconv.FormatUint(run.ID, 10))
	inputDir := filepath.Join(workRoot, "inputs", strconv.FormatUint(run.ID, 10))
	for _, path := range []string{runDir, inputDir} {
		relative, relErr := filepath.Rel(workRoot, path)
		if relErr != nil || strings.HasPrefix(relative, "..") {
			return failTrainingRun(db, run, "RESOURCE", "WORK_ROOT_ESCAPE", "training directory escapes configured root")
		}
	}
	defer cleanupTrainingWorkspace(workRoot, run.ID)
	if _, err = platformtraining.PrepareOutputDirectory(runDir); err != nil {
		return failTrainingRun(db, run, "RESOURCE", "OUTPUT_DIRECTORY_INVALID", err.Error())
	}
	objectStore, err := storage.NewMinIO(cfg)
	if err != nil {
		return failTrainingRun(db, run, "EXTERNAL_SERVICE", "STORAGE_CONFIG_INVALID", err.Error())
	}
	if err = objectStore.EnsureBucket(ctx); err != nil {
		return failTrainingRun(db, run, "EXTERNAL_SERVICE", "STORAGE_UNAVAILABLE", err.Error())
	}
	if err = stageTrainingDataset(ctx, db, objectStore, run, dataset, inputDir); err != nil {
		return failTrainingRun(db, run, "DATA", "DATASET_STAGE_FAILED", err.Error())
	}
	var runtimeSpec struct {
		MemoryBytes int64   `json:"memoryBytes"`
		CPUs        float64 `json:"cpus"`
		CPU         float64 `json:"cpu"`
	}
	_ = json.Unmarshal([]byte(run.RuntimeSpec), &runtimeSpec)
	if runtimeSpec.MemoryBytes == 0 {
		runtimeSpec.MemoryBytes = 1 << 30
	}
	if runtimeSpec.CPUs == 0 {
		runtimeSpec.CPUs = runtimeSpec.CPU
	}
	if runtimeSpec.CPUs == 0 {
		runtimeSpec.CPUs = 1
	}
	var parameters map[string]any
	_ = json.Unmarshal([]byte(run.Parameters), &parameters)
	if parameters == nil {
		parameters = map[string]any{}
	}
	parameters["visionaiDatasetManifest"] = dataset.ManifestURI
	parameters["visionaiRunId"] = run.ID
	containerArguments := []string{
		"--user", "0:0",
		"--network", cfg.DockerNetwork,
		"--volumes-from", cfg.DockerVolumesFrom,
		"--memory", strconv.FormatInt(runtimeSpec.MemoryBytes, 10),
		"--cpus", strconv.FormatFloat(runtimeSpec.CPUs, 'f', 2, 64),
		"--env", "VISIONAI_RUN_ID=" + strconv.FormatUint(run.ID, 10),
		"--env", "VISIONAI_OUTPUT_DIR=" + runDir,
		"--env", "VISIONAI_INPUT_DIR=" + inputDir,
		"--env", "VISIONAI_DATASET_MANIFEST_URI=" + dataset.ManifestURI,
		"--env", "VISIONAI_PARAMETERS_JSON=" + run.Parameters,
	}
	entrypoint := strings.TrimSpace(template.Entrypoint)
	if entrypoint == "" {
		// Managed trainer images expose this stable framework entrypoint. An
		// explicit template entrypoint always takes precedence.
		entrypoint = "/usr/local/bin/visionai-gpu-trainer"
	}
	trainingCtx, cancel := context.WithTimeout(ctx, cfg.TrainingTimeout)
	defer cancel()
	external, err := client.CreateAndEnqueue(trainingCtx, platformtraining.ClearMLSpec{
		Name: run.Name, Queue: run.Queue, ImageRef: template.ImageRef, Entrypoint: entrypoint,
		ScriptBinary: "python3", ContainerArguments: containerArguments,
		Parameters: parameters, Tags: []string{
			"visionai", "project-" + strconv.FormatUint(run.ProjectID, 10),
			"run-" + strconv.FormatUint(run.ID, 10),
		},
	})
	if err != nil {
		return failTrainingRun(db, run, "EXTERNAL_SERVICE", "CLEARML_SUBMIT_FAILED", err.Error())
	}
	binding := ExternalResourceBinding{
		TenantID: run.TenantID, ProviderType: "CLEARML", InstanceID: "default",
		InternalType: "TRAINING_RUN", InternalID: run.ID, ExternalType: "TASK",
		ExternalID: external.ID, ExternalURL: client.TaskURL(external.ID), SyncCursor: "QUEUED",
	}
	if err = db.Where("tenant_id = ? AND provider_type = ? AND internal_type = ? AND internal_id = ?",
		run.TenantID, "CLEARML", "TRAINING_RUN", run.ID).FirstOrCreate(&binding).Error; err != nil {
		return failTrainingRun(db, run, "EXTERNAL_SERVICE", "CLEARML_BINDING_FAILED", err.Error())
	}
	run.ExternalBindingID, run.Status, run.Progress = binding.ID, TrainingQueued, 10
	db.Save(run)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-trainingCtx.Done():
			_ = client.Cancel(context.Background(), external.ID)
			return failTrainingRun(db, run, "RESOURCE", "TRAINING_TIMEOUT", trainingCtx.Err().Error())
		case <-ticker.C:
			var cancellation TrainingRun
			if db.Select("cancel_requested_at").First(&cancellation, run.ID).Error == nil && cancellation.CancelRequestedAt != nil {
				_ = client.Cancel(context.Background(), external.ID)
				run.Status, run.FinishedAt = TrainingCancelled, cancellation.CancelRequestedAt
				db.Save(run)
				return finishTrainingJob(db, run, false, "CANCELLED", "ClearML task cancelled")
			}
			task, getErr := client.GetTask(trainingCtx, external.ID)
			if getErr != nil {
				return failTrainingRun(db, run, "EXTERNAL_SERVICE", "CLEARML_SYNC_FAILED", getErr.Error())
			}
			syncedAt := time.Now()
			binding.SyncCursor, binding.LastSyncAt = strings.ToUpper(task.Status), &syncedAt
			db.Save(&binding)
			switch strings.ToLower(task.Status) {
			case "in_progress":
				run.Status, run.Progress = TrainingRunning, max(task.Progress, 20)
				db.Save(run)
			case "completed", "published":
				log, logErr := client.TaskLog(trainingCtx, external.ID)
				if logErr != nil {
					log = []byte("ClearML console log unavailable: " + logErr.Error() + "\n")
				}
				manifest, manifestErr := platformtraining.ReadResultManifest(runDir)
				if manifestErr != nil {
					_ = storeTrainingLog(context.Background(), db, cfg, run, log)
					return failTrainingRun(db, run, "FRAMEWORK", "RESULT_MANIFEST_INVALID", manifestErr.Error())
				}
				if metricErr := client.ReportMetrics(trainingCtx, external.ID, manifest.Metrics); metricErr != nil {
					log = append(log, []byte("\nVisionAI metric mirror warning: "+metricErr.Error()+"\n")...)
				}
				run.Status, run.Progress = TrainingExporting, 85
				db.Save(run)
				db.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "TRAINING_RUN", run.ID).
					Updates(map[string]any{"stage": "EXPORTING", "progress": 85})
				if err = collectTrainingResults(trainingCtx, db, objectStore, run, runDir, manifest, log); err != nil {
					return failTrainingRun(db, run, "FRAMEWORK", "RESULT_EXPORT_FAILED", err.Error())
				}
				now := time.Now()
				run.Status, run.Progress, run.FinishedAt = TrainingSucceeded, 100, &now
				summary := make(map[string]any, len(manifest.Summary)+2)
				for key, value := range manifest.Summary {
					summary[key] = value
				}
				summary["provider"] = "CLEARML"
				summary["externalTaskId"] = external.ID
				run.MetricSummary = jsonValue(summary)
				upsertClearMLTrainingNode(db, run, task, manifest)
				if err = db.Save(run).Error; err != nil {
					return err
				}
				if err = enqueueAutomaticEvaluations(db, run); err != nil {
					return err
				}
				return finishTrainingJob(db, run, true, "", "")
			case "failed":
				return failTrainingRun(db, run, "FRAMEWORK", "CLEARML_TASK_FAILED", task.StatusMessage)
			case "stopped", "closed":
				return failTrainingRun(db, run, "EXTERNAL_SERVICE", "CLEARML_TASK_STOPPED", task.StatusMessage)
			}
		}
	}
}

func upsertClearMLTrainingNode(db *gorm.DB, run *TrainingRun, task platformtraining.ClearMLTask, manifest platformtraining.ResultManifest) {
	if strings.TrimSpace(task.WorkerID) == "" {
		return
	}
	nodeKey := "clearml:" + task.WorkerID
	var node ComputeNode
	db.Where("tenant_id = ? AND node_key = ?", run.TenantID, nodeKey).First(&node)
	node.TenantID, node.NodeKey, node.Name = run.TenantID, nodeKey, task.WorkerID
	node.Status, node.LastHeartbeatAt = "ONLINE", time.Now()
	node.GPUModel = fmt.Sprint(manifest.Summary["gpuModel"])
	node.CUDAVersion = fmt.Sprint(manifest.Summary["cudaRuntime"])
	if value, ok := manifest.Summary["gpuMemoryBytes"].(float64); ok {
		node.GPUMemoryBytes = int64(value)
	}
	if node.GPUModel != "" {
		node.GPUCount = max(run.GPUCount, 1)
	}
	node.Labels = jsonValue(map[string]any{
		"provider": "CLEARML", "queue": run.Queue,
		"externalTaskId": task.ID, "runtime": "clearml-agent",
	})
	if node.ID == 0 {
		_ = db.Create(&node).Error
	} else {
		_ = db.Save(&node).Error
	}
}

func classifyTrainingFailure(err error) (string, string) {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "deadline"), strings.Contains(message, "timeout"):
		return "RESOURCE", "TRAINING_TIMEOUT"
	case strings.Contains(message, "no space"), strings.Contains(message, "memory"), strings.Contains(message, "oom"):
		return "RESOURCE", "RESOURCE_EXHAUSTED"
	case strings.Contains(message, "pull access denied"), strings.Contains(message, "no such image"):
		return "IMAGE", "IMAGE_UNAVAILABLE"
	case strings.Contains(message, "result-manifest"):
		return "FRAMEWORK", "RESULT_MANIFEST_INVALID"
	case strings.Contains(message, "docker"):
		return "EXTERNAL_SERVICE", "DOCKER_EXECUTION_FAILED"
	default:
		return "UNKNOWN", "TRAINING_EXECUTION_FAILED"
	}
}

func failTrainingRun(db *gorm.DB, run *TrainingRun, category, code, message string) error {
	now := time.Now()
	run.Status, run.Progress, run.ErrorCategory, run.ErrorCode, run.ErrorMessage, run.FinishedAt = TrainingFailed, 0, category, code, message, &now
	_ = db.Save(run).Error
	return finishTrainingJob(db, run, false, code, message)
}

func finishTrainingJob(db *gorm.DB, run *TrainingRun, success bool, code, message string) error {
	status, stage, progress := JobFailed, "FAILED", 0
	if success {
		status, stage, progress = JobSucceeded, "SUCCEEDED", 100
	} else if code == "CANCELLED" {
		status, stage = JobCancelled, "CANCELLED"
	}
	return db.Model(&PlatformJob{}).
		Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "TRAINING_RUN", run.ID).
		Updates(map[string]any{
			"status": status, "stage": stage, "progress": progress, "error_code": code,
			"error_message": message, "finished_at": time.Now(),
		}).Error
}

func safeArtifactPath(root, value string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(value))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("unsafe artifact path")
	}
	resolved := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, resolved)
	if err != nil || strings.HasPrefix(relative, "..") {
		return "", errors.New("artifact path escapes result root")
	}
	return resolved, nil
}

func collectTrainingResults(ctx context.Context, db *gorm.DB, objectStore storage.Provider, run *TrainingRun, runDir string, manifest platformtraining.ResultManifest, log []byte) error {
	root := assetRoot(run.TenantID, run.ProjectID) + "/training/runs/" + strconv.FormatUint(run.ID, 10)
	manifestRaw, _ := json.Marshal(manifest)
	manifestKey := root + "/result-manifest.json"
	if err := objectStore.Put(ctx, manifestKey, bytes.NewReader(manifestRaw), int64(len(manifestRaw)), "application/json"); err != nil {
		return err
	}
	run.ResultManifestURI = objectStore.URI(manifestKey)
	artifacts := make([]TrainingArtifact, 0, len(manifest.Artifacts)+1)
	for _, item := range manifest.Artifacts {
		localPath, err := safeArtifactPath(runDir, item.Path)
		if err != nil {
			return err
		}
		file, err := os.Open(localPath)
		if err != nil {
			return err
		}
		hash := sha256.New()
		size, err := io.Copy(hash, file)
		_ = file.Close()
		if err != nil {
			return err
		}
		file, err = os.Open(localPath)
		if err != nil {
			return err
		}
		key := root + "/artifacts/" + filepath.Base(localPath)
		err = objectStore.Put(ctx, key, file, size, item.MediaType)
		_ = file.Close()
		if err != nil {
			return err
		}
		artifacts = append(artifacts, TrainingArtifact{
			TenantID: run.TenantID, ProjectID: run.ProjectID, TrainingRunID: run.ID,
			Kind: item.Kind, Name: item.Name, URI: objectStore.URI(key),
			SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size, MediaType: item.MediaType,
		})
	}
	if len(log) > 0 {
		sum := sha256.Sum256(log)
		key := root + "/logs/execution.log"
		if err := objectStore.Put(ctx, key, bytes.NewReader(log), int64(len(log)), "text/plain"); err != nil {
			return err
		}
		artifacts = append(artifacts, TrainingArtifact{
			TenantID: run.TenantID, ProjectID: run.ProjectID, TrainingRunID: run.ID,
			Kind: "LOG", Name: "execution.log", URI: objectStore.URI(key),
			SHA256: hex.EncodeToString(sum[:]), Size: int64(len(log)), MediaType: "text/plain",
		})
	}
	metrics := make([]TrainingMetric, 0, len(manifest.Metrics))
	for _, metric := range manifest.Metrics {
		metrics = append(metrics, TrainingMetric{
			TenantID: run.TenantID, ProjectID: run.ProjectID, TrainingRunID: run.ID,
			Name: metric.Name, Step: metric.Step, Value: metric.Value,
		})
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if len(artifacts) > 0 {
			if err := tx.Create(&artifacts).Error; err != nil {
				return err
			}
		}
		if len(metrics) > 0 {
			if err := tx.Create(&metrics).Error; err != nil {
				return err
			}
		}
		return tx.Save(run).Error
	})
}

func storeTrainingLog(ctx context.Context, db *gorm.DB, cfg config.Config, run *TrainingRun, log []byte) error {
	if len(log) == 0 {
		return nil
	}
	objectStore, err := storage.NewMinIO(cfg)
	if err != nil {
		return err
	}
	key := assetRoot(run.TenantID, run.ProjectID) + "/training/runs/" + strconv.FormatUint(run.ID, 10) + "/logs/failure.log"
	if err = objectStore.Put(ctx, key, bytes.NewReader(log), int64(len(log)), "text/plain"); err != nil {
		return err
	}
	sum := sha256.Sum256(log)
	return db.Create(&TrainingArtifact{
		TenantID: run.TenantID, ProjectID: run.ProjectID, TrainingRunID: run.ID,
		Kind: "LOG", Name: "failure.log", URI: objectStore.URI(key),
		SHA256: hex.EncodeToString(sum[:]), Size: int64(len(log)), MediaType: "text/plain",
	}).Error
}
