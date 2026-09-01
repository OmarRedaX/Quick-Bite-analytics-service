// Package eventhandlers maps event type strings to service methods — the
// only place that knows both "what order.placed looks like on the wire" and
// "which service method handles it". Go analogue of order-service's
// app/order/core-events.handlers.ts.
package eventhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"analytics-service/app/analytics"
	"analytics-service/app/analytics/service"
	"analytics-service/lib/coreevents"
)

// orderPlacedPayload matches order-service's buildOrderPlacedPayload
// (src/app/order/service/order.service.ts) — only the fields this handler
// needs are declared; json.Unmarshal ignores the rest (region, countryCode,
// customerId, status, paymentMethod, etc.) untouched.
type orderPlacedPayload struct {
	OrderID      string            `json:"orderId"`
	RestaurantID int64             `json:"restaurantId"`
	BranchID     int64             `json:"branchId"`
	Total        int64             `json:"total"`
	Currency     string            `json:"currency"`
	PlacedAt     string            `json:"placedAt"`
	Items        []orderPlacedItem `json:"items"`
}

type orderPlacedItem struct {
	ProductID int64 `json:"productId"`
	Quantity  int64 `json:"quantity"`
	LineTotal int64 `json:"lineTotal"`
}

// orderDeliveredPayload matches settlement.service.ts's order.delivered
// outbox payload. No placedAt on the wire — see service.OnOrderDelivered
// for how the delivery duration is derived without it.
type orderDeliveredPayload struct {
	OrderID      string `json:"orderId"`
	RestaurantID int64  `json:"restaurantId"`
	BranchID     int64  `json:"branchId"`
	Currency     string `json:"currency"`
	DeliveredAt  string `json:"deliveredAt"`
}

// orderRejectedPayload matches order.service.ts's buildOrderTransitionPayload
// for the ORDER_REJECTED event. No currency/total on the wire — see
// service.OnOrderRejected for how the platform-day update handles that.
type orderRejectedPayload struct {
	OrderID      string `json:"orderId"`
	RestaurantID int64  `json:"restaurantId"`
	BranchID     int64  `json:"branchId"`
	OccurredAt   string `json:"occurredAt"`
}

// paymentCompletedPayload matches kashier-webhook.service.ts's
// payment.completed outbox payload.
type paymentCompletedPayload struct {
	Currency    string `json:"currency"`
	Amount      int64  `json:"amount"`
	CompletedAt string `json:"completedAt"`
}

// Register wires every event type this service consumes to its handler.
func Register(consumer *coreevents.Consumer, svc *service.Service) {
	consumer.Register("order.placed", onOrderPlaced(svc))
	consumer.Register("order.delivered", onOrderDelivered(svc))
	consumer.Register("order.rejected", onOrderRejected(svc))
	consumer.Register("payment.completed", onPaymentCompleted(svc))
}

func onOrderPlaced(svc *service.Service) coreevents.EventHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p orderPlacedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("order.placed: unmarshal payload: %w", err)
		}

		placedAt, err := time.Parse(time.RFC3339Nano, p.PlacedAt)
		if err != nil {
			return fmt.Errorf("order.placed: parse placedAt %q: %w", p.PlacedAt, err)
		}

		items := make([]analytics.OrderPlacedItem, 0, len(p.Items))
		for _, item := range p.Items {
			items = append(items, analytics.OrderPlacedItem{
				ProductID:      item.ProductID,
				Quantity:       item.Quantity,
				LineTotalMinor: item.LineTotal,
			})
		}

		return svc.OnOrderPlaced(ctx, analytics.OnOrderPlacedInput{
			OrderID:      p.OrderID,
			RestaurantID: p.RestaurantID,
			BranchID:     p.BranchID,
			Currency:     p.Currency,
			TotalMinor:   p.Total,
			PlacedAt:     placedAt,
			Items:        items,
		})
	}
}

func onOrderDelivered(svc *service.Service) coreevents.EventHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p orderDeliveredPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("order.delivered: unmarshal payload: %w", err)
		}

		deliveredAt, err := time.Parse(time.RFC3339Nano, p.DeliveredAt)
		if err != nil {
			return fmt.Errorf("order.delivered: parse deliveredAt %q: %w", p.DeliveredAt, err)
		}

		return svc.OnOrderDelivered(ctx, analytics.OnOrderDeliveredInput{
			OrderID:      p.OrderID,
			RestaurantID: p.RestaurantID,
			BranchID:     p.BranchID,
			Currency:     p.Currency,
			DeliveredAt:  deliveredAt,
		})
	}
}

func onOrderRejected(svc *service.Service) coreevents.EventHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p orderRejectedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("order.rejected: unmarshal payload: %w", err)
		}

		occurredAt, err := time.Parse(time.RFC3339Nano, p.OccurredAt)
		if err != nil {
			return fmt.Errorf("order.rejected: parse occurredAt %q: %w", p.OccurredAt, err)
		}

		return svc.OnOrderRejected(ctx, analytics.OnOrderRejectedInput{
			OrderID:      p.OrderID,
			RestaurantID: p.RestaurantID,
			BranchID:     p.BranchID,
			OccurredAt:   occurredAt,
		})
	}
}

func onPaymentCompleted(svc *service.Service) coreevents.EventHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p paymentCompletedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("payment.completed: unmarshal payload: %w", err)
		}

		completedAt, err := time.Parse(time.RFC3339Nano, p.CompletedAt)
		if err != nil {
			return fmt.Errorf("payment.completed: parse completedAt %q: %w", p.CompletedAt, err)
		}

		return svc.OnPaymentCompleted(ctx, analytics.OnPaymentCompletedInput{
			Currency:    p.Currency,
			AmountMinor: p.Amount,
			CompletedAt: completedAt,
		})
	}
}
