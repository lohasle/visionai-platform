package visionai

import "time"

type AssetImportRun struct {
	ID           uint64     `gorm:"primaryKey" json:"id"`
	TenantID     uint64     `gorm:"index;not null" json:"tenantId"`
	ProjectID    uint64     `gorm:"index;not null" json:"projectId"`
	JobID        uint64     `gorm:"uniqueIndex;not null" json:"jobId"`
	SourceType   string     `gorm:"size:24;index;not null" json:"sourceType"`
	SourceConfig string     `gorm:"type:json;not null" json:"-"`
	Status       string     `gorm:"size:32;index;not null" json:"status"`
	Total        int64      `gorm:"not null;default:0" json:"total"`
	Succeeded    int64      `gorm:"not null;default:0" json:"succeeded"`
	Failed       int64      `gorm:"not null;default:0" json:"failed"`
	Report       string     `gorm:"type:json;not null" json:"-"`
	CreatedBy    uint64     `gorm:"index;not null" json:"createdBy"`
	StartedAt    *time.Time `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
	CreatedAt    time.Time  `json:"createTime"`
	UpdatedAt    time.Time  `json:"updateTime"`
}

func (AssetImportRun) TableName() string { return "ai_asset_import_run" }
