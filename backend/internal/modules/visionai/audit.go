package visionai

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuditEvent is an append-only record of governed VisionAI mutations.
type AuditEvent struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	TenantID     uint64    `gorm:"index;not null" json:"tenantId"`
	ProjectID    uint64    `gorm:"index;not null;default:0" json:"projectId"`
	ActorUserID  uint64    `gorm:"index;not null" json:"actorUserId"`
	Action       string    `gorm:"size:128;index;not null" json:"action"`
	ResourceType string    `gorm:"size:64;index;not null" json:"resourceType"`
	ResourceID   uint64    `gorm:"index;not null;default:0" json:"resourceId"`
	BeforeJSON   string    `gorm:"type:json" json:"before"`
	AfterJSON    string    `gorm:"type:json" json:"after"`
	TraceID      string    `gorm:"size:64;index" json:"traceId"`
	IP           string    `gorm:"size:64" json:"ip"`
	CreatedAt    time.Time `json:"createTime"`
}

func (AuditEvent) TableName() string { return "ai_audit_event" }

func (*AuditEvent) BeforeUpdate(*gorm.DB) error { return errors.New("audit events are immutable") }
func (*AuditEvent) BeforeDelete(*gorm.DB) error { return errors.New("audit events are immutable") }

func appendAudit(tx *gorm.DB, c *gin.Context, projectID uint64, action, resourceType string, resourceID uint64, before, after any) error {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	return tx.Create(&AuditEvent{
		TenantID: tenantID(c), ProjectID: projectID, ActorUserID: c.GetUint64("user_id"),
		Action: action, ResourceType: resourceType, ResourceID: resourceID,
		BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON),
		TraceID: c.GetString("trace_id"), IP: c.ClientIP(),
	}).Error
}
