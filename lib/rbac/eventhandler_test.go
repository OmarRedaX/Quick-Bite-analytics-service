package rbac

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// Colocated unit test for HandlePermissionsChanged — same package so it can
// build a real *Cache with a fake fetcher (no HTTP double needed, same
// pattern as cache_test.go) and drive the returned coreevents.EventHandler
// directly against raw JSON payloads.

func TestHandlePermissionsChanged_SpecificRole_InvalidatesOnlyThatRole(t *testing.T) {
	calls := map[string]int{}
	fetch := func(_ context.Context, role string) ([]string, error) {
		calls[role]++
		return []string{"analytics:read"}, nil
	}
	cache := NewCache(time.Hour, fetch)
	ctx := context.Background()

	_, _ = cache.GetPermissions(ctx, "owner")
	_, _ = cache.GetPermissions(ctx, "staff")

	handler := cache.HandlePermissionsChanged(slog.Default())
	if err := handler(ctx, []byte(`{"role":"owner"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = cache.GetPermissions(ctx, "owner") // should refetch
	_, _ = cache.GetPermissions(ctx, "staff") // should stay cached

	if calls["owner"] != 2 {
		t.Fatalf("expected owner refetched after targeted invalidate, got %d calls", calls["owner"])
	}
	if calls["staff"] != 1 {
		t.Fatalf("expected staff to remain cached (untouched by a single-role invalidate), got %d calls", calls["staff"])
	}
}

func TestHandlePermissionsChanged_EmptyRole_InvalidatesEverything(t *testing.T) {
	calls := map[string]int{}
	fetch := func(_ context.Context, role string) ([]string, error) {
		calls[role]++
		return []string{"analytics:read"}, nil
	}
	cache := NewCache(time.Hour, fetch)
	ctx := context.Background()

	_, _ = cache.GetPermissions(ctx, "owner")
	_, _ = cache.GetPermissions(ctx, "staff")

	handler := cache.HandlePermissionsChanged(slog.Default())
	if err := handler(ctx, []byte(`{"role":""}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, _ = cache.GetPermissions(ctx, "owner")
	_, _ = cache.GetPermissions(ctx, "staff")

	if calls["owner"] != 2 || calls["staff"] != 2 {
		t.Fatalf("expected every role refetched after an empty-role (invalidate-all) payload, got %v", calls)
	}
}

func TestHandlePermissionsChanged_MalformedPayload_ReturnsErrorAndDoesNotInvalidate(t *testing.T) {
	calls := 0
	fetch := func(_ context.Context, _ string) ([]string, error) {
		calls++
		return []string{"analytics:read"}, nil
	}
	cache := NewCache(time.Hour, fetch)
	ctx := context.Background()

	_, _ = cache.GetPermissions(ctx, "owner")

	handler := cache.HandlePermissionsChanged(slog.Default())
	if err := handler(ctx, []byte("not json")); err == nil {
		t.Fatal("expected an error for a malformed payload")
	}

	_, _ = cache.GetPermissions(ctx, "owner")
	if calls != 1 {
		t.Fatalf("expected owner to remain cached since the malformed payload never invalidated it, got %d calls", calls)
	}
}
