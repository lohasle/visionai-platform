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
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	evaluationplatform "github.com/lohasle/nimbus-framework-go/internal/platform/evaluation"
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
	Filename        string            `json:"filename"`
	ContentType     string            `json:"contentType"`
	TotalSize       int64             `json:"totalSize"`
	ChunkSize       int64             `json:"chunkSize"`
	DuplicatePolicy string            `json:"duplicatePolicy"`
	Metadata        map[string]string `json:"metadata"`
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
		Metadata:  normalizeAssetMetadata(req.Metadata),
		ExpiresAt: time.Now().Add(48 * time.Hour),
	}
	if err := h.db.Create(&session).Error; err != nil {
		httpx.Fail(c, 500, 500, "上传会话创建失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "UPLOAD_SESSION_CREATED", "UPLOAD_SESSION", 0, nil, gin.H{"sessionId": session.ID, "filename": session.Filename, "totalSize": session.TotalSize})
	httpx.OK(c, session)
}

func normalizeAssetMetadata(values map[string]string) string {
	result := map[string]string{}
	for _, key := range []string{"language", "sourceDevice", "businessScene"} {
		value := strings.TrimSpace(values[key])
		if len(value) > 256 {
			value = value[:256]
		}
		if value != "" {
			result[key] = value
		}
	}
	return jsonValue(result)
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

type storedAssetInspection struct {
	Hash            string
	ContentType     string
	MediaKind       string
	Width           int
	Height          int
	DurationSeconds float64
	Codec           string
	Language        string
	SourceDevice    string
	Size            int64
	Metadata        map[string]any
}

type ffprobeResult struct {
	Streams []struct {
		CodecType string            `json:"codec_type"`
		CodecName string            `json:"codec_name"`
		Width     int               `json:"width"`
		Height    int               `json:"height"`
		Tags      map[string]string `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration   string            `json:"duration"`
		FormatName string            `json:"format_name"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
}

func isVideoAsset(filename, declaredType string) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(declaredType)), "video/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4", ".mov", ".m4v", ".webm", ".avi", ".mkv":
		return true
	default:
		return false
	}
}

func isTextAsset(filename, declaredType string) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(declaredType)), "text/") ||
		strings.EqualFold(strings.TrimSpace(declaredType), "application/json") ||
		strings.EqualFold(strings.TrimSpace(declaredType), "application/x-ndjson") {
		return true
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".txt", ".json", ".jsonl", ".csv", ".tsv", ".xml", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

var sensitiveTextPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|passwd|secret|access[_-]?key|api[_-]?key|bearer|authorization)\s*[:=]`),
	regexp.MustCompile(`(?i)(id[_-]?card|identity[_-]?number|phone|mobile|email)\s*[:=]`),
	regexp.MustCompile(`(身份证|手机号|银行卡|访问密钥|密码)\s*[:：]`),
	regexp.MustCompile(`\b1[3-9]\d{9}\b`),
	regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
}

func inspectStoredText(ctx context.Context, h *Handler, key, filename, declaredType string) (storedAssetInspection, error) {
	var result storedAssetInspection
	object, info, err := h.storage.Get(ctx, key)
	if err != nil {
		return result, err
	}
	defer object.Close()
	if info.Size > 32<<20 {
		return result, errors.New("text asset exceeds the 32 MiB inspection limit")
	}
	content, err := io.ReadAll(io.LimitReader(object, (32<<20)+1))
	if err != nil {
		return result, fmt.Errorf("read text asset: %w", err)
	}
	contentType := strings.TrimSpace(declaredType)
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	}
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	sum := sha256.Sum256(content)
	result = storedAssetInspection{
		Hash: hex.EncodeToString(sum[:]), ContentType: contentType, MediaKind: "TEXT",
		Codec: "utf-8", Size: info.Size, Metadata: map[string]any{"actualFormat": contentType},
	}
	if !utf8.Valid(content) {
		return result, errors.New("text asset is not valid UTF-8")
	}
	text := strings.TrimSpace(string(content))
	result.Metadata["textCharacters"] = utf8.RuneCountInString(text)
	if text == "" {
		return result, errors.New("empty text asset")
	}
	for _, pattern := range sensitiveTextPatterns {
		if pattern.MatchString(text) {
			result.Metadata["sensitiveFieldDetected"] = true
			return result, errors.New("sensitive field detected in text asset")
		}
	}
	return result, nil
}

func inspectStoredVideo(ctx context.Context, h *Handler, key, filename string) (storedAssetInspection, error) {
	var result storedAssetInspection
	object, info, err := h.storage.Get(ctx, key)
	if err != nil {
		return result, err
	}
	hasher := sha256.New()
	if _, err = io.Copy(hasher, object); err != nil {
		_ = object.Close()
		return result, fmt.Errorf("hash video: %w", err)
	}
	_ = object.Close()
	probeInput, _, err := h.storage.Get(ctx, key)
	if err != nil {
		return result, err
	}
	defer probeInput.Close()
	command := exec.CommandContext(
		ctx, "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", "pipe:0",
	)
	command.Stdin = probeInput
	output, err := command.Output()
	if err != nil {
		return result, fmt.Errorf("ffprobe video: %w", err)
	}
	var probe ffprobeResult
	if json.Unmarshal(output, &probe) != nil {
		return result, errors.New("decode ffprobe output")
	}
	var video *struct {
		CodecType string            `json:"codec_type"`
		CodecName string            `json:"codec_name"`
		Width     int               `json:"width"`
		Height    int               `json:"height"`
		Tags      map[string]string `json:"tags"`
	}
	for i := range probe.Streams {
		if probe.Streams[i].CodecType == "video" {
			video = &probe.Streams[i]
			break
		}
	}
	if video == nil || video.Width <= 0 || video.Height <= 0 {
		return result, errors.New("video stream is missing")
	}
	duration, durationErr := strconv.ParseFloat(strings.TrimSpace(probe.Format.Duration), 64)
	if durationErr != nil || duration <= 0 {
		return result, errors.New("video duration is invalid")
	}
	language := strings.TrimSpace(video.Tags["language"])
	if language == "" {
		language = strings.TrimSpace(probe.Format.Tags["language"])
	}
	device := strings.TrimSpace(probe.Format.Tags["com.apple.quicktime.make"] + " " + probe.Format.Tags["com.apple.quicktime.model"])
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if contentType == "" {
		contentType = "video/" + strings.Split(probe.Format.FormatName, ",")[0]
	}
	result = storedAssetInspection{
		Hash: hex.EncodeToString(hasher.Sum(nil)), ContentType: contentType, MediaKind: "VIDEO",
		Width: video.Width, Height: video.Height, DurationSeconds: duration, Codec: video.CodecName,
		Language: language, SourceDevice: device, Size: info.Size,
		Metadata: map[string]any{
			"actualFormat": probe.Format.FormatName, "durationSeconds": duration,
			"codec": video.CodecName, "language": language, "sourceDevice": device,
		},
	}
	return result, nil
}

func inspectStoredAsset(ctx context.Context, h *Handler, key, filename, declaredType string) (storedAssetInspection, error) {
	if isVideoAsset(filename, declaredType) {
		return inspectStoredVideo(ctx, h, key, filename)
	}
	if isTextAsset(filename, declaredType) {
		return inspectStoredText(ctx, h, key, filename, declaredType)
	}
	hash, contentType, width, height, size, err := inspectImage(ctx, h, key)
	return storedAssetInspection{
		Hash: hash, ContentType: contentType, MediaKind: "IMAGE", Width: width, Height: height,
		Size: size, Metadata: map[string]any{"actualFormat": contentType},
	}, err
}

func assetInspectionErrorCode(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "empty text"):
		return "EMPTY_TEXT"
	case strings.Contains(message, "sensitive field"):
		return "SENSITIVE_FIELD"
	case strings.Contains(message, "utf-8"):
		return "TEXT_DECODE_FAILED"
	case strings.Contains(message, "unsupported"), strings.Contains(message, "format"):
		return "FORMAT_MISSING"
	default:
		return "MEDIA_DECODE_FAILED"
	}
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
	inspection, inspectErr := inspectStoredAsset(c.Request.Context(), h, destination, session.Filename, session.DeclaredType)
	thumbnailKey := destination + ".thumbnail.jpg"
	if inspectErr == nil && inspection.MediaKind == "IMAGE" {
		inspectErr = generateThumbnail(c.Request.Context(), h, destination, thumbnailKey)
	}
	if inspectErr != nil {
		errorCode := assetInspectionErrorCode(inspectErr)
		metadata, _ := json.Marshal(inspection.Metadata)
		asset := Asset{
			TenantID: project.TenantID, ProjectID: project.ID, Filename: session.Filename,
			ObjectKey: destination, URI: h.storage.URI(destination), SHA256: inspection.Hash,
			ContentType: firstNonEmpty(inspection.ContentType, "application/octet-stream"),
			Size:        session.TotalSize, MediaKind: inspection.MediaKind, Status: AssetInvalid,
			Metadata: string(metadata), ErrorCode: errorCode,
			ErrorMessage: inspectErr.Error(), CreatedBy: c.GetUint64("user_id"),
		}
		_ = h.db.Create(&asset).Error
		h.failUploadWithAsset(session, asset.ID, errorCode, inspectErr.Error())
		httpx.Fail(c, 422, 422, "文件质量检查未通过："+inspectErr.Error())
		return
	}
	var duplicate Asset
	hasDuplicate := h.db.Where("tenant_id = ? AND project_id = ? AND sha256 = ? AND status = ?", project.TenantID, project.ID, inspection.Hash, AssetReady).First(&duplicate).Error == nil
	if hasDuplicate && session.DuplicatePolicy == "SKIP" {
		_ = h.storage.Delete(c.Request.Context(), destination)
		_ = h.storage.Delete(c.Request.Context(), thumbnailKey)
		h.completeUpload(session, duplicate.ID)
		h.cleanupChunks(c.Request.Context(), chunks)
		httpx.OK(c, gin.H{"asset": duplicate, "duplicate": true, "strategy": "SKIP"})
		return
	}
	var supplied map[string]any
	_ = json.Unmarshal([]byte(session.Metadata), &supplied)
	for key, value := range supplied {
		inspection.Metadata[key] = value
	}
	if inspection.Language == "" {
		inspection.Language = assetMetadataString(supplied, "language")
	}
	if inspection.SourceDevice == "" {
		inspection.SourceDevice = assetMetadataString(supplied, "sourceDevice")
	}
	businessScene := assetMetadataString(supplied, "businessScene")
	inspection.Metadata["declaredContentType"] = session.DeclaredType
	metadata, _ := json.Marshal(inspection.Metadata)
	asset := Asset{
		TenantID: project.TenantID, ProjectID: project.ID, Filename: session.Filename,
		ObjectKey: destination, URI: h.storage.URI(destination), SHA256: inspection.Hash, ContentType: inspection.ContentType,
		Size: inspection.Size, Width: inspection.Width, Height: inspection.Height, MediaKind: inspection.MediaKind,
		DurationSeconds: inspection.DurationSeconds, Codec: inspection.Codec, Language: inspection.Language,
		SourceDevice: inspection.SourceDevice, BusinessScene: businessScene, Status: AssetReady,
		Metadata: string(metadata), CreatedBy: c.GetUint64("user_id"),
	}
	if inspection.MediaKind == "IMAGE" {
		asset.ThumbnailObjectKey, asset.ThumbnailURI = thumbnailKey, h.storage.URI(thumbnailKey)
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

func assetMetadataString(values map[string]any, key string) string {
	value, exists := values[key]
	if !exists || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
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
	if mediaKind := strings.ToUpper(strings.TrimSpace(c.Query("mediaKind"))); mediaKind != "" {
		query = query.Where("media_kind = ?", mediaKind)
	}
	if errorCode := strings.ToUpper(strings.TrimSpace(c.Query("errorCode"))); errorCode != "" {
		query = query.Where("error_code = ?", errorCode)
	}
	if sourceDevice := strings.TrimSpace(c.Query("sourceDevice")); sourceDevice != "" {
		query = query.Where("source_device LIKE ?", "%"+sourceDevice+"%")
	}
	if businessScene := strings.TrimSpace(c.Query("businessScene")); businessScene != "" {
		query = query.Where("business_scene LIKE ?", "%"+businessScene+"%")
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
		ThumbnailURL    string         `json:"thumbnailUrl"`
		Tags            []assetTagView `json:"tags"`
		PurgeEligibleAt *time.Time     `json:"purgeEligibleAt,omitempty"`
	}
	retentionDays := assetRecycleRetentionDays(h.db, project)
	assetIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		assetIDs = append(assetIDs, row.ID)
	}
	tagsByAsset := assetTagViews(h.db, project, assetIDs)
	result := make([]assetPageView, 0, len(rows))
	for _, row := range rows {
		view := assetPageView{Asset: row, Tags: tagsByAsset[row.ID]}
		if row.Status == AssetDeleted && row.DeletedAt != nil {
			eligibleAt := row.DeletedAt.Add(time.Duration(retentionDays) * 24 * time.Hour)
			view.PurgeEligibleAt = &eligibleAt
		}
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
	type referenceView struct {
		ResourceType string `json:"resourceType"`
		ResourceID   uint64 `json:"resourceId"`
		Name         string `json:"name"`
		Status       string `json:"status"`
		Frozen       bool   `json:"frozen"`
	}
	references := make([]referenceView, 0)
	var storedReferences []AssetReference
	h.db.Where("tenant_id = ? AND project_id = ? AND asset_id = ?", project.TenantID, project.ID, asset.ID).
		Order("resource_type,resource_id").Find(&storedReferences)
	for _, reference := range storedReferences {
		name, status := "", ""
		switch reference.ResourceType {
		case "DATASET_VERSION":
			var version DatasetVersion
			if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, reference.ResourceID).
				First(&version).Error == nil {
				name, status = version.SemanticVersion, string(version.Status)
			}
		case "ASSET_COLLECTION":
			var collection AssetCollection
			if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, reference.ResourceID).
				First(&collection).Error == nil {
				name = collection.Name
				if collection.Frozen {
					status = "FROZEN"
				}
			}
		}
		references = append(references, referenceView{
			ResourceType: reference.ResourceType, ResourceID: reference.ResourceID,
			Name: name, Status: status, Frozen: reference.Frozen,
		})
	}
	var annotationReferences []struct {
		ID     uint64
		Name   string
		Status string
	}
	h.db.Table("ai_annotation_task AS task").
		Select("DISTINCT task.id, task.name, task.status").
		Joins("JOIN ai_asset_collection_item AS item ON item.collection_id = task.collection_id AND item.tenant_id = task.tenant_id").
		Where("task.tenant_id = ? AND task.project_id = ? AND item.asset_id = ?", project.TenantID, project.ID, asset.ID).
		Order("task.id").Scan(&annotationReferences)
	for _, reference := range annotationReferences {
		references = append(references, referenceView{
			ResourceType: "ANNOTATION_TASK", ResourceID: reference.ID,
			Name: reference.Name, Status: reference.Status,
		})
	}
	var feedbackReferences []struct {
		ID     uint64
		Name   string
		Status string
	}
	h.db.Table("ai_feedback_batch AS batch").
		Select("DISTINCT batch.id, batch.name, batch.status").
		Joins("JOIN ai_feedback_sample AS sample ON sample.batch_id = batch.id AND sample.tenant_id = batch.tenant_id").
		Where("batch.tenant_id = ? AND batch.project_id = ? AND sample.asset_id = ?", project.TenantID, project.ID, asset.ID).
		Order("batch.id").Scan(&feedbackReferences)
	for _, reference := range feedbackReferences {
		references = append(references, referenceView{
			ResourceType: "FEEDBACK_BATCH", ResourceID: reference.ID,
			Name: reference.Name, Status: reference.Status,
		})
	}
	httpx.OK(c, gin.H{
		"asset": asset, "metadata": metadata, "tags": assetTagViews(h.db, project, []uint64{asset.ID})[asset.ID],
		"references": references, "previewUrl": previewURL, "thumbnailUrl": thumbnailURL, "previewExpiresIn": 600,
		"recycleRetentionDays": assetRecycleRetentionDays(h.db, project),
		"purgeEligibleAt":      assetPurgeEligibleAt(asset, assetRecycleRetentionDays(h.db, project)),
	})
}

func assetRecycleRetentionDays(db *gorm.DB, project Project) int {
	const defaultDays = 30
	var configRow ProjectConfig
	if db.Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).First(&configRow).Error != nil {
		return defaultDays
	}
	var storageConfig map[string]any
	if json.Unmarshal([]byte(configRow.StorageConfig), &storageConfig) != nil {
		return defaultDays
	}
	value, ok := storageConfig["recycleRetentionDays"]
	if !ok {
		return defaultDays
	}
	days, err := strconv.Atoi(fmt.Sprint(value))
	if err != nil || days < 1 || days > 3650 {
		return defaultDays
	}
	return days
}

func assetPurgeEligibleAt(asset Asset, retentionDays int) *time.Time {
	if asset.Status != AssetDeleted || asset.DeletedAt == nil {
		return nil
	}
	eligibleAt := asset.DeletedAt.Add(time.Duration(retentionDays) * 24 * time.Hour)
	return &eligibleAt
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
	if asset.Status == AssetDeleted || asset.Status == AssetPurged {
		httpx.Fail(c, 409, 409, "资产已在回收站或已物理清理")
		return
	}
	now := time.Now()
	before := asset
	asset.RecycleFromStatus = asset.Status
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
	asset.Status = restoredAssetStatus(asset)
	asset.RecycleFromStatus, asset.DeletedAt, asset.DeletedBy = "", nil, 0
	if err := h.db.Save(&asset).Error; err != nil {
		httpx.Fail(c, 500, 500, "资产恢复失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "ASSET_RESTORED", "ASSET", asset.ID, nil, asset)
	httpx.OK(c, asset)
}

func restoredAssetStatus(asset Asset) AssetStatus {
	switch asset.RecycleFromStatus {
	case AssetReady, AssetInvalid, AssetMissing:
		return asset.RecycleFromStatus
	}
	if asset.ErrorCode != "" {
		return AssetInvalid
	}
	return AssetReady
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
	retentionDays := assetRecycleRetentionDays(h.db, project)
	eligibleAt := assetPurgeEligibleAt(asset, retentionDays)
	if eligibleAt == nil {
		httpx.Fail(c, 409, 409, "回收站资产缺少删除时间，不能自动物理清理")
		return
	}
	if time.Now().Before(*eligibleAt) {
		httpx.Fail(c, 409, 409, fmt.Sprintf("资产仍在 %d 天回收站保留期内，最早可于 %s 清理", retentionDays, eligibleAt.Format(time.RFC3339)))
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
	counts := make([]statusCount, 0)
	h.db.Model(&Asset{}).Select("status, count(*) as count").Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).Group("status").Scan(&counts)
	var duplicateGroups int64
	h.db.Raw("SELECT COUNT(*) FROM (SELECT sha256 FROM ai_asset WHERE tenant_id = ? AND project_id = ? AND status = ? GROUP BY sha256 HAVING COUNT(*) > 1) d", project.TenantID, project.ID, AssetReady).Scan(&duplicateGroups)
	var nearDuplicateGroups int64
	h.db.Raw("SELECT COUNT(DISTINCT near_duplicate_of_id) FROM ai_asset WHERE tenant_id = ? AND project_id = ? AND status = ? AND near_duplicate_of_id > 0", project.TenantID, project.ID, AssetReady).Scan(&nearDuplicateGroups)
	httpx.OK(c, gin.H{"byStatus": counts, "duplicateGroups": duplicateGroups, "nearDuplicateGroups": nearDuplicateGroups})
}

// AssetSimilarityAnalyze godoc
// @Summary Analyze project image similarity through FiftyOne
// @Tags VisionAI Asset
// @Security BearerAuth
// @Param distanceThreshold query int false "64-bit perceptual hash Hamming distance (0..32)"
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/assets/similarity [post]
func (h *Handler) AssetSimilarityAnalyze(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") || !h.storageReady(c) {
		return
	}
	threshold, err := strconv.Atoi(c.DefaultQuery("distanceThreshold", "6"))
	if err != nil || threshold < 0 || threshold > 32 {
		httpx.Fail(c, 400, 400, "近似重复距离必须在 0 到 32 之间")
		return
	}
	var assets []Asset
	if h.db.Where(
		"tenant_id = ? AND project_id = ? AND status = ? AND media_kind = ?",
		project.TenantID, project.ID, AssetReady, "IMAGE",
	).Order("id").Limit(5000).Find(&assets).Error != nil {
		httpx.Fail(c, 500, 500, "资产列表读取失败")
		return
	}
	if len(assets) == 0 {
		httpx.Fail(c, 409, 409, "项目中没有可分析的图像")
		return
	}
	samples := make([]evaluationplatform.SimilaritySample, 0, len(assets))
	for _, asset := range assets {
		signed, signErr := h.storage.PresignedGet(c.Request.Context(), asset.ObjectKey, 30*time.Minute)
		if signErr != nil {
			httpx.Fail(c, 503, 503, "图像读取地址生成失败")
			return
		}
		samples = append(samples, evaluationplatform.SimilaritySample{
			AssetID: asset.ID, Filename: asset.Filename, SourceURL: signed.String(),
		})
	}
	cfg := config.Load()
	result, analyzeErr := evaluationplatform.NewClient(cfg.FiftyOneAPIURL, cfg.FiftyOneTimeout).
		AnalyzeSimilarity(c.Request.Context(), evaluationplatform.SimilarityRequest{
			Name:      "visionai-similarity-project-" + strconv.FormatUint(project.ID, 10),
			ProjectID: project.ID, DistanceThreshold: threshold, Samples: samples,
		})
	if analyzeErr != nil {
		httpx.Fail(c, 502, 502, "FiftyOne 近似重复分析失败："+analyzeErr.Error())
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Asset{}).Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID).
			Updates(map[string]any{"perceptual_hash": "", "near_duplicate_of_id": 0}).Error; err != nil {
			return err
		}
		for rawID, hash := range result.Hashes {
			assetID, parseErr := strconv.ParseUint(rawID, 10, 64)
			if parseErr != nil {
				continue
			}
			if err := tx.Model(&Asset{}).Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, assetID).
				Update("perceptual_hash", hash).Error; err != nil {
				return err
			}
		}
		for _, group := range result.Groups {
			for _, assetID := range group.AssetIDs {
				if err := tx.Model(&Asset{}).Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, assetID).
					Update("near_duplicate_of_id", group.CanonicalAssetID).Error; err != nil {
					return err
				}
			}
		}
		return appendAudit(tx, c, project.ID, "ASSET_SIMILARITY_ANALYZED", "PROJECT", project.ID, nil, gin.H{
			"provider": "FIFTYONE", "analyzedCount": result.AnalyzedCount,
			"groupCount": len(result.Groups), "distanceThreshold": threshold,
		})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "近似重复分析结果保存失败")
		return
	}
	httpx.OK(c, result)
}
