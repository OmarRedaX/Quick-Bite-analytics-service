// Package entity holds plain structs with bson tags — the Mongo analogue of
// order-service's entity/<module>.entity.ts. No methods beyond simple
// invariants, no DB knowledge (that's repository/), no JSON tags (DTOs own
// the wire shape).
package entity

import "time"

// RestaurantDay is the agg_restaurant_day document: one row per
// (restaurant_id, date). Averages are stored as sum+count, never as a
// pre-divided average, so day rows merge associatively no matter what order
// events are replayed in — see docs/system-design.md.
type RestaurantDay struct {
	RestaurantID    int64     `bson:"restaurant_id"`
	Date            string    `bson:"date"` // YYYY-MM-DD, UTC
	Currency        string    `bson:"currency"`
	OrdersCount     int64     `bson:"orders_count"`
	RevenueSumMinor int64     `bson:"revenue_sum"`
	DeliveryMsSum   int64     `bson:"delivery_ms_sum"`
	DeliveryMsCount int64     `bson:"delivery_ms_count"`
	UpdatedAt       time.Time `bson:"updated_at"`
}
