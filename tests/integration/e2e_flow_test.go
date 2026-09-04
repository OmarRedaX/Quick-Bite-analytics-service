//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"analytics-service/app/analytics"
	"analytics-service/app/analytics/service"
)

// Phase 4 — the one real user-facing "flow" this service has: event ->
// aggregate -> read. Unlike events_test.go (Phase 3 item 5), which drives
// *service.Service and asserts against repository reads directly to prove
// multi-collection fan-out, these tests go through the real chi router via
// HTTP (httptest) for the read half, proving the row an event just wrote is
// actually visible through the public API a caller would hit — the
// distinction Phase 4 in testing-implementation-plan.md calls for.

func TestFlow_OrderPlaced_ThenGetRestaurantDays_RowIsVisible(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	svc := service.New(repos.RestaurantDay, repos.BranchDay, repos.ProductDay, repos.PlatformDay, repos.OrderContext)

	restaurantID := int64(9001)
	branchID := int64(9101)
	placedAt := time.Date(2026, 3, 20, 13, 30, 0, 0, time.UTC)

	if err := svc.OnOrderPlaced(ctx, analytics.OnOrderPlacedInput{
		OrderID:      "flow-order-1",
		RestaurantID: restaurantID,
		BranchID:     branchID,
		Currency:     "EGP",
		TotalMinor:   2500,
		PlacedAt:     placedAt,
		Items: []analytics.OrderPlacedItem{
			{ProductID: 8001, Quantity: 1, LineTotalMinor: 2500},
		},
	}); err != nil {
		t.Fatalf("OnOrderPlaced: %v", err)
	}

	rec, body := doGet(t, router, "/api/v1/analytics/restaurants/9001/days?from=2026-03-20&to=2026-03-20", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Data []struct {
			Date          string `json:"date"`
			OrdersCount   int64  `json:"ordersCount"`
			RevenueMinor  int64  `json:"revenueMinor"`
			AvgOrderMinor int64  `json:"avgOrderMinor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v, body: %s", err, body)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected the just-placed order visible as 1 day row, got %d: %s", len(resp.Data), body)
	}
	row := resp.Data[0]
	if row.Date != "2026-03-20" || row.OrdersCount != 1 || row.RevenueMinor != 2500 || row.AvgOrderMinor != 2500 {
		t.Fatalf("expected date=2026-03-20 orders=1 revenue=2500 avg=2500, got %+v", row)
	}
}

func TestFlow_OrderPlacedThenDelivered_BucketsOnPlacedDate_VisibleViaDeliveryAvgEndpoint(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	svc := service.New(repos.RestaurantDay, repos.BranchDay, repos.ProductDay, repos.PlatformDay, repos.OrderContext)

	restaurantID := int64(9002)
	branchID := int64(9102)
	// Placed just before midnight UTC, delivered the next calendar day — the
	// trickiest cross-event behavior this service has: the duration must
	// still land on the *placed* date, not the delivered date, when read
	// back through the public endpoint.
	placedAt := time.Date(2026, 3, 20, 23, 55, 0, 0, time.UTC)
	deliveredAt := time.Date(2026, 3, 21, 0, 25, 0, 0, time.UTC) // 30 min later

	if err := svc.OnOrderPlaced(ctx, analytics.OnOrderPlacedInput{
		OrderID:      "flow-order-2",
		RestaurantID: restaurantID,
		BranchID:     branchID,
		Currency:     "EGP",
		TotalMinor:   1000,
		PlacedAt:     placedAt,
	}); err != nil {
		t.Fatalf("OnOrderPlaced: %v", err)
	}

	if err := svc.OnOrderDelivered(ctx, analytics.OnOrderDeliveredInput{
		OrderID:      "flow-order-2",
		RestaurantID: restaurantID,
		BranchID:     branchID,
		Currency:     "EGP",
		DeliveredAt:  deliveredAt,
	}); err != nil {
		t.Fatalf("OnOrderDelivered: %v", err)
	}

	// Query the placed date (2026-03-20), not the delivered date — the row
	// should be here, not on 2026-03-21.
	rec, body := doGet(t, router, "/api/v1/analytics/restaurants/9002/delivery-avg?from=2026-03-20&to=2026-03-20", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Data []struct {
			Date           string `json:"date"`
			DeliveredCount int64  `json:"deliveredCount"`
			AvgDeliveryMs  int64  `json:"avgDeliveryMs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v, body: %s", err, body)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected delivery duration bucketed on the placed date (2026-03-20), got %d rows: %s", len(resp.Data), body)
	}
	row := resp.Data[0]
	wantMs := int64(30 * time.Minute / time.Millisecond)
	if row.Date != "2026-03-20" || row.DeliveredCount != 1 || row.AvgDeliveryMs != wantMs {
		t.Fatalf("expected date=2026-03-20 deliveredCount=1 avgDeliveryMs=%d, got %+v", wantMs, row)
	}

	// Confirm nothing landed on the delivered date instead.
	rec2, body2 := doGet(t, router, "/api/v1/analytics/restaurants/9002/delivery-avg?from=2026-03-21&to=2026-03-21", token)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec2.Code, body2)
	}
	var resp2 struct {
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(body2, &resp2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp2.Data) != 0 {
		t.Fatalf("expected no row on the delivered date, got %d: %s", len(resp2.Data), body2)
	}
}
