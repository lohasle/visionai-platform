package visionai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	platforminference "github.com/lohasle/nimbus-framework-go/internal/platform/inference"
	"gorm.io/gorm"
)

func syncModelDeploymentLifecycle(tx *gorm.DB, tenantID, projectID uint64, modelVersionIDs ...uint64) error {
	seen := map[uint64]struct{}{}
	for _, modelVersionID := range modelVersionIDs {
		if modelVersionID == 0 {
			continue
		}
		if _, ok := seen[modelVersionID]; ok {
			continue
		}
		seen[modelVersionID] = struct{}{}
		var version ModelVersion
		if tx.Where("tenant_id = ? AND project_id = ? AND id = ?", tenantID, projectID, modelVersionID).First(&version).Error != nil ||
			strings.EqualFold(version.Status, "RETIRED") {
			continue
		}
		var environments []string
		tx.Table("ai_deployment_revision AS revision").
			Select("deployment.environment").
			Joins("JOIN ai_deployment AS deployment ON deployment.id = revision.deployment_id AND deployment.tenant_id = revision.tenant_id").
			Where("revision.tenant_id = ? AND revision.project_id = ? AND revision.model_version_id = ? AND revision.status = ? AND deployment.status = ?",
				tenantID, projectID, modelVersionID, "RUNNING", "RUNNING").
			Scan(&environments)
		status := "DRAFT"
		if version.EvaluationRunID > 0 {
			status = "EVALUATED"
		}
		if version.ApprovedAt != nil {
			status = "APPROVED"
		}
		rank := map[string]int{"STAGING": 1, "CANARY": 2, "PRODUCTION": 3}
		for _, environment := range environments {
			if rank[strings.ToUpper(environment)] > rank[status] {
				status = strings.ToUpper(environment)
			}
		}
		if err := tx.Model(&version).Update("status", status).Error; err != nil {
			return err
		}
	}
	return nil
}

func modelProductionEligible(version ModelVersion) bool {
	if version.ApprovedAt == nil {
		return false
	}
	switch strings.ToUpper(version.Status) {
	case "APPROVED", "STAGING", "CANARY", "PRODUCTION":
		return true
	default:
		return false
	}
}

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
		var previousModelIDs []uint64
		tx.Model(&DeploymentRevision{}).
			Where("deployment_id = ? AND id <> ? AND status = ?", deployment.ID, revision.ID, "RUNNING").
			Pluck("model_version_id", &previousModelIDs)
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
		if err := syncModelDeploymentLifecycle(tx, deployment.TenantID, deployment.ProjectID, append(previousModelIDs, revision.ModelVersionID)...); err != nil {
			return err
		}
		return tx.Model(&PlatformJob{}).Where("resource_type = ? AND resource_id = ?", "DEPLOYMENT_REVISION", revision.ID).
			Updates(map[string]any{"status": JobSucceeded, "stage": "RUNNING", "progress": 100, "finished_at": now}).Error
	})
}
