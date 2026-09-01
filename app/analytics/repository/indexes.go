// Package repository is the only place the mongo-driver appears in this
// module (per CLAUDE.md's strict layering). Repository functions take
// typed structs (entity.*) and return typed structs — no bson.M soup
// leaking into the service layer.
package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	CollRestaurantDay = "agg_restaurant_day"
	CollBranchDay     = "agg_branch_day"
	CollProductDay    = "agg_product_day"
	CollPlatformDay   = "agg_platform_day"
	CollOrderContext  = "order_context"
	CollEventIDs      = "event_ids"
)

// EnsureIndexes creates every index this module needs, idempotently. Called
// once at boot (lib/boot). Mongo is schemaless — there is no DDL migration
// to run, only indexes to declare — so this file is this module's entire
// "migration" surface. Lives here, not in pkg/mongo, because pkg/ must not
// know collection names (app-specific knowledge).
func EnsureIndexes(ctx context.Context, db *mongo.Database, eventDedupeTTL, orderContextTTL time.Duration) error {
	restaurantDay := db.Collection(CollRestaurantDay)
	if _, err := restaurantDay.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			// supports the upsert in restaurant_day.repo.go's
			// ApplyOrderPlaced (equality point-lookup on both fields).
			Keys:    bson.D{{Key: "restaurant_id", Value: 1}, {Key: "date", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uq_restaurant_id_date"),
		},
		{
			// supports GET /restaurants/:id/days?from=&to= range scans —
			// (equality col first would be restaurant_id, but this index
			// exists specifically for cross-restaurant day-range scans a
			// future homework endpoint needs; the query this endpoint runs
			// today is fully served by the unique index above).
			Keys:    bson.D{{Key: "date", Value: 1}, {Key: "restaurant_id", Value: 1}},
			Options: options.Index().SetName("idx_date_restaurant_id"),
		},
	}); err != nil {
		return err
	}

	branchDay := db.Collection(CollBranchDay)
	if _, err := branchDay.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			// supports branch_day.repo.go's ApplyOrderPlaced/Delivered/Rejected
			// upserts (equality point-lookup on both fields).
			Keys:    bson.D{{Key: "branch_id", Value: 1}, {Key: "date", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uq_branch_id_date"),
		},
	}); err != nil {
		return err
	}

	productDay := db.Collection(CollProductDay)
	if _, err := productDay.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			// supports product_day.repo.go's BulkWrite upserts and
			// GET /branches/:id/products/:productId/days range scans.
			Keys:    bson.D{{Key: "branch_id", Value: 1}, {Key: "product_id", Value: 1}, {Key: "date", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uq_branch_id_product_id_date"),
		},
	}); err != nil {
		return err
	}

	platformDay := db.Collection(CollPlatformDay)
	if _, err := platformDay.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			// keyed by (date, currency) — not just date — so two currencies
			// active the same day never get summed into one row. Supports
			// both the upsert and GET /platform/days|summary range scans.
			Keys:    bson.D{{Key: "date", Value: 1}, {Key: "currency", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uq_date_currency"),
		},
	}); err != nil {
		return err
	}

	orderContext := db.Collection(CollOrderContext)
	if _, err := orderContext.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "order_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uq_order_id"),
		},
		{
			// TTL reaper — bounds this collection's size without an app-side
			// cleanup job, same pattern as event_ids below. Window is longer
			// than event_ids' because it must outlive the gap between
			// order.placed and a (possibly slow) order.delivered/rejected.
			Keys:    bson.D{{Key: "recorded_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32(orderContextTTL.Seconds())).SetName("ttl_recorded_at"),
		},
	}); err != nil {
		return err
	}

	eventIDs := db.Collection(CollEventIDs)
	if _, err := eventIDs.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uq_event_id"),
		},
		{
			// TTL index: Mongo's background reaper drops documents once
			// received_at is older than eventDedupeTTL. Bounds the
			// collection's size without any application-side cleanup job.
			Keys:    bson.D{{Key: "received_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32(eventDedupeTTL.Seconds())).SetName("ttl_received_at"),
		},
	}); err != nil {
		return err
	}

	return nil
}
