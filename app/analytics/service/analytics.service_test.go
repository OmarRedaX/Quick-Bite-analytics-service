package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"analytics-service/app/analytics"
	"analytics-service/app/analytics/entity"
)

// Colocated unit test — same package, so it can construct *Service directly
// and satisfy the five narrow, unexported repo interfaces declared in
// analytics.service.go with small in-package fakes. No Mongo needed: every
// dependency is a plain Go interface. See lib/rbac/cache_test.go for the
// same pattern applied to a simpler package.

type fakeRestaurantDayRepo struct {
	applyOrderPlacedCalls    []analytics.OnOrderPlacedInput
	applyOrderPlacedErr      error
	applyOrderDeliveredCalls []struct {
		RestaurantID int64
		Date         string
		DeliveryMs   int64
	}
	applyOrderDeliveredErr  error
	applyOrderRejectedCalls []struct {
		RestaurantID int64
		Date         string
	}
	applyOrderRejectedErr error
	countActiveResult     int64
	countActiveErr        error
	findRows              []entity.RestaurantDay
	findErr               error
}

func (f *fakeRestaurantDayRepo) ApplyOrderPlaced(_ context.Context, in analytics.OnOrderPlacedInput) error {
	f.applyOrderPlacedCalls = append(f.applyOrderPlacedCalls, in)
	return f.applyOrderPlacedErr
}

func (f *fakeRestaurantDayRepo) ApplyOrderDelivered(_ context.Context, restaurantID int64, date string, deliveryMs int64) error {
	f.applyOrderDeliveredCalls = append(f.applyOrderDeliveredCalls, struct {
		RestaurantID int64
		Date         string
		DeliveryMs   int64
	}{restaurantID, date, deliveryMs})
	return f.applyOrderDeliveredErr
}

func (f *fakeRestaurantDayRepo) ApplyOrderRejected(_ context.Context, restaurantID int64, date string) error {
	f.applyOrderRejectedCalls = append(f.applyOrderRejectedCalls, struct {
		RestaurantID int64
		Date         string
	}{restaurantID, date})
	return f.applyOrderRejectedErr
}

func (f *fakeRestaurantDayRepo) CountActiveInRange(_ context.Context, _, _ string) (int64, error) {
	return f.countActiveResult, f.countActiveErr
}

func (f *fakeRestaurantDayRepo) FindByDateRange(_ context.Context, _ int64, _, _ string) ([]entity.RestaurantDay, error) {
	return f.findRows, f.findErr
}

type fakeBranchDayRepo struct {
	applyOrderPlacedCalls    int
	applyOrderPlacedErr      error
	applyOrderDeliveredCalls []struct {
		BranchID   int64
		Date       string
		DeliveryMs int64
	}
	applyOrderDeliveredErr  error
	applyOrderRejectedCalls int
	applyOrderRejectedErr   error
	findRows                []entity.BranchDay
	findErr                 error
}

func (f *fakeBranchDayRepo) ApplyOrderPlaced(_ context.Context, _ int64, _, _ string, _ int64) error {
	f.applyOrderPlacedCalls++
	return f.applyOrderPlacedErr
}

func (f *fakeBranchDayRepo) ApplyOrderDelivered(_ context.Context, branchID int64, date string, deliveryMs int64) error {
	f.applyOrderDeliveredCalls = append(f.applyOrderDeliveredCalls, struct {
		BranchID   int64
		Date       string
		DeliveryMs int64
	}{branchID, date, deliveryMs})
	return f.applyOrderDeliveredErr
}

func (f *fakeBranchDayRepo) ApplyOrderRejected(_ context.Context, _ int64, _ string) error {
	f.applyOrderRejectedCalls++
	return f.applyOrderRejectedErr
}

func (f *fakeBranchDayRepo) FindByDateRange(_ context.Context, _ int64, _, _ string) ([]entity.BranchDay, error) {
	return f.findRows, f.findErr
}

type fakeProductDayRepo struct {
	applyOrderPlacedItemsCalls int
	applyOrderPlacedItemsErr   error
	findRows                   []entity.ProductDay
	findErr                    error
}

func (f *fakeProductDayRepo) ApplyOrderPlacedItems(_ context.Context, _ int64, _, _ string, _ []analytics.OrderPlacedItem) error {
	f.applyOrderPlacedItemsCalls++
	return f.applyOrderPlacedItemsErr
}

func (f *fakeProductDayRepo) FindByDateRange(_ context.Context, _, _ int64, _, _ string) ([]entity.ProductDay, error) {
	return f.findRows, f.findErr
}

type fakePlatformDayRepo struct {
	applyOrderPlacedCalls    int
	applyOrderPlacedErr      error
	applyOrderDeliveredCalls []struct {
		Date       string
		Currency   string
		DeliveryMs int64
	}
	applyOrderDeliveredErr  error
	applyOrderRejectedCalls []struct {
		Date     string
		Currency string
	}
	applyOrderRejectedErr      error
	applyPaymentCompletedCalls int
	applyPaymentCompletedErr   error
	findRows                   []entity.PlatformDay
	findErr                    error
	summaryRows                []entity.PlatformDayCurrencyTotals
	summaryErr                 error
}

func (f *fakePlatformDayRepo) ApplyOrderPlaced(_ context.Context, _, _ string, _ int64) error {
	f.applyOrderPlacedCalls++
	return f.applyOrderPlacedErr
}

func (f *fakePlatformDayRepo) ApplyOrderDelivered(_ context.Context, date, currency string, deliveryMs int64) error {
	f.applyOrderDeliveredCalls = append(f.applyOrderDeliveredCalls, struct {
		Date       string
		Currency   string
		DeliveryMs int64
	}{date, currency, deliveryMs})
	return f.applyOrderDeliveredErr
}

func (f *fakePlatformDayRepo) ApplyOrderRejected(_ context.Context, date, currency string) error {
	f.applyOrderRejectedCalls = append(f.applyOrderRejectedCalls, struct {
		Date     string
		Currency string
	}{date, currency})
	return f.applyOrderRejectedErr
}

func (f *fakePlatformDayRepo) ApplyPaymentCompleted(_ context.Context, _, _ string, _ int64) error {
	f.applyPaymentCompletedCalls++
	return f.applyPaymentCompletedErr
}

func (f *fakePlatformDayRepo) FindByDateRange(_ context.Context, _, _ string) ([]entity.PlatformDay, error) {
	return f.findRows, f.findErr
}

func (f *fakePlatformDayRepo) SummaryByCurrency(_ context.Context, _, _ string) ([]entity.PlatformDayCurrencyTotals, error) {
	return f.summaryRows, f.summaryErr
}

type fakeOrderContextRepo struct {
	saveCalls int
	saveErr   error
	findRow   entity.OrderContext
	findFound bool
	findErr   error
}

func (f *fakeOrderContextRepo) Save(_ context.Context, _, _ string, _ time.Time) error {
	f.saveCalls++
	return f.saveErr
}

func (f *fakeOrderContextRepo) Find(_ context.Context, _ string) (entity.OrderContext, bool, error) {
	return f.findRow, f.findFound, f.findErr
}

// newTestService wires a Service against fresh fakes for one test, and
// returns the fakes so the test can both configure return values and assert
// on calls made.
func newTestService() (*Service, *fakeRestaurantDayRepo, *fakeBranchDayRepo, *fakeProductDayRepo, *fakePlatformDayRepo, *fakeOrderContextRepo) {
	rest := &fakeRestaurantDayRepo{}
	branch := &fakeBranchDayRepo{}
	product := &fakeProductDayRepo{}
	platform := &fakePlatformDayRepo{}
	orderCtx := &fakeOrderContextRepo{}
	return New(rest, branch, product, platform, orderCtx), rest, branch, product, platform, orderCtx
}

func TestService_OnOrderPlaced_FansOutToAllAggregates(t *testing.T) {
	svc, rest, branch, product, platform, orderCtx := newTestService()

	placedAt := time.Date(2026, 3, 5, 10, 30, 0, 0, time.UTC)
	in := analytics.OnOrderPlacedInput{
		OrderID:      "order-1",
		RestaurantID: 10,
		BranchID:     20,
		Currency:     "EGP",
		TotalMinor:   5000,
		PlacedAt:     placedAt,
		Items: []analytics.OrderPlacedItem{
			{ProductID: 1, Quantity: 2, LineTotalMinor: 5000},
		},
	}

	if err := svc.OnOrderPlaced(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rest.applyOrderPlacedCalls) != 1 || rest.applyOrderPlacedCalls[0].OrderID != "order-1" {
		t.Fatalf("expected restaurantDays.ApplyOrderPlaced called once with the input, got %+v", rest.applyOrderPlacedCalls)
	}
	if branch.applyOrderPlacedCalls != 1 {
		t.Fatalf("expected branchDays.ApplyOrderPlaced called once, got %d", branch.applyOrderPlacedCalls)
	}
	if product.applyOrderPlacedItemsCalls != 1 {
		t.Fatalf("expected productDays.ApplyOrderPlacedItems called once, got %d", product.applyOrderPlacedItemsCalls)
	}
	if platform.applyOrderPlacedCalls != 1 {
		t.Fatalf("expected platformDays.ApplyOrderPlaced called once, got %d", platform.applyOrderPlacedCalls)
	}
	if orderCtx.saveCalls != 1 {
		t.Fatalf("expected orderContexts.Save called once, got %d", orderCtx.saveCalls)
	}
}

func TestService_OnOrderPlaced_ShortCircuitsOnFirstFailure(t *testing.T) {
	svc, rest, branch, product, platform, orderCtx := newTestService()
	rest.applyOrderPlacedErr = errors.New("boom")

	err := svc.OnOrderPlaced(context.Background(), analytics.OnOrderPlacedInput{PlacedAt: time.Now()})
	if err == nil {
		t.Fatal("expected error to propagate from restaurantDays.ApplyOrderPlaced")
	}
	if branch.applyOrderPlacedCalls != 0 || product.applyOrderPlacedItemsCalls != 0 || platform.applyOrderPlacedCalls != 0 || orderCtx.saveCalls != 0 {
		t.Fatalf("expected no downstream repo calls after the first failure, got branch=%d product=%d platform=%d orderCtx=%d",
			branch.applyOrderPlacedCalls, product.applyOrderPlacedItemsCalls, platform.applyOrderPlacedCalls, orderCtx.saveCalls)
	}
}

func TestService_OnOrderDelivered_HitAppliesToAllThreeAggregates(t *testing.T) {
	svc, rest, branch, _, platform, orderCtx := newTestService()

	placedAt := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	deliveredAt := placedAt.Add(45 * time.Minute)
	orderCtx.findFound = true
	orderCtx.findRow = entity.OrderContext{OrderID: "order-1", Currency: "EGP", PlacedAt: placedAt}

	err := svc.OnOrderDelivered(context.Background(), analytics.OnOrderDeliveredInput{
		OrderID:      "order-1",
		RestaurantID: 10,
		BranchID:     20,
		Currency:     "EGP",
		DeliveredAt:  deliveredAt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantMs := int64(45 * time.Minute / time.Millisecond)
	if len(rest.applyOrderDeliveredCalls) != 1 || rest.applyOrderDeliveredCalls[0].DeliveryMs != wantMs {
		t.Fatalf("expected restaurantDays delivery ms %d, got %+v", wantMs, rest.applyOrderDeliveredCalls)
	}
	if len(rest.applyOrderDeliveredCalls) != 1 || rest.applyOrderDeliveredCalls[0].Date != "2026-03-05" {
		t.Fatalf("expected date bucketed on placedAt (2026-03-05), got %+v", rest.applyOrderDeliveredCalls)
	}
	if len(branch.applyOrderDeliveredCalls) != 1 || branch.applyOrderDeliveredCalls[0].DeliveryMs != wantMs {
		t.Fatalf("expected branchDays delivery ms %d, got %+v", wantMs, branch.applyOrderDeliveredCalls)
	}
	if len(platform.applyOrderDeliveredCalls) != 1 || platform.applyOrderDeliveredCalls[0].DeliveryMs != wantMs {
		t.Fatalf("expected platformDays delivery ms %d, got %+v", wantMs, platform.applyOrderDeliveredCalls)
	}
}

func TestService_OnOrderDelivered_NegativeDurationClampedToZero(t *testing.T) {
	svc, rest, _, _, _, orderCtx := newTestService()

	placedAt := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	orderCtx.findFound = true
	orderCtx.findRow = entity.OrderContext{PlacedAt: placedAt}

	// deliveredAt before placedAt (clock skew / out-of-order delivery) must
	// never produce a negative delivery duration.
	err := svc.OnOrderDelivered(context.Background(), analytics.OnOrderDeliveredInput{
		DeliveredAt: placedAt.Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rest.applyOrderDeliveredCalls) != 1 || rest.applyOrderDeliveredCalls[0].DeliveryMs != 0 {
		t.Fatalf("expected delivery ms clamped to 0, got %+v", rest.applyOrderDeliveredCalls)
	}
}

func TestService_OnOrderDelivered_MissIsNoopNotError(t *testing.T) {
	svc, rest, branch, _, platform, orderCtx := newTestService()
	orderCtx.findFound = false // no order_context row (out-of-order or TTL-expired)

	err := svc.OnOrderDelivered(context.Background(), analytics.OnOrderDeliveredInput{OrderID: "unknown"})
	if err != nil {
		t.Fatalf("expected a lookup miss to be a no-op, not an error, got %v", err)
	}
	if len(rest.applyOrderDeliveredCalls) != 0 || len(branch.applyOrderDeliveredCalls) != 0 || len(platform.applyOrderDeliveredCalls) != 0 {
		t.Fatalf("expected zero downstream calls on a miss, got rest=%d branch=%d platform=%d",
			len(rest.applyOrderDeliveredCalls), len(branch.applyOrderDeliveredCalls), len(platform.applyOrderDeliveredCalls))
	}
}

func TestService_OnOrderDelivered_OrderContextLookupError(t *testing.T) {
	svc, rest, _, _, _, orderCtx := newTestService()
	orderCtx.findErr = errors.New("mongo down")

	err := svc.OnOrderDelivered(context.Background(), analytics.OnOrderDeliveredInput{})
	if err == nil {
		t.Fatal("expected the order_context lookup error to propagate, not be swallowed")
	}
	if len(rest.applyOrderDeliveredCalls) != 0 {
		t.Fatalf("expected no downstream calls when the lookup itself errors, got %+v", rest.applyOrderDeliveredCalls)
	}
}

func TestService_OnOrderRejected_AppliesRestaurantAndBranchRegardlessOfOrderContext(t *testing.T) {
	svc, rest, branch, _, platform, orderCtx := newTestService()
	orderCtx.findFound = false

	occurredAt := time.Date(2026, 3, 6, 8, 0, 0, 0, time.UTC)
	err := svc.OnOrderRejected(context.Background(), analytics.OnOrderRejectedInput{
		OrderID:      "order-2",
		RestaurantID: 10,
		BranchID:     20,
		OccurredAt:   occurredAt,
	})
	if err != nil {
		t.Fatalf("expected a platform-day miss to be a no-op, not an error, got %v", err)
	}
	if len(rest.applyOrderRejectedCalls) != 1 || rest.applyOrderRejectedCalls[0].Date != "2026-03-06" {
		t.Fatalf("expected restaurantDays.ApplyOrderRejected bucketed on OccurredAt, got %+v", rest.applyOrderRejectedCalls)
	}
	if branch.applyOrderRejectedCalls != 1 {
		t.Fatalf("expected branchDays.ApplyOrderRejected called once, got %d", branch.applyOrderRejectedCalls)
	}
	if platform.applyOrderRejectedCalls != nil {
		t.Fatalf("expected platformDays.ApplyOrderRejected skipped on order_context miss, got %+v", platform.applyOrderRejectedCalls)
	}
}

func TestService_OnOrderRejected_AppliesPlatformDayOnOrderContextHit(t *testing.T) {
	svc, _, _, _, platform, orderCtx := newTestService()
	orderCtx.findFound = true
	orderCtx.findRow = entity.OrderContext{Currency: "SAR"}

	occurredAt := time.Date(2026, 3, 6, 8, 0, 0, 0, time.UTC)
	if err := svc.OnOrderRejected(context.Background(), analytics.OnOrderRejectedInput{OccurredAt: occurredAt}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(platform.applyOrderRejectedCalls) != 1 || platform.applyOrderRejectedCalls[0].Currency != "SAR" {
		t.Fatalf("expected platformDays.ApplyOrderRejected called with the order_context currency, got %+v", platform.applyOrderRejectedCalls)
	}
}

func TestService_OnPaymentCompleted_AppliesToPlatformDayOnly(t *testing.T) {
	svc, rest, branch, product, platform, _ := newTestService()

	err := svc.OnPaymentCompleted(context.Background(), analytics.OnPaymentCompletedInput{
		Currency:    "EGP",
		AmountMinor: 1000,
		CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform.applyPaymentCompletedCalls != 1 {
		t.Fatalf("expected platformDays.ApplyPaymentCompleted called once, got %d", platform.applyPaymentCompletedCalls)
	}
	if len(rest.applyOrderPlacedCalls) != 0 || branch.applyOrderPlacedCalls != 0 || product.applyOrderPlacedItemsCalls != 0 {
		t.Fatal("expected payment.completed to never touch restaurant/branch/product aggregates")
	}
}

// TestService_GetMethods_RejectInvertedDateRange table-drives the
// from>to -> ErrInvalidDateRange guard shared by every Get* method.
func TestService_GetMethods_RejectInvertedDateRange(t *testing.T) {
	svc, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	from, to := "2026-03-10", "2026-03-01"

	cases := []struct {
		name string
		call func() error
	}{
		{"GetRestaurantDays", func() error { _, err := svc.GetRestaurantDays(ctx, 1, from, to); return err }},
		{"GetBranchDays", func() error { _, err := svc.GetBranchDays(ctx, 1, from, to); return err }},
		{"GetProductDays", func() error { _, err := svc.GetProductDays(ctx, 1, 2, from, to); return err }},
		{"GetRestaurantFailures", func() error { _, err := svc.GetRestaurantFailures(ctx, 1, from, to); return err }},
		{"GetRestaurantDeliveryAvg", func() error { _, err := svc.GetRestaurantDeliveryAvg(ctx, 1, from, to); return err }},
		{"GetActiveRestaurants", func() error { _, err := svc.GetActiveRestaurants(ctx, from, to); return err }},
		{"GetPlatformDays", func() error { _, err := svc.GetPlatformDays(ctx, from, to); return err }},
		{"GetPlatformSummary", func() error { _, err := svc.GetPlatformSummary(ctx, from, to); return err }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, analytics.ErrInvalidDateRange) {
				t.Fatalf("expected ErrInvalidDateRange, got %v", err)
			}
		})
	}
}

func TestService_GetRestaurantDays_DerivesAvgOrderMinor(t *testing.T) {
	svc, rest, _, _, _, _ := newTestService()
	rest.findRows = []entity.RestaurantDay{
		{Date: "2026-03-01", Currency: "EGP", OrdersCount: 4, RevenueSumMinor: 4000},
		{Date: "2026-03-02", Currency: "EGP", OrdersCount: 0, RevenueSumMinor: 0}, // div-by-zero guard
	}

	rows, err := svc.GetRestaurantDays(context.Background(), 1, "2026-03-01", "2026-03-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].AvgOrderMinor != 1000 {
		t.Fatalf("expected avgOrderMinor 1000 (4000/4), got %d", rows[0].AvgOrderMinor)
	}
	if rows[1].AvgOrderMinor != 0 {
		t.Fatalf("expected avgOrderMinor 0 when ordersCount is 0 (no div-by-zero panic), got %d", rows[1].AvgOrderMinor)
	}
}

func TestService_GetBranchDays_DerivesAvgOrderMinor(t *testing.T) {
	svc, _, branch, _, _, _ := newTestService()
	branch.findRows = []entity.BranchDay{
		{Date: "2026-03-01", Currency: "EGP", OrdersCount: 5, RevenueSumMinor: 2500},
	}

	rows, err := svc.GetBranchDays(context.Background(), 1, "2026-03-01", "2026-03-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || rows[0].AvgOrderMinor != 500 {
		t.Fatalf("expected avgOrderMinor 500 (2500/5), got %+v", rows)
	}
}

func TestService_GetProductDays_DerivesAvgUnitPriceMinor(t *testing.T) {
	svc, _, _, product, _, _ := newTestService()
	product.findRows = []entity.ProductDay{
		{Date: "2026-03-01", Currency: "EGP", QuantitySum: 10, RevenueSumMinor: 1000},
		{Date: "2026-03-02", Currency: "EGP", QuantitySum: 0, RevenueSumMinor: 0},
	}

	rows, err := svc.GetProductDays(context.Background(), 1, 2, "2026-03-01", "2026-03-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows[0].AvgUnitPriceMinor != 100 {
		t.Fatalf("expected avgUnitPriceMinor 100 (1000/10), got %d", rows[0].AvgUnitPriceMinor)
	}
	if rows[1].AvgUnitPriceMinor != 0 {
		t.Fatalf("expected avgUnitPriceMinor 0 when quantitySum is 0, got %d", rows[1].AvgUnitPriceMinor)
	}
}

func TestService_GetRestaurantFailures_DerivesFailureRate(t *testing.T) {
	svc, rest, _, _, _, _ := newTestService()
	rest.findRows = []entity.RestaurantDay{
		{Date: "2026-03-01", OrdersCount: 10, FailedCount: 3},
		{Date: "2026-03-02", OrdersCount: 0, FailedCount: 0},
	}

	rows, err := svc.GetRestaurantFailures(context.Background(), 1, "2026-03-01", "2026-03-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows[0].FailureRate != 0.3 {
		t.Fatalf("expected failureRate 0.3 (3/10), got %v", rows[0].FailureRate)
	}
	if rows[1].FailureRate != 0 {
		t.Fatalf("expected failureRate 0 when ordersCount is 0, got %v", rows[1].FailureRate)
	}
}

func TestService_GetRestaurantDeliveryAvg_DerivesAvgDeliveryMs(t *testing.T) {
	svc, rest, _, _, _, _ := newTestService()
	rest.findRows = []entity.RestaurantDay{
		{Date: "2026-03-01", DeliveryMsSum: 9000, DeliveryMsCount: 3},
		{Date: "2026-03-02", DeliveryMsSum: 0, DeliveryMsCount: 0},
	}

	rows, err := svc.GetRestaurantDeliveryAvg(context.Background(), 1, "2026-03-01", "2026-03-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows[0].AvgDeliveryMs != 3000 {
		t.Fatalf("expected avgDeliveryMs 3000 (9000/3), got %d", rows[0].AvgDeliveryMs)
	}
	if rows[1].AvgDeliveryMs != 0 {
		t.Fatalf("expected avgDeliveryMs 0 when deliveryMsCount is 0, got %d", rows[1].AvgDeliveryMs)
	}
}

func TestService_GetActiveRestaurants_DelegatesToRepo(t *testing.T) {
	svc, rest, _, _, _, _ := newTestService()
	rest.countActiveResult = 7

	count, err := svc.GetActiveRestaurants(context.Background(), "2026-03-01", "2026-03-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 7 {
		t.Fatalf("expected count 7, got %d", count)
	}
}

func TestService_GetPlatformDays_DerivesAvgOrderMinor(t *testing.T) {
	svc, _, _, _, platform, _ := newTestService()
	platform.findRows = []entity.PlatformDay{
		{Date: "2026-03-01", Currency: "EGP", OrdersCount: 2, RevenueSumMinor: 400},
		{Date: "2026-03-01", Currency: "SAR", OrdersCount: 0, RevenueSumMinor: 0},
	}

	rows, err := svc.GetPlatformDays(context.Background(), "2026-03-01", "2026-03-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (one per currency for the same date), got %d", len(rows))
	}
	if rows[0].AvgOrderMinor != 200 {
		t.Fatalf("expected avgOrderMinor 200 (400/2), got %d", rows[0].AvgOrderMinor)
	}
	if rows[1].AvgOrderMinor != 0 {
		t.Fatalf("expected avgOrderMinor 0 when ordersCount is 0, got %d", rows[1].AvgOrderMinor)
	}
}

func TestService_GetPlatformSummary_DerivesEveryField(t *testing.T) {
	svc, _, _, _, platform, _ := newTestService()
	platform.summaryRows = []entity.PlatformDayCurrencyTotals{
		{
			Currency:                     "EGP",
			OrdersCount:                  10,
			RevenueSumMinor:              10000,
			FailedCount:                  2,
			DeliveryMsSum:                30000,
			DeliveryMsCount:              5,
			OnlinePaymentsCount:          3,
			OnlinePaymentsAmountSumMinor: 6000,
		},
		{Currency: "SAR"}, // all-zero row exercises every div-by-zero guard at once
	}

	rows, err := svc.GetPlatformSummary(context.Background(), "2026-03-01", "2026-03-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	egp := rows[0]
	if egp.AvgOrderMinor != 1000 {
		t.Fatalf("expected avgOrderMinor 1000 (10000/10), got %d", egp.AvgOrderMinor)
	}
	if egp.FailureRate != 0.2 {
		t.Fatalf("expected failureRate 0.2 (2/10), got %v", egp.FailureRate)
	}
	if egp.AvgDeliveryMs != 6000 {
		t.Fatalf("expected avgDeliveryMs 6000 (30000/5), got %d", egp.AvgDeliveryMs)
	}
	if egp.DeliveredCount != 5 {
		t.Fatalf("expected deliveredCount to mirror DeliveryMsCount (5), got %d", egp.DeliveredCount)
	}
	if egp.OnlinePaymentsCount != 3 || egp.OnlinePaymentsAmountSumMinor != 6000 {
		t.Fatalf("expected online payment totals passed through unchanged, got %+v", egp)
	}

	sar := rows[1]
	if sar.AvgOrderMinor != 0 || sar.FailureRate != 0 || sar.AvgDeliveryMs != 0 {
		t.Fatalf("expected every derived field to be 0 for an all-zero currency row (no div-by-zero panic), got %+v", sar)
	}
}
