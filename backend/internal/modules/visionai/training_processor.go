package visionai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/storage"
	platformtraining "github.com/lohasle/nimbus-framework-go/internal/platform/training"
	"gorm.io/gorm"
)

func processTrainingEvent(ctx context.Context, db *gorm.DB, cfg config.Config, event DomainEventEnvelope) error {
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
	relative, err := filepath.Rel(workRoot, runDir)
	if err != nil || strings.HasPrefix(relative, "..") {
		return failTrainingRun(db, &run, "RESOURCE", "WORK_ROOT_ESCAPE", "training run directory escapes configured root")
	}
	var runtimeSpec struct {
		MemoryBytes int64   `json:"memoryBytes"`
		CPUs        float64 `json:"cpus"`
	}
	_ = json.Unmarshal([]byte(run.RuntimeSpec), &runtimeSpec)
	if runtimeSpec.MemoryBytes == 0 {
		runtimeSpec.MemoryBytes = 1 << 30
	}
	if runtimeSpec.CPUs == 0 {
		runtimeSpec.CPUs = 1
	}
	run.Status, run.Progress = TrainingRunning, 20
	db.Save(&run)
	db.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "TRAINING_RUN", run.ID).
		Updates(map[string]any{"stage": "RUNNING", "progress": 20})
	trainingCtx, cancel := context.WithTimeout(ctx, cfg.TrainingTimeout)
	defer cancel()
	manifest, log, runErr := (platformtraining.LocalDocker{Binary: cfg.DockerBinary, VolumesFrom: cfg.DockerVolumesFrom}).Run(trainingCtx, platformtraining.LocalDockerSpec{
		RunID: run.ID, ImageRef: template.ImageRef, Entrypoint: template.Entrypoint, OutputDir: runDir,
		DatasetManifestURI: dataset.ManifestURI, ParametersJSON: run.Parameters, GPUCount: run.GPUCount,
		MemoryBytes: runtimeSpec.MemoryBytes, CPUs: runtimeSpec.CPUs,
	})
	if runErr != nil {
		category, code := classifyTrainingFailure(runErr)
		_ = storeTrainingLog(context.Background(), db, cfg, &run, log)
		return failTrainingRun(db, &run, category, code, runErr.Error())
	}
	run.Status, run.Progress = TrainingExporting, 85
	db.Save(&run)
	db.Model(&PlatformJob{}).Where("tenant_id = ? AND resource_type = ? AND resource_id = ?", run.TenantID, "TRAINING_RUN", run.ID).
		Updates(map[string]any{"stage": "EXPORTING", "progress": 85})
	objectStore, err := storage.NewMinIO(cfg)
	if err != nil {
		return failTrainingRun(db, &run, "EXTERNAL_SERVICE", "STORAGE_CONFIG_INVALID", err.Error())
	}
	if err = objectStore.EnsureBucket(ctx); err != nil {
		return failTrainingRun(db, &run, "EXTERNAL_SERVICE", "STORAGE_UNAVAILABLE", err.Error())
	}
	if err = collectTrainingResults(ctx, db, objectStore, &run, runDir, manifest, log); err != nil {
		return failTrainingRun(db, &run, "FRAMEWORK", "RESULT_MANIFEST_INVALID", err.Error())
	}
	now = time.Now()
	run.Status, run.Progress, run.FinishedAt = TrainingSucceeded, 100, &now
	run.MetricSummary = jsonValue(manifest.Summary)
	if err = db.Save(&run).Error; err != nil {
		return err
	}
	return finishTrainingJob(db, &run, true, "", "")
}

func processClearMLTraining(ctx context.Context, db *gorm.DB, cfg config.Config, run *TrainingRun, template TrainingTemplateVersion, dataset DatasetVersion) error {
	client, err := platformtraining.NewClearML(cfg.ClearMLAPIURL, cfg.ClearMLWebURL, cfg.ClearMLAccessKey, cfg.ClearMLSecretKey, 30*time.Second)
	if err != nil {
		return failTrainingRun(db, run, "CONFIGURATION", "CLEARML_CONFIG_INVALID", err.Error())
	}
	var parameters map[string]any
	_ = json.Unmarshal([]byte(run.Parameters), &parameters)
	parameters["visionaiDatasetManifest"] = dataset.ManifestURI
	parameters["visionaiRunId"] = run.ID
	trainingCtx, cancel := context.WithTimeout(ctx, cfg.TrainingTimeout)
	defer cancel()
	external, err := client.CreateAndEnqueue(trainingCtx, platformtraining.ClearMLSpec{
		Name: run.Name, Queue: run.Queue, ImageRef: template.ImageRef, Entrypoint: template.Entrypoint,
		Parameters: parameters, Tags: []string{"visionai", "project-" + strconv.FormatUint(run.ProjectID, 10)},
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
				now := time.Now()
				run.Status, run.Progress, run.FinishedAt = TrainingSucceeded, 100, &now
				run.ResultManifestURI = "clearml://tasks/" + external.ID
				run.MetricSummary = jsonValue(map[string]any{"provider": "CLEARML", "externalTaskId": external.ID})
				digest := sha256.Sum256([]byte(external.ID))
				db.Create(&TrainingArtifact{
					TenantID: run.TenantID, ProjectID: run.ProjectID, TrainingRunID: run.ID,
					Kind: "EXTERNAL_TASK", Name: external.ID, URI: binding.ExternalURL,
					SHA256: hex.EncodeToString(digest[:]), MediaType: "application/vnd.clearml.task",
				})
				db.Save(run)
				return finishTrainingJob(db, run, true, "", "")
			case "failed":
				return failTrainingRun(db, run, "FRAMEWORK", "CLEARML_TASK_FAILED", task.StatusMessage)
			case "stopped", "closed":
				return failTrainingRun(db, run, "EXTERNAL_SERVICE", "CLEARML_TASK_STOPPED", task.StatusMessage)
			}
		}
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
