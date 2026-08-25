// Package appcontext holds request-scoped values carried on
// context.Context: the authenticated caller's claims and the correlation
// id. This is the Go analogue of order-service's src/lib/types/express.d.ts
// — the single place that declares "what's attached to a request" — except
// Go has no ambient req object, so the values ride the context instead.
//
// Claims lives here (not in lib/auth) so lib/auth can depend on
// appcontext (to stash claims after verifying a token) without a import
// cycle: appcontext never imports lib/auth.
package appcontext

import "context"

// Claims is the authenticated caller, decoded from the access token. Field
// names mirror the JWT payload shape shared with core-service/order-service.
type Claims struct {
	UserID         int64
	Role           string
	Email          string
	RestaurantID   *int64
	RestaurantRole string
	BranchIDs      []int64
}

type ctxKey int

const (
	claimsKey ctxKey = iota
	correlationIDKey
)

func WithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// ClaimsFromContext returns the caller's claims and whether they were set
// (false for unauthenticated requests, e.g. GET /health).
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsKey).(Claims)
	return c, ok
}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

// CorrelationIDFromContext returns "" if none was set.
func CorrelationIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(correlationIDKey).(string)
	return id
}
