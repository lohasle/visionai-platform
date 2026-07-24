package visionai

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"github.com/lohasle/nimbus-framework-go/internal/platform/storage"
	"gorm.io/gorm"
)

type Handler struct {
	db           *gorm.DB
	storage      storage.Provider
	storageError error
}

func NewHandler(db *gorm.DB) *Handler {
	provider, err := storage.NewMinIO(config.Load())
	return &Handler{db: db, storage: provider, storageError: err}
}

func tenantID(c *gin.Context) uint64 { return c.GetUint64("tenant_id") }

func page(c *gin.Context) (int, int) {
	pageNo, _ := strconv.Atoi(c.DefaultQuery("pageNo", c.DefaultQuery("page", "1")))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if pageNo < 1 {
		pageNo = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return pageNo, pageSize
}

// DashboardSummary godoc
// @Summary VisionAI dashboard summary
// @Tags VisionAI Workbench
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/dashboard/summary [get]
func (h *Handler) DashboardSummary(c *gin.Context) {
	var running, failed, pending int64
	if h.db != nil {
		h.db.Model(&PlatformJob{}).Where("tenant_id = ? AND status IN ?", tenantID(c), []JobStatus{JobQueued, JobRunning}).Count(&running)
		h.db.Model(&PlatformJob{}).Where("tenant_id = ? AND status = ?", tenantID(c), JobFailed).Count(&failed)
		h.db.Model(&PlatformJob{}).Where("tenant_id = ? AND status = ?", tenantID(c), JobPending).Count(&pending)
	}
	httpx.OK(c, gin.H{
		"jobs": gin.H{"running": running, "failed": failed, "pending": pending},
		"lifecycle": []gin.H{
			{"key": "data", "label": "数据准备", "count": 0},
			{"key": "annotation", "label": "标注", "count": 0},
			{"key": "training", "label": "训练", "count": 0},
			{"key": "evaluation", "label": "评估", "count": 0},
			{"key": "deployment", "label": "部署", "count": 0},
		},
		"services": []gin.H{{"name": "platform-api", "status": "UP"}},
	})
}

// JobPage godoc
// @Summary Page platform jobs
// @Tags VisionAI Job
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/jobs [get]
func (h *Handler) JobPage(c *gin.Context) {
	if h.db == nil {
		httpx.OK(c, gin.H{"list": []PlatformJob{}, "total": 0})
		return
	}
	query := h.db.Model(&PlatformJob{}).Where("tenant_id = ?", tenantID(c))
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if jobType := c.Query("jobType"); jobType != "" {
		query = query.Where("job_type = ?", jobType)
	}
	var total int64
	query.Count(&total)
	pageNo, pageSize := page(c)
	var jobs []PlatformJob
	query.Order("id DESC").Offset((pageNo - 1) * pageSize).Limit(pageSize).Find(&jobs)
	httpx.OK(c, gin.H{"list": jobs, "total": total})
}

// JobGet godoc
// @Summary Get a platform job
// @Tags VisionAI Job
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/jobs/{id} [get]
func (h *Handler) JobGet(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var job PlatformJob
	if h.db == nil || h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&job).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "任务不存在")
		return
	}
	var attempts []JobAttempt
	h.db.Where("tenant_id = ? AND job_id = ?", tenantID(c), id).Order("attempt").Find(&attempts)
	httpx.OK(c, gin.H{"job": job, "attempts": attempts})
}

// JobCancel godoc
// @Summary Request platform job cancellation
// @Tags VisionAI Job
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/jobs/{id}/cancel [post]
func (h *Handler) JobCancel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var job PlatformJob
	if h.db == nil || h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&job).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "任务不存在")
		return
	}
	if !CanTransitionJob(job.Status, JobCancelled) {
		httpx.Fail(c, http.StatusConflict, 409, "当前任务状态不允许取消")
		return
	}
	now := time.Now()
	job.CancelAt, job.FinishedAt = &now, &now
	_ = TransitionJob(&job, JobCancelled)
	if err := h.db.Save(&job).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, 500, "取消任务失败")
		return
	}
	httpx.OK(c, true)
}

// JobRetry godoc
// @Summary Retry a failed platform job
// @Tags VisionAI Job
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/jobs/{id}/retry [post]
func (h *Handler) JobRetry(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var job PlatformJob
	if h.db == nil || h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&job).Error != nil {
		httpx.Fail(c, http.StatusNotFound, 404, "任务不存在")
		return
	}
	if job.RetryCount >= job.MaxRetries || TransitionJob(&job, JobQueued) != nil {
		httpx.Fail(c, http.StatusConflict, 409, "任务不可重试或已达到重试上限")
		return
	}
	job.RetryCount++
	if err := h.db.Save(&job).Error; err != nil {
		httpx.Fail(c, http.StatusInternalServerError, 500, "重试任务失败")
		return
	}
	httpx.OK(c, job)
}

// JobEvents godoc
// @Summary Stream platform job progress through SSE
// @Tags VisionAI Job
// @Produce text/event-stream
// @Security BearerAuth
// @Router /ai-platform/jobs/{id}/events [get]
func (h *Handler) JobEvents(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var job PlatformJob
		if h.db == nil || h.db.Where("tenant_id = ? AND id = ?", tenantID(c), id).First(&job).Error != nil {
			fmt.Fprintf(c.Writer, "event: error\ndata: {\"code\":\"JOB_NOT_FOUND\"}\n\n")
			c.Writer.Flush()
			return
		}
		c.SSEvent("progress", job)
		c.Writer.Flush()
		if job.Status == JobSucceeded || job.Status == JobFailed || job.Status == JobCancelled {
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
