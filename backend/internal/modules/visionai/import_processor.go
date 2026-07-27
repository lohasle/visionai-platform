package visionai

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/storage"
	"gorm.io/gorm"
)

type importSourceConfig struct {
	Source          string         `json:"source"`
	DuplicatePolicy string         `json:"duplicatePolicy"`
	Options         map[string]any `json:"options"`
}

type importFailure struct {
	Source      string `json:"source"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
}

// ProcessDomainEvent dispatches durable events to idempotent domain workers.
func ProcessDomainEvent(ctx context.Context, db *gorm.DB, cfg config.Config, event DomainEventEnvelope) error {
	switch event.EventType {
	case "annotation.task.prepare.v1", "annotation.export.requested.v1":
		return processAnnotationEvent(ctx, db, cfg, event)
	case "dataset.validate.requested.v1", "dataset.freeze.requested.v1":
		return processDatasetEvent(ctx, db, cfg, event)
	case "training.run.requested.v1", "training.template.smoke.requested.v1":
		return processTrainingEvent(ctx, db, cfg, event)
	case "evaluation.run.requested.v1":
		return processEvaluationEvent(ctx, db, cfg, event)
	case "deployment.revision.requested.v1":
		return processDeploymentEvent(ctx, db, cfg, event)
	case "asset.import.requested.v1":
		// Continue below.
	default:
		return nil
	}
	var payload struct {
		ImportRunID uint64 `json:"importRunId"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.ImportRunID == 0 {
		return errors.New("invalid asset import event payload")
	}
	provider, err := storage.NewMinIO(cfg)
	if err != nil {
		return err
	}
	if err = provider.EnsureBucket(ctx); err != nil {
		return err
	}
	handler := &Handler{db: db, storage: provider}
	if err = handler.processAssetImport(ctx, cfg, event.TenantID, payload.ImportRunID); err != nil {
		now := time.Now()
		var run AssetImportRun
		if db.Where("tenant_id = ? AND id = ?", event.TenantID, payload.ImportRunID).First(&run).Error == nil {
			db.Model(&run).Updates(map[string]any{"status": "FAILED", "finished_at": now, "report": marshalImportReport([]importFailure{{
				Source: run.SourceType, Code: "IMPORT_FATAL", Message: err.Error(), Remediation: "检查导入源、对象存储与文件格式后重试",
			}})})
			db.Model(&PlatformJob{}).Where("tenant_id = ? AND id = ?", event.TenantID, run.JobID).Updates(map[string]any{
				"status": JobFailed, "stage": "FAILED", "error_code": "IMPORT_FATAL",
				"error_message": err.Error(), "remediation": "检查导入源、对象存储与文件格式后重试", "finished_at": now,
			})
		}
	}
	return nil
}

func marshalImportReport(report []importFailure) string {
	value, _ := json.Marshal(report)
	return string(value)
}

func (h *Handler) processAssetImport(ctx context.Context, cfg config.Config, tenantID, runID uint64) error {
	var run AssetImportRun
	if err := h.db.Where("tenant_id = ? AND id = ?", tenantID, runID).First(&run).Error; err != nil {
		return err
	}
	if run.Status == "SUCCEEDED" || run.Status == "COMPLETED_WITH_ERRORS" {
		return nil
	}
	var source importSourceConfig
	if err := json.Unmarshal([]byte(run.SourceConfig), &source); err != nil {
		return fmt.Errorf("decode import source: %w", err)
	}
	now := time.Now()
	h.db.Model(&run).Updates(map[string]any{"status": "RUNNING", "started_at": now})
	h.db.Model(&PlatformJob{}).Where("tenant_id = ? AND id = ?", tenantID, run.JobID).Updates(map[string]any{
		"status": JobRunning, "stage": "DISCOVERING", "started_at": now,
	})
	failures := make([]importFailure, 0)
	var total, succeeded int64
	record := func(name string, err error) {
		total++
		if err == nil {
			succeeded++
		} else {
			failures = append(failures, importFailure{Source: name, Code: "ASSET_IMPORT_FAILED", Message: err.Error(), Remediation: "修复或移除该文件后重新导入"})
		}
		h.db.Model(&PlatformJob{}).Where("id = ?", run.JobID).Updates(map[string]any{
			"stage": "IMPORTING", "processed": total, "total": total, "progress": importProgress(total, succeeded),
		})
	}
	switch run.SourceType {
	case "DIRECTORY":
		err := h.importDirectory(ctx, cfg, run, source, record)
		if err != nil {
			return err
		}
	case "S3_PREFIX":
		objects, err := h.storage.List(ctx, source.Source)
		if err != nil {
			return err
		}
		for _, object := range objects {
			record(object.Key, h.importObject(ctx, run, source.DuplicatePolicy, object.Key, path.Base(object.Key)))
		}
	case "JSONL":
		if err := h.importJSONL(ctx, run, source, record); err != nil {
			return err
		}
	case "ZIP":
		if err := h.importZIP(ctx, run, source, record); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported import source type %s", run.SourceType)
	}
	finishedAt := time.Now()
	status := "SUCCEEDED"
	if len(failures) > 0 {
		status = "COMPLETED_WITH_ERRORS"
	}
	h.db.Model(&run).Updates(map[string]any{
		"status": status, "total": total, "succeeded": succeeded, "failed": total - succeeded,
		"report": marshalImportReport(failures), "finished_at": finishedAt,
	})
	h.db.Model(&PlatformJob{}).Where("tenant_id = ? AND id = ?", tenantID, run.JobID).Updates(map[string]any{
		"status": JobSucceeded, "stage": status, "progress": 100, "processed": total, "total": total, "finished_at": finishedAt,
	})
	return nil
}

func importProgress(total, succeeded int64) int {
	if total == 0 {
		return 0
	}
	progress := int(succeeded * 100 / total)
	if progress > 99 {
		return 99
	}
	return progress
}

func (h *Handler) importDirectory(ctx context.Context, cfg config.Config, run AssetImportRun, source importSourceConfig, record func(string, error)) error {
	root, err := filepath.Abs(cfg.ImportRoot)
	if err != nil {
		return err
	}
	sourcePath, err := filepath.Abs(filepath.Join(root, source.Source))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, sourcePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("directory import escaped configured import root")
	}
	return filepath.WalkDir(sourcePath, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			record(filePath, walkErr)
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			record(filePath, errors.New("non-regular files and symlinks are not allowed"))
			return nil
		}
		file, openErr := os.Open(filePath)
		if openErr != nil {
			record(filePath, openErr)
			return nil
		}
		info, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			record(filePath, statErr)
			return nil
		}
		filename, ok := safeFilename(entry.Name())
		if !ok {
			_ = file.Close()
			record(filePath, errors.New("unsafe filename"))
			return nil
		}
		destination := h.importDestination(run, filename)
		putErr := h.storage.Put(ctx, destination, file, info.Size(), "application/octet-stream")
		_ = file.Close()
		if putErr == nil {
			putErr = h.registerImportedObject(ctx, run, source.DuplicatePolicy, destination, filename)
		}
		record(filePath, putErr)
		return nil
	})
}

func (h *Handler) importJSONL(ctx context.Context, run AssetImportRun, source importSourceConfig, record func(string, error)) error {
	reader, _, err := h.storage.Get(ctx, source.Source)
	if err != nil {
		return err
	}
	defer reader.Close()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var row struct {
			Key      string `json:"key"`
			Filename string `json:"filename"`
		}
		if err = json.Unmarshal(scanner.Bytes(), &row); err != nil {
			record("JSONL line", err)
			continue
		}
		if !strings.HasPrefix(row.Key, assetRoot(run.TenantID, run.ProjectID)+"/") || strings.Contains(row.Key, "..") {
			record(row.Key, errors.New("object key is outside the project scope"))
			continue
		}
		if row.Filename == "" {
			row.Filename = path.Base(row.Key)
		}
		record(row.Key, h.importObject(ctx, run, source.DuplicatePolicy, row.Key, row.Filename))
	}
	return scanner.Err()
}

func safeArchiveEntry(name string) (string, bool) {
	normalized := strings.ReplaceAll(name, `\`, "/")
	clean := path.Clean(normalized)
	if clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", false
	}
	return safeFilename(path.Base(clean))
}

func (h *Handler) importZIP(ctx context.Context, run AssetImportRun, source importSourceConfig, record func(string, error)) error {
	reader, info, err := h.storage.Get(ctx, source.Source)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp("", "visionai-import-*.zip")
	if err != nil {
		_ = reader.Close()
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	_, copyErr := io.Copy(temp, io.LimitReader(reader, 20<<30))
	_ = reader.Close()
	_ = temp.Close()
	if copyErr != nil {
		return copyErr
	}
	archive, err := zip.OpenReader(tempPath)
	if err != nil {
		return err
	}
	defer archive.Close()
	var expanded uint64
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		filename, valid := safeArchiveEntry(entry.Name)
		expanded += entry.UncompressedSize64
		if !valid || entry.UncompressedSize64 > 2<<30 || expanded > uint64(maxUploadSize) ||
			(entry.CompressedSize64 > 0 && entry.UncompressedSize64/entry.CompressedSize64 > 1000) {
			record(entry.Name, errors.New("unsafe ZIP entry or expansion limit exceeded"))
			continue
		}
		file, openErr := entry.Open()
		if openErr != nil {
			record(entry.Name, openErr)
			continue
		}
		destination := h.importDestination(run, filename)
		putErr := h.storage.Put(ctx, destination, file, int64(entry.UncompressedSize64), "application/octet-stream")
		_ = file.Close()
		if putErr == nil {
			putErr = h.registerImportedObject(ctx, run, source.DuplicatePolicy, destination, filename)
		}
		record(entry.Name, putErr)
	}
	_ = info
	return nil
}

func (h *Handler) importObject(ctx context.Context, run AssetImportRun, policy, sourceKey, filename string) error {
	filename, ok := safeFilename(filename)
	if !ok {
		return errors.New("unsafe object filename")
	}
	destination := h.importDestination(run, filename)
	if err := h.storage.Compose(ctx, destination, []string{sourceKey}); err != nil {
		return err
	}
	return h.registerImportedObject(ctx, run, policy, destination, filename)
}

func (h *Handler) importDestination(run AssetImportRun, filename string) string {
	return fmt.Sprintf("%s/assets/%s/%s", assetRoot(run.TenantID, run.ProjectID), uuid.NewString(), filename)
}

func (h *Handler) registerImportedObject(ctx context.Context, run AssetImportRun, policy, destination, filename string) error {
	hash, contentType, width, height, size, err := inspectImage(ctx, h, destination)
	thumbnailKey := destination + ".thumbnail.jpg"
	if err == nil {
		err = generateThumbnail(ctx, h, destination, thumbnailKey)
	}
	if err != nil {
		invalid := Asset{
			TenantID: run.TenantID, ProjectID: run.ProjectID, Filename: filename,
			ObjectKey: destination, URI: h.storage.URI(destination), Size: size, Status: AssetInvalid,
			ContentType: "application/octet-stream", Metadata: "{}", ErrorCode: "IMAGE_DECODE_FAILED",
			ErrorMessage: err.Error(), CreatedBy: run.CreatedBy,
		}
		_ = h.db.Create(&invalid).Error
		return err
	}
	var duplicate Asset
	hasDuplicate := h.db.Where("tenant_id = ? AND project_id = ? AND sha256 = ? AND status = ?", run.TenantID, run.ProjectID, hash, AssetReady).First(&duplicate).Error == nil
	if hasDuplicate && policy == "SKIP" {
		_ = h.storage.Delete(ctx, destination)
		_ = h.storage.Delete(ctx, thumbnailKey)
		return nil
	}
	metadata, _ := json.Marshal(map[string]any{"importRunId": run.ID, "actualContentType": contentType})
	asset := Asset{
		TenantID: run.TenantID, ProjectID: run.ProjectID, Filename: filename,
		ObjectKey: destination, URI: h.storage.URI(destination), SHA256: hash, ContentType: contentType,
		ThumbnailObjectKey: thumbnailKey, ThumbnailURI: h.storage.URI(thumbnailKey),
		Size: size, Width: width, Height: height, Status: AssetReady, Metadata: string(metadata), CreatedBy: run.CreatedBy,
	}
	if hasDuplicate && policy == "REFERENCE" {
		_ = h.storage.Delete(ctx, destination)
		_ = h.storage.Delete(ctx, thumbnailKey)
		asset.ObjectKey, asset.URI, asset.DuplicateOfID = duplicate.ObjectKey, duplicate.URI, duplicate.ID
		asset.ThumbnailObjectKey, asset.ThumbnailURI = duplicate.ThumbnailObjectKey, duplicate.ThumbnailURI
	}
	return h.db.Create(&asset).Error
}
