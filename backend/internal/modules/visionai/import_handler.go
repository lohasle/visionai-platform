package visionai

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

type assetImportRequest struct {
	SourceType      string         `json:"sourceType"`
	Source          string         `json:"source"`
	DuplicatePolicy string         `json:"duplicatePolicy"`
	Options         map[string]any `json:"options"`
}

func validRelativeImportPath(value string) bool {
	value = strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	clean := filepath.ToSlash(filepath.Clean(value))
	return value != "" && !filepath.IsAbs(value) && !strings.HasPrefix(value, "/") &&
		!strings.Contains(value, ":") && clean != ".." && !strings.HasPrefix(clean, "../") &&
		!strings.Contains(value, "\x00")
}

// AssetImportCreate godoc
// @Summary Create an asynchronous asset import
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/asset-imports [post]
func (h *Handler) AssetImportCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	var req assetImportRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "导入请求格式错误")
		return
	}
	req.SourceType = strings.ToUpper(strings.TrimSpace(req.SourceType))
	req.DuplicatePolicy = strings.ToUpper(strings.TrimSpace(req.DuplicatePolicy))
	if req.DuplicatePolicy == "" {
		req.DuplicatePolicy = "REFERENCE"
	}
	if req.DuplicatePolicy != "REFERENCE" && req.DuplicatePolicy != "SKIP" && req.DuplicatePolicy != "KEEP" {
		httpx.Fail(c, 400, 400, "重复文件策略无效")
		return
	}
	root := assetRoot(project.TenantID, project.ID)
	switch req.SourceType {
	case "DIRECTORY":
		if !validRelativeImportPath(req.Source) {
			httpx.Fail(c, 400, 400, "目录导入路径无效")
			return
		}
	case "S3_PREFIX", "ZIP", "JSONL":
		if !strings.HasPrefix(req.Source, root+"/") || strings.Contains(req.Source, "..") {
			httpx.Fail(c, http.StatusForbidden, 403, "对象导入源不属于当前项目")
			return
		}
	default:
		httpx.Fail(c, 400, 400, "sourceType 必须是 DIRECTORY、S3_PREFIX、ZIP 或 JSONL")
		return
	}
	configJSON, _ := json.Marshal(gin.H{
		"source": req.Source, "duplicatePolicy": req.DuplicatePolicy, "options": req.Options,
	})
	idempotency := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotency == "" {
		idempotency = uuid.NewString()
	}
	traceID := c.GetString("trace_id")
	if traceID == "" {
		traceID = uuid.NewString()
	}
	job := PlatformJob{
		TenantID: project.TenantID, ProjectID: project.ID, JobType: "ASSET_IMPORT",
		ResourceType: "ASSET_IMPORT", Status: JobQueued, Stage: "QUEUED", TraceID: traceID,
		Idempotency: idempotency, MaxRetries: 3, CreatedBy: c.GetUint64("user_id"),
	}
	run := AssetImportRun{
		TenantID: project.TenantID, ProjectID: project.ID, SourceType: req.SourceType,
		SourceConfig: string(configJSON), Status: "QUEUED", Report: "[]", CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		run.JobID = job.ID
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		job.ResourceID = run.ID
		if err := tx.Model(&job).Update("resource_id", run.ID).Error; err != nil {
			return err
		}
		payload, _ := json.Marshal(gin.H{"importRunId": run.ID, "jobId": job.ID})
		if err := tx.Create(&OutboxEvent{
			TenantID: project.TenantID, EventID: uuid.NewString(), EventType: "asset.import.requested.v1",
			AggregateType: "ASSET_IMPORT", AggregateID: run.ID, Payload: string(payload), Status: outboxNew,
		}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "ASSET_IMPORT_REQUESTED", "ASSET_IMPORT", run.ID, nil, gin.H{"sourceType": run.SourceType, "jobId": job.ID})
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			var existing PlatformJob
			if h.db.Where("idempotency = ?", idempotency).First(&existing).Error == nil {
				httpx.OK(c, gin.H{"job": existing, "idempotentReplay": true})
				return
			}
		}
		httpx.Fail(c, 409, 409, "导入任务创建失败")
		return
	}
	httpx.OK(c, gin.H{"importRun": run, "job": job})
}

// AssetImportGet godoc
// @Summary Get an asset import report
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/asset-imports/{importId} [get]
func (h *Handler) AssetImportGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	importID, _ := strconv.ParseUint(c.Param("importId"), 10, 64)
	var run AssetImportRun
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, importID).First(&run).Error != nil {
		httpx.Fail(c, 404, 404, "导入任务不存在")
		return
	}
	var report []map[string]any
	_ = json.Unmarshal([]byte(run.Report), &report)
	httpx.OK(c, gin.H{"importRun": run, "report": report})
}

// AssetImportPage godoc
// @Summary Page project asset imports
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/asset-imports [get]
func (h *Handler) AssetImportPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	query := h.db.Model(&AssetImportRun{}).Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID)
	var total int64
	query.Count(&total)
	pageNo, pageSize := page(c)
	var rows []AssetImportRun
	query.Order("id DESC").Offset((pageNo - 1) * pageSize).Limit(pageSize).Find(&rows)
	httpx.OK(c, gin.H{"list": rows, "total": total, "serverTime": time.Now()})
}
