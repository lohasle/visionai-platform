package visionai

import (
	"context"
	"encoding/csv"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	platformtraining "github.com/lohasle/nimbus-framework-go/internal/platform/training"
	"gorm.io/gorm"
)

type nodeHeartbeatRequest struct {
	NodeKey        string         `json:"nodeKey"`
	Name           string         `json:"name"`
	GPUModel       string         `json:"gpuModel"`
	GPUCount       int            `json:"gpuCount"`
	GPUMemoryBytes int64          `json:"gpuMemoryBytes"`
	GPUUsedBytes   int64          `json:"gpuUsedBytes"`
	DriverVersion  string         `json:"driverVersion"`
	CUDAVersion    string         `json:"cudaVersion"`
	Labels         map[string]any `json:"labels"`
}

type computeQueueOverview struct {
	ComputeQueue
	QueuedJobs         int64   `json:"queuedJobs"`
	RunningJobs        int64   `json:"runningJobs"`
	AverageWaitSeconds float64 `json:"averageWaitSeconds"`
	CompletedJobs      int64   `json:"completedJobs"`
}

func (h *Handler) ComputeNodeHeartbeat(c *gin.Context) {
	if c.GetUint64("tenant_id") == 0 {
		httpx.Fail(c, 401, 401, "未认证")
		return
	}
	var req nodeHeartbeatRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.NodeKey) == "" {
		httpx.Fail(c, 400, 400, "节点标识必填")
		return
	}
	now := time.Now()
	var row ComputeNode
	err := h.db.Where("tenant_id = ? AND node_key = ?", tenantID(c), req.NodeKey).First(&row).Error
	row.TenantID, row.NodeKey, row.Name, row.Status = tenantID(c), req.NodeKey, req.Name, "ONLINE"
	row.GPUModel, row.GPUCount, row.GPUMemoryBytes, row.GPUUsedBytes = req.GPUModel, req.GPUCount, req.GPUMemoryBytes, req.GPUUsedBytes
	row.DriverVersion, row.CUDAVersion, row.Labels, row.LastHeartbeatAt = req.DriverVersion, req.CUDAVersion, jsonValue(req.Labels), now
	if err == gorm.ErrRecordNotFound {
		err = h.db.Create(&row).Error
	} else if err == nil {
		err = h.db.Save(&row).Error
	}
	if err != nil {
		httpx.Fail(c, 500, 500, "节点心跳保存失败")
		return
	}
	httpx.OK(c, row)
}

func (h *Handler) ResourceOverview(c *gin.Context) {
	h.refreshLocalDockerGPU(c)
	h.refreshClearMLWorkers(c)
	var nodes []ComputeNode
	var queues []ComputeQueue
	h.db.Where("tenant_id = ?", tenantID(c)).Order("id").Find(&nodes)
	h.db.Where("tenant_id = ?", tenantID(c)).Order("priority DESC,id").Find(&queues)
	var trainingRuns []TrainingRun
	h.db.Where("tenant_id = ?", tenantID(c)).
		Order("id DESC").Limit(1000).Find(&trainingRuns)
	queueRows := summarizeComputeQueues(queues, trainingRuns)
	now := time.Now()
	for i := range nodes {
		if now.Sub(nodes[i].LastHeartbeatAt) > 2*time.Minute {
			nodes[i].Status = "OFFLINE"
		}
	}
	var activeJobs, queuedJobs int64
	h.db.Model(&PlatformJob{}).Where("tenant_id = ? AND status = ?", tenantID(c), JobRunning).Count(&activeJobs)
	h.db.Model(&PlatformJob{}).Where("tenant_id = ? AND status IN ?", tenantID(c), []JobStatus{JobPending, JobQueued}).Count(&queuedJobs)
	var storageBytes int64
	h.db.Model(&Asset{}).Where("tenant_id = ? AND status <> ?", tenantID(c), AssetPurged).Select("COALESCE(SUM(size),0)").Scan(&storageBytes)
	httpx.OK(c, gin.H{"nodes": nodes, "queues": queueRows, "activeJobs": activeJobs, "queuedJobs": queuedJobs, "storageBytes": storageBytes})
}

func (h *Handler) refreshClearMLWorkers(c *gin.Context) {
	var recent int64
	h.db.Model(&ComputeNode{}).
		Where("tenant_id = ? AND node_key LIKE ? AND last_heartbeat_at > ?", tenantID(c), "clearml:%", time.Now().Add(-10*time.Second)).
		Count(&recent)
	if recent > 0 {
		return
	}
	cfg := config.Load()
	client, err := platformtraining.NewClearML(
		cfg.ClearMLAPIURL, cfg.ClearMLWebURL,
		cfg.ClearMLAccessKey, cfg.ClearMLSecretKey, 5*time.Second,
	)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	workers, err := client.Workers(ctx)
	if err != nil {
		return
	}
	for _, worker := range workers {
		heartbeat := worker.LastReport
		if heartbeat.IsZero() {
			heartbeat = time.Now()
		}
		var node ComputeNode
		nodeKey := "clearml:" + worker.ID
		h.db.Where("tenant_id = ? AND node_key = ?", tenantID(c), nodeKey).First(&node)
		node.TenantID, node.NodeKey, node.Name = tenantID(c), nodeKey, worker.ID
		node.Status, node.LastHeartbeatAt = "ONLINE", heartbeat
		node.Labels = jsonValue(map[string]any{
			"provider": "CLEARML", "queues": worker.Queues, "discovery": "workers.get_all",
		})
		if node.ID == 0 {
			_ = h.db.Create(&node).Error
		} else {
			_ = h.db.Save(&node).Error
		}
	}
}

func summarizeComputeQueues(queues []ComputeQueue, runs []TrainingRun) []computeQueueOverview {
	result := make([]computeQueueOverview, 0, len(queues))
	for _, queue := range queues {
		row := computeQueueOverview{ComputeQueue: queue}
		var totalWait time.Duration
		var started int64
		for _, run := range runs {
			if !strings.EqualFold(run.Provider, queue.Provider) ||
				(run.Queue != queue.Name && run.Queue != queue.ExternalQueue) {
				continue
			}
			switch run.Status {
			case TrainingQueued, TrainingAllocating:
				row.QueuedJobs++
			case TrainingRunning, TrainingExporting:
				row.RunningJobs++
			case TrainingSucceeded:
				row.CompletedJobs++
			}
			if run.StartedAt != nil && !run.CreatedAt.IsZero() && !run.StartedAt.Before(run.CreatedAt) {
				totalWait += run.StartedAt.Sub(run.CreatedAt)
				started++
			}
		}
		if started > 0 {
			row.AverageWaitSeconds = totalWait.Seconds() / float64(started)
		}
		result = append(result, row)
	}
	return result
}

func (h *Handler) refreshLocalDockerGPU(c *gin.Context) {
	const nodeKey = "local-docker-gpu-0"
	var current ComputeNode
	if h.db.Where("tenant_id = ? AND node_key = ?", tenantID(c), nodeKey).First(&current).Error == nil &&
		time.Since(current.LastHeartbeatAt) < 30*time.Second {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	cfg := config.Load()
	output, err := exec.CommandContext(
		ctx, cfg.DockerBinary,
		"run", "--rm", "--gpus", "all", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"nvidia/cuda:12.6.3-base-ubuntu24.04",
		"nvidia-smi", "--query-gpu=name,memory.total,driver_version", "--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return
	}
	model, memoryBytes, driver, ok := parseGPUProbe(string(output))
	if !ok {
		return
	}
	now := time.Now()
	node := ComputeNode{
		TenantID: tenantID(c), NodeKey: nodeKey, Name: "Local Docker GPU",
		GPUModel: model, GPUCount: 1, GPUMemoryBytes: memoryBytes,
		DriverVersion: driver, CUDAVersion: "12.6", Status: "ONLINE",
		Labels: jsonValue(map[string]any{
			"runtime": "docker", "discovery": "automatic", "trainer": "LOCAL_DOCKER",
		}),
		LastHeartbeatAt: now,
	}
	h.db.Where("tenant_id = ? AND node_key = ?", node.TenantID, node.NodeKey).
		Assign(node).FirstOrCreate(&node)
}

func parseGPUProbe(output string) (string, int64, string, bool) {
	line := strings.TrimSpace(strings.Split(output, "\n")[0])
	fields := strings.Split(line, ",")
	if len(fields) != 3 {
		return "", 0, "", false
	}
	memoryMiB, err := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
	if err != nil || memoryMiB < 1 {
		return "", 0, "", false
	}
	model, driver := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[2])
	return model, memoryMiB * 1024 * 1024, driver, model != "" && driver != ""
}

type queueRequest struct {
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	ExternalQueue string `json:"externalQueue"`
	Priority      int    `json:"priority"`
	Enabled       bool   `json:"enabled"`
}

func (h *Handler) ComputeQueueSave(c *gin.Context) {
	var req queueRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.ExternalQueue) == "" {
		httpx.Fail(c, 400, 400, "队列名称与外部队列必填")
		return
	}
	row := ComputeQueue{TenantID: tenantID(c), Name: strings.TrimSpace(req.Name), Provider: strings.ToUpper(req.Provider), ExternalQueue: req.ExternalQueue, Priority: req.Priority, Enabled: req.Enabled}
	if h.db.Where("tenant_id = ? AND name = ? AND provider = ?", row.TenantID, row.Name, row.Provider).Assign(row).FirstOrCreate(&row).Error != nil {
		httpx.Fail(c, 500, 500, "计算队列保存失败")
		return
	}
	httpx.OK(c, row)
}

type quotaRequest struct {
	MaxConcurrentJobs int     `json:"maxConcurrentJobs"`
	MonthlyGPUHours   float64 `json:"monthlyGpuHours"`
	StorageBytes      int64   `json:"storageBytes"`
}

func (h *Handler) ProjectQuotaSave(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER") {
		return
	}
	var req quotaRequest
	if c.ShouldBindJSON(&req) != nil || req.MaxConcurrentJobs < 1 || req.StorageBytes < 1 {
		httpx.Fail(c, 400, 400, "配额参数无效")
		return
	}
	var row ProjectQuota
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).First(&row)
	before := row
	row.TenantID, row.ProjectID, row.MaxConcurrentJobs = project.TenantID, project.ID, req.MaxConcurrentJobs
	row.MonthlyGPUHours, row.StorageBytes, row.UpdatedBy = req.MonthlyGPUHours, req.StorageBytes, c.GetUint64("user_id")
	if row.ID == 0 {
		h.db.Create(&row)
	} else {
		h.db.Save(&row)
	}
	_ = appendAudit(h.db, c, project.ID, "PROJECT_QUOTA_UPDATED", "PROJECT_QUOTA", row.ID, before, row)
	httpx.OK(c, row)
}

type integrationRequest struct {
	Name         string         `json:"name"`
	ProviderType string         `json:"providerType"`
	BaseURL      string         `json:"baseUrl"`
	SecretRef    string         `json:"secretRef"`
	Version      string         `json:"version"`
	Capabilities map[string]any `json:"capabilities"`
}

func (h *Handler) IntegrationPage(c *gin.Context) {
	var instances []IntegrationInstance
	var rules []CompatibilityRule
	var incidents []SyncIncident
	h.db.Where("tenant_id = ?", tenantID(c)).Order("id DESC").Find(&instances)
	h.db.Where("tenant_id = ?", tenantID(c)).Order("id DESC").Find(&rules)
	h.db.Where("tenant_id = ?", tenantID(c)).Order("id DESC").Limit(200).Find(&incidents)
	httpx.OK(c, gin.H{"instances": instances, "compatibilityRules": rules, "incidents": incidents})
}

func (h *Handler) IntegrationCreate(c *gin.Context) {
	var req integrationRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.BaseURL) == "" || strings.TrimSpace(req.SecretRef) == "" {
		httpx.Fail(c, 400, 400, "实例名称、URL 与 Secret Ref 必填")
		return
	}
	if strings.Contains(req.SecretRef, "://") || len(req.SecretRef) > 512 {
		httpx.Fail(c, 400, 400, "Secret 仅允许保存引用，不允许保存凭据值")
		return
	}
	row := IntegrationInstance{
		TenantID: tenantID(c), Name: req.Name, ProviderType: strings.ToUpper(req.ProviderType),
		BaseURL: strings.TrimRight(req.BaseURL, "/"), SecretRef: req.SecretRef, Version: req.Version,
		Status: "UNKNOWN", Capabilities: jsonValue(req.Capabilities), CreatedBy: c.GetUint64("user_id"),
	}
	if h.db.Create(&row).Error != nil {
		httpx.Fail(c, 409, 409, "集成实例创建失败")
		return
	}
	httpx.OK(c, row)
}

type integrationTestResult struct {
	IntegrationInstance
	Success   bool    `json:"success"`
	LatencyMS float64 `json:"latencyMs"`
	Stage     string  `json:"stage"`
	Message   string  `json:"message"`
	Target    string  `json:"target"`
}

// IntegrationTest godoc
// @Summary Test an integration using its provider-internal health endpoint
// @Tags VisionAI Operations
// @Security BearerAuth
// @Param instanceId path int true "Integration instance ID"
// @Success 200 {object} httpx.Response
// @Failure 502 {object} httpx.Response
// @Router /ai-platform/integrations/{instanceId}/test [post]
func (h *Handler) IntegrationTest(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("instanceId"), 10, 64)
	var row IntegrationInstance
	if h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "集成实例不存在")
		return
	}
	target := integrationProbeTarget(row, config.Load())
	client := &http.Client{Timeout: 10 * time.Second}
	started := time.Now()
	response, err := client.Get(target)
	latencyMS := float64(time.Since(started).Microseconds()) / 1000
	now := time.Now()
	row.LastCheckedAt = &now
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		row.Status = "UNREACHABLE"
		stage := "NETWORK"
		if err != nil {
			row.LastError = err.Error()
		} else {
			row.LastError = response.Status
			stage = "HTTP"
			if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
				row.Status = "AUTH_FAILED"
				stage = "AUTHENTICATION"
			}
			response.Body.Close()
		}
		h.db.Save(&row)
		incident := SyncIncident{
			TenantID: row.TenantID, InstanceID: row.ID, ResourceType: "INTEGRATION",
			ResourceID: strconv.FormatUint(row.ID, 10), Code: row.Status, Status: "OPEN",
		}
		h.db.Where(
			"tenant_id = ? AND instance_id = ? AND resource_type = ? AND resource_id = ? AND code = ? AND status = ?",
			row.TenantID, row.ID, incident.ResourceType, incident.ResourceID, incident.Code, incident.Status,
		).Assign(map[string]any{"message": row.LastError, "last_attempt_at": now}).FirstOrCreate(&incident)
		result := integrationTestResult{
			IntegrationInstance: row, Success: false, LatencyMS: latencyMS,
			Stage: stage, Message: row.LastError, Target: target,
		}
		c.AbortWithStatusJSON(http.StatusBadGateway, httpx.Response{
			Code: 502, Data: result, Msg: "连接测试失败：" + row.Status,
		})
		return
	}
	response.Body.Close()
	row.Status, row.LastError = "HEALTHY", ""
	h.db.Save(&row)
	h.db.Model(&SyncIncident{}).
		Where("tenant_id = ? AND instance_id = ? AND resource_type = ? AND status = ?", row.TenantID, row.ID, "INTEGRATION", "OPEN").
		Updates(map[string]any{"status": "RESOLVED", "resolved_at": now})
	httpx.OK(c, integrationTestResult{
		IntegrationInstance: row, Success: true, LatencyMS: latencyMS,
		Stage: "HEALTH_CHECK", Message: "连接测试通过", Target: target,
	})
}

func integrationProbeTarget(row IntegrationInstance, cfg config.Config) string {
	base := strings.TrimRight(row.BaseURL, "/")
	switch strings.ToUpper(row.ProviderType) {
	case "CVAT":
		if strings.TrimSpace(cfg.CVATBaseURL) != "" {
			base = strings.TrimRight(cfg.CVATBaseURL, "/")
		}
		return base + "/api/server/about"
	case "FIFTYONE":
		if strings.TrimSpace(cfg.FiftyOneAPIURL) != "" {
			base = strings.TrimRight(cfg.FiftyOneAPIURL, "/")
		}
		return base + "/health"
	case "CLEARML":
		if strings.TrimSpace(cfg.ClearMLAPIURL) != "" {
			base = strings.TrimRight(cfg.ClearMLAPIURL, "/")
		}
		return base + "/debug.ping"
	default:
		return base + "/health"
	}
}

type compatibilityRequest struct {
	ProviderType  string `json:"providerType"`
	PlatformRange string `json:"platformRange"`
	ProviderRange string `json:"providerRange"`
	Decision      string `json:"decision"`
	Notes         string `json:"notes"`
}

func (h *Handler) CompatibilityRuleCreate(c *gin.Context) {
	var req compatibilityRequest
	if c.ShouldBindJSON(&req) != nil || req.ProviderType == "" || req.PlatformRange == "" || req.ProviderRange == "" {
		httpx.Fail(c, 400, 400, "兼容规则参数无效")
		return
	}
	req.Decision = strings.ToUpper(req.Decision)
	if req.Decision != "ALLOWED" && req.Decision != "REVIEW_REQUIRED" && req.Decision != "BLOCKED" {
		httpx.Fail(c, 400, 400, "兼容决定无效")
		return
	}
	row := CompatibilityRule{TenantID: tenantID(c), ProviderType: strings.ToUpper(req.ProviderType), PlatformRange: req.PlatformRange, ProviderRange: req.ProviderRange, Decision: req.Decision, Notes: req.Notes, CreatedBy: c.GetUint64("user_id")}
	h.db.Create(&row)
	httpx.OK(c, row)
}

func (h *Handler) SyncIncidentReplay(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("incidentId"), 10, 64)
	var row SyncIncident
	if h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "同步异常不存在")
		return
	}
	now := time.Now()
	row.Attempts++
	row.LastAttemptAt = &now
	var instance IntegrationInstance
	if h.db.Where("tenant_id = ? AND id = ? AND status = ?", row.TenantID, row.InstanceID, "HEALTHY").First(&instance).Error == nil {
		row.Status, row.ResolvedAt = "RESOLVED", &now
	}
	h.db.Save(&row)
	httpx.OK(c, row)
}

func (h *Handler) AuditPage(c *gin.Context) {
	var rows []AuditEvent
	query := h.db.Where("tenant_id = ?", tenantID(c))
	if projectID := c.Query("projectId"); projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	if userID := c.Query("userId"); userID != "" {
		query = query.Where("actor_user_id = ?", userID)
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		query = query.Where("action LIKE ?", "%"+action+"%")
	}
	if resourceType := strings.TrimSpace(c.Query("resourceType")); resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	query.Order("id DESC").Limit(1000).Find(&rows)
	httpx.OK(c, rows)
}

func (h *Handler) AuditExport(c *gin.Context) {
	var rows []AuditEvent
	h.db.Where("tenant_id = ?", tenantID(c)).Order("id DESC").Limit(10000).Find(&rows)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=visionai-audit.csv")
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write([]string{"id", "time", "user", "project", "action", "resourceType", "resourceId", "traceId", "result"})
	for _, row := range rows {
		_ = writer.Write([]string{strconv.FormatUint(row.ID, 10), row.CreatedAt.Format(time.RFC3339), strconv.FormatUint(row.ActorUserID, 10), strconv.FormatUint(row.ProjectID, 10), row.Action, row.ResourceType, strconv.FormatUint(row.ResourceID, 10), row.TraceID, row.AfterJSON})
	}
	writer.Flush()
}
