package visionai

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/annotation"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"gorm.io/gorm"
)

// SyncActiveAnnotationTasks periodically reconciles CVAT progress with the governed
// platform task without bypassing the platform's business approval state.
func SyncActiveAnnotationTasks(ctx context.Context, db *gorm.DB, cfg config.Config) error {
	provider, err := annotation.NewCVAT(
		cfg.CVATBaseURL, cfg.CVATPublicURL, cfg.CVATUsername, cfg.CVATPassword, cfg.CVATTimeout,
	)
	if err != nil {
		return err
	}
	var tasks []AnnotationTask
	if err = db.Where("status IN ?", []AnnotationStatus{
		AnnotationPreparing, AnnotationReady, AnnotationAnnotating, AnnotationReviewing,
	}).Order("id").Limit(500).Find(&tasks).Error; err != nil {
		return err
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var binding ExternalResourceBinding
		if db.Where(
			"tenant_id = ? AND provider_type = ? AND internal_type = ? AND internal_id = ?",
			task.TenantID, "CVAT", "ANNOTATION_TASK", task.ID,
		).First(&binding).Error != nil {
			continue
		}
		if binding.LastSyncAt != nil && time.Since(*binding.LastSyncAt) < 20*time.Second {
			continue
		}
		externalID, parseErr := strconv.ParseInt(binding.ExternalID, 10, 64)
		if parseErr != nil || externalID == 0 {
			recordAnnotationSyncIncident(db, task, binding, "MAPPING_MISSING", "CVAT Task ID 无效")
			continue
		}
		external, syncErr := provider.GetTask(ctx, externalID)
		now := time.Now()
		if syncErr != nil {
			recordAnnotationSyncIncident(db, task, binding, "FAILED_SYNC", syncErr.Error())
			continue
		}
		before := task
		binding.LastSyncAt = &now
		binding.SyncCursor = external.Status + ":" + strconv.Itoa(external.Progress)
		task.Progress = external.Progress
		externalStatus := strings.ToLower(strings.TrimSpace(external.Status))
		switch {
		case task.Status == AnnotationPreparing && externalStatus == "annotation":
			task.Status = AnnotationReady
		case task.Status == AnnotationReady && external.Progress > 0:
			task.Status = AnnotationAnnotating
		case task.Status == AnnotationAnnotating &&
			(externalStatus == "validation" || externalStatus == "completed"):
			task.Status = AnnotationReviewing
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			if saveErr := tx.Save(&binding).Error; saveErr != nil {
				return saveErr
			}
			if saveErr := tx.Save(&task).Error; saveErr != nil {
				return saveErr
			}
			if before.Status != task.Status || before.Progress != task.Progress {
				return appendSystemAudit(tx, task, "ANNOTATION_CVAT_SYNCED", before, task)
			}
			return nil
		})
		if err != nil {
			return err
		}
		db.Model(&SyncIncident{}).Where(
			"tenant_id = ? AND instance_id = ? AND resource_type = ? AND resource_id = ? AND status = ?",
			task.TenantID, annotationSyncInstanceID(db, task.TenantID), "ANNOTATION_TASK", strconv.FormatUint(task.ID, 10), "OPEN",
		).Updates(map[string]any{"status": "RESOLVED", "resolved_at": now})
	}
	return nil
}

func recordAnnotationSyncIncident(db *gorm.DB, task AnnotationTask, binding ExternalResourceBinding, code, message string) {
	now := time.Now()
	incident := SyncIncident{
		TenantID: task.TenantID, InstanceID: annotationSyncInstanceID(db, task.TenantID), ResourceType: "ANNOTATION_TASK",
		ResourceID: strconv.FormatUint(task.ID, 10), Code: code, Message: message,
		ExternalID: binding.ExternalID, Status: "OPEN", Attempts: 1, LastAttemptAt: &now,
	}
	var existing SyncIncident
	result := db.Where(
		"tenant_id = ? AND instance_id = ? AND resource_type = ? AND resource_id = ? AND code = ? AND status = ?",
		incident.TenantID, incident.InstanceID, incident.ResourceType, incident.ResourceID, incident.Code, incident.Status,
	).First(&existing)
	if result.Error == nil {
		db.Model(&existing).Updates(map[string]any{
			"message": message, "last_attempt_at": now, "attempts": gorm.Expr("attempts + 1"),
		})
		return
	}
	db.Create(&incident)
}

func annotationSyncInstanceID(db *gorm.DB, tenantID uint64) uint64 {
	var instance IntegrationInstance
	if db.Where("tenant_id = ? AND provider_type = ?", tenantID, "CVAT").Order("id DESC").First(&instance).Error == nil {
		return instance.ID
	}
	return 0
}

func appendSystemAudit(tx *gorm.DB, task AnnotationTask, action string, before, after any) error {
	beforeJSON, afterJSON := jsonValue(before), jsonValue(after)
	row := AuditEvent{
		TenantID: task.TenantID, ProjectID: task.ProjectID, ActorUserID: 0,
		Action: action, ResourceType: "ANNOTATION_TASK", ResourceID: task.ID,
		Result:     "SUCCESS",
		BeforeJSON: beforeJSON, AfterJSON: afterJSON,
		TraceID: fmt.Sprintf("scheduled-cvat-sync-%d-%d", task.ID, time.Now().Unix()),
	}
	return tx.Create(&row).Error
}
