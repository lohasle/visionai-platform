package visionai

import "time"

type FeedbackPolicy struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	TenantID        uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID       uint64    `gorm:"uniqueIndex;not null" json:"projectId"`
	Enabled         bool      `gorm:"index;not null" json:"enabled"`
	RandomRate      float64   `json:"randomRate"`
	ConfidenceBelow float64   `json:"confidenceBelow"`
	CaptureEmpty    bool      `json:"captureEmpty"`
	CaptureErrors   bool      `json:"captureErrors"`
	DailyLimit      int       `gorm:"not null" json:"dailyLimit"`
	RetentionDays   int       `gorm:"not null" json:"retentionDays"`
	RedactionPolicy string    `gorm:"type:json;not null" json:"redactionPolicy"`
	SensitiveReview bool      `gorm:"not null" json:"sensitiveReview"`
	CreatedBy       uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt       time.Time `json:"createTime"`
	UpdatedAt       time.Time `json:"updateTime"`
}

type FeedbackSample struct {
	ID                   uint64     `gorm:"primaryKey" json:"id"`
	TenantID             uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID            uint64     `gorm:"index;not null" json:"projectId"`
	PolicyID             uint64     `gorm:"index;not null" json:"policyId"`
	InferenceTraceID     uint64     `gorm:"uniqueIndex;not null" json:"inferenceTraceId"`
	DeploymentRevisionID uint64     `gorm:"index;not null" json:"deploymentRevisionId"`
	ModelVersionID       uint64     `gorm:"index;not null" json:"modelVersionId"`
	AssetID              uint64     `gorm:"index;not null" json:"assetId"`
	Reason               string     `gorm:"size:64;index;not null" json:"reason"`
	PerceptualHash       string     `gorm:"size:64;index;not null" json:"perceptualHash"`
	Status               string     `gorm:"size:24;index;not null" json:"status"`
	ExpiresAt            time.Time  `gorm:"index;not null" json:"expiresAt"`
	BatchID              uint64     `gorm:"index" json:"batchId"`
	ReviewedBy           uint64     `gorm:"index" json:"reviewedBy"`
	ReviewedAt           *time.Time `json:"reviewedAt"`
	CreatedAt            time.Time  `json:"createTime"`
}

type FeedbackBatch struct {
	ID                   uint64     `gorm:"primaryKey" json:"id"`
	TenantID             uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID            uint64     `gorm:"index;not null" json:"projectId"`
	Name                 string     `gorm:"size:200;not null" json:"name"`
	Status               string     `gorm:"size:24;index;not null" json:"status"`
	SampleCount          int        `gorm:"not null" json:"sampleCount"`
	AnnotationTaskID     uint64     `gorm:"index" json:"annotationTaskId"`
	AnnotationRevisionID uint64     `gorm:"index" json:"annotationRevisionId"`
	DatasetVersionID     uint64     `gorm:"index" json:"datasetVersionId"`
	ModelVersionID       uint64     `gorm:"index" json:"modelVersionId"`
	CreatedBy            uint64     `gorm:"index;not null" json:"createdBy"`
	ReviewedBy           uint64     `gorm:"index" json:"reviewedBy"`
	ReviewedAt           *time.Time `json:"reviewedAt"`
	CreatedAt            time.Time  `json:"createTime"`
	UpdatedAt            time.Time  `json:"updateTime"`
}

func (FeedbackPolicy) TableName() string { return "ai_feedback_policy" }
func (FeedbackSample) TableName() string { return "ai_feedback_sample" }
func (FeedbackBatch) TableName() string  { return "ai_feedback_batch" }
