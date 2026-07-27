package visionai

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type assetTagDefinitionRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}

type assetTagBatchRequest struct {
	AssetIDs      []uint64 `json:"assetIds"`
	DefinitionIDs []uint64 `json:"definitionIds"`
	Mode          string   `json:"mode"`
}

type assetTagView struct {
	AssetTag
	Name  string `json:"name"`
	Color string `json:"color"`
}

func validTagCategory(category string) bool {
	return category == "BUSINESS" || category == "SCENE" || category == "SOURCE"
}

func normalizeTagDefinition(req *assetTagDefinitionRequest) bool {
	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.ToUpper(strings.TrimSpace(req.Category))
	req.Color = strings.ToUpper(strings.TrimSpace(req.Color))
	req.Description = strings.TrimSpace(req.Description)
	if req.Color == "" {
		req.Color = "#1677FF"
	}
	return ontologyCodePattern.MatchString(req.Code) && req.Name != "" && validTagCategory(req.Category)
}

// AssetTagDefinitionPage godoc
// @Summary List governed asset tag definitions
// @Tags VisionAI Asset Tag
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/tag-definitions [get]
func (h *Handler) AssetTagDefinitionPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	type definitionView struct {
		AssetTagDefinition
		UsageCount int64 `json:"usageCount"`
	}
	query := h.db.Table("ai_asset_tag_definition d").
		Select("d.*, COUNT(t.id) AS usage_count").
		Joins("LEFT JOIN ai_asset_tag t ON t.definition_id = d.id AND t.tenant_id = d.tenant_id").
		Where("d.tenant_id = ? AND d.project_id = ?", project.TenantID, project.ID)
	if category := strings.ToUpper(strings.TrimSpace(c.Query("category"))); category != "" {
		query = query.Where("d.category = ?", category)
	}
	if c.Query("enabled") != "" {
		query = query.Where("d.enabled = ?", c.Query("enabled") == "true" || c.Query("enabled") == "1")
	}
	rows := make([]definitionView, 0)
	query.Group("d.id").Order("d.category, d.name, d.id").Scan(&rows)
	httpx.OK(c, rows)
}

// AssetTagDefinitionCreate godoc
// @Summary Create a governed asset tag definition
// @Tags VisionAI Asset Tag
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/tag-definitions [post]
func (h *Handler) AssetTagDefinitionCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER", "DATA_MANAGER") {
		return
	}
	var req assetTagDefinitionRequest
	if c.ShouldBindJSON(&req) != nil || !normalizeTagDefinition(&req) {
		httpx.Fail(c, 400, 400, "标签编码、名称或分类无效")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row := AssetTagDefinition{
		TenantID: project.TenantID, ProjectID: project.ID, Code: req.Code, Name: req.Name,
		Category: req.Category, Color: req.Color, Description: req.Description,
		Enabled: enabled, CreatedBy: c.GetUint64("user_id"),
	}
	if err := h.db.Create(&row).Error; err != nil {
		httpx.Fail(c, 409, 409, "标签编码已存在")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "ASSET_TAG_DEFINITION_CREATED", "ASSET_TAG_DEFINITION", row.ID, nil, row)
	httpx.OK(c, row)
}

// AssetTagDefinitionUpdate godoc
// @Summary Update an asset tag definition
// @Tags VisionAI Asset Tag
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/tag-definitions/{definitionId} [put]
func (h *Handler) AssetTagDefinitionUpdate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER", "DATA_MANAGER") {
		return
	}
	id, _ := strconv.ParseUint(c.Param("definitionId"), 10, 64)
	var row AssetTagDefinition
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "标签定义不存在")
		return
	}
	var req assetTagDefinitionRequest
	if c.ShouldBindJSON(&req) != nil || !normalizeTagDefinition(&req) {
		httpx.Fail(c, 400, 400, "标签编码、名称或分类无效")
		return
	}
	before := row
	row.Code, row.Name, row.Category, row.Color = req.Code, req.Name, req.Category, req.Color
	row.Description = req.Description
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&AssetTag{}).Where("tenant_id = ? AND definition_id = ?", project.TenantID, row.ID).
			Updates(map[string]any{"tag": row.Code, "category": row.Category}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "ASSET_TAG_DEFINITION_UPDATED", "ASSET_TAG_DEFINITION", row.ID, before, row)
	}); err != nil {
		httpx.Fail(c, 409, 409, "标签定义保存失败或编码冲突")
		return
	}
	httpx.OK(c, row)
}

func loadTagDefinitions(db *gorm.DB, project Project, ids []uint64, requireEnabled bool) ([]AssetTagDefinition, error) {
	if len(ids) == 0 {
		return []AssetTagDefinition{}, nil
	}
	query := db.Where("tenant_id = ? AND project_id = ? AND id IN ?", project.TenantID, project.ID, ids)
	if requireEnabled {
		query = query.Where("enabled = ?", true)
	}
	var rows []AssetTagDefinition
	if err := query.Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, gorm.ErrRecordNotFound
	}
	return rows, nil
}

func replaceAssetTags(tx *gorm.DB, project Project, assetIDs []uint64, definitions []AssetTagDefinition, mode string) error {
	definitionIDs := make([]uint64, 0, len(definitions))
	for _, definition := range definitions {
		definitionIDs = append(definitionIDs, definition.ID)
	}
	switch mode {
	case "REPLACE":
		if err := tx.Where("tenant_id = ? AND project_id = ? AND asset_id IN ?", project.TenantID, project.ID, assetIDs).Delete(&AssetTag{}).Error; err != nil {
			return err
		}
	case "REMOVE":
		if len(definitionIDs) == 0 {
			return nil
		}
		return tx.Where("tenant_id = ? AND project_id = ? AND asset_id IN ? AND definition_id IN ?",
			project.TenantID, project.ID, assetIDs, definitionIDs).Delete(&AssetTag{}).Error
	}
	for _, assetID := range assetIDs {
		for _, definition := range definitions {
			row := AssetTag{
				TenantID: project.TenantID, ProjectID: project.ID, AssetID: assetID,
				DefinitionID: definition.ID, Category: definition.Category, Tag: definition.Code,
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// AssetTagsBatchUpdate godoc
// @Summary Add, replace, or remove governed tags on assets
// @Tags VisionAI Asset Tag
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/assets/tags [put]
func (h *Handler) AssetTagsBatchUpdate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "DATA_MANAGER") {
		return
	}
	var req assetTagBatchRequest
	if c.ShouldBindJSON(&req) != nil || len(req.AssetIDs) == 0 || len(req.AssetIDs) > 1000 || len(req.DefinitionIDs) > 50 {
		httpx.Fail(c, 400, 400, "资产或标签定义请求无效")
		return
	}
	req.Mode = strings.ToUpper(strings.TrimSpace(req.Mode))
	if req.Mode == "" {
		req.Mode = "REPLACE"
	}
	if req.Mode != "ADD" && req.Mode != "REPLACE" && req.Mode != "REMOVE" {
		httpx.Fail(c, 400, 400, "mode 必须是 ADD、REPLACE 或 REMOVE")
		return
	}
	var assetCount int64
	h.db.Model(&Asset{}).Where("tenant_id = ? AND project_id = ? AND id IN ? AND status <> ?",
		project.TenantID, project.ID, req.AssetIDs, AssetPurged).Count(&assetCount)
	if assetCount != int64(len(req.AssetIDs)) {
		httpx.Fail(c, 409, 409, "资产不存在、跨项目或已清理")
		return
	}
	definitions, err := loadTagDefinitions(h.db, project, req.DefinitionIDs, req.Mode != "REMOVE")
	if err != nil {
		httpx.Fail(c, 409, 409, "标签定义不存在、跨项目或已停用")
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := replaceAssetTags(tx, project, req.AssetIDs, definitions, req.Mode); err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "ASSET_TAGS_BATCH_UPDATED", "ASSET", 0, nil, gin.H{
			"assetIds": req.AssetIDs, "definitionIds": req.DefinitionIDs, "mode": req.Mode,
		})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "资产标签保存失败")
		return
	}
	httpx.OK(c, true)
}

func assetTagViews(db *gorm.DB, project Project, assetIDs []uint64) map[uint64][]assetTagView {
	result := make(map[uint64][]assetTagView, len(assetIDs))
	if len(assetIDs) == 0 {
		return result
	}
	var rows []struct {
		AssetTag
		Name  string
		Color string
	}
	db.Table("ai_asset_tag t").
		Select("t.*, COALESCE(d.name, t.tag) AS name, COALESCE(d.color, '#909399') AS color").
		Joins("LEFT JOIN ai_asset_tag_definition d ON d.id = t.definition_id AND d.tenant_id = t.tenant_id").
		Where("t.tenant_id = ? AND t.project_id = ? AND t.asset_id IN ?", project.TenantID, project.ID, assetIDs).
		Order("t.category, t.tag, t.id").Scan(&rows)
	for _, row := range rows {
		result[row.AssetID] = append(result[row.AssetID], assetTagView{AssetTag: row.AssetTag, Name: row.Name, Color: row.Color})
	}
	return result
}

func migrateLegacyAssetTags(db *gorm.DB) error {
	var groups []struct {
		TenantID  uint64
		ProjectID uint64
		Tag       string
	}
	if err := db.Model(&AssetTag{}).Select("tenant_id, project_id, tag").
		Where("COALESCE(definition_id, 0) = 0").Group("tenant_id, project_id, tag").Scan(&groups).Error; err != nil {
		return err
	}
	for _, group := range groups {
		sum := sha256.Sum256([]byte(group.Tag))
		code := "Legacy_" + hex.EncodeToString(sum[:6])
		definition := AssetTagDefinition{
			TenantID: group.TenantID, ProjectID: group.ProjectID, Code: code, Name: group.Tag,
			Category: "BUSINESS", Color: "#909399", Description: "由历史自由文本标签自动迁移。",
			Enabled: true, CreatedBy: 0,
		}
		if err := db.Where(
			"tenant_id = ? AND project_id = ? AND code = ?", group.TenantID, group.ProjectID, code,
		).FirstOrCreate(&definition).Error; err != nil {
			return err
		}
		if err := db.Model(&AssetTag{}).Where(
			"tenant_id = ? AND project_id = ? AND tag = ? AND COALESCE(definition_id, 0) = 0",
			group.TenantID, group.ProjectID, group.Tag,
		).Updates(map[string]any{
			"definition_id": definition.ID, "category": definition.Category, "tag": definition.Code,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
