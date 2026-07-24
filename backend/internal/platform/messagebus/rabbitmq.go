// Package messagebus contains the durable RabbitMQ transport used by domain events.
package messagebus

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQ struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	exchange   string
	queue      string
}

func Open(url, exchange, queue string) (*RabbitMQ, error) {
	connection, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("connect rabbitmq: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	bus := &RabbitMQ{connection: connection, channel: channel, exchange: exchange, queue: queue}
	if err = bus.declare(); err != nil {
		_ = bus.Close()
		return nil, err
	}
	return bus, nil
}

func (r *RabbitMQ) declare() error {
	if err := r.channel.ExchangeDeclare(r.exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}
	if _, err := r.channel.QueueDeclare(r.queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}
	if err := r.channel.QueueBind(r.queue, "#", r.exchange, false, nil); err != nil {
		return fmt.Errorf("bind queue: %w", err)
	}
	return nil
}

func (r *RabbitMQ) Publish(ctx context.Context, eventID, eventType string, body []byte) error {
	if err := r.channel.Confirm(false); err != nil {
		return fmt.Errorf("enable publisher confirms: %w", err)
	}
	confirmation, err := r.channel.PublishWithDeferredConfirmWithContext(ctx, r.exchange, eventType, true, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		MessageId:    eventID,
		Type:         eventType,
		Body:         body,
	})
	if err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	if confirmation == nil || !confirmation.Wait() {
		return fmt.Errorf("publish event %s was not acknowledged", eventID)
	}
	return nil
}

func (r *RabbitMQ) Consume() (<-chan amqp.Delivery, error) {
	if err := r.channel.Qos(1, 0, false); err != nil {
		return nil, fmt.Errorf("set consumer qos: %w", err)
	}
	deliveries, err := r.channel.Consume(r.queue, "", false, false, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("consume queue: %w", err)
	}
	return deliveries, nil
}

func (r *RabbitMQ) Close() error {
	if r.channel != nil {
		_ = r.channel.Close()
	}
	if r.connection != nil {
		return r.connection.Close()
	}
	return nil
}
