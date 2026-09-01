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

type ProductDayRepo struct {
	coll *mongo.Collection
}

func NewProductDayRepo(db *mongo.Database) *ProductDayRepo {
	return &ProductDayRepo{coll: db.Collection(CollProductDay)}
}

// ApplyOrderPlacedItems upserts one row per line item on a single
// order.placed event, in one BulkWrite instead of N round trips — a single
// order can have N items, and this is called once per order, not once per
// item (see plan.md's product-day gotcha).
func (r *ProductDayRepo) ApplyOrderPlacedItems(ctx context.Context, branchID int64, date, currency string, items []analytics.OrderPlacedItem) error {
	if len(items) == 0 {
		return nil
	}

	now := time.Now().UTC()
	models := make([]mongo.WriteModel, 0, len(items))
	for _, item := range items {
		filter := bson.D{
			{Key: "branch_id", Value: branchID},
			{Key: "product_id", Value: item.ProductID},
			{Key: "date", Value: date},
		}
		update := bson.D{
			{Key: "$inc", Value: bson.D{
				{Key: "quantity_sum", Value: item.Quantity},
				{Key: "revenue_sum", Value: item.LineTotalMinor},
			}},
			{Key: "$set", Value: bson.D{
				{Key: "currency", Value: currency},
				{Key: "updated_at", Value: now},
			}},
			{Key: "$setOnInsert", Value: bson.D{
				{Key: "branch_id", Value: branchID},
				{Key: "product_id", Value: item.ProductID},
				{Key: "date", Value: date},
			}},
		}
		models = append(models, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true))
	}

	_, err := r.coll.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
	return err
}

// FindByDateRange returns rows for (branchID, productID) with date in
// [from, to] (inclusive), ordered by date ascending. Backed by
// uq_branch_id_product_id_date.
func (r *ProductDayRepo) FindByDateRange(ctx context.Context, branchID, productID int64, from, to string) ([]entity.ProductDay, error) {
	filter := bson.D{
		{Key: "branch_id", Value: branchID},
		{Key: "product_id", Value: productID},
		{Key: "date", Value: bson.D{{Key: "$gte", Value: from}, {Key: "$lte", Value: to}}},
	}

	cursor, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	rows := make([]entity.ProductDay, 0)
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
