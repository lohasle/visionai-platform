package visionai

import "time"

type Model struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"uniqueIndex:uk_model;not null" json:"tenantId"`
	ProjectID   uint64    `gorm:"uniqueIndex:uk_model;not null" json:"projectId"`
	Name        string    `gorm:"size:200;uniqueIndex:uk_model;not null" json:"name"`
	AIType      string    `gorm:"size:40;index;not null" json:"aiType"`
	Description string    `gorm:"size:2048" json:"description"`
	CreatedBy   uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

type ModelVersion struct {
	ID                    uint64     `gorm:"primaryKey" json:"id"`
	TenantID              uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID             uint64     `gorm:"index;not null" json:"projectId"`
	ModelID               uint64     `gorm:"uniqueIndex:uk_model_version;not null" json:"modelId"`
	VersionNo             int        `gorm:"uniqueIndex:uk_model_version;not null" json:"versionNo"`
	SemanticVersion       string     `gorm:"size:32;index;not null" json:"semanticVersion"`
	SourceType            string     `gorm:"size:32;index;not null" json:"sourceType"`
	TrainingRunID         uint64     `gorm:"index" json:"trainingRunId"`
	EvaluationRunID       uint64     `gorm:"index" json:"evaluationRunId"`
	DatasetVersionID      uint64     `gorm:"index;not null" json:"datasetVersionId"`
	Status                string     `gorm:"size:32;index;not null" json:"status"`
	ModelCardURI          string     `gorm:"size:1024;not null" json:"modelCardUri"`
	ModelCardSHA256       string     `gorm:"size:64;not null" json:"modelCardSha256"`
	SupplyChainURI        string     `gorm:"size:1024;not null" json:"supplyChainUri"`
	SupplyChainSHA256     string     `gorm:"size:64;not null" json:"supplyChainSha256"`
	LicenseDecision       string     `gorm:"size:32;index;not null" json:"licenseDecision"`
	PreannotationApproved bool       `gorm:"index;not null;default:false" json:"preannotationApproved"`
	CreatedBy             uint64     `gorm:"index;not null" json:"createdBy"`
	ApprovedBy            uint64     `gorm:"index" json:"approvedBy"`
	ApprovedAt            *time.Time `json:"approvedAt"`
	CreatedAt             time.Time  `json:"createTime"`
	UpdatedAt             time.Time  `json:"updateTime"`
}

type ModelArtifact struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	TenantID       uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID      uint64    `gorm:"index;not null" json:"projectId"`
	ModelVersionID uint64    `gorm:"uniqueIndex:uk_model_artifact_name;not null" json:"modelVersionId"`
	Name           string    `gorm:"size:256;uniqueIndex:uk_model_artifact_name;not null" json:"name"`
	Format         string    `gorm:"size:32;index;not null" json:"format"`
	URI            string    `gorm:"size:1024;not null" json:"uri"`
	SHA256         string    `gorm:"size:64;index;not null" json:"sha256"`
	Size           int64     `gorm:"not null" json:"size"`
	MediaType      string    `gorm:"size:128" json:"mediaType"`
	CreatedAt      time.Time `json:"createTime"`
}

type ModelLicenseDeclaration struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	TenantID       uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID      uint64    `gorm:"index;not null" json:"projectId"`
	ModelVersionID uint64    `gorm:"uniqueIndex:uk_model_license_component;not null" json:"modelVersionId"`
	ComponentType  string    `gorm:"size:32;uniqueIndex:uk_model_license_component;not null" json:"componentType"`
	ComponentName  string    `gorm:"size:256;not null" json:"componentName"`
	LicenseID      string    `gorm:"size:128;not null" json:"licenseId"`
	SourceURI      string    `gorm:"size:1024" json:"sourceUri"`
	UseDeclaration string    `gorm:"size:2048;not null" json:"useDeclaration"`
	Decision       string    `gorm:"size:32;index;not null" json:"decision"`
	ReviewedBy     uint64    `gorm:"index" json:"reviewedBy"`
	CreatedAt      time.Time `json:"createTime"`
	UpdatedAt      time.Time `json:"updateTime"`
}

type ApprovalTemplate struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TenantID          uint64    `gorm:"uniqueIndex:uk_approval_template;not null" json:"tenantId"`
	Name              string    `gorm:"size:160;uniqueIndex:uk_approval_template;not null" json:"name"`
	ApprovalType      string    `gorm:"size:32;index;not null" json:"approvalType"`
	TargetEnvironment string    `gorm:"size:64;index;not null" json:"targetEnvironment"`
	Steps             string    `gorm:"type:json;not null" json:"steps"`
	AllowSelfApproval bool      `gorm:"not null;default:false" json:"allowSelfApproval"`
	Enabled           bool      `gorm:"index;not null;default:true" json:"enabled"`
	CreatedBy         uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt         time.Time `json:"createTime"`
	UpdatedAt         time.Time `json:"updateTime"`
}

type ApprovalRequest struct {
	ID                 uint64     `gorm:"primaryKey" json:"id"`
	TenantID           uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID          uint64     `gorm:"index;not null" json:"projectId"`
	ModelVersionID     uint64     `gorm:"index;not null" json:"modelVersionId"`
	ApprovalType       string     `gorm:"size:32;index;not null" json:"approvalType"`
	TargetEnvironment  string     `gorm:"size:64;index;not null" json:"targetEnvironment"`
	Status             string     `gorm:"size:24;index;not null" json:"status"`
	EvidenceSnapshot   string     `gorm:"type:longtext;not null" json:"evidenceSnapshot"`
	EvidenceSHA256     string     `gorm:"size:64;not null" json:"evidenceSha256"`
	TemplateID         uint64     `gorm:"index" json:"templateId"`
	TemplateSnapshot   string     `gorm:"type:longtext" json:"templateSnapshot"`
	TemplateSHA256     string     `gorm:"size:64" json:"templateSha256"`
	InputFingerprint   string     `gorm:"size:64;index" json:"inputFingerprint"`
	CurrentStep        int        `gorm:"not null;default:1" json:"currentStep"`
	TotalSteps         int        `gorm:"not null;default:1" json:"totalSteps"`
	InvalidatedAt      *time.Time `json:"invalidatedAt"`
	InvalidationReason string     `gorm:"size:1024" json:"invalidationReason"`
	SubmittedBy        uint64     `gorm:"index;not null" json:"submittedBy"`
	SubmittedAt        time.Time  `json:"submittedAt"`
	DecidedBy          uint64     `gorm:"index" json:"decidedBy"`
	DecidedAt          *time.Time `json:"decidedAt"`
	CreatedAt          time.Time  `json:"createTime"`
	UpdatedAt          time.Time  `json:"updateTime"`
}

type ApprovalStepDecision struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TenantID          uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID         uint64    `gorm:"index;not null" json:"projectId"`
	ApprovalRequestID uint64    `gorm:"uniqueIndex:uk_approval_step;not null" json:"approvalRequestId"`
	StepNo            int       `gorm:"uniqueIndex:uk_approval_step;not null" json:"stepNo"`
	StepName          string    `gorm:"size:160;not null" json:"stepName"`
	RequiredRole      string    `gorm:"size:64;not null" json:"requiredRole"`
	Decision          string    `gorm:"size:24;index;not null" json:"decision"`
	Comment           string    `gorm:"size:2048;not null" json:"comment"`
	EvidenceSHA256    string    `gorm:"size:64;not null" json:"evidenceSha256"`
	DecidedBy         uint64    `gorm:"index;not null" json:"decidedBy"`
	CreatedAt         time.Time `json:"createTime"`
}

type ApprovalDecision struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TenantID          uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID         uint64    `gorm:"index;not null" json:"projectId"`
	ApprovalRequestID uint64    `gorm:"uniqueIndex;not null" json:"approvalRequestId"`
	Decision          string    `gorm:"size:24;index;not null" json:"decision"`
	Comment           string    `gorm:"size:2048;not null" json:"comment"`
	EvidenceSHA256    string    `gorm:"size:64;not null" json:"evidenceSha256"`
	DecidedBy         uint64    `gorm:"index;not null" json:"decidedBy"`
	CreatedAt         time.Time `json:"createTime"`
}

func (Model) TableName() string                   { return "ai_model" }
func (ModelVersion) TableName() string            { return "ai_model_version" }
func (ModelArtifact) TableName() string           { return "ai_model_artifact" }
func (ModelLicenseDeclaration) TableName() string { return "ai_model_license_declaration" }
func (ApprovalTemplate) TableName() string        { return "ai_approval_template" }
func (ApprovalRequest) TableName() string         { return "ai_approval_request" }
func (ApprovalDecision) TableName() string        { return "ai_approval_decision" }
func (ApprovalStepDecision) TableName() string    { return "ai_approval_step_decision" }
