//go:build integration

package integration

import (
	"context"
	"testing"

	"analytics-service/tests/integration/testutil"
)

// Idempotency (Phase 3 item 6). Exercises the real unique index on
// event_ids.event_id (see repository/indexes.go's uq_event_id) — a unit
// test against a fake dedupe store can't prove Mongo actually enforces
// this; only a real duplicate InsertOne can.

func TestEventIDs_MarkSeen_DuplicateIsAckAndSkipNotError(t *testing.T) {
	db := testutil.ConnectMongo(t)
	repos := testutil.NewRepoBundle(db)
	ctx := context.Background()

	fresh1, err := repos.EventIDs.MarkSeen(ctx, "dup-event-1")
	if err != nil {
		t.Fatalf("first MarkSeen: unexpected error: %v", err)
	}
	if !fresh1 {
		t.Fatal("expected fresh=true on first MarkSeen")
	}

	fresh2, err := repos.EventIDs.MarkSeen(ctx, "dup-event-1")
	if err != nil {
		t.Fatalf("second MarkSeen (duplicate): expected the unique-index violation translated to (false, nil), got error: %v", err)
	}
	if fresh2 {
		t.Fatal("expected fresh=false for a duplicate eventId (real unique index enforced it)")
	}
}

func TestEventIDs_Unmark_AllowsRetryAfterHandlerFailure(t *testing.T) {
	db := testutil.ConnectMongo(t)
	repos := testutil.NewRepoBundle(db)
	ctx := context.Background()

	if _, err := repos.EventIDs.MarkSeen(ctx, "retry-event-1"); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	if err := repos.EventIDs.Unmark(ctx, "retry-event-1"); err != nil {
		t.Fatalf("Unmark: %v", err)
	}

	// After Unmark (simulating a handler failure that must allow a DLQ
	// replay to retry, not be silently skipped as "already seen"), the
	// event id must be markable fresh again.
	fresh, err := repos.EventIDs.MarkSeen(ctx, "retry-event-1")
	if err != nil {
		t.Fatalf("MarkSeen after Unmark: %v", err)
	}
	if !fresh {
		t.Fatal("expected fresh=true after Unmark — the replay must not be treated as a duplicate")
	}
}

func TestEventIDs_MarkSeen_DifferentEventIDsAreIndependentlyFresh(t *testing.T) {
	db := testutil.ConnectMongo(t)
	repos := testutil.NewRepoBundle(db)
	ctx := context.Background()

	fresh1, err := repos.EventIDs.MarkSeen(ctx, "independent-event-a")
	if err != nil || !fresh1 {
		t.Fatalf("expected fresh=true for event-a, got fresh=%v err=%v", fresh1, err)
	}
	fresh2, err := repos.EventIDs.MarkSeen(ctx, "independent-event-b")
	if err != nil || !fresh2 {
		t.Fatalf("expected fresh=true for event-b (unrelated eventId), got fresh=%v err=%v", fresh2, err)
	}
}
