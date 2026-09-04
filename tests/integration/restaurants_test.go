//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"analytics-service/app/analytics"
	"analytics-service/tests/integration/testutil"
)

// systemAdminRouter builds a router + bearer token for a system_admin
// caller — the RBAC bypass path, so these data/shape tests don't need a
// FixedPermissionsCache configured with the right permission. Auth/RBAC
// itself is covered exhaustively in auth_rbac_test.go.
func systemAdminRouter(t *testing.T) (http.Handler, string, *testutil.RepoBundle) {
	t.Helper()
	db := testutil.ConnectMongo(t)
	router := testutil.NewRouter(db, testutil.TestAccessSecret, testutil.FixedPermissionsCache())
	token := testutil.MintAccessToken(t, 1, "admin@quickbite.test", "system_admin")
	return router, token, testutil.NewRepoBundle(db)
}

func doGet(t *testing.T, router http.Handler, path, token string) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec, rec.Body.Bytes()
}

func TestRestaurantDays_SeededRows_DerivesAvgOrderMinor(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	restaurantID := int64(1001)
	placedAt := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		if err := repos.RestaurantDay.ApplyOrderPlaced(ctx, analytics.OnOrderPlacedInput{
			RestaurantID: restaurantID,
			Currency:     "EGP",
			TotalMinor:   1000,
			PlacedAt:     placedAt,
		}); err != nil {
			t.Fatalf("seed ApplyOrderPlaced: %v", err)
		}
	}

	rec, body := doGet(t, router, "/api/v1/analytics/restaurants/1001/days?from=2026-03-05&to=2026-03-05", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
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
		t.Fatalf("expected 1 day row, got %d: %s", len(resp.Data), body)
	}
	row := resp.Data[0]
	if row.Date != "2026-03-05" || row.OrdersCount != 2 || row.RevenueMinor != 2000 || row.AvgOrderMinor != 1000 {
		t.Fatalf("expected date=2026-03-05 orders=2 revenue=2000 avg=1000, got %+v", row)
	}
}

func TestRestaurantDays_UnknownRestaurant_EmptyResultNotError(t *testing.T) {
	router, token, _ := systemAdminRouter(t)

	rec, body := doGet(t, router, "/api/v1/analytics/restaurants/999999/days?from=2026-01-01&to=2026-12-31", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with an empty array (not 404) for an id with no rows, got %d, body: %s", rec.Code, body)
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

func TestRestaurantDays_FromAfterTo_ValidationError(t *testing.T) {
	router, token, _ := systemAdminRouter(t)

	rec, body := doGet(t, router, "/api/v1/analytics/restaurants/1001/days?from=2026-03-31&to=2026-03-01", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for from>to, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error.Code != "ANALYTICS_INVALID_DATE_RANGE" {
		t.Fatalf("expected code ANALYTICS_INVALID_DATE_RANGE, got %s", resp.Error.Code)
	}
}

func TestRestaurantFailures_DerivesFailureRate(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	restaurantID := int64(1002)
	placedAt := time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC)
	date := "2026-03-06"
	for i := 0; i < 4; i++ {
		if err := repos.RestaurantDay.ApplyOrderPlaced(ctx, analytics.OnOrderPlacedInput{RestaurantID: restaurantID, Currency: "EGP", TotalMinor: 500, PlacedAt: placedAt}); err != nil {
			t.Fatalf("seed placed: %v", err)
		}
	}
	if err := repos.RestaurantDay.ApplyOrderRejected(ctx, restaurantID, date); err != nil {
		t.Fatalf("seed rejected: %v", err)
	}

	rec, body := doGet(t, router, "/api/v1/analytics/restaurants/1002/failures?from=2026-03-06&to=2026-03-06", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Data []struct {
			FailedCount int64   `json:"failedCount"`
			OrdersCount int64   `json:"ordersCount"`
			FailureRate float64 `json:"failureRate"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].FailedCount != 1 || resp.Data[0].OrdersCount != 4 || resp.Data[0].FailureRate != 0.25 {
		t.Fatalf("expected 1 failed / 4 orders / rate 0.25, got %+v", resp.Data)
	}
}

func TestRestaurantDeliveryAvg_DerivesAvgDeliveryMs(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	restaurantID := int64(1003)
	date := "2026-03-07"
	if err := repos.RestaurantDay.ApplyOrderDelivered(ctx, restaurantID, date, 4000); err != nil {
		t.Fatalf("seed delivered: %v", err)
	}
	if err := repos.RestaurantDay.ApplyOrderDelivered(ctx, restaurantID, date, 2000); err != nil {
		t.Fatalf("seed delivered: %v", err)
	}

	rec, body := doGet(t, router, "/api/v1/analytics/restaurants/1003/delivery-avg?from=2026-03-07&to=2026-03-07", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Data []struct {
			DeliveredCount int64 `json:"deliveredCount"`
			AvgDeliveryMs  int64 `json:"avgDeliveryMs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].DeliveredCount != 2 || resp.Data[0].AvgDeliveryMs != 3000 {
		t.Fatalf("expected deliveredCount=2 avgDeliveryMs=3000 ((4000+2000)/2), got %+v", resp.Data)
	}
}

func TestActiveRestaurants_CountsDistinctRestaurantsInRange(t *testing.T) {
	router, token, repos := systemAdminRouter(t)
	ctx := context.Background()

	placedAt := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)
	for _, restaurantID := range []int64{2001, 2002, 2001} { // 2001 twice -> still counts once
		if err := repos.RestaurantDay.ApplyOrderPlaced(ctx, analytics.OnOrderPlacedInput{RestaurantID: restaurantID, Currency: "EGP", TotalMinor: 100, PlacedAt: placedAt}); err != nil {
			t.Fatalf("seed placed: %v", err)
		}
	}

	rec, body := doGet(t, router, "/api/v1/analytics/restaurants/active?from=2026-03-08&to=2026-03-08", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, body)
	}

	var resp struct {
		Data struct {
			Count int64 `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Count != 2 {
		t.Fatalf("expected count 2 (distinct restaurants 2001, 2002), got %d", resp.Data.Count)
	}
}
