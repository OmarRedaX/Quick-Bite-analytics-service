// Package coreevents is the generic inbound event consumer: envelope
// parsing, idempotency, dispatch-by-event-type, and DLQ routing. It knows
// nothing about analytics collections or order.placed specifically — that
// wiring happens in app/analytics/eventhandlers, registered here by event
// type string. Go analogue of order-service's lib/core-events/consumer.ts.
package coreevents

import (
	"context"
	"encoding/json"
	"log/slog"

	"analytics-service/pkg/messaging"
)

// EventDeduper is the "have I seen this event before?" capability the
// consumer needs. Defined here — not in app/analytics — so this package
// never imports app/*. app/analytics/repository.EventIDsRepo satisfies this
// interface implicitly via Go duck typing; lib/boot wires the concrete type
// in. This is the "invert the dependency" pattern called out in CLAUDE.md.
type EventDeduper interface {
	// MarkSeen records eventID as processed. fresh=false means it was
	// already recorded (duplicate delivery) — the caller should skip it.
	MarkSeen(ctx context.Context, eventID string) (fresh bool, err error)
	// Unmark removes the eventID record. Called when a handler fails after
	// MarkSeen succeeded, so the event isn't permanently stuck as "seen"
	// when it was never actually applied (see docs/system-design.md, "why
	// Mongo dedupe, not Redis SETNX").
	Unmark(ctx context.Context, eventID string) error
}

type Consumer struct {
	broker   messaging.Broker
	opts     messaging.ConsumerOptions
	dedupe   EventDeduper
	logger   *slog.Logger
	handlers map[string]EventHandler
}

func NewConsumer(broker messaging.Broker, opts messaging.ConsumerOptions, dedupe EventDeduper, logger *slog.Logger) *Consumer {
	return &Consumer{
		broker:   broker,
		opts:     opts,
		dedupe:   dedupe,
		logger:   logger,
		handlers: make(map[string]EventHandler),
	}
}

// Register maps one event type to its handler. Panics on duplicate
// registration — a wiring bug that should fail loudly at boot, same as
// order-service's registerHandler.
func (c *Consumer) Register(eventType string, handler EventHandler) {
	if _, exists := c.handlers[eventType]; exists {
		panic("coreevents: handler already registered for " + eventType)
	}
	c.handlers[eventType] = handler
}

// Start declares topology and begins consuming; message handling runs on a
// goroutine owned by the broker implementation (see pkg/messaging.AMQPClient.Consume).
func (c *Consumer) Start(ctx context.Context) error {
	if err := c.broker.Consume(ctx, c.opts, c.handleMessage); err != nil {
		return err
	}
	c.logger.Info("event consumer started", "queue", c.opts.Queue, "bindings", c.opts.BindingKeys)
	return nil
}

func (c *Consumer) handleMessage(msg messaging.ConsumeMessage) {
	ctx := context.Background()

	var envelope Envelope
	if err := json.Unmarshal(msg.Body, &envelope); err != nil || envelope.EventID == "" || envelope.EventType == "" {
		c.logger.Warn("coreevents: malformed message, nacking (no requeue)", "error", errString(err))
		msg.Nack(false)
		return
	}
	log := c.logger.With("eventId", envelope.EventID, "eventType", envelope.EventType)

	fresh, err := c.dedupe.MarkSeen(ctx, envelope.EventID)
	if err != nil {
		log.Error("dedupe check failed, nacking for redelivery", "error", err.Error())
		msg.Nack(true) // transient store failure — requeue, don't burn a DLQ slot
		return
	}
	if !fresh {
		log.Info("duplicate event, ack-and-skip")
		msg.Ack()
		return
	}

	handler, ok := c.handlers[envelope.EventType]
	if !ok {
		log.Warn("no handler registered, ack-and-skip")
		msg.Ack()
		return
	}

	if err := handler(ctx, envelope.Payload); err != nil {
		log.Error("handler failed, sending to DLQ", "error", err.Error())
		if unmarkErr := c.dedupe.Unmark(ctx, envelope.EventID); unmarkErr != nil {
			log.Error("failed to unmark event after handler failure — will be treated as already-seen on replay", "error", unmarkErr.Error())
		}
		msg.Nack(false)
		return
	}
	msg.Ack()
}

func errString(err error) string {
	if err == nil {
		return "empty eventId/eventType"
	}
	return err.Error()
}
