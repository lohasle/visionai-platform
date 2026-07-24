package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	appmodule "github.com/lohasle/nimbus-framework-go/internal/modules/app"
	"github.com/lohasle/nimbus-framework-go/internal/modules/application"
	"github.com/lohasle/nimbus-framework-go/internal/modules/im"
	"github.com/lohasle/nimbus-framework-go/internal/modules/infra"
	"github.com/lohasle/nimbus-framework-go/internal/modules/member"
	"github.com/lohasle/nimbus-framework-go/internal/modules/pay"
	"github.com/lohasle/nimbus-framework-go/internal/modules/system"
	"github.com/lohasle/nimbus-framework-go/internal/modules/visionai"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"github.com/lohasle/nimbus-framework-go/internal/platform/middleware"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

func New(handler *system.Handler, db *gorm.DB) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})
	r.Use(gin.Recovery(), middleware.CORS(), middleware.RequestContext(func(record middleware.RequestLog) {
		if record.TenantID == 0 {
			return
		}
		db.Create(&infra.APIAccessLog{
			TenantID: record.TenantID, TraceID: record.TraceID, UserID: record.UserID,
			Method: record.Method, Path: record.Path, Status: record.Status, Duration: record.Duration,
			IP: record.IP, UserAgent: record.UserAgent,
		})
		if record.Status >= http.StatusInternalServerError {
			db.Create(&infra.APIErrorLog{TenantID: record.TenantID, TraceID: record.TraceID, UserID: record.UserID, UserType: 2, ApplicationName: "nimbus-server", RequestMethod: record.Method, RequestURL: record.Path, UserIP: record.IP, UserAgent: record.UserAgent, ExceptionTime: time.Now(), ExceptionName: http.StatusText(record.Status), ExceptionMessage: "HTTP request failed", ExceptionRootCauseMessage: "HTTP request returned server error", ResultCode: record.Status})
		}
		if record.Method != http.MethodGet && record.Method != http.MethodOptions && record.Path != "/admin-api/system/auth/login" {
			var user system.AdminUser
			db.Select("username").First(&user, record.UserID)
			db.Create(&system.OperateLog{TenantID: record.TenantID, TraceID: record.TraceID, UserType: 2, UserID: record.UserID, UserName: user.Username, Type: "HTTP", SubType: record.Method, Action: record.Method + " " + record.Path, RequestMethod: record.Method, RequestURL: record.Path, UserIP: record.IP, UserAgent: record.UserAgent})
		}
	}))
	r.GET("/health", health("nimbus-server"))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	admin := r.Group("/admin-api")
	admin.GET("/system/tenant/get-id-by-name", handler.TenantID)
	admin.GET("/system/tenant/get-by-website", handler.TenantByWebsite)
	admin.POST("/system/auth/login", handler.Login)
	admin.POST("/system/auth/register", handler.Register)
	admin.POST("/system/auth/send-sms-code", handler.SendAuthSMSCode)
	admin.POST("/system/auth/sms-login", handler.SMSLogin)
	admin.POST("/system/auth/reset-password", handler.ResetPassword)
	admin.POST("/system/auth/social-login", handler.SocialLogin)
	admin.POST("/system/auth/refresh-token", handler.RefreshToken)
	admin.GET("/system/auth/social-auth-redirect", handler.SocialAuthRedirect)
	admin.POST("/system/oauth2/token", handler.OAuth2OpenToken)
	admin.DELETE("/system/oauth2/token", handler.OAuth2OpenRevoke)
	admin.POST("/system/oauth2/check-token", handler.OAuth2OpenCheckToken)
	admin.GET("/system/auth/get-permission-info", handler.Auth(), handler.PermissionInfo)
	admin.GET("/system/oauth2/authorize", handler.Auth(), handler.OAuth2AuthorizeInfo)
	admin.POST("/system/oauth2/authorize", handler.Auth(), handler.OAuth2Authorize)
	admin.GET("/system/dict-data/simple-list", handler.Auth(), handler.SimpleDictData)
	admin.GET("/system/notify-message/get-unread-count", handler.Auth(), handler.UnreadNotifyMessageCount)
	admin.GET("/system/notify-message/get-unread-list", handler.Auth(), handler.UnreadNotifyMessageList)
	admin.POST("/system/auth/logout", handler.Auth(), handler.Logout)
	systemAdmin := admin.Group("/system", handler.Auth())
	systemAdmin.GET("/user/page", handler.UserPage)
	systemAdmin.GET("/user/list", handler.UserList)
	systemAdmin.GET("/user/simple-list", handler.SimpleUsers)
	systemAdmin.GET("/user/get-simple", handler.SimpleUser)
	systemAdmin.GET("/user/list-by-nickname", handler.UsersByNickname)
	systemAdmin.GET("/user/get", handler.UserGet)
	systemAdmin.POST("/user/create", handler.UserCreate)
	systemAdmin.PUT("/user/update", handler.UserUpdate)
	systemAdmin.DELETE("/user/delete", handler.UserDelete)
	systemAdmin.DELETE("/user/delete-list", handler.UserDeleteList)
	systemAdmin.PUT("/user/update-password", handler.UserPassword)
	systemAdmin.PUT("/user/update-status", handler.UserStatus)
	systemAdmin.GET("/user/export-excel", handler.UserExport)
	systemAdmin.GET("/user/get-import-template", handler.UserImportTemplate)
	systemAdmin.POST("/user/import", handler.UserImport)
	systemAdmin.GET("/role/simple-list", handler.SimpleRoles)
	systemAdmin.GET("/role/page", handler.RolePage)
	systemAdmin.GET("/role/get", handler.RoleGet)
	systemAdmin.POST("/role/create", handler.RoleCreate)
	systemAdmin.PUT("/role/update", handler.RoleUpdate)
	systemAdmin.DELETE("/role/delete", handler.RoleDelete)
	systemAdmin.DELETE("/role/delete-list", handler.RoleDeleteList)
	systemAdmin.GET("/role/export-excel", handler.RoleExport)
	systemAdmin.GET("/permission/list-user-roles", handler.UserRoleList)
	systemAdmin.POST("/permission/assign-user-role", handler.AssignUserRole)
	systemAdmin.GET("/permission/list-role-menus", handler.RoleMenuList)
	systemAdmin.POST("/permission/assign-role-menu", handler.AssignRoleMenu)
	systemAdmin.POST("/permission/assign-role-data-scope", handler.AssignRoleDataScope)
	systemAdmin.GET("/menu/simple-list", handler.MenuSimpleList)
	systemAdmin.GET("/menu/list", handler.MenuList)
	systemAdmin.GET("/menu/get", handler.MenuGet)
	systemAdmin.POST("/menu/create", handler.MenuCreate)
	systemAdmin.PUT("/menu/update", handler.MenuUpdate)
	systemAdmin.DELETE("/menu/delete", handler.MenuDelete)
	systemAdmin.GET("/dept/simple-list", handler.SimpleDepartments)
	systemAdmin.GET("/dept/list", handler.DeptList)
	systemAdmin.GET("/dept/get", handler.DeptGet)
	systemAdmin.POST("/dept/create", handler.DeptCreate)
	systemAdmin.PUT("/dept/update", handler.DeptUpdate)
	systemAdmin.DELETE("/dept/delete", handler.DeptDelete)
	systemAdmin.DELETE("/dept/delete-list", handler.DeptDeleteList)
	systemAdmin.GET("/post/simple-list", handler.SimplePosts)
	systemAdmin.GET("/post/page", handler.PostPage)
	systemAdmin.GET("/post/get", handler.PostGet)
	systemAdmin.POST("/post/create", handler.PostCreate)
	systemAdmin.PUT("/post/update", handler.PostUpdate)
	systemAdmin.DELETE("/post/delete", handler.PostDelete)
	systemAdmin.DELETE("/post/delete-list", handler.PostDeleteList)
	systemAdmin.GET("/post/export-excel", handler.PostExport)
	systemAdmin.GET("/dict-type/simple-list", handler.DictTypeSimpleList)
	systemAdmin.GET("/dict-type/page", handler.DictTypePage)
	systemAdmin.GET("/dict-type/get", handler.DictTypeGet)
	systemAdmin.POST("/dict-type/create", handler.DictTypeCreate)
	systemAdmin.PUT("/dict-type/update", handler.DictTypeUpdate)
	systemAdmin.DELETE("/dict-type/delete", handler.DictTypeDelete)
	systemAdmin.DELETE("/dict-type/delete-list", handler.DictTypeDeleteList)
	systemAdmin.GET("/dict-type/export-excel", handler.DictTypeExport)
	systemAdmin.GET("/dict-data/page", handler.DictDataPage)
	systemAdmin.GET("/dict-data/get", handler.DictDataGet)
	systemAdmin.GET("/dict-data/type", handler.DictDataByType)
	systemAdmin.POST("/dict-data/create", handler.DictDataCreate)
	systemAdmin.PUT("/dict-data/update", handler.DictDataUpdate)
	systemAdmin.DELETE("/dict-data/delete", handler.DictDataDelete)
	systemAdmin.DELETE("/dict-data/delete-list", handler.DictDataDeleteList)
	systemAdmin.GET("/dict-data/export-excel", handler.DictDataExport)
	systemAdmin.GET("/tenant/page", handler.TenantPage)
	systemAdmin.GET("/tenant/get", handler.TenantGet)
	systemAdmin.GET("/tenant/simple-list", handler.TenantSimpleList)
	systemAdmin.POST("/tenant/create", handler.TenantCreate)
	systemAdmin.PUT("/tenant/update", handler.TenantUpdate)
	systemAdmin.DELETE("/tenant/delete", handler.TenantDelete)
	systemAdmin.DELETE("/tenant/delete-list", handler.TenantDeleteList)
	systemAdmin.GET("/tenant/export-excel", handler.TenantExport)
	systemAdmin.GET("/login-log/page", handler.LoginLogPage)
	systemAdmin.GET("/login-log/export-excel", handler.LoginLogExport)
	systemAdmin.GET("/operate-log/page", handler.OperateLogPage)
	systemAdmin.GET("/operate-log/export-excel", handler.OperateLogExport)
	systemAdmin.GET("/oauth2-client/page", handler.OAuth2ClientPage)
	systemAdmin.GET("/oauth2-client/get", handler.OAuth2ClientGet)
	systemAdmin.POST("/oauth2-client/create", handler.OAuth2ClientCreate)
	systemAdmin.PUT("/oauth2-client/update", handler.OAuth2ClientUpdate)
	systemAdmin.DELETE("/oauth2-client/delete", handler.OAuth2ClientDelete)
	systemAdmin.DELETE("/oauth2-client/delete-list", handler.OAuth2ClientDeleteList)
	systemAdmin.GET("/oauth2-token/page", handler.OAuth2TokenPage)
	systemAdmin.DELETE("/oauth2-token/delete", handler.OAuth2TokenDelete)
	systemAdmin.GET("/notice/page", handler.NoticePage)
	systemAdmin.GET("/notice/get", handler.NoticeGet)
	systemAdmin.POST("/notice/create", handler.NoticeCreate)
	systemAdmin.PUT("/notice/update", handler.NoticeUpdate)
	systemAdmin.DELETE("/notice/delete", handler.NoticeDelete)
	systemAdmin.DELETE("/notice/delete-list", handler.NoticeDeleteList)
	systemAdmin.POST("/notice/push", handler.NoticePush)
	systemAdmin.GET("/notify-template/simple-list", handler.NotifyTemplateSimpleList)
	systemAdmin.GET("/notify-template/page", handler.NotifyTemplatePage)
	systemAdmin.GET("/notify-template/get", handler.NotifyTemplateGet)
	systemAdmin.POST("/notify-template/create", handler.NotifyTemplateCreate)
	systemAdmin.PUT("/notify-template/update", handler.NotifyTemplateUpdate)
	systemAdmin.DELETE("/notify-template/delete", handler.NotifyTemplateDelete)
	systemAdmin.DELETE("/notify-template/delete-list", handler.NotifyTemplateDeleteList)
	systemAdmin.POST("/notify-template/send-notify", handler.NotifyTemplateSend)
	systemAdmin.GET("/notify-message/page", handler.NotifyMessagePage)
	systemAdmin.GET("/notify-message/my-page", handler.MyNotifyMessagePage)
	systemAdmin.PUT("/notify-message/update-read", handler.NotifyMessageRead)
	systemAdmin.PUT("/notify-message/update-all-read", handler.NotifyMessageReadAll)
	systemAdmin.GET("/user/profile/get", handler.UserProfileGet)
	systemAdmin.PUT("/user/profile/update", handler.UserProfileUpdate)
	systemAdmin.PUT("/user/profile/update-password", handler.UserProfilePassword)
	systemAdmin.GET("/mail-account/page", handler.MailAccountPage)
	systemAdmin.GET("/mail-account/simple-list", handler.MailAccountSimpleList)
	systemAdmin.GET("/mail-account/get", handler.MailAccountGet)
	systemAdmin.POST("/mail-account/create", handler.MailAccountCreate)
	systemAdmin.PUT("/mail-account/update", handler.MailAccountUpdate)
	systemAdmin.DELETE("/mail-account/delete", handler.MailAccountDelete)
	systemAdmin.DELETE("/mail-account/delete-list", handler.MailAccountDeleteList)
	systemAdmin.GET("/mail-template/page", handler.MailTemplatePage)
	systemAdmin.GET("/mail-template/simple-list", handler.MailTemplateSimpleList)
	systemAdmin.GET("/mail-template/get", handler.MailTemplateGet)
	systemAdmin.POST("/mail-template/create", handler.MailTemplateCreate)
	systemAdmin.PUT("/mail-template/update", handler.MailTemplateUpdate)
	systemAdmin.DELETE("/mail-template/delete", handler.MailTemplateDelete)
	systemAdmin.DELETE("/mail-template/delete-list", handler.MailTemplateDeleteList)
	systemAdmin.POST("/mail-template/send-mail", handler.MailTemplateSend)
	systemAdmin.GET("/mail-log/page", handler.MailLogPage)
	systemAdmin.GET("/mail-log/get", handler.MailLogGet)
	systemAdmin.GET("/mail-log/export-excel", handler.MailLogExport)
	systemAdmin.GET("/sms-channel/page", handler.SMSChannelPage)
	systemAdmin.GET("/sms-channel/simple-list", handler.SMSChannelSimpleList)
	systemAdmin.GET("/sms-channel/get", handler.SMSChannelGet)
	systemAdmin.POST("/sms-channel/create", handler.SMSChannelCreate)
	systemAdmin.PUT("/sms-channel/update", handler.SMSChannelUpdate)
	systemAdmin.DELETE("/sms-channel/delete", handler.SMSChannelDelete)
	systemAdmin.DELETE("/sms-channel/delete-list", handler.SMSChannelDeleteList)
	systemAdmin.GET("/sms-template/page", handler.SMSTemplatePage)
	systemAdmin.GET("/sms-template/simple-list", handler.SMSTemplateSimpleList)
	systemAdmin.GET("/sms-template/get", handler.SMSTemplateGet)
	systemAdmin.POST("/sms-template/create", handler.SMSTemplateCreate)
	systemAdmin.PUT("/sms-template/update", handler.SMSTemplateUpdate)
	systemAdmin.DELETE("/sms-template/delete", handler.SMSTemplateDelete)
	systemAdmin.DELETE("/sms-template/delete-list", handler.SMSTemplateDeleteList)
	systemAdmin.POST("/sms-template/send-sms", handler.SMSTemplateSend)
	systemAdmin.GET("/sms-template/export-excel", handler.SMSTemplateExport)
	systemAdmin.GET("/sms-log/page", handler.SMSLogPage)
	systemAdmin.GET("/sms-log/export-excel", handler.SMSLogExport)
	systemAdmin.GET("/social-client/page", handler.SocialClientPage)
	systemAdmin.GET("/social-client/get", handler.SocialClientGet)
	systemAdmin.POST("/social-client/create", handler.SocialClientCreate)
	systemAdmin.PUT("/social-client/update", handler.SocialClientUpdate)
	systemAdmin.DELETE("/social-client/delete", handler.SocialClientDelete)
	systemAdmin.GET("/social-user/page", handler.SocialUserPage)
	systemAdmin.GET("/social-user/get", handler.SocialUserGet)
	systemAdmin.GET("/social-user/get-bind-list", handler.SocialUserBindList)
	systemAdmin.POST("/social-user/bind", handler.SocialUserBind)
	systemAdmin.DELETE("/social-user/unbind", handler.SocialUserUnbind)
	systemAdmin.GET("/area/tree", handler.AreaTree)
	systemAdmin.GET("/area/get-by-ip", handler.AreaByIP)
	infra.Register(admin, db, handler.Auth())
	member.Register(admin, db, handler.Auth())
	pay.Register(admin, db, handler.Auth())
	visionai.Register(admin, db, handler.Auth())
	for _, module := range []string{
		system.ModuleName,
		infra.ModuleName,
		member.ModuleName,
		pay.ModuleName,
		application.ModuleName,
		im.ModuleName,
		appmodule.ModuleName,
		visionai.ModuleName,
	} {
		admin.GET("/"+module+"/health", health(module))
	}
	r.NoRoute(func(c *gin.Context) {
		httpx.Fail(c, http.StatusNotFound, 404, "请求地址不存在:"+c.Request.URL.Path)
	})
	return r
}

// health godoc
// @Summary Service health
// @Description Health endpoint used by deployment probes and scaffold modules.
// @Tags Health
// @Produce json
// @Success 200 {object} httpx.Response
// @Router /health [get]
// @Router /system/health [get]
// @Router /infra/health [get]
// @Router /member/health [get]
// @Router /pay/health [get]
// @Router /application/health [get]
// @Router /im/health [get]
// @Router /app/health [get]
func health(module string) gin.HandlerFunc {
	return func(c *gin.Context) { httpx.OK(c, gin.H{"status": "UP", "service": module}) }
}
