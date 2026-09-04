package coreclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"analytics-service/pkg/httpclient"
)

// Colocated unit test against httptest.Server — this is the one HTTP
// boundary test the plan calls out explicitly (Phase 2), since
// GetPermissionsByRole is where lib/coreclient's own request-shaping
// (path, api-key header) meets pkg/httpclient's generic transport.

func TestGetPermissionsByRole_200_ReturnsPermissions(t *testing.T) {
	var gotPath, gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotAPIKey = r.Header.Get("api-key")
		_ = json.NewEncoder(w).Encode(Envelope[RolePermissionsResponse]{
			Success: true,
			Data:    RolePermissionsResponse{Role: "owner", Permissions: []string{"analytics:read"}},
		})
	}))
	defer srv.Close()

	client := New(httpclient.New(httpclient.Config{}), srv.URL, "internal-key")
	perms, err := client.GetPermissionsByRole(context.Background(), "owner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(perms) != 1 || perms[0] != "analytics:read" {
		t.Fatalf("expected [analytics:read], got %v", perms)
	}
	if gotPath != "/api/internal/rbac/permissions?role=owner" {
		t.Fatalf("expected path /api/internal/rbac/permissions?role=owner, got %s", gotPath)
	}
	if gotAPIKey != "internal-key" {
		t.Fatalf("expected api-key header forwarded, got %q", gotAPIKey)
	}
}

func TestGetPermissionsByRole_4xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := New(httpclient.New(httpclient.Config{}), srv.URL, "wrong-key")
	if _, err := client.GetPermissionsByRole(context.Background(), "owner"); err == nil {
		t.Fatal("expected an error for a 401 response")
	}
}

func TestGetPermissionsByRole_5xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := New(httpclient.New(httpclient.Config{MaxRetries: 0}), srv.URL, "internal-key")
	if _, err := client.GetPermissionsByRole(context.Background(), "owner"); err == nil {
		t.Fatal("expected an error for a 500 response (after exhausting retries)")
	}
}

func TestGetPermissionsByRole_RoleIsQueryEscaped(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("role")
		_ = json.NewEncoder(w).Encode(Envelope[RolePermissionsResponse]{Success: true})
	}))
	defer srv.Close()

	client := New(httpclient.New(httpclient.Config{}), srv.URL, "key")
	if _, err := client.GetPermissionsByRole(context.Background(), "branch manager"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "branch manager" {
		t.Fatalf("expected the role query param round-tripped (net/url handles escaping), got %q", gotQuery)
	}
}
