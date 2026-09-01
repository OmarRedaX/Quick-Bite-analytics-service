package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"analytics-service/app/analytics/entity"
)

type OrderContextRepo struct {
	coll *mongo.Collection
}

func NewOrderContextRepo(db *mongo.Database) *OrderContextRepo {
	return &OrderContextRepo{coll: db.Collection(CollOrderContext)}
}

// Save records what order.delivered/order.rejected need but don't carry on
// their own payload. Upsert (not insert) so a replayed order.placed for the
// same order — e.g. after a handler failure and Unmark — doesn't error.
func (r *OrderContextRepo) Save(ctx context.Context, orderID, currency string, placedAt time.Time) error {
	filter := bson.D{{Key: "order_id", Value: orderID}}
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "currency", Value: currency},
			{Key: "placed_at", Value: placedAt},
			{Key: "recorded_at", Value: time.Now().UTC()},
		}},
		{Key: "$setOnInsert", Value: bson.D{{Key: "order_id", Value: orderID}}},
	}
	_, err := r.coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// Find returns (zero value, false, nil) when no row exists — the expected
// shape for "never saw order.placed for this order" (out-of-order delivery,
// or a replay past the TTL window), which callers must treat as a
// best-effort miss, not an error.
func (r *OrderContextRepo) Find(ctx context.Context, orderID string) (entity.OrderContext, bool, error) {
	var row entity.OrderContext
	err := r.coll.FindOne(ctx, bson.D{{Key: "order_id", Value: orderID}}).Decode(&row)
	if err == mongo.ErrNoDocuments {
		return entity.OrderContext{}, false, nil
	}
	if err != nil {
		return entity.OrderContext{}, false, err
	}
	return row, true, nil
}
