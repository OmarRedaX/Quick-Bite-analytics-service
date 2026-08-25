# Folder structure

Annotated tree of everything in this repo, in the order you'd read it to
understand the service. See `CLAUDE.md` §3 for the layering rules this tree
enforces.

```
analytics-service/
├── cmd/
│   └── api/
│       └── main.go              # entry point: `boot.Run()`, nothing else
│
├── pkg/                          # framework-free, NO app knowledge, NO imports from lib/ or app/
│   ├── mongo/
│   │   └── client.go             # Connect(ctx, Config) / Disconnect — knows nothing about collections
│   ├── messaging/
│   │   ├── types.go              # Broker interface, ConsumeMessage, ConsumerOptions
│   │   └── amqp.go               # amqp091-go implementation of Broker
│   └── httpclient/
│       └── client.go             # net/http wrapper: timeout, JSON, retry-on-5xx
│
├── lib/                          # app-aware glue: env, middleware, cross-cutting infra
│   ├── boot/
│   │   └── boot.go               # wires every singleton; the only place that imports everything
│   ├── config/
│   │   └── env.go                # raw struct (env tags) -> grouped Config, .env loader
│   ├── logger/
│   │   └── logger.go             # slog wrapper: New(), WithContext(), FromContext()
│   ├── appcontext/
│   │   └── context.go            # ctx keys: Claims, correlation id (the "express.d.ts" of this service)
│   ├── errors/
│   │   ├── apperror.go           # AppError type, New(), WithCause(), FromValidation()
│   │   └── handler.go            # Wrap(handler) middleware — renders AppError as the error envelope
│   ├── http/
│   │   └── response.go           # SendSuccess, SendPaginated, SendError, CodedError interface
│   ├── middleware/
│   │   └── correlation.go        # Correlation (id + logger binding) + AccessLog
│   ├── auth/
│   │   ├── jwt.go                # VerifyAccessToken — HS256, shared ACCESS_SECRET
│   │   ├── middleware.go         # Authenticate — cookie then Bearer
│   │   └── apikey.go             # RequireInternalAPIKey — scaffolded, unused by this slice
│   ├── rbac/
│   │   ├── cache.go              # in-process TTL cache, role -> permissions
│   │   └── middleware.go         # Require(cache, permission)
│   ├── coreclient/
│   │   ├── client.go             # base HTTP client to core-service (api-key header, correlation id)
│   │   ├── rbac.go               # GetPermissionsByRole — matches rbac.PermissionsFetcher exactly
│   │   └── types.go               # Envelope[T], RolePermissionsResponse
│   └── coreevents/
│       ├── consumer.go           # generic consumer: envelope parse, dedupe, dispatch, DLQ
│       └── payloads.go           # Envelope struct, EventHandler type
│
├── app/
│   └── analytics/                 # the one business module — package `analytics`
│       ├── types.go               # OnOrderPlacedInput, RestaurantDayRow (shared across subpackages)
│       ├── errors.go              # var ErrInvalidDateRange = apperror.New(...)
│       ├── enums.go               # const PermAnalyticsRead = "analytics:read"
│       ├── entity/
│       │   ├── restaurant_day.go  # agg_restaurant_day document shape (bson tags)
│       │   └── event_id.go        # event_ids document shape
│       ├── repository/            # ONLY place go.mongodb.org/mongo-driver is imported
│       │   ├── indexes.go         # EnsureIndexes — this module's entire "migration"
│       │   ├── restaurant_day.repo.go  # ApplyOrderPlaced (upsert), FindByDateRange
│       │   └── event_ids.repo.go  # MarkSeen / Unmark — satisfies coreevents.EventDeduper
│       ├── service/
│       │   └── analytics.service.go    # OnOrderPlaced, GetRestaurantDays (avg derived here)
│       ├── controller/
│       │   ├── analytics.controller.go # GetRestaurantDays HTTP handler
│       │   └── routes.go               # auth -> rbac -> controller, mounted at boot
│       ├── dto/
│       │   ├── days.request.go   # RestaurantDaysQuery + validator tags
│       │   └── days.response.go  # RestaurantDayResponse + mapping function
│       └── eventhandlers/
│           └── handlers.go       # Register(consumer, service) — event type -> handler map
│
├── play/                          # GITIGNORED — dev aids, not part of the service
│   ├── mint-jwt/main.go           # signs a token with the same ACCESS_SECRET as the API
│   ├── mock-core/main.go          # stands in for core's RBAC permissions endpoint
│   ├── publish-test/main.go       # publishes one order.placed event
│   └── check-mongo/main.go        # prints agg_restaurant_day rows for a restaurant
│
├── docs/
│   ├── folder-structure.md        # this file
│   ├── system-design.md
│   ├── api-contracts.md
│   ├── node-to-go-mapping.md
│   ├── ai-prompts.md
│   └── implementation-plan.md
│
├── go.mod / go.sum
├── .env.example
├── .gitignore
├── CLAUDE.md
├── README.md
└── plan.md
```

## Reading order for a newcomer

1. `README.md` — what it does, how to run it.
2. `CLAUDE.md` — the rules, why they exist.
3. `cmd/api/main.go` → `lib/boot/boot.go` — literally how the process comes
   up; this is the dependency graph made explicit.
4. `app/analytics/service/analytics.service.go` — the actual business
   logic (short — this is deliberately the smallest file with the most
   consequence).
5. `app/analytics/eventhandlers/handlers.go` and
   `app/analytics/controller/routes.go` — the two ways in.
6. Everything under `lib/` and `pkg/` as needed — they're infrastructure,
   not domain logic, and shouldn't need reading top-to-bottom.
