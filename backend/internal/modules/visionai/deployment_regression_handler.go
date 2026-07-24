package visionai

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	platforminference "github.com/lohasle/nimbus-framework-go/internal/platform/inference"
)

const maxRegressionImageSize = 20 << 20

type regressionResult struct {
	Status                string  `json:"status"`
	ExpectedLabel         string  `json:"expectedLabel"`
	MinimumConfidence     float64 `json:"minimumConfidence"`
	MatchedDetectionCount int     `json:"matchedDetectionCount"`
}

func validateRegressionImage(input []byte) (format string, width, height int, sha string, err error) {
	if len(input) == 0 || len(input) > maxRegressionImageSize {
		return "", 0, 0, "", fmt.Errorf("image size is invalid")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(input))
	if err != nil {
		return "", 0, 0, "", fmt.Errorf("decode image header: %w", err)
	}
	format = strings.ToLower(format)
	if format != "jpeg" && format != "png" && format != "webp" {
		return "", 0, 0, "", fmt.Errorf("unsupported image format")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return "", 0, 0, "", fmt.Errorf("unsafe image dimensions")
	}
	if _, _, err = image.Decode(bytes.NewReader(input)); err != nil {
		return "", 0, 0, "", fmt.Errorf("decode full image: %w", err)
	}
	sum := sha256.Sum256(input)
	return format, cfg.Width, cfg.Height, hex.EncodeToString(sum[:]), nil
}

func evaluateRegression(detections []struct {
	Label      string    `json:"label"`
	Confidence float64   `json:"confidence"`
	BBox       []float64 `json:"bbox"`
}, expectedLabel string, minimumConfidence float64) regressionResult {
	expectedLabel = strings.TrimSpace(expectedLabel)
	result := regressionResult{
		Status: "NOT_ASSERTED", ExpectedLabel: expectedLabel,
		MinimumConfidence: minimumConfidence,
	}
	if expectedLabel == "" {
		return result
	}
	result.Status = "FAILED"
	for _, detection := range detections {
		if strings.EqualFold(strings.TrimSpace(detection.Label), expectedLabel) && detection.Confidence >= minimumConfidence {
			result.MatchedDetectionCount++
		}
	}
	if result.MatchedDetectionCount > 0 {
		result.Status = "PASSED"
	}
	return result
}

func buildInferenceStatus(deployment Deployment, revisions []DeploymentRevision, traces []InferenceTrace) gin.H {
	var current *DeploymentRevision
	for i := range revisions {
		if revisions[i].ID == deployment.CurrentRevisionID {
			current = &revisions[i]
			break
		}
	}
	var lastTrace *InferenceTrace
	var lastError *InferenceTrace
	for i := range traces {
		if lastTrace == nil {
			lastTrace = &traces[i]
		}
		if lastError == nil && traces[i].Status == "FAILED" {
			lastError = &traces[i]
		}
	}
	ready := deployment.Status == "RUNNING" && current != nil && current.Status == "RUNNING"
	status := gin.H{
		"ready": ready, "deploymentStatus": deployment.Status,
		"currentRevisionId": deployment.CurrentRevisionID,
		"endpointUrl":       deployment.EndpointURL, "checkedAt": time.Now(),
	}
	if current != nil {
		status["revisionNo"] = current.RevisionNo
		status["revisionStatus"] = current.Status
		status["modelVersionId"] = current.ModelVersionID
	}
	if lastTrace != nil {
		status["lastTraceId"] = lastTrace.TraceID
		status["lastRequestAt"] = lastTrace.CreatedAt
		status["lastRequestStatus"] = lastTrace.Status
		status["lastLatencyMs"] = lastTrace.LatencyMS
	}
	if lastError != nil {
		status["lastErrorAt"] = lastError.CreatedAt
		status["lastErrorMessage"] = lastError.ErrorMessage
	}
	return status
}

// DeploymentPredictImage godoc
// @Summary Upload an image and run an online regression test
// @Tags VisionAI Deployment
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "JPEG, PNG or WebP image up to 20 MiB"
// @Param expectedLabel formData string false "Expected detection label"
// @Param minimumConfidence formData number false "Minimum confidence from 0 to 1"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/deployments/{deploymentId}/predict-image [post]
func (h *Handler) DeploymentPredictImage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	id, _ := strconv.ParseUint(c.Param("deploymentId"), 10, 64)
	var deployment Deployment
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?", project.TenantID, project.ID, id, "RUNNING").First(&deployment).Error != nil {
		httpx.Fail(c, http.StatusConflict, 409, "部署未就绪，请等待状态变为运行中")
		return
	}
	var revision DeploymentRevision
	if deployment.CurrentRevisionID == 0 || h.db.Where("id = ? AND deployment_id = ? AND status = ?", deployment.CurrentRevisionID, deployment.ID, "RUNNING").First(&revision).Error != nil {
		httpx.Fail(c, http.StatusConflict, 409, "当前部署修订尚未就绪")
		return
	}
	header, err := c.FormFile("file")
	if err != nil || header.Size <= 0 || header.Size > maxRegressionImageSize {
		httpx.Fail(c, http.StatusBadRequest, 400, "请选择不超过 20 MiB 的 JPEG、PNG 或 WebP 图片")
		return
	}
	filename, valid := safeFilename(header.Filename)
	if !valid {
		httpx.Fail(c, http.StatusBadRequest, 400, "图片文件名无效")
		return
	}
	minimumConfidence := 0.5
	if raw := strings.TrimSpace(c.PostForm("minimumConfidence")); raw != "" {
		minimumConfidence, err = strconv.ParseFloat(raw, 64)
		if err != nil || minimumConfidence < 0 || minimumConfidence > 1 {
			httpx.Fail(c, http.StatusBadRequest, 400, "最低置信度必须在 0 到 1 之间")
			return
		}
	}
	expectedLabel := strings.TrimSpace(c.PostForm("expectedLabel"))
	if len(expectedLabel) > 160 {
		httpx.Fail(c, http.StatusBadRequest, 400, "预期标签长度不能超过 160")
		return
	}
	file, err := header.Open()
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, 400, "图片文件无法读取")
		return
	}
	input, readErr := io.ReadAll(io.LimitReader(file, maxRegressionImageSize+1))
	_ = file.Close()
	if readErr != nil || len(input) > maxRegressionImageSize {
		httpx.Fail(c, http.StatusBadRequest, 400, "图片读取失败或超过 20 MiB")
		return
	}
	format, width, height, sourceHash, validationErr := validateRegressionImage(input)
	if validationErr != nil {
		httpx.Fail(c, http.StatusUnprocessableEntity, 422, "文件不是受支持的完整图片")
		return
	}

	traceID := uuid.NewString()
	started := time.Now()
	prediction, predictErr := platforminference.NewClient(config.Load().InferenceAPIURL, config.Load().InferenceTimeout).
		Predict(c.Request.Context(), filename, bytes.NewReader(input))
	elapsed := float64(time.Since(started).Microseconds()) / 1000
	regression := evaluateRegression(prediction.Detections, expectedLabel, minimumConfidence)
	testMode := "ONLINE"
	if expectedLabel != "" {
		testMode = "REGRESSION"
	}
	trace := InferenceTrace{
		TenantID: project.TenantID, ProjectID: project.ID, DeploymentID: deployment.ID,
		DeploymentRevisionID: revision.ID, ModelVersionID: revision.ModelVersionID,
		TraceID: traceID, SourceType: "UPLOAD", SourceName: filename, SourceSHA256: sourceHash,
		TestMode: testMode, ExpectedLabel: expectedLabel, MinimumConfidence: minimumConfidence,
		RegressionStatus: regression.Status, MatchedDetectionCount: regression.MatchedDetectionCount,
		Status: "SUCCEEDED", LatencyMS: elapsed, Result: "{}", CreatedAt: time.Now(),
	}
	if predictErr != nil {
		trace.Status, trace.ErrorMessage = "FAILED", predictErr.Error()
		if expectedLabel != "" {
			trace.RegressionStatus = "FAILED"
			regression.Status = "FAILED"
		}
		h.db.Create(&trace)
		evaluateAlertRules(h.db, trace)
		captureFeedback(h.db, trace)
		_ = appendAudit(h.db, c, project.ID, "INFERENCE_IMAGE_TEST_FAILED", "INFERENCE_TRACE", trace.ID, nil, gin.H{"traceId": traceID, "sourceSha256": sourceHash})
		httpx.Fail(c, http.StatusBadGateway, 502, "推理服务失败："+predictErr.Error())
		return
	}
	confidence := 0.0
	for _, detection := range prediction.Detections {
		confidence += detection.Confidence
	}
	if len(prediction.Detections) > 0 {
		confidence /= float64(len(prediction.Detections))
	}
	trace.DetectionCount = len(prediction.Detections)
	trace.MeanConfidence = confidence
	trace.Result = jsonValue(prediction)
	if err = h.db.Create(&trace).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, 500, "推理结果保存失败")
		return
	}
	evaluateAlertRules(h.db, trace)
	captureFeedback(h.db, trace)
	_ = appendAudit(h.db, c, project.ID, "INFERENCE_IMAGE_TESTED", "INFERENCE_TRACE", trace.ID, nil, gin.H{
		"traceId": traceID, "sourceSha256": sourceHash, "regressionStatus": regression.Status,
	})
	httpx.OK(c, gin.H{
		"traceId": traceID, "deploymentId": deployment.ID, "deploymentRevisionId": revision.ID,
		"modelVersionId": revision.ModelVersionID, "result": prediction, "platformLatencyMs": elapsed,
		"input":      gin.H{"filename": filename, "format": format, "width": width, "height": height, "sha256": sourceHash},
		"regression": regression,
	})
}
