package visionai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	defaultChunkSize = 16 << 20
	minChunkSize     = 5 << 20
	maxChunkSize     = 64 << 20
	maxUploadSize    = 50 << 30
)

type uploadCreateRequest struct {
	Filename        string `json:"filename"`
	ContentType     string `json:"contentType"`
	TotalSize       int64  `json:"totalSize"`
	ChunkSize       int64  `json:"chunkSize"`
	DuplicatePolicy string `json:"duplicatePolicy"`
}

func safeFilename(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 512 || strings.ContainsAny(name, `/\`) || name == "." || name == ".." || strings.Contains(name, "\x00") {
		return "", false
	}
	return filepath.Base(name), true
}

func assetRoot(tenantID, projectID uint64) string {
	return fmt.Sprintf("tenants/%d/projects/%d", tenantID, projectID)
}

func (h *Handler) storageReady(c *gin.Context) bool {
	if h.storageError != nil || h.storage == nil {
		httpx.Fail(c, http.StatusServiceUnavailable, 503, "对象存储配置不可用")
		return false
	}
	if err := h.storage.EnsureBucket(c.Request.Context()); err != nil {
		httpx.Fail(c, http.StatusServiceUnavailable, 503, "对象存储暂不可用")
		return false
	}
	return true
}

// UploadCreate godoc
// @Summary Create a resumable asset upload session
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/uploads [post]
func (h *Handler) UploadCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") || !h.storageReady(c) {
		return
	}
	var req uploadCreateRequest
	filename, validName := "", false
	if c.ShouldBindJSON(&req) == nil {
		filename, validName = safeFilename(req.Filename)
	}
	if !validName || req.TotalSize <= 0 || req.TotalSize > maxUploadSize {
		httpx.Fail(c, 400, 400, "文件名或文件大小无效")
		return
	}
	if req.ChunkSize == 0 {
		req.ChunkSize = defaultChunkSize
	}
	if req.ChunkSize < minChunkSize || req.ChunkSize > maxChunkSize {
		httpx.Fail(c, 400, 400, "分块大小必须在 5MiB 到 64MiB 之间")
		return
	}
	req.DuplicatePolicy = strings.ToUpper(req.DuplicatePolicy)
	if req.DuplicatePolicy == "" {
		req.DuplicatePolicy = "REFERENCE"
	}
	if req.DuplicatePolicy != "REFERENCE" && req.DuplicatePolicy != "SKIP" && req.DuplicatePolicy != "KEEP" {
		httpx.Fail(c, 400, 400, "重复文件策略无效")
		return
	}
	totalChunks := int((req.TotalSize + req.ChunkSize - 1) / req.ChunkSize)
	session := UploadSession{
		ID: uuid.NewString(), TenantID: project.TenantID, ProjectID: project.ID,
		Filename: filename, DeclaredType: req.ContentType, TotalSize: req.TotalSize,
		ChunkSize: req.ChunkSize, TotalChunks: totalChunks, Status: UploadCreated,
		DuplicatePolicy: req.DuplicatePolicy, CreatedBy: c.GetUint64("user_id"),
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}
	if err := h.db.Create(&session).Error; err != nil {
		httpx.Fail(c, 500, 500, "上传会话创建失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "UPLOAD_SESSION_CREATED", "UPLOAD_SESSION", 0, nil, gin.H{"sessionId": session.ID, "filename": session.Filename, "totalSize": session.TotalSize})
	httpx.OK(c, session)
}

func (h *Handler) uploadSession(c *gin.Context, project Project) (UploadSession, bool) {
	var session UploadSession
	if h.db.Where("id = ? AND tenant_id = ? AND project_id = ?", c.Param("sessionId"), project.TenantID, project.ID).First(&session).Error != nil {
		httpx.Fail(c, 404, 404, "上传会话不存在")
		return session, false
	}
	return session, true
}

// UploadGet godoc
// @Summary Get resumable upload progress
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/uploads/{sessionId} [get]
func (h *Handler) UploadGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	session, ok := h.uploadSession(c, project)
	if !ok {
		return
	}
	var chunks []UploadChunk
	h.db.Where("tenant_id = ? AND session_id = ?", project.TenantID, session.ID).Order("part").Find(&chunks)
	parts := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		parts = append(parts, chunk.Part)
	}
	httpx.OK(c, gin.H{"session": session, "receivedParts": parts})
}

// UploadChunkPut godoc
// @Summary Upload one resumable file chunk
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/uploads/{sessionId}/chunks/{part} [put]
func (h *Handler) UploadChunkPut(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") || !h.storageReady(c) {
		return
	}
	session, ok := h.uploadSession(c, project)
	if !ok {
		return
	}
	part, _ := strconv.Atoi(c.Param("part"))
	if session.Status == UploadCompleted || time.Now().After(session.ExpiresAt) || part < 1 || part > session.TotalChunks {
		httpx.Fail(c, 409, 409, "上传会话状态或分块序号无效")
		return
	}
	expectedSize := session.ChunkSize
	if part == session.TotalChunks {
		expectedSize = session.TotalSize - int64(part-1)*session.ChunkSize
	}
	if c.Request.ContentLength != expectedSize || c.Request.ContentLength <= 0 {
		httpx.Fail(c, 400, 400, "分块 Content-Length 与会话不一致")
		return
	}
	expectedHash := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Chunk-SHA256")))
	if len(expectedHash) != 64 {
		httpx.Fail(c, 400, 400, "X-Chunk-SHA256 必填")
		return
	}
	key := fmt.Sprintf("%s/uploads/%s/parts/%06d", assetRoot(project.TenantID, project.ID), session.ID, part)
	hasher := sha256.New()
	reader := io.TeeReader(io.LimitReader(c.Request.Body, expectedSize), hasher)
	if err := h.storage.Put(c.Request.Context(), key, reader, expectedSize, "application/octet-stream"); err != nil {
		httpx.Fail(c, 503, 503, "分块写入对象存储失败")
		return
	}
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		_ = h.storage.Delete(c.Request.Context(), key)
		httpx.Fail(c, 422, 422, "分块校验和不匹配")
		return
	}
	chunk := UploadChunk{TenantID: project.TenantID, SessionID: session.ID, Part: part, ObjectKey: key, Size: expectedSize, SHA256: actualHash}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var existing UploadChunk
		err := tx.Where("tenant_id = ? AND session_id = ? AND part = ?", project.TenantID, session.ID, part).First(&existing).Error
		if err == nil {
			chunk.ID = existing.ID
			if err = tx.Save(&chunk).Error; err != nil {
				return err
			}
		} else if err = tx.Create(&chunk).Error; err != nil {
			return err
		}
		var received int64
		tx.Model(&UploadChunk{}).Where("tenant_id = ? AND session_id = ?", project.TenantID, session.ID).Count(&received)
		return tx.Model(&UploadSession{}).Where("id = ?", session.ID).Updates(map[string]any{"status": UploadActive, "received_chunks": received}).Error
	}); err != nil {
		httpx.Fail(c, 500, 500, "分块状态保存失败")
		return
	}
	httpx.OK(c, gin.H{"part": part, "sha256": actualHash})
}

func inspectImage(ctx context.Context, h *Handler, key string) (hash, contentType string, width, height int, size int64, err error) {
	object, info, err := h.storage.Get(ctx, key)
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	defer object.Close()
	hasher := sha256.New()
	tee := io.TeeReader(object, hasher)
	cfg, format, err := image.DecodeConfig(tee)
	if err != nil {
		return "", "", 0, 0, info.Size, fmt.Errorf("decode image header: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
		return "", "", 0, 0, info.Size, fmt.Errorf("unsafe image dimensions")
	}
	if _, err = io.Copy(hasher, object); err != nil {
		return "", "", 0, 0, info.Size, fmt.Errorf("hash image: %w", err)
	}
	contentType = mime.TypeByExtension("." + format)
	if contentType == "" {
		contentType = "image/" + format
	}
	// Header parsing alone misses truncated pixel streams. Fully decode ordinary
	// assets while keeping very large files on the bounded header-validation path.
	if info.Size <= 128<<20 {
		verify, _, verifyErr := h.storage.Get(ctx, key)
		if verifyErr != nil {
			return "", "", 0, 0, info.Size, verifyErr
		}
		_, _, decodeErr := image.Decode(verify)
		_ = verify.Close()
		if decodeErr != nil {
			return "", "", 0, 0, info.Size, fmt.Errorf("decode full image: %w", decodeErr)
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), contentType, cfg.Width, cfg.Height, info.Size, nil
}

func generateThumbnail(ctx context.Context, h *Handler, sourceKey, thumbnailKey string) error {
	object, _, err := h.storage.Get(ctx, sourceKey)
	if err != nil {
		return err
	}
	source, _, err := image.Decode(object)
	_ = object.Close()
	if err != nil {
		return fmt.Errorf("decode thumbnail source: %w", err)
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	const maxSide = 256
	targetWidth, targetHeight := width, height
	if width > maxSide || height > maxSide {
		if width >= height {
			targetWidth = maxSide
			targetHeight = max(1, height*maxSide/width)
		} else {
			targetHeight = maxSide
			targetWidth = max(1, width*maxSide/height)
		}
	}
	thumbnail := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), source, bounds, xdraw.Over, nil)
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, thumbnail, &jpeg.Options{Quality: 82}); err != nil {
		return fmt.Errorf("encode thumbnail: %w", err)
	}
	return h.storage.Put(ctx, thumbnailKey, bytes.NewReader(encoded.Bytes()), int64(encoded.Len()), "image/jpeg")
}

// UploadComplete godoc
// @Summary Compose, validate and register an uploaded asset
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/uploads/{sessionId}/complete [post]
func (h *Handler) UploadComplete(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") || !h.storageReady(c) {
		return
	}
	session, ok := h.uploadSession(c, project)
	if !ok {
		return
	}
	if session.Status == UploadCompleted {
		var asset Asset
		h.db.Where("tenant_id = ? AND id = ?", project.TenantID, session.AssetID).First(&asset)
		httpx.OK(c, asset)
		return
	}
	var chunks []UploadChunk
	h.db.Where("tenant_id = ? AND session_id = ?", project.TenantID, session.ID).Order("part").Find(&chunks)
	if len(chunks) != session.TotalChunks {
		httpx.Fail(c, 409, 409, "分块尚未全部上传")
		return
	}
	keys := make([]string, len(chunks))
	for i, chunk := range chunks {
		if chunk.Part != i+1 {
			httpx.Fail(c, 409, 409, "分块序列不连续")
			return
		}
		keys[i] = chunk.ObjectKey
	}
	destination := fmt.Sprintf("%s/assets/%s/%s", assetRoot(project.TenantID, project.ID), uuid.NewString(), session.Filename)
	if err := h.storage.Compose(c.Request.Context(), destination, keys); err != nil {
		h.failUpload(session, "COMPOSE_FAILED", err.Error())
		httpx.Fail(c, 503, 503, "对象分块合并失败")
		return
	}
	hash, contentType, width, height, size, inspectErr := inspectImage(c.Request.Context(), h, destination)
	thumbnailKey := destination + ".thumbnail.jpg"
	if inspectErr == nil {
		inspectErr = generateThumbnail(c.Request.Context(), h, destination, thumbnailKey)
	}
	if inspectErr != nil {
		asset := Asset{
			TenantID: project.TenantID, ProjectID: project.ID, Filename: session.Filename,
			ObjectKey: destination, URI: h.storage.URI(destination), ContentType: "application/octet-stream",
			Size: session.TotalSize, Status: AssetInvalid, Metadata: "{}", ErrorCode: "IMAGE_DECODE_FAILED",
			ErrorMessage: inspectErr.Error(), CreatedBy: c.GetUint64("user_id"),
		}
		_ = h.db.Create(&asset).Error
		h.failUploadWithAsset(session, asset.ID, "IMAGE_DECODE_FAILED", inspectErr.Error())
		httpx.Fail(c, 422, 422, "文件不是受支持的完整图像")
		return
	}
	var duplicate Asset
	hasDuplicate := h.db.Where("tenant_id = ? AND project_id = ? AND sha256 = ? AND status = ?", project.TenantID, project.ID, hash, AssetReady).First(&duplicate).Error == nil
	if hasDuplicate && session.DuplicatePolicy == "SKIP" {
		_ = h.storage.Delete(c.Request.Context(), destination)
		_ = h.storage.Delete(c.Request.Context(), thumbnailKey)
		h.completeUpload(session, duplicate.ID)
		h.cleanupChunks(c.Request.Context(), chunks)
		httpx.OK(c, gin.H{"asset": duplicate, "duplicate": true, "strategy": "SKIP"})
		return
	}
	metadata, _ := json.Marshal(gin.H{"declaredContentType": session.DeclaredType, "actualFormat": contentType})
	asset := Asset{
		TenantID: project.TenantID, ProjectID: project.ID, Filename: session.Filename,
		ObjectKey: destination, URI: h.storage.URI(destination), SHA256: hash, ContentType: contentType,
		ThumbnailObjectKey: thumbnailKey, ThumbnailURI: h.storage.URI(thumbnailKey),
		Size: size, Width: width, Height: height, Status: AssetReady,
		Metadata: string(metadata), CreatedBy: c.GetUint64("user_id"),
	}
	if hasDuplicate && session.DuplicatePolicy == "REFERENCE" {
		_ = h.storage.Delete(c.Request.Context(), destination)
		_ = h.storage.Delete(c.Request.Context(), thumbnailKey)
		asset.ObjectKey, asset.URI, asset.DuplicateOfID = duplicate.ObjectKey, duplicate.URI, duplicate.ID
		asset.ThumbnailObjectKey, asset.ThumbnailURI = duplicate.ThumbnailObjectKey, duplicate.ThumbnailURI
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&asset).Error; err != nil {
			return err
		}
		if err := tx.Model(&UploadSession{}).Where("id = ?", session.ID).Updates(map[string]any{"status": UploadCompleted, "asset_id": asset.ID, "error_code": "", "error_message": ""}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "ASSET_CREATED", "ASSET", asset.ID, nil, asset)
	}); err != nil {
		_ = h.storage.Delete(c.Request.Context(), destination)
		httpx.Fail(c, 500, 500, "资产登记失败")
		return
	}
	h.cleanupChunks(c.Request.Context(), chunks)
	httpx.OK(c, gin.H{"asset": asset, "duplicate": hasDuplicate, "strategy": session.DuplicatePolicy})
}

func (h *Handler) failUpload(session UploadSession, code, message string) {
	h.failUploadWithAsset(session, 0, code, message)
}

func (h *Handler) failUploadWithAsset(session UploadSession, assetID uint64, code, message string) {
	h.db.Model(&UploadSession{}).Where("id = ?", session.ID).Updates(map[string]any{"status": UploadFailed, "asset_id": assetID, "error_code": code, "error_message": message})
}

func (h *Handler) completeUpload(session UploadSession, assetID uint64) {
	h.db.Model(&UploadSession{}).Where("id = ?", session.ID).Updates(map[string]any{"status": UploadCompleted, "asset_id": assetID})
}

func (h *Handler) cleanupChunks(ctx context.Context, chunks []UploadChunk) {
	for _, chunk := range chunks {
		_ = h.storage.Delete(ctx, chunk.ObjectKey)
	}
}

// AssetPage godoc
// @Summary Page project assets
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/assets [get]
func (h *Handler) AssetPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	query := h.db.Model(&Asset{}).Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID)
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status <> ?", AssetDeleted)
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		query = query.Where("filename LIKE ? OR sha256 LIKE ?", "%"+keyword+"%", keyword+"%")
	}
	tagValues := c.QueryArray("tagDefinitionIds")
	if len(tagValues) == 0 && strings.TrimSpace(c.Query("tagDefinitionIds")) != "" {
		tagValues = strings.Split(c.Query("tagDefinitionIds"), ",")
	}
	tagIDs := make([]uint64, 0, len(tagValues))
	for _, value := range tagValues {
		if id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64); err == nil && id > 0 {
			tagIDs = append(tagIDs, id)
		}
	}
	if len(tagIDs) > 0 {
		query = query.Where(`id IN (
			SELECT asset_id FROM ai_asset_tag
			WHERE tenant_id = ? AND project_id = ? AND definition_id IN ?
			GROUP BY asset_id HAVING COUNT(DISTINCT definition_id) = ?
		)`, project.TenantID, project.ID, tagIDs, len(tagIDs))
	}
	var total int64
	query.Count(&total)
	pageNo, pageSize := page(c)
	var rows []Asset
	query.Order("id DESC").Offset((pageNo - 1) * pageSize).Limit(pageSize).Find(&rows)
	type assetPageView struct {
		Asset
		ThumbnailURL string         `json:"thumbnailUrl"`
		Tags         []assetTagView `json:"tags"`
	}
	assetIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		assetIDs = append(assetIDs, row.ID)
	}
	tagsByAsset := assetTagViews(h.db, project, assetIDs)
	result := make([]assetPageView, 0, len(rows))
	for _, row := range rows {
		view := assetPageView{Asset: row, Tags: tagsByAsset[row.ID]}
		if h.storage != nil && row.ThumbnailObjectKey != "" {
			if signed, err := h.storage.PresignedGet(c.Request.Context(), row.ThumbnailObjectKey, 10*time.Minute); err == nil {
				view.ThumbnailURL = signed.String()
			}
		}
		result = append(result, view)
	}
	httpx.OK(c, gin.H{"list": result, "total": total})
}

func (h *Handler) getAsset(c *gin.Context, project Project) (Asset, bool) {
	id, _ := strconv.ParseUint(c.Param("assetId"), 10, 64)
	var asset Asset
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&asset).Error != nil {
		httpx.Fail(c, 404, 404, "资产不存在")
		return asset, false
	}
	return asset, true
}

// AssetGet godoc
// @Summary Get asset metadata and a short-lived preview URL
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/assets/{assetId} [get]
func (h *Handler) AssetGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok || !h.storageReady(c) {
		return
	}
	asset, ok := h.getAsset(c, project)
	if !ok {
		return
	}
	var metadata map[string]any
	_ = json.Unmarshal([]byte(asset.Metadata), &metadata)
	previewURL := ""
	thumbnailURL := ""
	if asset.Status != AssetPurged && asset.Status != AssetMissing {
		if signed, err := h.storage.PresignedGet(c.Request.Context(), asset.ObjectKey, 10*time.Minute); err == nil {
			previewURL = signed.String()
		}
	}
	if asset.ThumbnailObjectKey != "" {
		if signed, err := h.storage.PresignedGet(c.Request.Context(), asset.ThumbnailObjectKey, 10*time.Minute); err == nil {
			thumbnailURL = signed.String()
		}
	}
	httpx.OK(c, gin.H{
		"asset": asset, "metadata": metadata, "tags": assetTagViews(h.db, project, []uint64{asset.ID})[asset.ID],
		"previewUrl": previewURL, "thumbnailUrl": thumbnailURL, "previewExpiresIn": 600,
	})
}

// AssetDelete godoc
// @Summary Move an asset to the recycle bin
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/assets/{assetId} [delete]
func (h *Handler) AssetDelete(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	asset, ok := h.getAsset(c, project)
	if !ok {
		return
	}
	now := time.Now()
	before := asset
	asset.Status, asset.DeletedAt, asset.DeletedBy = AssetDeleted, &now, c.GetUint64("user_id")
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&asset).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "ASSET_RECYCLED", "ASSET", asset.ID, before, asset)
	}); err != nil {
		httpx.Fail(c, 500, 500, "资产移入回收站失败")
		return
	}
	httpx.OK(c, true)
}

// AssetRestore godoc
// @Summary Restore an asset from the recycle bin
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/assets/{assetId}/restore [post]
func (h *Handler) AssetRestore(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	asset, ok := h.getAsset(c, project)
	if !ok || asset.Status != AssetDeleted {
		if ok {
			httpx.Fail(c, 409, 409, "资产不在回收站")
		}
		return
	}
	asset.Status, asset.DeletedAt, asset.DeletedBy = AssetReady, nil, 0
	if err := h.db.Save(&asset).Error; err != nil {
		httpx.Fail(c, 500, 500, "资产恢复失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "ASSET_RESTORED", "ASSET", asset.ID, nil, asset)
	httpx.OK(c, asset)
}

// AssetPurge godoc
// @Summary Permanently purge an unreferenced recycled asset
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/assets/{assetId}/purge [post]
func (h *Handler) AssetPurge(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") || !h.storageReady(c) {
		return
	}
	asset, ok := h.getAsset(c, project)
	if !ok || asset.Status != AssetDeleted {
		if ok {
			httpx.Fail(c, 409, 409, "仅回收站资产可物理清理")
		}
		return
	}
	var frozen, shared int64
	h.db.Model(&AssetReference{}).Where("tenant_id = ? AND asset_id = ? AND frozen = ?", project.TenantID, asset.ID, true).Count(&frozen)
	h.db.Model(&Asset{}).Where("tenant_id = ? AND object_key = ? AND id <> ? AND status <> ?", project.TenantID, asset.ObjectKey, asset.ID, AssetPurged).Count(&shared)
	if frozen > 0 || asset.ReferenceCount > 0 || shared > 0 {
		httpx.Fail(c, 409, 409, "资产仍被数据集/模型版本引用，不能物理清理")
		return
	}
	if err := h.storage.Delete(c.Request.Context(), asset.ObjectKey); err != nil {
		httpx.Fail(c, 503, 503, "对象存储删除失败")
		return
	}
	if asset.ThumbnailObjectKey != "" {
		_ = h.storage.Delete(c.Request.Context(), asset.ThumbnailObjectKey)
	}
	asset.Status, asset.URI = AssetPurged, ""
	if err := h.db.Save(&asset).Error; err != nil {
		httpx.Fail(c, 500, 500, "资产清理状态保存失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "ASSET_PURGED", "ASSET", asset.ID, nil, gin.H{"status": AssetPurged})
	httpx.OK(c, true)
}

// AssetQuality godoc
// @Summary Summarize project asset quality states
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/assets/quality [get]
func (h *Handler) AssetQuality(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	type statusCount struct {
		Status AssetStatus `json:"status"`
		Count  int64       `json:"count"`
	}
	var counts []statusCount
	h.db.Model(&Asset{}).Select("status, count(*) as count").Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Group("status").Scan(&counts)
	var duplicateGroups int64
	h.db.Raw("SELECT COUNT(*) FROM (SELECT sha256 FROM ai_asset WHERE tenant_id = ? AND project_id = ? AND status = ? GROUP BY sha256 HAVING COUNT(*) > 1) d", project.TenantID, project.ID, AssetReady).Scan(&duplicateGroups)
	httpx.OK(c, gin.H{"byStatus": counts, "duplicateGroups": duplicateGroups})
}
