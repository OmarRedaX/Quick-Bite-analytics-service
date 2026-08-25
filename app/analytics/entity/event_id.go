package entity

import "time"

// EventID is one row in the event_ids dedupe collection. A unique index on
// event_id turns a duplicate InsertOne into a dup-key error, which
// repository.EventIDsRepo.MarkSeen turns into fresh=false. A TTL index on
// received_at reaps rows after the configured retention window.
type EventID struct {
	EventID    string    `bson:"event_id"`
	ReceivedAt time.Time `bson:"received_at"`
}
