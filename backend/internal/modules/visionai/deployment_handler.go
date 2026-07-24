package visionai

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	platforminference "github.com/lohasle/nimbus-framework-go/internal/platform/inference"
	"gorm.io/gorm"
)

type deploymentCreateRequest struct {
	Name                string         `json:"name"`
	Environment         string         `json:"environment"`
	ModelVersionID      uint64         `json:"modelVersionId"`
	Config              map[string]any `json:"config"`
	AllowUnapprovedTest bool           `json:"allowUnapprovedTest"`
}

func (h *Handler) DeploymentPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var deployments []Deployment
	var revisions []DeploymentRevision
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Order("id DESC").Find(&deployments)
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Order("id DESC").Find(&revisions)
	httpx.OK(c, gin.H{"deployments": deployments, "revisions": revisions})
}

func (h *Handler) DeploymentCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	var req deploymentCreateRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" || req.ModelVersionID == 0 {
		httpx.Fail(c, 400, 400, "名称和模型版本必填")
		return
	}
	req.Environment = strings.ToUpper(strings.TrimSpace(req.Environment))
	if req.Environment != "STAGING" && req.Environment != "CANARY" && req.Environment != "PRODUCTION" {
		httpx.Fail(c, 400, 400, "环境必须是 STAGING、CANARY 或 PRODUCTION")
		return
	}
	var version ModelVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, req.ModelVersionID).First(&version).Error != nil {
		httpx.Fail(c, 404, 404, "模型版本不存在")
		return
	}
	if req.Environment == "PRODUCTION" && version.Status != "APPROVED" {
		httpx.Fail(c, 409, 409, "生产部署仅允许 APPROVED 模型版本")
		return
	}
	if req.Environment != "PRODUCTION" && version.Status != "APPROVED" && !req.AllowUnapprovedTest {
		httpx.Fail(c, 409, 409, "测试环境使用未批准模型必须显式 allowUnapprovedTest")
		return
	}
	var artifact ModelArtifact
	if h.db.Where("model_version_id = ?", version.ID).Order("CASE WHEN format IN ('ONNX','ENGINE','PT') THEN 0 ELSE 1 END, id").First(&artifact).Error != nil {
		httpx.Fail(c, 409, 409, "模型制品缺失")
		return
	}
	userID := c.GetUint64("user_id")
	deployment := Deployment{
		TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name),
		Environment: req.Environment, Status: "DRAFT", CreatedBy: userID,
	}
	revision := DeploymentRevision{
		TenantID: project.TenantID, ProjectID: project.ID, RevisionNo: 1, ModelVersionID: version.ID,
		Status: "DRAFT", Config: jsonValue(req.Config), ArtifactURI: artifact.URI, ArtifactSHA256: artifact.SHA256, CreatedBy: userID,
	}
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "DEPLOYMENT",
		ResourceType: "DEPLOYMENT_REVISION", Status: JobQueued, Stage: "QUEUED",
		TraceID: uuid.NewString(), Idempotency: "deployment:" + uuid.NewString(), MaxRetries: 2, CreatedBy: userID,
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&deployment).Error; err != nil {
			return err
		}
		revision.DeploymentID = deployment.ID
		if err := tx.Create(&revision).Error; err != nil {
			return err
		}
		job.ResourceID = revision.ID
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "deployment.revision.requested.v1",
			AggregateType: "DEPLOYMENT_REVISION", AggregateID: revision.ID,
			Payload: jsonValue(gin.H{"revisionId": revision.ID}), Status: outboxNew,
		}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "DEPLOYMENT_CREATED", "DEPLOYMENT", deployment.ID, nil, gin.H{"revisionId": revision.ID, "modelVersionId": version.ID, "environment": req.Environment})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "部署创建失败")
		return
	}
	httpx.OK(c, gin.H{"deployment": deployment, "revision": revision, "job": job})
}

func (h *Handler) DeploymentGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	id, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&deployment).Error != nil {
		httpx.Fail(c, 404, 404, "部署不存在")
		return
	}
	var revisions []DeploymentRevision
	var traces []InferenceTrace
	var rules []AlertRule
	var alerts []AlertEvent
	h.db.Where("deployment_id = ?", deployment.ID).Order("revision_no DESC").Find(&revisions)
	h.db.Where("deployment_id = ?", deployment.ID).Order("id DESC").Limit(100).Find(&traces)
	h.db.Where("deployment_id = ?", deployment.ID).Order("id DESC").Find(&rules)
	h.db.Where("deployment_id = ?", deployment.ID).Order("id DESC").Limit(100).Find(&alerts)
	httpx.OK(c, gin.H{
		"deployment": deployment, "revisions": revisions, "traces": traces,
		"alertRules": rules, "alerts": alerts, "metrics": calculateInferenceMetrics(traces),
		"inferenceStatus": buildInferenceStatus(deployment, revisions, traces),
	})
}

type inferenceRequest struct {
	AssetID uint64 `json:"assetId"`
}

func (h *Handler) DeploymentPredict(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	id, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, id, "RUNNING").First(&deployment).Error != nil {
		httpx.Fail(c, 409, 409, "部署未运行")
		return
	}
	var req inferenceRequest
	if c.ShouldBindJSON(&req) != nil || req.AssetID == 0 {
		httpx.Fail(c, 400, 400, "Asset ID 必填")
		return
	}
	var asset Asset
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, req.AssetID, AssetReady).First(&asset).Error != nil {
		httpx.Fail(c, 404, 404, "推理图片不存在")
		return
	}
	var revision DeploymentRevision
	if h.db.Where("id = ? AND deployment_id = ?", deployment.CurrentRevisionID, deployment.ID).First(&revision).Error != nil {
		httpx.Fail(c, 409, 409, "当前部署修订不存在")
		return
	}
	traceID := uuid.NewString()
	started := time.Now()
	object, _, err := h.storage.Get(c.Request.Context(), asset.ObjectKey)
	if err != nil {
		httpx.Fail(c, 500, 500, "读取推理图片失败")
		return
	}
	defer object.Close()
	result, predictErr := platforminference.NewClient(config.Load().InferenceAPIURL, config.Load().InferenceTimeout).Predict(c.Request.Context(), asset.Filename, object)
	elapsed := float64(time.Since(started).Microseconds()) / 1000
	trace := InferenceTrace{
		TenantID: project.TenantID, ProjectID: project.ID, DeploymentID: deployment.ID,
		DeploymentRevisionID: revision.ID, ModelVersionID: revision.ModelVersionID,
		TraceID: traceID, AssetID: asset.ID, SourceType: "ASSET", SourceName: asset.Filename,
		SourceSHA256: asset.SHA256, TestMode: "ONLINE", RegressionStatus: "NOT_ASSERTED",
		Status: "SUCCEEDED", LatencyMS: elapsed,
		Result: "{}", CreatedAt: time.Now(),
	}
	if predictErr != nil {
		trace.Status, trace.ErrorMessage = "FAILED", predictErr.Error()
		h.db.Create(&trace)
		evaluateAlertRules(h.db, trace)
		captureFeedback(h.db, trace)
		httpx.Fail(c, 502, 502, "推理服务失败："+predictErr.Error())
		return
	}
	confidence := 0.0
	for _, detection := range result.Detections {
		confidence += detection.Confidence
	}
	if len(result.Detections) > 0 {
		confidence /= float64(len(result.Detections))
	}
	trace.DetectionCount, trace.MeanConfidence, trace.Result = len(result.Detections), confidence, jsonValue(result)
	h.db.Create(&trace)
	evaluateAlertRules(h.db, trace)
	captureFeedback(h.db, trace)
	httpx.OK(c, gin.H{
		"traceId": traceID, "deploymentId": deployment.ID, "deploymentRevisionId": revision.ID,
		"modelVersionId": revision.ModelVersionID, "result": result, "platformLatencyMs": elapsed,
	})
}

func (h *Handler) DeploymentPredictVideo(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	id, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, id, "RUNNING").First(&deployment).Error != nil {
		httpx.Fail(c, 409, 409, "部署未运行")
		return
	}
	header, err := c.FormFile("file")
	if err != nil || header.Size <= 0 || header.Size > 100<<20 || !strings.HasPrefix(header.Header.Get("Content-Type"), "video/") {
		httpx.Fail(c, 400, 400, "需要不超过 100MB 的视频文件")
		return
	}
	input, err := header.Open()
	if err != nil {
		httpx.Fail(c, 400, 400, "视频文件无法读取")
		return
	}
	defer input.Close()
	var revision DeploymentRevision
	if h.db.Where("id = ? AND deployment_id = ?", deployment.CurrentRevisionID, deployment.ID).First(&revision).Error != nil {
		httpx.Fail(c, 409, 409, "当前部署修订不存在")
		return
	}
	traceID := uuid.NewString()
	started := time.Now()
	result, predictErr := platforminference.NewClient(config.Load().InferenceAPIURL, config.Load().InferenceTimeout).PredictVideo(c.Request.Context(), header.Filename, input)
	elapsed := float64(time.Since(started).Microseconds()) / 1000
	trace := InferenceTrace{
		TenantID: project.TenantID, ProjectID: project.ID, DeploymentID: deployment.ID,
		DeploymentRevisionID: revision.ID, ModelVersionID: revision.ModelVersionID,
		TraceID: traceID, SourceType: "VIDEO_UPLOAD", SourceName: header.Filename,
		TestMode: "ONLINE", RegressionStatus: "NOT_ASSERTED",
		Status: "SUCCEEDED", LatencyMS: elapsed, DetectionCount: len(result.Frames),
		Result: jsonValue(result), CreatedAt: time.Now(),
	}
	if predictErr != nil {
		trace.Status, trace.ErrorMessage, trace.Result = "FAILED", predictErr.Error(), "{}"
		h.db.Create(&trace)
		evaluateAlertRules(h.db, trace)
		httpx.Fail(c, 502, 502, "视频推理失败："+predictErr.Error())
		return
	}
	h.db.Create(&trace)
	evaluateAlertRules(h.db, trace)
	httpx.OK(c, gin.H{
		"traceId": traceID, "deploymentId": deployment.ID, "deploymentRevisionId": revision.ID,
		"modelVersionId": revision.ModelVersionID, "result": result, "platformLatencyMs": elapsed,
	})
}

func calculateInferenceMetrics(traces []InferenceTrace) gin.H {
	if len(traces) == 0 {
		return gin.H{"requests": 0, "qps": 0, "errorRate": 0, "p50": 0, "p95": 0, "p99": 0, "emptyRate": 0, "meanConfidence": 0}
	}
	latencies := make([]float64, 0, len(traces))
	errors, empty := 0, 0
	var confidence float64
	for _, trace := range traces {
		latencies = append(latencies, trace.LatencyMS)
		if trace.Status != "SUCCEEDED" {
			errors++
		}
		if trace.DetectionCount == 0 {
			empty++
		}
		confidence += trace.MeanConfidence
	}
	sort.Float64s(latencies)
	percentile := func(value float64) float64 {
		index := int(math.Ceil(value*float64(len(latencies)))) - 1
		if index < 0 {
			index = 0
		}
		return latencies[index]
	}
	window := traces[0].CreatedAt.Sub(traces[len(traces)-1].CreatedAt).Seconds()
	if window < 1 {
		window = 1
	}
	return gin.H{
		"requests": len(traces), "qps": float64(len(traces)) / window,
		"errorRate": float64(errors) / float64(len(traces)), "emptyRate": float64(empty) / float64(len(traces)),
		"p50": percentile(.5), "p95": percentile(.95), "p99": percentile(.99),
		"meanConfidence": confidence / float64(len(traces)),
	}
}

type rollbackRequest struct {
	TargetRevisionID uint64 `json:"targetRevisionId"`
}

func (h *Handler) DeploymentRollback(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	deploymentID, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, deploymentID).First(&deployment).Error != nil {
		httpx.Fail(c, 404, 404, "部署不存在")
		return
	}
	var req rollbackRequest
	if c.ShouldBindJSON(&req) != nil || req.TargetRevisionID == 0 {
		httpx.Fail(c, 400, 400, "目标修订必填")
		return
	}
	var target DeploymentRevision
	if h.db.Where("tenant_id = ? AND deployment_id = ? AND id = ?", project.TenantID, deployment.ID, req.TargetRevisionID).First(&target).Error != nil {
		httpx.Fail(c, 404, 404, "目标修订不存在")
		return
	}
	var maxRevision int
	h.db.Model(&DeploymentRevision{}).Where("deployment_id = ?", deployment.ID).Select("COALESCE(MAX(revision_no),0)").Scan(&maxRevision)
	revision := DeploymentRevision{
		TenantID: project.TenantID, ProjectID: project.ID, DeploymentID: deployment.ID,
		RevisionNo: maxRevision + 1, ModelVersionID: target.ModelVersionID, SourceRevisionID: target.ID,
		Status: "ROLLING_BACK", Config: target.Config, ArtifactURI: target.ArtifactURI,
		ArtifactSHA256: target.ArtifactSHA256, CreatedBy: c.GetUint64("user_id"),
	}
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "DEPLOYMENT_ROLLBACK",
		ResourceType: "DEPLOYMENT_REVISION", Status: JobQueued, Stage: "ROLLING_BACK",
		TraceID: uuid.NewString(), Idempotency: "rollback:" + uuid.NewString(), MaxRetries: 2, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&revision).Error; err != nil {
			return err
		}
		job.ResourceID = revision.ID
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "deployment.revision.requested.v1",
			AggregateType: "DEPLOYMENT_REVISION", AggregateID: revision.ID,
			Payload: jsonValue(gin.H{"revisionId": revision.ID}), Status: outboxNew,
		}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "DEPLOYMENT_ROLLBACK_REQUESTED", "DEPLOYMENT_REVISION", revision.ID, gin.H{"currentRevisionId": deployment.CurrentRevisionID}, gin.H{"targetRevisionId": target.ID})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "回滚提交失败")
		return
	}
	httpx.OK(c, gin.H{"revision": revision, "job": job})
}

func (h *Handler) DeploymentRevisionCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	deploymentID, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, deploymentID).First(&deployment).Error != nil {
		httpx.Fail(c, 404, 404, "部署不存在")
		return
	}
	var req deploymentCreateRequest
	if c.ShouldBindJSON(&req) != nil || req.ModelVersionID == 0 {
		httpx.Fail(c, 400, 400, "模型版本必填")
		return
	}
	var version ModelVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, req.ModelVersionID).First(&version).Error != nil {
		httpx.Fail(c, 404, 404, "模型版本不存在")
		return
	}
	if deployment.Environment == "PRODUCTION" && version.Status != "APPROVED" {
		httpx.Fail(c, 409, 409, "生产修订仅允许 APPROVED 模型版本")
		return
	}
	var artifact ModelArtifact
	if h.db.Where("model_version_id = ?", version.ID).Order("CASE WHEN format IN ('ONNX','ENGINE','PT') THEN 0 ELSE 1 END, id").First(&artifact).Error != nil {
		httpx.Fail(c, 409, 409, "模型制品缺失")
		return
	}
	var maxRevision int
	h.db.Model(&DeploymentRevision{}).Where("deployment_id = ?", deployment.ID).Select("COALESCE(MAX(revision_no),0)").Scan(&maxRevision)
	revision := DeploymentRevision{
		TenantID: project.TenantID, ProjectID: project.ID, DeploymentID: deployment.ID,
		RevisionNo: maxRevision + 1, ModelVersionID: version.ID, Status: "DRAFT",
		Config: jsonValue(req.Config), ArtifactURI: artifact.URI, ArtifactSHA256: artifact.SHA256,
		CreatedBy: c.GetUint64("user_id"),
	}
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "DEPLOYMENT_RELEASE",
		ResourceType: "DEPLOYMENT_REVISION", Status: JobQueued, Stage: "QUEUED",
		TraceID: uuid.NewString(), Idempotency: "deployment-revision:" + uuid.NewString(), MaxRetries: 2, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&revision).Error; err != nil {
			return err
		}
		job.ResourceID = revision.ID
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "deployment.revision.requested.v1",
			AggregateType: "DEPLOYMENT_REVISION", AggregateID: revision.ID,
			Payload: jsonValue(gin.H{"revisionId": revision.ID}), Status: outboxNew,
		}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "DEPLOYMENT_REVISION_CREATED", "DEPLOYMENT_REVISION", revision.ID, nil, gin.H{"modelVersionId": version.ID, "config": req.Config})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "部署修订创建失败")
		return
	}
	httpx.OK(c, gin.H{"revision": revision, "job": job})
}

func (h *Handler) DeploymentRestart(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	deploymentID, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND current_revision_id > 0", project.TenantID, project.ID, deploymentID).First(&deployment).Error != nil {
		httpx.Fail(c, 409, 409, "部署没有可重启修订")
		return
	}
	var revision DeploymentRevision
	if h.db.Where("id = ?", deployment.CurrentRevisionID).First(&revision).Error != nil {
		httpx.Fail(c, 409, 409, "当前修订不存在")
		return
	}
	revision.Status, deployment.Status = "DEPLOYING", "DEPLOYING"
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "DEPLOYMENT_RESTART",
		ResourceType: "DEPLOYMENT_REVISION", ResourceID: revision.ID, Status: JobQueued, Stage: "RESTARTING",
		TraceID: uuid.NewString(), Idempotency: "deployment-restart:" + uuid.NewString(), MaxRetries: 2, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&revision).Error; err != nil {
			return err
		}
		if err := tx.Save(&deployment).Error; err != nil {
			return err
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		return tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "deployment.revision.requested.v1",
			AggregateType: "DEPLOYMENT_REVISION", AggregateID: revision.ID,
			Payload: jsonValue(gin.H{"revisionId": revision.ID}), Status: outboxNew,
		}).Error
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "部署重启失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "DEPLOYMENT_RESTARTED", "DEPLOYMENT", deployment.ID, nil, gin.H{"revisionId": revision.ID})
	httpx.OK(c, gin.H{"deployment": deployment, "job": job})
}

func (h *Handler) DeploymentStop(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	id, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&deployment).Error != nil {
		httpx.Fail(c, 404, 404, "部署不存在")
		return
	}
	if err := platforminference.NewClient(config.Load().InferenceAPIURL, config.Load().InferenceTimeout).Stop(c.Request.Context(), deployment.CurrentRevisionID); err != nil {
		httpx.Fail(c, 502, 502, err.Error())
		return
	}
	h.db.Model(&deployment).Update("status", "STOPPED")
	h.db.Model(&DeploymentRevision{}).Where("id = ?", deployment.CurrentRevisionID).Update("status", "STOPPED")
	_ = appendAudit(h.db, c, project.ID, "DEPLOYMENT_STOPPED", "DEPLOYMENT", deployment.ID, nil, nil)
	httpx.OK(c, true)
}

type alertRuleRequest struct {
	Name      string  `json:"name"`
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
}

func (h *Handler) AlertRuleCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	deploymentID, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, deploymentID).First(&deployment).Error != nil {
		httpx.Fail(c, 404, 404, "部署不存在")
		return
	}
	var req alertRuleRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, 400, 400, "告警规则无效")
		return
	}
	req.Metric = strings.ToUpper(req.Metric)
	if req.Metric != "LATENCY_MS" && req.Metric != "ERROR" && req.Metric != "EMPTY_RESULT" && req.Metric != "CONFIDENCE" {
		httpx.Fail(c, 400, 400, "不支持的告警指标")
		return
	}
	if req.Operator != ">" && req.Operator != "<" && req.Operator != ">=" && req.Operator != "<=" {
		httpx.Fail(c, 400, 400, "不支持的比较符")
		return
	}
	row := AlertRule{TenantID: project.TenantID, ProjectID: project.ID, DeploymentID: deployment.ID, Name: req.Name, Metric: req.Metric, Operator: req.Operator, Threshold: req.Threshold, Enabled: true, CreatedBy: c.GetUint64("user_id")}
	if h.db.Create(&row).Error != nil {
		httpx.Fail(c, 500, 500, "告警规则创建失败")
		return
	}
	httpx.OK(c, row)
}

func evaluateAlertRules(db *gorm.DB, trace InferenceTrace) {
	var rules []AlertRule
	db.Where("tenant_id = ? AND deployment_id = ? AND enabled = ?", trace.TenantID, trace.DeploymentID, true).Find(&rules)
	for _, rule := range rules {
		value := trace.LatencyMS
		switch rule.Metric {
		case "ERROR":
			if trace.Status == "FAILED" {
				value = 1
			} else {
				value = 0
			}
		case "EMPTY_RESULT":
			if trace.DetectionCount == 0 {
				value = 1
			} else {
				value = 0
			}
		case "CONFIDENCE":
			value = trace.MeanConfidence
		}
		triggered := (rule.Operator == ">" && value > rule.Threshold) ||
			(rule.Operator == ">=" && value >= rule.Threshold) ||
			(rule.Operator == "<" && value < rule.Threshold) ||
			(rule.Operator == "<=" && value <= rule.Threshold)
		if triggered {
			db.Create(&AlertEvent{
				TenantID: trace.TenantID, ProjectID: trace.ProjectID, DeploymentID: trace.DeploymentID,
				AlertRuleID: rule.ID, Status: "OPEN", MetricValue: value,
				Message: fmt.Sprintf("%s: %s %.4f (threshold %.4f)", rule.Name, rule.Metric, value, rule.Threshold),
			})
		}
	}
}

func (h *Handler) AlertAcknowledge(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	alertID, _ := strconv.ParseUint(c.Param("alertId"), 10, 64)
	var alert AlertEvent
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, alertID, "OPEN").First(&alert).Error != nil {
		httpx.Fail(c, 409, 409, "告警不存在或已处理")
		return
	}
	now := time.Now()
	alert.Status, alert.AcknowledgedBy, alert.AcknowledgedAt = "ACKNOWLEDGED", c.GetUint64("user_id"), &now
	h.db.Save(&alert)
	_ = appendAudit(h.db, c, project.ID, "ALERT_ACKNOWLEDGED", "ALERT_EVENT", alert.ID, nil, alert)
	httpx.OK(c, alert)
}
