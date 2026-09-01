package controller

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"analytics-service/app/analytics"
	"analytics-service/lib/auth"
	apperror "analytics-service/lib/errors"
	"analytics-service/lib/rbac"
)

// Routes mounts this module's endpoints. Middleware chain mirrors
// order-service's routes.ts: authenticate -> rbac -> controller (wrapped by
// apperror.Wrap so a returned error renders the standard envelope).
func Routes(accessSecret string, permCache *rbac.Cache, ctrl *Controller) http.Handler {
	r := chi.NewRouter()
	r.Use(auth.Authenticate(accessSecret))
	r.Use(rbac.Require(permCache, analytics.PermAnalyticsRead))

	r.Get("/restaurants/{restaurantId}/days", apperror.Wrap(ctrl.GetRestaurantDays))
	r.Get("/restaurants/{restaurantId}/failures", apperror.Wrap(ctrl.GetRestaurantFailures))
	r.Get("/restaurants/{restaurantId}/delivery-avg", apperror.Wrap(ctrl.GetRestaurantDeliveryAvg))
	r.Get("/restaurants/active", apperror.Wrap(ctrl.GetActiveRestaurants))

	r.Get("/branches/{branchId}/days", apperror.Wrap(ctrl.GetBranchDays))
	r.Get("/branches/{branchId}/products/{productId}/days", apperror.Wrap(ctrl.GetProductDays))

	r.Get("/platform/days", apperror.Wrap(ctrl.GetPlatformDays))
	r.Get("/platform/summary", apperror.Wrap(ctrl.GetPlatformSummary))

	return r
}
