package visionai

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/modules/system"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

type projectRequest struct {
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	AIDomain          string  `json:"aiDomain"`
	TaskType          string  `json:"taskType"`
	OwnerUserID       uint64  `json:"ownerUserId"`
	DefaultProvider   string  `json:"defaultProvider"`
	StorageBucket     string  `json:"storageBucket"`
	MaxConcurrentJobs int     `json:"maxConcurrentJobs"`
	MonthlyGPUHours   float64 `json:"monthlyGpuHours"`
	StorageBytes      int64   `json:"storageBytes"`
}

type memberRequest struct {
	UserID uint64 `json:"userId"`
}

type configRequest struct {
	StorageConfig  map[string]any    `json:"storageConfig"`
	ProviderConfig map[string]any    `json:"providerConfig"`
	SecretRefs     map[string]string `json:"secretRefs"`
	ChangeReason   string            `json:"changeReason"`
}

type projectStatusRequest struct {
	Status ProjectStatus `json:"status"`
}

func projectID(c *gin.Context) uint64 {
	id, _ := strconv.ParseUint(c.Param("projectId"), 10, 64)
	if id == 0 {
		id, _ = strconv.ParseUint(c.Param("id"), 10, 64)
	}
	return id
}

func (h *Handler) projectAccess(c *gin.Context, write bool) (Project, bool) {
	var project Project
	id := projectID(c)
	if id == 0 {
		id, _ = strconv.ParseUint(c.Param("id"), 10, 64)
	}
	if h.db == nil || h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&project).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "项目不存在")
		return project, false
	}
	var member ProjectMember
	if h.db.Where("tenant_id = ? AND project_id = ? AND user_id = ?", tenantID(c), project.ID, c.GetUint64("user_id")).First(&member).Error != nil {
		httpx.Fail(c, http.StatusForbidden, 403, "无权访问该项目")
		return project, false
	}
	if write && project.Status == ProjectArchived {
		httpx.Fail(c, http.StatusConflict, 409, "归档项目只读")
		return project, false
	}
	return project, true
}

func (h *Handler) requireProjectRole(c *gin.Context, project Project, allowed ...string) bool {
	if project.OwnerUserID == c.GetUint64("user_id") {
		return true
	}
	var member ProjectMember
	if h.db.Where("tenant_id = ? AND project_id = ? AND user_id = ?", project.TenantID, project.ID, c.GetUint64("user_id")).First(&member).Error != nil {
		httpx.Fail(c, http.StatusForbidden, 403, "无权执行该项目操作")
		return false
	}
	if h.userHasSystemRole(project.TenantID, member.UserID, allowed...) {
		return true
	}
	httpx.Fail(c, http.StatusForbidden, 403, "当前项目角色无权执行该操作")
	return false
}

func normalizedRoleCodes(codes []string) []string {
	result := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.ToUpper(strings.TrimSpace(code))
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	return result
}

func (h *Handler) userHasSystemRole(tenantID, userID uint64, allowed ...string) bool {
	codes := normalizedRoleCodes(allowed)
	if len(codes) == 0 {
		return false
	}
	var count int64
	h.db.Table("roles AS r").
		Joins("JOIN user_roles AS ur ON ur.role_id = r.id").
		Where("r.tenant_id = ? AND ur.user_id = ? AND r.status = ? AND UPPER(r.code) IN ?", tenantID, userID, 0, codes).
		Count(&count)
	return count > 0
}

func (h *Handler) systemRolesForUser(tenantID, userID uint64) []system.Role {
	roles := make([]system.Role, 0)
	h.db.Table("roles AS r").
		Select("r.*").
		Joins("JOIN user_roles AS ur ON ur.role_id = r.id").
		Where("r.tenant_id = ? AND ur.user_id = ?", tenantID, userID).
		Order("r.sort,r.id").
		Find(&roles)
	return roles
}

// ProjectPage godoc
// @Summary Page projects visible to current member
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects [get]
func (h *Handler) ProjectPage(c *gin.Context) {
	pageNo, pageSize := page(c)
	query := h.db.Model(&Project{}).
		Joins("JOIN ai_project_member m ON m.project_id = ai_project.id AND m.tenant_id = ai_project.tenant_id").
		Where("ai_project.tenant_id = ? AND m.user_id = ?", tenantID(c), c.GetUint64("user_id"))
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		query = query.Where("ai_project.name LIKE ? OR ai_project.code LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("ai_project.status = ?", status)
	}
	var total int64
	query.Count(&total)
	var rows []Project
	query.Order("ai_project.id DESC").Offset((pageNo - 1) * pageSize).Limit(pageSize).Find(&rows)
	httpx.OK(c, gin.H{"list": rows, "total": total})
}

// ProjectCreate godoc
// @Summary Create project
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects [post]
func (h *Handler) ProjectCreate(c *gin.Context) {
	var req projectRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, http.StatusBadRequest, 400, "项目编码和名称必填")
		return
	}
	if req.OwnerUserID == 0 {
		req.OwnerUserID = c.GetUint64("user_id")
	}
	if h.db.Where("tenant_id = ? AND id = ? AND status = ?", tenantID(c), req.OwnerUserID, 0).
		First(&system.AdminUser{}).Error != nil {
		httpx.Fail(c, http.StatusBadRequest, 400, "项目负责人不属于当前租户或已停用")
		return
	}
	req.AIDomain = strings.ToUpper(strings.TrimSpace(req.AIDomain))
	if req.AIDomain == "" {
		req.AIDomain = "CV"
	}
	req.TaskType = strings.ToUpper(strings.TrimSpace(req.TaskType))
	if req.TaskType == "" {
		req.TaskType = "CV_DETECTION"
	}
	req.DefaultProvider = strings.ToUpper(strings.TrimSpace(req.DefaultProvider))
	if req.DefaultProvider == "" {
		req.DefaultProvider = "LOCAL_DOCKER"
	}
	req.StorageBucket = strings.TrimSpace(req.StorageBucket)
	if req.StorageBucket == "" {
		req.StorageBucket = "visionai-assets"
	}
	if req.MaxConcurrentJobs < 1 {
		req.MaxConcurrentJobs = 2
	}
	if req.MonthlyGPUHours <= 0 {
		req.MonthlyGPUHours = 100
	}
	if req.StorageBytes < 1 {
		req.StorageBytes = 100 * 1024 * 1024 * 1024
	}
	project := Project{
		TenantID: tenantID(c), Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description), Status: ProjectDraft,
		AIDomain: req.AIDomain, TaskType: req.TaskType, DefaultProvider: req.DefaultProvider,
		StorageBucket: req.StorageBucket, OwnerUserID: req.OwnerUserID, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		if err := tx.Create(&ProjectMember{TenantID: project.TenantID, ProjectID: project.ID, UserID: project.OwnerUserID, LegacyRoles: "[]", CreatedBy: project.CreatedBy}).Error; err != nil {
			return err
		}
		if project.OwnerUserID != project.CreatedBy {
			if err := tx.Create(&ProjectMember{TenantID: project.TenantID, ProjectID: project.ID, UserID: project.CreatedBy, LegacyRoles: "[]", CreatedBy: project.CreatedBy}).Error; err != nil {
				return err
			}
		}
		storageConfig, _ := json.Marshal(gin.H{"bucket": project.StorageBucket, "recycleRetentionDays": 30})
		providerConfig, _ := json.Marshal(gin.H{"trainingProvider": project.DefaultProvider})
		if err := tx.Create(&ProjectConfig{
			TenantID: project.TenantID, ProjectID: project.ID,
			StorageConfig: string(storageConfig), ProviderConfig: string(providerConfig), SecretRefs: "{}",
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&ProjectQuota{
			TenantID: project.TenantID, ProjectID: project.ID,
			MaxConcurrentJobs: req.MaxConcurrentJobs, MonthlyGPUHours: req.MonthlyGPUHours,
			StorageBytes: req.StorageBytes, UpdatedBy: project.CreatedBy,
		}).Error; err != nil {
			return err
		}
		if err := appendAudit(tx, c, project.ID, "PROJECT_CREATED", "PROJECT", project.ID, nil, project); err != nil {
			return err
		}
		payload, _ := json.Marshal(gin.H{"projectId": project.ID, "code": project.Code})
		return tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "project.created.v1",
			AggregateType: "PROJECT", AggregateID: project.ID, Payload: string(payload), Status: outboxNew,
		}).Error
	})
	if err != nil {
		httpx.Fail(c, http.StatusConflict, 409, "项目编码已存在或保存失败")
		return
	}
	httpx.OK(c, project)
}

// ProjectStatusUpdate godoc
// @Summary Activate or suspend a project
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/status [put]
func (h *Handler) ProjectStatusUpdate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER") {
		return
	}
	var req projectStatusRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "项目状态格式错误")
		return
	}
	valid := (project.Status == ProjectDraft && req.Status == ProjectActive) ||
		(project.Status == ProjectActive && req.Status == ProjectSuspended) ||
		(project.Status == ProjectSuspended && req.Status == ProjectActive)
	if !valid {
		httpx.Fail(c, 409, 409, "不允许的项目状态流转")
		return
	}
	beforeStatus := project.Status
	project.Status = req.Status
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&project).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "PROJECT_STATUS_CHANGED", "PROJECT", project.ID, gin.H{"status": beforeStatus}, gin.H{"status": project.Status})
	}); err != nil {
		httpx.Fail(c, 500, 500, "项目状态更新失败")
		return
	}
	httpx.OK(c, project)
}

// ProjectGet godoc
// @Summary Get project
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id} [get]
func (h *Handler) ProjectGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if ok {
		httpx.OK(c, project)
	}
}

// ProjectOverview godoc
// @Summary Get project lifecycle counts, risks and business timeline
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/overview [get]
func (h *Handler) ProjectOverview(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	count := func(model any, query string, args ...any) int64 {
		var value int64
		h.db.Model(model).Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).
			Where(query, args...).Count(&value)
		return value
	}
	metrics := gin.H{
		"assets":          count(&Asset{}, "status <> ?", AssetPurged),
		"annotations":     count(&AnnotationTask{}, "1 = 1"),
		"datasetVersions": count(&DatasetVersion{}, "1 = 1"),
		"trainingRuns":    count(&TrainingRun{}, "1 = 1"),
		"modelVersions":   count(&ModelVersion{}, "1 = 1"),
		"deployments":     count(&Deployment{}, "1 = 1"),
	}
	risks := make([]gin.H, 0)
	failedJobs := count(&PlatformJob{}, "status = ?", JobFailed)
	if failedJobs > 0 {
		risks = append(risks, gin.H{
			"code": "FAILED_JOBS", "severity": "HIGH", "count": failedJobs,
			"message": "存在失败任务需要诊断或重试", "route": "/ai-platform/dashboard",
		})
	}
	openAlerts := count(&AlertEvent{}, "status IN ?", []string{"OPEN", "ACKNOWLEDGED"})
	if openAlerts > 0 {
		risks = append(risks, gin.H{
			"code": "OPEN_ALERTS", "severity": "HIGH", "count": openAlerts,
			"message": "存在未处置的生产告警", "route": "/ai-platform/deployments",
		})
	}
	pendingApprovals := count(&ApprovalRequest{}, "status IN ?", []string{"PENDING", "IN_REVIEW"})
	if pendingApprovals > 0 {
		risks = append(risks, gin.H{
			"code": "PENDING_APPROVALS", "severity": "MEDIUM", "count": pendingApprovals,
			"message": "存在待处理的模型审批", "route": "/ai-platform/models",
		})
	}
	var timeline []AuditEvent
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).
		Where("action NOT IN ?", []string{"WORKBENCH_OPENED", "DASHBOARD_VIEWED"}).
		Order("id DESC").Limit(30).Find(&timeline)
	var quota ProjectQuota
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).First(&quota)
	var projectConfig ProjectConfig
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).First(&projectConfig)
	httpx.OK(c, gin.H{
		"project": project, "metrics": metrics, "risks": risks, "timeline": timeline,
		"quota": quota, "config": configView(projectConfig),
	})
}

// ProjectUpdate godoc
// @Summary Update project
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id} [put]
func (h *Handler) ProjectUpdate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok {
		return
	}
	if !h.requireProjectRole(c, project, "PROJECT_OWNER") {
		return
	}
	var req projectRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, http.StatusBadRequest, 400, "项目名称必填")
		return
	}
	before := project
	project.Name, project.Description = strings.TrimSpace(req.Name), strings.TrimSpace(req.Description)
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&project).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "PROJECT_UPDATED", "PROJECT", project.ID, before, project)
	}); err != nil {
		httpx.Fail(c, 500, 500, "项目保存失败")
		return
	}
	httpx.OK(c, project)
}

// ProjectArchive godoc
// @Summary Archive project after lifecycle guard checks
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/archive [post]
func (h *Handler) ProjectArchive(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok {
		return
	}
	if !h.requireProjectRole(c, project, "PROJECT_OWNER") {
		return
	}
	var active int64
	h.db.Model(&PlatformJob{}).Where("tenant_id = ? AND project_id = ? AND status IN ?", project.TenantID, project.ID, []JobStatus{JobPending, JobQueued, JobRunning}).Count(&active)
	if active > 0 {
		httpx.Fail(c, http.StatusConflict, 409, "项目存在运行中任务，无法归档")
		return
	}
	var activeDeployments, unfinishedApprovals int64
	h.db.Model(&Deployment{}).Where(
		"tenant_id = ? AND project_id = ? AND status IN ?",
		project.TenantID, project.ID, []string{"DEPLOYING", "RUNNING"},
	).Count(&activeDeployments)
	h.db.Model(&ApprovalRequest{}).Where(
		"tenant_id = ? AND project_id = ? AND status IN ?",
		project.TenantID, project.ID, []string{"PENDING", "IN_REVIEW"},
	).Count(&unfinishedApprovals)
	if activeDeployments > 0 || unfinishedApprovals > 0 {
		httpx.Fail(c, http.StatusConflict, 409, "项目存在运行中部署或未完成审批，无法归档")
		return
	}
	now := time.Now()
	beforeStatus := project.Status
	project.Status, project.ArchivedAt = ProjectArchived, &now
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&project).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "PROJECT_ARCHIVED", "PROJECT", project.ID, gin.H{"status": beforeStatus}, gin.H{"status": project.Status})
	}); err != nil {
		httpx.Fail(c, 500, 500, "项目归档失败")
		return
	}
	httpx.OK(c, true)
}

// ProjectClone godoc
// @Summary Clone non-sensitive project configuration
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/clone [post]
func (h *Handler) ProjectClone(c *gin.Context) {
	source, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	if !h.requireProjectRole(c, source, "PROJECT_OWNER") {
		return
	}
	var req projectRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, 400, 400, "新项目编码和名称必填")
		return
	}
	var sourceConfig ProjectConfig
	h.db.Where("tenant_id = ? AND project_id = ?", source.TenantID, source.ID).First(&sourceConfig)
	clone := Project{
		TenantID: source.TenantID, Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name),
		Description: req.Description, AIDomain: source.AIDomain, TaskType: source.TaskType,
		DefaultProvider: source.DefaultProvider, StorageBucket: source.StorageBucket,
		Status: ProjectActive, OwnerUserID: c.GetUint64("user_id"), CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&clone).Error; err != nil {
			return err
		}
		if err := tx.Create(&ProjectMember{TenantID: clone.TenantID, ProjectID: clone.ID, UserID: clone.OwnerUserID, LegacyRoles: "[]", CreatedBy: clone.CreatedBy}).Error; err != nil {
			return err
		}
		if err := tx.Create(&ProjectConfig{TenantID: clone.TenantID, ProjectID: clone.ID, StorageConfig: sourceConfig.StorageConfig, ProviderConfig: sourceConfig.ProviderConfig, SecretRefs: "{}"}).Error; err != nil {
			return err
		}
		var sourceQuota ProjectQuota
		if tx.Where("tenant_id = ? AND project_id = ?", source.TenantID, source.ID).First(&sourceQuota).Error == nil {
			if err := tx.Create(&ProjectQuota{
				TenantID: clone.TenantID, ProjectID: clone.ID,
				MaxConcurrentJobs: sourceQuota.MaxConcurrentJobs, MonthlyGPUHours: sourceQuota.MonthlyGPUHours,
				StorageBytes: sourceQuota.StorageBytes, UpdatedBy: clone.CreatedBy,
			}).Error; err != nil {
				return err
			}
		}
		if err := cloneProjectOntologies(tx, source, clone, clone.CreatedBy); err != nil {
			return err
		}
		if err := cloneProjectTrainingTemplates(tx, source, clone, clone.CreatedBy); err != nil {
			return err
		}
		return appendAudit(tx, c, clone.ID, "PROJECT_CLONED", "PROJECT", clone.ID, gin.H{"sourceProjectId": source.ID}, clone)
	})
	if err != nil {
		httpx.Fail(c, 409, 409, "项目复制失败")
		return
	}
	httpx.OK(c, clone)
}

func cloneProjectTrainingTemplates(tx *gorm.DB, source, target Project, createdBy uint64) error {
	var templates []TrainingTemplate
	if err := tx.Where("tenant_id = ? AND project_id = ?", source.TenantID, source.ID).Order("id").Find(&templates).Error; err != nil {
		return err
	}
	for _, sourceTemplate := range templates {
		targetTemplate := TrainingTemplate{
			TenantID: target.TenantID, ProjectID: target.ID, Name: sourceTemplate.Name,
			AIType: sourceTemplate.AIType, Description: sourceTemplate.Description, CreatedBy: createdBy,
		}
		if err := tx.Create(&targetTemplate).Error; err != nil {
			return err
		}
		var versions []TrainingTemplateVersion
		if err := tx.Where("tenant_id = ? AND project_id = ? AND template_id = ?", source.TenantID, source.ID, sourceTemplate.ID).
			Order("version_no").Find(&versions).Error; err != nil {
			return err
		}
		for _, sourceVersion := range versions {
			targetVersion := TrainingTemplateVersion{
				TenantID: target.TenantID, ProjectID: target.ID, TemplateID: targetTemplate.ID,
				VersionNo: sourceVersion.VersionNo, SemanticVersion: sourceVersion.SemanticVersion,
				Trainer: sourceVersion.Trainer, ImageRef: sourceVersion.ImageRef, Entrypoint: sourceVersion.Entrypoint,
				ParameterSchema: sourceVersion.ParameterSchema, OutputProtocol: sourceVersion.OutputProtocol,
				ResourceRequirements: sourceVersion.ResourceRequirements, Compatibility: sourceVersion.Compatibility,
				LicensePolicy: sourceVersion.LicensePolicy, Published: sourceVersion.Published,
				SmokeStatus: sourceVersion.SmokeStatus, SmokeReport: sourceVersion.SmokeReport,
				CreatedBy: createdBy,
			}
			if sourceVersion.Published {
				targetVersion.PublishedBy = createdBy
				targetVersion.PublishedAt = sourceVersion.PublishedAt
			}
			if err := tx.Create(&targetVersion).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// ProjectMemberList godoc
// @Summary List project members
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{projectId}/members [get]
func (h *Handler) ProjectMemberList(c *gin.Context) {
	if _, ok := h.projectAccess(c, false); !ok {
		return
	}
	var rows []ProjectMember
	h.db.Where("tenant_id = ? AND project_id = ?", tenantID(c), projectID(c)).Order("id").Find(&rows)
	result := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		var user system.AdminUser
		h.db.Where("tenant_id = ? AND id = ?", row.TenantID, row.UserID).First(&user)
		roles := h.systemRolesForUser(row.TenantID, row.UserID)
		result = append(result, gin.H{
			"id": row.ID, "userId": row.UserID, "username": user.Username, "nickname": user.Nickname,
			"userStatus": user.Status, "roles": roles, "createTime": row.CreatedAt,
		})
	}
	httpx.OK(c, result)
}

// ProjectMemberUpsert godoc
// @Summary Add a project member whose roles are managed by System Management
// @Tags VisionAI Project
// @Security BearerAuth
// @Param request body memberRequest true "Project member"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{projectId}/members [put]
func (h *Handler) ProjectMemberUpsert(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok {
		return
	}
	if !h.requireProjectRole(c, project, "PROJECT_OWNER") {
		return
	}
	var req memberRequest
	if c.ShouldBindJSON(&req) != nil || req.UserID == 0 {
		httpx.Fail(c, 400, 400, "项目成员用户必填")
		return
	}
	if h.db.Where("tenant_id = ? AND id = ? AND status = ?", tenantID(c), req.UserID, 0).First(&system.AdminUser{}).Error != nil {
		httpx.Fail(c, 400, 400, "成员用户不属于当前租户或已停用")
		return
	}
	row := ProjectMember{TenantID: tenantID(c), ProjectID: projectID(c), UserID: req.UserID}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND project_id = ? AND user_id = ?", row.TenantID, row.ProjectID, row.UserID).
			Attrs(ProjectMember{LegacyRoles: "[]", CreatedBy: c.GetUint64("user_id")}).
			FirstOrCreate(&row).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "PROJECT_MEMBER_UPSERTED", "PROJECT_MEMBER", req.UserID, nil, gin.H{
			"userId": req.UserID, "roleSource": "SYSTEM_MANAGEMENT",
		})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "成员保存失败")
		return
	}
	httpx.OK(c, true)
}

// ProjectMemberDelete godoc
// @Summary Remove project member
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{projectId}/members/{userId} [delete]
func (h *Handler) ProjectMemberDelete(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok {
		return
	}
	if !h.requireProjectRole(c, project, "PROJECT_OWNER") {
		return
	}
	userID, _ := strconv.ParseUint(c.Param("userId"), 10, 64)
	if userID == project.OwnerUserID {
		httpx.Fail(c, 409, 409, "不能移除项目负责人")
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND project_id = ? AND user_id = ?", tenantID(c), project.ID, userID).Delete(&ProjectMember{}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "PROJECT_MEMBER_REMOVED", "PROJECT_MEMBER", userID, gin.H{"userId": userID}, nil)
	}); err != nil {
		httpx.Fail(c, 500, 500, "成员移除失败")
		return
	}
	httpx.OK(c, true)
}

// ProjectConfigGet godoc
// @Summary Get project provider configuration
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{projectId}/config [get]
func (h *Handler) ProjectConfigGet(c *gin.Context) {
	if _, ok := h.projectAccess(c, false); !ok {
		return
	}
	var row ProjectConfig
	if h.db.Where("tenant_id = ? AND project_id = ?", tenantID(c), projectID(c)).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "项目配置不存在")
		return
	}
	httpx.OK(c, configView(row))
}

func configView(row ProjectConfig) gin.H {
	var storage, providers map[string]any
	var secretRefs map[string]string
	_ = json.Unmarshal([]byte(row.StorageConfig), &storage)
	_ = json.Unmarshal([]byte(row.ProviderConfig), &providers)
	_ = json.Unmarshal([]byte(row.SecretRefs), &secretRefs)
	return gin.H{"storageConfig": storage, "providerConfig": providers, "secretRefs": secretRefs, "updateTime": row.UpdatedAt}
}

func containsInlineSecret(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if strings.Contains(normalized, "password") || strings.Contains(normalized, "secret") ||
				strings.Contains(normalized, "token") || strings.Contains(normalized, "apikey") ||
				strings.Contains(normalized, "credential") {
				return true
			}
			if containsInlineSecret(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsInlineSecret(nested) {
				return true
			}
		}
	}
	return false
}

// ProjectConfigUpdate godoc
// @Summary Update project provider configuration using secret references
// @Tags VisionAI Project
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{projectId}/config [put]
func (h *Handler) ProjectConfigUpdate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok {
		return
	}
	if !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "OPS") {
		return
	}
	var req configRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "配置格式错误")
		return
	}
	if strings.TrimSpace(req.ChangeReason) == "" {
		httpx.Fail(c, 400, 400, "配置变更原因必填")
		return
	}
	if containsInlineSecret(req.StorageConfig) || containsInlineSecret(req.ProviderConfig) {
		httpx.Fail(c, 400, 400, "敏感凭据不能明文保存，请使用 secretRefs")
		return
	}
	storage, _ := json.Marshal(req.StorageConfig)
	providers, _ := json.Marshal(req.ProviderConfig)
	secretRefs, _ := json.Marshal(req.SecretRefs)
	var row ProjectConfig
	if h.db.Where("tenant_id = ? AND project_id = ?", tenantID(c), projectID(c)).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "项目配置不存在")
		return
	}
	beforeConfig := configView(row)
	row.StorageConfig, row.ProviderConfig, row.SecretRefs = string(storage), string(providers), string(secretRefs)
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "PROJECT_CONFIG_UPDATED", "PROJECT_CONFIG", row.ID, beforeConfig, gin.H{
			"config": configView(row), "changeReason": strings.TrimSpace(req.ChangeReason),
		})
	}); err != nil {
		httpx.Fail(c, 500, 500, "项目配置保存失败")
		return
	}
	httpx.OK(c, configView(row))
}
