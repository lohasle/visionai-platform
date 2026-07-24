// Command orchestrator consumes durable VisionAI domain events independently
// from the control-plane API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/modules/visionai"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/database"
	"github.com/lohasle/nimbus-framework-go/internal/platform/messagebus"
	"gorm.io/gorm"
)

const consumerName = "visionai-orchestrator-v1"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := openDatabase(ctx, cfg)
	if err != nil {
		slog.Error("database initialization stopped", "error", err)
		return
	}
	for ctx.Err() == nil {
		if err = consume(ctx, db, cfg); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("orchestrator consumer disconnected", "error", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
}

func openDatabase(ctx context.Context, cfg config.Config) (*gorm.DB, error) {
	for {
		db, err := database.Open(cfg)
		if err == nil {
			err = visionai.Migrate(db)
		}
		if err == nil {
			return db, nil
		}
		slog.Warn("orchestrator database unavailable; retrying", "error", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func consume(ctx context.Context, db *gorm.DB, cfg config.Config) error {
	bus, err := messagebus.Open(cfg.RabbitMQURL, cfg.EventExchange, cfg.EventQueue)
	if err != nil {
		return err
	}
	defer bus.Close()
	deliveries, err := bus.Consume()
	if err != nil {
		return err
	}
	slog.Info("visionai orchestrator consuming", "queue", cfg.EventQueue)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq delivery channel closed")
			}
			var envelope visionai.DomainEventEnvelope
			if err = json.Unmarshal(delivery.Body, &envelope); err == nil && envelope.EventID != delivery.MessageId {
				err = errors.New("event envelope id does not match AMQP message id")
			}
			if err == nil {
				err = process(ctx, db, cfg, envelope)
			}
			if err != nil {
				slog.Error("event processing failed", "eventId", delivery.MessageId, "error", err)
				_ = delivery.Nack(false, true)
				continue
			}
			_ = delivery.Ack(false)
		}
	}
}

func process(ctx context.Context, db *gorm.DB, cfg config.Config, envelope visionai.DomainEventEnvelope) error {
	if envelope.EventID == "" {
		return errors.New("event message id is required")
	}
	var existing visionai.InboxEvent
	err := db.Where("consumer = ? AND event_id = ?", consumerName, envelope.EventID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err = visionai.ProcessDomainEvent(ctx, db, cfg, envelope); err != nil {
		return err
	}
	return db.Create(&visionai.InboxEvent{
		TenantID: envelope.TenantID, Consumer: consumerName,
		EventID: envelope.EventID, ProcessedAt: time.Now(),
	}).Error
}
