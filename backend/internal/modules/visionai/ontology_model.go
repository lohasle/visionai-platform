package visionai

import "time"

type OntologyStatus string
type OntologyVersionStatus string

const (
	OntologyActive   OntologyStatus = "ACTIVE"
	OntologyArchived OntologyStatus = "ARCHIVED"

	OntologyVersionDraft      OntologyVersionStatus = "DRAFT"
	OntologyVersionPublished  OntologyVersionStatus = "PUBLISHED"
	OntologyVersionDeprecated OntologyVersionStatus = "DEPRECATED"
)

type Ontology struct {
	ID          uint64         `gorm:"primaryKey" json:"id"`
	TenantID    uint64         `gorm:"uniqueIndex:uk_ontology_code;not null" json:"tenantId"`
	ProjectID   uint64         `gorm:"uniqueIndex:uk_ontology_code;index;not null" json:"projectId"`
	Code        string         `gorm:"size:64;uniqueIndex:uk_ontology_code;not null" json:"code"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	TaskType    string         `gorm:"size:40;index;not null" json:"taskType"`
	Description string         `gorm:"size:1024" json:"description"`
	Status      OntologyStatus `gorm:"size:16;index;not null" json:"status"`
	CreatedBy   uint64         `gorm:"index;not null" json:"createdBy"`
	CreatedAt   time.Time      `json:"createTime"`
	UpdatedAt   time.Time      `json:"updateTime"`
}

type OntologyVersion struct {
	ID              uint64                `gorm:"primaryKey" json:"id"`
	TenantID        uint64                `gorm:"uniqueIndex:uk_ontology_version;not null" json:"tenantId"`
	ProjectID       uint64                `gorm:"index;not null" json:"projectId"`
	OntologyID      uint64                `gorm:"uniqueIndex:uk_ontology_version;index;not null" json:"ontologyId"`
	VersionNo       int                   `gorm:"uniqueIndex:uk_ontology_version;not null" json:"versionNo"`
	SemanticVersion string                `gorm:"size:40;not null" json:"semanticVersion"`
	Status          OntologyVersionStatus `gorm:"size:16;index;not null" json:"status"`
	Checksum        string                `gorm:"size:64;index" json:"checksum"`
	PublishedBy     uint64                `gorm:"index" json:"publishedBy"`
	PublishedAt     *time.Time            `json:"publishedAt"`
	CreatedBy       uint64                `gorm:"index;not null" json:"createdBy"`
	CreatedAt       time.Time             `json:"createTime"`
	UpdatedAt       time.Time             `json:"updateTime"`
}

type OntologyLabel struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TenantID          uint64    `gorm:"not null" json:"tenantId"`
	ProjectID         uint64    `gorm:"index;not null" json:"projectId"`
	OntologyVersionID uint64    `gorm:"uniqueIndex:uk_ontology_label;index;not null" json:"ontologyVersionId"`
	Code              string    `gorm:"size:64;uniqueIndex:uk_ontology_label;not null" json:"code"`
	Name              string    `gorm:"size:128;not null" json:"name"`
	Color             string    `gorm:"size:16;not null" json:"color"`
	ShapeType         string    `gorm:"size:32;not null" json:"shapeType"`
	Sort              int       `gorm:"not null;default:0" json:"sort"`
	CreatedAt         time.Time `json:"createTime"`
	UpdatedAt         time.Time `json:"updateTime"`
}

type OntologyAttribute struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TenantID          uint64    `gorm:"not null" json:"tenantId"`
	ProjectID         uint64    `gorm:"index;not null" json:"projectId"`
	OntologyVersionID uint64    `gorm:"index;not null" json:"ontologyVersionId"`
	OntologyLabelID   uint64    `gorm:"uniqueIndex:uk_ontology_attribute;index;not null" json:"ontologyLabelId"`
	Name              string    `gorm:"size:64;uniqueIndex:uk_ontology_attribute;not null" json:"name"`
	InputType         string    `gorm:"size:24;not null" json:"inputType"`
	Values            string    `gorm:"type:json;not null" json:"values"`
	DefaultValue      string    `gorm:"size:256" json:"defaultValue"`
	Mutable           bool      `gorm:"not null;default:false" json:"mutable"`
	Sort              int       `gorm:"not null;default:0" json:"sort"`
	CreatedAt         time.Time `json:"createTime"`
	UpdatedAt         time.Time `json:"updateTime"`
}

type AssetTagDefinition struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"uniqueIndex:uk_asset_tag_definition;not null" json:"tenantId"`
	ProjectID   uint64    `gorm:"uniqueIndex:uk_asset_tag_definition;index;not null" json:"projectId"`
	Code        string    `gorm:"size:64;uniqueIndex:uk_asset_tag_definition;not null" json:"code"`
	Name        string    `gorm:"size:128;not null" json:"name"`
	Category    string    `gorm:"size:16;index;not null" json:"category"`
	Color       string    `gorm:"size:16;not null" json:"color"`
	Description string    `gorm:"size:512" json:"description"`
	Enabled     bool      `gorm:"index;not null;default:true" json:"enabled"`
	CreatedBy   uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

func (Ontology) TableName() string           { return "ai_ontology" }
func (OntologyVersion) TableName() string    { return "ai_ontology_version" }
func (OntologyLabel) TableName() string      { return "ai_ontology_label" }
func (OntologyAttribute) TableName() string  { return "ai_ontology_attribute" }
func (AssetTagDefinition) TableName() string { return "ai_asset_tag_definition" }
