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
// needs are declared; json.Unmarshal ignores the rest (items, orderId,
// branchId, etc.) untouched.
type orderPlacedPayload struct {
	RestaurantID int64  `json:"restaurantId"`
	Total        int64  `json:"total"`
	Currency     string `json:"currency"`
	PlacedAt     string `json:"placedAt"`
}

// Register wires every event type this service consumes to its handler.
// Adding a new consumed event type (payment.completed, order.delivered —
// see docs/implementation-plan.md Phase 7) means one line here plus one
// handler function below.
func Register(consumer *coreevents.Consumer, svc *service.Service) {
	consumer.Register("order.placed", onOrderPlaced(svc))
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

		return svc.OnOrderPlaced(ctx, analytics.OnOrderPlacedInput{
			RestaurantID: p.RestaurantID,
			Currency:     p.Currency,
			TotalMinor:   p.Total,
			PlacedAt:     placedAt,
		})
	}
}
