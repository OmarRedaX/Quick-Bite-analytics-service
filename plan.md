# plan.md — video scope vs. homework

## Built (this slice, fully end-to-end)

- **One aggregate collection:** `agg_restaurant_day`
  (`{restaurant_id, date, currency, orders_count, revenue_sum,
  delivery_ms_sum, delivery_ms_count, updated_at}`). The two delivery
  fields exist in the schema now so a homework `order.delivered` handler
  never needs a breaking document shape change — they're just always zero
  today.
- **Indexes:** unique `(restaurant_id, date)` + range `(date,
  restaurant_id)` on `agg_restaurant_day`; unique `event_id` + TTL on
  `received_at` (7 days) on `event_ids`.
- **One inbound event:** `order.placed`, consumed from the `order.events`
  topic exchange (queue `analytics-service.order-events`, bindings
  `order.#, payment.#`, DLQ wired).
- **Idempotency:** Mongo `event_ids` unique index; duplicate `InsertOne` →
  ack-and-skip. Handler failure after mark-seen → unmark → DLQ, so a replay
  can actually retry.
- **One read endpoint:**
  `GET /api/v1/analytics/restaurants/:restaurantId/days?from=&to=` — money
  in integer minor units, `avgOrderMinor` derived in the service layer.
- **All cross-cutting infra:** JWT auth, RBAC read-through cache from core,
  correlation IDs, structured `slog` logging, `{success,data}` /
  `{success,error:{code,message}}` envelopes, graceful shutdown.
- **Four dev tools** (`play/`, gitignored): `mint-jwt`, `mock-core`,
  `publish-test`, `check-mongo` — the whole slice is testable without
  Postgres or a running `order-service`.

Acceptance checklist for this slice lives in `README.md` §"Verifying the
slice end-to-end" and `docs/implementation-plan.md` Phases 1–6.

## Homework (documented, not built)

1. **Three more aggregate collections** — `agg_branch_day`,
   `agg_product_day`, `agg_platform_day`. Same sum+count-not-average
   pattern as `agg_restaurant_day`.
2. **Three more event handlers** — `payment.completed`, `order.delivered`,
   `order.rejected`.
   - **Gotcha:** out-of-order delivery. `order.delivered` can arrive before
     `order.placed` is fully processed (different queues/consumers in
     general, and this consumer processes one message at a time but nothing
     guarantees cross-event ordering at the broker). An upsert with `$inc`
     handles this fine for counters, but a delivery-time average needs the
     placed timestamp — either stash it on the row when `order.placed`
     lands and read it back, or compute delivery duration entirely from
     fields already on the `order.delivered` payload (order-service already
     knows `placedAt`) so this handler never depends on `order.placed`
     having run first.
   - **Gotcha:** `agg_product_day` is **one upsert per line item** — a
     single `order.placed` event has N items, so use `BulkWrite` with N
     `UpdateOne` models instead of N round trips.
3. **Seven more endpoints** — failures, delivery-avg, active-restaurants,
   etc. The failure-rate and delivery-avg ones are **derived** from fields
   already on `agg_restaurant_day`/`agg_branch_day` — no new collections,
   just new service methods and response DTOs.
4. **`rbac.permissions_changed` consumer** — wire a handler in
   `eventhandlers` that calls `permCache.Invalidate(role)`
   (`lib/rbac/cache.go` already has `Invalidate`; nothing calls it yet).
   Note this event flows on a *different* exchange (`core.events`, per
   `core-service`) than the one this service is bound to today
   (`order.events`) — this is a second consumer/binding, not a new routing
   key on the existing one.
5. **Backfill command**, `cmd/backfill-aggs/` — reads historical orders
   (from `order-service`'s Postgres, via a new sync endpoint or a direct
   read replica — a real design decision to make, not just plumbing) and
   replays them through the same `service.OnOrderPlaced`-shaped path so the
   backfill can never drift from the live aggregation logic.

See `docs/implementation-plan.md` for phase numbers and the acceptance
check each homework item should pass before being considered done.
