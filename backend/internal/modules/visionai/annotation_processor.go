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
	"path"
	"strconv"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/annotation"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/storage"
	"gorm.io/gorm"
)

func processAnnotationEvent(ctx context.Context, db *gorm.DB, cfg config.Config, event DomainEventEnvelope) error {
	var payload struct {
		AnnotationTaskID uint64 `json:"annotationTaskId"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil || payload.AnnotationTaskID == 0 {
		return errors.New("invalid annotation event payload")
	}
	var task AnnotationTask
	if db.Where("tenant_id = ? AND id = ?", event.TenantID, payload.AnnotationTaskID).First(&task).Error != nil {
		return nil
	}
	provider, err := annotation.NewCVAT(cfg.CVATBaseURL, cfg.CVATPublicURL, cfg.CVATUsername, cfg.CVATPassword, cfg.CVATTimeout)
	if err != nil {
		return markAnnotationFailure(db, &task, "CVAT_CONFIG_INVALID", err)
	}
	objectStore, err := storage.NewMinIO(cfg)
	if err != nil {
		return markAnnotationFailure(db, &task, "STORAGE_CONFIG_INVALID", err)
	}
	if err = objectStore.EnsureBucket(ctx); err != nil {
		return markAnnotationFailure(db, &task, "STORAGE_UNAVAILABLE", err)
	}
	switch event.EventType {
	case "annotation.task.prepare.v1":
		err = prepareCVATTask(ctx, db, provider, objectStore, &task)
	case "annotation.export.requested.v1":
		err = exportAnnotationRevision(ctx, db, provider, objectStore, &task)
	}
	if err != nil {
		return markAnnotationFailure(db, &task, "CVAT_OPERATION_FAILED", err)
	}
	return nil
}

func markAnnotationFailure(db *gorm.DB, task *AnnotationTask, code string, err error) error {
	now := time.Now()
	task.Status, task.ErrorCode, task.ErrorMessage = AnnotationFailed, code, err.Error()
	db.Transaction(func(tx *gorm.DB) error {
		if saveErr := tx.Save(task).Error; saveErr != nil {
			return saveErr
		}
		return tx.Model(&PlatformJob{}).
			Where("tenant_id = ? AND resource_type = ? AND resource_id = ? AND status IN ?", task.TenantID, "ANNOTATION_TASK", task.ID, []JobStatus{JobQueued, JobRunning}).
			Updates(map[string]any{"status": JobFailed, "stage": "FAILED", "error_code": code, "error_message": err.Error(), "finished_at": now}).Error
	})
	// The durable failure is acknowledged. A user retry generates a new event
	// while the existing binding keeps external creation idempotent.
	return nil
}

func prepareCVATTask(ctx context.Context, db *gorm.DB, provider annotation.Provider, objectStore storage.Provider, task *AnnotationTask) error {
	var annotatorIDs []uint64
	if err := json.Unmarshal([]byte(task.AnnotatorIDs), &annotatorIDs); err != nil || len(annotatorIDs) == 0 {
		var project Project
		if projectErr := db.Select("owner_user_id").Where(
			"tenant_id = ? AND id = ?", task.TenantID, task.ProjectID,
		).First(&project).Error; projectErr != nil || project.OwnerUserID == 0 {
			return errors.New("annotation task has no valid annotators")
		}
		annotatorIDs = []uint64{project.OwnerUserID}
		task.AnnotatorIDs = jsonValue(annotatorIDs)
		if saveErr := db.Model(task).Update("annotator_ids", task.AnnotatorIDs).Error; saveErr != nil {
			return fmt.Errorf("repair annotation task owner assignment: %w", saveErr)
		}
	}
	identities, err := (&Handler{db: db}).ensureCVATIdentities(ctx, task.TenantID, annotatorIDs)
	if err != nil {
		return err
	}
	var mediaCount int64
	if err = db.Model(&AssetCollectionItem{}).
		Where("tenant_id = ? AND collection_id = ?", task.TenantID, task.CollectionID).
		Count(&mediaCount).Error; err != nil {
		return err
	}
	if mediaCount == 0 {
		return errors.New("frozen collection contains no assets")
	}
	segmentSize := int((mediaCount + int64(len(identities)) - 1) / int64(len(identities)))
	var binding ExternalResourceBinding
	result := db.Where("tenant_id = ? AND provider_type = ? AND internal_type = ? AND internal_id = ?",
		task.TenantID, "CVAT", "ANNOTATION_TASK", task.ID).First(&binding)
	var external annotation.Task
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		var labels []annotation.Label
		if err = json.Unmarshal([]byte(task.Labels), &labels); err != nil {
			return fmt.Errorf("decode labels: %w", err)
		}
		if err = provider.About(ctx); err != nil {
			return fmt.Errorf("CVAT unavailable: %w", err)
		}
		external, err = provider.CreateTask(ctx, task.Name, labels, segmentSize)
		if err != nil {
			return fmt.Errorf("create CVAT task: %w", err)
		}
		binding = ExternalResourceBinding{
			TenantID: task.TenantID, ProviderType: "CVAT", InstanceID: "default",
			InternalType: "ANNOTATION_TASK", InternalID: task.ID, ExternalType: "TASK",
			ExternalID: strconv.FormatInt(external.ID, 10), ExternalURL: provider.TaskURL(external.ID), SyncCursor: "CREATED",
		}
		if err = db.Create(&binding).Error; err != nil {
			return fmt.Errorf("save CVAT binding: %w", err)
		}
	} else if result.Error != nil {
		return result.Error
	} else {
		external.ID, _ = strconv.ParseInt(binding.ExternalID, 10, 64)
	}
	if binding.SyncCursor != "DATA_ATTACHED" {
		remote, getErr := provider.GetTask(ctx, external.ID)
		if getErr == nil && remote.Size > 0 {
			binding.SyncCursor = "DATA_ATTACHED"
		} else {
			var items []AssetCollectionItem
			if err = db.Where("tenant_id = ? AND collection_id = ?", task.TenantID, task.CollectionID).Order("id").Find(&items).Error; err != nil {
				return err
			}
			if len(items) == 0 {
				return errors.New("frozen collection contains no assets")
			}
			assetIDs := make([]uint64, 0, len(items))
			for _, item := range items {
				assetIDs = append(assetIDs, item.AssetID)
			}
			var assets []Asset
			if err = db.Where("tenant_id = ? AND project_id = ? AND id IN ? AND status = ?", task.TenantID, task.ProjectID, assetIDs, AssetReady).Order("id").Find(&assets).Error; err != nil {
				return err
			}
			if len(assets) != len(items) {
				return errors.New("collection contains missing or unavailable assets")
			}
			media := make([]annotation.Media, 0, len(assets))
			closers := make([]io.Closer, 0, len(assets))
			defer func() {
				for _, closer := range closers {
					_ = closer.Close()
				}
			}()
			for _, asset := range assets {
				reader, _, getErr := objectStore.Get(ctx, asset.ObjectKey)
				if getErr != nil {
					return fmt.Errorf("read asset %d: %w", asset.ID, getErr)
				}
				closers = append(closers, reader)
				media = append(media, annotation.Media{Name: asset.Filename, Reader: reader})
			}
			if err = provider.AttachData(ctx, external.ID, media); err != nil {
				return fmt.Errorf("attach CVAT data: %w", err)
			}
			binding.SyncCursor = "DATA_ATTACHED"
		}
	}
	if cvat, ok := provider.(*annotation.CVAT); ok {
		if _, err = cvat.AssignTaskJobs(ctx, external.ID, mappingIDs(identities)); err != nil {
			return fmt.Errorf("assign CVAT jobs: %w", err)
		}
		binding.SyncCursor = "JOBS_ASSIGNED"
	}
	now := time.Now()
	binding.LastSyncAt = &now
	task.ExternalBindingID, task.Status, task.Progress = binding.ID, AnnotationReady, 0
	task.ErrorCode, task.ErrorMessage = "", ""
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&binding).Error; err != nil {
			return err
		}
		if err := tx.Save(task).Error; err != nil {
			return err
		}
		return tx.Model(&PlatformJob{}).
			Where("tenant_id = ? AND resource_type = ? AND resource_id = ? AND status IN ?", task.TenantID, "ANNOTATION_TASK", task.ID, []JobStatus{JobQueued, JobRunning}).
			Updates(map[string]any{"status": JobSucceeded, "stage": "READY", "progress": 100, "finished_at": now}).Error
	})
}

func countAnnotations(raw []byte) int64 {
	var data struct {
		Shapes []json.RawMessage `json:"shapes"`
		Tags   []json.RawMessage `json:"tags"`
		Tracks []json.RawMessage `json:"tracks"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return 0
	}
	return int64(len(data.Shapes) + len(data.Tags) + len(data.Tracks))
}

func exportAnnotationRevision(ctx context.Context, db *gorm.DB, provider annotation.Provider, objectStore storage.Provider, task *AnnotationTask) error {
	var binding ExternalResourceBinding
	if db.Where("tenant_id = ? AND internal_type = ? AND internal_id = ?", task.TenantID, "ANNOTATION_TASK", task.ID).First(&binding).Error != nil {
		return errors.New("CVAT binding missing")
	}
	externalID, _ := strconv.ParseInt(binding.ExternalID, 10, 64)
	raw, err := provider.GetAnnotations(ctx, externalID)
	if err != nil {
		return fmt.Errorf("download CVAT annotations: %w", err)
	}
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])
	categoryMapping, err := provider.GetTaskLabels(ctx, externalID)
	if err != nil {
		return fmt.Errorf("download CVAT category mapping: %w", err)
	}
	var latest AnnotationRevision
	revisionNo := 1
	if db.Where("tenant_id = ? AND annotation_task_id = ?", task.TenantID, task.ID).Order("revision_no DESC").First(&latest).Error == nil {
		if latest.Checksum == checksum {
			task.Status, task.CurrentRevisionID = AnnotationClosed, latest.ID
			return db.Save(task).Error
		}
		revisionNo = latest.RevisionNo + 1
	}
	objectKey := path.Join(assetRoot(task.TenantID, task.ProjectID), "annotations", strconv.FormatUint(task.ID, 10), fmt.Sprintf("revision-%04d.json", revisionNo))
	if err = objectStore.Put(ctx, objectKey, bytes.NewReader(raw), int64(len(raw)), "application/json"); err != nil {
		return err
	}
	revision := AnnotationRevision{
		TenantID: task.TenantID, ProjectID: task.ProjectID, AnnotationTaskID: task.ID, RevisionNo: revisionNo,
		SnapshotURI: objectStore.URI(objectKey), ObjectKey: objectKey, Format: "CVAT_JSON",
		Checksum: checksum, CategoryMapping: jsonValue(categoryMapping), OntologyVersionID: task.OntologyVersionID,
		OntologyChecksum: task.OntologyChecksum, AnnotationCount: countAnnotations(raw), ApprovedBy: task.CreatedBy,
	}
	now := time.Now()
	return db.Transaction(func(tx *gorm.DB) error {
		if err = tx.Create(&revision).Error; err != nil {
			return err
		}
		task.Status, task.CurrentRevisionID, task.Progress = AnnotationClosed, revision.ID, 100
		task.ErrorCode, task.ErrorMessage = "", ""
		if err = tx.Save(task).Error; err != nil {
			return err
		}
		return tx.Model(&PlatformJob{}).
			Where("tenant_id = ? AND resource_type = ? AND resource_id = ? AND status IN ?", task.TenantID, "ANNOTATION_TASK", task.ID, []JobStatus{JobQueued, JobRunning}).
			Updates(map[string]any{"status": JobSucceeded, "stage": "CLOSED", "progress": 100, "finished_at": now}).Error
	})
}
