//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"analytics-service/app/analytics"
)

func TestBranchDays_SeededRows_DerivesAvgOrderMinor(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	branchID := int64(3001)
	date := "2026-03-09"
	if err := repos.BranchDay.ApplyOrderPlaced(ctx, branchID, date, "EGP", 3000); err != nil {
		t.Fatalf("seed placed: %v", err)
	}
	if err := repos.BranchDay.ApplyOrderPlaced(ctx, branchID, date, "EGP", 1000); err != nil {
		t.Fatalf("seed placed: %v", err)
	}

	rec, body := doGet(t, router, "/api/v1/analytics/branches/3001/days?from=2026-03-09&to=2026-03-09", token)
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
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].OrdersCount != 2 || resp.Data[0].RevenueMinor != 4000 || resp.Data[0].AvgOrderMinor != 2000 {
		t.Fatalf("expected orders=2 revenue=4000 avg=2000, got %+v", resp.Data)
	}
}

func TestBranchDays_UnknownBranch_EmptyResultNotError(t *testing.T) {
	router, token, _ := systemAdminRouter(t)

	rec, body := doGet(t, router, "/api/v1/analytics/branches/888888/days?from=2026-01-01&to=2026-12-31", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty array, got %d, body: %s", rec.Code, body)
	}
	var resp struct {
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("expected empty data, got %d rows", len(resp.Data))
	}
}

func TestProductDays_SeededViaOrderPlaced_DerivesAvgUnitPriceMinor(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	branchID, productID := int64(3002), int64(77)

	// Seed through the real fan-out path (BulkWrite, not a hand-written
	// bson doc) so the test also proves ApplyOrderPlacedItems' per-item
	// upsert shape.
	if err := repos.ProductDay.ApplyOrderPlacedItems(ctx, branchID, "2026-03-10", "EGP", []analytics.OrderPlacedItem{
		{ProductID: productID, Quantity: 3, LineTotalMinor: 900},
	}); err != nil {
		t.Fatalf("seed items: %v", err)
	}
	if err := repos.ProductDay.ApplyOrderPlacedItems(ctx, branchID, "2026-03-10", "EGP", []analytics.OrderPlacedItem{
		{ProductID: productID, Quantity: 1, LineTotalMinor: 300},
	}); err != nil {
		t.Fatalf("seed items: %v", err)
	}

	rec, body := doGet(t, router, "/api/v1/analytics/branches/3002/products/77/days?from=2026-03-10&to=2026-03-10", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Data []struct {
			QuantitySum       int64 `json:"quantitySum"`
			RevenueMinor      int64 `json:"revenueMinor"`
			AvgUnitPriceMinor int64 `json:"avgUnitPriceMinor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].QuantitySum != 4 || resp.Data[0].RevenueMinor != 1200 || resp.Data[0].AvgUnitPriceMinor != 300 {
		t.Fatalf("expected quantity=4 revenue=1200 avgUnitPrice=300 (1200/4), got %+v", resp.Data)
	}
}

func TestProductDays_UnknownProduct_EmptyResultNotError(t *testing.T) {
	router, token, _ := systemAdminRouter(t)

	rec, body := doGet(t, router, "/api/v1/analytics/branches/3002/products/999999/days?from=2026-01-01&to=2026-12-31", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty array, got %d, body: %s", rec.Code, body)
	}
	var resp struct {
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("expected empty data, got %d rows", len(resp.Data))
	}
}
