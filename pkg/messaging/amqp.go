package messaging

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPConfig is the connection info needed to dial a broker.
type AMQPConfig struct {
	URL string
}

// AMQPClient is a Broker implementation over github.com/rabbitmq/amqp091-go.
//
// Deliberate simplification vs. core-service/order-service's Node client
// (which wraps amqp-connection-manager for automatic reconnect/replay):
// amqp091-go has no built-in reconnect manager, and adding one is out of
// scope for this teaching slice. Connect() is retried with backoff at boot
// (see lib/boot), but a connection drop mid-run is NOT auto-recovered —
// documented as homework in docs/implementation-plan.md.
type AMQPClient struct {
	url string

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewAMQPClient(cfg AMQPConfig) *AMQPClient {
	return &AMQPClient{url: cfg.URL}
}

func (c *AMQPClient) Connect(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil && !c.conn.IsClosed() {
		return nil
	}

	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("amqp dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("amqp channel: %w", err)
	}

	c.conn = conn
	c.channel = ch
	return nil
}

func (c *AMQPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var err error
	if c.channel != nil {
		err = c.channel.Close()
	}
	if c.conn != nil {
		if cerr := c.conn.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	c.channel = nil
	c.conn = nil
	return err
}

// DeclareTopology asserts the exchange, optional DLX/DLQ, queue, and
// bindings. Idempotent — safe to call every time a consumer starts.
func (c *AMQPClient) DeclareTopology(ctx context.Context, opts ConsumerOptions) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}
	ch := c.channel

	if err := ch.ExchangeDeclare(opts.Exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %s: %w", opts.Exchange, err)
	}

	args := amqp.Table{}
	if opts.DeadLetterExchange != "" && opts.DeadLetterQueue != "" {
		if err := ch.ExchangeDeclare(opts.DeadLetterExchange, "topic", true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare dlx %s: %w", opts.DeadLetterExchange, err)
		}
		if _, err := ch.QueueDeclare(opts.DeadLetterQueue, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare dlq %s: %w", opts.DeadLetterQueue, err)
		}
		if err := ch.QueueBind(opts.DeadLetterQueue, "#", opts.DeadLetterExchange, false, nil); err != nil {
			return fmt.Errorf("bind dlq %s: %w", opts.DeadLetterQueue, err)
		}
		args["x-dead-letter-exchange"] = opts.DeadLetterExchange
	}

	if _, err := ch.QueueDeclare(opts.Queue, true, false, false, false, args); err != nil {
		return fmt.Errorf("declare queue %s: %w", opts.Queue, err)
	}
	for _, key := range opts.BindingKeys {
		if err := ch.QueueBind(opts.Queue, key, opts.Exchange, false, nil); err != nil {
			return fmt.Errorf("bind %s -> %s (%s): %w", opts.Queue, opts.Exchange, key, err)
		}
	}
	return nil
}

// Consume declares topology, sets prefetch, and starts a background
// goroutine feeding deliveries to handler until the channel closes.
func (c *AMQPClient) Consume(ctx context.Context, opts ConsumerOptions, handler func(ConsumeMessage)) error {
	if err := c.DeclareTopology(ctx, opts); err != nil {
		return err
	}

	prefetch := opts.Prefetch
	if prefetch <= 0 {
		prefetch = 1
	}
	if err := c.channel.Qos(prefetch, 0, false); err != nil {
		return fmt.Errorf("qos: %w", err)
	}

	deliveries, err := c.channel.Consume(opts.Queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", opts.Queue, err)
	}

	go func() {
		for d := range deliveries {
			delivery := d
			handler(ConsumeMessage{
				RoutingKey: delivery.RoutingKey,
				Body:       delivery.Body,
				Ack:        func() { _ = delivery.Ack(false) },
				Nack:       func(requeue bool) { _ = delivery.Nack(false, requeue) },
			})
		}
	}()
	return nil
}
