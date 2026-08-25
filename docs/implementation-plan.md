# Implementation plan

Phases 1–6 are what's built in this repo today. Phase 0 and Phases 7+ are
homework — see `plan.md` for the plain-English version of the same split.
Each phase lists an acceptance check; don't start the next phase until the
current one's check passes for real (run the command, don't eyeball the
code).

## Phase 0 — scaffold

- `go mod init`, locked dependencies fetched (`go.mod`/`go.sum`).
- Folder skeleton per `CLAUDE.md` §3 / `docs/folder-structure.md`.
- `.env.example`, `.gitignore`.

**Acceptance:** `go build ./...` and `go vet ./...` exit 0 on an empty
`cmd/api/main.go` that just prints something — before any real logic
exists, the module and dependency graph must already be sound.

## Phase 1 — `pkg/` (framework-free layer)

- `pkg/mongo/client.go` — `Connect`/`Disconnect`, no collection knowledge.
- `pkg/messaging/{types.go,amqp.go}` — `Broker` interface + amqp091-go impl.
- `pkg/httpclient/client.go` — timeout, JSON, retry-on-5xx.

**Acceptance:** each file compiles standalone with zero imports from `lib/`
or `app/` (`go vet` won't catch this — check by eye, or by grepping import
blocks for `analytics-service/lib` / `analytics-service/app` inside `pkg/`
and confirming zero hits).

## Phase 2 — `lib/` cross-cutting infra

- `config`, `logger`, `appcontext`, `errors` (`AppError` + `Wrap`),
  `http` (response envelope), `middleware` (correlation + access log),
  `auth` (JWT + middleware), `rbac` (cache + middleware), `coreclient`,
  `coreevents` (generic consumer).

**Acceptance:** `go build ./lib/...` clean. No file in `lib/` imports
`analytics-service/app/...` (grep check, same as Phase 1).

## Phase 3 — `app/analytics`, storage layer

- `types.go`, `errors.go`, `enums.go` (parent package).
- `entity/{restaurant_day,event_id}.go`.
- `repository/indexes.go`, `repository/restaurant_day.repo.go`,
  `repository/event_ids.repo.go`.

**Acceptance:** with a local `mongod` running, a throwaway `go run` snippet
(or `play/check-mongo` once Phase 6 exists) can call `EnsureIndexes` twice
in a row without erroring — proves the index declarations are idempotent.

## Phase 4 — `app/analytics`, service + event consumption

- `service/analytics.service.go` (`OnOrderPlaced`, `GetRestaurantDays`).
- `eventhandlers/handlers.go` (`order.placed` only).

**Acceptance:** unit-level — construct a `service.Service` with a
hand-rolled fake `restaurantDayRepo`, call `OnOrderPlaced` twice with
different totals, call `GetRestaurantDays`, assert `avgOrderMinor` is the
integer-divided average. (No test files exist in this repo yet — this is
the shape a first test would take if/when tests are added; today this
phase's acceptance is exercised live via Phase 6's e2e run instead.)

## Phase 5 — `app/analytics`, HTTP surface

- `dto/{days.request,days.response}.go`.
- `controller/{analytics.controller,routes}.go`.

**Acceptance:** `go build ./app/analytics/...` clean; `routes.go`'s
middleware chain order matches `CLAUDE.md` §8 (`Authenticate` before
`rbac.Require`, both before the wrapped controller method).

## Phase 6 — wiring, dev tools, full e2e (the acceptance bar for "done")

- `lib/boot/boot.go`, `cmd/api/main.go`.
- `play/{mint-jwt,mock-core,publish-test,check-mongo}`.

**Acceptance — the full checklist, run for real, in order** (also in
`README.md` §"Verifying the slice end-to-end"):

1. `go build ./...` and `go vet ./...` clean.
2. Boot logs structured JSON: `mongo connected`, `mongo indexes ensured`,
   `rabbit connected`, `event consumer started`, `http listening`.
3. `GET /health` → 200 envelope.
4. Publish `order.placed` (`restaurant=42, total=2500`) → `check-mongo`
   shows `orders_count: 1, revenue_sum: 2500`.
5. Publish the same `eventId` again → unchanged (dedupe).
6. Publish a second, different event (`total=1500`) → `orders_count: 2,
   revenue_sum: 4000`.
7. `GET .../days` with no auth → 401 `UNAUTHENTICATED`.
8. `GET .../days` with a garbage token → 401.
9. `GET .../days` with a valid JWT (`mock-core` returning `analytics:read`)
   → 200, `avgOrderMinor: 2000` (confirms service-layer derivation).
10. `from > to` → 400 `ANALYTICS_INVALID_DATE_RANGE`. Missing `from` → 400
    `VALIDATION_ERROR`. Bad path id → 400.
11. Restaurant with no data → 200 `[]`.

Only once all eleven pass is this slice "done" — this is also the bar every
homework item below should be held to before calling *it* done.

---

## Phase 7 — homework: more aggregates + handlers

- `agg_branch_day`, `agg_product_day`, `agg_platform_day` (entity + repo +
  indexes, same pattern as `restaurant_day`).
- `payment.completed`, `order.delivered`, `order.rejected` handlers.

**Acceptance (per handler):** publish the event via a `publish-test`-style
tool (extend the existing one or add a flag), confirm the target
collection's row updates correctly with `check-mongo`, and specifically
confirm **replay-safety** (publish the same `eventId` twice, confirm no
double-count) and, for `order.delivered`, confirm **out-of-order safety**
(publish `order.delivered` for an order this service has never seen
`order.placed` for, confirm it doesn't crash the consumer or corrupt the
row).

## Phase 8 — homework: remaining endpoints

- Failures, delivery-avg, active-restaurants, branch days, product days,
  platform days, platform summary (see `docs/api-contracts.md`'s homework
  table for the full list).

**Acceptance (per endpoint):** matches the existing endpoint's bar — auth
401, RBAC 403, validation 400s, empty-result 200, and a real 200 with
correct derived numbers against seeded data.

## Phase 9 — homework: `rbac.permissions_changed` consumer

- New binding (or new consumer — see `docs/ai-prompts.md`'s prompt for this
  item, it's a real design question) on `core.events`, wired to
  `permCache.Invalidate(role)`.

**Acceptance:** change what `play/mock-core` returns for a role, publish a
`rbac.permissions_changed` event for that role, confirm the *next* request
picks up the new permission set without waiting for `RBAC_CACHE_TTL_SEC` to
expire.

## Phase 10 — homework: backfill command

- `cmd/backfill-aggs/main.go`.

**Acceptance:** run it against a seeded set of historical orders, confirm
the resulting `agg_restaurant_day` rows are bit-for-bit identical to what
replaying the same orders through the live `order.placed` consumer would
have produced — this is the check that proves the backfill didn't drift
from the live aggregation logic (see `docs/ai-prompts.md`'s prompt for
this item).
