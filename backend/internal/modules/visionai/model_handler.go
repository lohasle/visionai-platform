package visionai

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

type modelRegisterRequest struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	TrainingRunID   uint64 `json:"trainingRunId"`
	EvaluationRunID uint64 `json:"evaluationRunId"`
	SemanticVersion string `json:"semanticVersion"`
}

type modelImportArtifact struct {
	Name      string `json:"name"`
	Format    string `json:"format"`
	URI       string `json:"uri"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	MediaType string `json:"mediaType"`
}

type modelImportRequest struct {
	Name             string                      `json:"name"`
	Description      string                      `json:"description"`
	SemanticVersion  string                      `json:"semanticVersion"`
	AIType           string                      `json:"aiType"`
	LicenseDecision  string                      `json:"licenseDecision"`
	Licenses         []licenseDeclarationRequest `json:"licenses"`
	DatasetVersionID uint64                      `json:"datasetVersionId"`
	Artifacts        []modelImportArtifact       `json:"artifacts"`
}

func (h *Handler) ModelImport(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "PROJECT_OWNER") {
		return
	}
	var req modelImportRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" || req.SemanticVersion == "" || len(req.Artifacts) == 0 {
		httpx.Fail(c, 400, 400, "受控导入需要名称、语义版本和至少一个制品")
		return
	}
	for _, artifact := range req.Artifacts {
		format := strings.ToUpper(artifact.Format)
		if artifact.Name == "" || artifact.URI == "" || len(artifact.SHA256) != 64 || (format != "ONNX" && format != "TENSORRT" && format != "ENGINE" && format != "PT" && format != "SOURCE") {
			httpx.Fail(c, 400, 400, "制品必须包含受支持格式、URI 与 64 位 SHA-256")
			return
		}
		if err := h.validateImportedArtifact(c, artifact); err != nil {
			httpx.Fail(c, 400, 400, err.Error())
			return
		}
	}
	licenses, licenseErr := normalizeLicenseDeclarations(req.Licenses)
	if licenseErr != nil {
		httpx.Fail(c, 400, 400, licenseErr.Error())
		return
	}
	req.LicenseDecision = aggregateLicenseDecision(licenses)
	if req.AIType == "" {
		req.AIType = "OBJECT_DETECTION"
	}
	userID := c.GetUint64("user_id")
	model := Model{TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name), AIType: req.AIType, Description: req.Description, CreatedBy: userID}
	if err := h.db.Where("tenant_id = ? AND project_id = ? AND name = ?", project.TenantID, project.ID, model.Name).FirstOrCreate(&model).Error; err != nil {
		httpx.Fail(c, 409, 409, "模型名称冲突")
		return
	}
	var maxVersion int
	h.db.Model(&ModelVersion{}).Where("model_id = ?", model.ID).Select("COALESCE(MAX(version_no),0)").Scan(&maxVersion)
	supply := jsonValue(gin.H{"schemaVersion": "2.0", "sourceType": "CONTROLLED_IMPORT", "artifacts": req.Artifacts, "licenses": licenses, "licenseDecision": req.LicenseDecision})
	card := jsonValue(gin.H{"name": model.Name, "version": req.SemanticVersion, "task": model.AIType, "status": "DRAFT", "limitations": []string{"受控导入模型必须完成平台评估与审批后方可生产部署"}})
	version := ModelVersion{
		TenantID: project.TenantID, ProjectID: project.ID, ModelID: model.ID, VersionNo: maxVersion + 1,
		SemanticVersion: req.SemanticVersion, SourceType: "CONTROLLED_IMPORT", DatasetVersionID: req.DatasetVersionID,
		Status: "DRAFT", ModelCardSHA256: digestBytes([]byte(card)), SupplyChainSHA256: digestBytes([]byte(supply)),
		LicenseDecision: req.LicenseDecision, CreatedBy: userID,
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		root := assetRoot(project.TenantID, project.ID) + "/models/" + strconv.FormatUint(model.ID, 10) + "/versions/" + strconv.Itoa(version.VersionNo)
		cardKey, supplyKey := root+"/model-card.json", root+"/supply-chain.json"
		if err := h.storage.Put(c.Request.Context(), cardKey, strings.NewReader(card), int64(len(card)), "application/json"); err != nil {
			return err
		}
		if err := h.storage.Put(c.Request.Context(), supplyKey, strings.NewReader(supply), int64(len(supply)), "application/json"); err != nil {
			return err
		}
		version.ModelCardURI, version.SupplyChainURI = h.storage.URI(cardKey), h.storage.URI(supplyKey)
		if err := tx.Save(&version).Error; err != nil {
			return err
		}
		for _, artifact := range req.Artifacts {
			if err := tx.Create(&ModelArtifact{
				TenantID: project.TenantID, ProjectID: project.ID, ModelVersionID: version.ID,
				Name: artifact.Name, Format: strings.ToUpper(artifact.Format), URI: artifact.URI,
				SHA256: strings.ToLower(artifact.SHA256), Size: artifact.Size, MediaType: artifact.MediaType,
			}).Error; err != nil {
				return err
			}
		}
		for _, item := range licenses {
			if err := tx.Create(&ModelLicenseDeclaration{
				TenantID: project.TenantID, ProjectID: project.ID, ModelVersionID: version.ID,
				ComponentType: item.ComponentType, ComponentName: item.ComponentName, LicenseID: item.LicenseID,
				SourceURI: item.SourceURI, UseDeclaration: item.UseDeclaration, Decision: item.Decision,
				ReviewedBy: userID,
			}).Error; err != nil {
				return err
			}
		}
		return appendAudit(tx, c, project.ID, "MODEL_VERSION_IMPORTED", "MODEL_VERSION", version.ID, nil, gin.H{"artifactCount": len(req.Artifacts), "licenseDecision": req.LicenseDecision})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "模型受控导入失败")
		return
	}
	httpx.OK(c, version)
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func (h *Handler) ModelPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var models []Model
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Order("id DESC").Find(&models)
	var versions []ModelVersion
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Order("id DESC").Find(&versions)
	httpx.OK(c, gin.H{"models": models, "versions": versions})
}

func (h *Handler) ModelRegister(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "PROJECT_OWNER") {
		return
	}
	var req modelRegisterRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" || req.TrainingRunID == 0 || req.EvaluationRunID == 0 {
		httpx.Fail(c, 400, 400, "名称、成功训练运行和通过门禁的评估运行必填")
		return
	}
	var training TrainingRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, req.TrainingRunID, TrainingSucceeded).First(&training).Error != nil {
		httpx.Fail(c, 409, 409, "训练运行未成功")
		return
	}
	var evaluation EvaluationRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND training_run_id = ? AND status = ? AND gate_decision = ?", project.TenantID, project.ID, req.EvaluationRunID, training.ID, "SUCCEEDED", "PASSED").First(&evaluation).Error != nil {
		httpx.Fail(c, 409, 409, "评估运行未成功通过门禁或与训练运行不匹配")
		return
	}
	var dataset DatasetVersion
	var template TrainingTemplateVersion
	if h.db.First(&dataset, training.DatasetVersionID).Error != nil || h.db.First(&template, training.TemplateVersionID).Error != nil {
		httpx.Fail(c, 409, 409, "训练血缘不完整")
		return
	}
	var trainingArtifacts []TrainingArtifact
	h.db.Where("tenant_id = ? AND training_run_id = ?", project.TenantID, training.ID).Order("id").Find(&trainingArtifacts)
	if len(trainingArtifacts) == 0 {
		httpx.Fail(c, 409, 409, "训练制品缺失")
		return
	}
	userID := c.GetUint64("user_id")
	model := Model{TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name), AIType: "OBJECT_DETECTION", Description: strings.TrimSpace(req.Description), CreatedBy: userID}
	err := h.db.Where("tenant_id = ? AND project_id = ? AND name = ?", project.TenantID, project.ID, model.Name).FirstOrCreate(&model).Error
	if err != nil {
		httpx.Fail(c, 409, 409, "模型名称冲突")
		return
	}
	var maxVersion int
	h.db.Model(&ModelVersion{}).Where("model_id = ?", model.ID).Select("COALESCE(MAX(version_no),0)").Scan(&maxVersion)
	if req.SemanticVersion == "" {
		req.SemanticVersion = fmt.Sprintf("1.%d.0", maxVersion)
	}
	licenses := trainingLicenseDeclarations(template, dataset, training, model.Name)
	licenseDecision := aggregateLicenseDecision(licenses)
	card := map[string]any{
		"name": model.Name, "version": req.SemanticVersion, "task": model.AIType,
		"datasetVersionId": dataset.ID, "datasetChecksum": dataset.Checksum,
		"trainingRunId": training.ID, "evaluationRunId": evaluation.ID,
		"evaluation": json.RawMessage(evaluation.Summary), "limitations": []string{"仅适用于已验证的缺陷检测场景；生产上线前需审批"},
	}
	supply := map[string]any{
		"schemaVersion": "2.0", "dataset": map[string]any{"id": dataset.ID, "manifest": dataset.ManifestURI, "checksum": dataset.Checksum},
		"training":   map[string]any{"id": training.ID, "image": template.ImageRef, "codeCommit": training.CodeCommit, "parameters": json.RawMessage(training.Parameters)},
		"evaluation": map[string]any{"id": evaluation.ID, "gateDecision": evaluation.GateDecision, "evaluator": "visionai-evaluator/1.0.0"},
		"artifacts":  trainingArtifacts, "licenses": licenses, "licenseDecision": licenseDecision,
	}
	cardRaw, _ := json.MarshalIndent(card, "", "  ")
	supplyRaw, _ := json.MarshalIndent(supply, "", "  ")
	version := ModelVersion{
		TenantID: project.TenantID, ProjectID: project.ID, ModelID: model.ID, VersionNo: maxVersion + 1,
		SemanticVersion: req.SemanticVersion, SourceType: "TRAINING_RUN", TrainingRunID: training.ID,
		EvaluationRunID: evaluation.ID, DatasetVersionID: dataset.ID, Status: "EVALUATED",
		ModelCardSHA256: digestBytes(cardRaw), SupplyChainSHA256: digestBytes(supplyRaw),
		LicenseDecision: licenseDecision, CreatedBy: userID,
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		root := assetRoot(project.TenantID, project.ID) + "/models/" + strconv.FormatUint(model.ID, 10) + "/versions/" + strconv.Itoa(version.VersionNo)
		cardKey, supplyKey := root+"/model-card.json", root+"/supply-chain.json"
		if err := h.storage.Put(c.Request.Context(), cardKey, bytes.NewReader(cardRaw), int64(len(cardRaw)), "application/json"); err != nil {
			return err
		}
		if err := h.storage.Put(c.Request.Context(), supplyKey, bytes.NewReader(supplyRaw), int64(len(supplyRaw)), "application/json"); err != nil {
			return err
		}
		version.ModelCardURI, version.SupplyChainURI = h.storage.URI(cardKey), h.storage.URI(supplyKey)
		if err := tx.Save(&version).Error; err != nil {
			return err
		}
		usage := DatasetUsage{
			TenantID: project.TenantID, ProjectID: project.ID, DatasetVersionID: dataset.ID,
			ResourceType: "MODEL_VERSION", ResourceID: version.ID,
		}
		if err := tx.Where(
			"dataset_version_id = ? AND resource_type = ? AND resource_id = ?",
			usage.DatasetVersionID, usage.ResourceType, usage.ResourceID,
		).FirstOrCreate(&usage).Error; err != nil {
			return err
		}
		for _, artifact := range trainingArtifacts {
			format := strings.ToUpper(strings.TrimPrefix(path.Ext(artifact.Name), "."))
			if format == "" {
				format = strings.ToUpper(artifact.Kind)
			}
			row := ModelArtifact{TenantID: project.TenantID, ProjectID: project.ID, ModelVersionID: version.ID, Name: artifact.Name, Format: format, URI: artifact.URI, SHA256: artifact.SHA256, Size: artifact.Size, MediaType: artifact.MediaType}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		for _, item := range licenses {
			if err := tx.Create(&ModelLicenseDeclaration{
				TenantID: project.TenantID, ProjectID: project.ID, ModelVersionID: version.ID,
				ComponentType: item.ComponentType, ComponentName: item.ComponentName, LicenseID: item.LicenseID,
				SourceURI: item.SourceURI, UseDeclaration: item.UseDeclaration, Decision: item.Decision,
				ReviewedBy: userID,
			}).Error; err != nil {
				return err
			}
		}
		return appendAudit(tx, c, project.ID, "MODEL_VERSION_REGISTERED", "MODEL_VERSION", version.ID, nil, gin.H{"trainingRunId": training.ID, "evaluationRunId": evaluation.ID, "supplyChainSha256": version.SupplyChainSHA256})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "模型注册失败："+err.Error())
		return
	}
	httpx.OK(c, version)
}

func (h *Handler) ModelVersionGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	id, _ := strconv.ParseUint(c.Param("versionId"), 10, 64)
	var version ModelVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&version).Error != nil {
		httpx.Fail(c, 404, 404, "模型版本不存在")
		return
	}
	var artifacts []ModelArtifact
	var licenses []ModelLicenseDeclaration
	var approvals []ApprovalRequest
	var decisions []ApprovalDecision
	var stepDecisions []ApprovalStepDecision
	h.db.Where("model_version_id = ?", version.ID).Order("id").Find(&artifacts)
	h.db.Where("model_version_id = ?", version.ID).Order("component_type").Find(&licenses)
	h.db.Where("model_version_id = ?", version.ID).Order("id DESC").Find(&approvals)
	if len(approvals) > 0 {
		ids := make([]uint64, 0, len(approvals))
		for _, approval := range approvals {
			ids = append(ids, approval.ID)
		}
		h.db.Where("approval_request_id IN ?", ids).Order("id").Find(&decisions)
		h.db.Where("approval_request_id IN ?", ids).Order("approval_request_id,step_no").Find(&stepDecisions)
	}
	httpx.OK(c, gin.H{"version": version, "artifacts": artifacts, "licenses": licenses, "approvals": approvals, "decisions": decisions, "stepDecisions": stepDecisions})
}

type approvalSubmitRequest struct {
	ApprovalType      string `json:"approvalType"`
	TargetEnvironment string `json:"targetEnvironment"`
	TemplateID        uint64 `json:"templateId"`
	RiskSummary       string `json:"riskSummary"`
	RollbackPlan      string `json:"rollbackPlan"`
}

// ApprovalSubmit godoc
// @Summary Freeze evidence and submit a model to a tenant approval workflow
// @Tags VisionAI Model Governance
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param versionId path int true "Model version ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/model-versions/{versionId}/approvals [post]
func (h *Handler) ApprovalSubmit(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "PROJECT_OWNER") {
		return
	}
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 64)
	var version ModelVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, versionID, "EVALUATED").First(&version).Error != nil {
		httpx.Fail(c, 409, 409, "仅 EVALUATED 模型可提交审批")
		return
	}
	if version.LicenseDecision != "ALLOWED" {
		httpx.Fail(c, 409, 409, "许可证策略阻止审批")
		return
	}
	var req approvalSubmitRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "审批参数无效")
		return
	}
	if req.ApprovalType == "" {
		req.ApprovalType = "PRODUCTION_QUALIFICATION"
	}
	if req.TargetEnvironment == "" {
		req.TargetEnvironment = "PRODUCTION"
	}
	if strings.TrimSpace(req.RiskSummary) == "" || strings.TrimSpace(req.RollbackPlan) == "" {
		httpx.Fail(c, 400, 400, "风险说明和回滚方案必填")
		return
	}
	var licenseCount int64
	h.db.Model(&ModelLicenseDeclaration{}).Where("model_version_id = ? AND decision = ?", version.ID, "ALLOWED").Count(&licenseCount)
	if licenseCount != int64(len(requiredLicenseComponents)) {
		httpx.Fail(c, 409, 409, "数据、预训练权重、框架和模型制品的许可声明必须全部通过")
		return
	}
	template, steps, err := approvalTemplateForSubmit(h.db, project.TenantID, req.TemplateID, req.ApprovalType, req.TargetEnvironment)
	if err != nil {
		httpx.Fail(c, 409, 409, "审批模板不存在或已停用")
		return
	}
	snapshot, err := h.buildApprovalEvidence(version, req.TargetEnvironment, req.RollbackPlan, req.RiskSummary)
	if err != nil {
		httpx.Fail(c, 500, 500, "审批证据生成失败")
		return
	}
	fingerprint, err := h.approvalFingerprint(version, req.TargetEnvironment)
	if err != nil {
		httpx.Fail(c, 500, 500, "受控输入指纹生成失败")
		return
	}
	templateSnapshot := jsonValue(gin.H{
		"name": template.Name, "steps": steps, "allowSelfApproval": template.AllowSelfApproval,
	})
	row := ApprovalRequest{
		TenantID: project.TenantID, ProjectID: project.ID, ModelVersionID: version.ID,
		ApprovalType: strings.ToUpper(req.ApprovalType), TargetEnvironment: strings.ToUpper(req.TargetEnvironment),
		Status: "PENDING", EvidenceSnapshot: snapshot, EvidenceSHA256: digestBytes([]byte(snapshot)),
		TemplateID: template.ID, TemplateSnapshot: templateSnapshot, TemplateSHA256: digestBytes([]byte(templateSnapshot)),
		InputFingerprint: fingerprint, CurrentStep: 1, TotalSteps: len(steps),
		SubmittedBy: c.GetUint64("user_id"), SubmittedAt: time.Now(),
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&version).Update("status", "REVIEW_PENDING").Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "MODEL_APPROVAL_SUBMITTED", "APPROVAL_REQUEST", row.ID, nil, row)
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "审批提交失败")
		return
	}
	httpx.OK(c, row)
}

type approvalDecisionRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

// ApprovalDecide godoc
// @Summary Decide the current sequential approval step
// @Tags VisionAI Model Governance
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param approvalId path int true "Approval request ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/approvals/{approvalId}/decision [post]
func (h *Handler) ApprovalDecide(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok {
		return
	}
	approvalID, _ := strconv.ParseUint(c.Param("approvalId"), 10, 64)
	var approval ApprovalRequest
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status IN ?", project.TenantID, project.ID, approvalID, []string{"PENDING", "IN_PROGRESS"}).First(&approval).Error != nil {
		httpx.Fail(c, 409, 409, "审批不存在或已经作出决定")
		return
	}
	steps := decodeApprovalSteps(approval.TemplateSnapshot)
	if approval.CurrentStep < 1 || approval.CurrentStep > len(steps) {
		httpx.Fail(c, 409, 409, "审批步骤快照无效")
		return
	}
	current := steps[approval.CurrentStep-1]
	if project.OwnerUserID != c.GetUint64("user_id") && !hasRequiredRole(h, project.TenantID, c.GetUint64("user_id"), current.RequiredRole) {
		httpx.Fail(c, 403, 403, "当前步骤需要系统角色 "+current.RequiredRole)
		return
	}
	userID := c.GetUint64("user_id")
	if approval.SubmittedBy == userID && !approvalSnapshotAllowsSelf(approval.TemplateSnapshot) {
		httpx.Fail(c, 409, 409, "职责分离：提交人不能审批自己的生产资格")
		return
	}
	var req approvalDecisionRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Comment) == "" {
		httpx.Fail(c, 400, 400, "审批意见必填")
		return
	}
	req.Decision = strings.ToUpper(strings.TrimSpace(req.Decision))
	if req.Decision != "APPROVE" && req.Decision != "REJECT" {
		httpx.Fail(c, 400, 400, "决定必须是 APPROVE 或 REJECT")
		return
	}
	var version ModelVersion
	if h.db.Where("id = ?", approval.ModelVersionID).First(&version).Error != nil || version.SupplyChainSHA256 == "" {
		httpx.Fail(c, 409, 409, "审批证据不完整")
		return
	}
	if digestBytes([]byte(approval.EvidenceSnapshot)) != approval.EvidenceSHA256 {
		httpx.Fail(c, 409, 409, "审批证据快照校验失败")
		return
	}
	if digestBytes([]byte(approval.TemplateSnapshot)) != approval.TemplateSHA256 {
		httpx.Fail(c, 409, 409, "审批模板快照校验失败")
		return
	}
	fingerprint, err := h.approvalFingerprint(version, approval.TargetEnvironment)
	if err != nil || fingerprint != approval.InputFingerprint {
		_ = h.db.Transaction(func(tx *gorm.DB) error {
			if err := saveApprovalInvalidation(tx, &approval, &version, "CONTROLLED_INPUT_CHANGED"); err != nil {
				return err
			}
			return appendAudit(tx, c, project.ID, "MODEL_APPROVAL_INVALIDATED", "APPROVAL_REQUEST", approval.ID, nil, gin.H{"reason": approval.InvalidationReason})
		})
		httpx.Fail(c, 409, 409, "模型制品、配置或目标环境已变化，审批自动失效")
		return
	}
	if approval.TotalSteps > 1 {
		var prior int64
		h.db.Model(&ApprovalStepDecision{}).Where("approval_request_id = ? AND decided_by = ?", approval.ID, userID).Count(&prior)
		if prior > 0 {
			httpx.Fail(c, 409, 409, "多级审批要求不同步骤由不同人员完成")
			return
		}
	}
	now := time.Now()
	stepDecision := ApprovalStepDecision{
		TenantID: project.TenantID, ProjectID: project.ID, ApprovalRequestID: approval.ID,
		StepNo: approval.CurrentStep, StepName: current.Name, RequiredRole: current.RequiredRole,
		Decision: req.Decision, Comment: strings.TrimSpace(req.Comment),
		EvidenceSHA256: approval.EvidenceSHA256, DecidedBy: userID,
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&stepDecision).Error; err != nil {
			return err
		}
		if req.Decision == "REJECT" {
			approval.Status, approval.DecidedBy, approval.DecidedAt = "REJECTED", userID, &now
			if err := tx.Model(&version).Updates(map[string]any{"status": "EVALUATED", "approved_by": 0, "approved_at": nil, "preannotation_approved": false}).Error; err != nil {
				return err
			}
		} else if approval.CurrentStep < approval.TotalSteps {
			approval.CurrentStep++
			approval.Status = "IN_PROGRESS"
		} else {
			approval.Status, approval.DecidedBy, approval.DecidedAt = "APPROVED", userID, &now
			if err := tx.Model(&version).Updates(map[string]any{"status": "APPROVED", "approved_by": userID, "approved_at": now, "preannotation_approved": true}).Error; err != nil {
				return err
			}
			legacy := ApprovalDecision{
				TenantID: project.TenantID, ProjectID: project.ID, ApprovalRequestID: approval.ID,
				Decision: "APPROVE", Comment: strings.TrimSpace(req.Comment), EvidenceSHA256: approval.EvidenceSHA256, DecidedBy: userID,
			}
			if err := tx.Create(&legacy).Error; err != nil {
				return err
			}
		}
		if err := tx.Save(&approval).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "MODEL_APPROVAL_STEP_DECIDED", "APPROVAL_REQUEST", approval.ID, nil, gin.H{"approval": approval, "stepDecision": stepDecision})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "审批决定保存失败")
		return
	}
	httpx.OK(c, gin.H{"approval": approval, "stepDecision": stepDecision})
}

// ApprovalCancel godoc
// @Summary Cancel a pending approval request with an immutable reason
// @Tags VisionAI Model Governance
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param approvalId path int true "Approval request ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/approvals/{approvalId}/cancel [post]
func (h *Handler) ApprovalCancel(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok {
		return
	}
	approvalID, _ := strconv.ParseUint(c.Param("approvalId"), 10, 64)
	var approval ApprovalRequest
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status IN ?", project.TenantID, project.ID, approvalID, []string{"PENDING", "IN_PROGRESS"}).
		First(&approval).Error != nil {
		httpx.Fail(c, 409, 409, "审批不存在或已经结束")
		return
	}
	userID := c.GetUint64("user_id")
	if approval.SubmittedBy != userID && project.OwnerUserID != userID {
		httpx.Fail(c, 403, 403, "仅提交人或项目负责人可取消审批")
		return
	}
	var req struct {
		Comment string `json:"comment"`
	}
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Comment) == "" {
		httpx.Fail(c, 400, 400, "取消原因必填")
		return
	}
	now := time.Now()
	before := approval
	approval.Status, approval.DecidedBy, approval.DecidedAt = "CANCELLED", userID, &now
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&approval).Error; err != nil {
			return err
		}
		decision := ApprovalDecision{
			TenantID: project.TenantID, ProjectID: project.ID, ApprovalRequestID: approval.ID,
			Decision: "CANCEL", Comment: strings.TrimSpace(req.Comment),
			EvidenceSHA256: approval.EvidenceSHA256, DecidedBy: userID,
		}
		if err := tx.Create(&decision).Error; err != nil {
			return err
		}
		if err := tx.Model(&ModelVersion{}).
			Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, approval.ModelVersionID, "REVIEW_PENDING").
			Updates(map[string]any{"status": "EVALUATED", "preannotation_approved": false}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "MODEL_APPROVAL_CANCELLED", "APPROVAL_REQUEST", approval.ID, before, gin.H{"approval": approval, "reason": req.Comment})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "审批取消失败")
		return
	}
	httpx.OK(c, approval)
}

func (h *Handler) ApprovalPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var rows []ApprovalRequest
	query := h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", strings.ToUpper(status))
	}
	query.Order("id DESC").Find(&rows)
	httpx.OK(c, rows)
}

func (h *Handler) ModelExportManifest(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "OPS", "PROJECT_OWNER") {
		return
	}
	purpose := strings.TrimSpace(c.Query("purpose"))
	if purpose == "" {
		httpx.Fail(c, http.StatusBadRequest, 400, "导出用途必填；请使用 POST export 接口或提供 purpose 查询参数")
		return
	}
	h.modelExport(c, project, modelExportRequest{Purpose: purpose, IncludeArtifacts: true})
}

func (h *Handler) ModelVersionCompare(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	rawIDs := strings.Split(c.Query("ids"), ",")
	if len(rawIDs) < 2 || len(rawIDs) > 10 {
		httpx.Fail(c, 400, 400, "比较需要 2..10 个模型版本")
		return
	}
	var ids []uint64
	for _, raw := range rawIDs {
		id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			httpx.Fail(c, 400, 400, "模型版本 ID 无效")
			return
		}
		ids = append(ids, id)
	}
	var versions []ModelVersion
	h.db.Where("tenant_id = ? AND project_id = ? AND id IN ?", project.TenantID, project.ID, ids).Order("id").Find(&versions)
	result := make([]gin.H, 0, len(versions))
	for _, version := range versions {
		var evaluation EvaluationRun
		h.db.Where("id = ?", version.EvaluationRunID).First(&evaluation)
		var training TrainingRun
		h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, version.TrainingRunID).First(&training)
		var dataset DatasetVersion
		h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, training.DatasetVersionID).First(&dataset)
		var artifacts []ModelArtifact
		h.db.Where("tenant_id = ? AND project_id = ? AND model_version_id = ?", project.TenantID, project.ID, version.ID).Order("id").Find(&artifacts)
		var artifactBytes int64
		for _, artifact := range artifacts {
			artifactBytes += artifact.Size
		}
		var traces []InferenceTrace
		h.db.Where("tenant_id = ? AND project_id = ? AND model_version_id = ?", project.TenantID, project.ID, version.ID).
			Order("created_at DESC").Limit(10000).Find(&traces)
		result = append(result, gin.H{
			"version": version, "evaluationSummary": json.RawMessage(evaluation.Summary),
			"trainingParameters": json.RawMessage(training.Parameters), "runtimeSpec": json.RawMessage(training.RuntimeSpec),
			"datasetVersion": dataset, "artifactCount": len(artifacts), "artifactBytes": artifactBytes,
			"deploymentPerformance": calculateInferenceMetrics(traces),
		})
	}
	httpx.OK(c, result)
}

func (h *Handler) ModelVersionRetire(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER", "OPS") {
		return
	}
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 64)
	var version ModelVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, versionID).First(&version).Error != nil {
		httpx.Fail(c, 404, 404, "模型版本不存在")
		return
	}
	var active int64
	h.db.Model(&DeploymentRevision{}).Where("tenant_id = ? AND model_version_id = ? AND status = ?", project.TenantID, version.ID, "RUNNING").Count(&active)
	if active > 0 {
		httpx.Fail(c, 409, 409, "模型仍被运行中的部署修订使用，不能退役")
		return
	}
	before := version.Status
	version.Status, version.PreannotationApproved = "RETIRED", false
	h.db.Save(&version)
	_ = appendAudit(h.db, c, project.ID, "MODEL_VERSION_RETIRED", "MODEL_VERSION", version.ID, gin.H{"status": before}, gin.H{"status": version.Status})
	httpx.OK(c, version)
}
