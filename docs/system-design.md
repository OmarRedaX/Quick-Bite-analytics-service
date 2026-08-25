# System design

## Platform diagram

```
┌────────────────┐        ┌──────────────────┐        ┌───────────────────────┐
│  core-service   │        │  order-service    │        │  analytics-service     │
│  (Node/TS)      │        │  (Node/TS)        │        │  (Go)                  │
│                 │        │                   │        │                        │
│ users           │◀──HTTP─┤ sync reads:        │        │                        │
│ restaurants     │  sync  │  branch, product,  │        │                        │
│ products        │        │  address, stock    │        │                        │
│ RBAC catalog    │        │                    │        │                        │
│                 │        │ orders             │        │ agg_restaurant_day     │
│                 │        │ payments           │        │ (+ homework: branch,   │
│                 │        │ deliveries         │        │  product, platform)    │
│                 │        │                    │        │                        │
│ events_outbox   │        │ events_outbox      │        │ event_ids (dedupe)     │
│  (core.events)  │        │  (order.events)    │        │                        │
└────────┬────────┘        └─────────┬─────────┘        └───────────┬────────────┘
         │                            │                              │
         │ RabbitMQ                   │ RabbitMQ                     │
         │ topic: core.events         │ topic: order.events          │
         │                            │  (order.#, payment.#) ───────▶│ consumes
         │                            │                                │
         └──────────▶ order-service consumes core.events               │
                        (product.*, branch.*, restaurant.*, rbac.*)     │
                                                                        │
         ┌──────────────────────────────────────────────────────────────┘
         │ GET /api/internal/rbac/permissions?role=...  (sync HTTP, api-key header)
         ▼
   core-service                                          browser/client
                                                                │
                                                                │ JWT (access_token cookie or Bearer)
                                                                ▼
                                                  GET /api/v1/analytics/restaurants/:id/days
                                                          (analytics-service)
```

Three independent services, three independent databases (Postgres ×2 +
Mongo), one message broker shared by topic exchange (each service owns its
own exchange/queue — nobody publishes into another service's exchange).
`analytics-service` is a leaf: it consumes from `order-service`, reads
(never writes) from `core-service`, and nothing consumes from it.

## Sync vs. async flows

### Async (the one this service is built around)

```
order-service                RabbitMQ                    analytics-service
──────────────                ────────                    ─────────────────
placeOrder() trx:
  INSERT orders
  INSERT events_outbox  ──┐
  COMMIT                  │
                           │ (separate process, polls outbox)
outbox-drain worker  ◀─────┘
  claim batch (SKIP LOCKED)
  publish to order.events ────▶ queue: analytics-service.order-events
  mark dispatched                        │
                                          ▼
                                 coreevents.Consumer.handleMessage
                                   1. parse envelope
                                   2. dedupe.MarkSeen(eventId)   ── Mongo event_ids, unique index
                                   3. dispatch to handler          (see "why Mongo, not Redis" below)
                                   4. handler upserts agg_restaurant_day
                                   5. ack (or: unmark + nack -> DLQ on failure)
```

This is **at-least-once, eventually consistent**. A client hitting
`GET /days` immediately after an order is placed may not see it yet — the
outbox drain runs on a tick (`OUTBOUND_EVENTS_DRAIN_TICK_SEC` in
order-service), not synchronously with the HTTP response. That's an
accepted tradeoff for an analytics rollup; it is never used to gate a
transactional decision.

### Sync (the only one this service makes)

```
analytics-service                          core-service
──────────────────                          ────────────
rbac.Require(cache, "analytics:read")
  cache miss for role "owner"
  ──── GET /api/internal/rbac/permissions?role=owner (api-key header) ────▶
  ◀─── {"success":true,"data":{"role":"owner","permissions":[...]}} ───────
  cache.Set("owner", permissions, ttl=5m)
```

Cached, so this is off the hot path for every request except a cold cache
entry. Bounded retries (`pkg/httpclient`, retry-on-5xx only) and a
`RBAC_UNAVAILABLE` (503) response if core-service is down and the cache is
cold for that role — the request fails loudly rather than silently
granting or denying access.

## Failure modes

| Failure | What happens | Why it's safe |
| --- | --- | --- |
| RabbitMQ down at boot | `boot.Run()` retries connect 5x with backoff, then fails boot loudly | Never serves traffic pretending the consumer is running |
| RabbitMQ drops mid-run | Consumer goroutine's delivery channel closes; **not auto-reconnected** in this slice (documented limitation, see `docs/node-to-go-mapping.md`) | Homework: wrap `pkg/messaging.AMQPClient` with reconnect-on-`NotifyClose` |
| Mongo down at boot | Same retry-then-fail-loud pattern as RabbitMQ | No silent partial boot |
| Mongo write fails mid-handler | Handler returns error → event is **unmarked** then nacked → DLQ | A DLQ replay can actually retry; it isn't permanently "already seen" |
| Duplicate delivery (broker redelivery, at-least-once semantics) | `event_ids` unique-index insert fails with dup-key → ack-and-skip | No double counting, no consumer crash |
| Malformed message body | `json.Unmarshal` fails → nack, no requeue | Never crash-loops the consumer on one bad message |
| Unknown event type (e.g. a future `payment.completed` before its handler exists) | Logged, acked, skipped | Doesn't jam the queue or DLQ things this service simply hasn't implemented yet |
| core-service unreachable, RBAC cache cold for that role | `RBAC_UNAVAILABLE` (503) | Fails closed, not open — never grants access on a guess |
| `from`/`to` malformed or `from > to` | `VALIDATION_ERROR` / `ANALYTICS_INVALID_DATE_RANGE` (400) before any DB call | Cheap to reject; never runs a query with a broken range |
| Restaurant with no data in range | `200 []` | Empty result is not an error condition |

## Why Mongo for dedupe, not Redis `SETNX`

`order-service`'s inbound consumer (Redis `SETNX` + 24h TTL) is the right
choice **for that consumer** because every one of its handlers is an
idempotent cache invalidation — if a dedupe key is lost to a Redis restart
and an event is reprocessed, the worst case is invalidating a cache entry
that didn't need it. Losing the key and *skipping* a genuinely new event is
also harmless there, because the source of truth (Postgres) was never
touched by that consumer in the first place.

`analytics-service` is different: its handlers **write money** (revenue
sums, order counts) into the only copy of that data that exists. For this
consumer:

1. **Durability matters.** A Redis restart with no persistence configured
   (the common dev/cost-optimized setup) silently forgets every dedupe key,
   and a broker redelivery after that restart would double-count revenue
   with no error, no log, nothing to alert on. A Mongo unique index is
   backed by the same durable store the aggregates live in.
2. **The failure mode must be loud and compensable.** A duplicate insert
   against a unique index is a specific, catchable error
   (`mongo.IsDuplicateKeyError`) — not a silent "well, the SETNX succeeded
   anyway." And because the dedupe record and the aggregate live in the
   same database, a handler failure can cleanly **unmark** the event
   (`EventIDsRepo.Unmark`) so a DLQ replay is a real retry, not a permanent
   skip — see the consumer's `handleMessage` in `lib/coreevents/consumer.go`.
3. **One fewer moving part.** This service already needs Mongo for the
   aggregates; adding Redis solely for dedupe is another connection, another
   failure mode, another thing to run in dev — for a check that's on the
   write path of every single event, not a cache.
4. **The tradeoff Redis buys — lower write latency — doesn't matter here.**
   This consumer processes events at the rate `order-service` places
   orders, not at request-response latency; a Mongo insert is not the
   bottleneck.

If this service ever needs a *cache* invalidation dedupe (e.g. the homework
`rbac.permissions_changed` consumer), Redis `SETNX` (or even an in-process
map with a TTL) would be the right, cheaper choice there — the same
reasoning that put Redis in `order-service`'s consumer applies. The
decision is per-consumer, based on what the handler does, not a blanket
rule.
