package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// RequireInternalAPIKey is currently unwired from any route (see its doc
// comment: scaffolded for a future internal route / the homework backfill
// command) but is real, security-relevant logic — worth covering the same
// way jwt_test.go covers VerifyAccessToken, independent of whether anything
// calls it yet.

func TestRequireInternalAPIKey_CorrectKey_CallsNext(t *testing.T) {
	nextCalled := false
	handler := RequireInternalAPIKey("secret-123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/x", nil)
	req.Header.Set("api-key", "secret-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !nextCalled {
		t.Fatal("expected next handler called for a matching api-key")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequireInternalAPIKey_MissingKey_Returns401(t *testing.T) {
	nextCalled := false
	handler := RequireInternalAPIKey("secret-123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/x", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("expected next handler never called without an api-key header")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireInternalAPIKey_WrongKey_Returns401(t *testing.T) {
	nextCalled := false
	handler := RequireInternalAPIKey("secret-123")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/x", nil)
	req.Header.Set("api-key", "wrong-key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if nextCalled {
		t.Fatal("expected next handler never called for a mismatched api-key")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
