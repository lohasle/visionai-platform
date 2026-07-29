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
	GPUUtilization  float64   `json:"gpuUtilization"`
	GPUTemperatureC float64   `json:"gpuTemperatureC"`
	GPUPowerWatts   float64   `json:"gpuPowerWatts"`
	CPUUtilization  float64   `json:"cpuUtilization"`
	MemoryBytes     int64     `json:"memoryBytes"`
	MemoryUsedBytes int64     `json:"memoryUsedBytes"`
	DiskBytes       int64     `json:"diskBytes"`
	DiskUsedBytes   int64     `json:"diskUsedBytes"`
	NetworkRXBytes  int64     `json:"networkRxBytes"`
	NetworkTXBytes  int64     `json:"networkTxBytes"`
	DriverVersion   string    `gorm:"size:64" json:"driverVersion"`
	CUDAVersion     string    `gorm:"size:64" json:"cudaVersion"`
	Labels          string    `gorm:"type:json;not null" json:"labels"`
	LastHeartbeatAt time.Time `gorm:"index;not null" json:"lastHeartbeatAt"`
	CreatedAt       time.Time `json:"createTime"`
	UpdatedAt       time.Time `json:"updateTime"`
}

type ResourceMetricSample struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	TenantID        uint64    `gorm:"index;not null" json:"tenantId"`
	ComputeNodeID   uint64    `gorm:"index;not null" json:"computeNodeId"`
	NodeKey         string    `gorm:"size:128;index;not null" json:"nodeKey"`
	GPUUtilization  float64   `json:"gpuUtilization"`
	GPUMemoryBytes  int64     `json:"gpuMemoryBytes"`
	GPUUsedBytes    int64     `json:"gpuUsedBytes"`
	GPUTemperatureC float64   `json:"gpuTemperatureC"`
	GPUPowerWatts   float64   `json:"gpuPowerWatts"`
	CPUUtilization  float64   `json:"cpuUtilization"`
	MemoryBytes     int64     `json:"memoryBytes"`
	MemoryUsedBytes int64     `json:"memoryUsedBytes"`
	DiskBytes       int64     `json:"diskBytes"`
	DiskUsedBytes   int64     `json:"diskUsedBytes"`
	NetworkRXBytes  int64     `json:"networkRxBytes"`
	NetworkTXBytes  int64     `json:"networkTxBytes"`
	ActiveRunIDs    string    `gorm:"type:json;not null" json:"activeRunIds"`
	SampledAt       time.Time `gorm:"index;not null" json:"sampledAt"`
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
	NetworkRegion string     `gorm:"size:80;index;not null;default:LOCAL" json:"networkRegion"`
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

type IntegrationConfigRevision struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	TenantID       uint64    `gorm:"index;not null" json:"tenantId"`
	InstanceID     uint64    `gorm:"uniqueIndex:uk_integration_revision;not null" json:"instanceId"`
	RevisionNo     int       `gorm:"uniqueIndex:uk_integration_revision;not null" json:"revisionNo"`
	FromVersion    string    `gorm:"size:64" json:"fromVersion"`
	ToVersion      string    `gorm:"size:64" json:"toVersion"`
	BeforeConfig   string    `gorm:"type:longtext;not null" json:"beforeConfig"`
	AfterConfig    string    `gorm:"type:longtext;not null" json:"afterConfig"`
	BeforeSHA256   string    `gorm:"size:64;not null" json:"beforeSha256"`
	AfterSHA256    string    `gorm:"size:64;not null" json:"afterSha256"`
	Diff           string    `gorm:"type:json;not null" json:"diff"`
	Compatibility  string    `gorm:"size:32;index;not null" json:"compatibility"`
	SmokeResult    string    `gorm:"type:json;not null" json:"smokeResult"`
	Status         string    `gorm:"size:32;index;not null" json:"status"`
	RollbackReason string    `gorm:"size:2048" json:"rollbackReason"`
	CreatedBy      uint64    `gorm:"index;not null" json:"createdBy"`
	CreatedAt      time.Time `json:"createTime"`
}

type SyncIncident struct {
	ID               uint64     `gorm:"primaryKey" json:"id"`
	TenantID         uint64     `gorm:"index;not null" json:"tenantId"`
	InstanceID       uint64     `gorm:"index;not null" json:"instanceId"`
	ResourceType     string     `gorm:"size:64;index;not null" json:"resourceType"`
	ResourceID       string     `gorm:"size:128;index;not null" json:"resourceId"`
	Code             string     `gorm:"size:64;index;not null" json:"code"`
	Message          string     `gorm:"size:2048;not null" json:"message"`
	ExternalID       string     `gorm:"size:256" json:"externalId"`
	Status           string     `gorm:"size:24;index;not null" json:"status"`
	Attempts         int        `json:"attempts"`
	ResolutionAction string     `gorm:"size:32;index" json:"resolutionAction"`
	ResolutionReason string     `gorm:"size:2048" json:"resolutionReason"`
	ResolvedBy       uint64     `gorm:"index" json:"resolvedBy"`
	LastAttemptAt    *time.Time `json:"lastAttemptAt"`
	ResolvedAt       *time.Time `json:"resolvedAt"`
	CreatedAt        time.Time  `json:"createTime"`
	UpdatedAt        time.Time  `json:"updateTime"`
}

func (ComputeNode) TableName() string               { return "ai_compute_node" }
func (ResourceMetricSample) TableName() string      { return "ai_resource_metric_sample" }
func (ComputeQueue) TableName() string              { return "ai_compute_queue" }
func (ProjectQuota) TableName() string              { return "ai_project_quota" }
func (IntegrationInstance) TableName() string       { return "ai_integration_instance" }
func (CompatibilityRule) TableName() string         { return "ai_compatibility_rule" }
func (IntegrationConfigRevision) TableName() string { return "ai_integration_config_revision" }
func (SyncIncident) TableName() string              { return "ai_sync_incident" }
