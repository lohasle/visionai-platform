package visionai

import "time"

type AssetStatus string

const (
	AssetReady      AssetStatus = "READY"
	AssetInvalid    AssetStatus = "INVALID"
	AssetMissing    AssetStatus = "MISSING"
	AssetDeleted    AssetStatus = "DELETED"
	AssetPurged     AssetStatus = "PURGED"
	UploadCreated               = "CREATED"
	UploadActive                = "ACTIVE"
	UploadCompleted             = "COMPLETED"
	UploadFailed                = "FAILED"
)

type Asset struct {
	ID                 uint64      `gorm:"primaryKey" json:"id"`
	TenantID           uint64      `gorm:"index:idx_asset_scope;not null" json:"tenantId"`
	ProjectID          uint64      `gorm:"index:idx_asset_scope;not null" json:"projectId"`
	Filename           string      `gorm:"size:512;index;not null" json:"filename"`
	ObjectKey          string      `gorm:"size:768;index:,length:512;not null" json:"-"`
	URI                string      `gorm:"size:1024;not null" json:"uri"`
	ThumbnailObjectKey string      `gorm:"size:768" json:"-"`
	ThumbnailURI       string      `gorm:"size:1024" json:"thumbnailUri"`
	SHA256             string      `gorm:"size:64;index;not null" json:"sha256"`
	ContentType        string      `gorm:"size:128;index;not null" json:"contentType"`
	Size               int64       `gorm:"index;not null" json:"size"`
	Width              int         `gorm:"not null;default:0" json:"width"`
	Height             int         `gorm:"not null;default:0" json:"height"`
	MediaKind          string      `gorm:"size:16;index;not null;default:IMAGE" json:"mediaKind"`
	DurationSeconds    float64     `gorm:"not null;default:0" json:"durationSeconds"`
	Codec              string      `gorm:"size:64" json:"codec"`
	Language           string      `gorm:"size:32" json:"language"`
	SourceDevice       string      `gorm:"size:256" json:"sourceDevice"`
	BusinessScene      string      `gorm:"size:256;index" json:"businessScene"`
	PerceptualHash     string      `gorm:"size:64;index" json:"perceptualHash"`
	NearDuplicateOfID  uint64      `gorm:"index;not null;default:0" json:"nearDuplicateOfId"`
	Status             AssetStatus `gorm:"size:24;index;not null" json:"status"`
	RecycleFromStatus  AssetStatus `gorm:"size:24" json:"recycleFromStatus"`
	DuplicateOfID      uint64      `gorm:"index;not null;default:0" json:"duplicateOfId"`
	ReferenceCount     int         `gorm:"not null;default:0" json:"referenceCount"`
	Metadata           string      `gorm:"type:json;not null" json:"-"`
	ErrorCode          string      `gorm:"size:128" json:"errorCode"`
	ErrorMessage       string      `gorm:"size:1024" json:"errorMessage"`
	CreatedBy          uint64      `gorm:"index;not null" json:"createdBy"`
	DeletedBy          uint64      `gorm:"index;not null;default:0" json:"deletedBy"`
	DeletedAt          *time.Time  `gorm:"index" json:"deletedAt"`
	CreatedAt          time.Time   `json:"createTime"`
	UpdatedAt          time.Time   `json:"updateTime"`
}

type UploadSession struct {
	ID              string    `gorm:"size:64;primaryKey" json:"id"`
	TenantID        uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID       uint64    `gorm:"index;not null" json:"projectId"`
	Filename        string    `gorm:"size:512;not null" json:"filename"`
	DeclaredType    string    `gorm:"size:128" json:"declaredContentType"`
	TotalSize       int64     `gorm:"not null" json:"totalSize"`
	ChunkSize       int64     `gorm:"not null" json:"chunkSize"`
	TotalChunks     int       `gorm:"not null" json:"totalChunks"`
	ReceivedChunks  int       `gorm:"not null;default:0" json:"receivedChunks"`
	Status          string    `gorm:"size:24;index;not null" json:"status"`
	DuplicatePolicy string    `gorm:"size:24;not null;default:REFERENCE" json:"duplicatePolicy"`
	Metadata        string    `gorm:"type:json;not null" json:"metadata"`
	AssetID         uint64    `gorm:"index;not null;default:0" json:"assetId"`
	ErrorCode       string    `gorm:"size:128" json:"errorCode"`
	ErrorMessage    string    `gorm:"size:1024" json:"errorMessage"`
	CreatedBy       uint64    `gorm:"index;not null" json:"createdBy"`
	ExpiresAt       time.Time `gorm:"index;not null" json:"expiresAt"`
	CreatedAt       time.Time `json:"createTime"`
	UpdatedAt       time.Time `json:"updateTime"`
}

type UploadChunk struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint64    `gorm:"index;not null" json:"tenantId"`
	SessionID string    `gorm:"size:64;uniqueIndex:uk_upload_part;not null" json:"sessionId"`
	Part      int       `gorm:"uniqueIndex:uk_upload_part;not null" json:"part"`
	ObjectKey string    `gorm:"size:512;uniqueIndex;not null" json:"-"`
	Size      int64     `gorm:"not null" json:"size"`
	SHA256    string    `gorm:"size:64;not null" json:"sha256"`
	CreatedAt time.Time `json:"createTime"`
	UpdatedAt time.Time `json:"updateTime"`
}

type AssetReference struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	TenantID     uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID    uint64    `gorm:"index;not null" json:"projectId"`
	AssetID      uint64    `gorm:"uniqueIndex:uk_asset_reference;not null" json:"assetId"`
	ResourceType string    `gorm:"size:64;uniqueIndex:uk_asset_reference;not null" json:"resourceType"`
	ResourceID   uint64    `gorm:"uniqueIndex:uk_asset_reference;not null" json:"resourceId"`
	Frozen       bool      `gorm:"index;not null;default:false" json:"frozen"`
	CreatedAt    time.Time `json:"createTime"`
}

func (Asset) TableName() string          { return "ai_asset" }
func (UploadSession) TableName() string  { return "ai_upload_session" }
func (UploadChunk) TableName() string    { return "ai_upload_chunk" }
func (AssetReference) TableName() string { return "ai_asset_reference" }
