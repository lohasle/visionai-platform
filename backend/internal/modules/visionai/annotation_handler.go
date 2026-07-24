package visionai

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/annotation"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

var annotationTransitions = map[AnnotationStatus]map[AnnotationStatus]bool{
	AnnotationDraft:         {AnnotationPreparing: true, AnnotationCancelled: true},
	AnnotationPreparing:     {AnnotationPreannotating: true, AnnotationReady: true, AnnotationFailed: true},
	AnnotationPreannotating: {AnnotationReady: true, AnnotationFailed: true},
	AnnotationReady:         {AnnotationAnnotating: true, AnnotationCancelled: true},
	AnnotationAnnotating:    {AnnotationReviewing: true, AnnotationCancelled: true, AnnotationFailed: true},
	AnnotationReviewing:     {AnnotationApproved: true, AnnotationRejected: true, AnnotationFailed: true},
	AnnotationRejected:      {AnnotationAnnotating: true, AnnotationCancelled: true},
	AnnotationApproved:      {AnnotationExporting: true},
	AnnotationExporting:     {AnnotationClosed: true, AnnotationFailed: true},
	AnnotationFailed:        {AnnotationPreparing: true, AnnotationExporting: true, AnnotationCancelled: true},
}

func transitionAnnotation(task *AnnotationTask, target AnnotationStatus) bool {
	if task.Status == target {
		return true
	}
	if !annotationTransitions[task.Status][target] {
		return false
	}
	task.Status = target
	return true
}

type annotationTaskRequest struct {
	Name            string             `json:"name"`
	TaskType        string             `json:"taskType"`
	CollectionID    uint64             `json:"collectionId"`
	OntologyVersion string             `json:"ontologyVersion"`
	Labels          []annotation.Label `json:"labels"`
	AnnotatorIDs    []uint64           `json:"annotatorIds"`
	ReviewerIDs     []uint64           `json:"reviewerIds"`
	PlanStartAt     *time.Time         `json:"planStartAt"`
	PlanEndAt       *time.Time         `json:"planEndAt"`
}

type annotationDecisionRequest struct {
	Decision string `json:"decision"`
	Code     string `json:"code"`
	Reason   string `json:"reason"`
}

func (h *Handler) getAnnotationTask(c *gin.Context, project Project) (AnnotationTask, bool) {
	id, _ := strconv.ParseUint(c.Param("taskId"), 10, 64)
	var task AnnotationTask
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&task).Error != nil {
		httpx.Fail(c, 404, 404, "标注任务不存在")
		return task, false
	}
	return task, true
}

func jsonValue(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func (h *Handler) requireCVATMappings(tenantID uint64, userIDs []uint64) bool {
	if len(userIDs) == 0 {
		return false
	}
	unique := make([]uint64, 0, len(userIDs))
	seen := make(map[uint64]bool, len(userIDs))
	for _, userID := range userIDs {
		if userID != 0 && !seen[userID] {
			seen[userID] = true
			unique = append(unique, userID)
		}
	}
	if len(unique) == 0 {
		return false
	}
	var count int64
	h.db.Model(&CVATUserMapping{}).Where("tenant_id = ? AND platform_user_id IN ? AND active = ?", tenantID, unique, true).Count(&count)
	return count == int64(len(unique))
}

// AnnotationTaskPage godoc
// @Summary Page annotation tasks
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks [get]
func (h *Handler) AnnotationTaskPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	pageNo, pageSize := page(c)
	query := h.db.Model(&AnnotationTask{}).Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	query.Count(&total)
	var rows []AnnotationTask
	query.Order("id DESC").Offset((pageNo - 1) * pageSize).Limit(pageSize).Find(&rows)
	httpx.OK(c, gin.H{"list": rows, "total": total})
}

// AnnotationTaskCreate godoc
// @Summary Create a governed annotation task
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks [post]
func (h *Handler) AnnotationTaskCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	var req annotationTaskRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" || req.CollectionID == 0 ||
		len(req.Labels) == 0 || len(req.AnnotatorIDs) == 0 || len(req.ReviewerIDs) == 0 {
		httpx.Fail(c, 400, 400, "名称、资产集合、类别、标注员和审核员必填")
		return
	}
	var collection AssetCollection
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND frozen = ?", project.TenantID, project.ID, req.CollectionID, true).First(&collection).Error != nil {
		httpx.Fail(c, 409, 409, "标注任务只能引用已冻结资产集合")
		return
	}
	allUsers := append(append([]uint64{}, req.AnnotatorIDs...), req.ReviewerIDs...)
	if !h.requireCVATMappings(project.TenantID, allUsers) {
		httpx.Fail(c, 409, 409, "存在未映射或停用的 CVAT 用户，请先维护人员映射")
		return
	}
	taskType := strings.ToUpper(strings.TrimSpace(req.TaskType))
	if taskType == "" {
		taskType = "CV_DETECTION"
	}
	row := AnnotationTask{
		TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name),
		TaskType: taskType, CollectionID: req.CollectionID, OntologyVersion: strings.TrimSpace(req.OntologyVersion),
		Labels: jsonValue(req.Labels), AnnotatorIDs: jsonValue(req.AnnotatorIDs), ReviewerIDs: jsonValue(req.ReviewerIDs),
		Status: AnnotationDraft, PlanStartAt: req.PlanStartAt, PlanEndAt: req.PlanEndAt, CreatedBy: c.GetUint64("user_id"),
	}
	if row.OntologyVersion == "" {
		row.OntologyVersion = "v1"
	}
	if err := h.db.Create(&row).Error; err != nil {
		httpx.Fail(c, 500, 500, "标注任务创建失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "ANNOTATION_TASK_CREATED", "ANNOTATION_TASK", row.ID, nil, row)
	httpx.OK(c, row)
}

// AnnotationTaskGet godoc
// @Summary Get annotation task details
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks/{taskId} [get]
func (h *Handler) AnnotationTaskGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	task, ok := h.getAnnotationTask(c, project)
	if !ok {
		return
	}
	var binding ExternalResourceBinding
	h.db.Where("tenant_id = ? AND internal_type = ? AND internal_id = ?", project.TenantID, "ANNOTATION_TASK", task.ID).First(&binding)
	var revisions []AnnotationRevision
	h.db.Where("tenant_id = ? AND annotation_task_id = ?", project.TenantID, task.ID).Order("revision_no DESC").Find(&revisions)
	var prelabels []PreannotationRun
	h.db.Where("tenant_id = ? AND annotation_task_id = ?", project.TenantID, task.ID).Order("id DESC").Find(&prelabels)
	httpx.OK(c, gin.H{"task": task, "binding": binding, "revisions": revisions, "preannotationRuns": prelabels})
}

func (h *Handler) enqueueAnnotationEvent(c *gin.Context, project Project, task *AnnotationTask, eventType string, target AnnotationStatus) bool {
	before := *task
	if !transitionAnnotation(task, target) {
		httpx.Fail(c, 409, 409, "当前状态不允许执行该操作")
		return false
	}
	payload := jsonValue(gin.H{"annotationTaskId": task.ID})
	idempotency := eventType + ":" + strconv.FormatUint(task.ID, 10)
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "ANNOTATION",
		ResourceType: "ANNOTATION_TASK", ResourceID: task.ID, Status: JobQueued, Stage: string(target),
		TraceID: c.GetString("trace_id"), Idempotency: idempotency, MaxRetries: 5, CreatedBy: c.GetUint64("user_id"),
	}
	if job.TraceID == "" {
		job.TraceID = uuid.NewString()
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(task).Error; err != nil {
			return err
		}
		if err := tx.Where("idempotency = ?", idempotency).FirstOrCreate(&job).Error; err != nil {
			return err
		}
		return tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: eventType,
			AggregateType: "ANNOTATION_TASK", AggregateID: task.ID, Payload: payload, Status: outboxNew,
		}).Error
	})
	if err != nil {
		*task = before
		httpx.Fail(c, 500, 500, "标注编排任务创建失败")
		return false
	}
	_ = appendAudit(h.db, c, project.ID, strings.ToUpper(strings.ReplaceAll(eventType, ".", "_")), "ANNOTATION_TASK", task.ID, before, task)
	httpx.OK(c, gin.H{"task": task, "job": job})
	return true
}

// AnnotationPrepare godoc
// @Summary Create CVAT task and upload governed media
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks/{taskId}/prepare [post]
func (h *Handler) AnnotationPrepare(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	task, ok := h.getAnnotationTask(c, project)
	if ok {
		h.enqueueAnnotationEvent(c, project, &task, "annotation.task.prepare.v1", AnnotationPreparing)
	}
}

// AnnotationSync godoc
// @Summary Synchronize external CVAT status with rate limiting
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks/{taskId}/sync [post]
func (h *Handler) AnnotationSync(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	task, ok := h.getAnnotationTask(c, project)
	if !ok {
		return
	}
	var binding ExternalResourceBinding
	if h.db.Where("tenant_id = ? AND internal_type = ? AND internal_id = ?", project.TenantID, "ANNOTATION_TASK", task.ID).First(&binding).Error != nil {
		httpx.Fail(c, 409, 409, "CVAT Binding 尚未创建")
		return
	}
	if binding.LastSyncAt != nil && time.Since(*binding.LastSyncAt) < 10*time.Second {
		httpx.Fail(c, 429, 429, "刷新过于频繁，请稍后再试")
		return
	}
	provider, err := annotation.NewCVAT(config.Load().CVATBaseURL, config.Load().CVATPublicURL, config.Load().CVATUsername, config.Load().CVATPassword, config.Load().CVATTimeout)
	if err != nil {
		httpx.Fail(c, 503, 503, "CVAT Provider 配置无效")
		return
	}
	externalID, _ := strconv.ParseInt(binding.ExternalID, 10, 64)
	external, err := provider.GetTask(c.Request.Context(), externalID)
	if err != nil {
		httpx.Fail(c, 503, 503, "CVAT 状态同步失败")
		return
	}
	now := time.Now()
	binding.LastSyncAt, binding.SyncCursor = &now, external.Status+":"+strconv.Itoa(external.Progress)
	task.Progress = external.Progress
	if task.Status == AnnotationPreparing && strings.EqualFold(external.Status, "annotation") {
		task.Status = AnnotationReady
	}
	h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&binding).Error; err != nil {
			return err
		}
		return tx.Save(&task).Error
	})
	httpx.OK(c, gin.H{"task": task, "external": external})
}

type annotationStatusRequest struct {
	Status AnnotationStatus `json:"status"`
}

// AnnotationStatusUpdate godoc
// @Summary Advance an annotation task through work and review
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks/{taskId}/status [put]
func (h *Handler) AnnotationStatusUpdate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok {
		return
	}
	task, ok := h.getAnnotationTask(c, project)
	if !ok {
		return
	}
	var req annotationStatusRequest
	if c.ShouldBindJSON(&req) != nil || !transitionAnnotation(&task, req.Status) {
		httpx.Fail(c, 409, 409, "标注任务状态迁移无效")
		return
	}
	if req.Status == AnnotationAnnotating && !h.requireProjectRole(c, project, "ANNOTATOR", "DATA_MANAGER") {
		return
	}
	if req.Status == AnnotationReviewing && !h.requireProjectRole(c, project, "ANNOTATOR", "REVIEWER") {
		return
	}
	if err := h.db.Save(&task).Error; err != nil {
		httpx.Fail(c, 500, 500, "状态保存失败")
		return
	}
	httpx.OK(c, task)
}

// AnnotationReview godoc
// @Summary Record approval or structured rejection
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks/{taskId}/review [post]
func (h *Handler) AnnotationReview(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "REVIEWER") {
		return
	}
	task, ok := h.getAnnotationTask(c, project)
	if !ok {
		return
	}
	var req annotationDecisionRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "审核请求格式错误")
		return
	}
	target := AnnotationApproved
	if strings.EqualFold(req.Decision, "REJECT") {
		target = AnnotationRejected
		if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Reason) == "" {
			httpx.Fail(c, 400, 400, "驳回必须提供结构化原因代码和说明")
			return
		}
	}
	before := task
	if !transitionAnnotation(&task, target) {
		httpx.Fail(c, 409, 409, "当前状态不可审核")
		return
	}
	task.RejectionCode, task.RejectionReason = strings.TrimSpace(req.Code), strings.TrimSpace(req.Reason)
	if target == AnnotationApproved {
		task.RejectionCode, task.RejectionReason = "", ""
	}
	if err := h.db.Save(&task).Error; err != nil {
		httpx.Fail(c, 500, 500, "审核结果保存失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "ANNOTATION_"+string(target), "ANNOTATION_TASK", task.ID, before, task)
	httpx.OK(c, task)
}

// AnnotationExport godoc
// @Summary Export an approved immutable annotation revision
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks/{taskId}/export [post]
func (h *Handler) AnnotationExport(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "REVIEWER", "DATA_MANAGER") {
		return
	}
	task, ok := h.getAnnotationTask(c, project)
	if ok {
		h.enqueueAnnotationEvent(c, project, &task, "annotation.export.requested.v1", AnnotationExporting)
	}
}

// AnnotationWorkbench godoc
// @Summary Authorize and audit opening the CVAT workbench
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks/{taskId}/workbench [post]
func (h *Handler) AnnotationWorkbench(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok || !h.requireProjectRole(c, project, "ANNOTATOR", "REVIEWER", "DATA_MANAGER") {
		return
	}
	task, ok := h.getAnnotationTask(c, project)
	if !ok {
		return
	}
	var binding ExternalResourceBinding
	if h.db.Where("tenant_id = ? AND internal_type = ? AND internal_id = ?", project.TenantID, "ANNOTATION_TASK", task.ID).First(&binding).Error != nil {
		httpx.Fail(c, 409, 409, "CVAT Binding 尚未创建")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "CVAT_WORKBENCH_OPENED", "ANNOTATION_TASK", task.ID, nil, gin.H{"externalId": binding.ExternalID})
	httpx.OK(c, gin.H{"url": binding.ExternalURL})
}

// CVATUserMappingList godoc
// @Summary List CVAT user mappings
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/cvat-user-mappings [get]
func (h *Handler) CVATUserMappingList(c *gin.Context) {
	var rows []CVATUserMapping
	h.db.Where("tenant_id = ?", tenantID(c)).Order("platform_user_id").Find(&rows)
	httpx.OK(c, rows)
}

// CVATUserMappingUpsert godoc
// @Summary Upsert and verify a CVAT user mapping
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/cvat-user-mappings [put]
func (h *Handler) CVATUserMappingUpsert(c *gin.Context) {
	var req CVATUserMapping
	if c.ShouldBindJSON(&req) != nil || req.PlatformUserID == 0 || req.CVATUserID == 0 || strings.TrimSpace(req.CVATUsername) == "" {
		httpx.Fail(c, 400, 400, "平台用户、CVAT 用户 ID 与用户名必填")
		return
	}
	now := time.Now()
	req.TenantID, req.Active, req.VerifiedAt = tenantID(c), true, &now
	var row CVATUserMapping
	err := h.db.Where(CVATUserMapping{TenantID: req.TenantID, PlatformUserID: req.PlatformUserID}).
		Assign(map[string]any{"cvat_user_id": req.CVATUserID, "cvat_username": strings.TrimSpace(req.CVATUsername), "active": true, "verified_at": now}).
		FirstOrCreate(&row).Error
	if err != nil {
		httpx.Fail(c, 500, 500, "CVAT 用户映射保存失败")
		return
	}
	httpx.OK(c, row)
}

type preannotationRequest struct {
	ModelVersionID uint64         `json:"modelVersionId"`
	Parameters     map[string]any `json:"parameters"`
}

// PreannotationCreate godoc
// @Summary Create an idempotent pre-annotation run
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks/{taskId}/preannotations [post]
func (h *Handler) PreannotationCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER", "ALGORITHM_ENGINEER") {
		return
	}
	task, ok := h.getAnnotationTask(c, project)
	if !ok || (task.Status != AnnotationPreparing && task.Status != AnnotationReady) {
		if ok {
			httpx.Fail(c, 409, 409, "当前任务状态不可执行预标注")
		}
		return
	}
	var req preannotationRequest
	if c.ShouldBindJSON(&req) != nil || req.ModelVersionID == 0 {
		httpx.Fail(c, 400, 400, "必须选择模型版本")
		return
	}
	var approved int64
	h.db.Table("ai_model_version").Where("tenant_id = ? AND id = ? AND preannotation_approved = ?", project.TenantID, req.ModelVersionID, true).Count(&approved)
	if approved != 1 {
		httpx.Fail(c, 409, 409, "仅允许选择已批准用于预标注的模型版本")
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		key = uuid.NewString()
	}
	run := PreannotationRun{
		TenantID: project.TenantID, ProjectID: project.ID, AnnotationTaskID: task.ID,
		ModelVersionID: req.ModelVersionID, Status: "QUEUED", Parameters: jsonValue(req.Parameters), IdempotencyKey: key,
	}
	if err := h.db.Create(&run).Error; err != nil {
		var existing PreannotationRun
		if h.db.Where("idempotency_key = ?", key).First(&existing).Error == nil {
			httpx.OK(c, gin.H{"run": existing, "idempotentReplay": true})
			return
		}
		httpx.Fail(c, 409, 409, "预标注任务创建失败")
		return
	}
	task.PreannotationRunID, task.Status = run.ID, AnnotationPreannotating
	h.db.Save(&task)
	httpx.OK(c, gin.H{"run": run, "task": task})
}

type preannotationMetricsRequest struct {
	ProposedCount     int64 `json:"proposedCount"`
	AcceptedCount     int64 `json:"acceptedCount"`
	DeletedCount      int64 `json:"deletedCount"`
	ModifiedCount     int64 `json:"modifiedCount"`
	AddedCount        int64 `json:"addedCount"`
	CorrectionSeconds int64 `json:"correctionSeconds"`
}

// PreannotationMetricsUpdate godoc
// @Summary Record pre-annotation efficiency metrics
// @Tags VisionAI Annotation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/annotation-tasks/{taskId}/preannotations/{runId}/metrics [put]
func (h *Handler) PreannotationMetricsUpdate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER", "REVIEWER") {
		return
	}
	task, ok := h.getAnnotationTask(c, project)
	if !ok {
		return
	}
	runID, _ := strconv.ParseUint(c.Param("runId"), 10, 64)
	var run PreannotationRun
	if h.db.Where("tenant_id = ? AND annotation_task_id = ? AND id = ?", project.TenantID, task.ID, runID).First(&run).Error != nil {
		httpx.Fail(c, 404, 404, "预标注任务不存在")
		return
	}
	var req preannotationMetricsRequest
	if c.ShouldBindJSON(&req) != nil || req.ProposedCount < 0 || req.AcceptedCount < 0 || req.DeletedCount < 0 || req.ModifiedCount < 0 || req.AddedCount < 0 {
		httpx.Fail(c, 400, 400, "效率指标无效")
		return
	}
	run.ProposedCount, run.AcceptedCount, run.DeletedCount = req.ProposedCount, req.AcceptedCount, req.DeletedCount
	run.ModifiedCount, run.AddedCount, run.CorrectionSeconds, run.Status = req.ModifiedCount, req.AddedCount, req.CorrectionSeconds, "SUCCEEDED"
	h.db.Save(&run)
	if task.Status == AnnotationPreannotating {
		task.Status = AnnotationReady
		h.db.Save(&task)
	}
	denominator := float64(run.ProposedCount)
	rate := func(value int64) float64 {
		if denominator == 0 {
			return 0
		}
		return float64(value) / denominator
	}
	httpx.OK(c, gin.H{"run": run, "rates": gin.H{
		"accepted": rate(run.AcceptedCount), "deleted": rate(run.DeletedCount),
		"modified": rate(run.ModifiedCount), "added": rate(run.AddedCount),
	}})
}
