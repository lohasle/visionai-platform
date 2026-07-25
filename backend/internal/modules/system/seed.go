package system

import (
	"encoding/json"
	"strings"

	"gorm.io/gorm"
)

type menuSeed struct {
	ID            uint64
	ParentID      uint64
	Name          string
	Path          string
	Icon          string
	Component     string
	ComponentName string
	Sort          int
	Permissions   []string
}

type VisionAIRoleDefinition struct {
	Name string
	Code string
	Sort int
}

var VisionAIRoleDefinitions = []VisionAIRoleDefinition{
	{Name: "项目负责人", Code: "PROJECT_OWNER", Sort: 100},
	{Name: "数据管理员", Code: "DATA_MANAGER", Sort: 110},
	{Name: "标注员", Code: "ANNOTATOR", Sort: 120},
	{Name: "审核员", Code: "REVIEWER", Sort: 130},
	{Name: "算法工程师", Code: "ALGORITHM_ENGINEER", Sort: 140},
	{Name: "模型审批人", Code: "APPROVER", Sort: 150},
	{Name: "平台运维", Code: "OPS", Sort: 160},
	{Name: "审计员", Code: "AUDITOR", Sort: 170},
}

func ensureVisionAIRolesForTenant(db *gorm.DB, tenantID uint64) error {
	for _, definition := range VisionAIRoleDefinitions {
		role := Role{TenantID: tenantID, Code: definition.Code}
		if err := db.Where("tenant_id = ? AND code = ?", tenantID, definition.Code).Attrs(Role{
			Name: definition.Name, Sort: definition.Sort, Status: 0, Type: 1, DataScope: 1,
			DataScopeDeptIDs: "[]", Remark: "VisionAI 内置业务角色，在系统管理中统一分配",
		}).FirstOrCreate(&role).Error; err != nil {
			return err
		}
	}
	return nil
}

func EnsureVisionAIRoles(db *gorm.DB) error {
	var tenantIDs []uint64
	if err := db.Model(&Tenant{}).Order("id").Pluck("id", &tenantIDs).Error; err != nil {
		return err
	}
	for _, tenantID := range tenantIDs {
		if err := ensureVisionAIRolesForTenant(db, tenantID); err != nil {
			return err
		}
	}
	return nil
}

func SeedBase(db *gorm.DB) error {
	groups := []SystemMenu{
		{ID: 1, Name: "系统管理", Type: 1, Sort: 10, ParentID: 0, Path: "/system", Icon: "ep:tools", Status: 0, Visible: true, KeepAlive: true, AlwaysShow: true},
		{ID: 2, Name: "基础设施", Type: 1, Sort: 20, ParentID: 0, Path: "/infra", Icon: "ep:setting", Status: 0, Visible: true, KeepAlive: true, AlwaysShow: true},
		{ID: 5, Name: "VisionAI", Type: 1, Sort: 1, ParentID: 0, Path: "/ai-platform", Icon: "lucide:scan-eye", Status: 0, Visible: true, KeepAlive: true, AlwaysShow: true},
	}
	pages := []menuSeed{
		{100, 1, "用户管理", "user", "ep:avatar", "system/user/index", "SystemUser", 1, []string{"system:user:query", "system:user:create", "system:user:update", "system:user:delete", "system:user:update-password", "system:user:import", "system:user:export", "system:permission:assign-user-role"}},
		{101, 1, "角色管理", "role", "ep:user", "system/role/index", "SystemRole", 2, []string{"system:role:query", "system:role:create", "system:role:update", "system:role:delete", "system:role:export", "system:permission:assign-role-menu", "system:permission:assign-role-data-scope"}},
		{102, 1, "菜单管理", "menu", "ep:menu", "system/menu/index", "SystemMenu", 3, []string{"system:menu:query", "system:menu:create", "system:menu:update", "system:menu:delete"}},
		{103, 1, "部门管理", "dept", "ep:office-building", "system/dept/index", "SystemDept", 4, []string{"system:dept:query", "system:dept:create", "system:dept:update", "system:dept:delete"}},
		{104, 1, "岗位管理", "post", "ep:postcard", "system/post/index", "SystemPost", 5, []string{"system:post:query", "system:post:create", "system:post:update", "system:post:delete", "system:post:export"}},
		{105, 1, "字典管理", "dict", "ep:collection", "system/dict/index", "SystemDictType", 6, []string{"system:dict:query", "system:dict:create", "system:dict:update", "system:dict:delete", "system:dict:export"}},
		{106, 1, "租户管理", "tenant", "ep:office-building", "system/tenant/index", "SystemTenant", 7, []string{"system:tenant:query", "system:tenant:create", "system:tenant:update", "system:tenant:delete", "system:tenant:export"}},
		{108, 1, "登录日志", "login-log", "ep:document", "system/loginlog/index", "SystemLoginLog", 9, []string{"system:login-log:query", "system:login-log:export"}},
		{109, 1, "操作日志", "operate-log", "ep:document-checked", "system/operatelog/index", "SystemOperateLog", 10, []string{"system:operate-log:query", "system:operate-log:export"}},
		{110, 1, "OAuth2 客户端", "oauth2-client", "ep:key", "system/oauth2/client/index", "SystemOAuth2Client", 11, []string{"system:oauth2-client:query", "system:oauth2-client:create", "system:oauth2-client:update", "system:oauth2-client:delete"}},
		{111, 1, "OAuth2 令牌", "oauth2-token", "ep:connection", "system/oauth2/token/index", "SystemTokenClient", 12, []string{"system:oauth2-token:query", "system:oauth2-token:delete"}},
		{112, 1, "通知公告", "notice", "ep:bell", "system/notice/index", "SystemNotice", 13, []string{"system:notice:query", "system:notice:create", "system:notice:update", "system:notice:delete"}},
		{113, 1, "站内信模板", "notify-template", "ep:chat-line-square", "system/notify/template/index", "NotifySmsTemplate", 14, []string{"system:notify-template:query", "system:notify-template:create", "system:notify-template:update", "system:notify-template:delete", "system:notify-template:send-notify"}},
		{114, 1, "站内信消息", "notify-message", "ep:message", "system/notify/message/index", "SystemNotifyMessage", 15, []string{"system:notify-message:query"}},
		{115, 1, "邮箱账号", "mail-account", "ep:message-box", "system/mail/account/index", "SystemMailAccount", 16, []string{"system:mail-account:query", "system:mail-account:create", "system:mail-account:update", "system:mail-account:delete"}},
		{116, 1, "邮件模板", "mail-template", "ep:document-copy", "system/mail/template/index", "SystemMailTemplate", 17, []string{"system:mail-template:query", "system:mail-template:create", "system:mail-template:update", "system:mail-template:delete", "system:mail-template:send-mail"}},
		{117, 1, "邮件日志", "mail-log", "ep:document", "system/mail/log/index", "SystemMailLog", 18, []string{"system:mail-log:query", "system:mail-log:export"}},
		{118, 1, "短信渠道", "sms-channel", "ep:iphone", "system/sms/channel/index", "SystemSmsChannel", 19, []string{"system:sms-channel:query", "system:sms-channel:create", "system:sms-channel:update", "system:sms-channel:delete"}},
		{119, 1, "短信模板", "sms-template", "ep:chat-dot-square", "system/sms/template/index", "SystemSmsTemplate", 20, []string{"system:sms-template:query", "system:sms-template:create", "system:sms-template:update", "system:sms-template:delete", "system:sms-template:export", "system:sms-template:send-sms"}},
		{120, 1, "短信日志", "sms-log", "ep:document", "system/sms/log/index", "SystemSmsLog", 21, []string{"system:sms-log:query", "system:sms-log:export"}},
		{201, 2, "参数配置", "config", "ep:operation", "infra/config/index", "InfraConfig", 1, []string{"infra:config:query", "infra:config:create", "infra:config:update", "infra:config:delete", "infra:config:export"}},
		{202, 2, "文件配置", "file-config", "ep:folder-opened", "infra/fileConfig/index", "InfraFileConfig", 2, []string{"infra:file-config:query", "infra:file-config:create", "infra:file-config:update", "infra:file-config:delete"}},
		{203, 2, "访问日志", "api-access-log", "ep:document", "infra/apiAccessLog/index", "InfraApiAccessLog", 3, []string{"infra:api-access-log:query", "infra:api-access-log:export"}},
		{204, 2, "文件管理", "file", "ep:folder", "infra/file/index", "InfraFile", 4, []string{"infra:file:query", "infra:file:create", "infra:file:delete"}},
		{205, 2, "错误日志", "api-error-log", "ep:warning", "infra/apiErrorLog/index", "InfraApiErrorLog", 5, []string{"infra:api-error-log:query", "infra:api-error-log:update-status", "infra:api-error-log:export"}},
		{206, 2, "数据源配置", "data-source-config", "ep:coin", "infra/dataSourceConfig/index", "InfraDataSourceConfig", 6, []string{"infra:data-source-config:query", "infra:data-source-config:create", "infra:data-source-config:update", "infra:data-source-config:delete"}},
		{207, 2, "定时任务", "job", "ep:timer", "infra/job/index", "InfraJob", 7, []string{"infra:job:query", "infra:job:create", "infra:job:update", "infra:job:delete", "infra:job:trigger", "infra:job:export"}},
		{208, 2, "任务日志", "job-log", "ep:document", "infra/job/logger/index", "InfraJobLog", 8, []string{"infra:job-log:query", "infra:job-log:export"}},
		{209, 2, "Redis 监控", "redis", "ep:odometer", "infra/redis/index", "InfraRedis", 9, []string{"infra:redis:query"}},
		{501, 5, "工作台", "dashboard", "lucide:layout-dashboard", "ai-platform/dashboard/index", "VisionAIDashboard", 1, []string{"ai-platform:dashboard:query", "ai-platform:job:query", "ai-platform:job:cancel", "ai-platform:job:retry"}},
		{502, 5, "项目中心", "projects", "lucide:folder-kanban", "ai-platform/projects/index", "VisionAIProjects", 2, []string{"ai-platform:project:query", "ai-platform:project:create", "ai-platform:project:update", "ai-platform:project:archive", "ai-platform:project:member", "ai-platform:project:config"}},
		{503, 5, "数据资产", "assets", "lucide:images", "ai-platform/assets/index", "VisionAIAssets", 3, []string{"ai-platform:asset:query", "ai-platform:asset:upload", "ai-platform:asset:import", "ai-platform:asset:update", "ai-platform:asset:delete"}},
		{512, 5, "资产集合", "collections", "lucide:layers", "ai-platform/collections/index", "VisionAICollections", 4, []string{"ai-platform:collection:query", "ai-platform:collection:create", "ai-platform:collection:update", "ai-platform:collection:freeze"}},
		{504, 5, "标注任务", "annotations", "lucide:scan-search", "ai-platform/annotations/index", "VisionAIAnnotations", 5, []string{"ai-platform:annotation:query", "ai-platform:annotation:create", "ai-platform:annotation:prepare", "ai-platform:annotation:review", "ai-platform:annotation:export", "ai-platform:annotation:mapping"}},
		{505, 5, "数据集注册表", "datasets", "lucide:database-zap", "ai-platform/datasets/index", "VisionAIDatasets", 6, []string{"ai-platform:dataset:query", "ai-platform:dataset:create", "ai-platform:dataset:validate", "ai-platform:dataset:freeze", "ai-platform:dataset:deprecate"}},
		{506, 5, "训练与实验", "training", "lucide:brain-circuit", "ai-platform/training/index", "VisionAITraining", 7, []string{"ai-platform:training:query", "ai-platform:training:template", "ai-platform:training:smoke", "ai-platform:training:publish", "ai-platform:training:create", "ai-platform:training:cancel", "ai-platform:training:clone"}},
		{507, 5, "评估与困难样本", "evaluation", "lucide:scan-line", "ai-platform/evaluation/index", "VisionAIEvaluation", 8, []string{"ai-platform:evaluation:query", "ai-platform:evaluation:create", "ai-platform:evaluation:workbench", "ai-platform:evaluation:hard-sample"}},
		{508, 5, "模型注册与审批", "models", "lucide:badge-check", "ai-platform/models/index", "VisionAIModelRegistry", 9, []string{"ai-platform:model:query", "ai-platform:model:register", "ai-platform:model:approve", "ai-platform:model:export"}},
		{509, 5, "部署与监控", "deployments", "lucide:activity", "ai-platform/deployments/index", "VisionAIDeployments", 10, []string{"ai-platform:deployment:query", "ai-platform:deployment:create", "ai-platform:deployment:operate", "ai-platform:inference:test", "ai-platform:monitor:query"}},
		{510, 5, "生产反馈闭环", "feedback", "lucide:refresh-cw", "ai-platform/feedback/index", "VisionAIFeedback", 11, []string{"ai-platform:feedback:query", "ai-platform:feedback:policy", "ai-platform:feedback:review", "ai-platform:feedback:cleanup"}},
		{511, 5, "资源与集成", "operations", "lucide:server-cog", "ai-platform/operations/index", "VisionAIOperations", 12, []string{"ai-platform:resource:query", "ai-platform:resource:update", "ai-platform:integration:query", "ai-platform:integration:update", "ai-platform:audit:query", "ai-platform:audit:export"}},
	}
	for _, group := range groups {
		if err := upsertMenu(db, group); err != nil {
			return err
		}
	}
	// 清理当前底座未启用的菜单树及角色关联，重复启动保持幂等。
	if err := deleteMenuTrees(db, []uint64{3, 4, 107, 121, 122, 210, 211}); err != nil {
		return err
	}
	for _, page := range pages {
		row := SystemMenu{ID: page.ID, Name: page.Name, Type: 2, Sort: page.Sort, ParentID: page.ParentID, Path: page.Path, Icon: page.Icon, Component: page.Component, ComponentName: page.ComponentName, Status: 0, Visible: true, KeepAlive: true}
		if err := upsertMenu(db, row); err != nil {
			return err
		}
		for index, permission := range page.Permissions {
			button := SystemMenu{ID: page.ID*100 + uint64(index+1), Name: permissionName(permission), Permission: permission, Type: 3, Sort: index + 1, ParentID: page.ID, Status: 0, Visible: true}
			if err := upsertMenu(db, button); err != nil {
				return err
			}
		}
	}
	dictTypes := []DictType{
		{Name: "通用状态", Type: "common_status", Status: 0},
		{Name: "用户性别", Type: "system_user_sex", Status: 0},
		{Name: "角色类型", Type: "system_role_type", Status: 0},
		{Name: "菜单类型", Type: "system_menu_type", Status: 0},
		{Name: "数据范围", Type: "system_data_scope", Status: 0},
		{Name: "登录结果", Type: "system_login_result", Status: 0},
	}
	for _, item := range dictTypes {
		if err := db.Where("type = ?", item.Type).FirstOrCreate(&item).Error; err != nil {
			return err
		}
	}
	var menuIDs []uint64
	db.Model(&SystemMenu{}).Where("type IN ?", []int{1, 2}).Order("id").Pluck("id", &menuIDs)
	pack := TenantPackage{Name: "默认套餐", Status: 0, Remark: "Nimbus 完整 System、Infra、RBAC 与 OAuth2 底座", MenuIDs: joinIDs(menuIDs)}
	if err := db.Where("name = ?", pack.Name).Assign(pack).FirstOrCreate(&pack).Error; err != nil {
		return err
	}
	if err := db.Model(&Tenant{}).Where("package_id = 0").Update("package_id", pack.ID).Error; err != nil {
		return err
	}
	return EnsureVisionAIRoles(db)
}

func deleteMenuTrees(db *gorm.DB, roots []uint64) error {
	all := append([]uint64(nil), roots...)
	frontier := append([]uint64(nil), roots...)
	seen := make(map[uint64]struct{}, len(roots))
	for _, id := range roots {
		seen[id] = struct{}{}
	}
	for len(frontier) > 0 {
		var children []uint64
		if err := db.Model(&SystemMenu{}).Where("parent_id IN ?", frontier).Pluck("id", &children).Error; err != nil {
			return err
		}
		frontier = frontier[:0]
		for _, id := range children {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			all = append(all, id)
			frontier = append(frontier, id)
		}
	}
	if err := db.Where("menu_id IN ?", all).Delete(&RoleMenu{}).Error; err != nil {
		return err
	}
	return db.Where("id IN ?", all).Delete(&SystemMenu{}).Error
}

func upsertMenu(db *gorm.DB, row SystemMenu) error {
	return db.Where("id = ?", row.ID).Assign(row).FirstOrCreate(&row).Error
}

func permissionName(permission string) string {
	parts := strings.Split(permission, ":")
	if len(parts) == 0 {
		return permission
	}
	return strings.ToUpper(parts[len(parts)-1])
}

func joinIDs(ids []uint64) string {
	data, _ := json.Marshal(ids)
	return string(data)
}
