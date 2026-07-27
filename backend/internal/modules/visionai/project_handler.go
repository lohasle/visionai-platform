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
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type memberRequest struct {
	UserID uint64 `json:"userId"`
}

type configRequest struct {
	StorageConfig  map[string]any    `json:"storageConfig"`
	ProviderConfig map[string]any    `json:"providerConfig"`
	SecretRefs     map[string]string `json:"secretRefs"`
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
	project := Project{
		TenantID: tenantID(c), Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description), Status: ProjectDraft,
		OwnerUserID: c.GetUint64("user_id"), CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		if err := tx.Create(&ProjectMember{TenantID: project.TenantID, ProjectID: project.ID, UserID: project.OwnerUserID, LegacyRoles: "[]", CreatedBy: project.CreatedBy}).Error; err != nil {
			return err
		}
		if err := tx.Create(&ProjectConfig{TenantID: project.TenantID, ProjectID: project.ID, StorageConfig: "{}", ProviderConfig: "{}", SecretRefs: "{}"}).Error; err != nil {
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
	clone := Project{TenantID: source.TenantID, Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name), Description: req.Description, Status: ProjectActive, OwnerUserID: c.GetUint64("user_id"), CreatedBy: c.GetUint64("user_id")}
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
		if err := cloneProjectOntologies(tx, source, clone, clone.CreatedBy); err != nil {
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
		return appendAudit(tx, c, project.ID, "PROJECT_CONFIG_UPDATED", "PROJECT_CONFIG", row.ID, beforeConfig, configView(row))
	}); err != nil {
		httpx.Fail(c, 500, 500, "项目配置保存失败")
		return
	}
	httpx.OK(c, configView(row))
}
