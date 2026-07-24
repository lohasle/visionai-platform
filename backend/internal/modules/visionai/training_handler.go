package visionai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	platformtraining "github.com/lohasle/nimbus-framework-go/internal/platform/training"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type trainingTemplateRequest struct {
	Name        string `json:"name"`
	AIType      string `json:"aiType"`
	Description string `json:"description"`
}

type trainingTemplateVersionRequest struct {
	Trainer              string         `json:"trainer"`
	ImageRef             string         `json:"imageRef"`
	Entrypoint           string         `json:"entrypoint"`
	ParameterSchema      map[string]any `json:"parameterSchema"`
	OutputProtocol       string         `json:"outputProtocol"`
	ResourceRequirements map[string]any `json:"resourceRequirements"`
	Compatibility        map[string]any `json:"compatibility"`
	LicensePolicy        map[string]any `json:"licensePolicy"`
}

type trainingRunRequest struct {
	Name              string         `json:"name"`
	DatasetVersionID  uint64         `json:"datasetVersionId"`
	TemplateVersionID uint64         `json:"templateVersionId"`
	Provider          string         `json:"provider"`
	Queue             string         `json:"queue"`
	GPUCount          int            `json:"gpuCount"`
	Priority          int            `json:"priority"`
	Parameters        map[string]any `json:"parameters"`
	RuntimeSpec       map[string]any `json:"runtimeSpec"`
	CodeCommit        string         `json:"codeCommit"`
	PretrainedRef     string         `json:"pretrainedRef"`
}

// TrainingTemplatePage godoc
// @Summary List versioned training templates
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-templates [get]
func (h *Handler) TrainingTemplatePage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	type view struct {
		TrainingTemplate
		VersionCount   int64 `json:"versionCount"`
		PublishedCount int64 `json:"publishedCount"`
	}
	var rows []view
	h.db.Table("ai_training_template t").
		Select("t.*, COUNT(v.id) version_count, SUM(CASE WHEN v.published = 1 THEN 1 ELSE 0 END) published_count").
		Joins("LEFT JOIN ai_training_template_version v ON v.template_id = t.id AND v.tenant_id = t.tenant_id").
		Where("t.tenant_id = ? AND t.project_id = ?", project.TenantID, project.ID).
		Group("t.id").Order("t.id DESC").Scan(&rows)
	httpx.OK(c, rows)
}

// TrainingTemplateCreate godoc
// @Summary Create a training template
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-templates [post]
func (h *Handler) TrainingTemplateCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "PROJECT_OWNER") {
		return
	}
	var req trainingTemplateRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, 400, 400, "模板名称必填")
		return
	}
	aiType := strings.ToUpper(strings.TrimSpace(req.AIType))
	if aiType == "" {
		aiType = "CV_DETECTION"
	}
	row := TrainingTemplate{
		TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name),
		AIType: aiType, Description: strings.TrimSpace(req.Description), CreatedBy: c.GetUint64("user_id"),
	}
	if h.db.Create(&row).Error != nil {
		httpx.Fail(c, 409, 409, "同名训练模板已存在")
		return
	}
	httpx.OK(c, row)
}

// TrainingTemplateVersions godoc
// @Summary List immutable template versions
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-templates/{templateId}/versions [get]
func (h *Handler) TrainingTemplateVersions(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	templateID, _ := strconv.ParseUint(c.Param("templateId"), 10, 64)
	var rows []TrainingTemplateVersion
	h.db.Where("tenant_id = ? AND project_id = ? AND template_id = ?", project.TenantID, project.ID, templateID).Order("version_no DESC").Find(&rows)
	httpx.OK(c, rows)
}

// TrainingTemplateVersionCreate godoc
// @Summary Create an immutable training template version draft
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-templates/{templateId}/versions [post]
func (h *Handler) TrainingTemplateVersionCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "PROJECT_OWNER") {
		return
	}
	templateID, _ := strconv.ParseUint(c.Param("templateId"), 10, 64)
	var template TrainingTemplate
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, templateID).First(&template).Error != nil {
		httpx.Fail(c, 404, 404, "训练模板不存在")
		return
	}
	var req trainingTemplateVersionRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Trainer) == "" || !strings.HasPrefix(strings.TrimSpace(req.ImageRef), "sha256:") {
		httpx.Fail(c, 400, 400, "Trainer 必填，镜像必须使用不可变 sha256 引用")
		return
	}
	if req.OutputProtocol == "" {
		req.OutputProtocol = "visionai.result-manifest.v1"
	}
	if req.OutputProtocol != "visionai.result-manifest.v1" {
		httpx.Fail(c, 400, 400, "不支持的输出协议")
		return
	}
	var row TrainingTemplateVersion
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var latest TrainingTemplateVersion
		tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("template_id = ?", templateID).Order("version_no DESC").First(&latest)
		versionNo := latest.VersionNo + 1
		row = TrainingTemplateVersion{
			TenantID: project.TenantID, ProjectID: project.ID, TemplateID: templateID, VersionNo: versionNo,
			SemanticVersion: fmt.Sprintf("v%d", versionNo), Trainer: strings.TrimSpace(req.Trainer),
			ImageRef: strings.TrimSpace(req.ImageRef), Entrypoint: strings.TrimSpace(req.Entrypoint),
			ParameterSchema: jsonValue(req.ParameterSchema), OutputProtocol: req.OutputProtocol,
			ResourceRequirements: jsonValue(req.ResourceRequirements), Compatibility: jsonValue(req.Compatibility),
			LicensePolicy: jsonValue(req.LicensePolicy), SmokeStatus: "PENDING", SmokeReport: "{}",
			CreatedBy: c.GetUint64("user_id"),
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "训练模板版本创建失败")
		return
	}
	httpx.OK(c, row)
}

// TrainingTemplateSmoke godoc
// @Summary Run the mandatory LocalDocker template smoke test
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-template-versions/{templateVersionId}/smoke [post]
func (h *Handler) TrainingTemplateSmoke(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "PROJECT_OWNER") {
		return
	}
	versionID, _ := strconv.ParseUint(c.Param("templateVersionId"), 10, 64)
	var version TrainingTemplateVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND published = ?", project.TenantID, project.ID, versionID, false).First(&version).Error != nil {
		httpx.Fail(c, 409, 409, "模板版本不存在或已发布")
		return
	}
	cfg := config.Load()
	runID := uint64(time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	manifest, log, err := (platformtraining.LocalDocker{
		Binary:      cfg.DockerBinary,
		VolumesFrom: cfg.DockerVolumesFrom,
	}).Run(ctx, platformtraining.LocalDockerSpec{
		RunID: runID, ImageRef: version.ImageRef, Entrypoint: version.Entrypoint,
		OutputDir:          filepath.Join(cfg.TrainingWorkRoot, "template-smoke", strconv.FormatUint(version.ID, 10)),
		DatasetManifestURI: "s3://visionai-assets/smoke/dataset-manifest.json", ParametersJSON: "{}",
		MemoryBytes: 512 << 20, CPUs: 1,
	})
	if err != nil {
		version.SmokeStatus, version.SmokeReport = "FAILED", jsonValue(gin.H{"error": err.Error(), "log": string(log)})
		h.db.Save(&version)
		httpx.Fail(c, 422, 422, "模板冒烟测试失败")
		return
	}
	version.SmokeStatus, version.SmokeReport = "PASSED", jsonValue(gin.H{"summary": manifest.Summary, "log": string(log)})
	h.db.Save(&version)
	httpx.OK(c, version)
}

// TrainingTemplatePublish godoc
// @Summary Publish a smoke-tested immutable template version
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-template-versions/{templateVersionId}/publish [post]
func (h *Handler) TrainingTemplatePublish(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER", "ALGORITHM_ENGINEER") {
		return
	}
	versionID, _ := strconv.ParseUint(c.Param("templateVersionId"), 10, 64)
	var version TrainingTemplateVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, versionID).First(&version).Error != nil {
		httpx.Fail(c, 404, 404, "模板版本不存在")
		return
	}
	if version.Published {
		httpx.OK(c, version)
		return
	}
	if version.SmokeStatus != "PASSED" {
		httpx.Fail(c, 409, 409, "模板必须先通过最小训练冒烟")
		return
	}
	now := time.Now()
	version.Published, version.PublishedAt, version.PublishedBy = true, &now, c.GetUint64("user_id")
	h.db.Save(&version)
	httpx.OK(c, version)
}

func (h *Handler) validateTrainingDependencies(project Project, req trainingRunRequest) (DatasetVersion, TrainingTemplateVersion, string) {
	var dataset DatasetVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, req.DatasetVersionID, DatasetVersionFrozen).First(&dataset).Error != nil {
		return dataset, TrainingTemplateVersion{}, "训练只能引用 FROZEN DatasetVersion"
	}
	var templateVersion TrainingTemplateVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND published = ?", project.TenantID, project.ID, req.TemplateVersionID, true).First(&templateVersion).Error != nil {
		return dataset, templateVersion, "训练只能引用已发布模板版本"
	}
	var datasetRoot Dataset
	var templateRoot TrainingTemplate
	h.db.Where("id = ?", dataset.DatasetID).First(&datasetRoot)
	h.db.Where("id = ?", templateVersion.TemplateID).First(&templateRoot)
	if datasetRoot.TaskType != templateRoot.AIType {
		return dataset, templateVersion, "模板与数据集任务类型不兼容"
	}
	var license map[string]any
	_ = json.Unmarshal([]byte(templateVersion.LicensePolicy), &license)
	if allowed, exists := license["allowed"]; exists && allowed == false {
		return dataset, templateVersion, "模板许可证策略禁止执行"
	}
	if req.GPUCount < 0 || req.GPUCount > 8 {
		return dataset, templateVersion, "GPU 数量超出模板执行范围"
	}
	return dataset, templateVersion, ""
}

// TrainingRunPage godoc
// @Summary Page training runs and experiments
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-runs [get]
func (h *Handler) TrainingRunPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	query := h.db.Model(&TrainingRun{}).Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if provider := strings.TrimSpace(c.Query("provider")); provider != "" {
		query = query.Where("provider = ?", provider)
	}
	var total int64
	query.Count(&total)
	pageNo, pageSize := page(c)
	var rows []TrainingRun
	query.Order("priority DESC, id DESC").Offset((pageNo - 1) * pageSize).Limit(pageSize).Find(&rows)
	httpx.OK(c, gin.H{"list": rows, "total": total})
}

// TrainingRunCreate godoc
// @Summary Create and queue a reproducible training run
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-runs [post]
func (h *Handler) TrainingRunCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER") {
		return
	}
	var req trainingRunRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, 400, 400, "训练名称和配置必填")
		return
	}
	req.Provider = strings.ToUpper(strings.TrimSpace(req.Provider))
	if req.Provider == "" {
		req.Provider = "LOCAL_DOCKER"
	}
	if req.Provider != "LOCAL_DOCKER" && req.Provider != "CLEARML" {
		httpx.Fail(c, 400, 400, "Provider 必须是 LOCAL_DOCKER 或 CLEARML")
		return
	}
	dataset, _, reason := h.validateTrainingDependencies(project, req)
	if reason != "" {
		httpx.Fail(c, 409, 409, reason)
		return
	}
	if req.Queue == "" {
		req.Queue = "cpu-local"
	}
	run := TrainingRun{
		TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name),
		DatasetVersionID: req.DatasetVersionID, TemplateVersionID: req.TemplateVersionID,
		Provider: req.Provider, Queue: req.Queue, GPUCount: req.GPUCount, Priority: req.Priority,
		Parameters: jsonValue(req.Parameters), RuntimeSpec: jsonValue(req.RuntimeSpec),
		CodeCommit: strings.TrimSpace(req.CodeCommit), PretrainedRef: strings.TrimSpace(req.PretrainedRef),
		Status: TrainingQueued, MetricSummary: "{}", CreatedBy: c.GetUint64("user_id"),
	}
	idempotency := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotency == "" {
		idempotency = uuid.NewString()
	}
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "TRAINING", ResourceType: "TRAINING_RUN",
		Status: JobQueued, Stage: "QUEUED", TraceID: uuid.NewString(), Idempotency: idempotency, MaxRetries: 2, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		job.ResourceID = run.ID
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := tx.Create(&DatasetUsage{
			TenantID: project.TenantID, ProjectID: project.ID, DatasetVersionID: dataset.ID,
			ResourceType: "TRAINING_RUN", ResourceID: run.ID,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "training.run.requested.v1",
			AggregateType: "TRAINING_RUN", AggregateID: run.ID, Payload: jsonValue(gin.H{"trainingRunId": run.ID}), Status: outboxNew,
		}).Error
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			var existing PlatformJob
			if h.db.Where("idempotency = ?", idempotency).First(&existing).Error == nil {
				httpx.OK(c, gin.H{"job": existing, "idempotentReplay": true})
				return
			}
		}
		httpx.Fail(c, 409, 409, "训练任务创建失败")
		return
	}
	httpx.OK(c, gin.H{"run": run, "job": job})
}

// TrainingRunGet godoc
// @Summary Get training logs, metrics and artifacts
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-runs/{runId} [get]
func (h *Handler) TrainingRunGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	runID, _ := strconv.ParseUint(c.Param("runId"), 10, 64)
	var run TrainingRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, runID).First(&run).Error != nil {
		httpx.Fail(c, 404, 404, "训练任务不存在")
		return
	}
	var artifacts []TrainingArtifact
	var metrics []TrainingMetric
	h.db.Where("tenant_id = ? AND training_run_id = ?", project.TenantID, run.ID).Order("id").Find(&artifacts)
	h.db.Where("tenant_id = ? AND training_run_id = ?", project.TenantID, run.ID).Order("name, step").Find(&metrics)
	httpx.OK(c, gin.H{"run": run, "artifacts": artifacts, "metrics": metrics})
}

// TrainingRunCompare godoc
// @Summary Compare reproducible training experiments
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-runs/compare [get]
func (h *Handler) TrainingRunCompare(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	rawIDs := strings.Split(c.Query("ids"), ",")
	if len(rawIDs) < 2 || len(rawIDs) > 10 {
		httpx.Fail(c, 400, 400, "ids 必须包含 2 到 10 个训练运行")
		return
	}
	ids := make([]uint64, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
		if err != nil || id == 0 {
			httpx.Fail(c, 400, 400, "训练运行 ID 无效")
			return
		}
		ids = append(ids, id)
	}
	var runs []TrainingRun
	h.db.Where("tenant_id = ? AND project_id = ? AND id IN ?", project.TenantID, project.ID, ids).Find(&runs)
	if len(runs) != len(ids) {
		httpx.Fail(c, 404, 404, "包含不存在或跨项目训练运行")
		return
	}
	type comparison struct {
		TrainingRun
		Metrics   map[string]float64 `json:"metrics"`
		Artifacts int64              `json:"artifactCount"`
	}
	result := make([]comparison, 0, len(runs))
	for _, run := range runs {
		var metricRows []TrainingMetric
		h.db.Where("tenant_id = ? AND training_run_id = ?", project.TenantID, run.ID).Order("step").Find(&metricRows)
		metrics := make(map[string]float64)
		for _, metric := range metricRows {
			metrics[metric.Name] = metric.Value
		}
		var artifacts int64
		h.db.Model(&TrainingArtifact{}).Where("tenant_id = ? AND training_run_id = ?", project.TenantID, run.ID).Count(&artifacts)
		result = append(result, comparison{TrainingRun: run, Metrics: metrics, Artifacts: artifacts})
	}
	httpx.OK(c, result)
}

// TrainingRunCancel godoc
// @Summary Request training cancellation and preserve artifacts
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-runs/{runId}/cancel [post]
func (h *Handler) TrainingRunCancel(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER", "PROJECT_OWNER") {
		return
	}
	runID, _ := strconv.ParseUint(c.Param("runId"), 10, 64)
	var run TrainingRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, runID).First(&run).Error != nil {
		httpx.Fail(c, 404, 404, "训练任务不存在")
		return
	}
	if run.Status == TrainingSucceeded || run.Status == TrainingFailed || run.Status == TrainingCancelled {
		httpx.Fail(c, 409, 409, "终态训练不可取消")
		return
	}
	now := time.Now()
	run.CancelRequestedAt = &now
	if run.Status == TrainingQueued {
		run.Status, run.FinishedAt = TrainingCancelled, &now
	}
	h.db.Save(&run)
	httpx.OK(c, run)
}

// TrainingRunClone godoc
// @Summary Clone a reproducible training configuration
// @Tags VisionAI Training
// @Security BearerAuth
// @Router /ai-platform/projects/{id}/training-runs/{runId}/clone [post]
func (h *Handler) TrainingRunClone(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ALGORITHM_ENGINEER") {
		return
	}
	runID, _ := strconv.ParseUint(c.Param("runId"), 10, 64)
	var source TrainingRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, runID).First(&source).Error != nil {
		httpx.Fail(c, 404, 404, "源训练不存在")
		return
	}
	var overrides trainingRunRequest
	_ = c.ShouldBindJSON(&overrides)
	if overrides.Name == "" {
		overrides.Name = source.Name + " · clone"
	}
	if overrides.DatasetVersionID == 0 {
		overrides.DatasetVersionID = source.DatasetVersionID
	}
	if overrides.TemplateVersionID == 0 {
		overrides.TemplateVersionID = source.TemplateVersionID
	}
	if overrides.Provider == "" {
		overrides.Provider = source.Provider
	}
	if overrides.Queue == "" {
		overrides.Queue = source.Queue
	}
	if overrides.Parameters == nil {
		_ = json.Unmarshal([]byte(source.Parameters), &overrides.Parameters)
	}
	if overrides.RuntimeSpec == nil {
		_ = json.Unmarshal([]byte(source.RuntimeSpec), &overrides.RuntimeSpec)
	}
	dataset, _, reason := h.validateTrainingDependencies(project, overrides)
	if reason != "" {
		httpx.Fail(c, 409, 409, reason)
		return
	}
	clone := source
	clone.ID, clone.Name, clone.DatasetVersionID, clone.TemplateVersionID = 0, overrides.Name, overrides.DatasetVersionID, overrides.TemplateVersionID
	clone.Provider, clone.Queue, clone.Parameters, clone.RuntimeSpec = overrides.Provider, overrides.Queue, jsonValue(overrides.Parameters), jsonValue(overrides.RuntimeSpec)
	clone.ParentRunID, clone.Status, clone.Progress, clone.MetricSummary = source.ID, TrainingDraft, 0, "{}"
	clone.ExternalBindingID, clone.ResultManifestURI, clone.ErrorCategory, clone.ErrorCode, clone.ErrorMessage = 0, "", "", "", ""
	clone.CancelRequestedAt, clone.StartedAt, clone.FinishedAt = nil, nil, nil
	clone.CreatedAt, clone.UpdatedAt, clone.CreatedBy = time.Time{}, time.Time{}, c.GetUint64("user_id")
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&clone).Error; err != nil {
			return err
		}
		return tx.Create(&DatasetUsage{
			TenantID: project.TenantID, ProjectID: project.ID, DatasetVersionID: dataset.ID,
			ResourceType: "TRAINING_RUN", ResourceID: clone.ID,
		}).Error
	})
	if err != nil {
		httpx.Fail(c, http.StatusConflict, 409, "训练克隆失败")
		return
	}
	httpx.OK(c, clone)
}
