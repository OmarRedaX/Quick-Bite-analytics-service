package entity

import "time"

// PlatformDay is the agg_platform_day document: one row per (date,
// currency) — NOT just (date). The platform operates in more than one
// currency (order-service's Currency enum has EGP and SAR), so a
// currency-less daily row would silently sum two different currencies
// together. Same sum+count-not-average shape as RestaurantDay.
type PlatformDay struct {
	Date                         string    `bson:"date"` // YYYY-MM-DD, UTC
	Currency                     string    `bson:"currency"`
	OrdersCount                  int64     `bson:"orders_count"`
	RevenueSumMinor              int64     `bson:"revenue_sum"`
	DeliveryMsSum                int64     `bson:"delivery_ms_sum"`
	DeliveryMsCount              int64     `bson:"delivery_ms_count"`
	FailedCount                  int64     `bson:"failed_count"`
	OnlinePaymentsCount          int64     `bson:"online_payments_count"`
	OnlinePaymentsAmountSumMinor int64     `bson:"online_payments_amount_sum"`
	UpdatedAt                    time.Time `bson:"updated_at"`
}

// PlatformDayCurrencyTotals is the shape of one $group output row from
// PlatformDayRepo.SummaryByCurrency — a range total for one currency, not a
// stored document (no updated_at).
type PlatformDayCurrencyTotals struct {
	Currency                     string `bson:"_id"`
	OrdersCount                  int64  `bson:"orders_count"`
	RevenueSumMinor              int64  `bson:"revenue_sum"`
	FailedCount                  int64  `bson:"failed_count"`
	DeliveryMsSum                int64  `bson:"delivery_ms_sum"`
	DeliveryMsCount              int64  `bson:"delivery_ms_count"`
	OnlinePaymentsCount          int64  `bson:"online_payments_count"`
	OnlinePaymentsAmountSumMinor int64  `bson:"online_payments_amount_sum"`
}
