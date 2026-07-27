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
	CaptureManual   bool      `gorm:"not null;default:true" json:"captureManual"`
	CaptureDrift    bool      `gorm:"not null;default:true" json:"captureDrift"`
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

type FeedbackBenefitEvaluation struct {
	ID                       uint64    `gorm:"primaryKey" json:"id"`
	TenantID                 uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID                uint64    `gorm:"index;not null" json:"projectId"`
	FeedbackBatchID          uint64    `gorm:"uniqueIndex;not null" json:"feedbackBatchId"`
	BaselineModelVersionID   uint64    `gorm:"index;not null" json:"baselineModelVersionId"`
	CandidateModelVersionID  uint64    `gorm:"index;not null" json:"candidateModelVersionId"`
	BaselineEvaluationRunID  uint64    `gorm:"index;not null" json:"baselineEvaluationRunId"`
	CandidateEvaluationRunID uint64    `gorm:"index;not null" json:"candidateEvaluationRunId"`
	BaselineDeploymentID     uint64    `gorm:"index" json:"baselineDeploymentId"`
	CandidateDeploymentID    uint64    `gorm:"index" json:"candidateDeploymentId"`
	Slices                   string    `gorm:"type:json;not null" json:"slices"`
	BaselineQuality          string    `gorm:"type:json;not null" json:"baselineQuality"`
	CandidateQuality         string    `gorm:"type:json;not null" json:"candidateQuality"`
	BaselineProduction       string    `gorm:"type:json;not null" json:"baselineProduction"`
	CandidateProduction      string    `gorm:"type:json;not null" json:"candidateProduction"`
	Deltas                   string    `gorm:"type:json;not null" json:"deltas"`
	Conclusion               string    `gorm:"size:32;index;not null" json:"conclusion"`
	EvidenceSnapshot         string    `gorm:"type:longtext;not null" json:"evidenceSnapshot"`
	EvidenceSHA256           string    `gorm:"size:64;not null" json:"evidenceSha256"`
	CreatedBy                uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt                time.Time `json:"createTime"`
	UpdatedAt                time.Time `json:"updateTime"`
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
	PrivacyReviewedBy    uint64     `gorm:"index" json:"privacyReviewedBy"`
	PrivacyReviewedAt    *time.Time `json:"privacyReviewedAt"`
	CreatedAt            time.Time  `json:"createTime"`
	UpdatedAt            time.Time  `json:"updateTime"`
}

func (FeedbackPolicy) TableName() string { return "ai_feedback_policy" }
func (FeedbackSample) TableName() string { return "ai_feedback_sample" }
func (FeedbackBatch) TableName() string  { return "ai_feedback_batch" }
func (FeedbackBenefitEvaluation) TableName() string {
	return "ai_feedback_benefit_evaluation"
}
