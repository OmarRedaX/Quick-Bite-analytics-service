# API contracts

Base path: `/api/v1/analytics`. All responses use the envelope described
below. All timestamps are ISO-8601 UTC; all money is integer minor units.

## Envelope

```jsonc
// every 2xx
{ "success": true, "data": <endpoint-specific>, "meta"?: <pagination, if any> }

// every non-2xx
{ "success": false, "error": { "code": "STABLE_CODE", "message": "human-readable" } }
```

## Error codes (every one this service can return today)

| Code | HTTP | Meaning |
| --- | --- | --- |
| `UNAUTHENTICATED` | 401 | missing token, invalid signature, expired token, or invalid internal api-key on a (future) internal route |
| `FORBIDDEN` | 403 | authenticated, but caller's role/permissions don't include what the route requires |
| `VALIDATION_ERROR` | 400 | request failed struct-tag validation (missing/malformed field) |
| `ANALYTICS_INVALID_DATE_RANGE` | 400 | `from` is after `to` |
| `RBAC_UNAVAILABLE` | 503 | core-service unreachable and the permission cache has no entry for this role |
| `INTERNAL_ERROR` | 500 | unexpected failure; message is always the generic "Something went wrong" — internals never leak |

Every endpoint below shares the same auth (`access_token` cookie or
`Authorization: Bearer`) and RBAC (`analytics:read`, `system_admin`
bypasses) as `GET /restaurants/:id/days`, and the same `from`/`to`
`VALIDATION_ERROR`/`ANALYTICS_INVALID_DATE_RANGE` behavior — only the
path/response shape differs below.

---

## `GET /health`

No auth.

**200**
```json
{ "success": true, "data": { "status": "ok" } }
```

---

## `GET /api/v1/analytics/restaurants/{restaurantId}/days`

Day-grained order/revenue rollup for one restaurant over a date range.

**Auth:** required. `access_token` cookie, or `Authorization: Bearer <token>`.

**RBAC:** caller needs `analytics:read`. `system_admin` bypasses; a
`restaurant_user` is checked against the permissions for their
`restaurantRole`.

### Path params

| Param | Type | Notes |
| --- | --- | --- |
| `restaurantId` | positive integer | any other value → `VALIDATION_ERROR` |

### Query params

| Param | Type | Required | Notes |
| --- | --- | --- | --- |
| `from` | `YYYY-MM-DD` | yes | inclusive, UTC |
| `to` | `YYYY-MM-DD` | yes | inclusive, UTC; must be `>= from` |

### 200 response

```jsonc
{
  "success": true,
  "data": [
    {
      "date": "2026-08-25",
      "ordersCount": 2,
      "revenueMinor": 4000,
      "currency": "EGP",
      "avgOrderMinor": 2000
    }
  ]
}
```

- `avgOrderMinor` = `revenueMinor / ordersCount`, integer division,
  computed in the service layer — never stored.
- A restaurant with no rows in range returns `"data": []`, not an error.
- Rows are sorted by `date` ascending.
- `currency` reflects the currency on the most recently applied event for
  that day (this service does not attempt multi-currency aggregation
  within a single day — every order for a restaurant is expected to be in
  that restaurant's one operating currency).

### Error responses

| Condition | Status | Code |
| --- | --- | --- |
| no/invalid/expired token | 401 | `UNAUTHENTICATED` |
| valid token, missing `analytics:read` | 403 | `FORBIDDEN` |
| `restaurantId` not a positive integer | 400 | `VALIDATION_ERROR` |
| `from` or `to` missing or not `YYYY-MM-DD` | 400 | `VALIDATION_ERROR` |
| `from` after `to` | 400 | `ANALYTICS_INVALID_DATE_RANGE` |
| core-service unreachable, cache cold | 503 | `RBAC_UNAVAILABLE` |

### Example

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:4001/api/v1/analytics/restaurants/42/days?from=2026-01-01&to=2026-12-31"
```

---

## `GET /api/v1/analytics/restaurants/{restaurantId}/failures`

Day-grained order/failure counts for one restaurant, `failedCount`
populated by the `order.rejected` handler (bucketed by the rejection's own
date, not the order's original placed date — see
`analytics.OnOrderRejectedInput`'s doc comment).

**Query params:** `from`, `to` — same as `.../days`.

**200 response:**

```jsonc
{
  "success": true,
  "data": [
    { "date": "2026-08-31", "ordersCount": 1, "failedCount": 1, "failureRate": 1 }
  ]
}
```

`failureRate` = `failedCount / ordersCount` (0 if `ordersCount` is 0),
a fraction, computed in the service layer — never stored.

## `GET /api/v1/analytics/restaurants/{restaurantId}/delivery-avg`

Day-grained delivery-duration rollup for one restaurant, derived from
`delivery_ms_sum`/`delivery_ms_count` on `agg_restaurant_day`, populated by
the `order.delivered` handler.

**Query params:** `from`, `to` — same as `.../days`.

**200 response:**

```jsonc
{
  "success": true,
  "data": [
    { "date": "2026-08-31", "deliveredCount": 1, "avgDeliveryMs": 12481 }
  ]
}
```

`avgDeliveryMs` = `delivery_ms_sum / delivery_ms_count`, integer division,
computed in the service layer — never stored. Bucketed onto the date the
order was **placed** (looked up via `order_context`), not the date it was
delivered.

## `GET /api/v1/analytics/restaurants/active`

Count of distinct restaurants with `orders_count > 0` in `[from, to]` — an
aggregation pipeline over `agg_restaurant_day` (`$match` + `$group` by
`restaurant_id` + `$count`), not a stored field.

**Query params:** `from`, `to`.

**200 response:**

```jsonc
{ "success": true, "data": { "count": 1 } }
```

## `GET /api/v1/analytics/branches/{branchId}/days`

Day-grained order/revenue rollup for one branch, same shape as restaurant
days, backed by `agg_branch_day`.

**Query params:** `from`, `to`.

**200 response:**

```jsonc
{
  "success": true,
  "data": [
    { "date": "2026-08-31", "ordersCount": 1, "revenueMinor": 4000, "currency": "EGP", "avgOrderMinor": 4000 }
  ]
}
```

## `GET /api/v1/analytics/branches/{branchId}/products/{productId}/days`

Day-grained per-product rollup, backed by `agg_product_day`. Populated by
`order.placed`'s line items in one `BulkWrite` per order (N items → N
`UpdateOne` models, not N round trips).

**Query params:** `from`, `to`.

**200 response:**

```jsonc
{
  "success": true,
  "data": [
    { "date": "2026-08-31", "quantitySum": 2, "revenueMinor": 3000, "currency": "EGP", "avgUnitPriceMinor": 1500 }
  ]
}
```

`avgUnitPriceMinor` = `revenueMinor / quantitySum` — derived, never stored.

## `GET /api/v1/analytics/platform/days`

Day-grained platform-wide rollup, backed by `agg_platform_day`. A single
`date` can produce **more than one row** — the collection is keyed by
`(date, currency)` so two currencies active the same day are never summed
together.

**Query params:** `from`, `to`.

**200 response:**

```jsonc
{
  "success": true,
  "data": [
    { "date": "2026-08-31", "currency": "EGP", "ordersCount": 1, "revenueMinor": 4000, "avgOrderMinor": 4000 }
  ]
}
```

## `GET /api/v1/analytics/platform/summary`

Totals across `[from, to]`, one row per currency — a single `$group`
aggregation over `agg_platform_day`, no new storage.

**Query params:** `from`, `to`.

**200 response:**

```jsonc
{
  "success": true,
  "data": [
    {
      "currency": "EGP",
      "ordersCount": 1,
      "revenueMinor": 4000,
      "avgOrderMinor": 4000,
      "failedCount": 1,
      "failureRate": 1,
      "deliveredCount": 0,
      "avgDeliveryMs": 0,
      "onlinePaymentsCount": 0,
      "onlinePaymentsAmountMinor": 0
    }
  ]
}
```

`onlinePaymentsCount`/`onlinePaymentsAmountMinor` come from
`payment.completed` and track online-gateway capture volume for
reconciliation — a distinct KPI from `revenueMinor`, which already counts
every order (COD or online) once via `order.placed`.
