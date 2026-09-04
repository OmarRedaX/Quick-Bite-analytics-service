//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"analytics-service/app/analytics"
	"analytics-service/app/analytics/service"
	"analytics-service/tests/integration/testutil"
)

// Event-to-Mongo path (Phase 3 item 5). eventhandlers' onOrderPlaced/etc.
// factories are unexported and only callable from an in-package test (see
// app/analytics/eventhandlers/handlers_test.go's Phase 2 coverage of JSON
// unmarshaling/field-mapping against fakes) — coreevents.Consumer has no
// exported "dispatch one event type" hook either. So this suite drives the
// same real *service.Service the handlers call into (service.New and every
// repository constructor are exported), against real Mongo, to verify the
// actual multi-collection fan-out and OrderContext hit/miss behavior end to
// end. What's deliberately NOT re-tested here: wire-shape JSON parsing —
// that's Phase 2's job.

func newTestService(t *testing.T) (*service.Service, *testutil.RepoBundle) {
	t.Helper()
	db := testutil.ConnectMongo(t)
	repos := testutil.NewRepoBundle(db)
	svc := service.New(repos.RestaurantDay, repos.BranchDay, repos.ProductDay, repos.PlatformDay, repos.OrderContext)
	return svc, repos
}

func TestEventToMongo_OrderPlaced_FansOutToAllFourAggregatesPlusOrderContext(t *testing.T) {
	svc, repos := newTestService(t)
	ctx := context.Background()

	placedAt := time.Date(2026, 3, 13, 11, 0, 0, 0, time.UTC)
	in := analytics.OnOrderPlacedInput{
		OrderID:      "evt-order-1",
		RestaurantID: 5001,
		BranchID:     6001,
		Currency:     "EGP",
		TotalMinor:   4000,
		PlacedAt:     placedAt,
		Items: []analytics.OrderPlacedItem{
			{ProductID: 701, Quantity: 2, LineTotalMinor: 4000},
		},
	}
	if err := svc.OnOrderPlaced(ctx, in); err != nil {
		t.Fatalf("OnOrderPlaced: %v", err)
	}

	restRows, err := repos.RestaurantDay.FindByDateRange(ctx, 5001, "2026-03-13", "2026-03-13")
	if err != nil || len(restRows) != 1 || restRows[0].RevenueSumMinor != 4000 {
		t.Fatalf("expected restaurant_day row with revenue 4000, got rows=%+v err=%v", restRows, err)
	}

	branchRows, err := repos.BranchDay.FindByDateRange(ctx, 6001, "2026-03-13", "2026-03-13")
	if err != nil || len(branchRows) != 1 || branchRows[0].RevenueSumMinor != 4000 {
		t.Fatalf("expected branch_day row with revenue 4000, got rows=%+v err=%v", branchRows, err)
	}

	productRows, err := repos.ProductDay.FindByDateRange(ctx, 6001, 701, "2026-03-13", "2026-03-13")
	if err != nil || len(productRows) != 1 || productRows[0].QuantitySum != 2 {
		t.Fatalf("expected product_day row with quantitySum 2, got rows=%+v err=%v", productRows, err)
	}

	platformRows, err := repos.PlatformDay.FindByDateRange(ctx, "2026-03-13", "2026-03-13")
	if err != nil || len(platformRows) != 1 || platformRows[0].RevenueSumMinor != 4000 {
		t.Fatalf("expected platform_day row with revenue 4000, got rows=%+v err=%v", platformRows, err)
	}

	orderCtx, found, err := repos.OrderContext.Find(ctx, "evt-order-1")
	if err != nil || !found || orderCtx.Currency != "EGP" {
		t.Fatalf("expected order_context row saved, got %+v found=%v err=%v", orderCtx, found, err)
	}
}

func TestEventToMongo_OrderDelivered_HitAppliesDeliveryDurationToPlacedDate(t *testing.T) {
	svc, repos := newTestService(t)
	ctx := context.Background()

	placedAt := time.Date(2026, 3, 14, 8, 0, 0, 0, time.UTC)
	if err := svc.OnOrderPlaced(ctx, analytics.OnOrderPlacedInput{
		OrderID: "evt-order-2", RestaurantID: 5002, BranchID: 6002, Currency: "EGP", TotalMinor: 1000, PlacedAt: placedAt,
	}); err != nil {
		t.Fatalf("seed OnOrderPlaced: %v", err)
	}

	deliveredAt := placedAt.Add(30 * time.Minute)
	if err := svc.OnOrderDelivered(ctx, analytics.OnOrderDeliveredInput{
		OrderID: "evt-order-2", RestaurantID: 5002, BranchID: 6002, Currency: "EGP", DeliveredAt: deliveredAt,
	}); err != nil {
		t.Fatalf("OnOrderDelivered: %v", err)
	}

	rows, err := repos.RestaurantDay.FindByDateRange(ctx, 5002, "2026-03-14", "2026-03-14")
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected 1 restaurant_day row on the placed date, got %+v err=%v", rows, err)
	}
	wantMs := int64(30 * time.Minute / time.Millisecond)
	if rows[0].DeliveryMsCount != 1 || rows[0].DeliveryMsSum != wantMs {
		t.Fatalf("expected deliveryMsCount=1 deliveryMsSum=%d, got %+v", wantMs, rows[0])
	}
}

func TestEventToMongo_OrderDelivered_MissIsNoop_NoRowCreated(t *testing.T) {
	svc, repos := newTestService(t)
	ctx := context.Background()

	// No prior order.placed for this order — OrderContext lookup misses.
	if err := svc.OnOrderDelivered(ctx, analytics.OnOrderDeliveredInput{
		OrderID: "never-placed", RestaurantID: 5003, BranchID: 6003, Currency: "EGP", DeliveredAt: time.Now(),
	}); err != nil {
		t.Fatalf("expected a no-op (nil error) on OrderContext miss, got %v", err)
	}

	rows, err := repos.RestaurantDay.FindByDateRange(ctx, 5003, "2000-01-01", "2100-01-01")
	if err != nil {
		t.Fatalf("FindByDateRange: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected zero restaurant_day rows created for a delivery with no matching order_context, got %+v", rows)
	}
}

func TestEventToMongo_OrderRejected_PlatformDaySkippedOnOrderContextMiss(t *testing.T) {
	svc, repos := newTestService(t)
	ctx := context.Background()

	occurredAt := time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC)
	if err := svc.OnOrderRejected(ctx, analytics.OnOrderRejectedInput{
		OrderID: "never-placed-2", RestaurantID: 5004, BranchID: 6004, OccurredAt: occurredAt,
	}); err != nil {
		t.Fatalf("expected a no-op (nil error) on OrderContext miss, got %v", err)
	}

	restRows, err := repos.RestaurantDay.FindByDateRange(ctx, 5004, "2026-03-15", "2026-03-15")
	if err != nil || len(restRows) != 1 || restRows[0].FailedCount != 1 {
		t.Fatalf("expected restaurant_day still updated (rejection doesn't depend on order_context), got %+v err=%v", restRows, err)
	}

	platformRows, err := repos.PlatformDay.FindByDateRange(ctx, "2026-03-15", "2026-03-15")
	if err != nil {
		t.Fatalf("FindByDateRange: %v", err)
	}
	if len(platformRows) != 0 {
		t.Fatalf("expected zero platform_day rows (currency unknown without order_context), got %+v", platformRows)
	}
}
