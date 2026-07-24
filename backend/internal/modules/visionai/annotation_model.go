package visionai

import "time"

type AnnotationStatus string

const (
	AnnotationDraft         AnnotationStatus = "DRAFT"
	AnnotationPreparing     AnnotationStatus = "PREPARING"
	AnnotationPreannotating AnnotationStatus = "PREANNOTATING"
	AnnotationReady         AnnotationStatus = "READY"
	AnnotationAnnotating    AnnotationStatus = "ANNOTATING"
	AnnotationReviewing     AnnotationStatus = "REVIEWING"
	AnnotationApproved      AnnotationStatus = "APPROVED"
	AnnotationRejected      AnnotationStatus = "REJECTED"
	AnnotationExporting     AnnotationStatus = "EXPORTING"
	AnnotationClosed        AnnotationStatus = "CLOSED"
	AnnotationFailed        AnnotationStatus = "FAILED"
	AnnotationCancelled     AnnotationStatus = "CANCELLED"
)

type AnnotationTask struct {
	ID                 uint64           `gorm:"primaryKey" json:"id"`
	TenantID           uint64           `gorm:"index;not null" json:"tenantId"`
	ProjectID          uint64           `gorm:"index;not null" json:"projectId"`
	Name               string           `gorm:"size:160;not null" json:"name"`
	TaskType           string           `gorm:"size:40;not null" json:"taskType"`
	CollectionID       uint64           `gorm:"index;not null" json:"collectionId"`
	OntologyVersion    string           `gorm:"size:80;not null" json:"ontologyVersion"`
	Labels             string           `gorm:"type:json;not null" json:"labels"`
	AnnotatorIDs       string           `gorm:"type:json;not null" json:"annotatorIds"`
	ReviewerIDs        string           `gorm:"type:json;not null" json:"reviewerIds"`
	Status             AnnotationStatus `gorm:"size:24;index;not null" json:"status"`
	Progress           int              `gorm:"not null;default:0" json:"progress"`
	ExternalBindingID  uint64           `gorm:"index" json:"externalBindingId"`
	CurrentRevisionID  uint64           `gorm:"index" json:"currentRevisionId"`
	PreannotationRunID uint64           `gorm:"index" json:"preannotationRunId"`
	PlanStartAt        *time.Time       `json:"planStartAt"`
	PlanEndAt          *time.Time       `json:"planEndAt"`
	RejectionCode      string           `gorm:"size:64" json:"rejectionCode"`
	RejectionReason    string           `gorm:"size:1024" json:"rejectionReason"`
	ErrorCode          string           `gorm:"size:64" json:"errorCode"`
	ErrorMessage       string           `gorm:"size:2048" json:"errorMessage"`
	CreatedBy          uint64           `gorm:"index;not null" json:"createdBy"`
	CreatedAt          time.Time        `json:"createTime"`
	UpdatedAt          time.Time        `json:"updateTime"`
}

type AnnotationRevision struct {
	ID               uint64    `gorm:"primaryKey" json:"id"`
	TenantID         uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID        uint64    `gorm:"index;not null" json:"projectId"`
	AnnotationTaskID uint64    `gorm:"uniqueIndex:uk_annotation_revision;not null" json:"annotationTaskId"`
	RevisionNo       int       `gorm:"uniqueIndex:uk_annotation_revision;not null" json:"revisionNo"`
	SnapshotURI      string    `gorm:"size:1024;not null" json:"snapshotUri"`
	ObjectKey        string    `gorm:"size:1024;not null" json:"-"`
	Format           string    `gorm:"size:64;not null" json:"format"`
	Checksum         string    `gorm:"size:64;index;not null" json:"checksum"`
	CategoryMapping  string    `gorm:"type:json;not null" json:"categoryMapping"`
	AnnotationCount  int64     `gorm:"not null" json:"annotationCount"`
	ApprovedBy       uint64    `gorm:"index;not null" json:"approvedBy"`
	CreatedAt        time.Time `json:"createTime"`
}

type ExternalResourceBinding struct {
	ID           uint64     `gorm:"primaryKey" json:"id"`
	TenantID     uint64     `gorm:"uniqueIndex:uk_external_binding;not null" json:"tenantId"`
	ProviderType string     `gorm:"size:32;uniqueIndex:uk_external_binding;not null" json:"providerType"`
	InstanceID   string     `gorm:"size:80;uniqueIndex:uk_external_binding;not null" json:"instanceId"`
	InternalType string     `gorm:"size:64;uniqueIndex:uk_external_binding;not null" json:"internalType"`
	InternalID   uint64     `gorm:"uniqueIndex:uk_external_binding;not null" json:"internalId"`
	ExternalType string     `gorm:"size:64;not null" json:"externalType"`
	ExternalID   string     `gorm:"size:128;index;not null" json:"externalId"`
	ExternalURL  string     `gorm:"size:1024" json:"externalUrl"`
	SyncCursor   string     `gorm:"size:256" json:"syncCursor"`
	LastSyncAt   *time.Time `json:"lastSyncAt"`
	CreatedAt    time.Time  `json:"createTime"`
	UpdatedAt    time.Time  `json:"updateTime"`
}

type CVATUserMapping struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	TenantID       uint64     `gorm:"uniqueIndex:uk_cvat_user_mapping;not null" json:"tenantId"`
	PlatformUserID uint64     `gorm:"uniqueIndex:uk_cvat_user_mapping;not null" json:"platformUserId"`
	CVATUserID     int64      `gorm:"not null" json:"cvatUserId"`
	CVATUsername   string     `gorm:"size:128;not null" json:"cvatUsername"`
	Active         bool       `gorm:"index;not null;default:true" json:"active"`
	VerifiedAt     *time.Time `json:"verifiedAt"`
	CreatedAt      time.Time  `json:"createTime"`
	UpdatedAt      time.Time  `json:"updateTime"`
}

type PreannotationRun struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TenantID          uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID         uint64    `gorm:"index;not null" json:"projectId"`
	AnnotationTaskID  uint64    `gorm:"index;not null" json:"annotationTaskId"`
	ModelVersionID    uint64    `gorm:"index;not null" json:"modelVersionId"`
	Status            string    `gorm:"size:24;index;not null" json:"status"`
	Parameters        string    `gorm:"type:json;not null" json:"parameters"`
	IdempotencyKey    string    `gorm:"size:128;uniqueIndex;not null" json:"idempotencyKey"`
	ProposedCount     int64     `json:"proposedCount"`
	AcceptedCount     int64     `json:"acceptedCount"`
	DeletedCount      int64     `json:"deletedCount"`
	ModifiedCount     int64     `json:"modifiedCount"`
	AddedCount        int64     `json:"addedCount"`
	CorrectionSeconds int64     `json:"correctionSeconds"`
	CreatedAt         time.Time `json:"createTime"`
	UpdatedAt         time.Time `json:"updateTime"`
}

func (AnnotationTask) TableName() string          { return "ai_annotation_task" }
func (AnnotationRevision) TableName() string      { return "ai_annotation_revision" }
func (ExternalResourceBinding) TableName() string { return "ai_external_resource_binding" }
func (CVATUserMapping) TableName() string         { return "ai_cvat_user_mapping" }
func (PreannotationRun) TableName() string        { return "ai_preannotation_run" }
