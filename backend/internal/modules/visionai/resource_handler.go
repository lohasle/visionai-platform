package visionai

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	platformannotation "github.com/lohasle/nimbus-framework-go/internal/platform/annotation"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	platformtraining "github.com/lohasle/nimbus-framework-go/internal/platform/training"
	"gorm.io/gorm"
)

type nodeHeartbeatRequest struct {
	NodeKey         string         `json:"nodeKey"`
	Name            string         `json:"name"`
	GPUModel        string         `json:"gpuModel"`
	GPUCount        int            `json:"gpuCount"`
	GPUMemoryBytes  int64          `json:"gpuMemoryBytes"`
	GPUUsedBytes    int64          `json:"gpuUsedBytes"`
	GPUUtilization  float64        `json:"gpuUtilization"`
	GPUTemperatureC float64        `json:"gpuTemperatureC"`
	GPUPowerWatts   float64        `json:"gpuPowerWatts"`
	CPUUtilization  float64        `json:"cpuUtilization"`
	MemoryBytes     int64          `json:"memoryBytes"`
	MemoryUsedBytes int64          `json:"memoryUsedBytes"`
	DiskBytes       int64          `json:"diskBytes"`
	DiskUsedBytes   int64          `json:"diskUsedBytes"`
	NetworkRXBytes  int64          `json:"networkRxBytes"`
	NetworkTXBytes  int64          `json:"networkTxBytes"`
	DriverVersion   string         `json:"driverVersion"`
	CUDAVersion     string         `json:"cudaVersion"`
	Labels          map[string]any `json:"labels"`
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
	row.GPUUtilization, row.GPUTemperatureC, row.GPUPowerWatts = req.GPUUtilization, req.GPUTemperatureC, req.GPUPowerWatts
	row.CPUUtilization, row.MemoryBytes, row.MemoryUsedBytes = req.CPUUtilization, req.MemoryBytes, req.MemoryUsedBytes
	row.DiskBytes, row.DiskUsedBytes = req.DiskBytes, req.DiskUsedBytes
	row.NetworkRXBytes, row.NetworkTXBytes = req.NetworkRXBytes, req.NetworkTXBytes
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
	h.recordResourceSample(row)
	httpx.OK(c, row)
}

func (h *Handler) ResourceOverview(c *gin.Context) {
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
	var storageBytes, storageObjects, storageFailures, lifecyclePurged int64
	h.db.Model(&Asset{}).Where("tenant_id = ? AND status <> ?", tenantID(c), AssetPurged).Select("COALESCE(SUM(size),0)").Scan(&storageBytes)
	h.db.Model(&Asset{}).Where("tenant_id = ? AND status <> ?", tenantID(c), AssetPurged).Count(&storageObjects)
	h.db.Model(&Asset{}).Where("tenant_id = ? AND status = ?", tenantID(c), AssetInvalid).Count(&storageFailures)
	h.db.Model(&Asset{}).Where("tenant_id = ? AND status = ?", tenantID(c), AssetPurged).Count(&lifecyclePurged)
	var storageCapacityBytes int64
	h.db.Model(&ProjectQuota{}).Where("tenant_id = ?", tenantID(c)).
		Select("COALESCE(SUM(storage_bytes),0)").Scan(&storageCapacityBytes)
	storageUtilization := float64(0)
	if storageCapacityBytes > 0 {
		storageUtilization = float64(storageBytes) / float64(storageCapacityBytes)
	}
	var lifecyclePolicies int64
	h.db.Model(&ProjectConfig{}).Where("tenant_id = ? AND storage_config IS NOT NULL", tenantID(c)).Count(&lifecyclePolicies)
	gpuHours := h.measuredGPUHours(tenantID(c))
	httpx.OK(c, gin.H{
		"nodes": nodes, "queues": queueRows, "activeJobs": activeJobs, "queuedJobs": queuedJobs,
		"storageBytes": storageBytes, "storageObjects": storageObjects, "storageFailures": storageFailures,
		"storageCapacityBytes": storageCapacityBytes, "storageUtilization": storageUtilization,
		"lifecyclePolicies": lifecyclePolicies, "lifecyclePurged": lifecyclePurged, "gpuHours": gpuHours,
	})
}

// ResourceHistory godoc
// @Summary Query tenant compute-resource time series
// @Tags VisionAI Operations
// @Security BearerAuth
// @Param hours query int false "History window in hours (1..720)"
// @Param nodeKey query string false "Compute node key"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/resources/history [get]
func (h *Handler) ResourceHistory(c *gin.Context) {
	hours, err := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if err != nil || hours < 1 || hours > 24*30 {
		httpx.Fail(c, 400, 400, "hours must be between 1 and 720")
		return
	}
	query := h.db.Where("tenant_id = ? AND sampled_at >= ?", tenantID(c), time.Now().Add(-time.Duration(hours)*time.Hour))
	if nodeKey := strings.TrimSpace(c.Query("nodeKey")); nodeKey != "" {
		query = query.Where("node_key = ?", nodeKey)
	}
	var rows []ResourceMetricSample
	query.Order("sampled_at").Limit(10000).Find(&rows)
	httpx.OK(c, rows)
}

func (h *Handler) measuredGPUHours(tenant uint64) float64 {
	var runs []TrainingRun
	h.db.Where("tenant_id = ? AND gpu_count > 0 AND started_at IS NOT NULL", tenant).Find(&runs)
	now := time.Now()
	total := 0.0
	for _, run := range runs {
		end := now
		if run.FinishedAt != nil {
			end = *run.FinishedAt
		}
		if end.After(*run.StartedAt) {
			total += end.Sub(*run.StartedAt).Hours() * float64(run.GPUCount)
		}
	}
	return total
}

func (h *Handler) recordResourceSample(node ComputeNode) {
	var active []TrainingRun
	query := h.db.Where("tenant_id = ? AND status IN ?", node.TenantID, []TrainingStatus{TrainingAllocating, TrainingRunning, TrainingExporting})
	if strings.HasPrefix(node.NodeKey, "local-") {
		query = query.Where("provider = ?", "LOCAL_DOCKER")
	}
	query.Find(&active)
	runIDs := make([]uint64, 0, len(active))
	for _, run := range active {
		runIDs = append(runIDs, run.ID)
	}
	sample := ResourceMetricSample{
		TenantID: node.TenantID, ComputeNodeID: node.ID, NodeKey: node.NodeKey,
		GPUUtilization: node.GPUUtilization, GPUMemoryBytes: node.GPUMemoryBytes,
		GPUUsedBytes: node.GPUUsedBytes, GPUTemperatureC: node.GPUTemperatureC,
		GPUPowerWatts: node.GPUPowerWatts, CPUUtilization: node.CPUUtilization,
		MemoryBytes: node.MemoryBytes, MemoryUsedBytes: node.MemoryUsedBytes,
		DiskBytes: node.DiskBytes, DiskUsedBytes: node.DiskUsedBytes,
		NetworkRXBytes: node.NetworkRXBytes, NetworkTXBytes: node.NetworkTXBytes,
		ActiveRunIDs: jsonValue(runIDs), SampledAt: time.Now(),
	}
	h.db.Create(&sample)
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

// SampleLocalDockerGPU records host/container resource telemetry from the
// orchestrator process. The API container intentionally has no Docker socket.
func SampleLocalDockerGPU(ctx context.Context, db *gorm.DB, tenant uint64, cfg config.Config) error {
	const nodeKey = "local-docker-gpu-0"
	var current ComputeNode
	if db.Where("tenant_id = ? AND node_key = ?", tenant, nodeKey).First(&current).Error == nil &&
		time.Since(current.LastHeartbeatAt) < 30*time.Second {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	output, err := exec.CommandContext(
		ctx, cfg.DockerBinary,
		"run", "--rm", "--gpus", "all", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"nvidia/cuda:12.6.3-base-ubuntu24.04",
		"nvidia-smi", "--query-gpu=name,memory.total,driver_version,memory.used,utilization.gpu,temperature.gpu,power.draw", "--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return err
	}
	model, memoryBytes, driver, usedBytes, utilization, temperature, power, ok := parseGPUProbeMetrics(string(output))
	if !ok {
		return errors.New("NVIDIA GPU probe returned an invalid metrics row")
	}
	cpu, memoryUsed, memoryTotal, diskUsed, diskTotal, networkRX, networkTX := probeDockerRuntimeStats(ctx, cfg.DockerBinary)
	now := time.Now()
	node := ComputeNode{
		TenantID: tenant, NodeKey: nodeKey, Name: "Local Docker GPU",
		GPUModel: model, GPUCount: 1, GPUMemoryBytes: memoryBytes,
		GPUUsedBytes: usedBytes, GPUUtilization: utilization, GPUTemperatureC: temperature,
		GPUPowerWatts: power, CPUUtilization: cpu, MemoryUsedBytes: memoryUsed,
		MemoryBytes: memoryTotal, DiskUsedBytes: diskUsed, DiskBytes: diskTotal,
		NetworkRXBytes: networkRX, NetworkTXBytes: networkTX,
		DriverVersion: driver, CUDAVersion: "12.6", Status: "ONLINE",
		Labels: jsonValue(map[string]any{
			"runtime": "docker", "discovery": "automatic", "trainer": "LOCAL_DOCKER",
		}),
		LastHeartbeatAt: now,
	}
	if err = db.Where("tenant_id = ? AND node_key = ?", node.TenantID, node.NodeKey).
		Assign(node).FirstOrCreate(&node).Error; err != nil {
		return err
	}
	(&Handler{db: db}).recordResourceSample(node)
	return nil
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

func parseGPUProbeMetrics(output string) (string, int64, string, int64, float64, float64, float64, bool) {
	line := strings.TrimSpace(strings.Split(output, "\n")[0])
	fields := strings.Split(line, ",")
	if len(fields) != 7 {
		return "", 0, "", 0, 0, 0, 0, false
	}
	totalMiB, totalErr := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
	usedMiB, usedErr := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64)
	utilization, utilErr := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
	temperature, tempErr := strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
	power, powerErr := strconv.ParseFloat(strings.TrimSpace(fields[6]), 64)
	model, driver := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[2])
	ok := totalErr == nil && usedErr == nil && utilErr == nil && tempErr == nil && powerErr == nil &&
		totalMiB > 0 && usedMiB >= 0 && model != "" && driver != ""
	return model, totalMiB * 1024 * 1024, driver, usedMiB * 1024 * 1024, utilization, temperature, power, ok
}

type dockerStatsRow struct {
	CPUPercent string `json:"CPUPerc"`
	Memory     string `json:"MemUsage"`
	Network    string `json:"NetIO"`
	Block      string `json:"BlockIO"`
}

func probeDockerRuntimeStats(ctx context.Context, dockerBinary string) (float64, int64, int64, int64, int64, int64, int64) {
	output, err := exec.CommandContext(ctx, dockerBinary, "stats", "--no-stream", "--format", "{{json .}}", "visionai-backend").Output()
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0
	}
	var row dockerStatsRow
	if json.Unmarshal([]byte(strings.TrimSpace(string(output))), &row) != nil {
		return 0, 0, 0, 0, 0, 0, 0
	}
	cpu, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(row.CPUPercent), "%"), 64)
	memoryUsed, memoryTotal := parseDockerPair(row.Memory)
	networkRX, networkTX := parseDockerPair(row.Network)
	diskUsed, diskTotal := probeDockerDisk(ctx, dockerBinary)
	return cpu, memoryUsed, memoryTotal, diskUsed, diskTotal, networkRX, networkTX
}

func probeDockerDisk(ctx context.Context, dockerBinary string) (int64, int64) {
	output, err := exec.CommandContext(ctx, dockerBinary, "exec", "visionai-backend", "df", "-B1", "/").Output()
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return 0, 0
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 3 {
		return 0, 0
	}
	total, totalErr := strconv.ParseInt(fields[1], 10, 64)
	used, usedErr := strconv.ParseInt(fields[2], 10, 64)
	if totalErr != nil || usedErr != nil {
		return 0, 0
	}
	return used, total
}

func parseDockerPair(value string) (int64, int64) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0, 0
	}
	return parseHumanBytes(parts[0]), parseHumanBytes(parts[1])
}

func parseHumanBytes(value string) int64 {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	units := []struct {
		suffix string
		scale  float64
	}{
		{"GIB", 1 << 30}, {"GB", 1e9}, {"MIB", 1 << 20}, {"MB", 1e6},
		{"KIB", 1 << 10}, {"KB", 1e3}, {"B", 1},
	}
	for _, unit := range units {
		if strings.HasSuffix(normalized, unit.suffix) {
			number := strings.TrimSpace(strings.TrimSuffix(normalized, unit.suffix))
			parsed, err := strconv.ParseFloat(number, 64)
			if err == nil {
				return int64(parsed * unit.scale)
			}
		}
	}
	return 0
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
	Name          string         `json:"name"`
	ProviderType  string         `json:"providerType"`
	BaseURL       string         `json:"baseUrl"`
	NetworkRegion string         `json:"networkRegion"`
	SecretRef     string         `json:"secretRef"`
	Version       string         `json:"version"`
	Capabilities  map[string]any `json:"capabilities"`
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
		BaseURL: strings.TrimRight(req.BaseURL, "/"), NetworkRegion: strings.ToUpper(strings.TrimSpace(req.NetworkRegion)),
		SecretRef: req.SecretRef, Version: req.Version,
		Status: "UNKNOWN", Capabilities: jsonValue(req.Capabilities), CreatedBy: c.GetUint64("user_id"),
	}
	if row.NetworkRegion == "" {
		row.NetworkRegion = "LOCAL"
	}
	if h.db.Create(&row).Error != nil {
		httpx.Fail(c, 409, 409, "集成实例创建失败")
		return
	}
	httpx.OK(c, row)
}

type integrationTestResult struct {
	IntegrationInstance
	Success   bool               `json:"success"`
	LatencyMS float64            `json:"latencyMs"`
	Stage     string             `json:"stage"`
	Message   string             `json:"message"`
	Target    string             `json:"target"`
	Checks    []integrationCheck `json:"checks"`
}

type integrationCheck struct {
	Name      string  `json:"name"`
	Success   bool    `json:"success"`
	LatencyMS float64 `json:"latencyMs"`
	Detail    string  `json:"detail"`
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
	cfg := config.Load()
	target := integrationProbeTarget(row, config.Load())
	client := &http.Client{Timeout: 10 * time.Second}
	checks := make([]integrationCheck, 0, 4)
	record := func(name string, started time.Time, err error) error {
		check := integrationCheck{Name: name, Success: err == nil, LatencyMS: float64(time.Since(started).Microseconds()) / 1000, Detail: "通过"}
		if err != nil {
			check.Detail = err.Error()
		}
		checks = append(checks, check)
		return err
	}
	started := time.Now()
	response, err := client.Get(target)
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			err = fmt.Errorf("HTTP %s", response.Status)
		}
	}
	_ = record("网络与健康端点", started, err)
	if err == nil {
		started = time.Now()
		switch strings.ToUpper(row.ProviderType) {
		case "CVAT":
			baseURL := row.BaseURL
			if strings.TrimSpace(cfg.CVATBaseURL) != "" {
				baseURL = cfg.CVATBaseURL
			}
			var provider *platformannotation.CVAT
			provider, err = platformannotation.NewCVAT(baseURL, row.BaseURL, cfg.CVATUsername, cfg.CVATPassword, 10*time.Second)
			if err == nil {
				err = provider.About(c.Request.Context())
			}
		case "CLEARML":
			apiURL := row.BaseURL
			if strings.TrimSpace(cfg.ClearMLAPIURL) != "" {
				apiURL = cfg.ClearMLAPIURL
			}
			var provider *platformtraining.ClearML
			provider, err = platformtraining.NewClearML(apiURL, cfg.ClearMLWebURL, cfg.ClearMLAccessKey, cfg.ClearMLSecretKey, 10*time.Second)
			if err == nil {
				_, err = provider.Workers(c.Request.Context())
			}
		case "FIFTYONE":
			baseURL := row.BaseURL
			if strings.TrimSpace(cfg.FiftyOneAPIURL) != "" {
				baseURL = cfg.FiftyOneAPIURL
			}
			var request *http.Request
			request, err = http.NewRequestWithContext(
				c.Request.Context(), http.MethodGet, strings.TrimRight(baseURL, "/")+"/openapi.json", nil,
			)
			if err == nil {
				var keyResponse *http.Response
				keyResponse, err = client.Do(request)
				if err == nil {
					defer keyResponse.Body.Close()
					if keyResponse.StatusCode < 200 || keyResponse.StatusCode >= 300 {
						err = fmt.Errorf("关键 API 返回 HTTP %s", keyResponse.Status)
					} else {
						var contract []byte
						contract, err = io.ReadAll(io.LimitReader(keyResponse.Body, 2<<20))
						if err == nil && !bytes.Contains(contract, []byte(`"/similarity"`)) {
							err = errors.New("OpenAPI 未声明 /similarity 关键接口")
						}
					}
				}
			}
		default:
			err = errors.New("未提供此 Provider 的认证与关键 API 适配器")
		}
		_ = record("认证与关键 API", started, err)
	}
	if err == nil {
		started = time.Now()
		if h.storage == nil {
			err = errors.New("平台对象存储客户端未初始化")
		} else if err = h.storage.EnsureBucket(c.Request.Context()); err == nil {
			payload := []byte("visionai-integration-permission-check")
			key := fmt.Sprintf("healthchecks/tenant-%d/integration-%d-%d", row.TenantID, row.ID, time.Now().UnixNano())
			err = h.storage.Put(c.Request.Context(), key, bytes.NewReader(payload), int64(len(payload)), "text/plain")
			if err == nil {
				_, err = h.storage.Stat(c.Request.Context(), key)
			}
			deleteErr := h.storage.Delete(c.Request.Context(), key)
			if err == nil {
				err = deleteErr
			}
		}
		_ = record("对象存储读写权限", started, err)
	}
	latencyMS := float64(0)
	for _, check := range checks {
		latencyMS += check.LatencyMS
	}
	now := time.Now()
	row.LastCheckedAt = &now
	if err != nil {
		row.Status = "UNREACHABLE"
		stage := "NETWORK"
		if len(checks) > 0 {
			stage = checks[len(checks)-1].Name
		}
		if strings.Contains(stage, "认证") {
			row.Status = "AUTH_FAILED"
		} else if strings.Contains(stage, "对象存储") {
			row.Status = "PERMISSION_FAILED"
		}
		row.LastError = err.Error()
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
			Stage: stage, Message: row.LastError, Target: target, Checks: checks,
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
		Stage: "COMPLETED", Message: "网络、认证、关键 API 与对象存储权限均通过", Target: target, Checks: checks,
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
	h.syncIncidentAction(c, syncIncidentActionRequest{Action: "REPLAY", Reason: "兼容接口重放"})
}

func (h *Handler) AuditPage(c *gin.Context) {
	var rows []AuditEvent
	query := h.auditQuery(c)
	query.Order("id DESC").Limit(1000).Find(&rows)
	httpx.OK(c, rows)
}

func (h *Handler) AuditExport(c *gin.Context) {
	var rows []AuditEvent
	h.auditQuery(c).Order("id DESC").Limit(10000).Find(&rows)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=visionai-audit.csv")
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write([]string{"id", "time", "user", "project", "action", "resourceType", "resourceId", "traceId", "result"})
	for _, row := range rows {
		_ = writer.Write([]string{strconv.FormatUint(row.ID, 10), row.CreatedAt.Format(time.RFC3339), strconv.FormatUint(row.ActorUserID, 10), strconv.FormatUint(row.ProjectID, 10), row.Action, row.ResourceType, strconv.FormatUint(row.ResourceID, 10), row.TraceID, row.Result})
	}
	writer.Flush()
}

func (h *Handler) auditQuery(c *gin.Context) *gorm.DB {
	query := h.db.Where("tenant_id = ?", tenantID(c))
	if projectID := strings.TrimSpace(c.Query("projectId")); projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	if userID := strings.TrimSpace(c.Query("userId")); userID != "" {
		query = query.Where("actor_user_id = ?", userID)
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		query = query.Where("action LIKE ?", "%"+action+"%")
	}
	if resourceType := strings.TrimSpace(c.Query("resourceType")); resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if resourceID := strings.TrimSpace(c.Query("resourceId")); resourceID != "" {
		query = query.Where("resource_id = ?", resourceID)
	}
	if result := strings.ToUpper(strings.TrimSpace(c.Query("result"))); result != "" {
		query = query.Where("result = ?", result)
	}
	if createdFrom := strings.TrimSpace(c.Query("createdFrom")); createdFrom != "" {
		query = query.Where("created_at >= ?", createdFrom)
	}
	if createdTo := strings.TrimSpace(c.Query("createdTo")); createdTo != "" {
		query = query.Where("created_at <= ?", createdTo)
	}
	return query
}
