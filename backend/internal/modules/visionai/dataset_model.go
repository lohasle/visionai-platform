package visionai

import "time"

type DatasetVersionStatus string

const (
	DatasetVersionDraft      DatasetVersionStatus = "DRAFT"
	DatasetVersionValidating DatasetVersionStatus = "VALIDATING"
	DatasetVersionReady      DatasetVersionStatus = "READY"
	DatasetVersionFrozen     DatasetVersionStatus = "FROZEN"
	DatasetVersionDeprecated DatasetVersionStatus = "DEPRECATED"
)

type Dataset struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"uniqueIndex:uk_dataset_name;not null" json:"tenantId"`
	ProjectID   uint64    `gorm:"uniqueIndex:uk_dataset_name;not null" json:"projectId"`
	Name        string    `gorm:"size:160;uniqueIndex:uk_dataset_name;not null" json:"name"`
	TaskType    string    `gorm:"size:40;index;not null" json:"taskType"`
	Description string    `gorm:"size:1024" json:"description"`
	OwnerUserID uint64    `gorm:"index;not null" json:"ownerUserId"`
	CreatedBy   uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

type DatasetVersion struct {
	ID                   uint64               `gorm:"primaryKey" json:"id"`
	TenantID             uint64               `gorm:"index;not null" json:"tenantId"`
	ProjectID            uint64               `gorm:"index;not null" json:"projectId"`
	DatasetID            uint64               `gorm:"uniqueIndex:uk_dataset_version;not null" json:"datasetId"`
	VersionNo            int                  `gorm:"uniqueIndex:uk_dataset_version;not null" json:"versionNo"`
	SemanticVersion      string               `gorm:"size:32;index;not null" json:"semanticVersion"`
	ParentID             uint64               `gorm:"index" json:"parentId"`
	SourceType           string               `gorm:"size:32;not null" json:"sourceType"`
	SourceID             uint64               `gorm:"index;not null" json:"sourceId"`
	AnnotationRevisionID uint64               `gorm:"index;not null" json:"annotationRevisionId"`
	OntologyVersionID    uint64               `gorm:"index" json:"ontologyVersionId"`
	OntologyVersion      string               `gorm:"size:80;not null" json:"ontologyVersion"`
	OntologyChecksum     string               `gorm:"size:64" json:"ontologyChecksum"`
	SplitSeed            int64                `gorm:"not null" json:"splitSeed"`
	SplitConfig          string               `gorm:"type:json;not null" json:"splitConfig"`
	Status               DatasetVersionStatus `gorm:"size:24;index;not null" json:"status"`
	ItemCount            int64                `gorm:"not null" json:"itemCount"`
	TrainCount           int64                `gorm:"not null" json:"trainCount"`
	ValidationCount      int64                `gorm:"not null" json:"validationCount"`
	TestCount            int64                `gorm:"not null" json:"testCount"`
	ManifestURI          string               `gorm:"size:1024" json:"manifestUri"`
	ManifestObjectKey    string               `gorm:"size:1024" json:"-"`
	DatasetCardURI       string               `gorm:"size:1024" json:"datasetCardUri"`
	DatasetCardObjectKey string               `gorm:"size:1024" json:"-"`
	Checksum             string               `gorm:"size:64;index" json:"checksum"`
	ValidationSummary    string               `gorm:"type:json;not null" json:"validationSummary"`
	FrozenBy             uint64               `gorm:"index" json:"frozenBy"`
	FrozenAt             *time.Time           `json:"frozenAt"`
	DeprecatedBy         uint64               `gorm:"index" json:"deprecatedBy"`
	DeprecatedAt         *time.Time           `json:"deprecatedAt"`
	DeprecationReason    string               `gorm:"size:1024" json:"deprecationReason"`
	CreatedBy            uint64               `gorm:"index;not null" json:"createdBy"`
	CreatedAt            time.Time            `json:"createTime"`
	UpdatedAt            time.Time            `json:"updateTime"`
}

type DatasetVersionItem struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	TenantID         uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID        uint64    `gorm:"index;not null" json:"projectId"`
	DatasetVersionID uint64    `gorm:"uniqueIndex:uk_dataset_version_asset;not null" json:"datasetVersionId"`
	AssetID          uint64    `gorm:"uniqueIndex:uk_dataset_version_asset;not null" json:"assetId"`
	Split            string    `gorm:"size:16;index;not null" json:"split"`
	Source           string    `gorm:"size:32;not null" json:"source"`
	CreatedAt        time.Time `json:"createTime"`
}

type DatasetValidationIssue struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	TenantID         uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID        uint64    `gorm:"index;not null" json:"projectId"`
	DatasetVersionID uint64    `gorm:"index;not null" json:"datasetVersionId"`
	AssetID          uint64    `gorm:"index" json:"assetId"`
	Severity         string    `gorm:"size:16;index;not null" json:"severity"`
	Code             string    `gorm:"size:64;index;not null" json:"code"`
	Message          string    `gorm:"size:1024;not null" json:"message"`
	Remediation      string    `gorm:"size:1024" json:"remediation"`
	CreatedAt        time.Time `json:"createTime"`
}

type DatasetUsage struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	TenantID         uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID        uint64    `gorm:"index;not null" json:"projectId"`
	DatasetVersionID uint64    `gorm:"uniqueIndex:uk_dataset_usage;not null" json:"datasetVersionId"`
	ResourceType     string    `gorm:"size:64;uniqueIndex:uk_dataset_usage;not null" json:"resourceType"`
	ResourceID       uint64    `gorm:"uniqueIndex:uk_dataset_usage;not null" json:"resourceId"`
	CreatedAt        time.Time `json:"createTime"`
}

func (Dataset) TableName() string                { return "ai_dataset" }
func (DatasetVersion) TableName() string         { return "ai_dataset_version" }
func (DatasetVersionItem) TableName() string     { return "ai_dataset_version_item" }
func (DatasetValidationIssue) TableName() string { return "ai_dataset_validation_issue" }
func (DatasetUsage) TableName() string           { return "ai_dataset_usage" }
