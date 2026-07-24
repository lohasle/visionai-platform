package visionai

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/messagebus"
	"gorm.io/gorm"
)

const (
	outboxNew        = "NEW"
	outboxPublishing = "PUBLISHING"
	outboxRetry      = "RETRY"
	outboxPublished  = "PUBLISHED"
	outboxDead       = "DEAD"
	outboxMaxRetries = 8
)

func RunOutboxPublisher(ctx context.Context, db *gorm.DB, cfg config.Config) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := publishOutboxBatch(ctx, db, cfg); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("visionai outbox publish cycle failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func publishOutboxBatch(ctx context.Context, db *gorm.DB, cfg config.Config) error {
	now := time.Now()
	// A process can stop after claiming a row but before publishing it. Reclaim
	// those rows; downstream Inbox idempotency makes an uncertain duplicate safe.
	_ = db.Model(&OutboxEvent{}).
		Where("status = ? AND updated_at < ?", outboxPublishing, now.Add(-5*time.Minute)).
		Updates(map[string]any{"status": outboxRetry, "next_attempt_at": now}).Error
	var events []OutboxEvent
	if err := db.Where(
		"status IN ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
		[]string{outboxNew, outboxRetry}, now,
	).Order("id ASC").Limit(50).Find(&events).Error; err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	bus, err := messagebus.Open(cfg.RabbitMQURL, cfg.EventExchange, cfg.EventQueue)
	if err != nil {
		return err
	}
	defer bus.Close()
	for i := range events {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		event := &events[i]
		claimed := db.Model(&OutboxEvent{}).
			Where("id = ? AND status IN ?", event.ID, []string{outboxNew, outboxRetry}).
			Update("status", outboxPublishing).RowsAffected
		if claimed == 0 {
			continue
		}
		payload := json.RawMessage(event.Payload)
		if !json.Valid(payload) {
			payload = json.RawMessage(`{}`)
		}
		body, marshalErr := json.Marshal(DomainEventEnvelope{
			SchemaVersion: "1.0",
			EventID:       event.EventID,
			EventType:     event.EventType,
			TenantID:      event.TenantID,
			AggregateType: event.AggregateType,
			AggregateID:   event.AggregateID,
			OccurredAt:    event.CreatedAt,
			Payload:       payload,
		})
		if marshalErr != nil {
			return marshalErr
		}
		if err = bus.Publish(ctx, event.EventID, event.EventType, body); err != nil {
			attempts := event.Attempts + 1
			status := outboxRetry
			if attempts >= outboxMaxRetries {
				status = outboxDead
			}
			nextAttempt := now.Add(outboxBackoff(attempts))
			_ = db.Model(event).Updates(map[string]any{
				"status":          status,
				"attempts":        attempts,
				"next_attempt_at": nextAttempt,
			}).Error
			slog.Warn("visionai event publish failed", "eventId", event.EventID, "attempt", attempts, "error", err)
			continue
		}
		publishedAt := time.Now()
		if err = db.Model(event).Updates(map[string]any{
			"status":          outboxPublished,
			"published_at":    publishedAt,
			"next_attempt_at": nil,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func outboxBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<uint(attempt-1)) * time.Second
}
