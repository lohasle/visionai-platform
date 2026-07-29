package visionai

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

type evaluationSuiteRequest struct {
	Name             string             `json:"name"`
	DatasetVersionID uint64             `json:"datasetVersionId"`
	Slices           []string           `json:"slices"`
	Thresholds       map[string]float64 `json:"thresholds"`
	GatePolicy       string             `json:"gatePolicy"`
	AutoTrigger      bool               `json:"autoTrigger"`
}

type evaluationRunRequest struct {
	TrainingRunID uint64 `json:"trainingRunId"`
	BaselineRunID uint64 `json:"baselineRunId"`
}

type evaluationSavedSliceRequest struct {
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	TargetSize    string   `json:"targetSize"`
	Scene         string   `json:"scene"`
	Device        string   `json:"device"`
	TimeFrom      string   `json:"timeFrom"`
	TimeTo        string   `json:"timeTo"`
	ConfidenceMin *float64 `json:"confidenceMin"`
	ConfidenceMax *float64 `json:"confidenceMax"`
}

func (h *Handler) EvaluationSuitePage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var rows []EvaluationSuite
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Order("id DESC").Find(&rows)
	httpx.OK(c, rows)
}

func (h *Handler) EvaluationSuiteCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "PROJECT_OWNER") {
		return
	}
	var req evaluationSuiteRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" || req.DatasetVersionID == 0 {
		httpx.Fail(c, 400, 400, "名称和冻结 DatasetVersion 必填")
		return
	}
	var dataset DatasetVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, req.DatasetVersionID, DatasetVersionFrozen).First(&dataset).Error != nil {
		httpx.Fail(c, 409, 409, "评估套件只能引用 FROZEN DatasetVersion")
		return
	}
	if len(req.Slices) == 0 {
		req.Slices = []string{"all", "category", "target-size", "scene", "device", "time", "confidence"}
	}
	if len(req.Thresholds) == 0 {
		req.Thresholds = map[string]float64{"mAP": 0.5, "precision": 0.5, "recall": 0.5}
	}
	policy := strings.ToUpper(strings.TrimSpace(req.GatePolicy))
	if policy != "MUST_PASS" && policy != "ALLOW_REGRESSION" && policy != "MANUAL_REVIEW" {
		policy = "MUST_PASS"
	}
	row := EvaluationSuite{
		TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name),
		DatasetVersionID: dataset.ID, Slices: jsonValue(req.Slices), Thresholds: jsonValue(req.Thresholds),
		GatePolicy: policy, AutoTrigger: req.AutoTrigger,
		EvaluatorVersion: "visionai-evaluator/1.0.0", CreatedBy: c.GetUint64("user_id"),
	}
	if h.db.Create(&row).Error != nil {
		httpx.Fail(c, 500, 500, "评估套件创建失败")
		return
	}
	httpx.OK(c, row)
}

func (h *Handler) EvaluationRunPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var rows []EvaluationRun
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Order("id DESC").Find(&rows)
	httpx.OK(c, rows)
}

func (h *Handler) EvaluationRunCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER") {
		return
	}
	suiteID, _ := strconv.ParseUint(c.Param("suiteId"), 10, 64)
	var suite EvaluationSuite
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, suiteID).First(&suite).Error != nil {
		httpx.Fail(c, 404, 404, "评估套件不存在")
		return
	}
	var req evaluationRunRequest
	if c.ShouldBindJSON(&req) != nil || req.TrainingRunID == 0 {
		httpx.Fail(c, 400, 400, "成功 TrainingRun 必填")
		return
	}
	var training TrainingRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ? AND dataset_version_id = ?", project.TenantID, project.ID, req.TrainingRunID, TrainingSucceeded, suite.DatasetVersionID).First(&training).Error != nil {
		httpx.Fail(c, 409, 409, "训练未成功或数据集与 Suite 不一致")
		return
	}
	if req.BaselineRunID != 0 {
		var baseline EvaluationRun
		if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ? AND suite_id = ?", project.TenantID, project.ID, req.BaselineRunID, "SUCCEEDED", suite.ID).First(&baseline).Error != nil {
			httpx.Fail(c, 409, 409, "基线必须是同一评估套件下成功完成的 EvaluationRun")
			return
		}
	}
	run := EvaluationRun{
		TenantID: project.TenantID, ProjectID: project.ID, SuiteID: suite.ID,
		TrainingRunID: training.ID, BaselineRunID: req.BaselineRunID, Status: "QUEUED",
		GateDecision: "PENDING", GateEvidence: "{}", Summary: "{}", CreatedBy: c.GetUint64("user_id"),
	}
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "EVALUATION",
		ResourceType: "EVALUATION_RUN", Status: JobQueued, Stage: "QUEUED",
		TraceID: uuid.NewString(), Idempotency: "evaluation:" + uuid.NewString(), MaxRetries: 2, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		job.ResourceID = run.ID
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := tx.Create(&DatasetUsage{
			TenantID: project.TenantID, ProjectID: project.ID, DatasetVersionID: suite.DatasetVersionID,
			ResourceType: "EVALUATION_RUN", ResourceID: run.ID,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "evaluation.run.requested.v1",
			AggregateType: "EVALUATION_RUN", AggregateID: run.ID,
			Payload: jsonValue(gin.H{"evaluationRunId": run.ID}), Status: outboxNew,
		}).Error
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "评估任务创建失败")
		return
	}
	httpx.OK(c, gin.H{"run": run, "job": job})
}

func (h *Handler) EvaluationRunGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	runID, _ := strconv.ParseUint(c.Param("runId"), 10, 64)
	var run EvaluationRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, runID).First(&run).Error != nil {
		httpx.Fail(c, 404, 404, "评估运行不存在")
		return
	}
	var metrics []EvaluationMetric
	var samples []EvaluationSample
	var slices []EvaluationSavedSlice
	h.db.Where("tenant_id = ? AND evaluation_run_id = ?", project.TenantID, run.ID).Order("slice,name").Find(&metrics)
	query := h.db.Model(&EvaluationSample{}).Where("tenant_id = ? AND evaluation_run_id = ?", project.TenantID, run.ID)
	if errorType := strings.TrimSpace(c.Query("errorType")); errorType != "" {
		query = query.Where("error_type = ?", strings.ToUpper(errorType))
	}
	var activeSlice EvaluationSavedSlice
	if sliceID, _ := strconv.ParseUint(c.Query("savedSliceId"), 10, 64); sliceID > 0 {
		if h.db.Where("tenant_id = ? AND evaluation_run_id = ? AND id = ?", project.TenantID, run.ID, sliceID).First(&activeSlice).Error != nil {
			httpx.Fail(c, 404, 404, "保存切片不存在")
			return
		}
		var filter map[string]any
		if json.Unmarshal([]byte(activeSlice.Filter), &filter) != nil {
			httpx.Fail(c, 500, 500, "保存切片条件损坏")
			return
		}
		query = applyEvaluationSliceFilter(query, filter)
	}
	query.Order("io_u, confidence").Limit(500).Find(&samples)
	h.db.Where("tenant_id = ? AND evaluation_run_id = ?", project.TenantID, run.ID).Order("id").Find(&slices)
	httpx.OK(c, gin.H{"run": run, "metrics": metrics, "samples": samples, "savedSlices": slices, "activeSlice": activeSlice})
}

func applyEvaluationSliceFilter(query *gorm.DB, filter map[string]any) *gorm.DB {
	if value := strings.TrimSpace(fmt.Sprint(filter["errorType"])); value != "" && value != "<nil>" {
		query = query.Where("error_type = ?", strings.ToUpper(value))
	}
	if value := strings.TrimSpace(fmt.Sprint(filter["category"])); value != "" && value != "<nil>" {
		encoded, _ := json.Marshal(value)
		query = query.Where("JSON_CONTAINS(category_labels, ?)", string(encoded))
	}
	for key, column := range map[string]string{"targetSize": "target_size", "scene": "scene", "device": "device"} {
		if value := strings.TrimSpace(fmt.Sprint(filter[key])); value != "" && value != "<nil>" {
			query = query.Where(column+" = ?", value)
		}
	}
	if value := strings.TrimSpace(fmt.Sprint(filter["capturedMonth"])); value != "" && value != "<nil>" {
		if from, err := time.Parse("2006-01", value); err == nil {
			query = query.Where("captured_at >= ? AND captured_at < ?", from, from.AddDate(0, 1, 0))
		}
	}
	for key, operator := range map[string]string{
		"confidenceLt": "<", "confidenceMin": ">=", "confidenceMax": "<=",
	} {
		if value, exists := filter[key]; exists {
			query = query.Where("confidence "+operator+" ?", numberFromAny(value))
		}
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(filter["confidenceBucket"]))) {
	case "low":
		query = query.Where("confidence < ?", 0.5)
	case "medium":
		query = query.Where("confidence >= ? AND confidence < ?", 0.5, 0.8)
	case "high":
		query = query.Where("confidence >= ?", 0.8)
	}
	if value, exists := filter["timeFrom"]; exists {
		query = query.Where("captured_at >= ?", value)
	}
	if value, exists := filter["timeTo"]; exists {
		query = query.Where("captured_at <= ?", value)
	}
	return query
}

// EvaluationSavedSliceCreate godoc
// @Summary Save a governed evaluation slice by category, size, scene, device, time and confidence
// @Tags VisionAI Evaluation
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/evaluation-runs/{runId}/slices [post]
func (h *Handler) EvaluationSavedSliceCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "REVIEWER", "PROJECT_OWNER") {
		return
	}
	runID, _ := strconv.ParseUint(c.Param("runId"), 10, 64)
	var run EvaluationRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, runID, "SUCCEEDED").First(&run).Error != nil {
		httpx.Fail(c, 409, 409, "仅成功评估运行可保存切片")
		return
	}
	var req evaluationSavedSliceRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" || len(strings.TrimSpace(req.Name)) > 160 {
		httpx.Fail(c, 400, 400, "切片名称无效")
		return
	}
	query := h.db.Model(&EvaluationSample{}).Where("tenant_id = ? AND evaluation_run_id = ?", project.TenantID, run.ID)
	filter := map[string]any{}
	if value := strings.TrimSpace(req.Category); value != "" {
		encoded, _ := json.Marshal(value)
		query = query.Where("JSON_CONTAINS(category_labels, ?)", string(encoded))
		filter["category"] = value
	}
	if value := strings.ToLower(strings.TrimSpace(req.TargetSize)); value != "" {
		if value != "small" && value != "medium" && value != "large" {
			httpx.Fail(c, 400, 400, "目标尺寸必须是 small、medium 或 large")
			return
		}
		query = query.Where("target_size = ?", value)
		filter["targetSize"] = value
	}
	if value := strings.TrimSpace(req.Scene); value != "" {
		query = query.Where("scene = ?", value)
		filter["scene"] = value
	}
	if value := strings.TrimSpace(req.Device); value != "" {
		query = query.Where("device = ?", value)
		filter["device"] = value
	}
	if value := strings.TrimSpace(req.TimeFrom); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			httpx.Fail(c, 400, 400, "timeFrom 必须是 RFC3339")
			return
		}
		query = query.Where("captured_at >= ?", parsed)
		filter["timeFrom"] = parsed
	}
	if value := strings.TrimSpace(req.TimeTo); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			httpx.Fail(c, 400, 400, "timeTo 必须是 RFC3339")
			return
		}
		query = query.Where("captured_at <= ?", parsed)
		filter["timeTo"] = parsed
	}
	if req.ConfidenceMin != nil {
		if *req.ConfidenceMin < 0 || *req.ConfidenceMin > 1 {
			httpx.Fail(c, 400, 400, "最低置信度无效")
			return
		}
		query = query.Where("confidence >= ?", *req.ConfidenceMin)
		filter["confidenceMin"] = *req.ConfidenceMin
	}
	if req.ConfidenceMax != nil {
		if *req.ConfidenceMax < 0 || *req.ConfidenceMax > 1 {
			httpx.Fail(c, 400, 400, "最高置信度无效")
			return
		}
		query = query.Where("confidence <= ?", *req.ConfidenceMax)
		filter["confidenceMax"] = *req.ConfidenceMax
	}
	if len(filter) == 0 {
		httpx.Fail(c, 400, 400, "至少设置一个切片条件")
		return
	}
	var count int64
	if query.Count(&count).Error != nil {
		httpx.Fail(c, 500, 500, "切片样本统计失败")
		return
	}
	row := EvaluationSavedSlice{
		TenantID: project.TenantID, ProjectID: project.ID, EvaluationRunID: run.ID,
		Name: strings.TrimSpace(req.Name), Filter: jsonValue(filter), SampleCount: int(count),
		CreatedBy: c.GetUint64("user_id"),
	}
	if h.db.Create(&row).Error != nil {
		httpx.Fail(c, 500, 500, "保存切片失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "EVALUATION_SLICE_SAVED", "EVALUATION_SAVED_SLICE", row.ID, nil, row)
	httpx.OK(c, row)
}

func (h *Handler) EvaluationWorkbench(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "REVIEWER") {
		return
	}
	runID, _ := strconv.ParseUint(c.Param("runId"), 10, 64)
	var run EvaluationRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, runID, "SUCCEEDED").First(&run).Error != nil {
		httpx.Fail(c, 409, 409, "评估尚未完成")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "FIFTYONE_WORKBENCH_OPENED", "EVALUATION_RUN", run.ID, nil, gin.H{"dataset": run.FiftyOneDataset})
	target := configuredWorkbenchBase("FIFTYONE") + "/?dataset=" + url.QueryEscape(run.FiftyOneDataset)
	launchURL, err := h.createWorkbenchLaunch(c, "FIFTYONE", project, c.GetUint64("user_id"), "EVALUATION_RUN", run.ID, target)
	if err != nil {
		workbenchLaunchError(c, "FiftyOne 工作台授权失败："+err.Error())
		return
	}
	httpx.OK(c, gin.H{
		"url":       launchURL,
		"dataset":   run.FiftyOneDataset,
		"expiresIn": int(workbenchTicketTTL.Seconds()),
	})
}
