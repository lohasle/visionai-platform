package visionai

import "time"

type ProjectStatus string

const (
	ProjectDraft     ProjectStatus = "DRAFT"
	ProjectActive    ProjectStatus = "ACTIVE"
	ProjectSuspended ProjectStatus = "SUSPENDED"
	ProjectArchived  ProjectStatus = "ARCHIVED"
)

type Project struct {
	ID          uint64        `gorm:"primaryKey" json:"id"`
	TenantID    uint64        `gorm:"uniqueIndex:uk_project_code;not null" json:"tenantId"`
	Code        string        `gorm:"size:64;uniqueIndex:uk_project_code;not null" json:"code"`
	Name        string        `gorm:"size:128;not null" json:"name"`
	Description string        `gorm:"size:1024" json:"description"`
	Status      ProjectStatus `gorm:"size:24;index;not null" json:"status"`
	OwnerUserID uint64        `gorm:"index;not null" json:"ownerUserId"`
	CreatedBy   uint64        `gorm:"index;not null" json:"createdBy"`
	ArchivedAt  *time.Time    `json:"archivedAt"`
	CreatedAt   time.Time     `json:"createTime"`
	UpdatedAt   time.Time     `json:"updateTime"`
}

type ProjectMember struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	TenantID    uint64    `gorm:"index;uniqueIndex:uk_project_user;not null" json:"tenantId"`
	ProjectID   uint64    `gorm:"index;uniqueIndex:uk_project_user;not null" json:"projectId"`
	UserID      uint64    `gorm:"index;uniqueIndex:uk_project_user;not null" json:"userId"`
	LegacyRoles string    `gorm:"column:roles;type:json;not null" json:"-"`
	CreatedBy   uint64    `gorm:"not null" json:"createdBy"`
	CreatedAt   time.Time `json:"createTime"`
	UpdatedAt   time.Time `json:"updateTime"`
}

type ProjectConfig struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	TenantID       uint64    `gorm:"index;uniqueIndex:uk_project_config;not null" json:"tenantId"`
	ProjectID      uint64    `gorm:"index;uniqueIndex:uk_project_config;not null" json:"projectId"`
	StorageConfig  string    `gorm:"type:json;not null" json:"-"`
	ProviderConfig string    `gorm:"type:json;not null" json:"-"`
	SecretRefs     string    `gorm:"type:json;not null" json:"-"`
	CreatedAt      time.Time `json:"createTime"`
	UpdatedAt      time.Time `json:"updateTime"`
}

func (Project) TableName() string       { return "ai_project" }
func (ProjectMember) TableName() string { return "ai_project_member" }
func (ProjectConfig) TableName() string { return "ai_project_config" }
