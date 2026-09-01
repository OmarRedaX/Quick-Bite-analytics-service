package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"analytics-service/app/analytics/entity"
)

type PlatformDayRepo struct {
	coll *mongo.Collection
}

func NewPlatformDayRepo(db *mongo.Database) *PlatformDayRepo {
	return &PlatformDayRepo{coll: db.Collection(CollPlatformDay)}
}

func (r *PlatformDayRepo) upsert(ctx context.Context, date, currency string, inc bson.D) error {
	filter := bson.D{
		{Key: "date", Value: date},
		{Key: "currency", Value: currency},
	}
	update := bson.D{
		{Key: "$inc", Value: inc},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "date", Value: date},
			{Key: "currency", Value: currency},
		}},
	}
	_, err := r.coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// ApplyOrderPlaced increments the platform-wide order/revenue counters for
// (date, currency).
func (r *PlatformDayRepo) ApplyOrderPlaced(ctx context.Context, date, currency string, totalMinor int64) error {
	return r.upsert(ctx, date, currency, bson.D{
		{Key: "orders_count", Value: 1},
		{Key: "revenue_sum", Value: totalMinor},
	})
}

// ApplyOrderDelivered increments the platform-wide delivery-duration
// counters for (date, currency).
func (r *PlatformDayRepo) ApplyOrderDelivered(ctx context.Context, date, currency string, deliveryMs int64) error {
	return r.upsert(ctx, date, currency, bson.D{
		{Key: "delivery_ms_sum", Value: deliveryMs},
		{Key: "delivery_ms_count", Value: 1},
	})
}

// ApplyOrderRejected increments the platform-wide rejected-order counter
// for (date, currency).
func (r *PlatformDayRepo) ApplyOrderRejected(ctx context.Context, date, currency string) error {
	return r.upsert(ctx, date, currency, bson.D{{Key: "failed_count", Value: 1}})
}

// ApplyPaymentCompleted increments the platform-wide online-payment
// counters for (date, currency).
func (r *PlatformDayRepo) ApplyPaymentCompleted(ctx context.Context, date, currency string, amountMinor int64) error {
	return r.upsert(ctx, date, currency, bson.D{
		{Key: "online_payments_count", Value: 1},
		{Key: "online_payments_amount_sum", Value: amountMinor},
	})
}

// FindByDateRange returns rows with date in [from, to] (inclusive), ordered
// by date then currency. A single date can produce more than one row (one
// per currency active that day).
func (r *PlatformDayRepo) FindByDateRange(ctx context.Context, from, to string) ([]entity.PlatformDay, error) {
	filter := bson.D{
		{Key: "date", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}},
	}

	cursor, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "date", Value: 1}, {Key: "currency", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	rows := make([]entity.PlatformDay, 0)
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// SummaryByCurrency totals every counter across [from, to], grouped by
// currency — backs GET /platform/summary. One $group aggregation, no new
// storage (per docs/api-contracts.md).
func (r *PlatformDayRepo) SummaryByCurrency(ctx context.Context, from, to string) ([]entity.PlatformDayCurrencyTotals, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "date", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$currency"},
			{Key: "orders_count", Value: bson.D{{Key: "$sum", Value: "$orders_count"}}},
			{Key: "revenue_sum", Value: bson.D{{Key: "$sum", Value: "$revenue_sum"}}},
			{Key: "failed_count", Value: bson.D{{Key: "$sum", Value: "$failed_count"}}},
			{Key: "delivery_ms_sum", Value: bson.D{{Key: "$sum", Value: "$delivery_ms_sum"}}},
			{Key: "delivery_ms_count", Value: bson.D{{Key: "$sum", Value: "$delivery_ms_count"}}},
			{Key: "online_payments_count", Value: bson.D{{Key: "$sum", Value: "$online_payments_count"}}},
			{Key: "online_payments_amount_sum", Value: bson.D{{Key: "$sum", Value: "$online_payments_amount_sum"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}

	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	rows := make([]entity.PlatformDayCurrencyTotals, 0)
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
