package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/modules/system"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
)

func TestVisibleAdminRouteContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := New(system.NewHandler(system.NewService(nil, config.Config{})), nil)
	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	want := []string{
		"POST /admin-api/system/auth/login",
		"POST /admin-api/system/auth/refresh-token",
		"GET /admin-api/system/auth/get-permission-info",
		"GET /admin-api/system/user/page",
		"GET /admin-api/system/user/list",
		"GET /admin-api/system/user/get",
		"POST /admin-api/system/user/create",
		"PUT /admin-api/system/user/update",
		"DELETE /admin-api/system/user/delete",
		"GET /admin-api/system/user/export-excel",
		"GET /admin-api/system/user/get-import-template",
		"POST /admin-api/system/user/import",
		"GET /admin-api/system/role/simple-list",
		"GET /admin-api/system/role/page",
		"GET /admin-api/system/menu/list",
		"GET /admin-api/system/dept/list",
		"GET /admin-api/system/post/page",
		"GET /admin-api/system/dict-type/page",
		"GET /admin-api/system/dict-data/page",
		"GET /admin-api/system/tenant/page",
		"GET /admin-api/system/login-log/page",
		"GET /admin-api/system/operate-log/page",
		"GET /admin-api/system/oauth2-client/page",
		"GET /admin-api/system/oauth2-token/page",
		"GET /admin-api/system/notice/page",
		"GET /admin-api/system/notify-template/page",
		"GET /admin-api/system/notify-message/page",
		"GET /admin-api/system/mail-account/page",
		"GET /admin-api/system/mail-template/page",
		"GET /admin-api/system/mail-log/page",
		"GET /admin-api/system/sms-channel/page",
		"GET /admin-api/system/sms-template/page",
		"GET /admin-api/system/sms-log/page",
		"GET /admin-api/system/permission/list-user-roles",
		"POST /admin-api/system/permission/assign-user-role",
		"GET /admin-api/system/area/tree",
		"GET /admin-api/infra/config/page",
		"GET /admin-api/infra/config/export-excel",
		"GET /admin-api/infra/file-config/page",
		"GET /admin-api/infra/api-access-log/page",
		"GET /admin-api/infra/api-access-log/export-excel",
		"GET /admin-api/infra/file/page",
		"GET /admin-api/infra/api-error-log/page",
		"GET /admin-api/infra/data-source-config/list",
		"GET /admin-api/infra/job/page",
		"GET /admin-api/infra/job-log/page",
		"GET /admin-api/infra/redis/get-monitor-info",
		"GET /admin-api/ai-platform/dashboard/summary",
		"GET /admin-api/ai-platform/jobs",
		"GET /admin-api/ai-platform/jobs/:id",
		"GET /admin-api/ai-platform/jobs/:id/events",
		"GET /admin-api/ai-platform/workbench-sso/cvat",
		"GET /admin-api/ai-platform/workbench-sso/fiftyone",
		"GET /admin-api/ai-platform/workbench-sso/fiftyone/validate",
		"POST /admin-api/ai-platform/jobs/:id/cancel",
		"POST /admin-api/ai-platform/jobs/:id/retry",
		"GET /admin-api/ai-platform/projects",
		"POST /admin-api/ai-platform/projects",
		"GET /admin-api/ai-platform/projects/:id",
		"PUT /admin-api/ai-platform/projects/:id",
		"PUT /admin-api/ai-platform/projects/:id/status",
		"POST /admin-api/ai-platform/projects/:id/archive",
		"POST /admin-api/ai-platform/projects/:id/clone",
		"GET /admin-api/ai-platform/projects/:id/ontologies",
		"POST /admin-api/ai-platform/projects/:id/ontologies",
		"GET /admin-api/ai-platform/projects/:id/ontologies/:ontologyId",
		"POST /admin-api/ai-platform/projects/:id/ontologies/:ontologyId/versions",
		"GET /admin-api/ai-platform/projects/:id/ontology-versions",
		"GET /admin-api/ai-platform/projects/:id/ontology-versions/:versionId",
		"PUT /admin-api/ai-platform/projects/:id/ontology-versions/:versionId/labels",
		"POST /admin-api/ai-platform/projects/:id/ontology-versions/:versionId/publish",
		"POST /admin-api/ai-platform/projects/:id/ontology-versions/:versionId/deprecate",
		"GET /admin-api/ai-platform/projects/:id/members",
		"PUT /admin-api/ai-platform/projects/:id/members",
		"DELETE /admin-api/ai-platform/projects/:id/members/:userId",
		"GET /admin-api/ai-platform/projects/:id/config",
		"PUT /admin-api/ai-platform/projects/:id/config",
		"POST /admin-api/ai-platform/projects/:id/uploads",
		"GET /admin-api/ai-platform/projects/:id/uploads/:sessionId",
		"PUT /admin-api/ai-platform/projects/:id/uploads/:sessionId/chunks/:part",
		"POST /admin-api/ai-platform/projects/:id/uploads/:sessionId/complete",
		"GET /admin-api/ai-platform/projects/:id/assets",
		"PUT /admin-api/ai-platform/projects/:id/assets/tags",
		"GET /admin-api/ai-platform/projects/:id/assets/quality",
		"GET /admin-api/ai-platform/projects/:id/assets/:assetId",
		"DELETE /admin-api/ai-platform/projects/:id/assets/:assetId",
		"POST /admin-api/ai-platform/projects/:id/assets/:assetId/restore",
		"POST /admin-api/ai-platform/projects/:id/assets/:assetId/purge",
		"GET /admin-api/ai-platform/projects/:id/training-runs/:runId/export",
		"GET /admin-api/ai-platform/projects/:id/training-runs/:runId/artifacts/:artifactId/download",
		"POST /admin-api/ai-platform/projects/:id/deployments/:deploymentId/predict-image",
		"GET /admin-api/ai-platform/projects/:id/asset-imports",
		"POST /admin-api/ai-platform/projects/:id/asset-imports",
		"GET /admin-api/ai-platform/projects/:id/asset-imports/:importId",
		"GET /admin-api/ai-platform/projects/:id/collections",
		"POST /admin-api/ai-platform/projects/:id/collections",
		"GET /admin-api/ai-platform/projects/:id/collections/:collectionId/assets",
		"PUT /admin-api/ai-platform/projects/:id/collections/:collectionId/assets",
		"POST /admin-api/ai-platform/projects/:id/collections/:collectionId/freeze",
		"PUT /admin-api/ai-platform/projects/:id/assets/:assetId/tags",
		"GET /admin-api/ai-platform/projects/:id/tag-definitions",
		"POST /admin-api/ai-platform/projects/:id/tag-definitions",
		"PUT /admin-api/ai-platform/projects/:id/tag-definitions/:definitionId",
	}

	for _, route := range want {
		if _, ok := routes[route]; !ok {
			t.Errorf("missing visible admin route %s", route)
		}
	}
}
