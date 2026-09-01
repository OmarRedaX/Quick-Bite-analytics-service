package entity

import "time"

// ProductDay is the agg_product_day document: one row per (branch_id,
// product_id, date). Quantity and revenue are summed, never pre-divided —
// avgUnitPriceMinor is derived at read time in the service layer.
type ProductDay struct {
	BranchID        int64     `bson:"branch_id"`
	ProductID       int64     `bson:"product_id"`
	Date            string    `bson:"date"` // YYYY-MM-DD, UTC
	Currency        string    `bson:"currency"`
	QuantitySum     int64     `bson:"quantity_sum"`
	RevenueSumMinor int64     `bson:"revenue_sum"`
	UpdatedAt       time.Time `bson:"updated_at"`
}
