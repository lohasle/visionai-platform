package visionai

import "time"

type EvaluationSuite struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	TenantID         uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID        uint64    `gorm:"index;not null" json:"projectId"`
	Name             string    `gorm:"size:200;not null" json:"name"`
	DatasetVersionID uint64    `gorm:"index;not null" json:"datasetVersionId"`
	Slices           string    `gorm:"type:json;not null" json:"slices"`
	Thresholds       string    `gorm:"type:json;not null" json:"thresholds"`
	GatePolicy       string    `gorm:"size:32;not null" json:"gatePolicy"`
	AutoTrigger      bool      `gorm:"index;not null;default:false" json:"autoTrigger"`
	EvaluatorVersion string    `gorm:"size:64;not null" json:"evaluatorVersion"`
	CreatedBy        uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt        time.Time `json:"createTime"`
	UpdatedAt        time.Time `json:"updateTime"`
}

type EvaluationRun struct {
	ID              uint64     `gorm:"primaryKey" json:"id"`
	TenantID        uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID       uint64     `gorm:"index;not null" json:"projectId"`
	SuiteID         uint64     `gorm:"index;not null" json:"suiteId"`
	TrainingRunID   uint64     `gorm:"index;not null" json:"trainingRunId"`
	BaselineRunID   uint64     `gorm:"index" json:"baselineRunId"`
	Status          string     `gorm:"size:24;index;not null" json:"status"`
	GateDecision    string     `gorm:"size:32;index;not null" json:"gateDecision"`
	GateEvidence    string     `gorm:"type:json;not null" json:"gateEvidence"`
	Summary         string     `gorm:"type:json;not null" json:"summary"`
	FiftyOneDataset string     `gorm:"size:256" json:"fiftyOneDataset"`
	ErrorCode       string     `gorm:"size:128" json:"errorCode"`
	ErrorMessage    string     `gorm:"size:2048" json:"errorMessage"`
	CreatedBy       uint64     `gorm:"index;not null" json:"createdBy"`
	StartedAt       *time.Time `json:"startedAt"`
	FinishedAt      *time.Time `json:"finishedAt"`
	CreatedAt       time.Time  `json:"createTime"`
	UpdatedAt       time.Time  `json:"updateTime"`
}

type EvaluationMetric struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	TenantID        uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID       uint64    `gorm:"index;not null" json:"projectId"`
	EvaluationRunID uint64    `gorm:"uniqueIndex:uk_eval_metric;not null" json:"evaluationRunId"`
	Slice           string    `gorm:"size:128;uniqueIndex:uk_eval_metric;not null" json:"slice"`
	Name            string    `gorm:"size:128;uniqueIndex:uk_eval_metric;not null" json:"name"`
	Value           float64   `json:"value"`
	CreatedAt       time.Time `json:"createTime"`
}

type EvaluationSample struct {
	ID              uint64     `gorm:"primaryKey" json:"id"`
	TenantID        uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID       uint64     `gorm:"index;not null" json:"projectId"`
	EvaluationRunID uint64     `gorm:"uniqueIndex:uk_eval_sample;not null" json:"evaluationRunId"`
	AssetID         uint64     `gorm:"uniqueIndex:uk_eval_sample;index;not null" json:"assetId"`
	Split           string     `gorm:"size:16;index;not null" json:"split"`
	Slice           string     `gorm:"size:128;index;not null" json:"slice"`
	CategoryLabels  string     `gorm:"type:json;not null" json:"categoryLabels"`
	TargetSize      string     `gorm:"size:24;index" json:"targetSize"`
	Scene           string     `gorm:"size:256;index" json:"scene"`
	Device          string     `gorm:"size:256;index" json:"device"`
	CapturedAt      *time.Time `gorm:"index" json:"capturedAt"`
	ErrorType       string     `gorm:"size:24;index;not null" json:"errorType"`
	Confidence      float64    `json:"confidence"`
	IoU             float64    `json:"iou"`
	GTCount         int        `json:"gtCount"`
	PredictionCount int        `json:"predictionCount"`
	LatencyMS       float64    `json:"latencyMs"`
	CreatedAt       time.Time  `json:"createTime"`
}

type EvaluationSavedSlice struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	TenantID        uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID       uint64    `gorm:"index;not null" json:"projectId"`
	EvaluationRunID uint64    `gorm:"index;not null" json:"evaluationRunId"`
	Name            string    `gorm:"size:160;not null" json:"name"`
	Filter          string    `gorm:"type:json;not null" json:"filter"`
	SampleCount     int       `gorm:"not null" json:"sampleCount"`
	CreatedBy       uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt       time.Time `json:"createTime"`
}

func (EvaluationSuite) TableName() string      { return "ai_evaluation_suite" }
func (EvaluationRun) TableName() string        { return "ai_evaluation_run" }
func (EvaluationMetric) TableName() string     { return "ai_evaluation_metric" }
func (EvaluationSample) TableName() string     { return "ai_evaluation_sample" }
func (EvaluationSavedSlice) TableName() string { return "ai_evaluation_saved_slice" }
