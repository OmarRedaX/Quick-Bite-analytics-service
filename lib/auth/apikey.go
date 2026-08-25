package auth

import (
	"crypto/subtle"
	"net/http"

	apperror "analytics-service/lib/errors"
	response "analytics-service/lib/http"
	"analytics-service/lib/logger"
)

// ErrInvalidAPIKey mirrors order-service's requireInternalApiKey guard
// (lib/auth/api-key.ts) — shared-secret header for service-to-service
// calls. Not wired into any route in this slice (analytics-service exposes
// no internal endpoints yet), but scaffolded here for the homework backfill
// command / any future internal route.
var ErrInvalidAPIKey = apperror.New("UNAUTHENTICATED", http.StatusUnauthorized, "invalid api key")

// RequireInternalAPIKey guards a route with a shared-secret `api-key`
// header, constant-time compared against expected.
func RequireInternalAPIKey(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("api-key")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
				response.SendError(w, logger.FromContext(r.Context()), ErrInvalidAPIKey)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
