package visionai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/annotation"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ontologyCodePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{1,63}$`)

type ontologyRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	TaskType    string `json:"taskType"`
	Description string `json:"description"`
}

type ontologyVersionRequest struct {
	SourceVersionID uint64 `json:"sourceVersionId"`
}

type ontologyAttributeInput struct {
	Name         string   `json:"name"`
	InputType    string   `json:"inputType"`
	Values       []string `json:"values"`
	DefaultValue string   `json:"defaultValue"`
	Mutable      bool     `json:"mutable"`
	Sort         int      `json:"sort"`
}

type ontologyLabelInput struct {
	Code       string                   `json:"code"`
	Name       string                   `json:"name"`
	Color      string                   `json:"color"`
	ShapeType  string                   `json:"shapeType"`
	Sort       int                      `json:"sort"`
	Attributes []ontologyAttributeInput `json:"attributes"`
}

type ontologyLabelsRequest struct {
	Labels []ontologyLabelInput `json:"labels"`
}

type ontologyLabelView struct {
	OntologyLabel
	Attributes []OntologyAttribute `json:"attributes"`
}

func (h *Handler) ontologyForProject(c *gin.Context, project Project) (Ontology, bool) {
	id, _ := strconv.ParseUint(c.Param("ontologyId"), 10, 64)
	var row Ontology
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "类别体系不存在")
		return row, false
	}
	return row, true
}

func (h *Handler) ontologyVersionForProject(c *gin.Context, project Project) (OntologyVersion, bool) {
	id, _ := strconv.ParseUint(c.Param("versionId"), 10, 64)
	var row OntologyVersion
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, id).First(&row).Error != nil {
		httpx.Fail(c, 404, 404, "类别体系版本不存在")
		return row, false
	}
	return row, true
}

func loadOntologyLabelViews(db *gorm.DB, version OntologyVersion) ([]ontologyLabelView, error) {
	var labels []OntologyLabel
	if err := db.Where("tenant_id = ? AND ontology_version_id = ?", version.TenantID, version.ID).
		Order("sort, id").Find(&labels).Error; err != nil {
		return nil, err
	}
	labelIDs := make([]uint64, 0, len(labels))
	for _, label := range labels {
		labelIDs = append(labelIDs, label.ID)
	}
	var attributes []OntologyAttribute
	if len(labelIDs) > 0 {
		if err := db.Where("tenant_id = ? AND ontology_label_id IN ?", version.TenantID, labelIDs).
			Order("sort, id").Find(&attributes).Error; err != nil {
			return nil, err
		}
	}
	byLabel := make(map[uint64][]OntologyAttribute, len(labels))
	for _, attribute := range attributes {
		byLabel[attribute.OntologyLabelID] = append(byLabel[attribute.OntologyLabelID], attribute)
	}
	result := make([]ontologyLabelView, 0, len(labels))
	for _, label := range labels {
		result = append(result, ontologyLabelView{OntologyLabel: label, Attributes: byLabel[label.ID]})
	}
	return result, nil
}

// OntologyPage godoc
// @Summary List project ontologies
// @Tags VisionAI Ontology
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/ontologies [get]
func (h *Handler) OntologyPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	type ontologyView struct {
		Ontology
		VersionCount       int64  `json:"versionCount"`
		PublishedCount     int64  `json:"publishedCount"`
		LatestVersionID    uint64 `json:"latestVersionId"`
		LatestVersion      string `json:"latestVersion"`
		LatestVersionState string `json:"latestVersionStatus"`
	}
	rows := make([]ontologyView, 0)
	h.db.Table("ai_ontology o").
		Select(`o.*, COUNT(v.id) AS version_count,
			SUM(CASE WHEN v.status = 'PUBLISHED' THEN 1 ELSE 0 END) AS published_count,
			COALESCE(MAX(v.id), 0) AS latest_version_id`).
		Joins("LEFT JOIN ai_ontology_version v ON v.ontology_id = o.id AND v.tenant_id = o.tenant_id").
		Where("o.tenant_id = ? AND o.project_id = ?", project.TenantID, project.ID).
		Group("o.id").Order("o.id DESC").Scan(&rows)
	for index := range rows {
		if rows[index].LatestVersionID == 0 {
			continue
		}
		var version OntologyVersion
		if h.db.Where("id = ?", rows[index].LatestVersionID).First(&version).Error == nil {
			rows[index].LatestVersion = version.SemanticVersion
			rows[index].LatestVersionState = string(version.Status)
		}
	}
	httpx.OK(c, rows)
}

// OntologyVersionPage godoc
// @Summary List selectable ontology versions
// @Tags VisionAI Ontology
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/ontology-versions [get]
func (h *Handler) OntologyVersionPage(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	type selectableVersion struct {
		OntologyVersion
		OntologyCode string `json:"ontologyCode"`
		OntologyName string `json:"ontologyName"`
		TaskType     string `json:"taskType"`
		LabelCount   int64  `json:"labelCount"`
	}
	query := h.db.Table("ai_ontology_version v").
		Select(`v.*, o.code AS ontology_code, o.name AS ontology_name, o.task_type,
			COUNT(l.id) AS label_count`).
		Joins("JOIN ai_ontology o ON o.id = v.ontology_id AND o.tenant_id = v.tenant_id").
		Joins("LEFT JOIN ai_ontology_label l ON l.ontology_version_id = v.id AND l.tenant_id = v.tenant_id").
		Where("v.tenant_id = ? AND v.project_id = ? AND o.status = ?", project.TenantID, project.ID, OntologyActive)
	status := strings.ToUpper(strings.TrimSpace(c.Query("status")))
	if status == "" {
		status = string(OntologyVersionPublished)
	}
	query = query.Where("v.status = ?", status)
	if taskType := strings.ToUpper(strings.TrimSpace(c.Query("taskType"))); taskType != "" {
		query = query.Where("o.task_type = ?", taskType)
	}
	rows := make([]selectableVersion, 0)
	query.Group("v.id, o.id").Order("o.name, v.version_no DESC").Scan(&rows)
	httpx.OK(c, rows)
}

// OntologyCreate godoc
// @Summary Create an ontology and its first draft
// @Tags VisionAI Ontology
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/ontologies [post]
func (h *Handler) OntologyCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER", "DATA_MANAGER") {
		return
	}
	var req ontologyRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "类别体系请求格式错误")
		return
	}
	req.Code, req.Name = strings.TrimSpace(req.Code), strings.TrimSpace(req.Name)
	req.TaskType = strings.ToUpper(strings.TrimSpace(req.TaskType))
	if req.TaskType == "" {
		req.TaskType = "CV_DETECTION"
	}
	if !ontologyCodePattern.MatchString(req.Code) || req.Name == "" {
		httpx.Fail(c, 400, 400, "编码必须以字母开头且仅含字母、数字、下划线或连字符，名称必填")
		return
	}
	row := Ontology{
		TenantID: project.TenantID, ProjectID: project.ID, Code: req.Code, Name: req.Name,
		TaskType: req.TaskType, Description: strings.TrimSpace(req.Description), Status: OntologyActive,
		CreatedBy: c.GetUint64("user_id"),
	}
	version := OntologyVersion{
		TenantID: project.TenantID, ProjectID: project.ID, VersionNo: 1, SemanticVersion: "v1",
		Status: OntologyVersionDraft, CreatedBy: c.GetUint64("user_id"),
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		version.OntologyID = row.ID
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "ONTOLOGY_CREATED", "ONTOLOGY", row.ID, nil, gin.H{"ontology": row, "draftVersionId": version.ID})
	})
	if err != nil {
		httpx.Fail(c, 409, 409, "类别体系编码已存在")
		return
	}
	httpx.OK(c, gin.H{"ontology": row, "version": version})
}

// OntologyGet godoc
// @Summary Get an ontology and all versions
// @Tags VisionAI Ontology
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/ontologies/{ontologyId} [get]
func (h *Handler) OntologyGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	row, ok := h.ontologyForProject(c, project)
	if !ok {
		return
	}
	versions := make([]OntologyVersion, 0)
	h.db.Where("tenant_id = ? AND ontology_id = ?", project.TenantID, row.ID).Order("version_no DESC").Find(&versions)
	httpx.OK(c, gin.H{"ontology": row, "versions": versions})
}

// OntologyVersionCreate godoc
// @Summary Create an editable ontology draft
// @Tags VisionAI Ontology
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/ontologies/{ontologyId}/versions [post]
func (h *Handler) OntologyVersionCreate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER", "DATA_MANAGER") {
		return
	}
	ontology, ok := h.ontologyForProject(c, project)
	if !ok {
		return
	}
	if ontology.Status != OntologyActive {
		httpx.Fail(c, 409, 409, "已归档类别体系不能创建版本")
		return
	}
	var req ontologyVersionRequest
	if c.Request.ContentLength > 0 && c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "版本请求格式错误")
		return
	}
	var created OntologyVersion
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var latest OntologyVersion
		tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"tenant_id = ? AND ontology_id = ?", project.TenantID, ontology.ID,
		).Order("version_no DESC").First(&latest)
		created = OntologyVersion{
			TenantID: project.TenantID, ProjectID: project.ID, OntologyID: ontology.ID,
			VersionNo: latest.VersionNo + 1, Status: OntologyVersionDraft, CreatedBy: c.GetUint64("user_id"),
		}
		if created.VersionNo < 1 {
			created.VersionNo = 1
		}
		created.SemanticVersion = fmt.Sprintf("v%d", created.VersionNo)
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if req.SourceVersionID == 0 {
			return nil
		}
		var source OntologyVersion
		if err := tx.Where("tenant_id = ? AND ontology_id = ? AND id = ? AND status IN ?",
			project.TenantID, ontology.ID, req.SourceVersionID,
			[]OntologyVersionStatus{OntologyVersionPublished, OntologyVersionDeprecated},
		).First(&source).Error; err != nil {
			return err
		}
		views, err := loadOntologyLabelViews(tx, source)
		if err != nil {
			return err
		}
		for _, view := range views {
			label := view.OntologyLabel
			label.ID, label.OntologyVersionID = 0, created.ID
			label.CreatedAt, label.UpdatedAt = time.Time{}, time.Time{}
			if err := tx.Create(&label).Error; err != nil {
				return err
			}
			for _, sourceAttribute := range view.Attributes {
				attribute := sourceAttribute
				attribute.ID, attribute.OntologyVersionID, attribute.OntologyLabelID = 0, created.ID, label.ID
				attribute.CreatedAt, attribute.UpdatedAt = time.Time{}, time.Time{}
				if err := tx.Create(&attribute).Error; err != nil {
					return err
				}
			}
		}
		return appendAudit(tx, c, project.ID, "ONTOLOGY_VERSION_CREATED", "ONTOLOGY_VERSION", created.ID, nil, created)
	})
	if err != nil {
		httpx.Fail(c, 409, 409, "类别版本创建失败或源版本无效")
		return
	}
	httpx.OK(c, created)
}

// OntologyVersionGet godoc
// @Summary Get ontology version labels and attributes
// @Tags VisionAI Ontology
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/ontology-versions/{versionId} [get]
func (h *Handler) OntologyVersionGet(c *gin.Context) {
	project, ok := h.projectAccess(c, false)
	if !ok {
		return
	}
	version, ok := h.ontologyVersionForProject(c, project)
	if !ok {
		return
	}
	var ontology Ontology
	h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, version.OntologyID).First(&ontology)
	labels, err := loadOntologyLabelViews(h.db, version)
	if err != nil {
		httpx.Fail(c, 500, 500, "类别版本读取失败")
		return
	}
	httpx.OK(c, gin.H{"ontology": ontology, "version": version, "labels": labels})
}

func normalizeOntologyLabels(inputs []ontologyLabelInput) ([]ontologyLabelInput, error) {
	if len(inputs) == 0 || len(inputs) > 500 {
		return nil, fmt.Errorf("类别数量必须为 1 到 500")
	}
	seenCodes, seenNames := map[string]bool{}, map[string]bool{}
	for index := range inputs {
		inputs[index].Code = strings.TrimSpace(inputs[index].Code)
		inputs[index].Name = strings.TrimSpace(inputs[index].Name)
		inputs[index].Color = strings.ToUpper(strings.TrimSpace(inputs[index].Color))
		inputs[index].ShapeType = strings.ToLower(strings.TrimSpace(inputs[index].ShapeType))
		if inputs[index].ShapeType == "" {
			inputs[index].ShapeType = "rectangle"
		}
		if inputs[index].Color == "" {
			inputs[index].Color = []string{"#FF4D4F", "#1677FF", "#52C41A", "#FAAD14", "#722ED1"}[index%5]
		}
		key := strings.ToLower(inputs[index].Code)
		nameKey := strings.ToLower(inputs[index].Name)
		if !ontologyCodePattern.MatchString(inputs[index].Code) || inputs[index].Name == "" || seenCodes[key] || seenNames[nameKey] {
			return nil, fmt.Errorf("类别编码/名称为空、重复或格式错误")
		}
		seenCodes[key], seenNames[nameKey] = true, true
		seenAttributes := map[string]bool{}
		for attributeIndex := range inputs[index].Attributes {
			attribute := &inputs[index].Attributes[attributeIndex]
			attribute.Name = strings.TrimSpace(attribute.Name)
			attribute.InputType = strings.ToLower(strings.TrimSpace(attribute.InputType))
			if attribute.InputType == "" {
				attribute.InputType = "select"
			}
			attributeKey := strings.ToLower(attribute.Name)
			if attribute.Name == "" || seenAttributes[attributeKey] {
				return nil, fmt.Errorf("类别 %s 的属性为空或重复", inputs[index].Name)
			}
			seenAttributes[attributeKey] = true
			values := make([]string, 0, len(attribute.Values))
			valueSet := map[string]bool{}
			for _, value := range attribute.Values {
				value = strings.TrimSpace(value)
				if value != "" && !valueSet[value] {
					valueSet[value] = true
					values = append(values, value)
				}
			}
			attribute.Values = values
		}
	}
	sort.SliceStable(inputs, func(i, j int) bool {
		if inputs[i].Sort == inputs[j].Sort {
			return inputs[i].Code < inputs[j].Code
		}
		return inputs[i].Sort < inputs[j].Sort
	})
	return inputs, nil
}

// OntologyLabelsReplace godoc
// @Summary Replace all labels in a draft ontology version
// @Tags VisionAI Ontology
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/ontology-versions/{versionId}/labels [put]
func (h *Handler) OntologyLabelsReplace(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER", "DATA_MANAGER") {
		return
	}
	version, ok := h.ontologyVersionForProject(c, project)
	if !ok {
		return
	}
	if version.Status != OntologyVersionDraft {
		httpx.Fail(c, 409, 409, "已发布或废弃的类别版本不可修改")
		return
	}
	var req ontologyLabelsRequest
	if c.ShouldBindJSON(&req) != nil {
		httpx.Fail(c, 400, 400, "类别请求格式错误")
		return
	}
	labels, err := normalizeOntologyLabels(req.Labels)
	if err != nil {
		httpx.Fail(c, 400, 400, err.Error())
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND ontology_version_id = ?", project.TenantID, version.ID).
			Delete(&OntologyAttribute{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND ontology_version_id = ?", project.TenantID, version.ID).
			Delete(&OntologyLabel{}).Error; err != nil {
			return err
		}
		for _, input := range labels {
			label := OntologyLabel{
				TenantID: project.TenantID, ProjectID: project.ID, OntologyVersionID: version.ID,
				Code: input.Code, Name: input.Name, Color: input.Color, ShapeType: input.ShapeType, Sort: input.Sort,
			}
			if err := tx.Create(&label).Error; err != nil {
				return err
			}
			for _, inputAttribute := range input.Attributes {
				values, _ := json.Marshal(inputAttribute.Values)
				attribute := OntologyAttribute{
					TenantID: project.TenantID, ProjectID: project.ID, OntologyVersionID: version.ID, OntologyLabelID: label.ID,
					Name: inputAttribute.Name, InputType: inputAttribute.InputType, Values: string(values),
					DefaultValue: inputAttribute.DefaultValue, Mutable: inputAttribute.Mutable, Sort: inputAttribute.Sort,
				}
				if err := tx.Create(&attribute).Error; err != nil {
					return err
				}
			}
		}
		return appendAudit(tx, c, project.ID, "ONTOLOGY_LABELS_REPLACED", "ONTOLOGY_VERSION", version.ID, nil, gin.H{"labelCount": len(labels)})
	})
	if err != nil {
		httpx.Fail(c, 500, 500, "类别保存失败")
		return
	}
	views, _ := loadOntologyLabelViews(h.db, version)
	httpx.OK(c, views)
}

func ontologyCanonicalSnapshot(ontology Ontology, version OntologyVersion, views []ontologyLabelView) []byte {
	type canonicalAttribute struct {
		Name         string   `json:"name"`
		InputType    string   `json:"inputType"`
		Values       []string `json:"values"`
		DefaultValue string   `json:"defaultValue"`
		Mutable      bool     `json:"mutable"`
	}
	type canonicalLabel struct {
		Code       string               `json:"code"`
		Name       string               `json:"name"`
		Color      string               `json:"color"`
		ShapeType  string               `json:"shapeType"`
		Attributes []canonicalAttribute `json:"attributes"`
	}
	labels := make([]canonicalLabel, 0, len(views))
	for _, view := range views {
		label := canonicalLabel{
			Code: view.Code, Name: view.Name, Color: view.Color, ShapeType: view.ShapeType,
			Attributes: make([]canonicalAttribute, 0, len(view.Attributes)),
		}
		for _, attribute := range view.Attributes {
			var values []string
			_ = json.Unmarshal([]byte(attribute.Values), &values)
			label.Attributes = append(label.Attributes, canonicalAttribute{
				Name: attribute.Name, InputType: attribute.InputType, Values: values,
				DefaultValue: attribute.DefaultValue, Mutable: attribute.Mutable,
			})
		}
		labels = append(labels, label)
	}
	raw, _ := json.Marshal(gin.H{
		"schemaVersion": "visionai.ontology.v1", "code": ontology.Code, "taskType": ontology.TaskType,
		"version": version.SemanticVersion, "labels": labels,
	})
	return raw
}

// OntologyVersionPublish godoc
// @Summary Publish and lock an ontology version
// @Tags VisionAI Ontology
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/ontology-versions/{versionId}/publish [post]
func (h *Handler) OntologyVersionPublish(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER", "DATA_MANAGER") {
		return
	}
	version, ok := h.ontologyVersionForProject(c, project)
	if !ok {
		return
	}
	if version.Status == OntologyVersionPublished {
		httpx.OK(c, version)
		return
	}
	if version.Status != OntologyVersionDraft {
		httpx.Fail(c, 409, 409, "只有草稿类别版本可以发布")
		return
	}
	var ontology Ontology
	if h.db.Where("tenant_id = ? AND project_id = ? AND id = ?", project.TenantID, project.ID, version.OntologyID).First(&ontology).Error != nil {
		httpx.Fail(c, 404, 404, "类别体系不存在")
		return
	}
	views, err := loadOntologyLabelViews(h.db, version)
	if err != nil || len(views) == 0 {
		httpx.Fail(c, 409, 409, "类别版本至少包含一个类别")
		return
	}
	raw := ontologyCanonicalSnapshot(ontology, version, views)
	sum := sha256.Sum256(raw)
	now := time.Now()
	before := version
	version.Status, version.Checksum = OntologyVersionPublished, hex.EncodeToString(sum[:])
	version.PublishedAt, version.PublishedBy = &now, c.GetUint64("user_id")
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&version).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, project.ID, "ONTOLOGY_VERSION_PUBLISHED", "ONTOLOGY_VERSION", version.ID, before, version)
	}); err != nil {
		httpx.Fail(c, 500, 500, "类别版本发布失败")
		return
	}
	httpx.OK(c, version)
}

// OntologyVersionDeprecate godoc
// @Summary Deprecate an ontology version for new work
// @Tags VisionAI Ontology
// @Security BearerAuth
// @Success 200 {object} httpx.Response
// @Router /ai-platform/projects/{id}/ontology-versions/{versionId}/deprecate [post]
func (h *Handler) OntologyVersionDeprecate(c *gin.Context) {
	project, ok := h.projectAccess(c, true)
	if !ok || !h.requireProjectRole(c, project, "PROJECT_OWNER", "DATA_MANAGER") {
		return
	}
	version, ok := h.ontologyVersionForProject(c, project)
	if !ok {
		return
	}
	if version.Status != OntologyVersionPublished {
		httpx.Fail(c, 409, 409, "只有已发布类别版本可以废弃")
		return
	}
	before := version
	version.Status = OntologyVersionDeprecated
	if err := h.db.Save(&version).Error; err != nil {
		httpx.Fail(c, 500, 500, "类别版本废弃失败")
		return
	}
	_ = appendAudit(h.db, c, project.ID, "ONTOLOGY_VERSION_DEPRECATED", "ONTOLOGY_VERSION", version.ID, before, version)
	httpx.OK(c, version)
}

func annotationLabelsForOntologyVersion(db *gorm.DB, version OntologyVersion) ([]annotation.Label, error) {
	views, err := loadOntologyLabelViews(db, version)
	if err != nil || len(views) == 0 {
		return nil, fmt.Errorf("类别体系版本没有可用类别")
	}
	labels := make([]annotation.Label, 0, len(views))
	for _, view := range views {
		label := annotation.Label{Name: view.Name, Color: view.Color, Type: view.ShapeType}
		for _, attribute := range view.Attributes {
			var values []string
			_ = json.Unmarshal([]byte(attribute.Values), &values)
			label.Attributes = append(label.Attributes, annotation.Attribute{
				Name: attribute.Name, InputType: attribute.InputType, Values: values,
				DefaultValue: attribute.DefaultValue, Mutable: attribute.Mutable,
			})
		}
		labels = append(labels, label)
	}
	return labels, nil
}

func loadPublishedOntologyVersion(db *gorm.DB, project Project, versionID uint64, taskType string) (OntologyVersion, []annotation.Label, error) {
	var version OntologyVersion
	if versionID == 0 || db.Where(
		"tenant_id = ? AND project_id = ? AND id = ? AND status = ?",
		project.TenantID, project.ID, versionID, OntologyVersionPublished,
	).First(&version).Error != nil {
		return version, nil, fmt.Errorf("必须选择当前项目已发布的类别体系版本")
	}
	var ontology Ontology
	if db.Where("tenant_id = ? AND project_id = ? AND id = ? AND status = ?",
		project.TenantID, project.ID, version.OntologyID, OntologyActive,
	).First(&ontology).Error != nil {
		return version, nil, fmt.Errorf("类别体系不存在或已归档")
	}
	if taskType != "" && ontology.TaskType != taskType {
		return version, nil, fmt.Errorf("类别体系任务类型 %s 与任务 %s 不兼容", ontology.TaskType, taskType)
	}
	labels, err := annotationLabelsForOntologyVersion(db, version)
	if err != nil {
		return version, nil, err
	}
	return version, labels, nil
}

func cloneProjectOntologies(tx *gorm.DB, source, target Project, createdBy uint64) error {
	var ontologies []Ontology
	if err := tx.Where("tenant_id = ? AND project_id = ?", source.TenantID, source.ID).Order("id").Find(&ontologies).Error; err != nil {
		return err
	}
	for _, sourceOntology := range ontologies {
		targetOntology := sourceOntology
		targetOntology.ID, targetOntology.ProjectID, targetOntology.CreatedBy = 0, target.ID, createdBy
		targetOntology.CreatedAt, targetOntology.UpdatedAt = time.Time{}, time.Time{}
		if err := tx.Create(&targetOntology).Error; err != nil {
			return err
		}
		var versions []OntologyVersion
		if err := tx.Where("tenant_id = ? AND ontology_id = ?", source.TenantID, sourceOntology.ID).
			Order("version_no").Find(&versions).Error; err != nil {
			return err
		}
		for _, sourceVersion := range versions {
			views, err := loadOntologyLabelViews(tx, sourceVersion)
			if err != nil {
				return err
			}
			targetVersion := sourceVersion
			targetVersion.ID, targetVersion.ProjectID, targetVersion.OntologyID = 0, target.ID, targetOntology.ID
			targetVersion.CreatedBy = createdBy
			targetVersion.CreatedAt, targetVersion.UpdatedAt = time.Time{}, time.Time{}
			if err := tx.Create(&targetVersion).Error; err != nil {
				return err
			}
			for _, view := range views {
				label := view.OntologyLabel
				label.ID, label.ProjectID, label.OntologyVersionID = 0, target.ID, targetVersion.ID
				label.CreatedAt, label.UpdatedAt = time.Time{}, time.Time{}
				if err := tx.Create(&label).Error; err != nil {
					return err
				}
				for _, sourceAttribute := range view.Attributes {
					attribute := sourceAttribute
					attribute.ID, attribute.ProjectID = 0, target.ID
					attribute.OntologyVersionID, attribute.OntologyLabelID = targetVersion.ID, label.ID
					attribute.CreatedAt, attribute.UpdatedAt = time.Time{}, time.Time{}
					if err := tx.Create(&attribute).Error; err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func ensureLegacyOntologyVersion(db *gorm.DB, tenantID, projectID uint64, versionName, labelsJSON string, createdBy uint64) (OntologyVersion, error) {
	if strings.TrimSpace(versionName) == "" {
		versionName = "legacy-v1"
	}
	if strings.TrimSpace(labelsJSON) == "" || labelsJSON == "null" || labelsJSON == "[]" {
		labelsJSON = `[{"name":"defect","color":"#FF4D4F","type":"rectangle"}]`
	}
	keyRaw := []byte(fmt.Sprintf("%d:%d:%s:%s", tenantID, projectID, versionName, labelsJSON))
	keySum := sha256.Sum256(keyRaw)
	code := "Legacy_" + hex.EncodeToString(keySum[:6])
	var ontology Ontology
	result := db.Where("tenant_id = ? AND project_id = ? AND code = ?", tenantID, projectID, code).First(&ontology)
	if result.Error == nil {
		var existing OntologyVersion
		err := db.Where("tenant_id = ? AND ontology_id = ? AND version_no = 1", tenantID, ontology.ID).First(&existing).Error
		return existing, err
	}
	if result.Error != gorm.ErrRecordNotFound {
		return OntologyVersion{}, result.Error
	}
	ontology = Ontology{
		TenantID: tenantID, ProjectID: projectID, Code: code,
		Name: "历史类别体系 " + versionName, TaskType: "CV_DETECTION",
		Description: "由历史自由文本类别自动迁移；只读保留用于血缘兼容。",
		Status:      OntologyActive, CreatedBy: createdBy,
	}
	version := OntologyVersion{
		TenantID: tenantID, ProjectID: projectID, VersionNo: 1, SemanticVersion: versionName,
		Status: OntologyVersionPublished, CreatedBy: createdBy, PublishedBy: createdBy,
	}
	now := time.Now()
	version.PublishedAt = &now
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ontology).Error; err != nil {
			return err
		}
		version.OntologyID = ontology.ID
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		var legacyLabels []annotation.Label
		if json.Unmarshal([]byte(labelsJSON), &legacyLabels) != nil || len(legacyLabels) == 0 {
			legacyLabels = []annotation.Label{{Name: "defect", Color: "#FF4D4F", Type: "rectangle"}}
		}
		for index, source := range legacyLabels {
			label := OntologyLabel{
				TenantID: tenantID, ProjectID: projectID, OntologyVersionID: version.ID,
				Code: fmt.Sprintf("C%03d", index+1), Name: source.Name, Color: source.Color,
				ShapeType: source.Type, Sort: index,
			}
			if label.Name == "" {
				label.Name = label.Code
			}
			if label.Color == "" {
				label.Color = "#1677FF"
			}
			if label.ShapeType == "" {
				label.ShapeType = "rectangle"
			}
			if err := tx.Create(&label).Error; err != nil {
				return err
			}
			for attributeIndex, sourceAttribute := range source.Attributes {
				values, _ := json.Marshal(sourceAttribute.Values)
				attribute := OntologyAttribute{
					TenantID: tenantID, ProjectID: projectID, OntologyVersionID: version.ID, OntologyLabelID: label.ID,
					Name: sourceAttribute.Name, InputType: sourceAttribute.InputType, Values: string(values),
					DefaultValue: sourceAttribute.DefaultValue, Mutable: sourceAttribute.Mutable, Sort: attributeIndex,
				}
				if err := tx.Create(&attribute).Error; err != nil {
					return err
				}
			}
		}
		views, err := loadOntologyLabelViews(tx, version)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(ontologyCanonicalSnapshot(ontology, version, views))
		version.Checksum = hex.EncodeToString(sum[:])
		return tx.Save(&version).Error
	})
	return version, err
}

func migrateLegacyOntologies(db *gorm.DB) error {
	var tasks []AnnotationTask
	if err := db.Where("COALESCE(ontology_version_id, 0) = 0").Order("id").Find(&tasks).Error; err != nil {
		return err
	}
	for _, task := range tasks {
		version, err := ensureLegacyOntologyVersion(
			db, task.TenantID, task.ProjectID, task.OntologyVersion, task.Labels, task.CreatedBy,
		)
		if err != nil {
			return err
		}
		if err := db.Model(&AnnotationTask{}).Where("id = ?", task.ID).Updates(map[string]any{
			"ontology_version_id": version.ID, "ontology_version": version.SemanticVersion, "ontology_checksum": version.Checksum,
		}).Error; err != nil {
			return err
		}
		if err := db.Model(&AnnotationRevision{}).Where(
			"tenant_id = ? AND annotation_task_id = ? AND COALESCE(ontology_version_id, 0) = 0", task.TenantID, task.ID,
		).Updates(map[string]any{"ontology_version_id": version.ID, "ontology_checksum": version.Checksum}).Error; err != nil {
			return err
		}
	}
	var versions []DatasetVersion
	if err := db.Where("COALESCE(ontology_version_id, 0) = 0").Order("id").Find(&versions).Error; err != nil {
		return err
	}
	for _, datasetVersion := range versions {
		var revision AnnotationRevision
		if datasetVersion.AnnotationRevisionID != 0 && db.Where(
			"tenant_id = ? AND id = ? AND ontology_version_id > 0", datasetVersion.TenantID, datasetVersion.AnnotationRevisionID,
		).First(&revision).Error == nil {
			if err := db.Model(&DatasetVersion{}).Where("id = ?", datasetVersion.ID).Updates(map[string]any{
				"ontology_version_id": revision.OntologyVersionID, "ontology_checksum": revision.OntologyChecksum,
			}).Error; err != nil {
				return err
			}
			continue
		}
		version, err := ensureLegacyOntologyVersion(
			db, datasetVersion.TenantID, datasetVersion.ProjectID, datasetVersion.OntologyVersion, "", datasetVersion.CreatedBy,
		)
		if err != nil {
			return err
		}
		if err := db.Model(&DatasetVersion{}).Where("id = ?", datasetVersion.ID).Updates(map[string]any{
			"ontology_version_id": version.ID, "ontology_version": version.SemanticVersion, "ontology_checksum": version.Checksum,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
