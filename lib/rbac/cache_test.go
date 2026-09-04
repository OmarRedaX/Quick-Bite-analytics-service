package rbac

import (
	"context"
	"testing"
	"time"
)

// Colocated unit test — Go's mandatory convention (same package, same
// directory) so it can construct *Cache directly. No Mongo/HTTP/RabbitMQ
// needed: PermissionsFetcher is a plain injected function. See
// testing-implementation-plan.md Phase 0 for why unit tests can't live in
// a tests/unit folder the way the Node services' can.
func TestCache_GetPermissions_CachesWithinTTL(t *testing.T) {
	calls := 0
	fetch := func(_ context.Context, role string) ([]string, error) {
		calls++
		return []string{"analytics:read"}, nil
	}
	cache := NewCache(time.Hour, fetch)

	first, err := cache.GetPermissions(context.Background(), "branch_manager")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := cache.GetPermissions(context.Background(), "branch_manager")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected fetch called once (second call served from cache), got %d calls", calls)
	}
	if !HasPermission(first, "analytics:read") || !HasPermission(second, "analytics:read") {
		t.Fatalf("expected both results to contain analytics:read, got %v and %v", first, second)
	}
}

func TestCache_GetPermissions_RefetchesAfterTTLExpiry(t *testing.T) {
	calls := 0
	fetch := func(_ context.Context, role string) ([]string, error) {
		calls++
		return []string{"analytics:read"}, nil
	}
	cache := NewCache(0, fetch) // ttl=0 -> every call is stale immediately

	_, _ = cache.GetPermissions(context.Background(), "branch_manager")
	_, _ = cache.GetPermissions(context.Background(), "branch_manager")

	if calls != 2 {
		t.Fatalf("expected fetch called on every request when ttl is 0, got %d calls", calls)
	}
}

func TestCache_Invalidate_SingleRoleVsAll(t *testing.T) {
	calls := map[string]int{}
	fetch := func(_ context.Context, role string) ([]string, error) {
		calls[role]++
		return []string{"analytics:read"}, nil
	}
	cache := NewCache(time.Hour, fetch)

	_, _ = cache.GetPermissions(context.Background(), "owner")
	_, _ = cache.GetPermissions(context.Background(), "staff")

	cache.Invalidate("owner")
	_, _ = cache.GetPermissions(context.Background(), "owner") // refetches
	_, _ = cache.GetPermissions(context.Background(), "staff") // still cached

	if calls["owner"] != 2 {
		t.Fatalf("expected owner refetched after single-role invalidate, got %d calls", calls["owner"])
	}
	if calls["staff"] != 1 {
		t.Fatalf("expected staff to stay cached after invalidating only owner, got %d calls", calls["staff"])
	}

	cache.Invalidate("")
	_, _ = cache.GetPermissions(context.Background(), "staff") // refetches after full invalidate

	if calls["staff"] != 2 {
		t.Fatalf("expected staff refetched after invalidate-all, got %d calls", calls["staff"])
	}
}

func TestHasPermission(t *testing.T) {
	perms := []string{"analytics:read", "order:read"}

	if !HasPermission(perms, "analytics:read") {
		t.Fatal("expected analytics:read to be found")
	}
	if HasPermission(perms, "analytics:write") {
		t.Fatal("expected analytics:write to be absent")
	}
	if HasPermission(nil, "analytics:read") {
		t.Fatal("expected no match against a nil permission list")
	}
}
