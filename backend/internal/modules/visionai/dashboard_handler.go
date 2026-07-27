package visionai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

type dashboardProject struct {
	ID     uint64        `json:"id"`
	Code   string        `json:"code"`
	Name   string        `json:"name"`
	Status ProjectStatus `json:"status"`
}

type dashboardLifecycle struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Count   int64  `json:"count"`
	Blocked int64  `json:"blocked"`
	Route   string `json:"route"`
}

type dashboardTodo struct {
	Key          string `json:"key"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	Reason       string `json:"reason"`
	ResourceType string `json:"resourceType"`
	ResourceID   uint64 `json:"resourceId"`
	ProjectID    uint64 `json:"projectId"`
	Priority     string `json:"priority"`
	Route        string `json:"route"`
	CreatedAt    string `json:"createTime"`
}

type dashboardJob struct {
	ID           uint64    `json:"id"`
	ProjectID    uint64    `json:"projectId"`
	JobType      string    `json:"jobType"`
	ResourceType string    `json:"resourceType"`
	ResourceID   uint64    `json:"resourceId"`
	Status       JobStatus `json:"status"`
	Stage        string    `json:"stage"`
	Progress     int       `json:"progress"`
	DurationSec  int64     `json:"durationSeconds"`
	ErrorMessage string    `json:"errorMessage"`
	Remediation  string    `json:"remediation"`
	Actions      []string  `json:"actions"`
	Route        string    `json:"route"`
	CreatedAt    string    `json:"createTime"`
}

type dashboardService struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	LastChecked string `json:"lastCheckedAt,omitempty"`
}

type dashboardActivity struct {
	ID           uint64 `json:"id"`
	Action       string `json:"action"`
	ResourceType string `json:"resourceType"`
	ResourceID   uint64 `json:"resourceId"`
	ProjectID    uint64 `json:"projectId"`
	ActorUserID  uint64 `json:"actorUserId"`
	CreatedAt    string `json:"createTime"`
}

func dashboardRoute(resourceType string) string {
	switch strings.ToUpper(resourceType) {
	case "ASSET", "ASSET_IMPORT", "COLLECTION", "DATASET_VERSION":
		return "/ai-platform/assets"
	case "ANNOTATION_TASK", "PREANNOTATION_RUN":
		return "/ai-platform/annotations"
	case "TRAINING_RUN", "TRAINING_TEMPLATE_VERSION":
		return "/ai-platform/training"
	case "EVALUATION_RUN", "EVALUATION_SUITE":
		return "/ai-platform/evaluation"
	case "MODEL", "MODEL_VERSION", "APPROVAL_REQUEST":
		return "/ai-platform/models"
	case "DEPLOYMENT", "ALERT_EVENT":
		return "/ai-platform/deployments"
	default:
		return "/ai-platform/operations"
	}
}

func parseDashboardIDs(raw string) map[uint64]struct{} {
	result := map[uint64]struct{}{}
	var ids []uint64
	if json.Unmarshal([]byte(raw), &ids) == nil {
		for _, id := range ids {
			result[id] = struct{}{}
		}
	}
	return result
}

func dashboardCount(db *gorm.DB, model any, tenant uint64, projectIDs []uint64, extra string, args ...any) int64 {
	var count int64
	query := db.Model(model).Where("tenant_id = ? AND project_id IN ?", tenant, projectIDs)
	if extra != "" {
		query = query.Where(extra, args...)
	}
	query.Count(&count)
	return count
}

func dashboardGPUHours(db *gorm.DB, tenant uint64, projectIDs []uint64) float64 {
	var runs []TrainingRun
	db.Where("tenant_id = ? AND project_id IN ? AND gpu_count > 0 AND started_at IS NOT NULL", tenant, projectIDs).Find(&runs)
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

func probeDashboardService(ctx *gin.Context, name, provider, target string) dashboardService {
	service := dashboardService{Name: name, Provider: provider, Status: "DOWN", Detail: "健康检查失败"}
	if strings.TrimSpace(target) == "" {
		service.Status, service.Detail = "UNCONFIGURED", "尚未配置服务地址"
		return service
	}
	request, err := http.NewRequestWithContext(ctx.Request.Context(), http.MethodGet, strings.TrimRight(target, "/")+"/health", nil)
	if err != nil {
		service.Detail = err.Error()
		return service
	}
	client := &http.Client{Timeout: 3 * time.Second}
	startedAt := time.Now()
	response, err := client.Do(request)
	if err != nil {
		service.Detail = err.Error()
		return service
	}
	defer response.Body.Close()
	service.LastChecked = time.Now().Format(time.RFC3339)
	service.Detail = fmt.Sprintf("HTTP %d · %dms", response.StatusCode, time.Since(startedAt).Milliseconds())
	if response.StatusCode >= 200 && response.StatusCode < 400 {
		service.Status = "UP"
	}
	return service
}

// DashboardSummary returns a permission-scoped workbench with actionable data.
func (h *Handler) DashboardSummary(c *gin.Context) {
	if h.db == nil {
		httpx.Fail(c, http.StatusServiceUnavailable, 503, "数据库不可用")
		return
	}
	tenant, user := tenantID(c), c.GetUint64("user_id")
	var projects []Project
	h.db.Model(&Project{}).
		Joins("JOIN ai_project_member AS member ON member.project_id = ai_project.id AND member.tenant_id = ai_project.tenant_id").
		Where("ai_project.tenant_id = ? AND member.user_id = ?", tenant, user).
		Order("ai_project.updated_at DESC").
		Find(&projects)
	projectOptions := make([]dashboardProject, 0, len(projects))
	accessible := make(map[uint64]struct{}, len(projects))
	projectIDs := make([]uint64, 0, len(projects))
	for _, project := range projects {
		accessible[project.ID] = struct{}{}
		projectIDs = append(projectIDs, project.ID)
		projectOptions = append(projectOptions, dashboardProject{ID: project.ID, Code: project.Code, Name: project.Name, Status: project.Status})
	}
	selectedID, _ := strconv.ParseUint(c.Query("projectId"), 10, 64)
	if selectedID > 0 {
		if _, ok := accessible[selectedID]; !ok {
			httpx.Fail(c, http.StatusForbidden, 403, "无权查看该项目工作台")
			return
		}
		projectIDs = []uint64{selectedID}
	}
	if len(projectIDs) == 0 {
		httpx.OK(c, gin.H{
			"scope":   gin.H{"projectId": selectedID, "projects": projectOptions},
			"metrics": gin.H{"projects": 0, "datasetVersions": 0, "activeJobs": 0, "models": 0, "deployments": 0, "gpuHours": 0},
			"jobs":    gin.H{"running": 0, "failed": 0, "pending": 0}, "lifecycle": []dashboardLifecycle{},
			"todos": []dashboardTodo{}, "recentJobs": []dashboardJob{}, "services": []dashboardService{},
			"activities": []dashboardActivity{}, "resources": gin.H{"visibility": "QUOTA_ONLY"},
		})
		return
	}

	var running, failed, pending int64
	jobScope := h.db.Model(&PlatformJob{}).Where("tenant_id = ? AND project_id IN ?", tenant, projectIDs)
	jobScope.Where("status IN ?", []JobStatus{JobQueued, JobRunning}).Count(&running)
	jobScope.Where("status = ?", JobFailed).Count(&failed)
	jobScope.Where("status = ?", JobPending).Count(&pending)

	lifecycle := []dashboardLifecycle{
		{Key: "data", Label: "数据准备", Count: dashboardCount(h.db, &Asset{}, tenant, projectIDs, "status = ?", AssetReady), Blocked: dashboardCount(h.db, &Asset{}, tenant, projectIDs, "status IN ?", []AssetStatus{AssetInvalid, AssetMissing}), Route: "/ai-platform/assets"},
		{Key: "annotation", Label: "标注", Count: dashboardCount(h.db, &AnnotationTask{}, tenant, projectIDs, "", nil), Blocked: dashboardCount(h.db, &AnnotationTask{}, tenant, projectIDs, "status IN ?", []AnnotationStatus{AnnotationFailed, AnnotationRejected}), Route: "/ai-platform/annotations"},
		{Key: "training", Label: "训练", Count: dashboardCount(h.db, &TrainingRun{}, tenant, projectIDs, "", nil), Blocked: dashboardCount(h.db, &TrainingRun{}, tenant, projectIDs, "status IN ?", []TrainingStatus{TrainingFailed, TrainingTimeout}), Route: "/ai-platform/training"},
		{Key: "evaluation", Label: "评估", Count: dashboardCount(h.db, &EvaluationRun{}, tenant, projectIDs, "", nil), Blocked: dashboardCount(h.db, &EvaluationRun{}, tenant, projectIDs, "(status = ? OR gate_decision = ?)", "FAILED", "FAILED"), Route: "/ai-platform/evaluation"},
		{Key: "deployment", Label: "部署", Count: dashboardCount(h.db, &Deployment{}, tenant, projectIDs, "", nil), Blocked: dashboardCount(h.db, &Deployment{}, tenant, projectIDs, "status IN ?", []string{"FAILED", "STOPPED"}), Route: "/ai-platform/deployments"},
	}

	todos := make([]dashboardTodo, 0, 30)
	seenTodo := map[string]struct{}{}
	addTodo := func(todo dashboardTodo) {
		if len(todos) >= 30 {
			return
		}
		if _, exists := seenTodo[todo.Key]; exists {
			return
		}
		seenTodo[todo.Key] = struct{}{}
		todos = append(todos, todo)
	}
	var annotationTasks []AnnotationTask
	h.db.Where("tenant_id = ? AND project_id IN ? AND status IN ?", tenant, projectIDs,
		[]AnnotationStatus{AnnotationReady, AnnotationAnnotating, AnnotationReviewing, AnnotationRejected, AnnotationFailed}).
		Order("updated_at DESC").Limit(100).Find(&annotationTasks)
	for _, task := range annotationTasks {
		_, annotator := parseDashboardIDs(task.AnnotatorIDs)[user]
		_, reviewer := parseDashboardIDs(task.ReviewerIDs)[user]
		if (task.Status == AnnotationReady || task.Status == AnnotationAnnotating || task.Status == AnnotationRejected) && annotator {
			addTodo(dashboardTodo{Key: fmt.Sprintf("annotation:%d", task.ID), Kind: "ANNOTATION", Title: task.Name, Reason: "待标注或修订", ResourceType: "ANNOTATION_TASK", ResourceID: task.ID, ProjectID: task.ProjectID, Priority: "NORMAL", Route: dashboardRoute("ANNOTATION_TASK"), CreatedAt: task.UpdatedAt.Format(time.RFC3339)})
		}
		if task.Status == AnnotationReviewing && reviewer {
			addTodo(dashboardTodo{Key: fmt.Sprintf("review:%d", task.ID), Kind: "REVIEW", Title: task.Name, Reason: "待审核", ResourceType: "ANNOTATION_TASK", ResourceID: task.ID, ProjectID: task.ProjectID, Priority: "HIGH", Route: dashboardRoute("ANNOTATION_TASK"), CreatedAt: task.UpdatedAt.Format(time.RFC3339)})
		}
	}
	var approvals []ApprovalRequest
	h.db.Where("tenant_id = ? AND project_id IN ? AND status IN ?", tenant, projectIDs, []string{"PENDING", "IN_REVIEW"}).
		Order("updated_at DESC").Limit(30).Find(&approvals)
	for _, request := range approvals {
		addTodo(dashboardTodo{Key: fmt.Sprintf("approval:%d", request.ID), Kind: "APPROVAL", Title: fmt.Sprintf("模型审批 #%d", request.ID), Reason: fmt.Sprintf("等待第 %d/%d 步决策", request.CurrentStep, request.TotalSteps), ResourceType: "APPROVAL_REQUEST", ResourceID: request.ID, ProjectID: request.ProjectID, Priority: "HIGH", Route: dashboardRoute("APPROVAL_REQUEST"), CreatedAt: request.UpdatedAt.Format(time.RFC3339)})
	}
	var failedJobs []PlatformJob
	h.db.Where("tenant_id = ? AND project_id IN ? AND status = ?", tenant, projectIDs, JobFailed).
		Order("updated_at DESC").Limit(10).Find(&failedJobs)
	for _, job := range failedJobs {
		reason := strings.TrimSpace(job.ErrorMessage)
		if reason == "" {
			reason = "任务失败，请查看日志与修复建议"
		}
		addTodo(dashboardTodo{Key: fmt.Sprintf("job:%d", job.ID), Kind: "FAILED_JOB", Title: fmt.Sprintf("%s #%d", job.JobType, job.ID), Reason: reason, ResourceType: job.ResourceType, ResourceID: job.ResourceID, ProjectID: job.ProjectID, Priority: "URGENT", Route: dashboardRoute(job.ResourceType), CreatedAt: job.UpdatedAt.Format(time.RFC3339)})
	}
	var alerts []AlertEvent
	h.db.Where("tenant_id = ? AND project_id IN ? AND status IN ?", tenant, projectIDs, []string{"OPEN", "ACKNOWLEDGED"}).
		Order("created_at DESC").Limit(20).Find(&alerts)
	for _, alert := range alerts {
		addTodo(dashboardTodo{Key: fmt.Sprintf("alert:%d", alert.ID), Kind: "ALERT", Title: fmt.Sprintf("部署告警 #%d", alert.ID), Reason: alert.Message, ResourceType: "ALERT_EVENT", ResourceID: alert.ID, ProjectID: alert.ProjectID, Priority: "URGENT", Route: dashboardRoute("ALERT_EVENT"), CreatedAt: alert.CreatedAt.Format(time.RFC3339)})
	}
	sort.SliceStable(todos, func(i, j int) bool {
		rank := map[string]int{"URGENT": 0, "HIGH": 1, "NORMAL": 2}
		return rank[todos[i].Priority] < rank[todos[j].Priority]
	})

	var recentRows []PlatformJob
	h.db.Where("tenant_id = ? AND project_id IN ?", tenant, projectIDs).Order("created_at DESC").Limit(20).Find(&recentRows)
	recentJobs := make([]dashboardJob, 0, len(recentRows))
	now := time.Now()
	for _, job := range recentRows {
		start, end := job.CreatedAt, now
		if job.StartedAt != nil {
			start = *job.StartedAt
		}
		if job.FinishedAt != nil {
			end = *job.FinishedAt
		}
		actions := []string{"VIEW"}
		if job.Status == JobPending || job.Status == JobQueued || job.Status == JobRunning {
			actions = append(actions, "CANCEL")
		}
		if job.Status == JobFailed && job.RetryCount < job.MaxRetries {
			actions = append(actions, "RETRY")
		}
		duration := int64(end.Sub(start).Seconds())
		if duration < 0 {
			duration = 0
		}
		recentJobs = append(recentJobs, dashboardJob{ID: job.ID, ProjectID: job.ProjectID, JobType: job.JobType, ResourceType: job.ResourceType, ResourceID: job.ResourceID, Status: job.Status, Stage: job.Stage, Progress: job.Progress, DurationSec: duration, ErrorMessage: job.ErrorMessage, Remediation: job.Remediation, Actions: actions, Route: dashboardRoute(job.ResourceType), CreatedAt: job.CreatedAt.Format(time.RFC3339)})
	}

	services := make([]dashboardService, 0, 8)
	dbStatus, dbDetail := "UP", "连接正常"
	if sqlDB, err := h.db.DB(); err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
		dbStatus, dbDetail = "DOWN", "数据库连接失败"
	}
	services = append(services, dashboardService{Name: "平台数据库", Provider: "DATABASE", Status: dbStatus, Detail: dbDetail})
	storageStatus, storageDetail := "UP", "对象存储客户端就绪"
	if h.storageError != nil {
		storageStatus, storageDetail = "DOWN", h.storageError.Error()
	}
	services = append(services, dashboardService{Name: "MinIO 对象存储", Provider: "MINIO", Status: storageStatus, Detail: storageDetail})
	var node ComputeNode
	nodeStatus, nodeDetail, nodeChecked := "DOWN", "尚未收到本机运行器心跳", ""
	if h.db.Where("tenant_id = ? AND node_key LIKE ?", tenant, "local-%").Order("last_heartbeat_at DESC").First(&node).Error == nil {
		if time.Since(node.LastHeartbeatAt) <= 2*time.Minute {
			nodeStatus = "UP"
		} else {
			nodeStatus = "DEGRADED"
		}
		nodeDetail = fmt.Sprintf("%s · %s · GPU %.0f%%", node.Name, node.GPUModel, node.GPUUtilization)
		nodeChecked = node.LastHeartbeatAt.Format(time.RFC3339)
	}
	services = append(services, dashboardService{Name: "LocalDocker Runner", Provider: "LOCAL_DOCKER", Status: nodeStatus, Detail: nodeDetail, LastChecked: nodeChecked})
	var integrations []IntegrationInstance
	h.db.Where("tenant_id = ?", tenant).Order("provider_type,name").Find(&integrations)
	presentProviders := map[string]struct{}{}
	for _, integration := range integrations {
		presentProviders[strings.ToUpper(integration.ProviderType)] = struct{}{}
		status := strings.ToUpper(integration.Status)
		if status == "HEALTHY" || status == "CONNECTED" || status == "UP" {
			status = "UP"
		} else if status == "" {
			status = "UNKNOWN"
		}
		detail := integration.Version
		if integration.LastError != "" {
			detail = integration.LastError
		}
		lastChecked := ""
		if integration.LastCheckedAt != nil {
			lastChecked = integration.LastCheckedAt.Format(time.RFC3339)
		}
		services = append(services, dashboardService{Name: integration.Name, Provider: integration.ProviderType, Status: status, Detail: detail, LastChecked: lastChecked})
	}
	if _, exists := presentProviders["FIFTYONE"]; !exists {
		services = append(services, probeDashboardService(c, "FiftyOne 数据工作台", "FIFTYONE", config.Load().FiftyOneAPIURL))
	}

	var audits []AuditEvent
	h.db.Where("tenant_id = ? AND project_id IN ? AND action NOT IN ?", tenant, projectIDs, []string{"READ", "GET", "LIST", "OPEN", "WORKBENCH_OPEN"}).
		Order("created_at DESC").Limit(30).Find(&audits)
	activities := make([]dashboardActivity, 0, len(audits))
	for _, event := range audits {
		activities = append(activities, dashboardActivity{ID: event.ID, Action: event.Action, ResourceType: event.ResourceType, ResourceID: event.ResourceID, ProjectID: event.ProjectID, ActorUserID: event.ActorUserID, CreatedAt: event.CreatedAt.Format(time.RFC3339)})
	}

	frozenVersions := dashboardCount(h.db, &DatasetVersion{}, tenant, projectIDs, "status = ?", DatasetVersionFrozen)
	models := dashboardCount(h.db, &ModelVersion{}, tenant, projectIDs, "", nil)
	deployments := dashboardCount(h.db, &Deployment{}, tenant, projectIDs, "status = ?", "RUNNING")
	gpuHours := dashboardGPUHours(h.db, tenant, projectIDs)
	var storageBytes int64
	h.db.Model(&Asset{}).Where("tenant_id = ? AND project_id IN ? AND status <> ?", tenant, projectIDs, AssetPurged).Select("COALESCE(SUM(size),0)").Scan(&storageBytes)
	var quotas []ProjectQuota
	h.db.Where("tenant_id = ? AND project_id IN ?", tenant, projectIDs).Find(&quotas)
	resourcePayload := gin.H{"visibility": "QUOTA_ONLY", "quotas": quotas, "storageBytes": storageBytes, "gpuHours": gpuHours}
	if h.userHasSystemRole(tenant, user, "OPS", "PROJECT_OWNER") {
		var nodes []ComputeNode
		h.db.Where("tenant_id = ?", tenant).Order("name").Find(&nodes)
		resourcePayload["visibility"] = "DETAIL"
		resourcePayload["nodes"] = nodes
	}
	httpx.OK(c, gin.H{
		"scope":     gin.H{"projectId": selectedID, "projects": projectOptions},
		"metrics":   gin.H{"projects": len(projectIDs), "datasetVersions": frozenVersions, "activeJobs": running, "models": models, "deployments": deployments, "gpuHours": gpuHours},
		"jobs":      gin.H{"running": running, "failed": failed, "pending": pending},
		"lifecycle": lifecycle, "todos": todos, "recentJobs": recentJobs,
		"services": services, "activities": activities, "resources": resourcePayload,
	})
}
