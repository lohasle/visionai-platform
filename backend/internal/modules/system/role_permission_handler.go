package system

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
)

type RoleSaveRequest struct {
	ID               uint64   `json:"id"`
	Name             string   `json:"name" binding:"required"`
	Code             string   `json:"code" binding:"required"`
	Sort             int      `json:"sort"`
	Status           int      `json:"status"`
	DataScope        int      `json:"dataScope"`
	DataScopeDeptIDs []uint64 `json:"dataScopeDeptIds"`
	Remark           string   `json:"remark"`
}

type RoleView struct {
	Role
	DataScopeDeptIDs []uint64 `json:"dataScopeDeptIds"`
}

func roleView(row Role) RoleView {
	view := RoleView{Role: row, DataScopeDeptIDs: []uint64{}}
	_ = json.Unmarshal([]byte(row.DataScopeDeptIDs), &view.DataScopeDeptIDs)
	return view
}

// RolePage godoc
// @Summary Page roles
// @Tags System Role
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/role/page [get]
func (h *Handler) RolePage(c *gin.Context) {
	query := h.service.db.Model(&Role{}).Where("tenant_id = ?", tenantIDFromContext(c))
	if name := strings.TrimSpace(c.Query("name")); name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}
	if code := strings.TrimSpace(c.Query("code")); code != "" {
		query = query.Where("code LIKE ?", "%"+code+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	query.Count(&total)
	pageNo, pageSize := pageParams(c)
	var rows []Role
	query.Order("sort,id").Offset((pageNo - 1) * pageSize).Limit(pageSize).Find(&rows)
	views := make([]RoleView, 0, len(rows))
	for _, row := range rows {
		views = append(views, roleView(row))
	}
	httpx.OK(c, gin.H{"list": views, "total": total})
}

// RoleGet godoc
// @Summary Get a role
// @Tags System Role
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/role/get [get]
func (h *Handler) RoleGet(c *gin.Context) {
	var row Role
	if h.service.db.Where("tenant_id = ? AND id = ?", tenantIDFromContext(c), queryID(c)).First(&row).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "角色不存在")
		return
	}
	httpx.OK(c, roleView(row))
}

func applyRole(row *Role, req RoleSaveRequest) {
	row.Name, row.Code, row.Sort, row.Status, row.DataScope, row.Remark = strings.TrimSpace(req.Name), strings.TrimSpace(req.Code), req.Sort, req.Status, req.DataScope, req.Remark
	if row.DataScope == 0 {
		row.DataScope = 1
	}
	data, _ := json.Marshal(uniqueIDs(req.DataScopeDeptIDs))
	row.DataScopeDeptIDs = string(data)
}

// RoleCreate godoc
// @Summary Create a role
// @Tags System Role
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body RoleSaveRequest true "Role"
// @Success 200 {object} httpx.Response
// @Router /system/role/create [post]
func (h *Handler) RoleCreate(c *gin.Context) {
	var req RoleSaveRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "请求参数错误")
		return
	}
	row := Role{TenantID: tenantIDFromContext(c), Type: 2}
	applyRole(&row, req)
	if h.service.db.Where("tenant_id = ? AND code = ?", row.TenantID, row.Code).First(&Role{}).Error == nil {
		httpx.Fail(c, 409, 409, "角色标识已存在")
		return
	}
	if err := h.service.db.Create(&row).Error; err != nil {
		httpx.Fail(c, 500, 500, "创建角色失败")
		return
	}
	httpx.OK(c, row.ID)
}

// RoleUpdate godoc
// @Summary Update a role
// @Tags System Role
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body RoleSaveRequest true "Role"
// @Success 200 {object} httpx.Response
// @Router /system/role/update [put]
func (h *Handler) RoleUpdate(c *gin.Context) {
	var req RoleSaveRequest
	if c.ShouldBindJSON(&req) != nil || req.ID == 0 {
		httpx.Fail(c, 400, 400, "请求参数错误")
		return
	}
	var row Role
	if h.service.db.Where("tenant_id = ? AND id = ?", tenantIDFromContext(c), req.ID).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "角色不存在")
		return
	}
	if row.Type == 1 && req.Code != row.Code {
		httpx.Fail(c, 400, 400, "系统内置角色标识不可修改")
		return
	}
	applyRole(&row, req)
	if err := h.service.db.Save(&row).Error; err != nil {
		httpx.Fail(c, 409, 409, "角色标识已存在")
		return
	}
	httpx.OK(c, true)
}

// RoleDelete godoc
// @Summary Delete a role
// @Tags System Role
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/role/delete [delete]
func (h *Handler) RoleDelete(c *gin.Context) { h.deleteRoles(c, []uint64{queryID(c)}) }

// RoleDeleteList godoc
// @Summary Delete roles in batch
// @Tags System Role
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/role/delete-list [delete]
func (h *Handler) RoleDeleteList(c *gin.Context) { h.deleteRoles(c, splitIDs(c.Query("ids"))) }

func (h *Handler) deleteRoles(c *gin.Context, ids []uint64) {
	var protected int64
	h.service.db.Model(&Role{}).Where("tenant_id = ? AND id IN ? AND type = ?", tenantIDFromContext(c), ids, 1).Count(&protected)
	if protected > 0 {
		httpx.Fail(c, 400, 400, "系统内置角色不可删除，可在角色管理中停用")
		return
	}
	tx := h.service.db.Begin()
	tx.Where("role_id IN ?", ids).Delete(&RoleMenu{})
	tx.Where("role_id IN ?", ids).Delete(&UserRole{})
	tx.Where("tenant_id = ? AND id IN ?", tenantIDFromContext(c), ids).Delete(&Role{})
	tx.Commit()
	httpx.OK(c, true)
}

// SimpleRoles godoc
// @Summary List enabled roles
// @Tags System Role
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/role/simple-list [get]
func (h *Handler) SimpleRoles(c *gin.Context) {
	var rows []Role
	h.service.db.Where("tenant_id = ? AND status = 0", tenantIDFromContext(c)).Order("sort,id").Find(&rows)
	views := make([]RoleView, 0, len(rows))
	for _, row := range rows {
		views = append(views, roleView(row))
	}
	httpx.OK(c, views)
}

// RoleMenuList godoc
// @Summary List menu IDs assigned to a role
// @Tags System Permission
// @Produce json
// @Security BearerAuth
// @Param roleId query int true "Role ID"
// @Success 200 {object} httpx.Response
// @Router /system/permission/list-role-menus [get]
func (h *Handler) RoleMenuList(c *gin.Context) {
	roleID := queryUint64(c, "roleId")
	if h.service.db.Where("tenant_id = ? AND id = ?", tenantIDFromContext(c), roleID).First(&Role{}).Error != nil {
		httpx.Fail(c, 404, 404, "角色不存在")
		return
	}
	var ids []uint64
	h.service.db.Model(&RoleMenu{}).Where("role_id = ?", roleID).Order("menu_id").Pluck("menu_id", &ids)
	httpx.OK(c, ids)
}

type AssignRoleMenuRequest struct {
	RoleID  uint64   `json:"roleId" binding:"required"`
	MenuIDs []uint64 `json:"menuIds"`
}

// AssignRoleMenu godoc
// @Summary Assign menus to a role
// @Tags System Permission
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body AssignRoleMenuRequest true "Role menu assignment"
// @Success 200 {object} httpx.Response
// @Router /system/permission/assign-role-menu [post]
func (h *Handler) AssignRoleMenu(c *gin.Context) {
	var req AssignRoleMenuRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "请求参数错误")
		return
	}
	var role Role
	if h.service.db.Where("tenant_id = ? AND id = ?", tenantIDFromContext(c), req.RoleID).First(&role).Error != nil {
		httpx.Fail(c, 404, 404, "角色不存在")
		return
	}
	if role.Code == "super_admin" {
		httpx.Fail(c, 400, 400, "超级管理员默认拥有全部权限")
		return
	}
	ids := uniqueIDs(req.MenuIDs)
	if len(ids) > 0 {
		if allowed := h.allowedMenuIDs(tenantIDFromContext(c)); len(allowed) > 0 {
			allowedSet := map[uint64]struct{}{}
			for _, id := range allowed {
				allowedSet[id] = struct{}{}
			}
			for _, id := range ids {
				if _, ok := allowedSet[id]; !ok {
					httpx.Fail(c, 400, 400, "菜单超出租户套餐范围")
					return
				}
			}
		}
		var count int64
		h.service.db.Model(&SystemMenu{}).Where("id IN ?", ids).Count(&count)
		if count != int64(len(ids)) {
			httpx.Fail(c, 400, 400, "包含无效菜单")
			return
		}
	}
	tx := h.service.db.Begin()
	tx.Where("role_id = ?", req.RoleID).Delete(&RoleMenu{})
	for _, id := range ids {
		tx.Create(&RoleMenu{RoleID: req.RoleID, MenuID: id})
	}
	if tx.Commit().Error != nil {
		httpx.Fail(c, 500, 500, "分配菜单失败")
		return
	}
	httpx.OK(c, true)
}

type AssignRoleDataScopeRequest struct {
	RoleID           uint64   `json:"roleId" binding:"required"`
	DataScope        int      `json:"dataScope" binding:"required"`
	DataScopeDeptIDs []uint64 `json:"dataScopeDeptIds"`
}

// AssignRoleDataScope godoc
// @Summary Assign data scope to a role
// @Tags System Permission
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body AssignRoleDataScopeRequest true "Role data scope"
// @Success 200 {object} httpx.Response
// @Router /system/permission/assign-role-data-scope [post]
func (h *Handler) AssignRoleDataScope(c *gin.Context) {
	var req AssignRoleDataScopeRequest
	if c.ShouldBindJSON(&req) != nil || req.DataScope < 1 || req.DataScope > 5 {
		httpx.Fail(c, 400, 400, "请求参数错误")
		return
	}
	var role Role
	if h.service.db.Where("tenant_id = ? AND id = ?", tenantIDFromContext(c), req.RoleID).First(&role).Error != nil {
		httpx.Fail(c, 404, 404, "角色不存在")
		return
	}
	if role.Code == "super_admin" {
		httpx.Fail(c, 400, 400, "超级管理员数据权限不可修改")
		return
	}
	data, _ := json.Marshal(uniqueIDs(req.DataScopeDeptIDs))
	h.service.db.Model(&role).Updates(map[string]any{"data_scope": req.DataScope, "data_scope_dept_ids": string(data)})
	httpx.OK(c, true)
}

// UserRoleList godoc
// @Summary List role IDs assigned to a user
// @Tags System Permission
// @Produce json
// @Security BearerAuth
// @Param userId query int true "User ID"
// @Success 200 {object} httpx.Response
// @Router /system/permission/list-user-roles [get]
func (h *Handler) UserRoleList(c *gin.Context) {
	userID := queryUint64(c, "userId")
	if userID == 0 || h.service.db.Where("tenant_id = ? AND id = ?", tenantIDFromContext(c), userID).First(&AdminUser{}).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "用户不存在")
		return
	}
	var roleIDs []uint64
	h.service.db.Model(&UserRole{}).Where("user_id = ?", userID).Order("role_id").Pluck("role_id", &roleIDs)
	httpx.OK(c, roleIDs)
}

type AssignUserRoleRequest struct {
	UserID  uint64   `json:"userId" binding:"required"`
	RoleIDs []uint64 `json:"roleIds"`
}

// AssignUserRole godoc
// @Summary Assign roles to an operations-console user
// @Tags System Permission
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body AssignUserRoleRequest true "User role assignment"
// @Success 200 {object} httpx.Response
// @Router /system/permission/assign-user-role [post]
func (h *Handler) AssignUserRole(c *gin.Context) {
	var req AssignUserRoleRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, http.StatusBadRequest, 400, "请求参数错误")
		return
	}
	tenantID := tenantIDFromContext(c)
	if h.service.db.Where("tenant_id = ? AND id = ?", tenantID, req.UserID).First(&AdminUser{}).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "用户不存在")
		return
	}
	if len(req.RoleIDs) > 0 {
		var count int64
		h.service.db.Model(&Role{}).Where("tenant_id = ? AND id IN ?", tenantID, req.RoleIDs).Count(&count)
		if count != int64(len(uniqueIDs(req.RoleIDs))) {
			httpx.Fail(c, http.StatusBadRequest, 400, "包含无效角色")
			return
		}
	}
	tx := h.service.db.Begin()
	tx.Where("user_id = ?", req.UserID).Delete(&UserRole{})
	for _, roleID := range uniqueIDs(req.RoleIDs) {
		tx.Create(&UserRole{UserID: req.UserID, RoleID: roleID})
	}
	if tx.Commit().Error != nil {
		httpx.Fail(c, http.StatusInternalServerError, 500, "分配角色失败")
		return
	}
	httpx.OK(c, true)
}

func queryUint64(c *gin.Context, key string) uint64 {
	value, _ := strconv.ParseUint(c.Query(key), 10, 64)
	return value
}

func uniqueIDs(values []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(values))
	result := make([]uint64, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
