# Node → Go mapping

The real skill in this repo isn't "learn Go." It's **directing an AI
through a non-trivial implementation in an unfamiliar language by anchoring
it to a codebase you already know.** You already understand
`order-service`'s architecture — every decision in `analytics-service` is
that same decision, translated. This doc is the translation table. Read it
before touching any homework item; `docs/ai-prompts.md` shows how to make
an AI use it instead of inventing its own conventions.

Every section: a few lines of the Node original, the Go equivalent from
this repo, and why the shape changed (or didn't).

---

## Config

**Node (`order-service/src/lib/config/env.ts`):** `zod` schema parses
`process.env` into a flat object, then a second block groups it into a
nested, typed `env` export.

```ts
const baseSchema = z.object({
    PORT: z.string().default("4000"),
    ACCESS_SECRET: z.string(),
});
const parsed = baseSchema.parse(process.env);
export const env = {
    port: Number(parsed.PORT),
    jwt: { accessSecret: parsed.ACCESS_SECRET },
};
```

**Go (`lib/config/env.go`):** struct tags (`caarlos0/env/v11`) instead of a
schema builder; same two-phase shape — flat `raw` struct parsed from env,
then grouped into the exported `Config`.

```go
type raw struct {
    Port         int    `env:"PORT" envDefault:"4001"`
    AccessSecret string `env:"ACCESS_SECRET,required"`
}
type Config struct {
    Port int
    JWT  struct{ AccessSecret string }
}
func Load() *Config {
    var r raw
    env.Parse(&r) // panics on missing required — same as zod .parse() throwing
    cfg := &Config{Port: r.Port}
    cfg.JWT.AccessSecret = r.AccessSecret
    return cfg
}
```

**Why the shape is identical:** both are "the env is only trustworthy
right after boot" — parse once, fail loudly, hand the rest of the app a
typed value it can't get wrong. **Gotcha:** Go has no `dotenv` in the
locked stack, so `lib/config/env.go` includes a ~20-line hand-rolled
`.env` reader. Don't reach for a library here — it's not worth a
locked-stack exception for something this small.

---

## Logger

**Node:** a singleton class, `logger.info(msg, {field: value})`, imported
directly everywhere.

```ts
export const logger = new Logger();
logger.info("order placed", {orderId, correlationId: req.correlationId});
```

**Go:** stdlib `log/slog`, but *not* a bare package-level singleton for
request-scoped fields — a `*slog.Logger` with `correlation_id` already
bound is stashed on `context.Context` by middleware, and every layer pulls
it back out.

```go
// middleware, once per request:
ctx = logger.WithContext(ctx, base.With("correlation_id", id))

// anywhere downstream:
logger.FromContext(ctx).Info("order placed", "orderId", orderID)
```

**Why the shape changed:** Node's `req.correlationId` is ambient — every
handler has `req` in scope. Go has no ambient request object; `context.Context`
*is* the thing threaded through every call, so it's the natural place to
carry "fields every log line in this request should have," not a metadata
object repeated at every call site.

---

## Auth (JWT + middleware)

**Node (`order-service/src/lib/auth/{jwt,guard}.ts`):**

```ts
export function verifyAccessToken(token: string): JWTPayload {
    const decoded = jwt.verify(token, env.jwt.accessSecret) as jwt.JwtPayload & JWTPayload;
    return { userId: decoded.userId, role: decoded.role, ... };
}
export function authenticate(req, res, next) {
    const token = req.cookies?.access_token;
    if (!token) throw NotAuthenticated;
    req.user = verifyAccessToken(token);
    next();
}
```

**Go (`lib/auth/{jwt,middleware}.go`):**

```go
func VerifyAccessToken(tokenString, secret string) (appcontext.Claims, error) {
    var claims jwtClaims
    token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (any, error) {
        return []byte(secret), nil
    }, jwt.WithValidMethods([]string{"HS256"}))
    ...
}

func Authenticate(accessSecret string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)
            claims, err := VerifyAccessToken(token, accessSecret)
            ctx := appcontext.WithClaims(r.Context(), claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Two real differences, both deliberate:**

1. **Secret as a parameter, not an import.** Node's `verifyAccessToken`
   imports `env` directly. Go's `Authenticate(accessSecret string)` takes
   it as a constructor argument. This isn't cosmetic — it's the "no DI
   framework, explicit constructor wiring" rule applied to middleware:
   `lib/boot/boot.go` is the only place that reads `cfg.JWT.AccessSecret`
   and hands it to `Authenticate`, so `lib/auth` stays testable without an
   env in scope.
2. **`req.user = ...` becomes a new `context.Context`.** Go's
   `http.Request` is conceptually immutable per middleware layer — you
   don't mutate it, you derive a new context and call
   `r.WithContext(ctx)`. `appcontext.Claims` is the Go analogue of
   `express.d.ts`'s `req.user: JWTPayload` augmentation.

**Gotcha:** `appcontext.Claims` is declared in `lib/appcontext`, *not*
`lib/auth`, even though `lib/auth.VerifyAccessToken` returns it. If it
lived in `lib/auth`, `lib/appcontext` would need to import `lib/auth` (to
type the context value) and `lib/auth`'s middleware would need to import
`lib/appcontext` (to set it) — an import cycle. Putting the shared type in
the package that has no reason to import the other breaks the cycle. This
is a Go-specific problem; TS's structural typing means `express.d.ts`
never has this issue (it doesn't need to import `JWTPayload`'s *runtime*
value, just describe its shape, and even so `express.d.ts` typically does
import the type with no cycle risk since `.d.ts` files aren't part of the
runtime import graph).

---

## Errors

**Node (`order-service/src/lib/error/AppError.ts`):**

```ts
export class AppError extends Error {
    constructor(public message: string, public statusCode: number = 500, public isOperational = true) {
        super(message);
    }
}
export const OrderNotFoundError = new AppError("Order not found", 404);
// service:
throw OrderNotFoundError;
// centralized middleware:
res.status(err.statusCode).json({error: err.message});
```

**Go (`lib/errors/apperror.go` + `handler.go`):**

```go
type AppError struct {
    code, message string
    statusCode    int
}
func New(code string, statusCode int, message string) *AppError { ... }

var ErrInvalidDateRange = apperror.New("ANALYTICS_INVALID_DATE_RANGE", 400, "from must not be after to")

// service:
func (s *Service) GetRestaurantDays(...) (..., error) {
    if from > to {
        return nil, analytics.ErrInvalidDateRange
    }
}

// controller, wrapped by errors.Wrap:
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error
func Wrap(h HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if err := h(w, r); err != nil {
            response.SendError(w, logger.FromContext(r.Context()), err)
        }
    }
}
```

**The real translation is `throw` → `return err`.** Go has no exceptions;
"throw an AppError, let centralized middleware catch it" becomes "return an
error, let a wrapping function render it." Every controller method has
signature `func(http.ResponseWriter, *http.Request) error` instead of
`void`, and `errors.Wrap` is what used to be Express's `errorHandler`
middleware.

**Gotcha — the package name.** `lib/errors/apperror.go` declares
`package apperror`, not `package errors`. If it were `package errors`, any
file that also needs stdlib `errors` (for `errors.Is`/`errors.As`) would
have to alias one of the two imports at every call site. Naming the
package `apperror` up front means both import cleanly, unaliased,
everywhere. The directory is still `lib/errors` (matches the required
folder structure) — Go lets the package name and directory name differ,
and this is a case where you should use that.

**Gotcha — one more required code field.** Node's `AppError` is just
`message` + `statusCode`; this service's response envelope needs a stable
`code` string too (`{"error": {"code", "message"}}`), so Go's `AppError`
carries one more field than its Node ancestor. Not a Go-vs-Node thing —
just a slightly stricter envelope contract for this service.

---

## Response envelope

**Node (`order-service/src/lib/http/response.ts`):**

```ts
export function sendSuccess<T>(res: Response, data: T, statusCode = 200) {
    res.status(statusCode).json({success: true, data});
}
```

**Go (`lib/http/response.go`):**

```go
func SendSuccess(w http.ResponseWriter, statusCode int, data any) {
    writeJSON(w, statusCode, successEnvelope{Success: true, Data: data})
}
```

Nearly identical — `any` is Go's `unknown`/generic-object equivalent here
(no generics needed since the envelope wraps arbitrary already-serializable
data). **Gotcha, same trick as `apperror`:** the package is
`package response`, not `package http`, specifically so call sites can
`import "net/http"` and this package in the same file without aliasing.
Order-service doesn't hit this problem because TS has no equivalent
naming collision — `http` (Node builtin) and a local `response.ts` module
are never imported under the same bare identifier by accident.

---

## DTOs (request validation, response shaping)

**Node (`order-service`, `class-validator` decorators):**

```ts
export class CreateOrderRequestDTO {
    @IsString() branchId!: string;
    @IsEnum(PaymentMethod) paymentMethod!: PaymentMethod;
}
// controller: validateBody(CreateOrderRequestDTO, req.body)
```

**Go (`app/analytics/dto/days.request.go`, struct tags +
`go-playground/validator/v10`):**

```go
type RestaurantDaysQuery struct {
    RestaurantID int64  `validate:"required,gt=0"`
    From         string `validate:"required,datetime=2006-01-02"`
    To           string `validate:"required,datetime=2006-01-02"`
}
if err := validate.Struct(q); err != nil {
    return RestaurantDaysQuery{}, apperror.FromValidation(err)
}
```

**Decorators → struct tags** is the single biggest syntactic shift in this
whole codebase, and it's mechanical: `@IsString()` → `validate:"required"`
(Go has no separate "is a string" check — the field's Go type already
guarantees that), `@IsEnum(X)` → `validate:"oneof=a b c"`, `@IsOptional()`
→ simply omit `required`. `apperror.FromValidation` is the Go analogue of
`validateBody` throwing — one function that turns a validator library's
error type into the service's stable `VALIDATION_ERROR` shape.

**Response DTOs** map 1:1 in intent (`static from(entity)` factory ↔
`XResponseFrom(row)` function) but Go has no `static` methods — a
package-level function taking the domain type is the idiom:

```go
// TS
static from(order: OrderEntity): OrderResponseDTO { ... }
// Go
func RestaurantDayResponseFrom(row analytics.RestaurantDayRow) RestaurantDayResponse { ... }
```

---

## Repositories

**Node (`order-service`, Knex, exported functions over a shared `db`):**

```ts
export async function findOrderByPublicId(publicId: string, conn: Knex = db): Promise<OrderEntity | null> {
    const row = await conn("orders").where({public_id: publicId}).first();
    return row ? toEntity(row) : null;
}
```

**Go (`app/analytics/repository/restaurant_day.repo.go`, mongo-driver,
methods on a struct wrapping one collection):**

```go
type RestaurantDayRepo struct { coll *mongo.Collection }
func NewRestaurantDayRepo(db *mongo.Database) *RestaurantDayRepo {
    return &RestaurantDayRepo{coll: db.Collection(CollRestaurantDay)}
}
func (r *RestaurantDayRepo) FindByDateRange(ctx context.Context, restaurantID int64, from, to string) ([]entity.RestaurantDay, error) {
    cursor, err := r.coll.Find(ctx, bson.D{...})
    ...
}
```

**"Exported functions with an optional trx param" → "methods on a struct
holding the collection handle."** Both patterns exist to make the
repository composable (Knex's `conn: Knex = db` lets a caller pass a trx;
Go's method receiver is just how Go attaches behavior to a value). There is
no Mongo transaction here to thread through — single-document `UpdateOne`
with `$inc` is already atomic, so this repo doesn't need Knex's
"trx-or-default-connection" trick at all. If a future homework handler ever
needs a multi-document Mongo transaction (`session.WithTransaction`), the
session would be threaded through the same way Knex threads a `trx` — an
explicit parameter, not an ambient global.

---

## Services

**Node:** `@injectable()` class, dependencies via constructor + `tsyringe`
tokens, resolved from a DI container.

```ts
@injectable()
export class OrderService {
    constructor(@inject(TOKENS.CacheProvider) private cache: ICacheProvider) {}
}
```

**Go:** plain struct, dependencies via constructor, wired explicitly in
`lib/boot/boot.go` — no container, no tokens, no reflection.

```go
type Service struct { restaurantDays restaurantDayRepo }
func New(restaurantDays restaurantDayRepo) *Service { return &Service{restaurantDays: restaurantDays} }

// boot.go:
restaurantDayRepo := repository.NewRestaurantDayRepo(db)
analyticsService := service.New(restaurantDayRepo)
```

**This is the biggest architectural simplification in the whole port, and
it's a locked decision (`CLAUDE.md` §2: "DI: none — explicit constructor
wiring").** `tsyringe` exists because Node has no compile-time way to
verify a dependency graph is complete short of running it; Go's compiler
already refuses to build `service.New(nil)` if `nil` doesn't satisfy the
required interface, and `go build` catches a missing wire-up as a compile
error, not a runtime "cannot resolve dependency" exception. A DI framework
in Go would be solving a problem Go doesn't have.

**Gotcha — narrow interfaces, not concrete types.** `service.New` doesn't
take `*repository.RestaurantDayRepo` — it takes an unexported interface
(`restaurantDayRepo`) with just the two methods the service calls. This
keeps `mongo-driver` types from leaking into the `service` package (only
`repository` may import mongo-driver, per `CLAUDE.md` §3) and is a Go
idiom ("accept interfaces, return structs") with no direct TypeScript
analogue — TS's structural typing means every class is already usable as
"the shape it happens to have," so there's no equivalent ceremony there.

---

## Controllers

**Node:** `@injectable()` class, arrow-function methods (to preserve
`this` when passed to Express), explicit `validateBody` → service call →
DTO → `sendSuccess`.

```ts
getById = async (req: Request, res: Response) => {
    const order = await this.orderService.getOrder(req.user!, req.params.id);
    sendSuccess(res, OrderResponseDTO.from(order));
};
```

**Go:** plain methods with signature `(w, r) error`, no `this`-binding
concern (Go methods don't lose their receiver when passed around — no
arrow-function trick needed).

```go
func (c *Controller) GetRestaurantDays(w http.ResponseWriter, r *http.Request) error {
    q, err := dto.ParseRestaurantDaysQuery(r, chi.URLParam(r, "restaurantId"))
    if err != nil { return err }
    rows, err := c.service.GetRestaurantDays(r.Context(), q.RestaurantID, q.From, q.To)
    if err != nil { return err }
    response.SendSuccess(w, http.StatusOK, dto.RestaurantDaysResponseFrom(rows))
    return nil
}
```

**Gotcha the Node side has and Go doesn't:** Express method references
(`controller.getById` passed to `router.get(...)`) lose their `this`
binding unless declared as arrow-function class properties — a well-known
TS/JS foot-gun. Go method values (`ctrl.GetRestaurantDays`) always carry
their receiver; there's nothing to work around.

---

## Routes

**Node:**

```ts
router.get("/restaurants/:restaurantId/days", authenticate, rbac({resource: "analytics", action: "read"}), controller.getDays);
```

**Go (`chi`):**

```go
r.Use(auth.Authenticate(accessSecret))
r.Use(rbac.Require(permCache, analytics.PermAnalyticsRead))
r.Get("/restaurants/{restaurantId}/days", apperror.Wrap(ctrl.GetRestaurantDays))
```

Nearly 1:1. **Gotcha:** Express path params use `:param`; chi uses
`{param}`. Easy to typo when porting a route string by hand.

---

## Messaging (consumer)

**Node (`order-service/src/lib/core-events/consumer.ts`):** dedupe via
Redis `SETNX`, dispatch via a `Map<eventType, handler>`, manual ack/nack on
an `amqplib` message wrapped by `amqp-connection-manager` (which
auto-reconnects and replays channel setup).

**Go (`lib/coreevents/consumer.go` + `pkg/messaging/amqp.go`):** dedupe via
a Mongo unique index (see `docs/system-design.md` for why), dispatch via a
`map[string]EventHandler`, manual ack/nack on a `ConsumeMessage` wrapping
raw `amqp091-go` deliveries.

**The one real gap, not just a translation:** `amqp-connection-manager` has
no equivalent in this codebase's locked dependency list. `pkg/messaging.AMQPClient`
connects once and does **not** auto-reconnect if the connection drops
mid-run — documented as a homework item, not silently glossed over. If you
port a Node consumer that leans on `amqp-connection-manager`'s reconnect
behavior, you must design that behavior explicitly in Go (a supervisor
goroutine watching `conn.NotifyClose` and re-running `Connect` + `Consume`)
— it will not come for free the way it does in Node.

---

## Common gotchas when porting a Node file to Go

1. **`Promise<T>` → `(T, error)`, not just `T`.** Every async Node function
   that can fail becomes a Go function returning two values. Don't forget
   the error return on functions that "can't really fail" in the Node
   version — Go still makes you handle it (e.g. `json.Marshal` almost
   never fails on a well-formed struct, but the signature still returns
   `error`).
2. **`undefined`/optional fields → pointers or zero values, deliberately
   chosen.** `RestaurantID *int64` (a pointer) is used where "absent" must
   be distinguishable from `0`; plain `int64` is used where zero is a
   legitimate, indistinguishable-from-absent value (e.g. `OrdersCount`).
   Don't reflexively make everything a pointer — that's the `any` of Go
   nil-safety, and it reintroduces the exact null-checking tax Go's zero
   values are supposed to remove.
3. **`class` → `struct` + free functions/methods, not `struct` +
   "everything becomes a method."** Node's `OrderService` methods that
   don't touch `this` are usually cleaner as Go package-level functions
   (see `order.service.ts`'s own `buildOrderPlacedPayload` — already a
   module-level helper, not a class method, for exactly this reason. This
   codebase follows the same instinct: `dto.RestaurantDayResponseFrom` is a
   function, not a method on some DTO builder struct).
4. **Enums:** TS `enum`/string-literal unions → Go `const` blocks with a
   named string type, or plain untyped `const` strings for something as
   simple as a single permission constant (`PermAnalyticsRead`). Don't
   reach for `iota` for anything that crosses the wire (JSON/Mongo) as a
   string — `iota` ints have no stable external representation.
5. **`interface X {}` vs Go `interface`:** a TS `interface` is almost
   always a **data shape** (translate to a Go `struct`). A Go `interface`
   is a **behavior contract** (a set of methods) — translate a TS
   interface to a Go `interface` only when the TS code actually uses it
   polymorphically (multiple implementations swapped at runtime), which is
   rare in this codebase's DTOs/entities and common in its
   `IMessageBroker`/`ICacheProvider`-style abstractions.
6. **Zero values are not `null`.** A Go `RestaurantDay{}` has
   `OrdersCount: 0`, not "missing." When porting a Node check like
   `if (!order.deliveryAgentId)`, ask whether the Go field should be a
   pointer (to represent "truly absent") before reflexively comparing
   against `0`.

## When to break the mapping

Not everything should be ported 1:1. Break from the Node pattern when:

- **The Node pattern exists to work around a Node/TS limitation Go doesn't
  have** (DI containers for compile-time-unverifiable graphs; arrow
  functions to preserve `this`; `class-validator` decorators because TS has
  no native struct-tag equivalent). Porting the *workaround* instead of the
  *underlying need* produces worse Go, not idiomatic Go.
- **A locked dependency doesn't have a direct Go equivalent**
  (`amqp-connection-manager`, `tsyringe`) — design the Go-native solution
  to the same problem (explicit wiring, an explicit reconnect loop) rather
  than hand-rolling a port of the library itself.
- **Go idiom actively prefers a different shape for the same guarantee**
  (narrow consumer-defined interfaces over concrete repo types; multiple
  return values over throwing; explicit `context.Context` propagation over
  an ambient request object).
- **The "why" documented in this service's own `CLAUDE.md`/`system-design.md`
  points somewhere else** — e.g. Mongo dedupe instead of Redis `SETNX`
  isn't a Node-vs-Go translation choice at all, it's a different tradeoff
  for a different kind of handler (idempotent cache invalidation vs.
  money-bearing aggregation). Don't "fix" it back to Redis just because
  that's what `order-service` does; re-read *why* first.

When none of the above apply — when the Node pattern exists because it's
just *correct*, not because of a language limitation — port it as
faithfully as this doc does. Most of this service is exactly that.
