package visionai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	platforminference "github.com/lohasle/nimbus-framework-go/internal/platform/inference"
	"gorm.io/gorm"
)

func processDeploymentEvent(ctx context.Context, db *gorm.DB, cfg config.Config, event DomainEventEnvelope) error {
	var payload struct {
		RevisionID uint64 `json:"revisionId"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil || payload.RevisionID == 0 {
		return errors.New("invalid deployment event payload")
	}
	var revision DeploymentRevision
	if db.Where("tenant_id = ? AND id = ?", event.TenantID, payload.RevisionID).First(&revision).Error != nil || revision.Status == "RUNNING" {
		return nil
	}
	var deployment Deployment
	if db.Where("tenant_id = ? AND id = ?", event.TenantID, revision.DeploymentID).First(&deployment).Error != nil {
		return nil
	}
	revision.Status, deployment.Status = "DEPLOYING", "DEPLOYING"
	db.Save(&revision)
	db.Save(&deployment)
	db.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "DEPLOYMENT_REVISION", revision.ID).
		Updates(map[string]any{"status": JobRunning, "stage": "DEPLOYING", "progress": 40, "started_at": time.Now()})
	var values map[string]any
	_ = json.Unmarshal([]byte(revision.Config), &values)
	client := platforminference.NewClient(cfg.InferenceAPIURL, cfg.InferenceTimeout)
	err := client.Activate(ctx, platforminference.Revision{
		DeploymentID: deployment.ID, RevisionID: revision.ID, ModelVersionID: revision.ModelVersionID,
		ArtifactURI: revision.ArtifactURI, ArtifactSHA256: revision.ArtifactSHA256, Config: values,
	})
	if err != nil {
		now := time.Now()
		revision.Status, deployment.Status = "FAILED", "FAILED"
		db.Save(&revision)
		db.Save(&deployment)
		return db.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "DEPLOYMENT_REVISION", revision.ID).
			Updates(map[string]any{"status": JobFailed, "stage": "FAILED", "error_code": "PROVIDER_ACTIVATION_FAILED", "error_message": err.Error(), "finished_at": now}).Error
	}
	now := time.Now()
	return db.Transaction(func(tx *gorm.DB) error {
		revision.Status, revision.ActivatedAt = "RUNNING", &now
		if err := tx.Save(&revision).Error; err != nil {
			return err
		}
		if err := tx.Model(&DeploymentRevision{}).Where("deployment_id = ? AND id <> ? AND status = ?", deployment.ID, revision.ID, "RUNNING").Update("status", "SUPERSEDED").Error; err != nil {
			return err
		}
		deployment.Status, deployment.CurrentRevisionID, deployment.EndpointURL = "RUNNING", revision.ID, cfg.InferencePublicURL+"/v1/predict"
		if err := tx.Save(&deployment).Error; err != nil {
			return err
		}
		return tx.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "DEPLOYMENT_REVISION", revision.ID).
			Updates(map[string]any{"status": JobSucceeded, "stage": "RUNNING", "progress": 100, "finished_at": now}).Error
	})
}
