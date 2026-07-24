package visionai

import "time"

type ComputeNode struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	TenantID        uint64    `gorm:"uniqueIndex:uk_compute_node;not null" json:"tenantId"`
	NodeKey         string    `gorm:"size:128;uniqueIndex:uk_compute_node;not null" json:"nodeKey"`
	Name            string    `gorm:"size:160;not null" json:"name"`
	Status          string    `gorm:"size:24;index;not null" json:"status"`
	GPUModel        string    `gorm:"size:160" json:"gpuModel"`
	GPUCount        int       `json:"gpuCount"`
	GPUMemoryBytes  int64     `json:"gpuMemoryBytes"`
	GPUUsedBytes    int64     `json:"gpuUsedBytes"`
	DriverVersion   string    `gorm:"size:64" json:"driverVersion"`
	CUDAVersion     string    `gorm:"size:64" json:"cudaVersion"`
	Labels          string    `gorm:"type:json;not null" json:"labels"`
	LastHeartbeatAt time.Time `gorm:"index;not null" json:"lastHeartbeatAt"`
	CreatedAt       time.Time `json:"createTime"`
	UpdatedAt       time.Time `json:"updateTime"`
}

type ComputeQueue struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	TenantID      uint64    `gorm:"index;not null" json:"tenantId"`
	Name          string    `gorm:"size:128;uniqueIndex:uk_compute_queue;not null" json:"name"`
	Provider      string    `gorm:"size:32;uniqueIndex:uk_compute_queue;not null" json:"provider"`
	ExternalQueue string    `gorm:"size:256;not null" json:"externalQueue"`
	Priority      int       `json:"priority"`
	Enabled       bool      `gorm:"index;not null" json:"enabled"`
	CreatedAt     time.Time `json:"createTime"`
	UpdatedAt     time.Time `json:"updateTime"`
}

type ProjectQuota struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	TenantID          uint64    `gorm:"uniqueIndex:uk_project_quota;not null" json:"tenantId"`
	ProjectID         uint64    `gorm:"uniqueIndex:uk_project_quota;not null" json:"projectId"`
	MaxConcurrentJobs int       `json:"maxConcurrentJobs"`
	MonthlyGPUHours   float64   `json:"monthlyGpuHours"`
	StorageBytes      int64     `json:"storageBytes"`
	UpdatedBy         uint64    `gorm:"index" json:"updatedBy"`
	CreatedAt         time.Time `json:"createTime"`
	UpdatedAt         time.Time `json:"updateTime"`
}

type IntegrationInstance struct {
	ID            uint64     `gorm:"primaryKey" json:"id"`
	TenantID      uint64     `gorm:"index;not null" json:"tenantId"`
	Name          string     `gorm:"size:160;uniqueIndex:uk_integration;not null" json:"name"`
	ProviderType  string     `gorm:"size:32;uniqueIndex:uk_integration;not null" json:"providerType"`
	BaseURL       string     `gorm:"size:1024;not null" json:"baseUrl"`
	SecretRef     string     `gorm:"size:512;not null" json:"secretRef"`
	Version       string     `gorm:"size:64" json:"version"`
	Status        string     `gorm:"size:32;index;not null" json:"status"`
	Capabilities  string     `gorm:"type:json;not null" json:"capabilities"`
	LastError     string     `gorm:"size:2048" json:"lastError"`
	LastCheckedAt *time.Time `json:"lastCheckedAt"`
	CreatedBy     uint64     `gorm:"index;not null" json:"createdBy"`
	CreatedAt     time.Time  `json:"createTime"`
	UpdatedAt     time.Time  `json:"updateTime"`
}

type CompatibilityRule struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	TenantID      uint64    `gorm:"index;not null" json:"tenantId"`
	ProviderType  string    `gorm:"size:32;index;not null" json:"providerType"`
	PlatformRange string    `gorm:"size:64;not null" json:"platformRange"`
	ProviderRange string    `gorm:"size:64;not null" json:"providerRange"`
	Decision      string    `gorm:"size:32;index;not null" json:"decision"`
	Notes         string    `gorm:"size:1024" json:"notes"`
	CreatedBy     uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt     time.Time `json:"createTime"`
	UpdatedAt     time.Time `json:"updateTime"`
}

type SyncIncident struct {
	ID            uint64     `gorm:"primaryKey" json:"id"`
	TenantID      uint64     `gorm:"index;not null" json:"tenantId"`
	InstanceID    uint64     `gorm:"index;not null" json:"instanceId"`
	ResourceType  string     `gorm:"size:64;index;not null" json:"resourceType"`
	ResourceID    string     `gorm:"size:128;index;not null" json:"resourceId"`
	Code          string     `gorm:"size:64;index;not null" json:"code"`
	Message       string     `gorm:"size:2048;not null" json:"message"`
	Status        string     `gorm:"size:24;index;not null" json:"status"`
	Attempts      int        `json:"attempts"`
	LastAttemptAt *time.Time `json:"lastAttemptAt"`
	ResolvedAt    *time.Time `json:"resolvedAt"`
	CreatedAt     time.Time  `json:"createTime"`
	UpdatedAt     time.Time  `json:"updateTime"`
}

func (ComputeNode) TableName() string         { return "ai_compute_node" }
func (ComputeQueue) TableName() string        { return "ai_compute_queue" }
func (ProjectQuota) TableName() string        { return "ai_project_quota" }
func (IntegrationInstance) TableName() string { return "ai_integration_instance" }
func (CompatibilityRule) TableName() string   { return "ai_compatibility_rule" }
func (SyncIncident) TableName() string        { return "ai_sync_incident" }
