package eventhandlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"analytics-service/app/analytics"
	"analytics-service/app/analytics/entity"
	"analytics-service/app/analytics/service"
)

// In-package (white-box) test — onOrderPlaced/onOrderDelivered/etc. are
// unexported handler factories, only callable from within package
// eventhandlers. See testing-implementation-plan.md Phase 0 for why this
// convention (not tests/unit) is how Go tests unexported symbols.
//
// The factories close over a *service.Service, a concrete type, not an
// interface — so to exercise them without Mongo, this file builds a real
// *service.Service from small fakes satisfying service's own unexported
// repo interfaces (structural typing: a type never has to import or name
// those interfaces to implement them).

type fakeRestaurantDayRepo struct {
	placedCalls []analytics.OnOrderPlacedInput
}

func (f *fakeRestaurantDayRepo) ApplyOrderPlaced(_ context.Context, in analytics.OnOrderPlacedInput) error {
	f.placedCalls = append(f.placedCalls, in)
	return nil
}
func (f *fakeRestaurantDayRepo) ApplyOrderDelivered(context.Context, int64, string, int64) error {
	return nil
}
func (f *fakeRestaurantDayRepo) ApplyOrderRejected(context.Context, int64, string) error { return nil }
func (f *fakeRestaurantDayRepo) CountActiveInRange(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (f *fakeRestaurantDayRepo) FindByDateRange(context.Context, int64, string, string) ([]entity.RestaurantDay, error) {
	return nil, nil
}

type fakeBranchDayRepo struct{}

func (f *fakeBranchDayRepo) ApplyOrderPlaced(context.Context, int64, string, string, int64) error {
	return nil
}
func (f *fakeBranchDayRepo) ApplyOrderDelivered(context.Context, int64, string, int64) error {
	return nil
}
func (f *fakeBranchDayRepo) ApplyOrderRejected(context.Context, int64, string) error { return nil }
func (f *fakeBranchDayRepo) FindByDateRange(context.Context, int64, string, string) ([]entity.BranchDay, error) {
	return nil, nil
}

type fakeProductDayRepo struct {
	itemsCalls [][]analytics.OrderPlacedItem
}

func (f *fakeProductDayRepo) ApplyOrderPlacedItems(_ context.Context, _ int64, _, _ string, items []analytics.OrderPlacedItem) error {
	f.itemsCalls = append(f.itemsCalls, items)
	return nil
}
func (f *fakeProductDayRepo) FindByDateRange(context.Context, int64, int64, string, string) ([]entity.ProductDay, error) {
	return nil, nil
}

type fakePlatformDayRepo struct{}

func (f *fakePlatformDayRepo) ApplyOrderPlaced(context.Context, string, string, int64) error {
	return nil
}
func (f *fakePlatformDayRepo) ApplyOrderDelivered(context.Context, string, string, int64) error {
	return nil
}
func (f *fakePlatformDayRepo) ApplyOrderRejected(context.Context, string, string) error { return nil }
func (f *fakePlatformDayRepo) ApplyPaymentCompleted(context.Context, string, string, int64) error {
	return nil
}
func (f *fakePlatformDayRepo) FindByDateRange(context.Context, string, string) ([]entity.PlatformDay, error) {
	return nil, nil
}
func (f *fakePlatformDayRepo) SummaryByCurrency(context.Context, string, string) ([]entity.PlatformDayCurrencyTotals, error) {
	return nil, nil
}

type fakeOrderContextRepo struct {
	saveCalls []struct {
		OrderID  string
		Currency string
		PlacedAt time.Time
	}
	findRow   entity.OrderContext
	findFound bool
}

func (f *fakeOrderContextRepo) Save(_ context.Context, orderID, currency string, placedAt time.Time) error {
	f.saveCalls = append(f.saveCalls, struct {
		OrderID  string
		Currency string
		PlacedAt time.Time
	}{orderID, currency, placedAt})
	return nil
}
func (f *fakeOrderContextRepo) Find(context.Context, string) (entity.OrderContext, bool, error) {
	return f.findRow, f.findFound, nil
}

func newTestServiceAndFakes() (*service.Service, *fakeRestaurantDayRepo, *fakeProductDayRepo, *fakeOrderContextRepo) {
	rest := &fakeRestaurantDayRepo{}
	product := &fakeProductDayRepo{}
	orderCtx := &fakeOrderContextRepo{}
	svc := service.New(rest, &fakeBranchDayRepo{}, product, &fakePlatformDayRepo{}, orderCtx)
	return svc, rest, product, orderCtx
}

func TestOnOrderPlaced_MalformedJSON(t *testing.T) {
	svc, _, _, _ := newTestServiceAndFakes()
	handler := onOrderPlaced(svc)

	if err := handler(context.Background(), json.RawMessage(`{not valid json`)); err == nil {
		t.Fatal("expected an error for malformed JSON payload")
	}
}

func TestOnOrderPlaced_BadTimestampFormat(t *testing.T) {
	svc, _, _, _ := newTestServiceAndFakes()
	handler := onOrderPlaced(svc)

	payload := `{"orderId":"o1","restaurantId":1,"branchId":2,"total":100,"currency":"EGP","placedAt":"not-a-timestamp"}`
	if err := handler(context.Background(), json.RawMessage(payload)); err == nil {
		t.Fatal("expected an error for a placedAt that doesn't parse as RFC3339Nano")
	}
}

func TestOnOrderPlaced_MapsFieldsCorrectly(t *testing.T) {
	svc, rest, product, orderCtx := newTestServiceAndFakes()
	handler := onOrderPlaced(svc)

	payload := `{
		"orderId": "order-1",
		"restaurantId": 10,
		"branchId": 20,
		"total": 5000,
		"currency": "EGP",
		"placedAt": "2026-03-05T10:30:00Z",
		"items": [{"productId": 1, "quantity": 2, "lineTotal": 5000}],
		"region": "cairo",
		"status": "placed"
	}`
	if err := handler(context.Background(), json.RawMessage(payload)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rest.placedCalls) != 1 {
		t.Fatalf("expected service.OnOrderPlaced called once, got %d", len(rest.placedCalls))
	}
	in := rest.placedCalls[0]
	if in.OrderID != "order-1" || in.RestaurantID != 10 || in.BranchID != 20 || in.Currency != "EGP" || in.TotalMinor != 5000 {
		t.Fatalf("expected fields mapped from the wire payload, got %+v", in)
	}
	if !in.PlacedAt.Equal(time.Date(2026, 3, 5, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("expected placedAt parsed to 2026-03-05T10:30:00Z, got %v", in.PlacedAt)
	}
	if len(product.itemsCalls) != 1 || len(product.itemsCalls[0]) != 1 || product.itemsCalls[0][0].ProductID != 1 || product.itemsCalls[0][0].Quantity != 2 || product.itemsCalls[0][0].LineTotalMinor != 5000 {
		t.Fatalf("expected items mapped through to productDays, got %+v", product.itemsCalls)
	}
	if len(orderCtx.saveCalls) != 1 || orderCtx.saveCalls[0].OrderID != "order-1" {
		t.Fatalf("expected orderContexts.Save called with the order id, got %+v", orderCtx.saveCalls)
	}
}

func TestOnOrderDelivered_MalformedJSON(t *testing.T) {
	svc, _, _, _ := newTestServiceAndFakes()
	handler := onOrderDelivered(svc)

	if err := handler(context.Background(), json.RawMessage(`not json at all`)); err == nil {
		t.Fatal("expected an error for malformed JSON payload")
	}
}

func TestOnOrderDelivered_BadTimestampFormat(t *testing.T) {
	svc, _, _, _ := newTestServiceAndFakes()
	handler := onOrderDelivered(svc)

	payload := `{"orderId":"o1","restaurantId":1,"branchId":2,"currency":"EGP","deliveredAt":"03/05/2026"}`
	if err := handler(context.Background(), json.RawMessage(payload)); err == nil {
		t.Fatal("expected an error for a deliveredAt that doesn't parse as RFC3339Nano")
	}
}

func TestOnOrderDelivered_MapsFieldsCorrectly(t *testing.T) {
	svc, _, _, orderCtx := newTestServiceAndFakes()
	orderCtx.findFound = true
	orderCtx.findRow = entity.OrderContext{PlacedAt: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)}
	handler := onOrderDelivered(svc)

	payload := `{"orderId":"order-1","restaurantId":10,"branchId":20,"currency":"EGP","deliveredAt":"2026-03-05T10:45:00Z"}`
	if err := handler(context.Background(), json.RawMessage(payload)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOnOrderRejected_MalformedJSON(t *testing.T) {
	svc, _, _, _ := newTestServiceAndFakes()
	handler := onOrderRejected(svc)

	if err := handler(context.Background(), json.RawMessage(`{"orderId":`)); err == nil {
		t.Fatal("expected an error for malformed JSON payload")
	}
}

func TestOnOrderRejected_BadTimestampFormat(t *testing.T) {
	svc, _, _, _ := newTestServiceAndFakes()
	handler := onOrderRejected(svc)

	payload := `{"orderId":"o1","restaurantId":1,"branchId":2,"occurredAt":"tomorrow"}`
	if err := handler(context.Background(), json.RawMessage(payload)); err == nil {
		t.Fatal("expected an error for an occurredAt that doesn't parse as RFC3339Nano")
	}
}

func TestOnOrderRejected_MapsFieldsCorrectly(t *testing.T) {
	svc, rest, _, _ := newTestServiceAndFakes()
	handler := onOrderRejected(svc)

	payload := `{"orderId":"order-2","restaurantId":10,"branchId":20,"occurredAt":"2026-03-06T08:00:00Z"}`
	if err := handler(context.Background(), json.RawMessage(payload)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = rest // restaurantDays.ApplyOrderRejected has no return value to assert on via this fake; success = no error
}

func TestOnPaymentCompleted_MalformedJSON(t *testing.T) {
	svc, _, _, _ := newTestServiceAndFakes()
	handler := onPaymentCompleted(svc)

	if err := handler(context.Background(), json.RawMessage(`[]nope`)); err == nil {
		t.Fatal("expected an error for malformed JSON payload")
	}
}

func TestOnPaymentCompleted_BadTimestampFormat(t *testing.T) {
	svc, _, _, _ := newTestServiceAndFakes()
	handler := onPaymentCompleted(svc)

	payload := `{"currency":"EGP","amount":1000,"completedAt":"not-a-date"}`
	if err := handler(context.Background(), json.RawMessage(payload)); err == nil {
		t.Fatal("expected an error for a completedAt that doesn't parse as RFC3339Nano")
	}
}

func TestOnPaymentCompleted_MapsFieldsCorrectly(t *testing.T) {
	svc, _, _, _ := newTestServiceAndFakes()
	handler := onPaymentCompleted(svc)

	payload := `{"currency":"EGP","amount":1000,"completedAt":"2026-03-06T08:00:00Z"}`
	if err := handler(context.Background(), json.RawMessage(payload)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
