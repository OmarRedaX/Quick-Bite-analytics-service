package coreevents

import (
	"context"
	"encoding/json"
)

// Envelope matches the wire shape order-service's outbox-drain publishes
// (drainOutboxForRegion in outbox-drain.ts): eventId, eventType, occurredAt,
// aggregateType, aggregateId, region, payload. Generic across every event
// type — the concrete payload shape lives in app/analytics/types.go and is
// unmarshaled by each handler.
type Envelope struct {
	EventID       string          `json:"eventId"`
	EventType     string          `json:"eventType"`
	OccurredAt    string          `json:"occurredAt"`
	AggregateType string          `json:"aggregateType"`
	AggregateID   string          `json:"aggregateId"`
	Region        string          `json:"region"`
	Payload       json.RawMessage `json:"payload"`
}

// EventHandler processes one event's raw payload. Returning an error means
// "not applied" — the consumer un-marks the event as seen and nacks so a
// redelivery or DLQ replay can retry it. Handlers must be safe to call more
// than once for the same eventId (Mongo upserts with $inc make this true
// for every handler in this service).
type EventHandler func(ctx context.Context, payload json.RawMessage) error
