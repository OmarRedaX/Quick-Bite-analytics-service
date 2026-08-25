package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"analytics-service/app/analytics/entity"
)

type EventIDsRepo struct {
	coll *mongo.Collection
}

func NewEventIDsRepo(db *mongo.Database) *EventIDsRepo {
	return &EventIDsRepo{coll: db.Collection(CollEventIDs)}
}

// MarkSeen satisfies lib/coreevents.EventDeduper via Go duck typing — this
// type is never declared to implement that interface, it just has the
// matching methods. The unique index on event_id turns a duplicate
// InsertOne into a well-known error (mongo.IsDuplicateKeyError), which
// becomes fresh=false: ack-and-skip for the consumer.
func (r *EventIDsRepo) MarkSeen(ctx context.Context, eventID string) (bool, error) {
	_, err := r.coll.InsertOne(ctx, entity.EventID{EventID: eventID, ReceivedAt: time.Now().UTC()})
	if err == nil {
		return true, nil
	}
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	return false, err
}

// Unmark removes the event_id record. Called when a handler fails after
// MarkSeen already succeeded, so the event isn't permanently stuck as
// "processed" when its aggregate write never actually happened.
func (r *EventIDsRepo) Unmark(ctx context.Context, eventID string) error {
	_, err := r.coll.DeleteOne(ctx, bson.D{{Key: "event_id", Value: eventID}})
	return err
}
