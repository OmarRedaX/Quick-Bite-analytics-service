// Package mongo is a thin, app-agnostic wrapper around the official Mongo
// driver. It knows how to open and close a connection — nothing about
// collections, indexes, or documents. That knowledge belongs to
// app/analytics/repository.
package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Config is the minimal set of knobs needed to open a connection.
type Config struct {
	URI            string
	Database       string
	ConnectTimeout time.Duration
}

// Connect dials Mongo, pings it to fail fast on boot, and returns both the
// client (for lifecycle management) and the target database handle.
func Connect(ctx context.Context, cfg Config) (*mongo.Client, *mongo.Database, error) {
	timeout := cfg.ConnectTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, nil, fmt.Errorf("mongo connect: %w", err)
	}

	pingCtx, cancelPing := context.WithTimeout(ctx, timeout)
	defer cancelPing()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		return nil, nil, fmt.Errorf("mongo ping: %w", err)
	}

	return client, client.Database(cfg.Database), nil
}

// Disconnect closes the client. Safe to call with a nil client.
func Disconnect(ctx context.Context, client *mongo.Client) error {
	if client == nil {
		return nil
	}
	return client.Disconnect(ctx)
}
