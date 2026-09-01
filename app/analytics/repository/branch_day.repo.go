package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"analytics-service/app/analytics/entity"
)

type BranchDayRepo struct {
	coll *mongo.Collection
}

func NewBranchDayRepo(db *mongo.Database) *BranchDayRepo {
	return &BranchDayRepo{coll: db.Collection(CollBranchDay)}
}

// upsert is the shared shape behind every Apply* method below — see
// RestaurantDayRepo.upsert for the same pattern, keyed by branch_id here
// instead of restaurant_id.
func (r *BranchDayRepo) upsert(ctx context.Context, branchID int64, date string, inc, extraSet bson.D) error {
	filter := bson.D{
		{Key: "branch_id", Value: branchID},
		{Key: "date", Value: date},
	}
	set := append(bson.D{{Key: "updated_at", Value: time.Now().UTC()}}, extraSet...)
	update := bson.D{
		{Key: "$inc", Value: inc},
		{Key: "$set", Value: set},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "branch_id", Value: branchID},
			{Key: "date", Value: date},
		}},
	}
	_, err := r.coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// ApplyOrderPlaced mirrors RestaurantDayRepo.ApplyOrderPlaced, keyed by
// branch instead of restaurant.
func (r *BranchDayRepo) ApplyOrderPlaced(ctx context.Context, branchID int64, date, currency string, totalMinor int64) error {
	return r.upsert(ctx, branchID, date,
		bson.D{
			{Key: "orders_count", Value: 1},
			{Key: "revenue_sum", Value: totalMinor},
		},
		bson.D{{Key: "currency", Value: currency}},
	)
}

// ApplyOrderDelivered mirrors RestaurantDayRepo.ApplyOrderDelivered.
func (r *BranchDayRepo) ApplyOrderDelivered(ctx context.Context, branchID int64, date string, deliveryMs int64) error {
	return r.upsert(ctx, branchID, date,
		bson.D{
			{Key: "delivery_ms_sum", Value: deliveryMs},
			{Key: "delivery_ms_count", Value: 1},
		},
		nil,
	)
}

// ApplyOrderRejected mirrors RestaurantDayRepo.ApplyOrderRejected.
func (r *BranchDayRepo) ApplyOrderRejected(ctx context.Context, branchID int64, date string) error {
	return r.upsert(ctx, branchID, date, bson.D{{Key: "failed_count", Value: 1}}, nil)
}

// FindByDateRange returns rows for branchID with date in [from, to]
// (inclusive), ordered by date ascending. Backed by uq_branch_id_date.
func (r *BranchDayRepo) FindByDateRange(ctx context.Context, branchID int64, from, to string) ([]entity.BranchDay, error) {
	filter := bson.D{
		{Key: "branch_id", Value: branchID},
		{Key: "date", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}},
	}

	cursor, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	rows := make([]entity.BranchDay, 0)
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
