package visionai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math/bits"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	xdraw "golang.org/x/image/draw"
	"gorm.io/gorm"
)

type feedbackPolicyRequest struct {
	Enabled         bool           `json:"enabled"`
	RandomRate      float64        `json:"randomRate"`
	ConfidenceBelow float64        `json:"confidenceBelow"`
	CaptureEmpty    bool           `json:"captureEmpty"`
	CaptureErrors   bool           `json:"captureErrors"`
	CaptureManual   bool           `json:"captureManual"`
	CaptureDrift    bool           `json:"captureDrift"`
	DailyLimit      int            `json:"dailyLimit"`
	RetentionDays   int            `json:"retentionDays"`
	RedactionPolicy map[string]any `json:"redactionPolicy"`
	SensitiveReview bool           `json:"sensitiveReview"`
}

func (h *Handler) FeedbackPolicyGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var policy FeedbackPolicy
	if h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).First(&policy).Error != nil {
		httpx.OK(c, nil)
		return
	}
	httpx.OK(c, policy)
}

func (h *Handler) FeedbackPolicySave(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "DATA_MANAGER", "PROJECT_OWNER") {
		return
	}
	var req feedbackPolicyRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "反馈策略无效")
		return
	}
	if req.RandomRate < 0 || req.RandomRate > 1 || req.ConfidenceBelow < 0 || req.ConfidenceBelow > 1 {
		httpx.Fail(c, 400, 400, "采样比例和置信度必须在 0..1")
		return
	}
	if req.DailyLimit <= 0 {
		req.DailyLimit = 1000
	}
	if req.RetentionDays <= 0 {
		req.RetentionDays = 30
	}
	var row FeedbackPolicy
	err := h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).First(&row).Error
	before := row
	row.TenantID, row.ProjectID, row.Enabled = project.TenantID, project.ID, req.Enabled
	row.RandomRate, row.ConfidenceBelow, row.CaptureEmpty, row.CaptureErrors = req.RandomRate, req.ConfidenceBelow, req.CaptureEmpty, req.CaptureErrors
	row.CaptureManual, row.CaptureDrift = req.CaptureManual, req.CaptureDrift
	row.DailyLimit, row.RetentionDays, row.RedactionPolicy, row.SensitiveReview = req.DailyLimit, req.RetentionDays, jsonValue(req.RedactionPolicy), req.SensitiveReview
	if err == gorm.ErrRecordNotFound {
		row.CreatedBy = c.GetUint64("user_id")
		err = h.db.Create(&row).Error
	} else if err == nil {
		err = h.db.Save(&row).Error
	}
	if err != nil {
		httpx.Fail(c, 500, 500, "反馈策略保存失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "FEEDBACK_POLICY_SAVED", "FEEDBACK_POLICY", row.ID, before, row)
	httpx.OK(c, row)
}

func captureFeedback(h *Handler, trace InferenceTrace) {
	db := h.db
	var policy FeedbackPolicy
	if db.Where("tenant_id = ? AND project_id = ? AND enabled = ?", trace.TenantID, trace.ProjectID, true).First(&policy).Error != nil {
		return
	}
	reason := ""
	switch {
	case trace.Status == "FAILED" && policy.CaptureErrors:
		reason = "ERROR"
	case policy.CaptureDrift && traceDeploymentDrifted(db, trace):
		reason = "DRIFT"
	case trace.DetectionCount == 0 && policy.CaptureEmpty:
		reason = "EMPTY_RESULT"
	case trace.MeanConfidence < policy.ConfidenceBelow:
		reason = "LOW_CONFIDENCE"
	case policy.RandomRate > 0 && float64(trace.ID%10000)/10000 < policy.RandomRate:
		reason = "RANDOM"
	}
	if reason == "" {
		return
	}
	captureFeedbackWithReason(h, trace, policy, reason)
}

func traceDeploymentDrifted(db *gorm.DB, trace InferenceTrace) bool {
	var baseline DriftBaseline
	if db.Where("tenant_id = ? AND project_id = ? AND deployment_id = ?", trace.TenantID, trace.ProjectID, trace.DeploymentID).
		First(&baseline).Error != nil {
		return false
	}
	var traces []InferenceTrace
	db.Where("tenant_id = ? AND project_id = ? AND deployment_id = ?", trace.TenantID, trace.ProjectID, trace.DeploymentID).
		Order("created_at DESC").Limit(1000).Find(&traces)
	return fmt.Sprint(calculateDriftMetrics(baseline, traces)["status"]) == "DRIFTED"
}

func captureFeedbackWithReason(h *Handler, trace InferenceTrace, policy FeedbackPolicy, reason string) bool {
	db := h.db
	start := time.Now().Truncate(24 * time.Hour)
	var daily int64
	db.Model(&FeedbackSample{}).Where("policy_id = ? AND created_at >= ?", policy.ID, start).Count(&daily)
	if daily >= int64(policy.DailyLimit) {
		return false
	}
	var asset Asset
	if db.Where("tenant_id = ? AND id = ?", trace.TenantID, trace.AssetID).First(&asset).Error != nil {
		return false
	}
	perceptualHash := asset.SHA256
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if calculated, hashErr := h.assetPerceptualHash(ctx, asset); hashErr == nil {
		perceptualHash = calculated
	}
	cancel()
	redactionCtx, redactionCancel := context.WithTimeout(context.Background(), 30*time.Second)
	feedbackAsset, redactionErr := h.redactFeedbackAsset(redactionCtx, asset, policy)
	redactionCancel()
	if redactionErr != nil {
		return false
	}
	var existing []FeedbackSample
	db.Where("tenant_id = ? AND project_id = ? AND status IN ?", trace.TenantID, trace.ProjectID, []string{"PENDING", "BATCHED", "ACCEPTED"}).
		Select("perceptual_hash").Limit(5000).Find(&existing)
	duplicate := false
	for _, sample := range existing {
		if perceptualHashesSimilar(perceptualHash, sample.PerceptualHash) {
			duplicate = true
			break
		}
	}
	status := "PENDING"
	if duplicate {
		status = "DUPLICATE"
	}
	return db.Create(&FeedbackSample{
		TenantID: trace.TenantID, ProjectID: trace.ProjectID, PolicyID: policy.ID,
		InferenceTraceID: trace.ID, DeploymentRevisionID: trace.DeploymentRevisionID,
		ModelVersionID: trace.ModelVersionID, AssetID: feedbackAsset.ID, Reason: reason,
		PerceptualHash: perceptualHash, Status: status, ExpiresAt: time.Now().AddDate(0, 0, policy.RetentionDays),
	}).Error == nil
}

func (h *Handler) redactFeedbackAsset(ctx context.Context, source Asset, policy FeedbackPolicy) (Asset, error) {
	var redaction map[string]any
	_ = json.Unmarshal([]byte(policy.RedactionPolicy), &redaction)
	mode := strings.ToUpper(strings.TrimSpace(fmt.Sprint(redaction["mode"])))
	if mode == "" || mode == "<NIL>" || mode == "NONE" {
		return source, nil
	}
	if source.MediaKind != "IMAGE" {
		return Asset{}, errors.New("configured feedback redaction currently requires an image asset")
	}
	object, _, err := h.storage.Get(ctx, source.ObjectKey)
	if err != nil {
		return Asset{}, err
	}
	decoded, _, err := image.Decode(object)
	_ = object.Close()
	if err != nil {
		return Asset{}, err
	}
	bounds := decoded.Bounds()
	redacted := image.NewRGBA(bounds)
	draw.Draw(redacted, bounds, decoded, bounds.Min, draw.Src)
	switch mode {
	case "MASK_CENTER":
		widthRatio, heightRatio := 0.45, 0.3
		if value, ok := redaction["centerWidthRatio"].(float64); ok && value > 0 && value <= 1 {
			widthRatio = value
		}
		if value, ok := redaction["centerHeightRatio"].(float64); ok && value > 0 && value <= 1 {
			heightRatio = value
		}
		width, height := bounds.Dx(), bounds.Dy()
		maskWidth, maskHeight := int(float64(width)*widthRatio), int(float64(height)*heightRatio)
		mask := image.Rect(
			bounds.Min.X+(width-maskWidth)/2, bounds.Min.Y+(height-maskHeight)/2,
			bounds.Min.X+(width+maskWidth)/2, bounds.Min.Y+(height+maskHeight)/2,
		)
		draw.Draw(redacted, mask, &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	case "PIXELATE":
		downWidth, downHeight := max(8, bounds.Dx()/32), max(8, bounds.Dy()/32)
		downsampled := image.NewRGBA(image.Rect(0, 0, downWidth, downHeight))
		xdraw.NearestNeighbor.Scale(downsampled, downsampled.Bounds(), decoded, bounds, xdraw.Src, nil)
		xdraw.NearestNeighbor.Scale(redacted, bounds, downsampled, downsampled.Bounds(), xdraw.Src, nil)
	default:
		return Asset{}, fmt.Errorf("unsupported feedback redaction mode %s", mode)
	}
	var output bytes.Buffer
	if err = jpeg.Encode(&output, redacted, &jpeg.Options{Quality: 88}); err != nil {
		return Asset{}, err
	}
	sum := sha256.Sum256(output.Bytes())
	key := fmt.Sprintf("%s/feedback/redacted/%s.jpg", assetRoot(source.TenantID, source.ProjectID), uuid.NewString())
	if err = h.storage.Put(ctx, key, bytes.NewReader(output.Bytes()), int64(output.Len()), "image/jpeg"); err != nil {
		return Asset{}, err
	}
	metadata := jsonValue(map[string]any{
		"redactedFromAssetId": source.ID, "redactionMode": mode,
		"sourceTracePolicyId": policy.ID,
	})
	result := Asset{
		TenantID: source.TenantID, ProjectID: source.ProjectID,
		Filename: fmt.Sprintf("redacted-%d.jpg", source.ID), ObjectKey: key, URI: h.storage.URI(key),
		SHA256: hex.EncodeToString(sum[:]), ContentType: "image/jpeg", Size: int64(output.Len()),
		Width: bounds.Dx(), Height: bounds.Dy(), MediaKind: "IMAGE", Status: AssetReady,
		SourceDevice: source.SourceDevice, BusinessScene: source.BusinessScene,
		Metadata: metadata, CreatedBy: policy.CreatedBy,
	}
	if err = h.db.Create(&result).Error; err != nil {
		_ = h.storage.Delete(ctx, key)
		return Asset{}, err
	}
	return result, nil
}

// FeedbackManualCapture godoc
// @Summary Add a production trace to the feedback loop manually
// @Tags VisionAI Feedback
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Param traceId path string true "Inference trace ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/inference-traces/{traceId}/feedback [post]
func (h *Handler) FeedbackManualCapture(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER", "OPS", "PROJECT_OWNER") {
		return
	}
	var req struct {
		Comment string `json:"comment"`
	}
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Comment) == "" {
		httpx.Fail(c, 400, 400, "人工反馈说明必填")
		return
	}
	var policy FeedbackPolicy
	if h.db.Where("tenant_id = ? AND project_id = ? AND enabled = ? AND capture_manual = ?",
		project.TenantID, project.ID, true, true).First(&policy).Error != nil {
		httpx.Fail(c, 409, 409, "当前反馈策略未启用人工反馈采样")
		return
	}
	var trace InferenceTrace
	if h.db.Where("tenant_id = ? AND project_id = ? AND trace_id = ?", project.TenantID, project.ID, c.Param("traceId")).
		First(&trace).Error != nil {
		httpx.Fail(c, 404, 404, "推理 Trace 不存在")
		return
	}
	if !captureFeedbackWithReason(h, trace, policy, "MANUAL") {
		httpx.Fail(c, 409, 409, "Trace 已进入反馈池、达到每日上限或缺少可回流资产")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "FEEDBACK_MANUAL_CAPTURED", "INFERENCE_TRACE", trace.ID, nil,
		gin.H{"traceId": trace.TraceID, "comment": req.Comment})
	httpx.OK(c, true)
}

func (h *Handler) assetPerceptualHash(ctx context.Context, asset Asset) (string, error) {
	object, _, err := h.storage.Get(ctx, asset.ObjectKey)
	if err != nil {
		return "", err
	}
	defer object.Close()
	source, _, err := image.Decode(object)
	if err != nil {
		return "", err
	}
	target := image.NewGray(image.Rect(0, 0, 8, 8))
	xdraw.CatmullRom.Scale(target, target.Bounds(), source, source.Bounds(), xdraw.Over, nil)
	var sum uint64
	values := make([]uint8, 64)
	for index := range values {
		x, y := index%8, index/8
		value := color.GrayModel.Convert(target.At(x, y)).(color.Gray).Y
		values[index] = value
		sum += uint64(value)
	}
	average := uint8(sum / 64)
	var hash uint64
	for index, value := range values {
		if value >= average {
			hash |= uint64(1) << index
		}
	}
	return fmt.Sprintf("%016x", hash), nil
}

func perceptualHashesSimilar(left, right string) bool {
	if left == right {
		return true
	}
	if len(left) != 16 || len(right) != 16 {
		return false
	}
	leftValue, leftErr := strconv.ParseUint(left, 16, 64)
	rightValue, rightErr := strconv.ParseUint(right, 16, 64)
	return leftErr == nil && rightErr == nil && bits.OnesCount64(leftValue^rightValue) <= 6
}

func (h *Handler) FeedbackSamplePage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var rows []FeedbackSample
	query := h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", strings.ToUpper(status))
	}
	query.Order("id DESC").Limit(500).Find(&rows)
	httpx.OK(c, rows)
}

type feedbackBatchRequest struct {
	Name      string   `json:"name"`
	SampleIDs []uint64 `json:"sampleIds"`
}

func (h *Handler) FeedbackBatchCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER", "PROJECT_OWNER") {
		return
	}
	var req feedbackBatchRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, 400, 400, "批次名称必填")
		return
	}
	query := h.db.Where("tenant_id = ? AND project_id = ? AND status = ? AND expires_at > ?", project.TenantID, project.ID, "PENDING", time.Now())
	if len(req.SampleIDs) > 0 {
		query = query.Where("id IN ?", req.SampleIDs)
	}
	var samples []FeedbackSample
	query.Order("id").Limit(500).Find(&samples)
	if len(samples) == 0 {
		httpx.Fail(c, 409, 409, "没有可入批的待审样本")
		return
	}
	status := "READY"
	var policy FeedbackPolicy
	if h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).First(&policy).Error == nil && policy.SensitiveReview {
		status = "PRIVACY_REVIEW"
	}
	row := FeedbackBatch{TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name), Status: status, SampleCount: len(samples), CreatedBy: c.GetUint64("user_id")}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		ids := make([]uint64, 0, len(samples))
		for _, sample := range samples {
			ids = append(ids, sample.ID)
		}
		if err := tx.Model(&FeedbackSample{}).Where("id IN ? AND status = ?", ids, "PENDING").Updates(map[string]any{"status": "BATCHED", "batch_id": row.ID}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "FEEDBACK_BATCH_CREATED", "FEEDBACK_BATCH", row.ID, nil, gin.H{"sampleCount": len(samples)})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "反馈批次创建失败")
		return
	}
	httpx.OK(c, row)
}

func (h *Handler) FeedbackBatchPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var rows []FeedbackBatch
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Order("id DESC").Find(&rows)
	httpx.OK(c, rows)
}

func (h *Handler) FeedbackBatchGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	batchID, _ := strconv.ParseUint(c.Param("batchId"), 10, 64)
	var batch FeedbackBatch
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, batchID).First(&batch).Error != nil {
		httpx.Fail(c, 404, 404, "反馈批次不存在")
		return
	}
	var samples []FeedbackSample
	h.db.Where("batch_id = ?", batch.ID).Order("id").Find(&samples)
	var traceIDs []uint64
	for _, sample := range samples {
		traceIDs = append(traceIDs, sample.InferenceTraceID)
	}
	var traces []InferenceTrace
	if len(traceIDs) > 0 {
		h.db.Where("id IN ?", traceIDs).Order("id").Find(&traces)
	}
	var task AnnotationTask
	var annotationRevision AnnotationRevision
	var datasetVersions []DatasetVersion
	var modelVersions []ModelVersion
	var deploymentRevisions []DeploymentRevision
	if batch.AnnotationTaskID != 0 {
		h.db.Where("id = ?", batch.AnnotationTaskID).First(&task)
		h.db.Where("annotation_task_id = ?", task.ID).Order("revision_no DESC").First(&annotationRevision)
	}
	if annotationRevision.ID != 0 {
		h.db.Where("tenant_id = ? AND annotation_revision_id = ?", project.TenantID, annotationRevision.ID).Order("id").Find(&datasetVersions)
		if len(datasetVersions) > 0 {
			batch.AnnotationRevisionID = annotationRevision.ID
			batch.DatasetVersionID = datasetVersions[len(datasetVersions)-1].ID
			var datasetIDs []uint64
			for _, version := range datasetVersions {
				datasetIDs = append(datasetIDs, version.ID)
			}
			h.db.Where("tenant_id = ? AND dataset_version_id IN ?", project.TenantID, datasetIDs).Order("id").Find(&modelVersions)
		}
	}
	if len(modelVersions) > 0 {
		batch.ModelVersionID = modelVersions[len(modelVersions)-1].ID
		var modelIDs []uint64
		for _, version := range modelVersions {
			modelIDs = append(modelIDs, version.ID)
		}
		h.db.Where("tenant_id = ? AND model_version_id IN ?", project.TenantID, modelIDs).Order("id").Find(&deploymentRevisions)
	}
	if batch.AnnotationRevisionID != 0 || batch.DatasetVersionID != 0 || batch.ModelVersionID != 0 {
		h.db.Save(&batch)
	}
	httpx.OK(c, gin.H{
		"batch": batch, "samples": samples, "inferenceTraces": traces, "annotationTask": task,
		"annotationRevision": annotationRevision, "datasetVersions": datasetVersions,
		"modelVersions": modelVersions, "deploymentRevisions": deploymentRevisions,
	})
}

type feedbackReviewRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

func (h *Handler) FeedbackBatchReview(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "REVIEWER", "DATA_MANAGER", "PROJECT_OWNER") {
		return
	}
	batchID, _ := strconv.ParseUint(c.Param("batchId"), 10, 64)
	var batch FeedbackBatch
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status IN ?", project.TenantID, project.ID, batchID, []string{"READY", "PRIVACY_REVIEW"}).First(&batch).Error != nil {
		httpx.Fail(c, 409, 409, "批次不存在或已审核")
		return
	}
	var req feedbackReviewRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Comment) == "" {
		httpx.Fail(c, 400, 400, "审核意见必填")
		return
	}
	req.Decision = strings.ToUpper(req.Decision)
	if req.Decision != "ACCEPT" && req.Decision != "REJECT" && req.Decision != "PRIVACY_APPROVE" {
		httpx.Fail(c, 400, 400, "决定必须是 ACCEPT 或 REJECT")
		return
	}
	now := time.Now()
	if batch.Status == "PRIVACY_REVIEW" {
		if req.Decision != "PRIVACY_APPROVE" && req.Decision != "REJECT" {
			httpx.Fail(c, 409, 409, "sensitive batches require privacy approval before business review")
			return
		}
		if req.Decision == "PRIVACY_APPROVE" {
			if !h.requireProjectRole(c, project, "DATA_MANAGER", "PROJECT_OWNER") {
				return
			}
			if batch.CreatedBy == c.GetUint64("user_id") {
				httpx.Fail(c, 409, 409, "privacy reviewer must differ from batch creator")
				return
			}
			before := batch
			batch.Status, batch.PrivacyReviewedBy, batch.PrivacyReviewedAt = "READY", c.GetUint64("user_id"), &now
			h.db.Save(&batch)
			_ = appendAudit(h.db, c, project.ID, "FEEDBACK_PRIVACY_APPROVED", "FEEDBACK_BATCH", batch.ID, before, batch)
			httpx.OK(c, batch)
			return
		}
	}
	if req.Decision == "REJECT" {
		h.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&batch).Updates(map[string]any{"status": "REJECTED", "reviewed_by": c.GetUint64("user_id"), "reviewed_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&FeedbackSample{}).Where("batch_id = ?", batch.ID).Updates(map[string]any{"status": "REJECTED", "reviewed_by": c.GetUint64("user_id"), "reviewed_at": now}).Error; err != nil {
				return err
			}
			return appendAudit(tx, c, project.ID, "FEEDBACK_BATCH_REJECTED", "FEEDBACK_BATCH", batch.ID, nil, req)
		})
		httpx.OK(c, true)
		return
	}
	var samples []FeedbackSample
	h.db.Where("batch_id = ? AND status = ?", batch.ID, "BATCHED").Order("id").Find(&samples)
	if len(samples) == 0 {
		httpx.Fail(c, 409, 409, "批次没有有效样本")
		return
	}
	collection := AssetCollection{
		TenantID: project.TenantID, ProjectID: project.ID, Name: "反馈回流-" + batch.Name,
		Description: "来源 FeedbackBatch #" + strconv.FormatUint(batch.ID, 10), Filter: jsonValue(gin.H{"feedbackBatchId": batch.ID}),
		Frozen: true, Version: 1, CreatedBy: c.GetUint64("user_id"),
	}
	var deploymentRevision DeploymentRevision
	var modelVersion ModelVersion
	var datasetVersion DatasetVersion
	var ontologyVersion OntologyVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, samples[0].DeploymentRevisionID).First(&deploymentRevision).Error != nil ||
		h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, deploymentRevision.ModelVersionID).First(&modelVersion).Error != nil ||
		h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, modelVersion.DatasetVersionID).First(&datasetVersion).Error != nil ||
		h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, datasetVersion.OntologyVersionID).First(&ontologyVersion).Error != nil {
		httpx.Fail(c, 409, 409, "无法从部署血缘解析类别体系版本")
		return
	}
	ontologyLabels, ontologyErr := annotationLabelsForOntologyVersion(h.db, ontologyVersion)
	if ontologyErr != nil {
		httpx.Fail(c, 409, 409, ontologyErr.Error())
		return
	}
	task := AnnotationTask{
		TenantID: project.TenantID, ProjectID: project.ID, Name: "反馈返标-" + batch.Name,
		TaskType: "CV_DETECTION", OntologyVersionID: ontologyVersion.ID,
		OntologyVersion: ontologyVersion.SemanticVersion, OntologyChecksum: ontologyVersion.Checksum,
		Labels:       jsonValue(ontologyLabels),
		AnnotatorIDs: jsonValue([]uint64{project.OwnerUserID}),
		ReviewerIDs:  jsonValue([]uint64{c.GetUint64("user_id")}),
		Status:       AnnotationPreparing, CreatedBy: c.GetUint64("user_id"),
	}
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "ANNOTATION_PREPARE",
		ResourceType: "ANNOTATION_TASK", Status: JobQueued, Stage: "PREPARING",
		TraceID: uuid.NewString(), Idempotency: "feedback-annotation:" + strconv.FormatUint(batch.ID, 10), MaxRetries: 3, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&collection).Error; err != nil {
			return err
		}
		seen := map[uint64]bool{}
		for _, sample := range samples {
			if seen[sample.AssetID] {
				continue
			}
			seen[sample.AssetID] = true
			if err := tx.Create(&AssetCollectionItem{TenantID: project.TenantID, ProjectID: project.ID, CollectionID: collection.ID, AssetID: sample.AssetID}).Error; err != nil {
				return err
			}
			if err := tx.Model(&Asset{}).Where("id = ?", sample.AssetID).UpdateColumn("reference_count", gorm.Expr("reference_count + 1")).Error; err != nil {
				return err
			}
		}
		task.CollectionID = collection.ID
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		job.ResourceID = task.ID
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "annotation.task.prepare.v1",
			AggregateType: "ANNOTATION_TASK", AggregateID: task.ID, Payload: jsonValue(gin.H{"annotationTaskId": task.ID}), Status: outboxNew,
		}).Error; err != nil {
			return err
		}
		batch.Status, batch.AnnotationTaskID, batch.ReviewedBy, batch.ReviewedAt = "ANNOTATING", task.ID, c.GetUint64("user_id"), &now
		if err := tx.Save(&batch).Error; err != nil {
			return err
		}
		if err := tx.Model(&FeedbackSample{}).Where("batch_id = ?", batch.ID).Updates(map[string]any{"status": "ACCEPTED", "reviewed_by": c.GetUint64("user_id"), "reviewed_at": now}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "FEEDBACK_BATCH_ACCEPTED", "FEEDBACK_BATCH", batch.ID, nil, gin.H{"annotationTaskId": task.ID, "sourceRevisionIdsPreserved": true})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "反馈批次接受失败")
		return
	}
	httpx.OK(c, gin.H{"batch": batch, "annotationTask": task, "job": job})
}

func (h *Handler) FeedbackCleanup(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "OPS", "PROJECT_OWNER") {
		return
	}
	result := h.db.Model(&FeedbackSample{}).Where("tenant_id = ? AND project_id = ? AND expires_at < ? AND status IN ?", project.TenantID, project.ID, time.Now(), []string{"PENDING", "DUPLICATE"}).Update("status", "EXPIRED")
	_ = appendAudit(h.db, c, project.ID, "FEEDBACK_RETENTION_APPLIED", "PROJECT", project.ID, nil, gin.H{"expired": result.RowsAffected})
	httpx.OK(c, gin.H{"expired": result.RowsAffected})
}

type feedbackBenefitRequest struct {
	FeedbackBatchID          uint64   `json:"feedbackBatchId"`
	BaselineEvaluationRunID  uint64   `json:"baselineEvaluationRunId"`
	CandidateEvaluationRunID uint64   `json:"candidateEvaluationRunId"`
	BaselineDeploymentID     uint64   `json:"baselineDeploymentId"`
	CandidateDeploymentID    uint64   `json:"candidateDeploymentId"`
	Slices                   []string `json:"slices"`
}

// FeedbackBenefitPage godoc
// @Summary List immutable feedback-loop benefit evaluations
// @Tags VisionAI Feedback
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/feedback-benefits [get]
func (h *Handler) FeedbackBenefitPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	var rows []FeedbackBenefitEvaluation
	h.db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Order("id DESC").Find(&rows)
	httpx.OK(c, rows)
}

// FeedbackBenefitCreate godoc
// @Summary Compare closed-loop baseline and candidate quality and production metrics
// @Tags VisionAI Feedback
// @Security BearerAuth
// @Param id path int true "Project ID"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/feedback-benefits [post]
func (h *Handler) FeedbackBenefitCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "ML_ENGINEER", "REVIEWER", "PROJECT_OWNER") {
		return
	}
	var req feedbackBenefitRequest
	if c.ShouldBindJSON(&req) != nil || req.FeedbackBatchID == 0 || req.BaselineEvaluationRunID == 0 ||
		req.CandidateEvaluationRunID == 0 || req.BaselineDeploymentID == 0 || req.CandidateDeploymentID == 0 {
		httpx.Fail(c, 400, 400, "batch, evaluation runs and deployments are required")
		return
	}
	if len(req.Slices) == 0 {
		req.Slices = []string{"all"}
	}
	req.Slices = normalizedSlices(req.Slices)
	var batch FeedbackBatch
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, req.FeedbackBatchID).First(&batch).Error != nil {
		httpx.Fail(c, 404, 404, "feedback batch not found")
		return
	}
	var baselineEvaluation, candidateEvaluation EvaluationRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, req.BaselineEvaluationRunID, "SUCCEEDED").First(&baselineEvaluation).Error != nil ||
		h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, req.CandidateEvaluationRunID, "SUCCEEDED").First(&candidateEvaluation).Error != nil {
		httpx.Fail(c, 409, 409, "both evaluation runs must have succeeded")
		return
	}
	var baselineModel, candidateModel ModelVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND evaluation_run_id = ?", project.TenantID, project.ID, baselineEvaluation.ID).First(&baselineModel).Error != nil ||
		h.db.Where("tenant_id = ? AND project_id = ? AND evaluation_run_id = ?", project.TenantID, project.ID, candidateEvaluation.ID).First(&candidateModel).Error != nil {
		httpx.Fail(c, 409, 409, "evaluation runs must be registered as model versions")
		return
	}
	if baselineModel.ID == candidateModel.ID {
		httpx.Fail(c, 409, 409, "candidate model must differ from baseline model")
		return
	}
	var sourceCount int64
	h.db.Model(&FeedbackSample{}).Where("batch_id = ? AND model_version_id = ?", batch.ID, baselineModel.ID).Count(&sourceCount)
	if sourceCount == 0 {
		httpx.Fail(c, 409, 409, "baseline model is not the source of this feedback batch")
		return
	}
	if batch.DatasetVersionID != 0 && candidateModel.DatasetVersionID != batch.DatasetVersionID {
		httpx.Fail(c, 409, 409, "candidate model is not trained from the feedback lineage dataset")
		return
	}
	baselineQuality, qualityErr := h.feedbackQualityEvidence(baselineEvaluation.ID, req.Slices)
	if qualityErr != nil {
		httpx.Fail(c, 409, 409, qualityErr.Error())
		return
	}
	candidateQuality, qualityErr := h.feedbackQualityEvidence(candidateEvaluation.ID, req.Slices)
	if qualityErr != nil {
		httpx.Fail(c, 409, 409, qualityErr.Error())
		return
	}
	baselineProduction, prodErr := h.feedbackProductionEvidence(project, req.BaselineDeploymentID, baselineModel.ID)
	if prodErr != nil {
		httpx.Fail(c, 409, 409, prodErr.Error())
		return
	}
	candidateProduction, prodErr := h.feedbackProductionEvidence(project, req.CandidateDeploymentID, candidateModel.ID)
	if prodErr != nil {
		httpx.Fail(c, 409, 409, prodErr.Error())
		return
	}
	deltas := feedbackBenefitDeltas(baselineQuality, candidateQuality, baselineProduction, candidateProduction)
	conclusion := "NO_BENEFIT"
	if deltas["mAP"] >= 0 && deltas["errorRate"] <= 0 && deltas["p95"] <= baselineProduction["p95"]*0.2 {
		conclusion = "IMPROVED"
	}
	evidence := gin.H{
		"feedbackBatchId": batch.ID, "slices": req.Slices,
		"baseline":  gin.H{"modelVersionId": baselineModel.ID, "evaluationRunId": baselineEvaluation.ID, "deploymentId": req.BaselineDeploymentID, "quality": baselineQuality, "production": baselineProduction},
		"candidate": gin.H{"modelVersionId": candidateModel.ID, "evaluationRunId": candidateEvaluation.ID, "deploymentId": req.CandidateDeploymentID, "quality": candidateQuality, "production": candidateProduction},
		"deltas":    deltas, "conclusion": conclusion,
	}
	evidenceJSON := jsonValue(evidence)
	row := FeedbackBenefitEvaluation{
		TenantID: project.TenantID, ProjectID: project.ID, FeedbackBatchID: batch.ID,
		BaselineModelVersionID: baselineModel.ID, CandidateModelVersionID: candidateModel.ID,
		BaselineEvaluationRunID: baselineEvaluation.ID, CandidateEvaluationRunID: candidateEvaluation.ID,
		BaselineDeploymentID: req.BaselineDeploymentID, CandidateDeploymentID: req.CandidateDeploymentID,
		Slices: jsonValue(req.Slices), BaselineQuality: jsonValue(baselineQuality),
		CandidateQuality: jsonValue(candidateQuality), BaselineProduction: jsonValue(baselineProduction),
		CandidateProduction: jsonValue(candidateProduction), Deltas: jsonValue(deltas),
		Conclusion: conclusion, EvidenceSnapshot: evidenceJSON,
		EvidenceSHA256: digestBytes([]byte(evidenceJSON)), CreatedBy: c.GetUint64("user_id"),
	}
	var existing FeedbackBenefitEvaluation
	h.db.Where("tenant_id = ? AND feedback_batch_id = ?", project.TenantID, batch.ID).First(&existing)
	if existing.ID == 0 {
		h.db.Create(&row)
	} else {
		row.ID, row.CreatedAt = existing.ID, existing.CreatedAt
		h.db.Save(&row)
	}
	batch.Status, batch.ModelVersionID = "COMPLETED", candidateModel.ID
	h.db.Save(&batch)
	_ = appendAudit(h.db, c, project.ID, "FEEDBACK_BENEFIT_EVALUATED", "FEEDBACK_BENEFIT", row.ID, existing, evidence)
	httpx.OK(c, row)
}

func normalizedSlices(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func (h *Handler) feedbackQualityEvidence(runID uint64, slices []string) (map[string]float64, error) {
	var metrics []EvaluationMetric
	h.db.Where("evaluation_run_id = ? AND slice IN ?", runID, slices).Find(&metrics)
	result := map[string]float64{}
	for _, metric := range metrics {
		key := metric.Slice + "." + metric.Name
		result[key] = metric.Value
		if strings.EqualFold(metric.Slice, "all") {
			result[metric.Name] = metric.Value
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("selected slices have no evaluation metrics for run %d", runID)
	}
	for _, slice := range slices {
		found := false
		for key := range result {
			if strings.HasPrefix(key, slice+".") {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("slice %s is missing from evaluation run %d", slice, runID)
		}
	}
	return result, nil
}

func (h *Handler) feedbackProductionEvidence(project Project, deploymentID, modelVersionID uint64) (map[string]float64, error) {
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, deploymentID).First(&deployment).Error != nil {
		return nil, fmt.Errorf("deployment %d not found", deploymentID)
	}
	var traces []InferenceTrace
	h.db.Where("tenant_id = ? AND project_id = ? AND deployment_id = ? AND model_version_id = ?", project.TenantID, project.ID, deployment.ID, modelVersionID).
		Order("id DESC").Limit(1000).Find(&traces)
	if len(traces) == 0 {
		return nil, fmt.Errorf("deployment %d has no production traces for model %d", deploymentID, modelVersionID)
	}
	raw := calculateInferenceMetrics(traces)
	result := map[string]float64{}
	for key, value := range raw {
		switch typed := value.(type) {
		case float64:
			result[key] = typed
		case int:
			result[key] = float64(typed)
		}
	}
	return result, nil
}

func feedbackBenefitDeltas(baselineQuality, candidateQuality, baselineProduction, candidateProduction map[string]float64) map[string]float64 {
	result := map[string]float64{}
	for _, metric := range []string{"mAP", "precision", "recall"} {
		result[metric] = candidateQuality[metric] - baselineQuality[metric]
	}
	for _, metric := range []string{"qps", "errorRate", "p50", "p95", "p99", "emptyRate", "meanConfidence"} {
		result[metric] = candidateProduction[metric] - baselineProduction[metric]
	}
	return result
}
