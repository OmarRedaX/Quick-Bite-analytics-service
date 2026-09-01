package entity

import "time"

// OrderContext is a short-lived per-order lookup, written by the
// order.placed handler and read back by order.delivered and
// order.rejected — the two downstream events whose own payloads don't
// carry everything analytics needs (order.delivered has no placedAt to
// compute delivery duration from; order.rejected has no currency to key an
// agg_platform_day row with). A TTL index on recorded_at reaps rows once
// they're older than any realistic order lifecycle — see indexes.go.
//
// Handlers must treat a missing row as "never saw order.placed for this
// order" (out-of-order delivery, or replay past the TTL window) and skip
// only the fields that depended on it, never crash or corrupt the
// aggregate — see eventhandlers/handlers.go.
type OrderContext struct {
	OrderID    string    `bson:"order_id"`
	Currency   string    `bson:"currency"`
	PlacedAt   time.Time `bson:"placed_at"`
	RecordedAt time.Time `bson:"recorded_at"`
}
