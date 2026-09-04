//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"analytics-service/app/analytics"
	"analytics-service/tests/integration/testutil"
)

// Cross-cutting auth/rbac coverage (Phase 3 item 1). GET
// /restaurants/active is the representative endpoint here — it needs no
// path param, so these tests isolate the auth.Authenticate + rbac.Require
// middleware chain from any endpoint-specific parsing. Every other
// endpoint shares the same middleware chain (controller/routes.go mounts
// it once for the whole module), so this suite is not repeated per
// endpoint.

func activeRestaurantsRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/restaurants/active?from=2026-01-01&to=2026-01-02", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestAuthRBAC_NoToken_Unauthenticated(t *testing.T) {
	db := testutil.ConnectMongo(t)
	router := testutil.NewRouter(db, testutil.TestAccessSecret, testutil.FixedPermissionsCache())

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, activeRestaurantsRequest(""))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthRBAC_GarbageToken_Unauthenticated(t *testing.T) {
	db := testutil.ConnectMongo(t)
	router := testutil.NewRouter(db, testutil.TestAccessSecret, testutil.FixedPermissionsCache())

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, activeRestaurantsRequest("not-a-real-jwt"))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthRBAC_UnrecognizedRole_Forbidden(t *testing.T) {
	db := testutil.ConnectMongo(t)
	router := testutil.NewRouter(db, testutil.TestAccessSecret, testutil.FixedPermissionsCache(analytics.PermAnalyticsRead))

	// "customer" is neither system_admin (bypass) nor restaurant_user
	// (permission-checked) — rbac.Require forbids everything else outright.
	token := testutil.MintAccessToken(t, 1, "customer@quickbite.test", "customer")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, activeRestaurantsRequest(token))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthRBAC_RestaurantUserMissingPermission_Forbidden(t *testing.T) {
	db := testutil.ConnectMongo(t)
	// cache has permissions, but not analytics:read.
	router := testutil.NewRouter(db, testutil.TestAccessSecret, testutil.FixedPermissionsCache("order:read"))

	token := testutil.MintAccessToken(t, 1, "staff@quickbite.test", "restaurant_user", testutil.WithRestaurantRole("staff"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, activeRestaurantsRequest(token))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthRBAC_SystemAdmin_BypassesPermissionCheck(t *testing.T) {
	db := testutil.ConnectMongo(t)
	// Empty cache — system_admin must never consult it (rbac.Require's own
	// short-circuit), so this proves the bypass rather than getting lucky.
	router := testutil.NewRouter(db, testutil.TestAccessSecret, testutil.FixedPermissionsCache())

	token := testutil.MintAccessToken(t, 1, "admin@quickbite.test", "system_admin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, activeRestaurantsRequest(token))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthRBAC_RestaurantUserWithPermission_Allowed(t *testing.T) {
	db := testutil.ConnectMongo(t)
	router := testutil.NewRouter(db, testutil.TestAccessSecret, testutil.FixedPermissionsCache(analytics.PermAnalyticsRead))

	token := testutil.MintAccessToken(t, 1, "owner@quickbite.test", "restaurant_user", testutil.WithRestaurantRole("owner"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, activeRestaurantsRequest(token))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}
}
