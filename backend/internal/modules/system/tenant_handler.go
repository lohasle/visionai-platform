package system

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"golang.org/x/crypto/bcrypt"
)

type TenantSaveRequest struct {
	ID            uint64    `json:"id"`
	Name          string    `json:"name" binding:"required"`
	ContactName   string    `json:"contactName"`
	ContactMobile string    `json:"contactMobile"`
	Status        int       `json:"status"`
	Domain        string    `json:"domain"`
	Websites      []string  `json:"websites"`
	Username      string    `json:"username"`
	Password      string    `json:"password"`
	ExpireTime    time.Time `json:"expireTime"`
	AccountCount  int       `json:"accountCount"`
}

type TenantView struct {
	Tenant
	Websites []string `json:"websites"`
}

func tenantView(row Tenant) TenantView {
	websites := []string{}
	for _, value := range strings.Split(row.Domain, ",") {
		if value = strings.TrimSpace(value); value != "" {
			websites = append(websites, value)
		}
	}
	return TenantView{Tenant: row, Websites: websites}
}

// TenantPage godoc
// @Summary Page tenants
// @Tags System Tenant
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/tenant/page [get]
func (h *Handler) TenantPage(c *gin.Context) {
	query := h.service.db.Model(&Tenant{})
	if name := strings.TrimSpace(c.Query("name")); name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}
	if contact := strings.TrimSpace(c.Query("contactName")); contact != "" {
		query = query.Where("contact_name LIKE ?", "%"+contact+"%")
	}
	if mobile := strings.TrimSpace(c.Query("contactMobile")); mobile != "" {
		query = query.Where("contact_mobile LIKE ?", "%"+mobile+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	query.Count(&total)
	pn, ps := pageParams(c)
	var rows []Tenant
	query.Order("id DESC").Offset((pn - 1) * ps).Limit(ps).Find(&rows)
	views := make([]TenantView, 0, len(rows))
	for _, row := range rows {
		views = append(views, tenantView(row))
	}
	httpx.OK(c, gin.H{"list": views, "total": total})
}

// TenantGet godoc
// @Summary Get a tenant
// @Tags System Tenant
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/tenant/get [get]
func (h *Handler) TenantGet(c *gin.Context) {
	var row Tenant
	if h.service.db.First(&row, queryID(c)).Error != nil {
		httpx.Fail(c, 404, 404, "租户不存在")
		return
	}
	httpx.OK(c, tenantView(row))
}

// TenantSimpleList godoc
// @Summary List enabled tenants
// @Tags System Tenant
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/tenant/simple-list [get]
func (h *Handler) TenantSimpleList(c *gin.Context) {
	var rows []Tenant
	h.service.db.Where("status = 0").Order("id").Find(&rows)
	views := make([]TenantView, 0, len(rows))
	for _, row := range rows {
		views = append(views, tenantView(row))
	}
	httpx.OK(c, views)
}

func applyTenant(row *Tenant, req TenantSaveRequest) {
	row.Name, row.ContactName, row.ContactMobile, row.Status, row.ExpireTime, row.AccountCount = strings.TrimSpace(req.Name), req.ContactName, req.ContactMobile, req.Status, req.ExpireTime, req.AccountCount
	if len(req.Websites) > 0 {
		row.Domain = strings.Join(req.Websites, ",")
	} else {
		row.Domain = req.Domain
	}
	if row.AccountCount <= 0 {
		row.AccountCount = 100
	}
	if row.ExpireTime.IsZero() {
		row.ExpireTime = time.Now().AddDate(10, 0, 0)
	}
}

// TenantCreate godoc
// @Summary Create a tenant and its initial administrator
// @Tags System Tenant
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body TenantSaveRequest true "Tenant"
// @Success 200 {object} httpx.Response
// @Router /system/tenant/create [post]
func (h *Handler) TenantCreate(c *gin.Context) {
	var req TenantSaveRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Username) == "" || req.Password == "" {
		httpx.Fail(c, 400, 400, "租户名称、管理员账号和密码不能为空")
		return
	}
	var pkg TenantPackage
	if h.service.db.Where("status = 0").Order("id").First(&pkg).Error != nil {
		httpx.Fail(c, 500, 500, "默认租户权限配置不存在")
		return
	}
	row := Tenant{}
	applyTenant(&row, req)
	row.PackageID = pkg.ID
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.Fail(c, 500, 500, "密码处理失败")
		return
	}
	tx := h.service.db.Begin()
	if err = tx.Create(&row).Error; err != nil {
		tx.Rollback()
		httpx.Fail(c, 409, 409, "租户名称已存在")
		return
	}
	dept := Department{TenantID: row.ID, Name: row.Name, Sort: 0, Status: 0}
	if err = tx.Create(&dept).Error; err != nil {
		tx.Rollback()
		httpx.Fail(c, 500, 500, "初始化部门失败")
		return
	}
	post := Post{TenantID: row.ID, Name: "管理员", Code: "admin", Sort: 0, Status: 0}
	if err = tx.Create(&post).Error; err != nil {
		tx.Rollback()
		httpx.Fail(c, 500, 500, "初始化岗位失败")
		return
	}
	role := Role{TenantID: row.ID, Name: "超级管理员", Code: "super_admin", Sort: 0, Status: 0, Type: 1, DataScope: 1, DataScopeDeptIDs: "[]"}
	if err = tx.Create(&role).Error; err != nil {
		tx.Rollback()
		httpx.Fail(c, 500, 500, "初始化角色失败")
		return
	}
	if err = ensureVisionAIRolesForTenant(tx, row.ID); err != nil {
		tx.Rollback()
		httpx.Fail(c, 500, 500, "初始化 VisionAI 角色失败")
		return
	}
	user := AdminUser{TenantID: row.ID, Username: req.Username, PasswordHash: string(hash), Nickname: req.ContactName, Mobile: req.ContactMobile, DeptID: dept.ID, Status: 0, LoginDate: time.Now()}
	if user.Nickname == "" {
		user.Nickname = "管理员"
	}
	if err = tx.Create(&user).Error; err != nil {
		tx.Rollback()
		httpx.Fail(c, 409, 409, "管理员账号已存在")
		return
	}
	tx.Create(&UserRole{UserID: user.ID, RoleID: role.ID})
	tx.Create(&UserPost{UserID: user.ID, PostID: post.ID})
	if err = tx.Commit().Error; err != nil {
		httpx.Fail(c, 500, 500, "创建租户失败")
		return
	}
	httpx.OK(c, row.ID)
}

// TenantUpdate godoc
// @Summary Update a tenant
// @Tags System Tenant
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body TenantSaveRequest true "Tenant"
// @Success 200 {object} httpx.Response
// @Router /system/tenant/update [put]
func (h *Handler) TenantUpdate(c *gin.Context) {
	var req TenantSaveRequest
	if c.ShouldBindJSON(&req) != nil || req.ID == 0 {
		httpx.Fail(c, 400, 400, "请求参数错误")
		return
	}
	var row Tenant
	if h.service.db.First(&row, req.ID).Error != nil {
		httpx.Fail(c, 404, 404, "租户不存在")
		return
	}
	applyTenant(&row, req)
	if h.service.db.Save(&row).Error != nil {
		httpx.Fail(c, 409, 409, "租户名称已存在")
		return
	}
	httpx.OK(c, true)
}

// TenantDelete godoc
// @Summary Delete a tenant and tenant-owned system data
// @Tags System Tenant
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/tenant/delete [delete]
func (h *Handler) TenantDelete(c *gin.Context) { h.deleteTenants(c, []uint64{queryID(c)}) }

// TenantDeleteList godoc
// @Summary Delete tenants in batch
// @Tags System Tenant
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /system/tenant/delete-list [delete]
func (h *Handler) TenantDeleteList(c *gin.Context) { h.deleteTenants(c, splitIDs(c.Query("ids"))) }

func (h *Handler) deleteTenants(c *gin.Context, ids []uint64) {
	for _, id := range ids {
		if id == tenantIDFromContext(c) {
			httpx.Fail(c, http.StatusBadRequest, 400, "不能删除当前登录租户")
			return
		}
	}
	var userIDs, roleIDs []uint64
	h.service.db.Model(&AdminUser{}).Where("tenant_id IN ?", ids).Pluck("id", &userIDs)
	h.service.db.Model(&Role{}).Where("tenant_id IN ?", ids).Pluck("id", &roleIDs)
	tx := h.service.db.Begin()
	tx.Where("user_id IN ?", userIDs).Delete(&UserRole{})
	tx.Where("user_id IN ?", userIDs).Delete(&UserPost{})
	tx.Where("role_id IN ?", roleIDs).Delete(&RoleMenu{})
	tx.Where("tenant_id IN ?", ids).Delete(&AdminUser{})
	tx.Where("tenant_id IN ?", ids).Delete(&Role{})
	tx.Where("tenant_id IN ?", ids).Delete(&Post{})
	tx.Where("tenant_id IN ?", ids).Delete(&Department{})
	tx.Where("id IN ?", ids).Delete(&Tenant{})
	tx.Commit()
	httpx.OK(c, true)
}
