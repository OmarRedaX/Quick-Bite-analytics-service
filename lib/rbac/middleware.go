package rbac

import (
	"net/http"

	"analytics-service/lib/appcontext"
	"analytics-service/lib/auth"
	apperror "analytics-service/lib/errors"
	response "analytics-service/lib/http"
	"analytics-service/lib/logger"
)

// ErrForbidden mirrors order-service's rbac() middleware 403 response.
var ErrForbidden = apperror.New("FORBIDDEN", http.StatusForbidden, "Permission denied")

var errRBACUnavailable = apperror.New("RBAC_UNAVAILABLE", http.StatusServiceUnavailable, "permission check unavailable")

const (
	systemAdminRole    = "system_admin"
	restaurantUserRole = "restaurant_user"
)

// Require gates a route behind a single permission string (e.g.
// "analytics:read"). Same shape as order-service's rbac({resource, action}):
// system_admin always passes; a restaurant_user is checked against the
// permissions for their restaurantRole (owner/branch_manager/staff), fetched
// through cache; anyone else is forbidden.
func Require(cache *Cache, permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log := logger.FromContext(r.Context())

			claims, ok := appcontext.ClaimsFromContext(r.Context())
			if !ok {
				response.SendError(w, log, auth.ErrUnauthenticated)
				return
			}
			if claims.Role == systemAdminRole {
				next.ServeHTTP(w, r)
				return
			}
			if claims.Role != restaurantUserRole {
				response.SendError(w, log, ErrForbidden)
				return
			}

			perms, err := cache.GetPermissions(r.Context(), claims.RestaurantRole)
			if err != nil {
				log.Warn("rbac: permission fetch failed", "error", err.Error())
				response.SendError(w, log, errRBACUnavailable)
				return
			}
			if !HasPermission(perms, permission) {
				response.SendError(w, log, ErrForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
