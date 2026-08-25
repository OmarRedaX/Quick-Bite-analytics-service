// Package analytics holds module-shared types, errors, and enums — the Go
// analogue of order-service's app/<module>/types.ts + errors.ts + enums.ts,
// deliberately kept in this parent package (not service/) so every
// subpackage (service, controller, eventhandlers) imports them the same
// way: `analytics.OnOrderPlacedInput`, `analytics.ErrInvalidDateRange`,
// `analytics.PermAnalyticsRead`.
package analytics

import "time"

// OnOrderPlacedInput is what eventhandlers extracts from the order.placed
// event payload and hands to service.OnOrderPlaced. The service never sees
// the transport envelope (eventId, routing key, etc.) — only this.
type OnOrderPlacedInput struct {
	RestaurantID int64
	Currency     string
	TotalMinor   int64
	PlacedAt     time.Time
}

// RestaurantDayRow is one day's aggregate for one restaurant, as returned by
// the service layer to the controller. AvgOrderMinor is derived here
// (RevenueSumMinor/OrdersCount) — it is never stored in Mongo.
type RestaurantDayRow struct {
	Date            string // YYYY-MM-DD, UTC
	Currency        string
	OrdersCount     int64
	RevenueSumMinor int64
	AvgOrderMinor   int64
}
