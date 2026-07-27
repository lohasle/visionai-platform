package visionai

import (
	"encoding/json"
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
	ChangeReason        string         `json:"changeReason"`
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
	if req.Environment == "PRODUCTION" && !modelProductionEligible(version) {
		httpx.Fail(c, 409, 409, "生产部署仅允许 APPROVED 模型版本")
		return
	}
	approvalRequestID := uint64(0)
	if req.Environment == "PRODUCTION" {
		var approval ApprovalRequest
		if h.db.Where("tenant_id = ? AND project_id = ? AND model_version_id = ? AND target_environment = ? AND status = ?",
			project.TenantID, project.ID, version.ID, req.Environment, "APPROVED").Order("id DESC").First(&approval).Error != nil {
			httpx.Fail(c, 409, 409, "生产部署缺少有效的目标环境审批")
			return
		}
		fingerprint, err := h.approvalFingerprint(version, req.Environment)
		if err != nil || fingerprint != approval.InputFingerprint {
			_ = h.db.Transaction(func(tx *gorm.DB) error {
				return saveApprovalInvalidation(tx, &approval, &version, "CONTROLLED_INPUT_CHANGED")
			})
			httpx.Fail(c, 409, 409, "模型制品、配置或目标环境已变化，原审批自动失效")
			return
		}
		approvalRequestID = approval.ID
	}
	if req.Environment != "PRODUCTION" && !modelProductionEligible(version) && !req.AllowUnapprovedTest {
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
		Status: "DRAFT", Config: jsonValue(req.Config), ArtifactURI: artifact.URI, ArtifactSHA256: artifact.SHA256,
		ChangeReason: firstNonEmpty(req.ChangeReason, "创建部署"), ApprovalRequestID: approvalRequestID, CreatedBy: userID,
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
		usage := DatasetUsage{
			TenantID: project.TenantID, ProjectID: project.ID, DatasetVersionID: version.DatasetVersionID,
			ResourceType: "DEPLOYMENT", ResourceID: deployment.ID,
		}
		if err := tx.Where(
			"dataset_version_id = ? AND resource_type = ? AND resource_id = ?",
			usage.DatasetVersionID, usage.ResourceType, usage.ResourceID,
		).FirstOrCreate(&usage).Error; err != nil {
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
	var baseline DriftBaseline
	h.db.Where("tenant_id = ? AND project_id = ? AND deployment_id = ?", project.TenantID, project.ID, deployment.ID).First(&baseline)
	httpx.OK(c, gin.H{
		"deployment": deployment, "revisions": revisions, "traces": traces,
		"alertRules": rules, "alerts": alerts, "metrics": calculateInferenceMetrics(traces),
		"drift":           calculateDriftMetrics(baseline, traces),
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
		captureFeedback(h, trace)
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
	captureFeedback(h, trace)
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

type driftBaselineRequest struct {
	WindowMinutes int `json:"windowMinutes"`
}

// DriftBaselineSave godoc
// @Summary Freeze a deployment drift baseline and configure its rolling window
// @Tags VisionAI Deployment
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param deploymentId path int true "Deployment ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/deployments/{deploymentId}/drift-baseline [post]
func (h *Handler) DriftBaselineSave(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	deploymentID, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var req driftBaselineRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "invalid drift baseline request")
		return
	}
	if req.WindowMinutes == 0 {
		req.WindowMinutes = 60
	}
	if req.WindowMinutes < 5 || req.WindowMinutes > 24*60*30 {
		httpx.Fail(c, 400, 400, "windowMinutes must be between 5 and 43200")
		return
	}
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, deploymentID).First(&deployment).Error != nil {
		httpx.Fail(c, 404, 404, "deployment not found")
		return
	}
	var traces []InferenceTrace
	h.db.Where("tenant_id = ? AND project_id = ? AND deployment_id = ? AND status = ?", project.TenantID, project.ID, deployment.ID, "SUCCEEDED").
		Order("created_at DESC").Limit(1000).Find(&traces)
	if len(traces) == 0 {
		httpx.Fail(c, 409, 409, "successful inference traces are required")
		return
	}
	from, to := traces[len(traces)-1].CreatedAt, traces[0].CreatedAt
	row := DriftBaseline{
		TenantID: project.TenantID, ProjectID: project.ID, DeploymentID: deployment.ID,
		ModelVersionID: traces[0].ModelVersionID, WindowMinutes: req.WindowMinutes,
		SampleCount: len(traces), MetricsSnapshot: jsonValue(calculateInferenceMetrics(traces)),
		ClassSnapshot: jsonValue(inferenceClassDistribution(traces)), BaselineFrom: from, BaselineTo: to,
		CreatedBy: c.GetUint64("user_id"),
	}
	var before DriftBaseline
	h.db.Where("tenant_id = ? AND deployment_id = ?", project.TenantID, deployment.ID).First(&before)
	if before.ID == 0 {
		h.db.Create(&row)
	} else {
		row.ID, row.CreatedAt, row.CreatedBy = before.ID, before.CreatedAt, before.CreatedBy
		h.db.Save(&row)
	}
	_ = appendAudit(h.db, c, project.ID, "DRIFT_BASELINE_SAVED", "DRIFT_BASELINE", row.ID, before, row)
	httpx.OK(c, row)
}

func calculateDriftMetrics(baseline DriftBaseline, traces []InferenceTrace) gin.H {
	if baseline.ID == 0 {
		return gin.H{"status": "BASELINE_REQUIRED", "windowMinutes": 0, "sampleCount": 0}
	}
	cutoff := time.Now().Add(-time.Duration(baseline.WindowMinutes) * time.Minute)
	current := make([]InferenceTrace, 0, len(traces))
	for _, trace := range traces {
		if !trace.CreatedAt.Before(cutoff) {
			current = append(current, trace)
		}
	}
	if len(current) == 0 {
		return gin.H{"status": "NO_CURRENT_SAMPLES", "windowMinutes": baseline.WindowMinutes, "sampleCount": 0, "baseline": baseline}
	}
	var baselineMetrics map[string]float64
	var baselineClasses map[string]float64
	_ = json.Unmarshal([]byte(baseline.MetricsSnapshot), &baselineMetrics)
	_ = json.Unmarshal([]byte(baseline.ClassSnapshot), &baselineClasses)
	currentMetricsAny := calculateInferenceMetrics(current)
	currentMetrics := make(map[string]float64, len(currentMetricsAny))
	for key, value := range currentMetricsAny {
		switch typed := value.(type) {
		case float64:
			currentMetrics[key] = typed
		case int:
			currentMetrics[key] = float64(typed)
		}
	}
	currentClasses := inferenceClassDistribution(current)
	psi := populationStabilityIndex(baselineClasses, currentClasses)
	status := "STABLE"
	if psi >= 0.25 || math.Abs(currentMetrics["meanConfidence"]-baselineMetrics["meanConfidence"]) >= 0.15 {
		status = "DRIFTED"
	} else if psi >= 0.1 {
		status = "WATCH"
	}
	return gin.H{
		"status": status, "windowMinutes": baseline.WindowMinutes, "sampleCount": len(current),
		"baseline": baseline, "currentMetrics": currentMetrics, "currentClasses": currentClasses,
		"classPSI":        psi,
		"confidenceDelta": currentMetrics["meanConfidence"] - baselineMetrics["meanConfidence"],
		"emptyRateDelta":  currentMetrics["emptyRate"] - baselineMetrics["emptyRate"],
	}
}

func inferenceClassDistribution(traces []InferenceTrace) map[string]float64 {
	counts := map[string]float64{}
	total := 0.0
	for _, trace := range traces {
		var result struct {
			Detections []struct {
				Label string `json:"label"`
			} `json:"detections"`
		}
		if json.Unmarshal([]byte(trace.Result), &result) != nil {
			continue
		}
		for _, detection := range result.Detections {
			label := strings.TrimSpace(detection.Label)
			if label == "" {
				label = "_unknown"
			}
			counts[label]++
			total++
		}
	}
	if total == 0 {
		return map[string]float64{"_empty": 1}
	}
	for label := range counts {
		counts[label] /= total
	}
	return counts
}

func populationStabilityIndex(baseline, current map[string]float64) float64 {
	keys := map[string]bool{}
	for key := range baseline {
		keys[key] = true
	}
	for key := range current {
		keys[key] = true
	}
	const epsilon = 0.000001
	total := 0.0
	for key := range keys {
		expected, actual := math.Max(baseline[key], epsilon), math.Max(current[key], epsilon)
		total += (actual - expected) * math.Log(actual/expected)
	}
	return total
}

type rollbackRequest struct {
	TargetRevisionID uint64 `json:"targetRevisionId"`
	Reason           string `json:"reason"`
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
	if c.ShouldBindJSON(&req) != nil || req.TargetRevisionID == 0 || strings.TrimSpace(req.Reason) == "" {
		httpx.Fail(c, 400, 400, "目标修订和回滚原因必填")
		return
	}
	var target DeploymentRevision
	if h.db.Where("tenant_id = ? AND deployment_id = ? AND id = ?", project.TenantID, deployment.ID, req.TargetRevisionID).First(&target).Error != nil {
		httpx.Fail(c, 404, 404, "目标修订不存在")
		return
	}
	approvalRequestID := uint64(0)
	if deployment.Environment == "PRODUCTION" {
		var approval ApprovalRequest
		if h.db.Where(
			"tenant_id = ? AND project_id = ? AND model_version_id = ? AND target_environment = ? AND status = ?",
			project.TenantID, project.ID, target.ModelVersionID, deployment.Environment, "APPROVED",
		).Order("id DESC").First(&approval).Error != nil {
			httpx.Fail(c, 409, 409, "生产回滚目标缺少有效审批")
			return
		}
		approvalRequestID = approval.ID
	}
	var targetVersion ModelVersion
	if h.db.Where(
		"tenant_id = ? AND project_id = ? AND id = ?",
		project.TenantID, project.ID, target.ModelVersionID,
	).First(&targetVersion).Error != nil {
		httpx.Fail(c, 409, 409, "回滚目标模型版本不存在")
		return
	}
	var maxRevision int
	h.db.Model(&DeploymentRevision{}).Where("deployment_id = ?", deployment.ID).Select("COALESCE(MAX(revision_no),0)").Scan(&maxRevision)
	revision := DeploymentRevision{
		TenantID: project.TenantID, ProjectID: project.ID, DeploymentID: deployment.ID,
		RevisionNo: maxRevision + 1, ModelVersionID: target.ModelVersionID, SourceRevisionID: target.ID,
		Status: "ROLLING_BACK", Config: target.Config, ArtifactURI: target.ArtifactURI,
		ArtifactSHA256: target.ArtifactSHA256, ChangeReason: strings.TrimSpace(req.Reason),
		ApprovalRequestID: approvalRequestID, CreatedBy: c.GetUint64("user_id"),
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
		usage := DatasetUsage{
			TenantID: project.TenantID, ProjectID: project.ID, DatasetVersionID: targetVersion.DatasetVersionID,
			ResourceType: "DEPLOYMENT", ResourceID: deployment.ID,
		}
		if err := tx.Where(
			"dataset_version_id = ? AND resource_type = ? AND resource_id = ?",
			usage.DatasetVersionID, usage.ResourceType, usage.ResourceID,
		).FirstOrCreate(&usage).Error; err != nil {
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
		return appendAudit(tx, c, project.ID, "DEPLOYMENT_ROLLBACK_REQUESTED", "DEPLOYMENT_REVISION", revision.ID,
			gin.H{"currentRevisionId": deployment.CurrentRevisionID},
			gin.H{"targetRevisionId": target.ID, "reason": req.Reason, "approvalRequestId": approvalRequestID, "executionStatus": revision.Status})
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
	if c.ShouldBindJSON(&req) != nil || req.ModelVersionID == 0 || strings.TrimSpace(req.ChangeReason) == "" {
		httpx.Fail(c, 400, 400, "模型版本和配置变更原因必填")
		return
	}
	var version ModelVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, req.ModelVersionID).First(&version).Error != nil {
		httpx.Fail(c, 404, 404, "模型版本不存在")
		return
	}
	if deployment.Environment == "PRODUCTION" && !modelProductionEligible(version) {
		httpx.Fail(c, 409, 409, "生产修订仅允许 APPROVED 模型版本")
		return
	}
	approvalRequestID := uint64(0)
	if deployment.Environment == "PRODUCTION" {
		var approval ApprovalRequest
		if h.db.Where(
			"tenant_id = ? AND project_id = ? AND model_version_id = ? AND target_environment = ? AND status = ?",
			project.TenantID, project.ID, version.ID, deployment.Environment, "APPROVED",
		).Order("id DESC").First(&approval).Error != nil {
			httpx.Fail(c, 409, 409, "生产修订缺少有效审批")
			return
		}
		fingerprint, fingerprintErr := h.approvalFingerprint(version, deployment.Environment)
		if fingerprintErr != nil || fingerprint != approval.InputFingerprint {
			httpx.Fail(c, 409, 409, "生产审批输入指纹已失效")
			return
		}
		approvalRequestID = approval.ID
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
		ChangeReason: strings.TrimSpace(req.ChangeReason), ApprovalRequestID: approvalRequestID,
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
		usage := DatasetUsage{
			TenantID: project.TenantID, ProjectID: project.ID, DatasetVersionID: version.DatasetVersionID,
			ResourceType: "DEPLOYMENT", ResourceID: deployment.ID,
		}
		if err := tx.Where(
			"dataset_version_id = ? AND resource_type = ? AND resource_id = ?",
			usage.DatasetVersionID, usage.ResourceType, usage.ResourceID,
		).FirstOrCreate(&usage).Error; err != nil {
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
		return appendAudit(tx, c, project.ID, "DEPLOYMENT_REVISION_CREATED", "DEPLOYMENT_REVISION", revision.ID, nil,
			gin.H{"modelVersionId": version.ID, "config": req.Config, "changeReason": req.ChangeReason, "approvalRequestId": approvalRequestID})
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
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&deployment).Update("status", "STOPPED").Error; err != nil {
			return err
		}
		var revision DeploymentRevision
		if err := tx.Where("id = ?", deployment.CurrentRevisionID).First(&revision).Error; err != nil {
			return err
		}
		if err := tx.Model(&revision).Update("status", "STOPPED").Error; err != nil {
			return err
		}
		return syncModelDeploymentLifecycle(tx, project.TenantID, project.ID, revision.ModelVersionID)
	}); err != nil {
		httpx.Fail(c, 500, 500, "部署停止状态保存失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "DEPLOYMENT_STOPPED", "DEPLOYMENT", deployment.ID, nil, nil)
	httpx.OK(c, true)
}

type alertRuleRequest struct {
	Name                string   `json:"name"`
	Metric              string   `json:"metric"`
	Operator            string   `json:"operator"`
	Threshold           float64  `json:"threshold"`
	DurationSeconds     int      `json:"durationSeconds"`
	NotificationChannel string   `json:"notificationChannel"`
	Recipients          []uint64 `json:"recipients"`
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
	req.NotificationChannel = strings.ToUpper(strings.TrimSpace(req.NotificationChannel))
	if req.NotificationChannel == "" {
		req.NotificationChannel = "IN_APP"
	}
	if req.NotificationChannel != "IN_APP" && req.NotificationChannel != "AUDIT" {
		httpx.Fail(c, 400, 400, "unsupported notification channel")
		return
	}
	if req.DurationSeconds < 0 || req.DurationSeconds > 86400 {
		httpx.Fail(c, 400, 400, "告警持续时间必须在 0 到 86400 秒之间")
		return
	}
	if len(req.Recipients) == 0 {
		req.Recipients = []uint64{c.GetUint64("user_id")}
	}
	row := AlertRule{
		TenantID: project.TenantID, ProjectID: project.ID, DeploymentID: deployment.ID,
		Name: req.Name, Metric: req.Metric, Operator: req.Operator, Threshold: req.Threshold,
		DurationSeconds: req.DurationSeconds, Enabled: true,
		NotificationChannel: req.NotificationChannel, Recipients: jsonValue(req.Recipients),
		CreatedBy: c.GetUint64("user_id"),
	}
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
		if rule.SilencedUntil != nil && rule.SilencedUntil.After(time.Now()) {
			continue
		}
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
		now := time.Now()
		if triggered && rule.PendingSince == nil {
			rule.PendingSince = &now
			db.Model(&rule).Update("pending_since", now)
			if rule.DurationSeconds > 0 {
				continue
			}
		}
		if triggered && rule.PendingSince != nil &&
			now.Sub(*rule.PendingSince) < time.Duration(rule.DurationSeconds)*time.Second {
			continue
		}
		if !triggered && rule.PendingSince != nil {
			rule.PendingSince = nil
			db.Model(&rule).Update("pending_since", nil)
		}
		var active AlertEvent
		db.Where("tenant_id = ? AND alert_rule_id = ? AND status IN ?", trace.TenantID, rule.ID, []string{"OPEN", "ACKNOWLEDGED"}).
			Order("id DESC").First(&active)
		if triggered && active.ID == 0 {
			db.Create(&AlertEvent{
				TenantID: trace.TenantID, ProjectID: trace.ProjectID, DeploymentID: trace.DeploymentID,
				AlertRuleID: rule.ID, Status: "OPEN", MetricValue: value,
				Message:             fmt.Sprintf("%s: %s %.4f (threshold %.4f)", rule.Name, rule.Metric, value, rule.Threshold),
				NotificationChannel: rule.NotificationChannel, Recipients: rule.Recipients, NotifiedAt: &now,
			})
		} else if !triggered && active.ID != 0 {
			db.Model(&active).Updates(map[string]any{
				"status": "RECOVERED", "resolved_at": now, "resolution": "metric returned to normal",
			})
			db.Create(&AlertEvent{
				TenantID: trace.TenantID, ProjectID: trace.ProjectID, DeploymentID: trace.DeploymentID,
				AlertRuleID: rule.ID, Status: "RECOVERY_NOTIFICATION", MetricValue: value,
				Message:             fmt.Sprintf("%s recovered: %s %.4f", rule.Name, rule.Metric, value),
				NotificationChannel: rule.NotificationChannel, Recipients: rule.Recipients, NotifiedAt: &now,
				RecoveryEvent: true, RecoveryOfID: active.ID, ResolvedAt: &now,
			})
		}
	}
}

type alertSilenceRequest struct {
	DurationMinutes int    `json:"durationMinutes"`
	Reason          string `json:"reason"`
}

// AlertRuleSilence godoc
// @Summary Silence an alert rule with an auditable reason
// @Tags VisionAI Deployment
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param deploymentId path int true "Deployment ID"
// @Param ruleId path int true "Alert rule ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/deployments/{deploymentId}/alert-rules/{ruleId}/silence [post]
func (h *Handler) AlertRuleSilence(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	deploymentID, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	ruleID, _ := strconv.ParseUint(c.Param("ruleId"), 10, 64)
	var req alertSilenceRequest
	if c.ShouldBindJSON(&req) != nil || req.DurationMinutes < 1 || req.DurationMinutes > 10080 || strings.TrimSpace(req.Reason) == "" {
		httpx.Fail(c, 400, 400, "durationMinutes (1..10080) and reason are required")
		return
	}
	var rule AlertRule
	if h.db.Where("tenant_id = ? AND project_id = ? AND deployment_id = ? AND id = ?", project.TenantID, project.ID, deploymentID, ruleID).First(&rule).Error != nil {
		httpx.Fail(c, 404, 404, "alert rule not found")
		return
	}
	before := rule
	until := time.Now().Add(time.Duration(req.DurationMinutes) * time.Minute)
	rule.SilencedUntil, rule.SilenceReason = &until, strings.TrimSpace(req.Reason)
	h.db.Save(&rule)
	_ = appendAudit(h.db, c, project.ID, "ALERT_RULE_SILENCED", "ALERT_RULE", rule.ID, before, rule)
	httpx.OK(c, rule)
}

type alertResolveRequest struct {
	Resolution string `json:"resolution"`
}

// AlertResolve godoc
// @Summary Resolve an active alert with a disposition
// @Tags VisionAI Deployment
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param alertId path int true "Alert event ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/alerts/{alertId}/resolve [post]
func (h *Handler) AlertResolve(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	alertID, _ := strconv.ParseUint(c.Param("alertId"), 10, 64)
	var req alertResolveRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Resolution) == "" {
		httpx.Fail(c, 400, 400, "resolution is required")
		return
	}
	var alert AlertEvent
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status IN ?", project.TenantID, project.ID, alertID, []string{"OPEN", "ACKNOWLEDGED"}).First(&alert).Error != nil {
		httpx.Fail(c, 409, 409, "alert is not active")
		return
	}
	now := time.Now()
	before := alert
	alert.Status, alert.ResolvedBy, alert.ResolvedAt, alert.Resolution = "RESOLVED", c.GetUint64("user_id"), &now, strings.TrimSpace(req.Resolution)
	h.db.Save(&alert)
	_ = appendAudit(h.db, c, project.ID, "ALERT_RESOLVED", "ALERT_EVENT", alert.ID, before, alert)
	httpx.OK(c, alert)
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
