package visionai

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

type feedbackPolicyRequest struct {
	Enabled         bool           `json:"enabled"`
	RandomRate      float64        `json:"randomRate"`
	ConfidenceBelow float64        `json:"confidenceBelow"`
	CaptureEmpty    bool           `json:"captureEmpty"`
	CaptureErrors   bool           `json:"captureErrors"`
	DailyLimit      int            `json:"dailyLimit"`
	RetentionDays   int            `json:"retentionDays"`
	RedactionPolicy map[string]any `json:"redactionPolicy"`
	SensitiveReview bool           `json:"sensitiveReview"`
}

func (h *Handler) FeedbackPolicyGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var policy FeedbackPolicy
	if h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).First(&policy).Error != nil {
		httpx.OK(c, nil)
		return
	}
	httpx.OK(c, policy)
}

func (h *Handler) FeedbackPolicySave(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "DATA_MANAGER", "PROJECT_OWNER") {
		return
	}
	var req feedbackPolicyRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "反馈策略无效")
		return
	}
	if req.RandomRate < 0 || req.RandomRate > 1 || req.ConfidenceBelow < 0 || req.ConfidenceBelow > 1 {
		httpx.Fail(c, 400, 400, "采样比例和置信度必须在 0..1")
		return
	}
	if req.DailyLimit <= 0 {
		req.DailyLimit = 1000
	}
	if req.RetentionDays <= 0 {
		req.RetentionDays = 30
	}
	var row FeedbackPolicy
	err := h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).First(&row).Error
	before := row
	row.TenantID, row.ProjectID, row.Enabled = project.TenantID, project.ID, req.Enabled
	row.RandomRate, row.ConfidenceBelow, row.CaptureEmpty, row.CaptureErrors = req.RandomRate, req.ConfidenceBelow, req.CaptureEmpty, req.CaptureErrors
	row.DailyLimit, row.RetentionDays, row.RedactionPolicy, row.SensitiveReview = req.DailyLimit, req.RetentionDays, jsonValue(req.RedactionPolicy), req.SensitiveReview
	if err == gorm.ErrRecordNotFound {
		row.CreatedBy = c.GetUint64("user_id")
		err = h.db.Create(&row).Error
	} else if err == nil {
		err = h.db.Save(&row).Error
	}
	if err != nil {
		httpx.Fail(c, 500, 500, "反馈策略保存失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "FEEDBACK_POLICY_SAVED", "FEEDBACK_POLICY", row.ID, before, row)
	httpx.OK(c, row)
}

func captureFeedback(db *gorm.DB, trace InferenceTrace) {
	var policy FeedbackPolicy
	if db.Where("tenant_id = ? AND project_id = ? AND enabled = ?", trace.TenantID, trace.ProjectID, true).First(&policy).Error != nil {
		return
	}
	reason := ""
	switch {
	case trace.Status == "FAILED" && policy.CaptureErrors:
		reason = "ERROR"
	case trace.DetectionCount == 0 && policy.CaptureEmpty:
		reason = "EMPTY_RESULT"
	case trace.MeanConfidence < policy.ConfidenceBelow:
		reason = "LOW_CONFIDENCE"
	case policy.RandomRate > 0 && float64(trace.ID%10000)/10000 < policy.RandomRate:
		reason = "RANDOM"
	}
	if reason == "" {
		return
	}
	start := time.Now().Truncate(24 * time.Hour)
	var daily int64
	db.Model(&FeedbackSample{}).Where("policy_id = ? AND created_at >= ?", policy.ID, start).Count(&daily)
	if daily >= int64(policy.DailyLimit) {
		return
	}
	var asset Asset
	if db.Where("tenant_id = ? AND id = ?", trace.TenantID, trace.AssetID).First(&asset).Error != nil {
		return
	}
	var duplicate int64
	db.Model(&FeedbackSample{}).Where("tenant_id = ? AND project_id = ? AND perceptual_hash = ? AND status IN ?", trace.TenantID, trace.ProjectID, asset.SHA256, []string{"PENDING", "BATCHED", "ACCEPTED"}).Count(&duplicate)
	status := "PENDING"
	if duplicate > 0 {
		status = "DUPLICATE"
	}
	db.Create(&FeedbackSample{
		TenantID: trace.TenantID, ProjectID: trace.ProjectID, PolicyID: policy.ID,
		InferenceTraceID: trace.ID, DeploymentRevisionID: trace.DeploymentRevisionID,
		ModelVersionID: trace.ModelVersionID, AssetID: trace.AssetID, Reason: reason,
		PerceptualHash: asset.SHA256, Status: status, ExpiresAt: time.Now().AddDate(0, 0, policy.RetentionDays),
	})
}

func (h *Handler) FeedbackSamplePage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var rows []FeedbackSample
	query := h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", strings.ToUpper(status))
	}
	query.Order("id DESC").Limit(500).Find(&rows)
	httpx.OK(c, rows)
}

type feedbackBatchRequest struct {
	Name      string   `json:"name"`
	SampleIDs []uint64 `json:"sampleIds"`
}

func (h *Handler) FeedbackBatchCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER", "PROJECT_OWNER") {
		return
	}
	var req feedbackBatchRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, 400, 400, "批次名称必填")
		return
	}
	query := h.db.Where("tenant_id = ? AND project_id = ? AND status = ? AND expires_at > ?", project.TenantID, project.ID, "PENDING", time.Now())
	if len(req.SampleIDs) > 0 {
		query = query.Where("id IN ?", req.SampleIDs)
	}
	var samples []FeedbackSample
	query.Order("id").Limit(500).Find(&samples)
	if len(samples) == 0 {
		httpx.Fail(c, 409, 409, "没有可入批的待审样本")
		return
	}
	row := FeedbackBatch{TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name), Status: "READY", SampleCount: len(samples), CreatedBy: c.GetUint64("user_id")}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		ids := make([]uint64, 0, len(samples))
		for _, sample := range samples {
			ids = append(ids, sample.ID)
		}
		if err := tx.Model(&FeedbackSample{}).Where("id IN ? AND status = ?", ids, "PENDING").Updates(map[string]any{"status": "BATCHED", "batch_id": row.ID}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "FEEDBACK_BATCH_CREATED", "FEEDBACK_BATCH", row.ID, nil, gin.H{"sampleCount": len(samples)})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "反馈批次创建失败")
		return
	}
	httpx.OK(c, row)
}

func (h *Handler) FeedbackBatchPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var rows []FeedbackBatch
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Order("id DESC").Find(&rows)
	httpx.OK(c, rows)
}

func (h *Handler) FeedbackBatchGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	batchID, _ := strconv.ParseUint(c.Param("batchId"), 10, 64)
	var batch FeedbackBatch
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, batchID).First(&batch).Error != nil {
		httpx.Fail(c, 404, 404, "反馈批次不存在")
		return
	}
	var samples []FeedbackSample
	h.db.Where("batch_id = ?", batch.ID).Order("id").Find(&samples)
	var traceIDs []uint64
	for _, sample := range samples {
		traceIDs = append(traceIDs, sample.InferenceTraceID)
	}
	var traces []InferenceTrace
	if len(traceIDs) > 0 {
		h.db.Where("id IN ?", traceIDs).Order("id").Find(&traces)
	}
	var task AnnotationTask
	var annotationRevision AnnotationRevision
	var datasetVersions []DatasetVersion
	var modelVersions []ModelVersion
	var deploymentRevisions []DeploymentRevision
	if batch.AnnotationTaskID != 0 {
		h.db.Where("id = ?", batch.AnnotationTaskID).First(&task)
		h.db.Where("annotation_task_id = ?", task.ID).Order("revision_no DESC").First(&annotationRevision)
	}
	if annotationRevision.ID != 0 {
		h.db.Where("tenant_id = ? AND annotation_revision_id = ?", project.TenantID, annotationRevision.ID).Order("id").Find(&datasetVersions)
		if len(datasetVersions) > 0 {
			batch.AnnotationRevisionID = annotationRevision.ID
			batch.DatasetVersionID = datasetVersions[len(datasetVersions)-1].ID
			var datasetIDs []uint64
			for _, version := range datasetVersions {
				datasetIDs = append(datasetIDs, version.ID)
			}
			h.db.Where("tenant_id = ? AND dataset_version_id IN ?", project.TenantID, datasetIDs).Order("id").Find(&modelVersions)
		}
	}
	if len(modelVersions) > 0 {
		batch.ModelVersionID = modelVersions[len(modelVersions)-1].ID
		var modelIDs []uint64
		for _, version := range modelVersions {
			modelIDs = append(modelIDs, version.ID)
		}
		h.db.Where("tenant_id = ? AND model_version_id IN ?", project.TenantID, modelIDs).Order("id").Find(&deploymentRevisions)
	}
	if batch.AnnotationRevisionID != 0 || batch.DatasetVersionID != 0 || batch.ModelVersionID != 0 {
		h.db.Save(&batch)
	}
	httpx.OK(c, gin.H{
		"batch": batch, "samples": samples, "inferenceTraces": traces, "annotationTask": task,
		"annotationRevision": annotationRevision, "datasetVersions": datasetVersions,
		"modelVersions": modelVersions, "deploymentRevisions": deploymentRevisions,
	})
}

type feedbackReviewRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

func (h *Handler) FeedbackBatchReview(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "REVIEWER", "DATA_MANAGER", "PROJECT_OWNER") {
		return
	}
	batchID, _ := strconv.ParseUint(c.Param("batchId"), 10, 64)
	var batch FeedbackBatch
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, batchID, "READY").First(&batch).Error != nil {
		httpx.Fail(c, 409, 409, "批次不存在或已审核")
		return
	}
	var req feedbackReviewRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Comment) == "" {
		httpx.Fail(c, 400, 400, "审核意见必填")
		return
	}
	req.Decision = strings.ToUpper(req.Decision)
	if req.Decision != "ACCEPT" && req.Decision != "REJECT" {
		httpx.Fail(c, 400, 400, "决定必须是 ACCEPT 或 REJECT")
		return
	}
	now := time.Now()
	if req.Decision == "REJECT" {
		h.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&batch).Updates(map[string]any{"status": "REJECTED", "reviewed_by": c.GetUint64("user_id"), "reviewed_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&FeedbackSample{}).Where("batch_id = ?", batch.ID).Updates(map[string]any{"status": "REJECTED", "reviewed_by": c.GetUint64("user_id"), "reviewed_at": now}).Error; err != nil {
				return err
			}
			return appendAudit(tx, c, project.ID, "FEEDBACK_BATCH_REJECTED", "FEEDBACK_BATCH", batch.ID, nil, req)
		})
		httpx.OK(c, true)
		return
	}
	var samples []FeedbackSample
	h.db.Where("batch_id = ? AND status = ?", batch.ID, "BATCHED").Order("id").Find(&samples)
	if len(samples) == 0 {
		httpx.Fail(c, 409, 409, "批次没有有效样本")
		return
	}
	collection := AssetCollection{
		TenantID: project.TenantID, ProjectID: project.ID, Name: "反馈回流-" + batch.Name,
		Description: "来源 FeedbackBatch #" + strconv.FormatUint(batch.ID, 10), Filter: jsonValue(gin.H{"feedbackBatchId": batch.ID}),
		Frozen: true, Version: 1, CreatedBy: c.GetUint64("user_id"),
	}
	task := AnnotationTask{
		TenantID: project.TenantID, ProjectID: project.ID, Name: "反馈返标-" + batch.Name,
		TaskType: "DETECTION", OntologyVersion: "feedback-v1",
		Labels:       jsonValue([]gin.H{{"name": "defect", "color": "#ef4444"}}),
		AnnotatorIDs: jsonValue([]uint64{project.OwnerUserID}),
		ReviewerIDs:  jsonValue([]uint64{c.GetUint64("user_id")}),
		Status:       AnnotationPreparing, CreatedBy: c.GetUint64("user_id"),
	}
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "ANNOTATION_PREPARE",
		ResourceType: "ANNOTATION_TASK", Status: JobQueued, Stage: "PREPARING",
		TraceID: uuid.NewString(), Idempotency: "feedback-annotation:" + strconv.FormatUint(batch.ID, 10), MaxRetries: 3, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&collection).Error; err != nil {
			return err
		}
		seen := map[uint64]bool{}
		for _, sample := range samples {
			if seen[sample.AssetID] {
				continue
			}
			seen[sample.AssetID] = true
			if err := tx.Create(&AssetCollectionItem{TenantID: project.TenantID, ProjectID: project.ID, CollectionID: collection.ID, AssetID: sample.AssetID}).Error; err != nil {
				return err
			}
			if err := tx.Model(&Asset{}).Where("id = ?", sample.AssetID).UpdateColumn("reference_count", gorm.Expr("reference_count + 1")).Error; err != nil {
				return err
			}
		}
		task.CollectionID = collection.ID
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		job.ResourceID = task.ID
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "annotation.task.prepare.v1",
			AggregateType: "ANNOTATION_TASK", AggregateID: task.ID, Payload: jsonValue(gin.H{"annotationTaskId": task.ID}), Status: outboxNew,
		}).Error; err != nil {
			return err
		}
		batch.Status, batch.AnnotationTaskID, batch.ReviewedBy, batch.ReviewedAt = "ANNOTATING", task.ID, c.GetUint64("user_id"), &now
		if err := tx.Save(&batch).Error; err != nil {
			return err
		}
		if err := tx.Model(&FeedbackSample{}).Where("batch_id = ?", batch.ID).Updates(map[string]any{"status": "ACCEPTED", "reviewed_by": c.GetUint64("user_id"), "reviewed_at": now}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "FEEDBACK_BATCH_ACCEPTED", "FEEDBACK_BATCH", batch.ID, nil, gin.H{"annotationTaskId": task.ID, "sourceRevisionIdsPreserved": true})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "反馈批次接受失败")
		return
	}
	httpx.OK(c, gin.H{"batch": batch, "annotationTask": task, "job": job})
}

func (h *Handler) FeedbackCleanup(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	result := h.db.Model(&FeedbackSample{}).Where("tenant_id = ? AND project_id = ? AND expires_at < ? AND status IN ?", project.TenantID, project.ID, time.Now(), []string{"PENDING", "DUPLICATE"}).Update("status", "EXPIRED")
	_ = appendAudit(h.db, c, project.ID, "FEEDBACK_RETENTION_APPLIED", "PROJECT", project.ID, nil, gin.H{"expired": result.RowsAffected})
	httpx.OK(c, gin.H{"expired": result.RowsAffected})
}
