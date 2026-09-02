# analytics-service (Quick Bite)

Go + MongoDB + RabbitMQ microservice that owns per-day rollups of order
activity for the Quick Bite platform. It **consumes** domain events from
`order-service` over RabbitMQ, **upserts** day-grained aggregates into
MongoDB, and **serves** read-only HTTP endpoints for restaurant analytics.

It does **not** own users, restaurants, products, orders, or payments, does
**not** write to any operational data store, and does **not** emit events of
its own.

> This is a teaching artifact built as a full vertical slice end-to-end —
> four aggregate collections, four inbound events, eight read endpoints —
> with a `rbac.permissions_changed` cache-invalidation consumer and a
> backfill command left as homework. See [`plan.md`](./plan.md) for
> exactly what's built vs. what's left, and
> [`docs/node-to-go-mapping.md`](./docs/node-to-go-mapping.md) if you're
> coming from the Node services in this monorepo (`core-service`,
> `order-service`) and want the TS → Go translation.

---

## Table of contents

- [Stack](#stack)
- [Architecture at a glance](#architecture-at-a-glance)
- [Folder structure](#folder-structure)
- [Prerequisites](#prerequisites)
- [Setup](#setup)
- [Running it — three terminals](#running-it--three-terminals)
- [Verifying the slice end-to-end](#verifying-the-slice-end-to-end)
- [Postman collection (manual QA)](#postman-collection-manual-qa)
- [API](#api)
- [Events consumed](#events-consumed)
- [Response envelope & error codes](#response-envelope--error-codes)
- [Environment variables](#environment-variables)
- [Dev tools (`play/`)](#dev-tools-play)
- [Mongo collections & indexes](#mongo-collections--indexes)
- [Auth & RBAC](#auth--rbac)
- [Troubleshooting](#troubleshooting)
- [What's built vs. homework](#whats-built-vs-homework)
- [Further docs](#further-docs)

---

## Stack

Locked — see `CLAUDE.md` §2 before adding anything not on this list.

| Concern      | Choice                                       |
| ------------ | --------------------------------------------- |
| Runtime      | Go 1.21+                                      |
| HTTP router  | `github.com/go-chi/chi/v5`                    |
| DB driver    | `go.mongodb.org/mongo-driver` (official, v1)  |
| Messaging    | `github.com/rabbitmq/amqp091-go`              |
| Config       | `github.com/caarlos0/env/v11` (struct tags)   |
| Logger       | stdlib `log/slog`                             |
| Validation   | `github.com/go-playground/validator/v10`      |
| JWT          | `github.com/golang-jwt/jwt/v5`                |
| UUID         | `github.com/google/uuid`                      |
| DI           | none — explicit constructor wiring in `lib/boot/boot.go` |

No ORM/ODM (mongo-driver directly, typed structs + `bson` tags). No DDL
migrations (Mongo is schemaless; indexes are declared idempotently on boot —
see `app/analytics/repository/indexes.go`).

## Architecture at a glance

```
order-service ──(order.placed, RabbitMQ topic "order.events")──▶ analytics-service ──▶ MongoDB
                                                                        │
                                                                        ▼
                                                          core-service (RBAC permissions, sync HTTP)
                                                                        ▲
                                                                        │
                                                          browser/client ──(JWT)──▶ GET /api/v1/analytics/...
```

- **Inbound async:** one RabbitMQ consumer, queue
  `analytics-service.order-events`, bound to `order.#` and `payment.#` on
  the `order.events` topic exchange. `order.placed`, `order.delivered`,
  `order.rejected`, and `payment.completed` all have handlers wired up;
  anything else is acked and skipped (see
  `app/analytics/eventhandlers/handlers.go`).
- **Idempotency:** every event is deduped through a `event_ids` Mongo
  collection with a unique index on `event_id` — see
  [`docs/system-design.md`](./docs/system-design.md) for why Mongo, not
  Redis `SETNX`.
- **Outbound sync:** one HTTP call to core-service
  (`GET /api/internal/rbac/permissions?role=...`), cached in-process by role
  with a TTL (`lib/rbac/cache.go`).
- **HTTP API:** eight read endpoints, JWT-authenticated, RBAC-gated.

## Folder structure

See [`docs/folder-structure.md`](./docs/folder-structure.md) for the fully
annotated tree and layering rules. Summary:

```
cmd/api/main.go        entry point — ~10 lines, just calls lib/boot.Run()
pkg/                    framework-free: mongo client, messaging broker, http client
lib/                    app-aware glue: config, logger, errors, http response, auth, rbac,
                        coreclient (sync HTTP to core), coreevents (generic consumer), boot
app/analytics/          the one business module: types/errors/enums (parent),
                        entity, repository (only place mongo-driver appears),
                        service, controller, dto, eventhandlers
play/                   gitignored dev tools — mint-jwt, mock-core, publish-test, check-mongo
docs/                   design docs + the Node→Go teaching doc
```

Layering (enforced by review, see `CLAUDE.md` §Layering):
`app/` → may import `lib/`, `pkg/` · `lib/` → may import `pkg/`; may **not**
import `app/<module>/*` · `pkg/` → imports nothing app-specific.

## Prerequisites

- **Go 1.21+** (`go version`)
- **MongoDB** running locally (or reachable via `MONGO_URI`)
- **RabbitMQ** running locally (or reachable via `RABBITMQ_URL`)

On this dev machine both run as native Windows services rather than Docker.
Check/start them from PowerShell:

```powershell
Get-Service | Where-Object { $_.Name -match 'mongo|rabbit' }

# MongoDB is often stopped by default — start it:
Start-Service MongoDB
# or, from an elevated prompt: net start MongoDB

# RabbitMQ usually auto-starts; if not:
Start-Service RabbitMQ
```

If you'd rather run them in Docker instead:

```bash
docker run -d --name qb-mongo -p 27017:27017 mongo:7
docker run -d --name qb-rabbit -p 5672:5672 -p 15672:15672 rabbitmq:3-management
```

## Setup

```bash
cd analytics-service
go mod download

cp .env.example .env
# defaults already match a fresh local mongod (mongodb://localhost:27017)
# and the native Windows RabbitMQ service (amqp://guest:guest@localhost:5672).
# You only need to edit .env if your ports/credentials differ.
```

`.env` is read from the current working directory at boot — always run `go
run`/`go build` output from the repo root (`analytics-service/`), not from a
subdirectory.

Key vars (full list in [Environment variables](#environment-variables) and
`.env.example`):

- `ACCESS_SECRET` — same JWT secret as `core-service`/`order-service`. Local
  dev default (`change-me`) matches `order-service/.env`.
- `CORE_SERVICE_BASE_URL` — point this at `play/mock-core` for local dev
  (default `http://localhost:4100`) or at a real running core-service.

## Running it — three terminals

This slice is fully testable without Postgres or a running order-service,
using the dev tools in `play/`.

**Terminal 1 — the API + event consumer:**

```bash
go run ./cmd/api
```

Expected boot log (structured JSON, one line per event):

```json
{"time":"...","level":"INFO","msg":"mongo connected","database":"quickbite_analytics"}
{"time":"...","level":"INFO","msg":"mongo indexes ensured"}
{"time":"...","level":"INFO","msg":"rabbit connected"}
{"time":"...","level":"INFO","msg":"event consumer started","queue":"analytics-service.order-events","bindings":["order.#","payment.#"]}
{"time":"...","level":"INFO","msg":"http listening","port":4002}
```

**Terminal 2 — mock core-service** (serves the RBAC permissions lookup):

```bash
go run ./play/mock-core -port 4100 -permissions analytics:read
```

**Terminal 3 — publish a test event:**

```bash
go run ./play/publish-test -restaurant 42 -total 2500 -currency EGP
```

Then mint a token and call the API:

```bash
go run ./play/mint-jwt -role restaurant_user -restaurantRole owner -restaurantId 42
# copy the printed token into TOKEN=

curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:4002/api/v1/analytics/restaurants/42/days?from=2026-01-01&to=2026-12-31" | jq
```

Expected response after one `publish-test` run:

```json
{
  "success": true,
  "data": [
    { "date": "2026-08-25", "ordersCount": 1, "revenueMinor": 2500, "currency": "EGP", "avgOrderMinor": 2500 }
  ]
}
```

Stop everything with Ctrl+C in each terminal — the API does a graceful
shutdown (HTTP server, broker, Mongo client all closed with a 10s timeout).

## Verifying the slice end-to-end

This mirrors the acceptance checklist in `plan.md` / `docs/implementation-plan.md`.
Run these **in order** against the three terminals above.

1. **Build & vet clean:**
   ```bash
   go build ./... && go vet ./...
   ```
2. **Boot logs** — confirm the four structured lines above appear (mongo
   connected, mongo indexes ensured, rabbit connected, event consumer
   started, http listening).
3. **Health check:**
   ```bash
   curl -s http://localhost:4002/health
   # {"success":true,"data":{"status":"ok"}}
   ```
4. **Publish an event and check Mongo:**
   ```bash
   go run ./play/publish-test -restaurant 42 -total 2500
   go run ./play/check-mongo -restaurant 42
   # -> one doc: {"orders_count":1,"revenue_sum":2500,...}
   ```
5. **Replay the same event (dedupe check)** — reuse `-eventId`:
   ```bash
   go run ./play/publish-test -restaurant 42 -total 2500 -eventId <same-id-as-above>
   go run ./play/check-mongo -restaurant 42
   # -> unchanged: still orders_count:1, revenue_sum:2500
   ```
6. **Publish a second, different event:**
   ```bash
   go run ./play/publish-test -restaurant 42 -total 1500
   go run ./play/check-mongo -restaurant 42
   # -> orders_count:2, revenue_sum:4000
   ```
7. **No auth → 401:**
   ```bash
   curl -s "http://localhost:4002/api/v1/analytics/restaurants/42/days?from=2026-01-01&to=2026-12-31"
   # {"success":false,"error":{"code":"UNAUTHENTICATED","message":"User not authenticated"}}
   ```
8. **Garbage token → 401** (same shape as above, `-H "Authorization: Bearer garbage"`).
9. **Valid JWT → 200 with the derived average:**
   ```bash
   TOKEN=$(go run ./play/mint-jwt -role restaurant_user -restaurantRole owner -restaurantId 42 | head -1)
   curl -s -H "Authorization: Bearer $TOKEN" \
     "http://localhost:4002/api/v1/analytics/restaurants/42/days?from=2026-01-01&to=2026-12-31" | jq
   # data: [{"date":"...","ordersCount":2,"revenueMinor":4000,"currency":"EGP","avgOrderMinor":2000}]
   ```
   `avgOrderMinor: 2000` (4000/2) confirms the average is derived in the
   service layer, not stored.
10. **Validation errors:**
    ```bash
    # from > to
    curl -s -H "Authorization: Bearer $TOKEN" \
      ".../restaurants/42/days?from=2026-12-31&to=2026-01-01"
    # 400 ANALYTICS_INVALID_DATE_RANGE

    # missing from
    curl -s -H "Authorization: Bearer $TOKEN" ".../restaurants/42/days?to=2026-12-31"
    # 400 VALIDATION_ERROR

    # bad path id
    curl -s -H "Authorization: Bearer $TOKEN" ".../restaurants/not-a-number/days?from=2026-01-01&to=2026-12-31"
    # 400 VALIDATION_ERROR
    ```
11. **Unknown restaurant → empty array, not an error:**
    ```bash
    curl -s -H "Authorization: Bearer $TOKEN" ".../restaurants/999999/days?from=2026-01-01&to=2026-12-31"
    # {"success":true,"data":[]}
    ```

## Postman collection (manual QA)

[`postman/`](./postman) holds the shared QuickBite Postman collections for
manual, cross-service QA — `core-service.postman_collection.json`,
`order-service.postman_collection.json`, and the
`QuickBite.postman_environment.json` environment they run against.

[`TESTING_GUIDE.md`](./TESTING_GUIDE.md) is the scripted run-book: import
the three files into Postman, start `core-service` + `order-service` (+
worker), and hit the requests in order — the collection auto-captures the
tokens/IDs each step needs from the last. This service's own read
endpoints aren't in that collection yet (they're read-only and covered by
the `curl` walkthrough in
[Verifying the slice end-to-end](#verifying-the-slice-end-to-end) above);
the guide is for exercising the order-placement flow that produces the
events this service consumes.

## API

### `GET /health`

No auth. Returns `{"success":true,"data":{"status":"ok"}}`.

### `GET /api/v1/analytics/restaurants/:restaurantId/days`

Auth: JWT required (cookie `access_token`, or `Authorization: Bearer`).
RBAC: caller needs the `analytics:read` permission (see
[Auth & RBAC](#auth--rbac)).

**Query params** (both required):

| Param | Format       | Notes                          |
| ----- | ------------ | ------------------------------- |
| `from`| `YYYY-MM-DD` | inclusive, UTC                  |
| `to`  | `YYYY-MM-DD` | inclusive, UTC, must be `>= from` |

**Response `data`** — array of:

```jsonc
{
  "date": "2026-08-25",          // YYYY-MM-DD, UTC
  "ordersCount": 2,
  "revenueMinor": 4000,          // integer minor units (piasters/cents)
  "currency": "EGP",
  "avgOrderMinor": 2000           // derived: revenueMinor / ordersCount, service layer only
}
```

### Everything else

Seven more endpoints — `GET /restaurants/:id/failures`,
`GET /restaurants/:id/delivery-avg`, `GET /restaurants/active`,
`GET /branches/:id/days`, `GET /branches/:id/products/:productId/days`,
`GET /platform/days`, `GET /platform/summary` — share this same
auth/RBAC/`from`/`to` pattern. Full request/response shapes and every
error code: [`docs/api-contracts.md`](./docs/api-contracts.md).

## Events consumed

| Event type           | Exchange        | Handler                                                              | Status |
| --------------------- | --------------- | ---------------------------------------------------------------------- | ------ |
| `order.placed`        | `order.events`  | fans out to `agg_restaurant_day`, `agg_branch_day`, `agg_product_day`, `agg_platform_day`, `order_context` | ✅ built |
| `order.delivered`     | `order.events`  | delivery-duration onto `agg_restaurant_day`/`agg_branch_day`/`agg_platform_day`, looked up via `order_context` | ✅ built |
| `order.rejected`      | `order.events`  | `failed_count` onto `agg_restaurant_day`/`agg_branch_day`/`agg_platform_day` | ✅ built |
| `payment.completed`   | `order.events`  | online-payment counters onto `agg_platform_day`                         | ✅ built |
| `rbac.permissions_changed` | `core.events` | `permCache.Invalidate(role)` — see `lib/rbac/eventhandler.go`      | ✅ built |

Topology: topic exchange `order.events` (declared by `order-service`),
queue `analytics-service.order-events`, bindings `order.#, payment.#`, DLQ
`analytics-service.order-events.dlq`. A handler error un-marks the event as
seen (see `lib/coreevents/consumer.go`) and nacks without requeue, routing
the message to the DLQ so a bad message can be inspected/replayed rather
than looping forever.

`rbac.permissions_changed` arrives on a **second**, unrelated exchange
(`core.events`, declared by `core-service`) — a second consumer/queue/
binding (`analytics-service.core-events`), not a new routing key on the
`order.events` consumer above. Same broker connection and `event_ids`
dedupe collection are reused (see `lib/boot/boot.go`).

`order.placed` payload consumed (subset of what `order-service` actually
publishes — extra fields like `status`, `paymentMethod`, `subtotal` are
ignored):

```json
{
  "orderId": "2eebdcdc-554d-4e1d-b2fe-ee8a9f152e02",
  "restaurantId": 42,
  "branchId": 7,
  "total": 2500,
  "currency": "EGP",
  "items": [{ "productId": 1, "quantity": 2, "unitPrice": 1000, "lineTotal": 2000 }],
  "placedAt": "2026-08-25T10:00:00.000Z"
}
```

`order.delivered`, `order.rejected`, and `payment.completed` payloads
consumed: see the doc comments on the matching payload structs in
`app/analytics/eventhandlers/handlers.go` — each declares only the fields
that handler actually reads, same convention as `order.placed` above.

## Response envelope & error codes

```jsonc
// success
{ "success": true, "data": ... }

// error
{ "success": false, "error": { "code": "...", "message": "..." } }
```

| Code                          | HTTP | When                                              |
| ------------------------------ | ---- | -------------------------------------------------- |
| `UNAUTHENTICATED`             | 401  | missing/invalid/expired token                     |
| `FORBIDDEN`                   | 403  | authenticated but missing `analytics:read`        |
| `VALIDATION_ERROR`            | 400  | missing/malformed `from`/`to`, bad `restaurantId`  |
| `ANALYTICS_INVALID_DATE_RANGE`| 400  | `from` after `to`                                 |
| `RBAC_UNAVAILABLE`            | 503  | core-service unreachable during permission check  |
| `INTERNAL_ERROR`              | 500  | unexpected failure (never leaks internals)        |

## Environment variables

See `.env.example` for the authoritative, always-up-to-date list with
defaults. Parsed and validated at boot in `lib/config/env.go` — missing a
`,required` var panics immediately rather than limping along with a zero
value.

| Var | Default | Notes |
| --- | ------- | ----- |
| `PORT` | `4002` | |
| `NODE_ENV` | `development` | |
| `ACCESS_SECRET` | *(required)* | shared with core/order-service |
| `MONGO_URI` | `mongodb://localhost:27017` | |
| `MONGO_DATABASE` | `quickbite_analytics` | |
| `MONGO_CONNECT_TIMEOUT_SEC` | `10` | |
| `RABBITMQ_URL` | *(required)* | e.g. `amqp://guest:guest@localhost:5672` |
| `RABBITMQ_ORDER_EVENTS_EXCHANGE` | `order.events` | declared by order-service |
| `RABBITMQ_ANALYTICS_QUEUE` | `analytics-service.order-events` | |
| `RABBITMQ_ANALYTICS_BINDINGS` | `order.#,payment.#` | |
| `RABBITMQ_ANALYTICS_DLX` / `_DLQ` | `order.events.dlx` / `analytics-service.order-events.dlq` | |
| `RABBITMQ_PREFETCH` | `32` | max in-flight unacked messages |
| `RABBITMQ_CORE_EVENTS_EXCHANGE` | `core.events` | declared by core-service; unrelated to `RABBITMQ_ORDER_EVENTS_EXCHANGE` above |
| `RABBITMQ_ANALYTICS_CORE_EVENTS_QUEUE` | `analytics-service.core-events` | |
| `RABBITMQ_ANALYTICS_CORE_EVENTS_BINDINGS` | `rbac.permissions_changed` | |
| `RABBITMQ_ANALYTICS_CORE_EVENTS_DLX` / `_DLQ` | `core.events.dlx` / `analytics-service.core-events.dlq` | |
| `CORE_SERVICE_BASE_URL` | *(required)* | point at `play/mock-core` in dev |
| `CORE_INTERNAL_API_KEY` | *(required)* | `api-key` header value |
| `CORE_HTTP_TIMEOUT_MS` | `5000` | |
| `ORDER_SERVICE_BASE_URL` | `http://localhost:4000` | `cmd/backfill-aggs` only — the live api process never calls order-service |
| `ORDER_SERVICE_INTERNAL_API_KEY` | *(empty)* | must match order-service's own `INTERNAL_API_KEY`; `cmd/backfill-aggs` refuses to start without it |
| `RBAC_CACHE_TTL_SEC` | `300` | in-process permission cache TTL — bounds staleness even if a `rbac.permissions_changed` event is lost |
| `EVENT_DEDUPE_TTL_DAYS` | `7` | TTL index on `event_ids.received_at` |
| `ORDER_CONTEXT_TTL_DAYS` | `45` | TTL index on `order_context.recorded_at` — must outlive the gap between `order.placed` and a (possibly slow) `order.delivered`/`order.rejected` |

`.env` is loaded with a tiny built-in dotenv reader (`lib/config/env.go`) —
it never overrides a var already set in the real environment, same
semantics as Node's `dotenv`.

## Dev tools (`play/`)

Gitignored, not part of the service — see `CLAUDE.md` for why these live
outside `cmd/`.

| Tool | Purpose |
| ---- | ------- |
| `play/mint-jwt` | prints an access token signed with the same `ACCESS_SECRET` the API reads from `.env` |
| `play/mock-core` | stands in for core-service's RBAC permissions endpoint; `POST /set-permissions?role=<role>` (body: comma-separated permissions) overrides a role's answer at runtime, no restart, for testing `rbac.permissions_changed` |
| `play/publish-test` | publishes one event to RabbitMQ in the real envelope shape — `-event order.placed\|order.delivered\|order.rejected\|payment.completed\|rbac.permissions_changed` (the last one publishes to `core.events`, not `order.events`) |
| `play/check-mongo` | prints rows from any of this service's collections, no mongo shell needed — `-collection agg_restaurant_day\|agg_branch_day\|agg_product_day\|agg_platform_day\|order_context` |

Each is `go run`-able directly; run `go run ./play/<tool> -h` for flags.

### Testing the `rbac.permissions_changed` consumer

With `cmd/api`, `play/mock-core -permissions ""`, and RabbitMQ all running:

```powershell
# 1. Warm the cache with a 403 (role has no analytics:read yet)
curl.exe -H "Authorization: Bearer $TOKEN" http://localhost:4002/api/v1/analytics/restaurants/42/days?from=2026-08-01&to=2026-08-31
# -> 403 FORBIDDEN

# 2. Grant the permission — no restart needed
curl.exe -X POST "http://localhost:4100/set-permissions?role=restaurant_owner" -d "analytics:read"

# 3. Tell analytics-service the permission set changed
go run ./play/publish-test -event rbac.permissions_changed -role restaurant_owner

# 4. Same request, immediately — no need to wait out RBAC_CACHE_TTL_SEC
curl.exe -H "Authorization: Bearer $TOKEN" http://localhost:4002/api/v1/analytics/restaurants/42/days?from=2026-08-01&to=2026-08-31
# -> 200
```

## Backfill command (`cmd/backfill-aggs`)

Replays historical orders through `service.OnOrderPlaced` — the exact same
call the live `order.placed` consumer makes — so a backfilled row can never
drift from what the live event would have produced. Reads from
order-service's internal `GET /api/internal/orders/history` endpoint (never
Postgres directly); see `docs/api-contracts.md` in `order-service` and
`docs/implementation-plan.md` Phase 10 here for the full design.

```powershell
# Sanity-check first: fetch and log without writing to Mongo
go run ./cmd/backfill-aggs -region eg -year 2025 -dry-run

# Then actually apply it
go run ./cmd/backfill-aggs -region eg -year 2025
```

One region and one calendar year per run (a year never straddles
order-service's hot/archive Postgres boundary, so each run reads a single
source). Safe to re-run for the same `region`/`year` — already-backfilled
orders are skipped via the `event_ids` collection, keyed
`backfill:order.placed:<orderId>`. That key is independent of a live
event's real `eventId`, so it does **not** protect against double-counting
an order the live consumer already processed off RabbitMQ — only backfill
historical dates the live consumer never saw.

## Mongo collections & indexes

Declared idempotently on boot in `app/analytics/repository/indexes.go` —
there is no migration system; Mongo is schemaless.

- **`agg_restaurant_day`** — `{restaurant_id, date, currency, orders_count,
  revenue_sum, delivery_ms_sum, delivery_ms_count, failed_count,
  updated_at}`.
  - unique `(restaurant_id, date)`
  - `(date, restaurant_id)` — supports cross-restaurant range scans
    (`GET /restaurants/active`)
- **`agg_branch_day`** — same shape, keyed by `branch_id`.
  - unique `(branch_id, date)`
- **`agg_product_day`** — `{branch_id, product_id, date, currency,
  quantity_sum, revenue_sum, updated_at}`.
  - unique `(branch_id, product_id, date)`
- **`agg_platform_day`** — `{date, currency, orders_count, revenue_sum,
  delivery_ms_sum, delivery_ms_count, failed_count,
  online_payments_count, online_payments_amount_sum, updated_at}`, keyed
  by **(date, currency)** — not just date — so two currencies active the
  same day are never summed together.
  - unique `(date, currency)`
- **`order_context`** — `{order_id, currency, placed_at, recorded_at}`, a
  short-lived per-order lookup written by `order.placed` and read back by
  `order.delivered`/`order.rejected`.
  - unique `order_id`
  - TTL on `recorded_at` (`ORDER_CONTEXT_TTL_DAYS`, default 45 days)
- **`event_ids`** — `{event_id, received_at}`, the idempotency ledger.
  - unique `event_id`
  - TTL on `received_at` (`EVENT_DEDUPE_TTL_DAYS`, default 7 days)

Averages are stored as `revenue_sum` + `orders_count`, never as a
pre-divided average, so replaying events (even out of order) always
converges to the same totals — see
[`docs/system-design.md`](./docs/system-design.md).

## Auth & RBAC

- JWT verified with `ACCESS_SECRET`, HS256, same claims shape as
  core/order (`userId, role, restaurantId?, restaurantRole?, branchIds?`).
  Token read from the `access_token` cookie first, then
  `Authorization: Bearer`.
- This service owns no permissions catalog. `system_admin` bypasses RBAC
  entirely; a `restaurant_user`'s permissions are looked up by
  `restaurantRole` (owner/branch_manager/staff) via core-service's
  `GET /api/internal/rbac/permissions?role=...`, cached in-process per role
  (`RBAC_CACHE_TTL_SEC`, default 5 min). Any other top-level role is 403.
- The cache is invalidated on `rbac.permissions_changed` (see
  `lib/rbac/eventhandler.go`, wired in `lib/boot/boot.go`) — a role's
  updated permissions take effect on the *next* request, not after
  `RBAC_CACHE_TTL_SEC` expires.

## Troubleshooting

- **`config: required environment variable "..." missing"`** — copy
  `.env.example` to `.env` and fill in the required vars, or check you're
  running from the repo root (`.env` is read relative to cwd).
- **`mongo connect failed, retrying` forever** — MongoDB isn't running.
  `Get-Service MongoDB` / `Start-Service MongoDB` (Windows) or start your
  Docker container.
- **`rabbit connect failed, retrying` forever** — same for RabbitMQ
  (`Start-Service RabbitMQ`). Check the RabbitMQ management UI at
  `http://localhost:15672` (guest/guest) if installed.
- **401 with a token you just minted** — confirm `play/mint-jwt` and
  `cmd/api` are reading the *same* `.env` (both resolve it relative to the
  directory you ran `go run` from — always run from the repo root).
- **403 after a valid token** — `play/mock-core` must be running and
  `CORE_SERVICE_BASE_URL` must point at it; check the mock-core terminal
  logs the permissions request.
- **Published event never shows up in Mongo** — check the API terminal for
  `"handler failed, sending to DLQ"` (bad payload — check `placedAt` is a
  parseable RFC3339 timestamp) or `"no handler registered"` (wrong
  `-routingKey`).

## What's built

Full breakdown in [`plan.md`](./plan.md). Four aggregate collections
(`agg_restaurant_day`, `agg_branch_day`, `agg_product_day`,
`agg_platform_day`) plus the `order_context` lookup; five event handlers
(`order.placed`, `order.delivered`, `order.rejected`, `payment.completed`,
`rbac.permissions_changed`); eight read endpoints; full
auth/RBAC/correlation/error/logging cross-cutting infra; and the
`cmd/backfill-aggs` historical-replay command — verified both via the
`play/` dev tools and live against real `core-service` + `order-service`
instances (see `plan.md`'s "Live end-to-end verification").

## Further docs

- [`docs/folder-structure.md`](./docs/folder-structure.md) — annotated tree.
- [`docs/system-design.md`](./docs/system-design.md) — platform diagram,
  sync/async flows, failure modes, Mongo-dedupe rationale.
- [`docs/api-contracts.md`](./docs/api-contracts.md) — every
  request/response shape and error code.
- [`docs/node-to-go-mapping.md`](./docs/node-to-go-mapping.md) — TS idiom →
  Go idiom, side by side, for every layer.
- [`docs/implementation-plan.md`](./docs/implementation-plan.md) — phased
  build order with acceptance checks per phase.
