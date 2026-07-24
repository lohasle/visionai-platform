package visionai

import "time"

type PlatformJob struct {
	ID           uint64     `gorm:"primaryKey" json:"id"`
	TenantID     uint64     `gorm:"index:idx_job_scope;not null" json:"tenantId"`
	ProjectID    uint64     `gorm:"index:idx_job_scope;not null;default:0" json:"projectId"`
	JobType      string     `gorm:"size:64;index;not null" json:"jobType"`
	ResourceType string     `gorm:"size:64;index;not null" json:"resourceType"`
	ResourceID   uint64     `gorm:"index;not null;default:0" json:"resourceId"`
	Status       JobStatus  `gorm:"size:24;index;not null" json:"status"`
	Stage        string     `gorm:"size:128" json:"stage"`
	Progress     int        `gorm:"not null;default:0" json:"progress"`
	Processed    int64      `gorm:"not null;default:0" json:"processed"`
	Total        int64      `gorm:"not null;default:0" json:"total"`
	RetryCount   int        `gorm:"not null;default:0" json:"retryCount"`
	MaxRetries   int        `gorm:"not null;default:3" json:"maxRetries"`
	TraceID      string     `gorm:"size:64;index;not null" json:"traceId"`
	Idempotency  string     `gorm:"size:128;uniqueIndex:uk_job_idempotency;not null" json:"idempotencyKey"`
	ErrorCode    string     `gorm:"size:128" json:"errorCode"`
	ErrorMessage string     `gorm:"size:1024" json:"errorMessage"`
	Remediation  string     `gorm:"size:1024" json:"remediation"`
	CancelAt     *time.Time `json:"cancelRequestedAt"`
	StartedAt    *time.Time `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
	CreatedBy    uint64     `gorm:"index;not null" json:"createdBy"`
	CreatedAt    time.Time  `json:"createTime"`
	UpdatedAt    time.Time  `json:"updateTime"`
}

type JobAttempt struct {
	ID           uint64     `gorm:"primaryKey" json:"id"`
	TenantID     uint64     `gorm:"index;not null" json:"tenantId"`
	JobID        uint64     `gorm:"uniqueIndex:uk_job_attempt;not null" json:"jobId"`
	Attempt      int        `gorm:"uniqueIndex:uk_job_attempt;not null" json:"attempt"`
	Status       JobStatus  `gorm:"size:24;index;not null" json:"status"`
	Provider     string     `gorm:"size:64" json:"provider"`
	ExternalID   string     `gorm:"size:256" json:"externalId"`
	LogURI       string     `gorm:"size:1024" json:"logUri"`
	ErrorCode    string     `gorm:"size:128" json:"errorCode"`
	ErrorMessage string     `gorm:"size:1024" json:"errorMessage"`
	StartedAt    *time.Time `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
	CreatedAt    time.Time  `json:"createTime"`
	UpdatedAt    time.Time  `json:"updateTime"`
}

type OutboxEvent struct {
	ID            uint64     `gorm:"primaryKey" json:"id"`
	TenantID      uint64     `gorm:"index;not null" json:"tenantId"`
	EventID       string     `gorm:"size:64;uniqueIndex;not null" json:"eventId"`
	EventType     string     `gorm:"size:128;index;not null" json:"eventType"`
	AggregateType string     `gorm:"size:64;index;not null" json:"aggregateType"`
	AggregateID   uint64     `gorm:"index;not null" json:"aggregateId"`
	Payload       string     `gorm:"type:json;not null" json:"payload"`
	Status        string     `gorm:"size:24;index;not null;default:NEW" json:"status"`
	Attempts      int        `gorm:"not null;default:0" json:"attempts"`
	NextAttemptAt *time.Time `gorm:"index" json:"nextAttemptAt"`
	PublishedAt   *time.Time `json:"publishedAt"`
	CreatedAt     time.Time  `json:"createTime"`
	UpdatedAt     time.Time  `json:"updateTime"`
}

type InboxEvent struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"index;not null" json:"tenantId"`
	Consumer    string    `gorm:"size:128;uniqueIndex:uk_inbox_consumer_event;not null" json:"consumer"`
	EventID     string    `gorm:"size:64;uniqueIndex:uk_inbox_consumer_event;not null" json:"eventId"`
	ProcessedAt time.Time `gorm:"not null" json:"processedAt"`
	CreatedAt   time.Time `json:"createTime"`
}

func (PlatformJob) TableName() string { return "ai_platform_job" }
func (JobAttempt) TableName() string  { return "ai_job_attempt" }
func (OutboxEvent) TableName() string { return "ai_outbox_event" }
func (InboxEvent) TableName() string  { return "ai_inbox_event" }
