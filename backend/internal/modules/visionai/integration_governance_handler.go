package visionai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"golang.org/x/mod/semver"
	"gorm.io/gorm"
)

const visionAIPlatformVersion = "1.3.0"

type integrationUpgradeRequest struct {
	Version        string         `json:"version"`
	BaseURL        string         `json:"baseUrl"`
	SecretRef      string         `json:"secretRef"`
	Capabilities   map[string]any `json:"capabilities"`
	FaultInjection bool           `json:"faultInjection"`
}

func normalizedSemver(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	if semver.IsValid(value) {
		return value
	}
	return ""
}

func versionMatches(value, expression string) bool {
	value = normalizedSemver(value)
	if value == "" {
		return false
	}
	expression = strings.TrimSpace(expression)
	if expression == "" || expression == "*" {
		return true
	}
	tokens := strings.Fields(strings.ReplaceAll(expression, ",", " "))
	for _, token := range tokens {
		if token == "" || token == "*" {
			continue
		}
		if strings.HasSuffix(token, ".x") || strings.HasSuffix(token, ".*") {
			prefix := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(token, "v"), ".x"), ".*")
			if !strings.HasPrefix(strings.TrimPrefix(value, "v"), prefix+".") {
				return false
			}
			continue
		}
		operator := "="
		raw := token
		for _, candidate := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(token, candidate) {
				operator, raw = candidate, strings.TrimPrefix(token, candidate)
				break
			}
		}
		expected := normalizedSemver(raw)
		if expected == "" {
			return false
		}
		comparison := semver.Compare(value, expected)
		matches := map[string]bool{"=": comparison == 0, ">": comparison > 0, ">=": comparison >= 0, "<": comparison < 0, "<=": comparison <= 0}
		if !matches[operator] {
			return false
		}
	}
	return true
}

func (h *Handler) compatibilityDecision(tenant uint64, providerType, providerVersion string) (string, *CompatibilityRule) {
	var rows []CompatibilityRule
	h.db.Where("tenant_id = ? AND provider_type = ?", tenant, strings.ToUpper(providerType)).Order("id DESC").Find(&rows)
	for i := range rows {
		if versionMatches(visionAIPlatformVersion, rows[i].PlatformRange) && versionMatches(providerVersion, rows[i].ProviderRange) {
			return rows[i].Decision, &rows[i]
		}
	}
	return "REVIEW_REQUIRED", nil
}

func integrationConfigSnapshot(row IntegrationInstance) string {
	return jsonValue(gin.H{
		"providerType": row.ProviderType, "baseUrl": row.BaseURL, "secretRef": row.SecretRef,
		"networkRegion": row.NetworkRegion, "version": row.Version, "capabilities": json.RawMessage(row.Capabilities),
	})
}

func integrationConfigDiff(before, after IntegrationInstance) string {
	return jsonValue(gin.H{
		"version":             gin.H{"before": before.Version, "after": after.Version},
		"baseUrl":             gin.H{"before": before.BaseURL, "after": after.BaseURL},
		"secretRefChanged":    before.SecretRef != after.SecretRef,
		"capabilitiesChanged": before.Capabilities != after.Capabilities,
	})
}

func probeIntegration(row IntegrationInstance, useConfiguredEndpoint bool, faultInjection bool) (map[string]any, error) {
	target := strings.TrimRight(row.BaseURL, "/") + "/health"
	if useConfiguredEndpoint {
		target = integrationProbeTarget(row, config.Load())
	} else {
		switch strings.ToUpper(row.ProviderType) {
		case "CVAT":
			target = strings.TrimRight(row.BaseURL, "/") + "/api/server/about"
		case "FIFTYONE":
			target = strings.TrimRight(row.BaseURL, "/") + "/health"
		case "CLEARML":
			target = strings.TrimRight(row.BaseURL, "/") + "/debug.ping"
		}
	}
	started := time.Now()
	if faultInjection {
		return map[string]any{"success": false, "target": target, "latencyMs": 0, "faultInjection": true}, fmt.Errorf("故障注入：升级后冒烟测试失败")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(target)
	latency := float64(time.Since(started).Microseconds()) / 1000
	if err != nil {
		return map[string]any{"success": false, "target": target, "latencyMs": latency, "error": err.Error()}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err = fmt.Errorf("HTTP %s", response.Status)
		return map[string]any{"success": false, "target": target, "latencyMs": latency, "status": response.Status}, err
	}
	return map[string]any{"success": true, "target": target, "latencyMs": latency, "status": response.Status}, nil
}

// IntegrationRevisionPage godoc
// @Summary List immutable configuration backup, smoke and rollback revisions
// @Tags VisionAI Operations
// @Security BearerAuth
// @Param instanceId path int true "Integration instance ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/integrations/{instanceId}/revisions [get]
func (h *Handler) IntegrationRevisionPage(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("instanceId"), 10, 64)
	var instance IntegrationInstance
	if h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&instance).Error != nil {
		httpx.Fail(c, 404, 404, "集成实例不存在")
		return
	}
	var rows []IntegrationConfigRevision
	h.db.Where("tenant_id = ? AND instance_id = ?", tenantID(c), id).Order("revision_no DESC").Find(&rows)
	httpx.OK(c, rows)
}

// IntegrationUpgrade godoc
// @Summary Upgrade an integration with compatibility gate, smoke test and automatic rollback
// @Tags VisionAI Operations
// @Security BearerAuth
// @Param instanceId path int true "Integration instance ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/integrations/{instanceId}/upgrade [post]
func (h *Handler) IntegrationUpgrade(c *gin.Context) {
	if !h.userHasSystemRole(tenantID(c), c.GetUint64("user_id"), "OPS", "PROJECT_OWNER") {
		httpx.Fail(c, 403, 403, "仅运维或项目负责人可升级集成")
		return
	}
	id, _ := strconv.ParseUint(c.Param("instanceId"), 10, 64)
	var instance IntegrationInstance
	if h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&instance).Error != nil {
		httpx.Fail(c, 404, 404, "集成实例不存在")
		return
	}
	var req integrationUpgradeRequest
	if c.ShouldBindJSON(&req) != nil || normalizedSemver(req.Version) == "" {
		httpx.Fail(c, 400, 400, "目标版本必须是有效语义版本")
		return
	}
	if req.SecretRef != "" && (strings.Contains(req.SecretRef, "://") || len(req.SecretRef) > 512) {
		httpx.Fail(c, 400, 400, "Secret 仅允许保存引用")
		return
	}
	decision, rule := h.compatibilityDecision(instance.TenantID, instance.ProviderType, req.Version)
	if decision != "ALLOWED" {
		httpx.Fail(c, 409, 409, fmt.Sprintf("兼容性矩阵决策为 %s，升级已阻止", decision))
		return
	}
	before := instance
	candidate := instance
	candidate.Version = strings.TrimPrefix(normalizedSemver(req.Version), "v")
	if strings.TrimSpace(req.BaseURL) != "" {
		candidate.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	}
	if req.SecretRef != "" {
		candidate.SecretRef = req.SecretRef
	}
	if req.Capabilities != nil {
		candidate.Capabilities = jsonValue(req.Capabilities)
	}
	beforeConfig, afterConfig := integrationConfigSnapshot(before), integrationConfigSnapshot(candidate)
	var maxRevision int
	h.db.Model(&IntegrationConfigRevision{}).Where("instance_id = ?", instance.ID).
		Select("COALESCE(MAX(revision_no),0)").Scan(&maxRevision)
	smoke, smokeErr := probeIntegration(candidate, req.BaseURL == "", req.FaultInjection)
	status := "ACTIVE"
	rollbackReason := ""
	if smokeErr != nil {
		status, rollbackReason = "ROLLED_BACK", smokeErr.Error()
	}
	ruleID := uint64(0)
	if rule != nil {
		ruleID = rule.ID
	}
	revision := IntegrationConfigRevision{
		TenantID: instance.TenantID, InstanceID: instance.ID, RevisionNo: maxRevision + 1,
		FromVersion: before.Version, ToVersion: candidate.Version,
		BeforeConfig: beforeConfig, AfterConfig: afterConfig,
		BeforeSHA256: digestBytes([]byte(beforeConfig)), AfterSHA256: digestBytes([]byte(afterConfig)),
		Diff: integrationConfigDiff(before, candidate), Compatibility: decision,
		SmokeResult: jsonValue(smoke), Status: status, RollbackReason: rollbackReason,
		CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if smokeErr == nil {
			candidate.Status, candidate.LastError = "HEALTHY", ""
			now := time.Now()
			candidate.LastCheckedAt = &now
			if err := tx.Save(&candidate).Error; err != nil {
				return err
			}
		} else {
			// Persist the exact pre-upgrade snapshot as the active configuration.
			before.LastError = ""
			if err := tx.Save(&before).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&revision).Error; err != nil {
			return err
		}
		if smokeErr != nil {
			incident := SyncIncident{
				TenantID: instance.TenantID, InstanceID: instance.ID, ResourceType: "INTEGRATION_UPGRADE",
				ResourceID: strconv.FormatUint(revision.ID, 10), Code: "UPGRADE_SMOKE_FAILED",
				Message: smokeErr.Error(), Status: "OPEN",
			}
			if err := tx.Create(&incident).Error; err != nil {
				return err
			}
		}
		action := "INTEGRATION_UPGRADED"
		if smokeErr != nil {
			action = "INTEGRATION_UPGRADE_ROLLED_BACK"
		}
		return appendAudit(tx, c, 0, action, "INTEGRATION", instance.ID, gin.H{
			"config": beforeConfig, "sha256": revision.BeforeSHA256,
		}, gin.H{
			"config": afterConfig, "sha256": revision.AfterSHA256, "status": status,
			"compatibilityRuleId": ruleID, "smoke": smoke,
		})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "集成升级事务失败")
		return
	}
	httpx.OK(c, gin.H{"instance": func() IntegrationInstance {
		if smokeErr == nil {
			return candidate
		}
		return before
	}(), "revision": revision, "rolledBack": smokeErr != nil})
}

type syncIncidentActionRequest struct {
	Action        string `json:"action"`
	NewExternalID string `json:"newExternalId"`
	Reason        string `json:"reason"`
}

// SyncIncidentAction godoc
// @Summary Replay, rebind, ignore or manually close a synchronization incident
// @Tags VisionAI Operations
// @Security BearerAuth
// @Param incidentId path int true "Sync incident ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/sync-incidents/{incidentId}/action [post]
func (h *Handler) SyncIncidentAction(c *gin.Context) {
	var req syncIncidentActionRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "同步事件处置参数无效")
		return
	}
	h.syncIncidentAction(c, req)
}

func (h *Handler) syncIncidentAction(c *gin.Context, req syncIncidentActionRequest) {
	if !h.userHasSystemRole(tenantID(c), c.GetUint64("user_id"), "OPS", "PROJECT_OWNER") {
		httpx.Fail(c, 403, 403, "仅运维或项目负责人可处置同步事件")
		return
	}
	id, _ := strconv.ParseUint(c.Param("incidentId"), 10, 64)
	var row SyncIncident
	if h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "同步事件不存在")
		return
	}
	before := row
	req.Action = strings.ToUpper(strings.TrimSpace(req.Action))
	if req.Action == "" {
		req.Action = "REPLAY"
	}
	now := time.Now()
	switch req.Action {
	case "REPLAY":
		row.Attempts++
		row.LastAttemptAt = &now
		var instance IntegrationInstance
		if h.db.Where("tenant_id = ? AND id = ?", row.TenantID, row.InstanceID).First(&instance).Error != nil {
			httpx.Fail(c, 409, 409, "关联集成实例不存在")
			return
		}
		smoke, err := probeIntegration(instance, true, false)
		row.Message = jsonValue(smoke)
		if err == nil {
			row.Status, row.ResolvedAt, row.ResolvedBy = "RESOLVED", &now, c.GetUint64("user_id")
			row.ResolutionAction, row.ResolutionReason = "REPLAY", strings.TrimSpace(req.Reason)
		} else {
			row.Status = "OPEN"
			row.ResolutionAction = "REPLAY_FAILED"
		}
	case "REBIND":
		if strings.TrimSpace(req.NewExternalID) == "" || strings.TrimSpace(req.Reason) == "" {
			httpx.Fail(c, 400, 400, "重新绑定需要新的外部 ID 和原因")
			return
		}
		row.ExternalID, row.Status = strings.TrimSpace(req.NewExternalID), "RESOLVED"
		row.ResolutionAction, row.ResolutionReason = "REBIND", strings.TrimSpace(req.Reason)
		row.ResolvedAt, row.ResolvedBy = &now, c.GetUint64("user_id")
	case "IGNORE":
		if strings.TrimSpace(req.Reason) == "" {
			httpx.Fail(c, 400, 400, "忽略事件必须填写原因")
			return
		}
		row.Status, row.ResolutionAction, row.ResolutionReason = "IGNORED", "IGNORE", strings.TrimSpace(req.Reason)
		row.ResolvedAt, row.ResolvedBy = &now, c.GetUint64("user_id")
	case "CLOSE":
		if strings.TrimSpace(req.Reason) == "" {
			httpx.Fail(c, 400, 400, "人工关闭必须填写原因")
			return
		}
		row.Status, row.ResolutionAction, row.ResolutionReason = "CLOSED", "CLOSE", strings.TrimSpace(req.Reason)
		row.ResolvedAt, row.ResolvedBy = &now, c.GetUint64("user_id")
	default:
		httpx.Fail(c, 400, 400, "处置动作必须是 REPLAY、REBIND、IGNORE 或 CLOSE")
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, 0, "SYNC_INCIDENT_"+req.Action, "SYNC_INCIDENT", row.ID, before, row)
	}); err != nil {
		httpx.Fail(c, 500, 500, "同步事件处置失败")
		return
	}
	httpx.OK(c, row)
}
