//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Complements the Phase 0/1 smoke tests in platform_summary_test.go (empty
// range + unauthenticated) with seeded-data coverage — Phase 3 items
// "platform: /days, /summary" plus the Phase 5 currency-mixing regression
// this schema was specifically designed to prevent (agg_platform_day is
// keyed by (date, currency), never just date — see entity/platform_day.go).

func TestPlatformDays_TwoCurrenciesOnSameDate_NeverSummedTogether(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	date := "2026-03-11"
	if err := repos.PlatformDay.ApplyOrderPlaced(ctx, date, "EGP", 1000); err != nil {
		t.Fatalf("seed EGP: %v", err)
	}
	if err := repos.PlatformDay.ApplyOrderPlaced(ctx, date, "SAR", 500); err != nil {
		t.Fatalf("seed SAR: %v", err)
	}

	rec, body := doGet(t, router, "/api/v1/analytics/platform/days?from=2026-03-11&to=2026-03-11", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Data []struct {
			Date         string `json:"date"`
			Currency     string `json:"currency"`
			OrdersCount  int64  `json:"ordersCount"`
			RevenueMinor int64  `json:"revenueMinor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 rows (one per currency), got %d: %+v", len(resp.Data), resp.Data)
	}

	byCurrency := map[string]int64{}
	for _, row := range resp.Data {
		if row.Date != date {
			t.Fatalf("expected every row dated %s, got %+v", date, row)
		}
		byCurrency[row.Currency] = row.RevenueMinor
	}
	if byCurrency["EGP"] != 1000 {
		t.Fatalf("expected EGP revenue 1000 (not summed with SAR), got %d", byCurrency["EGP"])
	}
	if byCurrency["SAR"] != 500 {
		t.Fatalf("expected SAR revenue 500 (not summed with EGP), got %d", byCurrency["SAR"])
	}
}

func TestPlatformSummary_TotalsPerCurrency(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	date := "2026-03-12"
	if err := repos.PlatformDay.ApplyOrderPlaced(ctx, date, "EGP", 2000); err != nil {
		t.Fatalf("seed placed: %v", err)
	}
	if err := repos.PlatformDay.ApplyOrderRejected(ctx, date, "EGP"); err != nil {
		t.Fatalf("seed rejected: %v", err)
	}
	if err := repos.PlatformDay.ApplyOrderDelivered(ctx, date, "EGP", 6000); err != nil {
		t.Fatalf("seed delivered: %v", err)
	}
	if err := repos.PlatformDay.ApplyPaymentCompleted(ctx, date, "EGP", 1500); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	rec, body := doGet(t, router, "/api/v1/analytics/platform/summary?from=2026-03-12&to=2026-03-12", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Data []struct {
			Currency                  string  `json:"currency"`
			OrdersCount               int64   `json:"ordersCount"`
			RevenueMinor              int64   `json:"revenueMinor"`
			FailedCount               int64   `json:"failedCount"`
			FailureRate               float64 `json:"failureRate"`
			DeliveredCount            int64   `json:"deliveredCount"`
			AvgDeliveryMs             int64   `json:"avgDeliveryMs"`
			OnlinePaymentsCount       int64   `json:"onlinePaymentsCount"`
			OnlinePaymentsAmountMinor int64   `json:"onlinePaymentsAmountMinor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 currency row, got %d: %+v", len(resp.Data), resp.Data)
	}
	row := resp.Data[0]
	if row.Currency != "EGP" || row.OrdersCount != 1 || row.RevenueMinor != 2000 {
		t.Fatalf("expected EGP orders=1 revenue=2000, got %+v", row)
	}
	if row.FailedCount != 1 || row.FailureRate != 1.0 {
		t.Fatalf("expected failedCount=1 failureRate=1.0 (1 failed / 1 order), got %+v", row)
	}
	if row.DeliveredCount != 1 || row.AvgDeliveryMs != 6000 {
		t.Fatalf("expected deliveredCount=1 avgDeliveryMs=6000, got %+v", row)
	}
	if row.OnlinePaymentsCount != 1 || row.OnlinePaymentsAmountMinor != 1500 {
		t.Fatalf("expected onlinePaymentsCount=1 onlinePaymentsAmountMinor=1500, got %+v", row)
	}
}

func TestPlatformDays_FromAfterTo_ValidationError(t *testing.T) {
	router, token, _ := systemAdminRouter(t)

	rec, body := doGet(t, router, "/api/v1/analytics/platform/days?from=2026-03-31&to=2026-03-01", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for from>to, got %d, body: %s", rec.Code, body)
	}
}
