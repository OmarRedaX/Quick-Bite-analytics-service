package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"analytics-service/app/analytics"
	"analytics-service/app/analytics/entity"
)

type RestaurantDayRepo struct {
	coll *mongo.Collection
}

func NewRestaurantDayRepo(db *mongo.Database) *RestaurantDayRepo {
	return &RestaurantDayRepo{coll: db.Collection(CollRestaurantDay)}
}

// upsert is the shared shape behind every Apply* method below: point-lookup
// on (restaurant_id, date), $inc the counters that changed, $set updated_at
// (plus any extra fields, e.g. currency), $setOnInsert the key fields.
// Collapses what was three near-identical ~15-line UpdateOne blocks into
// one place — see platform_day.repo.go's upsert for the same pattern keyed
// by (date, currency) instead.
func (r *RestaurantDayRepo) upsert(ctx context.Context, restaurantID int64, date string, inc, extraSet bson.D) error {
	filter := bson.D{
		{Key: "restaurant_id", Value: restaurantID},
		{Key: "date", Value: date},
	}
	set := append(bson.D{{Key: "updated_at", Value: time.Now().UTC()}}, extraSet...)
	update := bson.D{
		{Key: "$inc", Value: inc},
		{Key: "$set", Value: set},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "restaurant_id", Value: restaurantID},
			{Key: "date", Value: date},
		}},
	}
	_, err := r.coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// ApplyOrderPlaced upserts one day's row: +1 order, +total revenue. Sum and
// count are $inc'd (never overwritten), so this single operation is safe to
// call concurrently for the same restaurant/day and is naturally
// associative under event replay.
func (r *RestaurantDayRepo) ApplyOrderPlaced(ctx context.Context, in analytics.OnOrderPlacedInput) error {
	date := in.PlacedAt.UTC().Format("2006-01-02")
	return r.upsert(ctx, in.RestaurantID, date,
		bson.D{
			{Key: "orders_count", Value: 1},
			{Key: "revenue_sum", Value: in.TotalMinor},
		},
		bson.D{{Key: "currency", Value: in.Currency}},
	)
}

// ApplyOrderDelivered adds one order's delivery duration to the day the
// order was placed on (date is passed in by the caller, derived from the
// looked-up OrderContext.PlacedAt — see service.OnOrderDelivered). Upsert
// with $setOnInsert covers the (rare, out-of-order) case where delivered
// arrives and the placed row hasn't been created yet.
func (r *RestaurantDayRepo) ApplyOrderDelivered(ctx context.Context, restaurantID int64, date string, deliveryMs int64) error {
	return r.upsert(ctx, restaurantID, date,
		bson.D{
			{Key: "delivery_ms_sum", Value: deliveryMs},
			{Key: "delivery_ms_count", Value: 1},
		},
		nil,
	)
}

// ApplyOrderRejected increments the rejected-order counter for the day the
// rejection happened (see analytics.OnOrderRejectedInput's doc comment for
// why this is OccurredAt, not the order's original placed date).
func (r *RestaurantDayRepo) ApplyOrderRejected(ctx context.Context, restaurantID int64, date string) error {
	return r.upsert(ctx, restaurantID, date, bson.D{{Key: "failed_count", Value: 1}}, nil)
}

// CountActiveInRange counts distinct restaurants with at least one order in
// [from, to] — backs GET /restaurants/active. An aggregation pipeline, not
// a stored field: this is a rare, cross-restaurant read, not a hot path
// that needs its own maintained counter.
func (r *RestaurantDayRepo) CountActiveInRange(ctx context.Context, from, to string) (int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "date", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}},
			{Key: "orders_count", Value: bson.D{{Key: "$gt", Value: 0}}},
		}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$restaurant_id"}}}},
		{{Key: "$count", Value: "count"}},
	}

	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		Count int64 `bson:"count"`
	}
	if err := cursor.All(ctx, &results); err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 0, nil
	}
	return results[0].Count, nil
}

// FindByDateRange returns rows for restaurantID with date in [from, to]
// (both YYYY-MM-DD, inclusive), ordered by date ascending. Backed by the
// uq_restaurant_id_date index (equality on restaurant_id, range on date).
func (r *RestaurantDayRepo) FindByDateRange(ctx context.Context, restaurantID int64, from, to string) ([]entity.RestaurantDay, error) {
	filter := bson.D{
		{Key: "restaurant_id", Value: restaurantID},
		{Key: "date", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}},
	}

	cursor, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	rows := make([]entity.RestaurantDay, 0)
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
