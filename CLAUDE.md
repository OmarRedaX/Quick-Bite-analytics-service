# CLAUDE.md — Analytics Service Guidelines

These rules apply to the **`analytics-service`** microservice of the
**QuickBite** platform. It is the Go sibling of `core-service` and
`order-service` (both Node/TS) — same platform conventions (JWT shape,
response envelope philosophy, layering discipline), translated to Go
idiom. When in doubt, look at how `order-service` does the equivalent thing
and find the Go analogue — see `docs/node-to-go-mapping.md`. Deviate only
where a deviation is documented here or in that doc's "when to break the
mapping" section.

---

## 1. Mission of this service

This service owns **per-day rollups of order/payment/delivery activity**:

- **Consumes** events from `order-service` via RabbitMQ.
- **Upserts** day-grained aggregates into MongoDB (`agg_restaurant_day`,
  and — as homework — `agg_branch_day`, `agg_product_day`,
  `agg_platform_day`).
- **Serves** read-only HTTP endpoints under `/api/v1/analytics/...`.

It does **not** own users, restaurants, products, orders, or payments —
those live in `core-service`/`order-service`. It does **not** write to any
operational data store. It does **not** emit events; it is a pure sink.

---

## 2. Tech stack (locked — do not deviate)

| Concern      | Library / Tool                                |
| ------------ | ---------------------------------------------- |
| Runtime      | Go 1.21+                                       |
| HTTP router  | `github.com/go-chi/chi/v5`                     |
| DB driver    | `go.mongodb.org/mongo-driver` (official, v1)   |
| Messaging    | `github.com/rabbitmq/amqp091-go`               |
| Config       | `github.com/caarlos0/env/v11` (struct tags)    |
| Logger       | stdlib `log/slog`                              |
| Validation   | `github.com/go-playground/validator/v10`       |
| JWT          | `github.com/golang-jwt/jwt/v5`                 |
| UUID         | `github.com/google/uuid`                       |
| DI           | **none** — explicit constructor wiring in `lib/boot/boot.go` |

**Forbidden:** GORM, Ent, or any ODM. Repositories use the official
mongo-driver directly with typed structs and `bson` tags — the deliberate
Go analogue of "Knex query builder, not an ORM."

**Forbidden:** DDL migrations. Mongo is schemaless. Indexes live in one
file per module (`app/analytics/repository/indexes.go`) and are created
idempotently on boot via `EnsureIndexes`. `pkg/mongo` knows nothing about
collections — see §3.

---

## 3. Folder structure

```
cmd/api/main.go                 ~10 lines: calls lib/boot.Run(), nothing else
pkg/                             framework-free, NO app knowledge
  mongo/client.go                Connect/Disconnect only
  messaging/{types.go,amqp.go}   Broker interface + amqp091-go impl
  httpclient/client.go           net/http wrapper: timeout, JSON, retry-on-5xx
lib/                              app-aware glue: env, middleware, cross-cutting infra
  boot/boot.go                   wires every singleton; main.go just calls Run()
  config/env.go                  struct + Load()
  logger/logger.go                slog wrapper + FromContext
  appcontext/context.go          ctx keys: claims, correlation_id
  errors/{apperror.go,handler.go} AppError + Wrap(handler) middleware
  http/response.go               SendSuccess, SendPaginated, SendError
  middleware/correlation.go      Correlation + AccessLog
  auth/{jwt.go,middleware.go,apikey.go}
  rbac/{cache.go,middleware.go}  in-process cache + Require("perm")
  coreclient/{client.go,rbac.go,types.go}
  coreevents/{consumer.go,payloads.go}  generic only; app payloads live in app/
app/
  analytics/                     PACKAGE analytics — shared module types
    types.go                     OnOrderPlacedInput, RestaurantDayRow
    errors.go                    var Err… = apperror.New(...)
    enums.go                     const PermAnalyticsRead = "analytics:read"
    entity/                      plain structs + bson tags
    repository/                  ONLY place mongo-driver appears
      indexes.go                 EnsureIndexes for this module's collections
    service/analytics.service.go JUST the service struct + methods
    controller/{*.controller.go,routes.go}
    dto/{*.request.go,*.response.go}
    eventhandlers/handlers.go    event type -> service method map
play/                             GITIGNORED — dev aids: mock-core, mint-jwt, publish-test, check-mongo
```

### Layering (enforced by review)

```
app/  -> may import lib, pkg
lib/  -> may import pkg, config; may NOT import app/<module>/*
pkg/  -> no imports from lib or app, no config, no globals, no app-specific knowledge
```

`pkg/mongo` knows how to connect; it does not know this service has an
`agg_restaurant_day` collection — that's app knowledge, lives in
`app/analytics/repository/indexes.go`. If you find yourself writing a
collection name in `pkg/`, stop.

If a `lib/*` file needs something from `app/*`, **invert the dependency**:
define a small interface in `lib/`. Example: `lib/coreevents` needs a "have
I seen this event before?" capability — it defines `EventDeduper` as an
interface, and `app/analytics/repository.EventIDsRepo` satisfies it
implicitly via Go duck typing. Never add an `app/*` import to `lib/`.

### Why `cmd/api/main.go` is tiny

The entry point does one thing: hand control to `lib/boot.Run()` and exit
with its error. Every new singleton is wired in `boot.go`, not `main.go` —
adding a module means editing one function, not the entry point.

### Why `play/` instead of `cmd/`

`play/` is gitignored. `mock-core`, `mint-jwt`, `publish-test`,
`check-mongo` make the one built slice testable without Postgres or a
running order-service — they are dev aids, not production binaries.
Anything ad-hoc/experimental goes in `play/`, never committed at the repo
root or inside `app/`/`lib`/`pkg`.

### Why types/errors/enums live in `app/analytics/` (parent), not `service/`

The service struct is one concern; the types it consumes and returns are a
different concern. Putting `OnOrderPlacedInput`, `ErrInvalidDateRange`,
`PermAnalyticsRead` in the parent `analytics` package means `service`,
`controller`, and `eventhandlers` all import them the same way
(`analytics.X`), and `service/` stays focused on business logic only.

### Inline types — forbidden

Never declare a `type X struct{...}` inside a `*.service.go`,
`*.controller.go`, or `*.repo.go` file. Module-shared types go in
`app/<module>/types.go`; cross-cutting infra types go in
`lib/<area>/types.go` or `lib/<area>/payloads.go`.

**One narrow exception:** an unexported, single-purpose interface declared
in the file that *consumes* it (e.g. `service`'s `restaurantDayRepo`
interface in `analytics.service.go`, satisfied implicitly by the concrete
repo type) is idiomatic Go dependency injection, not a data type — it has
no TS equivalent and is not what the "no inline types" rule is guarding
against. See `docs/node-to-go-mapping.md`.

---

## 4. Naming conventions

### Files
- `snake_case.go` for most files (`restaurant_day.repo.go`); `camelCase`
  only where Go tooling expects it (none here). One export focus per file.

### Go
- `PascalCase` exported types/functions; `camelCase` unexported.
- Package names are short, lowercase, no underscores (`apperror`,
  `response`, `coreclient`) — chosen deliberately in a couple of spots
  (`lib/errors` → package `apperror`, `lib/http` → package `response`) to
  avoid colliding with stdlib `errors`/`net/http` at every call site. See
  `docs/node-to-go-mapping.md` for the rationale.
- Constants: `PascalCase` for exported (`PermAnalyticsRead`), matching Go
  convention (not `UPPER_SNAKE` like the Node services).

### MongoDB
- Collections: plural, `snake_case` (`agg_restaurant_day`, `event_ids`).
- Fields: `snake_case` via `bson` tags (`restaurant_id`, `orders_count`).
- Money: `INT64` minor units (piasters/cents). Never a float for money.
- Dates that key a document: plain `YYYY-MM-DD` string, UTC — not a
  `time.Time` at midnight, so range queries are simple string comparisons
  and there's no timezone ambiguity in the key itself.

### Routes
- Plural resource nouns, nested under the owning resource:
  `/restaurants/:restaurantId/days`.
- This service only ever reads — no `POST`/`PATCH`/`DELETE` in its public
  API today.

---

## 5. Module file conventions

Every module under `app/<module>/` follows this skeleton (see
`app/analytics/` as the canonical, only example today):

1. **`types.go`** (parent package) — module-shared input/output types.
2. **`errors.go`** (parent package) — `var Err… = apperror.New(...)`.
3. **`enums.go`** (parent package) — permission/status constants.
4. **`entity/*.go`** — plain structs, `bson` tags, no DB knowledge, no
   methods beyond trivial invariants.
5. **`repository/*.repo.go`** — one file per collection-shaped concern.
   Exported methods on a small struct wrapping `*mongo.Collection`. The
   *only* place `go.mongodb.org/mongo-driver` is imported.
6. **`repository/indexes.go`** — `EnsureIndexes(ctx, db, ...)`, called once
   at boot.
7. **`service/*.service.go`** — business logic. Composes repositories
   through narrow, unexported interfaces (not concrete repo types) so the
   service package never imports mongo-driver.
8. **`controller/*.controller.go`** — methods with signature
   `func(w http.ResponseWriter, r *http.Request) error`. Validate → call
   service → DTO → respond. No business logic.
9. **`controller/routes.go`** — wires middleware (`auth.Authenticate`,
   `rbac.Require`) and mounts controller methods via `apperror.Wrap`.
10. **`dto/*.request.go`** — parses + validates path/query/body via
    `github.com/go-playground/validator/v10` struct tags.
11. **`dto/*.response.go`** — response shape + a `...ResponseFrom(...)`
    mapping function. Controllers never return an internal type directly.
12. **`eventhandlers/handlers.go`** — `Register(consumer, service)` maps
    event type strings to service calls.

---

## 6. Response shape

```jsonc
// success
{ "success": true, "data": ... }

// error
{ "success": false, "error": { "code": "STABLE_CODE", "message": "human-readable" } }
```

- Every HTTP response goes through `lib/http` (package `response`):
  `SendSuccess`, `SendPaginated`, or (from `errors.Wrap`) `SendError`.
  Controllers never `w.Write` JSON by hand.
- Controllers **never** return an entity or repository row directly — only
  a DTO from `dto/*.response.go`.
- Money fields are integer minor units, paired with a `currency` field.
  Never pre-formatted/localized server-side.
- Timestamps are ISO-8601 UTC strings; date-only keys (`"date"`) are
  `YYYY-MM-DD`.
- Every `AppError` (`lib/errors`, package `apperror`) carries a stable
  `code`, an HTTP `statusCode`, and a `message`. Module-level
  `var Err... = apperror.New(...)` for named cases — never construct an
  ad-hoc `apperror.New(...)` at a call site for a case that already has a
  name.

---

## 7. Database (MongoDB) rules

- **No ODM.** Typed structs with `bson` tags, official driver, explicit
  `bson.D` filters/updates.
- **No DDL migrations.** `EnsureIndexes` runs on every boot and is
  idempotent (`CreateMany` with named indexes — re-running is a no-op).
- **Averages are never stored pre-divided.** Store `sum` + `count`; derive
  the average at read time in the **service** layer. This makes rollups
  associative — replaying the same events (even out of order, even twice
  before a would-be duplicate is caught) converges to the same totals.
- **Every query is backed by a named index.** No collection scans in the
  hot path — see `app/analytics/repository/indexes.go` for the two indexes
  backing this slice's write (`ApplyOrderPlaced`) and read
  (`FindByDateRange`).
- **Idempotency ledger, not Redis SETNX:** duplicate event detection uses a
  Mongo `event_ids` collection with a unique index, not Redis. See
  `docs/system-design.md` for the full rationale — short version: this
  service aggregates money, and losing a `SETNX`-guarded key to a Redis
  restart silently double-counts revenue; a Mongo unique-index insert
  failure is a durable, observable, compensable signal.

---

## 8. Cross-cutting infra

### Auth
JWT verified with `ACCESS_SECRET` (shared with core/order), HS256. Claims:
`userId, role, email, restaurantId?, restaurantRole?, branchIds?`. Token
from the `access_token` cookie first, then `Authorization: Bearer`.

### RBAC
No local permissions catalog. `system_admin` bypasses. A `restaurant_user`
is checked against permissions for their `restaurantRole`
(owner/branch_manager/staff), fetched from core-service's
`GET /api/internal/rbac/permissions?role=...` (header `api-key: <internal>`)
and cached in-process per role with a TTL (`RBAC_CACHE_TTL_SEC`, default
5 min). Invalidating that cache on `rbac.permissions_changed` is
**homework** (`lib/rbac.Cache.Invalidate` already exists as the hook —
nothing calls it yet).

### Idempotency
Every event funnels through `EventDeduper.MarkSeen(eventId)` before
dispatch. Duplicate → ack-and-skip. Unknown event type → log + ack-skip
(never DLQ an event this service simply doesn't handle yet). A handler
*failure* after `MarkSeen` succeeded calls `Unmark` before nacking, so a
DLQ replay can actually retry instead of being silently skipped forever as
"already seen."

### Error handling
Services/controllers return `error`. `errors.Wrap(handler)`
(`lib/errors/handler.go`) renders it: an `*apperror.AppError` (satisfies
the structural `response.CodedError` interface) renders with its own
code/status/message; anything else renders as a generic 500 so internals
never leak.

### Correlation & logging
`lib/middleware.Correlation` reads/generates `X-CorrelationId`, echoes it on
the response, and binds a request-scoped `*slog.Logger` (with
`correlation_id` pre-attached) into the request context. Every layer pulls
its logger via `logger.FromContext(ctx)` instead of a global. Structured
JSON on stdout via stdlib `log/slog`.

### Messaging
Inbound only. Topic exchange `order.events` (declared by order-service),
queue `analytics-service.order-events`, bindings `order.#, payment.#`, DLQ
`analytics-service.order-events.dlq`. This service never publishes — see
`pkg/messaging.Broker`, which deliberately has no `Publish` method.

---

## 9. Performance & scale rules

1. **No N+1 queries.** Reads batch by design (`FindByDateRange` is one
   query for the whole range, not one per day).
2. **Every query is backed by a named index.**
3. **Writes are single upserts** (`UpdateOne` with `$inc` + `$setOnInsert`)
   — no read-then-write race for the aggregate counters.
4. **Event handlers must be safe to call more than once** for the same
   logical effect (idempotent by construction via `$inc` upserts) — the
   dedupe layer is a belt-and-suspenders guard against *duplicate delivery*,
   not the only thing preventing double-counting.
5. **HTTP client to core-service has a timeout and bounded retries**
   (`pkg/httpclient`, retry-on-5xx only, never on 4xx).
6. **RBAC is cached** — the hot read path never blocks on a core-service
   round trip per request.

---

## 10. Code style — what to avoid

- ❌ ODMs, struct methods that hide a DB call, repository interfaces
  leaking into `service/` as concrete mongo-driver types.
- ❌ Returning entities/rows from controllers — always a response DTO.
- ❌ Business logic in controllers. Controllers: validate → service → DTO →
  respond.
- ❌ Swallowing errors (`if err != nil { return nil }`). Wrap or return.
- ❌ `panic` outside of programmer-error cases caught at boot (e.g.
  duplicate handler registration). Never `panic` on request-path errors.
- ❌ Constructing `apperror.New(...)` inline at a call site for an error
  that already has a stable, named instance.
- ❌ New env vars without adding them to `lib/config/env.go`'s struct tags
  (and `.env.example`).
- ❌ `mongo-driver` imports outside `app/<module>/repository/`.
- ❌ Committing anything under `play/` (it's gitignored on purpose).

---

## 11. When implementing a new module or handler

1. Add the entity struct (`entity/*.go`).
2. Add/extend the repository (`repository/*.repo.go` + indexes if new
   collection).
3. Add/extend the service method (`service/*.service.go`).
4. If it's an inbound event: add the payload struct + handler in
   `eventhandlers/handlers.go`, register it in `Register(...)`.
5. If it's an HTTP endpoint: request DTO, response DTO, controller method,
   route in `routes.go`.
6. Wire any new singleton in `lib/boot/boot.go`.
7. Smoke test with `play/publish-test` / `curl` before moving on.

Build **one vertical slice end-to-end** before starting the next — this
repo's own history is the example: `agg_restaurant_day` +`order.placed` +
one read endpoint, fully wired, before any of the homework aggregates.

---

## 12. Reference docs (in `docs/`)

- `docs/folder-structure.md` — annotated tree.
- `docs/system-design.md` — platform diagram, sync/async flows, failure
  modes, Mongo-dedupe rationale.
- `docs/api-contracts.md` — every endpoint's request/response shape and
  error code.
- `docs/node-to-go-mapping.md` — TS idiom → Go idiom, side by side, for
  every layer. Read this before porting any homework item from the Node
  services.
- `docs/ai-prompts.md` — prompt templates for directing an AI through the
  homework.
- `docs/implementation-plan.md` — phased build order with acceptance
  checks per phase.
- `plan.md` — what shipped in this slice vs. what's homework.

---

## 13. Out of scope (do not build here)

- Anything that writes to Postgres, or reads from it directly — this
  service only ever talks to Mongo and (read-only) core-service HTTP.
- Publishing events — this service is a pure sink.
- A local permissions catalog / RBAC seed data — always sourced from
  core-service.
- DevOps/deploy infra, observability stack, load/perf testing — separate
  effort.
- The three extra aggregate collections, three extra event handlers, seven
  extra endpoints, the `rbac.permissions_changed` consumer, and the backfill
  command — all explicitly homework, see `plan.md`.
