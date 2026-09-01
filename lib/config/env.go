// Package config is the Go analogue of order-service's lib/config/env.ts:
// parse process env into a typed struct via struct tags (env/v11 instead of
// zod), fail fast on boot if anything required is missing, then group the
// flat vars into a nested, typed Config the rest of the app imports.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// raw mirrors the "baseSchema" in order-service's env.ts: one flat struct,
// one tag per env var, defaults inline. Nothing here is grouped yet.
type raw struct {
	Port    int    `env:"PORT" envDefault:"4002"`
	NodeEnv string `env:"NODE_ENV" envDefault:"development"`

	CORSOrigins string `env:"CORS_ORIGINS" envDefault:"http://localhost:3000"`

	AccessSecret string `env:"ACCESS_SECRET,required"`

	MongoURI            string `env:"MONGO_URI" envDefault:"mongodb://localhost:27017"`
	MongoDatabase       string `env:"MONGO_DATABASE" envDefault:"quickbite_analytics"`
	MongoConnectTimeout int    `env:"MONGO_CONNECT_TIMEOUT_SEC" envDefault:"10"`

	RabbitMQURL               string `env:"RABBITMQ_URL,required"`
	RabbitOrderEventsExchange string `env:"RABBITMQ_ORDER_EVENTS_EXCHANGE" envDefault:"order.events"`
	RabbitQueue               string `env:"RABBITMQ_ANALYTICS_QUEUE" envDefault:"analytics-service.order-events"`
	RabbitBindings            string `env:"RABBITMQ_ANALYTICS_BINDINGS" envDefault:"order.#,payment.#"`
	RabbitDLX                 string `env:"RABBITMQ_ANALYTICS_DLX" envDefault:"order.events.dlx"`
	RabbitDLQ                 string `env:"RABBITMQ_ANALYTICS_DLQ" envDefault:"analytics-service.order-events.dlq"`
	RabbitPrefetch            int    `env:"RABBITMQ_PREFETCH" envDefault:"32"`

	// core.events is a second exchange, declared by core-service, unrelated to
	// order.events above — a second consumer/queue/binding, not a new routing
	// key on the existing one (see CLAUDE.md's rbac.permissions_changed
	// homework note and docs/ai-prompts.md).
	RabbitCoreEventsExchange string `env:"RABBITMQ_CORE_EVENTS_EXCHANGE" envDefault:"core.events"`
	RabbitCoreEventsQueue    string `env:"RABBITMQ_ANALYTICS_CORE_EVENTS_QUEUE" envDefault:"analytics-service.core-events"`
	RabbitCoreEventsBindings string `env:"RABBITMQ_ANALYTICS_CORE_EVENTS_BINDINGS" envDefault:"rbac.permissions_changed"`
	RabbitCoreEventsDLX      string `env:"RABBITMQ_ANALYTICS_CORE_EVENTS_DLX" envDefault:"core.events.dlx"`
	RabbitCoreEventsDLQ      string `env:"RABBITMQ_ANALYTICS_CORE_EVENTS_DLQ" envDefault:"analytics-service.core-events.dlq"`

	CoreServiceBaseURL string `env:"CORE_SERVICE_BASE_URL,required"`
	CoreInternalAPIKey string `env:"CORE_INTERNAL_API_KEY,required"`
	CoreHTTPTimeoutMs  int    `env:"CORE_HTTP_TIMEOUT_MS" envDefault:"5000"`

	// order-service — read-only, internal-api-key-guarded HTTP dependency for
	// cmd/backfill-aggs only (not used by the live api process at all).
	OrderServiceBaseURL     string `env:"ORDER_SERVICE_BASE_URL" envDefault:"http://localhost:4000"`
	OrderServiceInternalKey string `env:"ORDER_SERVICE_INTERNAL_API_KEY" envDefault:""`

	RBACCacheTTLSec int `env:"RBAC_CACHE_TTL_SEC" envDefault:"300"`

	EventDedupeTTLDays int `env:"EVENT_DEDUPE_TTL_DAYS" envDefault:"7"`

	OrderContextTTLDays int `env:"ORDER_CONTEXT_TTL_DAYS" envDefault:"45"`
}

// Config is what the rest of the service imports — grouped, typed,
// time.Duration where it matters. Equivalent of the exported `env` object
// at the bottom of order-service's env.ts.
type Config struct {
	Port         int
	IsProduction bool
	CORSOrigins  []string

	JWT struct {
		AccessSecret string
	}

	Mongo struct {
		URI            string
		Database       string
		ConnectTimeout time.Duration
	}

	Rabbit struct {
		URL                 string
		OrderEventsExchange string
		Queue               string
		Bindings            []string
		DLX                 string
		DLQ                 string
		Prefetch            int

		CoreEventsExchange string
		CoreEventsQueue    string
		CoreEventsBindings []string
		CoreEventsDLX      string
		CoreEventsDLQ      string
	}

	Core struct {
		BaseURL        string
		InternalAPIKey string
		HTTPTimeout    time.Duration
	}

	OrderService struct {
		BaseURL        string
		InternalAPIKey string
	}

	RBAC struct {
		CacheTTL time.Duration
	}

	EventDedupeTTL  time.Duration
	OrderContextTTL time.Duration
}

// Load reads a .env file (if present, dev convenience — never overrides an
// already-set env var, same semantics as Node's dotenv), parses process env
// into `raw` via struct tags, and groups it into Config. Panics on missing
// required vars — boot should fail loudly, not limp along with zero values.
func Load() *Config {
	loadDotEnv(".env")

	var r raw
	if err := env.Parse(&r); err != nil {
		panic(fmt.Sprintf("config: %v", err))
	}

	cfg := &Config{
		Port:         r.Port,
		IsProduction: r.NodeEnv == "production",
		CORSOrigins:  splitTrim(r.CORSOrigins),
	}
	cfg.JWT.AccessSecret = r.AccessSecret

	cfg.Mongo.URI = r.MongoURI
	cfg.Mongo.Database = r.MongoDatabase
	cfg.Mongo.ConnectTimeout = time.Duration(r.MongoConnectTimeout) * time.Second

	cfg.Rabbit.URL = r.RabbitMQURL
	cfg.Rabbit.OrderEventsExchange = r.RabbitOrderEventsExchange
	cfg.Rabbit.Queue = r.RabbitQueue
	cfg.Rabbit.Bindings = splitTrim(r.RabbitBindings)
	cfg.Rabbit.DLX = r.RabbitDLX
	cfg.Rabbit.DLQ = r.RabbitDLQ
	cfg.Rabbit.Prefetch = r.RabbitPrefetch

	cfg.Rabbit.CoreEventsExchange = r.RabbitCoreEventsExchange
	cfg.Rabbit.CoreEventsQueue = r.RabbitCoreEventsQueue
	cfg.Rabbit.CoreEventsBindings = splitTrim(r.RabbitCoreEventsBindings)
	cfg.Rabbit.CoreEventsDLX = r.RabbitCoreEventsDLX
	cfg.Rabbit.CoreEventsDLQ = r.RabbitCoreEventsDLQ

	cfg.Core.BaseURL = r.CoreServiceBaseURL
	cfg.Core.InternalAPIKey = r.CoreInternalAPIKey
	cfg.Core.HTTPTimeout = time.Duration(r.CoreHTTPTimeoutMs) * time.Millisecond

	cfg.OrderService.BaseURL = r.OrderServiceBaseURL
	cfg.OrderService.InternalAPIKey = r.OrderServiceInternalKey

	cfg.RBAC.CacheTTL = time.Duration(r.RBACCacheTTLSec) * time.Second
	cfg.EventDedupeTTL = time.Duration(r.EventDedupeTTLDays) * 24 * time.Hour
	cfg.OrderContextTTL = time.Duration(r.OrderContextTTLDays) * 24 * time.Hour

	return cfg
}

func splitTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// loadDotEnv is a minimal KEY=VALUE parser — the Go stdlib has no dotenv,
// and pulling in a library for it isn't worth a locked-stack exception.
// Lines starting with # and blank lines are skipped. Does not overwrite
// vars already present in the real environment (matches dotenv's default).
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env file — fine, real env vars (or docker/CI) provide config
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
