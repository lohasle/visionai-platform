package visionai

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

type AssetCollection struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID   uint64    `gorm:"index;not null" json:"projectId"`
	Name        string    `gorm:"size:128;not null" json:"name"`
	Description string    `gorm:"size:1024" json:"description"`
	Filter      string    `gorm:"type:json;not null" json:"-"`
	Frozen      bool      `gorm:"index;not null;default:false" json:"frozen"`
	Version     int       `gorm:"not null;default:1" json:"version"`
	CreatedBy   uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

type AssetCollectionItem struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	TenantID     uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID    uint64    `gorm:"index;not null" json:"projectId"`
	CollectionID uint64    `gorm:"uniqueIndex:uk_collection_asset;not null" json:"collectionId"`
	AssetID      uint64    `gorm:"uniqueIndex:uk_collection_asset;not null" json:"assetId"`
	CreatedAt    time.Time `json:"createTime"`
}

type AssetTag struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	TenantID     uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID    uint64    `gorm:"index;not null" json:"projectId"`
	AssetID      uint64    `gorm:"uniqueIndex:uk_asset_tag;not null" json:"assetId"`
	DefinitionID uint64    `gorm:"index" json:"definitionId"`
	Category     string    `gorm:"size:16;index" json:"category"`
	Tag          string    `gorm:"size:64;uniqueIndex:uk_asset_tag;not null" json:"tag"`
	CreatedAt    time.Time `json:"createTime"`
}

func (AssetCollection) TableName() string     { return "ai_asset_collection" }
func (AssetCollectionItem) TableName() string { return "ai_asset_collection_item" }
func (AssetTag) TableName() string            { return "ai_asset_tag" }

type collectionRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Filter      map[string]any `json:"filter"`
}

type collectionAssetsRequest struct {
	AssetIDs []uint64 `json:"assetIds"`
}

type assetTagsRequest struct {
	DefinitionIDs []uint64 `json:"definitionIds"`
}

func (h *Handler) getCollection(c *gin.Context, project Project) (AssetCollection, bool) {
	id, _ := strconv.ParseUint(c.Param("collectionId"), 10, 64)
	var collection AssetCollection
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&collection).Error != nil {
		httpx.Fail(c, 404, 404, "资产集合不存在")
		return collection, false
	}
	return collection, true
}

// CollectionPage godoc
// @Summary Page asset collections
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/collections [get]
func (h *Handler) CollectionPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	type collectionView struct {
		AssetCollection
		AssetCount int64 `json:"assetCount"`
	}
	var rows []collectionView
	h.db.Table("ai_asset_collection c").
		Select("c.*, COUNT(i.id) AS asset_count").
		Joins("LEFT JOIN ai_asset_collection_item i ON i.collection_id = c.id AND i.tenant_id = c.tenant_id").
		Where("c.tenant_id = ? AND c.project_id = ?", project.TenantID, project.ID).
		Group("c.id").Order("c.id DESC").Scan(&rows)
	httpx.OK(c, rows)
}

// CollectionCreate godoc
// @Summary Create an asset collection
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/collections [post]
func (h *Handler) CollectionCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	var req collectionRequest
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Name) == "" {
		httpx.Fail(c, 400, 400, "集合名称必填")
		return
	}
	filter, _ := json.Marshal(req.Filter)
	row := AssetCollection{
		TenantID: project.TenantID, ProjectID: project.ID, Name: strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description), Filter: string(filter), Version: 1,
		CreatedBy: c.GetUint64("user_id"),
	}
	filteredAssetIDs := make([]uint64, 0)
	if len(req.Filter) > 0 {
		query := h.db.Model(&Asset{}).Where("tenant_id = ? AND project_id = ?", project.TenantID, project.ID)
		status, _ := req.Filter["status"].(string)
		if strings.TrimSpace(status) == "" {
			status = string(AssetReady)
		}
		query = query.Where("status = ?", status)
		if keyword, _ := req.Filter["keyword"].(string); strings.TrimSpace(keyword) != "" {
			query = query.Where("filename LIKE ? OR sha256 LIKE ?", "%"+strings.TrimSpace(keyword)+"%", strings.TrimSpace(keyword)+"%")
		}
		tagIDs := numericIDs(req.Filter["tagDefinitionIds"])
		if len(tagIDs) > 0 {
			query = query.Where(`id IN (
				SELECT asset_id FROM ai_asset_tag
				WHERE tenant_id = ? AND project_id = ? AND definition_id IN ?
				GROUP BY asset_id HAVING COUNT(DISTINCT definition_id) = ?
			)`, project.TenantID, project.ID, tagIDs, len(tagIDs))
		}
		if err := query.Order("id").Pluck("id", &filteredAssetIDs).Error; err != nil {
			httpx.Fail(c, 500, 500, "筛选资产失败")
			return
		}
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if len(filteredAssetIDs) > 0 {
			items := make([]AssetCollectionItem, 0, len(filteredAssetIDs))
			for _, assetID := range filteredAssetIDs {
				items = append(items, AssetCollectionItem{
					TenantID: project.TenantID, ProjectID: project.ID, CollectionID: row.ID, AssetID: assetID,
				})
			}
			if err := tx.CreateInBatches(items, 500).Error; err != nil {
				return err
			}
		}
		return appendAudit(tx, c, project.ID, "ASSET_COLLECTION_CREATED", "ASSET_COLLECTION", row.ID, nil, gin.H{
			"collection": row, "filteredAssetCount": len(filteredAssetIDs),
		})
	}); err != nil {
		httpx.Fail(c, 500, 500, "资产集合创建失败")
		return
	}
	httpx.OK(c, row)
}

func numericIDs(value any) []uint64 {
	result, seen := make([]uint64, 0), map[uint64]bool{}
	appendID := func(id uint64) {
		if id > 0 && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	switch values := value.(type) {
	case []any:
		for _, item := range values {
			switch typed := item.(type) {
			case float64:
				appendID(uint64(typed))
			case string:
				id, _ := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
				appendID(id)
			}
		}
	case []uint64:
		for _, id := range values {
			appendID(id)
		}
	case string:
		for _, item := range strings.Split(values, ",") {
			id, _ := strconv.ParseUint(strings.TrimSpace(item), 10, 64)
			appendID(id)
		}
	}
	return result
}

// CollectionAddAssets godoc
// @Summary Add assets to a mutable collection
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/collections/{collectionId}/assets [put]
func (h *Handler) CollectionAddAssets(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	collection, ok := h.getCollection(c, project)
	if !ok {
		return
	}
	if collection.Frozen {
		httpx.Fail(c, http.StatusConflict, 409, "冻结集合不可修改")
		return
	}
	var req collectionAssetsRequest
	if c.ShouldBindJSON(&req) != nil || len(req.AssetIDs) == 0 || len(req.AssetIDs) > 1000 {
		httpx.Fail(c, 400, 400, "assetIds 必填且单次不超过 1000")
		return
	}
	var count int64
	h.db.Model(&Asset{}).Where("tenant_id = ? AND project_id = ? AND id IN ? AND status = ?", project.TenantID, project.ID, req.AssetIDs, AssetReady).Count(&count)
	if count != int64(len(req.AssetIDs)) {
		httpx.Fail(c, 400, 400, "包含不存在、跨项目或不可用资产")
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, assetID := range req.AssetIDs {
			row := AssetCollectionItem{TenantID: project.TenantID, ProjectID: project.ID, CollectionID: collection.ID, AssetID: assetID}
			if err := tx.Where("collection_id = ? AND asset_id = ?", collection.ID, assetID).FirstOrCreate(&row).Error; err != nil {
				return err
			}
		}
		return appendAudit(tx, c, project.ID, "ASSET_COLLECTION_ITEMS_ADDED", "ASSET_COLLECTION", collection.ID, nil, gin.H{"assetIds": req.AssetIDs})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "集合资产保存失败")
		return
	}
	httpx.OK(c, true)
}

// CollectionFreeze godoc
// @Summary Freeze a collection and protect its asset references
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/collections/{collectionId}/freeze [post]
func (h *Handler) CollectionFreeze(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	collection, ok := h.getCollection(c, project)
	if !ok {
		return
	}
	if collection.Frozen {
		httpx.OK(c, collection)
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var items []AssetCollectionItem
		if err := tx.Where("tenant_id = ? AND collection_id = ?", project.TenantID, collection.ID).Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			ref := AssetReference{
				TenantID: project.TenantID, ProjectID: project.ID, AssetID: item.AssetID,
				ResourceType: "ASSET_COLLECTION", ResourceID: collection.ID, Frozen: true,
			}
			if err := tx.Where("asset_id = ? AND resource_type = ? AND resource_id = ?", item.AssetID, ref.ResourceType, ref.ResourceID).FirstOrCreate(&ref).Error; err != nil {
				return err
			}
			if err := tx.Model(&Asset{}).Where("id = ?", item.AssetID).UpdateColumn("reference_count", gorm.Expr("reference_count + 1")).Error; err != nil {
				return err
			}
		}
		collection.Frozen = true
		if err := tx.Save(&collection).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "ASSET_COLLECTION_FROZEN", "ASSET_COLLECTION", collection.ID, nil, collection)
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "资产集合冻结失败")
		return
	}
	httpx.OK(c, collection)
}

// CollectionAssetsPage godoc
// @Summary Page assets inside a collection
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/collections/{collectionId}/assets [get]
func (h *Handler) CollectionAssetsPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	collection, ok := h.getCollection(c, project)
	if !ok {
		return
	}
	query := h.db.Model(&Asset{}).
		Joins("JOIN ai_asset_collection_item i ON i.asset_id = ai_asset.id AND i.collection_id = ?", collection.ID).
		Where("ai_asset.tenant_id = ? AND ai_asset.project_id = ?", project.TenantID, project.ID)
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		query = query.Where("ai_asset.filename LIKE ? OR ai_asset.sha256 LIKE ?", "%"+keyword+"%", keyword+"%")
	}
	var total int64
	query.Count(&total)
	pageNo, pageSize := page(c)
	var rows []Asset
	query.Order("ai_asset.id DESC").Offset((pageNo - 1) * pageSize).Limit(pageSize).Find(&rows)
	type assetView struct {
		Asset
		ThumbnailURL string `json:"thumbnailUrl"`
	}
	result := make([]assetView, 0, len(rows))
	for _, row := range rows {
		view := assetView{Asset: row}
		if h.storage != nil && row.ThumbnailObjectKey != "" {
			if signed, err := h.storage.PresignedGet(c.Request.Context(), row.ThumbnailObjectKey, 10*time.Minute); err == nil {
				view.ThumbnailURL = signed.String()
			}
		}
		result = append(result, view)
	}
	httpx.OK(c, gin.H{"collection": collection, "list": result, "total": total})
}

// AssetTagsUpdate godoc
// @Summary Replace asset tags
// @Tags VisionAI Asset
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/assets/{assetId}/tags [put]
func (h *Handler) AssetTagsUpdate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	asset, ok := h.getAsset(c, project)
	if !ok {
		return
	}
	var req assetTagsRequest
	if c.ShouldBindJSON(&req) != nil || len(req.DefinitionIDs) > 50 {
		httpx.Fail(c, 400, 400, "标签定义格式错误或超过 50 个")
		return
	}
	definitions, err := loadTagDefinitions(h.db, project, req.DefinitionIDs, true)
	if err != nil {
		httpx.Fail(c, 409, 409, "标签定义不存在、跨项目或已停用")
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := replaceAssetTags(tx, project, []uint64{asset.ID}, definitions, "REPLACE"); err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "ASSET_TAGS_UPDATED", "ASSET", asset.ID, nil, gin.H{"definitionIds": req.DefinitionIDs})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "资产标签保存失败")
		return
	}
	httpx.OK(c, assetTagViews(h.db, project, []uint64{asset.ID})[asset.ID])
}
