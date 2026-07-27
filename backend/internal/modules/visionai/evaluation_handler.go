package visionai

import (
	"net/url"
	"strconv"
	"strings"

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
}

type evaluationRunRequest struct {
	TrainingRunID uint64 `json:"trainingRunId"`
	BaselineRunID uint64 `json:"baselineRunId"`
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
		req.Slices = []string{"all", "small-object", "occluded", "dense", "night", "low-confidence"}
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
		GatePolicy: policy, EvaluatorVersion: "visionai-evaluator/1.0.0", CreatedBy: c.GetUint64("user_id"),
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
	query.Order("io_u, confidence").Limit(500).Find(&samples)
	h.db.Where("tenant_id = ? AND evaluation_run_id = ?", project.TenantID, run.ID).Order("id").Find(&slices)
	httpx.OK(c, gin.H{"run": run, "metrics": metrics, "samples": samples, "savedSlices": slices})
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
