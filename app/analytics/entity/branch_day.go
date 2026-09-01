package entity

import "time"

// BranchDay is the agg_branch_day document: one row per (branch_id, date).
// Same sum+count-not-average shape as RestaurantDay — see that file's
// comment for the associativity rationale.
type BranchDay struct {
	BranchID        int64     `bson:"branch_id"`
	Date            string    `bson:"date"` // YYYY-MM-DD, UTC
	Currency        string    `bson:"currency"`
	OrdersCount     int64     `bson:"orders_count"`
	RevenueSumMinor int64     `bson:"revenue_sum"`
	DeliveryMsSum   int64     `bson:"delivery_ms_sum"`
	DeliveryMsCount int64     `bson:"delivery_ms_count"`
	FailedCount     int64     `bson:"failed_count"`
	UpdatedAt       time.Time `bson:"updated_at"`
}
