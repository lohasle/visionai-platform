package visionai

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func workbenchTestContext(host, origin string) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest("POST", "http://"+host+"/admin-api/ai-platform/test", nil)
	request.Host = host
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	ctx.Request = request
	return ctx
}

func TestWorkbenchTargetUsesCurrentAccessHostname(t *testing.T) {
	ctx := workbenchTestContext("192.168.88.20:48080", "http://192.168.88.20:48080")
	target, err := workbenchTargetForRequest(
		ctx,
		"FIFTYONE",
		configuredWorkbenchBase("FIFTYONE")+"/?dataset=tenant-1-project-2-evaluation-3",
	)
	if err != nil {
		t.Fatalf("rewrite workbench target: %v", err)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse rewritten target: %v", err)
	}
	if parsed.Hostname() != "192.168.88.20" {
		t.Fatalf("expected request hostname, got %q", parsed.Hostname())
	}
	if parsed.Query().Get("dataset") != "tenant-1-project-2-evaluation-3" {
		t.Fatalf("dataset query was not preserved: %q", parsed.RawQuery)
	}
	if !allowedWorkbenchRedirect(ctx, "FIFTYONE", target) {
		t.Fatal("rewritten same-host target should be trusted")
	}
}

func TestWorkbenchTargetRejectsOriginHostMismatch(t *testing.T) {
	ctx := workbenchTestContext("localhost:48080", "http://attacker.example:48080")
	if _, err := workbenchBaseForRequest(ctx, "CVAT"); err == nil {
		t.Fatal("expected a mismatched Origin hostname to be rejected")
	}
}

func TestWorkbenchRedirectRejectsDifferentAccessHostname(t *testing.T) {
	ctx := workbenchTestContext("localhost:25151", "")
	target := configuredWorkbenchBase("FIFTYONE") + "/?dataset=controlled"
	parsed, _ := url.Parse(target)
	parsed.Host = "192.168.88.20:" + parsed.Port()
	if allowedWorkbenchRedirect(ctx, "FIFTYONE", parsed.String()) {
		t.Fatal("redirect to a different access hostname must be rejected")
	}
}
