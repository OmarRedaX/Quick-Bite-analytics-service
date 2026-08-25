package auth

import (
	"net/http"
	"strings"

	"analytics-service/lib/appcontext"
	apperror "analytics-service/lib/errors"
	response "analytics-service/lib/http"
	"analytics-service/lib/logger"
)

// ErrUnauthenticated is the stable, module-level AppError for "no/invalid
// token" — matches order-service's NotAuthenticated instance in
// lib/auth/errors.ts (§Error handling: don't construct ad-hoc AppErrors for
// cases that have a stable name).
var ErrUnauthenticated = apperror.New("UNAUTHENTICATED", http.StatusUnauthorized, "User not authenticated")

const accessCookieName = "access_token"

// Authenticate takes the access secret as an explicit parameter (constructor
// wiring, per the "no DI framework" rule) rather than importing lib/config
// directly, so it stays unit-testable without an env in scope.
func Authenticate(accessSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				response.SendError(w, logger.FromContext(r.Context()), ErrUnauthenticated)
				return
			}

			claims, err := VerifyAccessToken(token, accessSecret)
			if err != nil {
				response.SendError(w, logger.FromContext(r.Context()), ErrUnauthenticated)
				return
			}

			ctx := appcontext.WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractToken reads the access token from the cookie first, falling back
// to `Authorization: Bearer <token>` — same precedence as order-service's
// guard.ts (cookie) plus the Bearer fallback this service adds for
// service-to-service/test callers that can't set cookies.
func extractToken(r *http.Request) string {
	if c, err := r.Cookie(accessCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}
