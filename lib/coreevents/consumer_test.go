package coreevents

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"analytics-service/pkg/messaging"
)

// Colocated unit test — same package so handleMessage (unexported) is
// directly callable. Broker and EventDeduper are both interfaces
// (pkg/messaging.Broker, this package's EventDeduper), so the dispatch
// logic — dedupe check, handler lookup, ack/nack/DLQ decisions — is fully
// unit-testable without a real RabbitMQ or Mongo. This is the exact
// idempotency/DLQ-routing logic CLAUDE.md flags as safety-critical for a
// service that aggregates money, so it earns its own test file even though
// it wasn't in Phase 2's original list.

// fakeDeduper lets each test script MarkSeen/Unmark behavior without a real
// Mongo unique index.
type fakeDeduper struct {
	markSeenFresh bool
	markSeenErr   error
	unmarkErr     error
	unmarkCalls   []string
	markSeenCalls []string
}

func (d *fakeDeduper) MarkSeen(_ context.Context, eventID string) (bool, error) {
	d.markSeenCalls = append(d.markSeenCalls, eventID)
	if d.markSeenErr != nil {
		return false, d.markSeenErr
	}
	return d.markSeenFresh, nil
}

func (d *fakeDeduper) Unmark(_ context.Context, eventID string) error {
	d.unmarkCalls = append(d.unmarkCalls, eventID)
	return d.unmarkErr
}

// fakeBroker is never actually driven by handleMessage (broker involvement
// stops at Start/Consume) but Consumer's fields require *a* messaging.Broker
// to construct via NewConsumer.
type fakeBroker struct{}

func (fakeBroker) Connect(context.Context) error                                    { return nil }
func (fakeBroker) Close() error                                                     { return nil }
func (fakeBroker) DeclareTopology(context.Context, messaging.ConsumerOptions) error { return nil }
func (fakeBroker) Consume(context.Context, messaging.ConsumerOptions, func(messaging.ConsumeMessage)) error {
	return nil
}

func newTestConsumer(dedupe EventDeduper) *Consumer {
	return NewConsumer(fakeBroker{}, messaging.ConsumerOptions{}, dedupe, slog.Default())
}

type ackTracker struct {
	acked       bool
	nacked      bool
	nackRequeue bool
}

func (a *ackTracker) message(eventID, eventType, payload string) messaging.ConsumeMessage {
	return messaging.ConsumeMessage{
		Body: []byte(`{"eventId":"` + eventID + `","eventType":"` + eventType + `","payload":` + payload + `}`),
		Ack:  func() { a.acked = true },
		Nack: func(requeue bool) { a.nacked = true; a.nackRequeue = requeue },
	}
}

func TestConsumer_HandleMessage_MalformedEnvelope_NacksNoRequeue(t *testing.T) {
	dedupe := &fakeDeduper{}
	c := newTestConsumer(dedupe)
	tracker := &ackTracker{}

	c.handleMessage(messaging.ConsumeMessage{
		Body: []byte("not json"),
		Ack:  func() { tracker.acked = true },
		Nack: func(requeue bool) { tracker.nacked = true; tracker.nackRequeue = requeue },
	})

	if !tracker.nacked || tracker.nackRequeue {
		t.Fatalf("expected nack(false) for malformed body, got nacked=%v requeue=%v", tracker.nacked, tracker.nackRequeue)
	}
	if tracker.acked {
		t.Fatal("expected no ack for malformed body")
	}
	if len(dedupe.markSeenCalls) != 0 {
		t.Fatal("expected dedupe never consulted for a message that fails envelope parsing")
	}
}

func TestConsumer_HandleMessage_MissingEventIDOrType_NacksNoRequeue(t *testing.T) {
	dedupe := &fakeDeduper{}
	c := newTestConsumer(dedupe)
	tracker := &ackTracker{}

	// Valid JSON, but eventId is empty.
	c.handleMessage(messaging.ConsumeMessage{
		Body: []byte(`{"eventId":"","eventType":"order.placed","payload":{}}`),
		Ack:  func() { tracker.acked = true },
		Nack: func(requeue bool) { tracker.nacked = true; tracker.nackRequeue = requeue },
	})

	if !tracker.nacked || tracker.nackRequeue {
		t.Fatalf("expected nack(false) for empty eventId, got nacked=%v requeue=%v", tracker.nacked, tracker.nackRequeue)
	}
}

func TestConsumer_HandleMessage_DedupeCheckFails_NacksWithRequeue(t *testing.T) {
	dedupe := &fakeDeduper{markSeenErr: errors.New("mongo down")}
	c := newTestConsumer(dedupe)
	tracker := &ackTracker{}
	handlerCalled := false
	c.Register("order.placed", func(context.Context, json.RawMessage) error { handlerCalled = true; return nil })

	c.handleMessage(tracker.message("evt-1", "order.placed", "{}"))

	if !tracker.nacked || !tracker.nackRequeue {
		t.Fatalf("expected nack(true) (requeue) on transient dedupe-store failure, got nacked=%v requeue=%v", tracker.nacked, tracker.nackRequeue)
	}
	if handlerCalled {
		t.Fatal("expected handler never called when dedupe check itself errors")
	}
}

func TestConsumer_HandleMessage_DuplicateEvent_AcksWithoutCallingHandler(t *testing.T) {
	dedupe := &fakeDeduper{markSeenFresh: false} // "already seen"
	c := newTestConsumer(dedupe)
	tracker := &ackTracker{}
	handlerCalled := false
	c.Register("order.placed", func(context.Context, json.RawMessage) error { handlerCalled = true; return nil })

	c.handleMessage(tracker.message("evt-1", "order.placed", "{}"))

	if !tracker.acked {
		t.Fatal("expected ack-and-skip for a duplicate event")
	}
	if tracker.nacked {
		t.Fatal("expected no nack for a duplicate event")
	}
	if handlerCalled {
		t.Fatal("expected handler never invoked for a duplicate event")
	}
}

func TestConsumer_HandleMessage_UnknownEventType_AcksWithoutHandler(t *testing.T) {
	dedupe := &fakeDeduper{markSeenFresh: true}
	c := newTestConsumer(dedupe)
	tracker := &ackTracker{}
	// No handler registered for "order.placed" at all.

	c.handleMessage(tracker.message("evt-1", "order.placed", "{}"))

	if !tracker.acked {
		t.Fatal("expected ack-and-skip for an event type this service doesn't handle (never DLQ an unknown type)")
	}
	if tracker.nacked {
		t.Fatal("expected no nack for an unknown event type")
	}
}

func TestConsumer_HandleMessage_HandlerSucceeds_Acks(t *testing.T) {
	dedupe := &fakeDeduper{markSeenFresh: true}
	c := newTestConsumer(dedupe)
	tracker := &ackTracker{}
	var receivedPayload string
	c.Register("order.placed", func(_ context.Context, payload json.RawMessage) error {
		receivedPayload = string(payload)
		return nil
	})

	c.handleMessage(tracker.message("evt-1", "order.placed", `{"orderId":"o1"}`))

	if !tracker.acked || tracker.nacked {
		t.Fatalf("expected ack only on handler success, got acked=%v nacked=%v", tracker.acked, tracker.nacked)
	}
	if receivedPayload != `{"orderId":"o1"}` {
		t.Fatalf("expected handler to receive the raw payload, got %s", receivedPayload)
	}
	if len(dedupe.unmarkCalls) != 0 {
		t.Fatal("expected no Unmark call on handler success")
	}
}

func TestConsumer_HandleMessage_HandlerFails_UnmarksAndNacksNoRequeue(t *testing.T) {
	dedupe := &fakeDeduper{markSeenFresh: true}
	c := newTestConsumer(dedupe)
	tracker := &ackTracker{}
	c.Register("order.placed", func(context.Context, json.RawMessage) error { return errors.New("write failed") })

	c.handleMessage(tracker.message("evt-1", "order.placed", "{}"))

	if !tracker.nacked || tracker.nackRequeue {
		t.Fatalf("expected nack(false) (straight to DLQ, not requeue) on handler failure, got nacked=%v requeue=%v", tracker.nacked, tracker.nackRequeue)
	}
	if tracker.acked {
		t.Fatal("expected no ack on handler failure")
	}
	if len(dedupe.unmarkCalls) != 1 || dedupe.unmarkCalls[0] != "evt-1" {
		t.Fatalf("expected Unmark(evt-1) so a DLQ replay can retry instead of being treated as already-seen, got %v", dedupe.unmarkCalls)
	}
}

func TestConsumer_HandleMessage_HandlerFailsAndUnmarkAlsoFails_StillNacks(t *testing.T) {
	dedupe := &fakeDeduper{markSeenFresh: true, unmarkErr: errors.New("mongo down")}
	c := newTestConsumer(dedupe)
	tracker := &ackTracker{}
	c.Register("order.placed", func(context.Context, json.RawMessage) error { return errors.New("write failed") })

	c.handleMessage(tracker.message("evt-1", "order.placed", "{}"))

	if !tracker.nacked {
		t.Fatal("expected nack even when the best-effort Unmark call itself fails (logged, not fatal)")
	}
}

func TestConsumer_Register_DuplicateEventType_Panics(t *testing.T) {
	c := newTestConsumer(&fakeDeduper{})
	c.Register("order.placed", func(context.Context, json.RawMessage) error { return nil })

	defer func() {
		if recover() == nil {
			t.Fatal("expected Register to panic on a duplicate event-type registration (a wiring bug, should fail loudly at boot)")
		}
	}()
	c.Register("order.placed", func(context.Context, json.RawMessage) error { return nil })
}
