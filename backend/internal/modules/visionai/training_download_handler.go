package visionai

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
)

type trainingExportManifest struct {
	ExportedAt time.Time          `json:"exportedAt"`
	Run        TrainingRun        `json:"run"`
	Metrics    []TrainingMetric   `json:"metrics"`
	Artifacts  []TrainingArtifact `json:"artifacts"`
}

func trainingStorageKey(uri, expectedBucket, expectedPrefix string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(uri))
	if err != nil || parsed.Scheme != "s3" || parsed.Host == "" {
		return "", fmt.Errorf("unsupported artifact URI")
	}
	if expectedBucket != "" && parsed.Host != expectedBucket {
		return "", fmt.Errorf("artifact bucket mismatch")
	}
	key := strings.TrimPrefix(path.Clean(parsed.Path), "/")
	prefix := strings.Trim(strings.TrimSpace(expectedPrefix), "/")
	if key == "." || prefix == "" || (key != prefix && !strings.HasPrefix(key, prefix+"/")) {
		return "", fmt.Errorf("artifact is outside the training run prefix")
	}
	return key, nil
}

func trainingExportEntry(kind, name string, id uint64) string {
	filename, ok := safeFilename(name)
	if !ok {
		filename = fmt.Sprintf("artifact-%d.bin", id)
	}
	folder := "artifacts"
	if strings.EqualFold(kind, "LOG") {
		folder = "logs"
	}
	return fmt.Sprintf("%s/%d-%s", folder, id, filename)
}

func (h *Handler) trainingRunForDownload(c *gin.Context) (Project, TrainingRun, bool) {
	project, ok := h.projectAccess(c, false)
	if !ok || !h.storageReady(c) {
		return Project{}, TrainingRun{}, false
	}
	runID, err := strconv.ParseUint(c.Param("runId"), 10, 64)
	if err != nil || runID == 0 {
		httpx.Fail(c, http.StatusBadRequest, 400, "训练运行 ID 无效")
		return Project{}, TrainingRun{}, false
	}
	var run TrainingRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, runID).First(&run).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "训练任务不存在")
		return Project{}, TrainingRun{}, false
	}
	return project, run, true
}

func (h *Handler) storageBucket() string {
	if h.storage == nil {
		return ""
	}
	parsed, err := url.Parse(h.storage.URI(""))
	if err != nil {
		return ""
	}
	return parsed.Host
}

// TrainingArtifactDownload godoc
// @Summary Download one training artifact
// @Tags VisionAI Training
// @Security BearerAuth
// @Produce application/octet-stream
// @Success 200 {file} binary
// @Router /ai-platform/projects/{id}/training-runs/{runId}/artifacts/{artifactId}/download [get]
func (h *Handler) TrainingArtifactDownload(c *gin.Context) {
	project, run, ok := h.trainingRunForDownload(c)
	if !ok {
		return
	}
	artifactID, err := strconv.ParseUint(c.Param("artifactId"), 10, 64)
	if err != nil || artifactID == 0 {
		httpx.Fail(c, http.StatusBadRequest, 400, "训练产物 ID 无效")
		return
	}
	var artifact TrainingArtifact
	if h.db.Where(
		"tenant_id = ? AND project_id = ? AND training_run_id = ? AND id = ?",
		project.TenantID, project.ID, run.ID, artifactID,
	).First(&artifact).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "训练产物不存在")
		return
	}
	prefix := fmt.Sprintf("%s/training/runs/%d", assetRoot(project.TenantID, project.ID), run.ID)
	key, err := trainingStorageKey(artifact.URI, h.storageBucket(), prefix)
	if err != nil {
		httpx.Fail(c, http.StatusConflict, 409, "该产物由外部训练服务管理，不能从平台下载")
		return
	}
	object, info, err := h.storage.Get(c.Request.Context(), key)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "对象存储中的训练产物不存在")
		return
	}
	defer object.Close()
	contentType := strings.TrimSpace(artifact.MediaType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	filename, valid := safeFilename(artifact.Name)
	if !valid {
		filename = fmt.Sprintf("artifact-%d.bin", artifact.ID)
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	_ = appendAudit(h.db, c, project.ID, "TRAINING_ARTIFACT_DOWNLOADED", "TRAINING_ARTIFACT", artifact.ID, nil, gin.H{"runId": run.ID, "name": filename})
	c.DataFromReader(http.StatusOK, info.Size, contentType, object, map[string]string{
		"Content-Disposition":    disposition,
		"Cache-Control":          "private, no-store",
		"X-Content-Type-Options": "nosniff",
	})
}

// TrainingRunExport godoc
// @Summary Export a training run and its artifacts as ZIP
// @Tags VisionAI Training
// @Security BearerAuth
// @Produce application/zip
// @Success 200 {file} binary
// @Router /ai-platform/projects/{id}/training-runs/{runId}/export [get]
func (h *Handler) TrainingRunExport(c *gin.Context) {
	project, run, ok := h.trainingRunForDownload(c)
	if !ok {
		return
	}
	var artifacts []TrainingArtifact
	var metrics []TrainingMetric
	h.db.Where("tenant_id = ? AND project_id = ? AND training_run_id = ?", project.TenantID, project.ID, run.ID).Order("id").Find(&artifacts)
	h.db.Where("tenant_id = ? AND project_id = ? AND training_run_id = ?", project.TenantID, project.ID, run.ID).Order("name, step").Find(&metrics)

	prefix := fmt.Sprintf("%s/training/runs/%d", assetRoot(project.TenantID, project.ID), run.ID)
	type downloadable struct {
		key  string
		name string
	}
	files := make([]downloadable, 0, len(artifacts)+1)
	for _, artifact := range artifacts {
		key, keyErr := trainingStorageKey(artifact.URI, h.storageBucket(), prefix)
		if keyErr != nil {
			continue
		}
		if _, statErr := h.storage.Stat(c.Request.Context(), key); statErr != nil {
			httpx.Fail(c, http.StatusConflict, 409, "训练产物不完整，请等待归集完成后重试")
			return
		}
		files = append(files, downloadable{key: key, name: trainingExportEntry(artifact.Kind, artifact.Name, artifact.ID)})
	}
	if strings.TrimSpace(run.ResultManifestURI) != "" {
		if key, keyErr := trainingStorageKey(run.ResultManifestURI, h.storageBucket(), prefix); keyErr == nil {
			if _, statErr := h.storage.Stat(c.Request.Context(), key); statErr != nil {
				httpx.Fail(c, http.StatusConflict, 409, "训练结果清单缺失，请等待归集完成后重试")
				return
			}
			files = append(files, downloadable{key: key, name: "result-manifest.json"})
		}
	}

	manifest, err := json.MarshalIndent(trainingExportManifest{
		ExportedAt: time.Now().UTC(), Run: run, Metrics: metrics, Artifacts: artifacts,
	}, "", "  ")
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, 500, "训练导出清单生成失败")
		return
	}
	filename := fmt.Sprintf("training-run-%d-export.zip", run.ID)
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)

	writer := zip.NewWriter(c.Writer)
	manifestEntry, createErr := writer.Create("training-run.json")
	if createErr == nil {
		_, createErr = manifestEntry.Write(manifest)
	}
	for _, file := range files {
		if createErr != nil {
			break
		}
		object, _, getErr := h.storage.Get(c.Request.Context(), file.key)
		if getErr != nil {
			createErr = getErr
			break
		}
		entry, entryErr := writer.Create(file.name)
		if entryErr == nil {
			_, entryErr = io.Copy(entry, object)
		}
		_ = object.Close()
		createErr = entryErr
	}
	_ = writer.Close()
	if createErr == nil {
		_ = appendAudit(h.db, c, project.ID, "TRAINING_RUN_EXPORTED", "TRAINING_RUN", run.ID, nil, gin.H{"downloadableFiles": len(files)})
	}
}
