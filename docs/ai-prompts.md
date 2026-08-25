# AI prompts for the homework

The skill you're practicing on this homework isn't "write Go." It's
**directing an AI through a non-trivial implementation in a language you
don't know well yet, by anchoring every request to a codebase you already
understand.** You already know how `order-service` does RBAC, outbox
publishing, and repository layering — every homework item here is "do that
again, in Go, in this repo." The prompts below are built around forcing the
AI to *show* it found the right anchor before it writes anything.

## The meta-rule: make the AI prove it understood the existing code first

Never open with "implement X." Open with a comprehension check you can
verify in five seconds. If the AI's answer to the comprehension prompt is
vague, generic, or doesn't cite actual file paths and function names, it
hasn't read the code — stop and make it read before you let it write
anything.

**Bad:**
> Add a `payment.completed` event handler to analytics-service.

**Better:**
> Before writing anything: read `app/analytics/eventhandlers/handlers.go`,
> `app/analytics/repository/restaurant_day.repo.go`, and
> `order-service/src/app/order/service/order.service.ts`'s
> `buildOrderTransitionPayload`/outbox-insert-on-status-transition. Tell me,
> in your own words: (1) what the `payment.completed` event payload
> actually contains on the wire, per order-service's code — not what you'd
> guess it contains, (2) which existing Mongo field(s) on
> `agg_restaurant_day` this handler would touch, (3) whether any of those
> fields are currently written by a different handler and could race. Don't
> write code yet.

If it can't do that accurately, it will confidently invent a payload shape
that doesn't match what `order-service` actually publishes.

## Ask for the Node analogue, then the Go translation — as two separate steps

Don't let the AI jump straight to Go. Make it name the Node file it's
translating, in a separate message/step from the implementation, so you can
catch a wrong anchor before code gets written on top of it.

> Which specific file/function in `order-service` or `core-service` is the
> closest analogue to what you're about to build? Quote the relevant lines.
> Then, separately, show me the Go translation and explain every place it
> deviates from a 1:1 port and why (cross-reference
> `docs/node-to-go-mapping.md`'s "when to break the mapping" section if it
> deviates).

This catches two failure modes at once: the AI picking the wrong Node
pattern to copy (e.g. copying `order-service`'s Redis-`SETNX` dedupe for a
new event handler here, when `docs/system-design.md` explains why this
service uses Mongo instead), and the AI silently inventing a Go convention
that doesn't match the rest of this repo (a new error-handling style, a new
response shape) because it didn't look.

## Concrete prompts, one per homework item

### `agg_branch_day` / `agg_product_day` / `agg_platform_day`

> Read `app/analytics/entity/restaurant_day.go`,
> `app/analytics/repository/restaurant_day.repo.go`, and
> `app/analytics/repository/indexes.go`. I want `agg_branch_day` — same
> sum+count-not-average pattern, keyed by `(branch_id, date)`. Show me: the
> entity struct, the two indexes (which one is unique — think about what
> makes a document identity here, same as `restaurant_day` did), and the
> repository method signature *before* writing the upsert body. Don't
> touch `service/` yet — I want to review the storage shape first.

### `payment.completed` / `order.delivered` / `order.rejected` handlers

> Read `app/analytics/eventhandlers/handlers.go` end to end — I want the
> new handler(s) to follow the exact same shape: a package-scoped payload
> struct with only the fields we need, a `coreevents.EventHandler` closure,
> one line added to `Register`. First, find and quote the actual payload
> shape order-service publishes for this event type (search
> `order-service/src/app/order/service/order.service.ts` for
> `OUTBOX_EVENT_FOR_STATUS` and `buildOrderTransitionPayload`). Then: does
> `order.delivered` arriving before `order.placed` finished processing
> break anything in the fields you're about to `$inc`? Walk me through that
> race before writing the handler.

### `agg_product_day` — the `BulkWrite` gotcha specifically

> `order.placed` payloads contain N items. Show me why a loop calling
> `UpdateOne` N times inside `onOrderPlaced` would be wrong for
> `agg_product_day` specifically — what does CLAUDE.md say about N+1
> writes? — then show the `BulkWrite` version with one `mongo.WriteModel`
> per line item. Don't write the whole handler yet, just this one method's
> body, so I can check the write models before you wire it in.

### Seven more endpoints (failures, delivery-avg, active-restaurants, ...)

> For `GET /restaurants/{id}/delivery-avg`: read
> `app/analytics/service/analytics.service.go`'s `GetRestaurantDays` — I
> want the new method to reuse `FindByDateRange` (or a near-identical
> query) rather than adding a new repository method, since
> `delivery_ms_sum`/`delivery_ms_count` are already on the same document.
> Confirm that's actually true by reading `entity/restaurant_day.go` before
> you write anything. Then show me just the DTO response shape — I want to
> approve the JSON field names before you wire the controller/route.

### `rbac.permissions_changed` consumer

> Read `lib/rbac/cache.go`'s `Invalidate` method and
> `order-service/src/lib/rbac/permission-cache.service.ts`'s
> `handlePermissionsChanged`. Two things before you write code: (1) this
> event is published on `core.events` by core-service, not `order.events`
> — does that mean a second `coreevents.Consumer`/second queue/second
> binding, or can the existing consumer just add a binding key? Reason
> through RabbitMQ topic-exchange binding semantics before answering. (2)
> Where does the payload's `role` field come from — quote the actual
> publish site in core-service.

### Backfill command (`cmd/backfill-aggs/`)

> This is the one homework item that's a genuine design decision, not just
> a port. Don't write code first. Answer: where does historical order data
> live that this command would read from (order-service's sharded
> Postgres — which shard(s)? all of them?), and what's the risk of the
> backfill logic drifting from `service.OnOrderPlaced`'s live aggregation
> logic if you reimplement the `$inc` upsert separately instead of calling
> the same service method? Propose an approach that makes that drift
> structurally impossible, not just "remember to keep them in sync."

## Prompts for verifying an AI's Go, once it's written

> Run `go build ./... && go vet ./...` and paste the output — don't tell me
> it builds, show me.

> Which package does this new file import that CLAUDE.md's layering rules
> (`app/ -> lib, pkg`; `lib/ -> pkg`; `pkg/ -> nothing app-specific`) say it
> shouldn't? Check every import line, not just the obviously suspicious
> ones.

> Does this handler leave the `event_ids` collection in a state where a DLQ
> replay would actually reprocess the event, or would it silently skip
> forever as "already seen"? Trace through what happens if your new
> handler's Mongo write fails after `MarkSeen` succeeds.

> Show me the exact `curl` commands you'd run against `play/mock-core` +
> `play/publish-test` + `play/mint-jwt` to prove this works end to end —
> then actually run them and paste the real output, not what you expect it
> to say.
