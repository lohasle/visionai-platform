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
	ID                uint64     `gorm:"primaryKey" json:"id"`
	TenantID          uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID         uint64     `gorm:"index;not null" json:"projectId"`
	DeploymentID      uint64     `gorm:"uniqueIndex:uk_deployment_revision;not null" json:"deploymentId"`
	RevisionNo        int        `gorm:"uniqueIndex:uk_deployment_revision;not null" json:"revisionNo"`
	ModelVersionID    uint64     `gorm:"index;not null" json:"modelVersionId"`
	SourceRevisionID  uint64     `gorm:"index" json:"sourceRevisionId"`
	Status            string     `gorm:"size:32;index;not null" json:"status"`
	Config            string     `gorm:"type:json;not null" json:"config"`
	ArtifactURI       string     `gorm:"size:1024;not null" json:"artifactUri"`
	ArtifactSHA256    string     `gorm:"size:64;not null" json:"artifactSha256"`
	ChangeReason      string     `gorm:"size:1024" json:"changeReason"`
	ApprovalRequestID uint64     `gorm:"index" json:"approvalRequestId"`
	CreatedBy         uint64     `gorm:"index;not null" json:"createdBy"`
	ActivatedAt       *time.Time `json:"activatedAt"`
	CreatedAt         time.Time  `json:"createTime"`
}

type InferenceTrace struct {
	ID                    uint64    `gorm:"primaryKey" json:"id"`
	TenantID              uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID             uint64    `gorm:"index;not null" json:"projectId"`
	DeploymentID          uint64    `gorm:"index;not null" json:"deploymentId"`
	DeploymentRevisionID  uint64    `gorm:"index;not null" json:"deploymentRevisionId"`
	ModelVersionID        uint64    `gorm:"index;not null" json:"modelVersionId"`
	TraceID               string    `gorm:"size:64;uniqueIndex;not null" json:"traceId"`
	AssetID               uint64    `gorm:"index" json:"assetId"`
	SourceType            string    `gorm:"size:24;index;not null;default:ASSET" json:"sourceType"`
	SourceName            string    `gorm:"size:512" json:"sourceName"`
	SourceSHA256          string    `gorm:"size:64;index" json:"sourceSha256"`
	TestMode              string    `gorm:"size:24;index;not null;default:ONLINE" json:"testMode"`
	ExpectedLabel         string    `gorm:"size:160" json:"expectedLabel"`
	MinimumConfidence     float64   `json:"minimumConfidence"`
	RegressionStatus      string    `gorm:"size:24;index;not null;default:NOT_ASSERTED" json:"regressionStatus"`
	MatchedDetectionCount int       `gorm:"not null;default:0" json:"matchedDetectionCount"`
	Status                string    `gorm:"size:24;index;not null" json:"status"`
	LatencyMS             float64   `gorm:"index" json:"latencyMs"`
	DetectionCount        int       `gorm:"index" json:"detectionCount"`
	MeanConfidence        float64   `gorm:"index" json:"meanConfidence"`
	Result                string    `gorm:"type:json;not null" json:"result"`
	ErrorMessage          string    `gorm:"size:2048" json:"errorMessage"`
	CreatedAt             time.Time `gorm:"index" json:"createTime"`
}

type AlertRule struct {
	ID                  uint64     `gorm:"primaryKey" json:"id"`
	TenantID            uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID           uint64     `gorm:"index;not null" json:"projectId"`
	DeploymentID        uint64     `gorm:"index;not null" json:"deploymentId"`
	Name                string     `gorm:"size:160;not null" json:"name"`
	Metric              string     `gorm:"size:64;index;not null" json:"metric"`
	Operator            string     `gorm:"size:8;not null" json:"operator"`
	Threshold           float64    `json:"threshold"`
	DurationSeconds     int        `gorm:"not null;default:0" json:"durationSeconds"`
	Enabled             bool       `gorm:"index;not null" json:"enabled"`
	NotificationChannel string     `gorm:"size:32;not null;default:IN_APP" json:"notificationChannel"`
	Recipients          string     `gorm:"type:json" json:"recipients"`
	PendingSince        *time.Time `gorm:"index" json:"pendingSince"`
	SilencedUntil       *time.Time `gorm:"index" json:"silencedUntil"`
	SilenceReason       string     `gorm:"size:1024" json:"silenceReason"`
	CreatedBy           uint64     `gorm:"index;not null" json:"createdBy"`
	CreatedAt           time.Time  `json:"createTime"`
	UpdatedAt           time.Time  `json:"updateTime"`
}

type AlertEvent struct {
	ID                  uint64     `gorm:"primaryKey" json:"id"`
	TenantID            uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID           uint64     `gorm:"index;not null" json:"projectId"`
	DeploymentID        uint64     `gorm:"index;not null" json:"deploymentId"`
	AlertRuleID         uint64     `gorm:"index;not null" json:"alertRuleId"`
	Status              string     `gorm:"size:24;index;not null" json:"status"`
	MetricValue         float64    `json:"metricValue"`
	Message             string     `gorm:"size:1024;not null" json:"message"`
	AcknowledgedBy      uint64     `gorm:"index" json:"acknowledgedBy"`
	AcknowledgedAt      *time.Time `json:"acknowledgedAt"`
	NotificationChannel string     `gorm:"size:32;not null;default:IN_APP" json:"notificationChannel"`
	Recipients          string     `gorm:"type:json" json:"recipients"`
	NotifiedAt          *time.Time `json:"notifiedAt"`
	ResolvedBy          uint64     `gorm:"index" json:"resolvedBy"`
	ResolvedAt          *time.Time `json:"resolvedAt"`
	Resolution          string     `gorm:"size:2048" json:"resolution"`
	RecoveryEvent       bool       `gorm:"index;not null;default:false" json:"recoveryEvent"`
	RecoveryOfID        uint64     `gorm:"index" json:"recoveryOfId"`
	CreatedAt           time.Time  `json:"createTime"`
}

type DriftBaseline struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	TenantID        uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID       uint64    `gorm:"index;not null" json:"projectId"`
	DeploymentID    uint64    `gorm:"uniqueIndex;not null" json:"deploymentId"`
	ModelVersionID  uint64    `gorm:"index;not null" json:"modelVersionId"`
	WindowMinutes   int       `gorm:"not null" json:"windowMinutes"`
	SampleCount     int       `gorm:"not null" json:"sampleCount"`
	MetricsSnapshot string    `gorm:"type:json;not null" json:"metricsSnapshot"`
	ClassSnapshot   string    `gorm:"type:json;not null" json:"classSnapshot"`
	BaselineFrom    time.Time `json:"baselineFrom"`
	BaselineTo      time.Time `json:"baselineTo"`
	CreatedBy       uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt       time.Time `json:"createTime"`
	UpdatedAt       time.Time `json:"updateTime"`
}

func (Deployment) TableName() string         { return "ai_deployment" }
func (DeploymentRevision) TableName() string { return "ai_deployment_revision" }
func (InferenceTrace) TableName() string     { return "ai_inference_trace" }
func (AlertRule) TableName() string          { return "ai_alert_rule" }
func (AlertEvent) TableName() string         { return "ai_alert_event" }
func (DriftBaseline) TableName() string      { return "ai_drift_baseline" }
