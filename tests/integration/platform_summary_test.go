//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"analytics-service/tests/integration/testutil"
)

// Smoke test proving the scaffold actually works end-to-end: real Mongo
// connection + index creation, real router assembly (repository -> service
// -> controller -> chi routes), real JWT verification, real RBAC
// short-circuit for system_admin. No data is seeded, so an empty range is
// the correct, meaningful assertion here — this is Phase 0's "baseline
// run" proving the harness, not a Phase 3 feature test.
func TestPlatformSummary_SystemAdminEmptyRange(t *testing.T) {
	db := testutil.ConnectMongo(t)
	router := testutil.NewRouter(db, testutil.TestAccessSecret, testutil.FixedPermissionsCache())

	token := testutil.MintAccessToken(t, 1, "admin@quickbite.test", "system_admin")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/platform/summary?from=2026-01-01&to=2026-01-02", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Success bool  `json:"success"`
		Data    []any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v, body: %s", err, rec.Body.String())
	}
	if !body.Success {
		t.Fatalf("expected success:true, got body: %s", rec.Body.String())
	}
	if len(body.Data) != 0 {
		t.Fatalf("expected empty data (no seeded rows), got %d rows", len(body.Data))
	}
}

func TestPlatformSummary_Unauthenticated(t *testing.T) {
	db := testutil.ConnectMongo(t)
	router := testutil.NewRouter(db, testutil.TestAccessSecret, testutil.FixedPermissionsCache())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/platform/summary?from=2026-01-01&to=2026-01-02", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body: %s", rec.Code, rec.Body.String())
	}
}
