package visionai

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&PlatformJob{}, &JobAttempt{}, &OutboxEvent{}, &InboxEvent{},
		&Project{}, &ProjectMember{}, &ProjectConfig{}, &AuditEvent{},
		&Asset{}, &UploadSession{}, &UploadChunk{}, &AssetReference{},
		&AssetImportRun{},
		&AssetCollection{}, &AssetCollectionItem{}, &AssetTag{},
		&AnnotationTask{}, &AnnotationRevision{}, &ExternalResourceBinding{},
		&CVATUserMapping{}, &WorkbenchTicket{}, &PreannotationRun{},
		&Dataset{}, &DatasetVersion{}, &DatasetVersionItem{},
		&DatasetValidationIssue{}, &DatasetUsage{},
		&TrainingTemplate{}, &TrainingTemplateVersion{}, &TrainingRun{},
		&TrainingArtifact{}, &TrainingMetric{},
		&EvaluationSuite{}, &EvaluationRun{}, &EvaluationMetric{}, &EvaluationSample{}, &EvaluationSavedSlice{},
		&Model{}, &ModelVersion{}, &ModelArtifact{}, &ApprovalRequest{}, &ApprovalDecision{},
		&Deployment{}, &DeploymentRevision{}, &InferenceTrace{}, &AlertRule{}, &AlertEvent{},
		&FeedbackPolicy{}, &FeedbackSample{}, &FeedbackBatch{},
		&ComputeNode{}, &ComputeQueue{}, &ProjectQuota{}, &IntegrationInstance{}, &CompatibilityRule{}, &SyncIncident{},
	); err != nil {
		return err
	}
	if db.Migrator().HasIndex(&ModelArtifact{}, "uk_model_artifact") {
		return db.Migrator().DropIndex(&ModelArtifact{}, "uk_model_artifact")
	}
	return nil
}

func Register(group *gin.RouterGroup, db *gorm.DB, auth gin.HandlerFunc) {
	h := NewHandler(db)
	sso := group.Group("/ai-platform/workbench-sso")
	sso.GET("/cvat", h.WorkbenchSSOCVAT)
	sso.GET("/fiftyone", h.WorkbenchSSOFiftyOne)
	sso.GET("/fiftyone/validate", h.WorkbenchSSOFiftyOneValidate)
	api := group.Group("/ai-platform", auth)
	api.GET("/dashboard/summary", h.DashboardSummary)
	api.GET("/jobs", h.JobPage)
	api.GET("/jobs/:id", h.JobGet)
	api.GET("/jobs/:id/events", h.JobEvents)
	api.POST("/jobs/:id/cancel", h.JobCancel)
	api.POST("/jobs/:id/retry", h.JobRetry)
	api.GET("/projects", h.ProjectPage)
	api.POST("/projects", h.ProjectCreate)
	api.GET("/projects/:id", h.ProjectGet)
	api.PUT("/projects/:id", h.ProjectUpdate)
	api.PUT("/projects/:id/status", h.ProjectStatusUpdate)
	api.POST("/projects/:id/archive", h.ProjectArchive)
	api.POST("/projects/:id/clone", h.ProjectClone)
	api.GET("/projects/:id/members", h.ProjectMemberList)
	api.PUT("/projects/:id/members", h.ProjectMemberUpsert)
	api.DELETE("/projects/:id/members/:userId", h.ProjectMemberDelete)
	api.GET("/projects/:id/config", h.ProjectConfigGet)
	api.PUT("/projects/:id/config", h.ProjectConfigUpdate)
	api.POST("/projects/:id/uploads", h.UploadCreate)
	api.GET("/projects/:id/uploads/:sessionId", h.UploadGet)
	api.PUT("/projects/:id/uploads/:sessionId/chunks/:part", h.UploadChunkPut)
	api.POST("/projects/:id/uploads/:sessionId/complete", h.UploadComplete)
	api.GET("/projects/:id/assets", h.AssetPage)
	api.GET("/projects/:id/assets/quality", h.AssetQuality)
	api.GET("/projects/:id/assets/:assetId", h.AssetGet)
	api.DELETE("/projects/:id/assets/:assetId", h.AssetDelete)
	api.POST("/projects/:id/assets/:assetId/restore", h.AssetRestore)
	api.POST("/projects/:id/assets/:assetId/purge", h.AssetPurge)
	api.GET("/projects/:id/asset-imports", h.AssetImportPage)
	api.POST("/projects/:id/asset-imports", h.AssetImportCreate)
	api.GET("/projects/:id/asset-imports/:importId", h.AssetImportGet)
	api.GET("/projects/:id/collections", h.CollectionPage)
	api.POST("/projects/:id/collections", h.CollectionCreate)
	api.PUT("/projects/:id/collections/:collectionId/assets", h.CollectionAddAssets)
	api.POST("/projects/:id/collections/:collectionId/freeze", h.CollectionFreeze)
	api.PUT("/projects/:id/assets/:assetId/tags", h.AssetTagsUpdate)
	api.GET("/cvat-user-mappings", h.CVATUserMappingList)
	api.PUT("/cvat-user-mappings", h.CVATUserMappingUpsert)
	api.GET("/projects/:id/annotation-tasks", h.AnnotationTaskPage)
	api.POST("/projects/:id/annotation-tasks", h.AnnotationTaskCreate)
	api.GET("/projects/:id/annotation-tasks/:taskId", h.AnnotationTaskGet)
	api.POST("/projects/:id/annotation-tasks/:taskId/prepare", h.AnnotationPrepare)
	api.POST("/projects/:id/annotation-tasks/:taskId/sync", h.AnnotationSync)
	api.PUT("/projects/:id/annotation-tasks/:taskId/status", h.AnnotationStatusUpdate)
	api.POST("/projects/:id/annotation-tasks/:taskId/review", h.AnnotationReview)
	api.POST("/projects/:id/annotation-tasks/:taskId/export", h.AnnotationExport)
	api.POST("/projects/:id/annotation-tasks/:taskId/workbench", h.AnnotationWorkbench)
	api.POST("/projects/:id/annotation-tasks/:taskId/preannotations", h.PreannotationCreate)
	api.PUT("/projects/:id/annotation-tasks/:taskId/preannotations/:runId/metrics", h.PreannotationMetricsUpdate)
	api.GET("/projects/:id/datasets", h.DatasetPage)
	api.POST("/projects/:id/datasets", h.DatasetCreate)
	api.GET("/projects/:id/datasets/:datasetId/versions", h.DatasetVersions)
	api.POST("/projects/:id/datasets/:datasetId/versions", h.DatasetVersionCreate)
	api.GET("/projects/:id/dataset-versions/compare", h.DatasetVersionCompare)
	api.GET("/projects/:id/dataset-versions/:versionId", h.DatasetVersionGet)
	api.POST("/projects/:id/dataset-versions/:versionId/validate", h.DatasetVersionValidate)
	api.POST("/projects/:id/dataset-versions/:versionId/freeze", h.DatasetVersionFreeze)
	api.POST("/projects/:id/dataset-versions/:versionId/deprecate", h.DatasetVersionDeprecate)
	api.GET("/projects/:id/training-templates", h.TrainingTemplatePage)
	api.POST("/projects/:id/training-templates", h.TrainingTemplateCreate)
	api.GET("/projects/:id/training-templates/:templateId/versions", h.TrainingTemplateVersions)
	api.POST("/projects/:id/training-templates/:templateId/versions", h.TrainingTemplateVersionCreate)
	api.POST("/projects/:id/training-template-versions/:templateVersionId/smoke", h.TrainingTemplateSmoke)
	api.POST("/projects/:id/training-template-versions/:templateVersionId/publish", h.TrainingTemplatePublish)
	api.GET("/projects/:id/training-runs", h.TrainingRunPage)
	api.POST("/projects/:id/training-runs", h.TrainingRunCreate)
	api.GET("/projects/:id/training-runs/compare", h.TrainingRunCompare)
	api.GET("/projects/:id/training-runs/:runId", h.TrainingRunGet)
	api.GET("/projects/:id/training-runs/:runId/export", h.TrainingRunExport)
	api.GET("/projects/:id/training-runs/:runId/artifacts/:artifactId/download", h.TrainingArtifactDownload)
	api.POST("/projects/:id/training-runs/:runId/cancel", h.TrainingRunCancel)
	api.POST("/projects/:id/training-runs/:runId/clone", h.TrainingRunClone)
	api.GET("/projects/:id/evaluation-suites", h.EvaluationSuitePage)
	api.POST("/projects/:id/evaluation-suites", h.EvaluationSuiteCreate)
	api.GET("/projects/:id/evaluation-runs", h.EvaluationRunPage)
	api.POST("/projects/:id/evaluation-suites/:suiteId/runs", h.EvaluationRunCreate)
	api.GET("/projects/:id/evaluation-runs/:runId", h.EvaluationRunGet)
	api.POST("/projects/:id/evaluation-runs/:runId/workbench", h.EvaluationWorkbench)
	api.GET("/projects/:id/models", h.ModelPage)
	api.POST("/projects/:id/models/register", h.ModelRegister)
	api.POST("/projects/:id/models/import", h.ModelImport)
	api.GET("/projects/:id/model-versions/compare", h.ModelVersionCompare)
	api.GET("/projects/:id/model-versions/:versionId", h.ModelVersionGet)
	api.GET("/projects/:id/model-versions/:versionId/export-manifest", h.ModelExportManifest)
	api.POST("/projects/:id/model-versions/:versionId/retire", h.ModelVersionRetire)
	api.POST("/projects/:id/model-versions/:versionId/approvals", h.ApprovalSubmit)
	api.GET("/projects/:id/approvals", h.ApprovalPage)
	api.POST("/projects/:id/approvals/:approvalId/decision", h.ApprovalDecide)
	api.GET("/projects/:id/deployments", h.DeploymentPage)
	api.POST("/projects/:id/deployments", h.DeploymentCreate)
	api.GET("/projects/:id/deployments/:deploymentId", h.DeploymentGet)
	api.POST("/projects/:id/deployments/:deploymentId/predict", h.DeploymentPredict)
	api.POST("/projects/:id/deployments/:deploymentId/predict-image", h.DeploymentPredictImage)
	api.POST("/projects/:id/deployments/:deploymentId/predict-video", h.DeploymentPredictVideo)
	api.POST("/projects/:id/deployments/:deploymentId/revisions", h.DeploymentRevisionCreate)
	api.POST("/projects/:id/deployments/:deploymentId/rollback", h.DeploymentRollback)
	api.POST("/projects/:id/deployments/:deploymentId/stop", h.DeploymentStop)
	api.POST("/projects/:id/deployments/:deploymentId/restart", h.DeploymentRestart)
	api.POST("/projects/:id/deployments/:deploymentId/alert-rules", h.AlertRuleCreate)
	api.POST("/projects/:id/alerts/:alertId/acknowledge", h.AlertAcknowledge)
	api.GET("/projects/:id/feedback-policy", h.FeedbackPolicyGet)
	api.PUT("/projects/:id/feedback-policy", h.FeedbackPolicySave)
	api.GET("/projects/:id/feedback-samples", h.FeedbackSamplePage)
	api.POST("/projects/:id/feedback-batches", h.FeedbackBatchCreate)
	api.GET("/projects/:id/feedback-batches", h.FeedbackBatchPage)
	api.GET("/projects/:id/feedback-batches/:batchId", h.FeedbackBatchGet)
	api.POST("/projects/:id/feedback-batches/:batchId/review", h.FeedbackBatchReview)
	api.POST("/projects/:id/feedback-cleanup", h.FeedbackCleanup)
	api.GET("/resources/overview", h.ResourceOverview)
	api.POST("/resources/nodes/heartbeat", h.ComputeNodeHeartbeat)
	api.PUT("/resources/queues", h.ComputeQueueSave)
	api.PUT("/projects/:id/quota", h.ProjectQuotaSave)
	api.GET("/integrations", h.IntegrationPage)
	api.POST("/integrations", h.IntegrationCreate)
	api.POST("/integrations/:instanceId/test", h.IntegrationTest)
	api.POST("/compatibility-rules", h.CompatibilityRuleCreate)
	api.POST("/sync-incidents/:incidentId/replay", h.SyncIncidentReplay)
	api.GET("/audit-events", h.AuditPage)
	api.GET("/audit-events/export", h.AuditExport)
}
