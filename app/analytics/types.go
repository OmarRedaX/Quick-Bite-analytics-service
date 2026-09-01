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
	OrderID      string
	RestaurantID int64
	BranchID     int64
	Currency     string
	TotalMinor   int64
	PlacedAt     time.Time
	Items        []OrderPlacedItem
}

// OrderPlacedItem is one line item off the order.placed payload's `items`
// array — enough for agg_product_day's per-product rollup.
type OrderPlacedItem struct {
	ProductID      int64
	Quantity       int64
	LineTotalMinor int64
}

// OnOrderDeliveredInput is what eventhandlers extracts from the
// order.delivered event payload. DeliveryMs/date-bucketing is derived by
// the service from a PlacedAt looked up via OrderContext — see
// service.OnOrderDelivered.
type OnOrderDeliveredInput struct {
	OrderID      string
	RestaurantID int64
	BranchID     int64
	Currency     string
	DeliveredAt  time.Time
}

// OnOrderRejectedInput is what eventhandlers extracts from the
// order.rejected event payload. Bucketed by OccurredAt (the rejection
// event's own timestamp), not the order's original placedAt — restaurant
// accept/reject decisions happen quickly after placement, so this is a
// deliberate simplification that avoids every rejection handler call
// depending on an OrderContext lookup for date-keying (see
// eventhandlers/handlers.go).
type OnOrderRejectedInput struct {
	OrderID      string
	RestaurantID int64
	BranchID     int64
	OccurredAt   time.Time
}

// OnPaymentCompletedInput is what eventhandlers extracts from the
// payment.completed event payload. Scoped to agg_platform_day only — see
// service.OnPaymentCompleted for why this doesn't also touch
// restaurant/branch day rows.
type OnPaymentCompletedInput struct {
	Currency    string
	AmountMinor int64
	CompletedAt time.Time
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

// BranchDayRow mirrors RestaurantDayRow for the branch-scoped days
// endpoint.
type BranchDayRow struct {
	Date            string
	Currency        string
	OrdersCount     int64
	RevenueSumMinor int64
	AvgOrderMinor   int64
}

// ProductDayRow is one day's per-product rollup, avgUnitPriceMinor derived
// here (RevenueSumMinor/QuantitySum) — never stored.
type ProductDayRow struct {
	Date              string
	Currency          string
	QuantitySum       int64
	RevenueSumMinor   int64
	AvgUnitPriceMinor int64
}

// PlatformDayRow is one day's platform-wide rollup for one currency —
// agg_platform_day rows are keyed by (date, currency), so a single date can
// produce more than one row.
type PlatformDayRow struct {
	Date            string
	Currency        string
	OrdersCount     int64
	RevenueSumMinor int64
	AvgOrderMinor   int64
}

// FailureRow is one day's order/failure counts for a restaurant or branch,
// FailureRate derived here (FailedCount/OrdersCount) — never stored.
type FailureRow struct {
	Date        string
	OrdersCount int64
	FailedCount int64
	FailureRate float64
}

// DeliveryAvgRow is one day's delivery-duration rollup for a restaurant or
// branch, AvgDeliveryMs derived here (DeliveryMsSum/DeliveryMsCount) —
// never stored.
type DeliveryAvgRow struct {
	Date           string
	DeliveredCount int64
	AvgDeliveryMs  int64
}

// PlatformSummaryRow is the totals-across-the-range view for one currency —
// every derived field (AvgOrderMinor, FailureRate, AvgDeliveryMs) computed
// here from a single $group aggregation's sums/counts.
type PlatformSummaryRow struct {
	Currency                     string
	OrdersCount                  int64
	RevenueSumMinor              int64
	AvgOrderMinor                int64
	FailedCount                  int64
	FailureRate                  float64
	DeliveredCount               int64
	AvgDeliveryMs                int64
	OnlinePaymentsCount          int64
	OnlinePaymentsAmountSumMinor int64
}
