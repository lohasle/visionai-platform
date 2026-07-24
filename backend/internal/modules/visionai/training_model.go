package visionai

import "time"

type TrainingStatus string

const (
	TrainingDraft      TrainingStatus = "DRAFT"
	TrainingQueued     TrainingStatus = "QUEUED"
	TrainingAllocating TrainingStatus = "ALLOCATING"
	TrainingRunning    TrainingStatus = "RUNNING"
	TrainingExporting  TrainingStatus = "EXPORTING"
	TrainingSucceeded  TrainingStatus = "SUCCEEDED"
	TrainingFailed     TrainingStatus = "FAILED"
	TrainingCancelled  TrainingStatus = "CANCELLED"
	TrainingTimeout    TrainingStatus = "TIMEOUT"
)

type TrainingTemplate struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"uniqueIndex:uk_training_template;not null" json:"tenantId"`
	ProjectID   uint64    `gorm:"uniqueIndex:uk_training_template;not null" json:"projectId"`
	Name        string    `gorm:"size:160;uniqueIndex:uk_training_template;not null" json:"name"`
	AIType      string    `gorm:"size:40;index;not null" json:"aiType"`
	Description string    `gorm:"size:1024" json:"description"`
	CreatedBy   uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

type TrainingTemplateVersion struct {
	ID                   uint64     `gorm:"primaryKey" json:"id"`
	TenantID             uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID            uint64     `gorm:"index;not null" json:"projectId"`
	TemplateID           uint64     `gorm:"uniqueIndex:uk_training_template_version;not null" json:"templateId"`
	VersionNo            int        `gorm:"uniqueIndex:uk_training_template_version;not null" json:"versionNo"`
	SemanticVersion      string     `gorm:"size:32;index;not null" json:"semanticVersion"`
	Trainer              string     `gorm:"size:80;not null" json:"trainer"`
	ImageRef             string     `gorm:"size:512;not null" json:"imageRef"`
	Entrypoint           string     `gorm:"size:512;not null" json:"entrypoint"`
	ParameterSchema      string     `gorm:"type:json;not null" json:"parameterSchema"`
	OutputProtocol       string     `gorm:"size:80;not null" json:"outputProtocol"`
	ResourceRequirements string     `gorm:"type:json;not null" json:"resourceRequirements"`
	Compatibility        string     `gorm:"type:json;not null" json:"compatibility"`
	LicensePolicy        string     `gorm:"type:json;not null" json:"licensePolicy"`
	Published            bool       `gorm:"index;not null;default:false" json:"published"`
	SmokeStatus          string     `gorm:"size:24;index;not null" json:"smokeStatus"`
	SmokeReport          string     `gorm:"type:json;not null" json:"smokeReport"`
	PublishedBy          uint64     `gorm:"index" json:"publishedBy"`
	PublishedAt          *time.Time `json:"publishedAt"`
	CreatedBy            uint64     `gorm:"index;not null" json:"createdBy"`
	CreatedAt            time.Time  `json:"createTime"`
	UpdatedAt            time.Time  `json:"updateTime"`
}

type TrainingRun struct {
	ID                uint64         `gorm:"primaryKey" json:"id"`
	TenantID          uint64         `gorm:"index;not null" json:"tenantId"`
	ProjectID         uint64         `gorm:"index;not null" json:"projectId"`
	Name              string         `gorm:"size:200;not null" json:"name"`
	DatasetVersionID  uint64         `gorm:"index;not null" json:"datasetVersionId"`
	TemplateVersionID uint64         `gorm:"index;not null" json:"templateVersionId"`
	Provider          string         `gorm:"size:32;index;not null" json:"provider"`
	Queue             string         `gorm:"size:128;index;not null" json:"queue"`
	GPUCount          int            `gorm:"not null" json:"gpuCount"`
	Priority          int            `gorm:"index;not null" json:"priority"`
	Parameters        string         `gorm:"type:json;not null" json:"parameters"`
	RuntimeSpec       string         `gorm:"type:json;not null" json:"runtimeSpec"`
	CodeCommit        string         `gorm:"size:128" json:"codeCommit"`
	PretrainedRef     string         `gorm:"size:1024" json:"pretrainedRef"`
	ParentRunID       uint64         `gorm:"index" json:"parentRunId"`
	Status            TrainingStatus `gorm:"size:24;index;not null" json:"status"`
	Progress          int            `gorm:"not null" json:"progress"`
	ExternalBindingID uint64         `gorm:"index" json:"externalBindingId"`
	ResultManifestURI string         `gorm:"size:1024" json:"resultManifestUri"`
	MetricSummary     string         `gorm:"type:json;not null" json:"metricSummary"`
	ErrorCategory     string         `gorm:"size:64" json:"errorCategory"`
	ErrorCode         string         `gorm:"size:128" json:"errorCode"`
	ErrorMessage      string         `gorm:"size:2048" json:"errorMessage"`
	CancelRequestedAt *time.Time     `json:"cancelRequestedAt"`
	StartedAt         *time.Time     `json:"startedAt"`
	FinishedAt        *time.Time     `json:"finishedAt"`
	CreatedBy         uint64         `gorm:"index;not null" json:"createdBy"`
	CreatedAt         time.Time      `json:"createTime"`
	UpdatedAt         time.Time      `json:"updateTime"`
}

type TrainingArtifact struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	TenantID      uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID     uint64    `gorm:"index;not null" json:"projectId"`
	TrainingRunID uint64    `gorm:"index;not null" json:"trainingRunId"`
	Kind          string    `gorm:"size:64;index;not null" json:"kind"`
	Name          string    `gorm:"size:256;not null" json:"name"`
	URI           string    `gorm:"size:1024;not null" json:"uri"`
	SHA256        string    `gorm:"size:64;index;not null" json:"sha256"`
	Size          int64     `gorm:"not null" json:"size"`
	MediaType     string    `gorm:"size:128" json:"mediaType"`
	CreatedAt     time.Time `json:"createTime"`
}

type TrainingMetric struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	TenantID      uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID     uint64    `gorm:"index;not null" json:"projectId"`
	TrainingRunID uint64    `gorm:"uniqueIndex:uk_training_metric;not null" json:"trainingRunId"`
	Name          string    `gorm:"size:128;uniqueIndex:uk_training_metric;not null" json:"name"`
	Step          int64     `gorm:"uniqueIndex:uk_training_metric;not null" json:"step"`
	Value         float64   `json:"value"`
	CreatedAt     time.Time `json:"createTime"`
}

func (TrainingTemplate) TableName() string        { return "ai_training_template" }
func (TrainingTemplateVersion) TableName() string { return "ai_training_template_version" }
func (TrainingRun) TableName() string             { return "ai_training_run" }
func (TrainingArtifact) TableName() string        { return "ai_training_artifact" }
func (TrainingMetric) TableName() string          { return "ai_training_metric" }
