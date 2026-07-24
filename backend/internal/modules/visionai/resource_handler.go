package visionai

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
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
	var nodes []ComputeNode
	var queues []ComputeQueue
	h.db.Where("tenant_id = ?", tenantID(c)).Order("id").Find(&nodes)
	h.db.Where("tenant_id = ?", tenantID(c)).Order("priority DESC,id").Find(&queues)
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
	httpx.OK(c, gin.H{"nodes": nodes, "queues": queues, "activeJobs": activeJobs, "queuedJobs": queuedJobs, "storageBytes": storageBytes})
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

func (h *Handler) IntegrationTest(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("instanceId"), 10, 64)
	var row IntegrationInstance
	if h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "集成实例不存在")
		return
	}
	target := row.BaseURL
	if row.ProviderType == "CVAT" {
		target += "/api/server/about"
	} else {
		target += "/health"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(target)
	now := time.Now()
	row.LastCheckedAt = &now
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		row.Status = "AUTH_FAILED"
		if err != nil {
			row.LastError = err.Error()
		} else {
			row.LastError = response.Status
			response.Body.Close()
		}
		h.db.Save(&row)
		h.db.Create(&SyncIncident{TenantID: row.TenantID, InstanceID: row.ID, ResourceType: "INTEGRATION", ResourceID: strconv.FormatUint(row.ID, 10), Code: row.Status, Message: row.LastError, Status: "OPEN"})
		httpx.Fail(c, 502, 502, "连接测试失败")
		return
	}
	response.Body.Close()
	row.Status, row.LastError = "HEALTHY", ""
	h.db.Save(&row)
	httpx.OK(c, row)
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
