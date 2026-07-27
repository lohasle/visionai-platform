package visionai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

type licenseDeclarationRequest struct {
	ComponentType  string `json:"componentType"`
	ComponentName  string `json:"componentName"`
	LicenseID      string `json:"licenseId"`
	SourceURI      string `json:"sourceUri"`
	UseDeclaration string `json:"useDeclaration"`
	Decision       string `json:"decision"`
}

var requiredLicenseComponents = []string{"DATA", "PRETRAINED_WEIGHT", "FRAMEWORK", "MODEL_ARTIFACT"}

func normalizeLicenseDeclarations(rows []licenseDeclarationRequest) ([]licenseDeclarationRequest, error) {
	byType := make(map[string]licenseDeclarationRequest, len(rows))
	for _, row := range rows {
		row.ComponentType = strings.ToUpper(strings.TrimSpace(row.ComponentType))
		row.ComponentName = strings.TrimSpace(row.ComponentName)
		row.LicenseID = strings.TrimSpace(row.LicenseID)
		row.UseDeclaration = strings.TrimSpace(row.UseDeclaration)
		row.Decision = strings.ToUpper(strings.TrimSpace(row.Decision))
		if row.ComponentType == "" || row.ComponentName == "" || row.LicenseID == "" || row.UseDeclaration == "" {
			return nil, fmt.Errorf("许可组件、名称、许可证标识和用途声明均为必填")
		}
		if row.Decision != "ALLOWED" && row.Decision != "REVIEW_REQUIRED" && row.Decision != "NOT_ALLOWED" {
			return nil, fmt.Errorf("许可结论必须为 ALLOWED、REVIEW_REQUIRED 或 NOT_ALLOWED")
		}
		if _, exists := byType[row.ComponentType]; exists {
			return nil, fmt.Errorf("许可组件 %s 重复", row.ComponentType)
		}
		byType[row.ComponentType] = row
	}
	normalized := make([]licenseDeclarationRequest, 0, len(requiredLicenseComponents))
	for _, component := range requiredLicenseComponents {
		row, exists := byType[component]
		if !exists {
			return nil, fmt.Errorf("缺少 %s 许可声明", component)
		}
		normalized = append(normalized, row)
	}
	return normalized, nil
}

func aggregateLicenseDecision(rows []licenseDeclarationRequest) string {
	result := "ALLOWED"
	for _, row := range rows {
		if row.Decision == "NOT_ALLOWED" {
			return "NOT_ALLOWED"
		}
		if row.Decision == "REVIEW_REQUIRED" {
			result = "REVIEW_REQUIRED"
		}
	}
	return result
}

func trainingLicenseDeclarations(template TrainingTemplateVersion, dataset DatasetVersion, training TrainingRun, modelName string) []licenseDeclarationRequest {
	policy := make(map[string]any)
	_ = json.Unmarshal([]byte(template.LicensePolicy), &policy)
	decision := "ALLOWED"
	if allowed, exists := policy["allowed"].(bool); exists && !allowed {
		decision = "NOT_ALLOWED"
	}
	stringValue := func(key, fallback string) string {
		if value, ok := policy[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	pretrained := strings.TrimSpace(training.PretrainedRef)
	if pretrained == "" {
		pretrained = "未使用外部预训练权重"
	}
	use := "仅用于本项目已审批的数据训练、评估和受控生产部署"
	return []licenseDeclarationRequest{
		{ComponentType: "DATA", ComponentName: fmt.Sprintf("DatasetVersion-%d", dataset.ID), LicenseID: stringValue("dataset", "PROJECT_DATA_POLICY"), SourceURI: dataset.DatasetCardURI, UseDeclaration: use, Decision: decision},
		{ComponentType: "PRETRAINED_WEIGHT", ComponentName: pretrained, LicenseID: stringValue("pretrainedWeight", "NO_EXTERNAL_WEIGHT"), SourceURI: training.PretrainedRef, UseDeclaration: use, Decision: decision},
		{ComponentType: "FRAMEWORK", ComponentName: template.Trainer + " / " + template.ImageRef, LicenseID: stringValue("framework", "FRAMEWORK_IMAGE_DECLARATION"), SourceURI: template.ImageRef, UseDeclaration: use, Decision: decision},
		{ComponentType: "MODEL_ARTIFACT", ComponentName: modelName, LicenseID: stringValue("modelArtifact", "PROJECT_MODEL_POLICY"), UseDeclaration: use, Decision: decision},
	}
}

func storageKeyFromURI(uri string) (string, bool) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", false
	}
	rest := strings.TrimPrefix(uri, "s3://")
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 || slash == len(rest)-1 {
		return "", false
	}
	return rest[slash+1:], true
}

func (h *Handler) validateImportedArtifact(c *gin.Context, artifact modelImportArtifact) error {
	key, ok := storageKeyFromURI(strings.TrimSpace(artifact.URI))
	if !ok {
		return fmt.Errorf("受控导入仅接受平台对象存储 s3:// URI")
	}
	reader, info, err := h.storage.Get(c.Request.Context(), key)
	if err != nil {
		return fmt.Errorf("读取制品 %s: %w", artifact.Name, err)
	}
	defer reader.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, reader)
	if err != nil {
		return fmt.Errorf("校验制品 %s: %w", artifact.Name, err)
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, artifact.SHA256) {
		return fmt.Errorf("制品 %s 的 SHA-256 不匹配", artifact.Name)
	}
	if artifact.Size > 0 && artifact.Size != size {
		return fmt.Errorf("制品 %s 的大小不匹配", artifact.Name)
	}
	if info.Size != size {
		return fmt.Errorf("制品 %s 的对象存储元数据异常", artifact.Name)
	}
	return nil
}

// ModelLicensesReplace godoc
// @Summary Replace and review the four model supply-chain license declarations
// @Tags VisionAI Model Governance
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param versionId path int true "Model version ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/model-versions/{versionId}/licenses [put]
func (h *Handler) ModelLicensesReplace(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "REVIEWER", "PROJECT_OWNER") {
		return
	}
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 64)
	var version ModelVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, versionID).First(&version).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "模型版本不存在")
		return
	}
	var req struct {
		Licenses []licenseDeclarationRequest `json:"licenses"`
	}
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, http.StatusBadRequest, 400, "许可声明参数无效")
		return
	}
	rows, err := normalizeLicenseDeclarations(req.Licenses)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	decision := aggregateLicenseDecision(rows)
	before := version
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND project_id = ? AND model_version_id = ?", project.TenantID, project.ID, version.ID).
			Delete(&ModelLicenseDeclaration{}).Error; err != nil {
			return err
		}
		for _, item := range rows {
			row := ModelLicenseDeclaration{
				TenantID: project.TenantID, ProjectID: project.ID, ModelVersionID: version.ID,
				ComponentType: item.ComponentType, ComponentName: item.ComponentName, LicenseID: item.LicenseID,
				SourceURI: strings.TrimSpace(item.SourceURI), UseDeclaration: item.UseDeclaration,
				Decision: item.Decision, ReviewedBy: c.GetUint64("user_id"),
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		updates := map[string]any{
			"license_decision": decision, "status": "EVALUATED",
			"approved_by": 0, "approved_at": nil, "preannotation_approved": false,
		}
		if version.Status == "DRAFT" {
			delete(updates, "status")
		}
		if err := tx.Model(&version).Updates(updates).Error; err != nil {
			return err
		}
		if err := invalidateApprovals(tx, project, version.ID, "LICENSE_DECLARATION_CHANGED"); err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "MODEL_LICENSES_REVIEWED", "MODEL_VERSION", version.ID, before, gin.H{
			"decision": decision, "components": rows,
		})
	})
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, 500, "许可声明保存失败")
		return
	}
	h.db.First(&version, version.ID)
	httpx.OK(c, version)
}

func invalidateApprovals(tx *gorm.DB, project Project, versionID uint64, reason string) error {
	now := time.Now()
	return tx.Model(&ApprovalRequest{}).
		Where("tenant_id = ? AND project_id = ? AND model_version_id = ? AND status IN ?", project.TenantID, project.ID, versionID, []string{"PENDING", "IN_PROGRESS", "APPROVED"}).
		Updates(map[string]any{"status": "INVALIDATED", "invalidated_at": now, "invalidation_reason": reason}).Error
}

type approvalTemplateStep struct {
	Name         string `json:"name"`
	RequiredRole string `json:"requiredRole"`
}

type approvalTemplateRequest struct {
	Name              string                 `json:"name"`
	ApprovalType      string                 `json:"approvalType"`
	TargetEnvironment string                 `json:"targetEnvironment"`
	Steps             []approvalTemplateStep `json:"steps"`
	AllowSelfApproval bool                   `json:"allowSelfApproval"`
	Enabled           *bool                  `json:"enabled"`
}

func normalizeApprovalSteps(steps []approvalTemplateStep) ([]approvalTemplateStep, error) {
	if len(steps) == 0 || len(steps) > 10 {
		return nil, fmt.Errorf("审批模板必须包含 1 至 10 个步骤")
	}
	allowedRoles := map[string]bool{"REVIEWER": true, "APPROVER": true, "OPS": true, "PROJECT_OWNER": true}
	for i := range steps {
		steps[i].Name = strings.TrimSpace(steps[i].Name)
		steps[i].RequiredRole = strings.ToUpper(strings.TrimSpace(steps[i].RequiredRole))
		if steps[i].Name == "" || !allowedRoles[steps[i].RequiredRole] {
			return nil, fmt.Errorf("第 %d 步的名称或系统角色无效", i+1)
		}
	}
	return steps, nil
}

// ApprovalTemplatePage godoc
// @Summary List tenant approval workflow templates
// @Tags VisionAI Model Governance
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/approval-templates [get]
func (h *Handler) ApprovalTemplatePage(c *gin.Context) {
	var rows []ApprovalTemplate
	query := h.db.Where("tenant_id = ?", tenantID(c))
	if c.Query("enabled") == "true" {
		query = query.Where("enabled = ?", true)
	}
	query.Order("id DESC").Find(&rows)
	httpx.OK(c, rows)
}

// ApprovalTemplateCreate godoc
// @Summary Create a tenant sequential approval workflow template
// @Tags VisionAI Model Governance
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/approval-templates [post]
func (h *Handler) ApprovalTemplateCreate(c *gin.Context) {
	if !h.userHasSystemRole(tenantID(c), c.GetUint64("user_id"), "PROJECT_OWNER", "APPROVER", "SUPER_ADMIN") {
		httpx.Fail(c, http.StatusForbidden, 403, "仅项目负责人或审批人可管理租户审批模板")
		return
	}
	var req approvalTemplateRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, http.StatusBadRequest, 400, "审批模板名称和步骤必填")
		return
	}
	steps, err := normalizeApprovalSteps(req.Steps)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.ApprovalType == "" {
		req.ApprovalType = "PRODUCTION_QUALIFICATION"
	}
	if req.TargetEnvironment == "" {
		req.TargetEnvironment = "PRODUCTION"
	}
	if req.AllowSelfApproval && !h.userHasSystemRole(tenantID(c), c.GetUint64("user_id"), "SUPER_ADMIN") {
		httpx.Fail(c, http.StatusForbidden, 403, "仅超级管理员可配置职责分离例外")
		return
	}
	row := ApprovalTemplate{
		TenantID: tenantID(c), Name: strings.TrimSpace(req.Name),
		ApprovalType: strings.ToUpper(req.ApprovalType), TargetEnvironment: strings.ToUpper(req.TargetEnvironment),
		Steps: jsonValue(steps), AllowSelfApproval: req.AllowSelfApproval,
		Enabled: enabled, CreatedBy: c.GetUint64("user_id"),
	}
	if h.db.Create(&row).Error != nil {
		httpx.Fail(c, http.StatusConflict, 409, "审批模板名称冲突")
		return
	}
	_ = appendAudit(h.db, c, 0, "APPROVAL_TEMPLATE_CREATED", "APPROVAL_TEMPLATE", row.ID, nil, row)
	httpx.OK(c, row)
}

func decodeApprovalSteps(raw string) []approvalTemplateStep {
	var steps []approvalTemplateStep
	if json.Unmarshal([]byte(raw), &steps) == nil && len(steps) > 0 {
		return steps
	}
	var snapshot struct {
		Steps []approvalTemplateStep `json:"steps"`
	}
	if json.Unmarshal([]byte(raw), &snapshot) == nil && len(snapshot.Steps) > 0 {
		return snapshot.Steps
	}
	return []approvalTemplateStep{{Name: "合规与技术复核", RequiredRole: "REVIEWER"}}
}

func approvalSnapshotAllowsSelf(raw string) bool {
	var snapshot struct {
		AllowSelfApproval bool `json:"allowSelfApproval"`
	}
	return json.Unmarshal([]byte(raw), &snapshot) == nil && snapshot.AllowSelfApproval
}

func (h *Handler) approvalFingerprint(version ModelVersion, targetEnvironment string) (string, error) {
	var artifacts []ModelArtifact
	var licenses []ModelLicenseDeclaration
	if err := h.db.Where("model_version_id = ?", version.ID).Order("id").Find(&artifacts).Error; err != nil {
		return "", err
	}
	if err := h.db.Where("model_version_id = ?", version.ID).Order("component_type").Find(&licenses).Error; err != nil {
		return "", err
	}
	input := struct {
		ModelCardSHA256   string                    `json:"modelCardSha256"`
		SupplyChainSHA256 string                    `json:"supplyChainSha256"`
		TargetEnvironment string                    `json:"targetEnvironment"`
		Artifacts         []ModelArtifact           `json:"artifacts"`
		Licenses          []ModelLicenseDeclaration `json:"licenses"`
	}{
		ModelCardSHA256: version.ModelCardSHA256, SupplyChainSHA256: version.SupplyChainSHA256,
		TargetEnvironment: strings.ToUpper(targetEnvironment), Artifacts: artifacts, Licenses: licenses,
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return digestBytes(raw), nil
}

func (h *Handler) buildApprovalEvidence(version ModelVersion, targetEnvironment, rollbackPlan, riskSummary string) (string, error) {
	var evaluation EvaluationRun
	var failures []EvaluationSample
	var artifacts []ModelArtifact
	var licenses []ModelLicenseDeclaration
	h.db.First(&evaluation, version.EvaluationRunID)
	h.db.Where("evaluation_run_id = ? AND error_type <> ?", evaluation.ID, "").Order("confidence DESC,id").Limit(20).Find(&failures)
	h.db.Where("model_version_id = ?", version.ID).Order("id").Find(&artifacts)
	h.db.Where("model_version_id = ?", version.ID).Order("component_type").Find(&licenses)
	sort.Slice(failures, func(i, j int) bool { return failures[i].Confidence > failures[j].Confidence })
	return jsonValue(gin.H{
		"schemaVersion": "2.0", "modelVersionId": version.ID,
		"modelCardSha256": version.ModelCardSHA256, "supplyChainSha256": version.SupplyChainSHA256,
		"targetEnvironment": strings.ToUpper(targetEnvironment), "licenseDecision": version.LicenseDecision,
		"licenses": licenses, "artifacts": artifacts, "evaluationRunId": evaluation.ID,
		"gateDecision": evaluation.GateDecision, "evaluationSummary": json.RawMessage(evaluation.Summary),
		"failureSamples": failures, "riskSummary": strings.TrimSpace(riskSummary),
		"rollbackPlan": strings.TrimSpace(rollbackPlan),
	}), nil
}

type modelExportRequest struct {
	Purpose          string `json:"purpose"`
	IncludeArtifacts bool   `json:"includeArtifacts"`
}

// ModelExport godoc
// @Summary Export an approved model package with purpose and downloader audit
// @Tags VisionAI Model Governance
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param versionId path int true "Model version ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/model-versions/{versionId}/export [post]
func (h *Handler) ModelExport(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "OPS", "PROJECT_OWNER") {
		return
	}
	var req modelExportRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Purpose) == "" {
		httpx.Fail(c, http.StatusBadRequest, 400, "导出用途必填")
		return
	}
	h.modelExport(c, project, req)
}

func (h *Handler) modelExport(c *gin.Context, project Project, req modelExportRequest) {
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 64)
	var version ModelVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, versionID).First(&version).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "模型版本不存在")
		return
	}
	if version.Status != "APPROVED" {
		httpx.Fail(c, http.StatusConflict, 409, "仅已审批模型版本可导出")
		return
	}
	var approval ApprovalRequest
	if h.db.Where("model_version_id = ? AND target_environment = ? AND status = ?", version.ID, "PRODUCTION", "APPROVED").
		Order("id DESC").First(&approval).Error != nil {
		httpx.Fail(c, http.StatusConflict, 409, "生产导出审批不存在或已失效")
		return
	}
	fingerprint, err := h.approvalFingerprint(version, approval.TargetEnvironment)
	if err != nil || fingerprint != approval.InputFingerprint {
		_ = invalidateApprovals(h.db, project, version.ID, "CONTROLLED_INPUT_CHANGED")
		httpx.Fail(c, http.StatusConflict, 409, "受控输入已变化，原审批自动失效")
		return
	}
	var artifacts []ModelArtifact
	h.db.Where("model_version_id = ?", version.ID).Order("id").Find(&artifacts)
	payloadArtifacts := artifacts
	if !req.IncludeArtifacts {
		payloadArtifacts = nil
	}
	audit := gin.H{
		"purpose": strings.TrimSpace(req.Purpose), "downloaderUserId": c.GetUint64("user_id"),
		"artifactCount": len(artifacts), "approvalRequestId": approval.ID,
		"inputFingerprint": fingerprint,
	}
	_ = appendAudit(h.db, c, project.ID, "MODEL_PACKAGE_EXPORTED", "MODEL_VERSION", version.ID, nil, audit)
	httpx.OK(c, gin.H{
		"schemaVersion": "2.0", "purpose": req.Purpose, "version": version,
		"artifacts": payloadArtifacts, "artifactManifest": artifacts,
		"approvalRequestId": approval.ID, "inputFingerprint": fingerprint,
	})
}

func hasRequiredRole(h *Handler, tenant, user uint64, role string) bool {
	return h.userHasSystemRole(tenant, user, role, "PROJECT_OWNER")
}

func saveApprovalInvalidation(tx *gorm.DB, approval *ApprovalRequest, version *ModelVersion, reason string) error {
	now := time.Now()
	approval.Status, approval.InvalidatedAt, approval.InvalidationReason = "INVALIDATED", &now, reason
	if err := tx.Save(approval).Error; err != nil {
		return err
	}
	return tx.Model(version).Updates(map[string]any{
		"status": "EVALUATED", "approved_by": 0, "approved_at": nil, "preannotation_approved": false,
	}).Error
}

func approvalTemplateForSubmit(db *gorm.DB, tenant uint64, templateID uint64, approvalType, target string) (ApprovalTemplate, []approvalTemplateStep, error) {
	var row ApprovalTemplate
	query := db.Where("tenant_id = ? AND enabled = ?", tenant, true)
	if templateID != 0 {
		query = query.Where("id = ?", templateID)
	} else {
		query = query.Where("approval_type = ? AND target_environment = ?", strings.ToUpper(approvalType), strings.ToUpper(target)).Order("id DESC")
	}
	if err := query.First(&row).Error; err != nil {
		if templateID != 0 || err != gorm.ErrRecordNotFound {
			return row, nil, err
		}
		steps := []approvalTemplateStep{{Name: "合规与技术复核", RequiredRole: "REVIEWER"}}
		row = ApprovalTemplate{
			TenantID: tenant, Name: "系统默认单步审批", ApprovalType: strings.ToUpper(approvalType),
			TargetEnvironment: strings.ToUpper(target), Steps: jsonValue(steps), Enabled: true,
		}
		return row, steps, nil
	}
	return row, decodeApprovalSteps(row.Steps), nil
}
