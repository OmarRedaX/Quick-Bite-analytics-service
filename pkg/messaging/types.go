// Package messaging defines a broker-agnostic interface for topic-exchange
// pub/sub and one concrete implementation (amqp091-go). Nothing in this
// package knows about analytics-service's event types or collections — that
// is app knowledge and belongs to lib/coreevents and app/analytics.
package messaging

import "context"

// ConsumeMessage is the broker-agnostic envelope handed to a consumer
// handler. Ack/Nack are closures bound to the underlying delivery so callers
// never see amqp091 types.
type ConsumeMessage struct {
	RoutingKey string
	Body       []byte
	Ack        func()
	Nack       func(requeue bool)
}

// ConsumerOptions describes the topology a consumer needs declared before it
// can start receiving: the topic exchange, its queue, the binding keys, and
// an optional dead-letter exchange/queue pair for poison messages.
type ConsumerOptions struct {
	Exchange           string
	Queue              string
	BindingKeys        []string
	DeadLetterExchange string
	DeadLetterQueue    string
	Prefetch           int
}

// Broker is the minimal surface analytics-service needs: connect, declare
// topology idempotently, and consume. This service never publishes, so no
// Publish method is exposed here (pkg stays honest about what's actually
// used — see docs/node-to-go-mapping.md for why order-service's broker
// interface additionally has Publish).
type Broker interface {
	Connect(ctx context.Context) error
	Close() error
	DeclareTopology(ctx context.Context, opts ConsumerOptions) error
	Consume(ctx context.Context, opts ConsumerOptions, handler func(ConsumeMessage)) error
}
