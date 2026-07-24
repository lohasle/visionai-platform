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
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID uint64    `gorm:"index;not null" json:"projectId"`
	AssetID   uint64    `gorm:"uniqueIndex:uk_asset_tag;not null" json:"assetId"`
	Tag       string    `gorm:"size:64;uniqueIndex:uk_asset_tag;not null" json:"tag"`
	CreatedAt time.Time `json:"createTime"`
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
	Tags []string `json:"tags"`
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
	if err := h.db.Create(&row).Error; err != nil {
		httpx.Fail(c, 500, 500, "资产集合创建失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "ASSET_COLLECTION_CREATED", "ASSET_COLLECTION", row.ID, nil, row)
	httpx.OK(c, row)
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
	if c.ShouldBindJSON(&req) != nil || len(req.Tags) > 50 {
		httpx.Fail(c, 400, 400, "标签格式错误或超过 50 个")
		return
	}
	clean := make([]string, 0, len(req.Tags))
	seen := map[string]bool{}
	for _, tag := range req.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || len(tag) > 64 || seen[tag] {
			continue
		}
		seen[tag] = true
		clean = append(clean, tag)
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND asset_id = ?", project.TenantID, asset.ID).Delete(&AssetTag{}).Error; err != nil {
			return err
		}
		for _, tag := range clean {
			if err := tx.Create(&AssetTag{TenantID: project.TenantID, ProjectID: project.ID, AssetID: asset.ID, Tag: tag}).Error; err != nil {
				return err
			}
		}
		return appendAudit(tx, c, project.ID, "ASSET_TAGS_UPDATED", "ASSET", asset.ID, nil, gin.H{"tags": clean})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "资产标签保存失败")
		return
	}
	httpx.OK(c, clean)
}
