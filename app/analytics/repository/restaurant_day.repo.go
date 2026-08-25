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

// ApplyOrderPlaced upserts one day's row: +1 order, +total revenue. Sum and
// count are $inc'd (never overwritten), so this single operation is safe to
// call concurrently for the same restaurant/day and is naturally
// associative under event replay.
func (r *RestaurantDayRepo) ApplyOrderPlaced(ctx context.Context, in analytics.OnOrderPlacedInput) error {
	date := in.PlacedAt.UTC().Format("2006-01-02")

	filter := bson.D{
		{Key: "restaurant_id", Value: in.RestaurantID},
		{Key: "date", Value: date},
	}
	update := bson.D{
		{Key: "$inc", Value: bson.D{
			{Key: "orders_count", Value: 1},
			{Key: "revenue_sum", Value: in.TotalMinor},
		}},
		{Key: "$set", Value: bson.D{
			{Key: "currency", Value: in.Currency},
			{Key: "updated_at", Value: time.Now().UTC()},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "restaurant_id", Value: in.RestaurantID},
			{Key: "date", Value: date},
		}},
	}

	_, err := r.coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
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
