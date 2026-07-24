package visionai

import "time"

type Deployment struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TenantID          uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID         uint64    `gorm:"index;not null" json:"projectId"`
	Name              string    `gorm:"size:200;not null" json:"name"`
	Environment       string    `gorm:"size:32;index;not null" json:"environment"`
	Status            string    `gorm:"size:32;index;not null" json:"status"`
	CurrentRevisionID uint64    `gorm:"index" json:"currentRevisionId"`
	EndpointURL       string    `gorm:"size:1024" json:"endpointUrl"`
	CreatedBy         uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt         time.Time `json:"createTime"`
	UpdatedAt         time.Time `json:"updateTime"`
}

type DeploymentRevision struct {
	ID               uint64     `gorm:"primaryKey" json:"id"`
	TenantID         uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID        uint64     `gorm:"index;not null" json:"projectId"`
	DeploymentID     uint64     `gorm:"uniqueIndex:uk_deployment_revision;not null" json:"deploymentId"`
	RevisionNo       int        `gorm:"uniqueIndex:uk_deployment_revision;not null" json:"revisionNo"`
	ModelVersionID   uint64     `gorm:"index;not null" json:"modelVersionId"`
	SourceRevisionID uint64     `gorm:"index" json:"sourceRevisionId"`
	Status           string     `gorm:"size:32;index;not null" json:"status"`
	Config           string     `gorm:"type:json;not null" json:"config"`
	ArtifactURI      string     `gorm:"size:1024;not null" json:"artifactUri"`
	ArtifactSHA256   string     `gorm:"size:64;not null" json:"artifactSha256"`
	CreatedBy        uint64     `gorm:"index;not null" json:"createdBy"`
	ActivatedAt      *time.Time `json:"activatedAt"`
	CreatedAt        time.Time  `json:"createTime"`
}

type InferenceTrace struct {
	ID                   uint64    `gorm:"primaryKey" json:"id"`
	TenantID             uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID            uint64    `gorm:"index;not null" json:"projectId"`
	DeploymentID         uint64    `gorm:"index;not null" json:"deploymentId"`
	DeploymentRevisionID uint64    `gorm:"index;not null" json:"deploymentRevisionId"`
	ModelVersionID       uint64    `gorm:"index;not null" json:"modelVersionId"`
	TraceID              string    `gorm:"size:64;uniqueIndex;not null" json:"traceId"`
	AssetID              uint64    `gorm:"index" json:"assetId"`
	Status               string    `gorm:"size:24;index;not null" json:"status"`
	LatencyMS            float64   `gorm:"index" json:"latencyMs"`
	DetectionCount       int       `gorm:"index" json:"detectionCount"`
	MeanConfidence       float64   `gorm:"index" json:"meanConfidence"`
	Result               string    `gorm:"type:json;not null" json:"result"`
	ErrorMessage         string    `gorm:"size:2048" json:"errorMessage"`
	CreatedAt            time.Time `gorm:"index" json:"createTime"`
}

type AlertRule struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	TenantID     uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID    uint64    `gorm:"index;not null" json:"projectId"`
	DeploymentID uint64    `gorm:"index;not null" json:"deploymentId"`
	Name         string    `gorm:"size:160;not null" json:"name"`
	Metric       string    `gorm:"size:64;index;not null" json:"metric"`
	Operator     string    `gorm:"size:8;not null" json:"operator"`
	Threshold    float64   `json:"threshold"`
	Enabled      bool      `gorm:"index;not null" json:"enabled"`
	CreatedBy    uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt    time.Time `json:"createTime"`
	UpdatedAt    time.Time `json:"updateTime"`
}

type AlertEvent struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	TenantID       uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID      uint64     `gorm:"index;not null" json:"projectId"`
	DeploymentID   uint64     `gorm:"index;not null" json:"deploymentId"`
	AlertRuleID    uint64     `gorm:"index;not null" json:"alertRuleId"`
	Status         string     `gorm:"size:24;index;not null" json:"status"`
	MetricValue    float64    `json:"metricValue"`
	Message        string     `gorm:"size:1024;not null" json:"message"`
	AcknowledgedBy uint64     `gorm:"index" json:"acknowledgedBy"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt"`
	CreatedAt      time.Time  `json:"createTime"`
}

func (Deployment) TableName() string         { return "ai_deployment" }
func (DeploymentRevision) TableName() string { return "ai_deployment_revision" }
func (InferenceTrace) TableName() string     { return "ai_inference_trace" }
func (AlertRule) TableName() string          { return "ai_alert_rule" }
func (AlertEvent) TableName() string         { return "ai_alert_event" }
