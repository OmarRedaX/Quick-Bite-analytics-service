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

## Homework endpoints (not built — documented for the shape they'd take)

All of the following follow the same envelope, auth, and RBAC pattern
above. None exist in code yet — see `plan.md` and
`docs/implementation-plan.md` Phase 7+.

| Endpoint | Sketch |
| --- | --- |
| `GET /restaurants/{id}/failures?from=&to=` | derived from a `failed_count`-style field added to `agg_restaurant_day` by the homework `order.rejected` handler — no new collection |
| `GET /restaurants/{id}/delivery-avg?from=&to=` | derived from `delivery_ms_sum`/`delivery_ms_count`, already reserved on the `agg_restaurant_day` document today, populated by the homework `order.delivered` handler |
| `GET /branches/{id}/days?from=&to=` | same shape as restaurant days, backed by the homework `agg_branch_day` |
| `GET /branches/{id}/products/{productId}/days?from=&to=` | backed by the homework `agg_product_day` |
| `GET /restaurants/active?from=&to=` | count of distinct restaurants with `orders_count > 0` in range — an aggregation pipeline over `agg_restaurant_day`, not a stored field |
| `GET /platform/days?from=&to=` | backed by the homework `agg_platform_day` |
| `GET /platform/summary?from=&to=` | totals across the range — a `$group` aggregation over `agg_platform_day`, no new storage |
