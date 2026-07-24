package visionai

import (
	"encoding/json"
	"time"
)

// DomainEventEnvelope is the versioned transport contract shared by the API
// outbox publisher and all asynchronous workers.
type DomainEventEnvelope struct {
	SchemaVersion string          `json:"schemaVersion"`
	EventID       string          `json:"eventId"`
	EventType     string          `json:"eventType"`
	TenantID      uint64          `json:"tenantId"`
	AggregateType string          `json:"aggregateType"`
	AggregateID   uint64          `json:"aggregateId"`
	OccurredAt    time.Time       `json:"occurredAt"`
	Payload       json.RawMessage `json:"payload"`
}
