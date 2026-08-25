// Package rbac is an in-process, TTL-based cache of role -> permissions,
// read-through to core-service. Go analogue of order-service's
// PermissionCacheService (lib/rbac/permission-cache.service.ts) — same
// Map+TTL shape, except the fetch function is injected at construction
// instead of imported, so the cache has zero knowledge of HTTP or
// core-service (that lives in lib/coreclient, wired together in lib/boot).
package rbac

import (
	"context"
	"sync"
	"time"
)

// PermissionsFetcher loads permissions for a role from the source of truth
// (core-service, via lib/coreclient.GetPermissionsByRole).
type PermissionsFetcher func(ctx context.Context, role string) ([]string, error)

type entry struct {
	permissions []string
	cachedAt    time.Time
}

type Cache struct {
	mu      sync.RWMutex
	entries map[string]entry
	ttl     time.Duration
	fetch   PermissionsFetcher
}

func NewCache(ttl time.Duration, fetch PermissionsFetcher) *Cache {
	return &Cache{entries: make(map[string]entry), ttl: ttl, fetch: fetch}
}

// GetPermissions returns the cached permission list for role if fresh,
// otherwise fetches, caches, and returns it.
func (c *Cache) GetPermissions(ctx context.Context, role string) ([]string, error) {
	c.mu.RLock()
	e, ok := c.entries[role]
	c.mu.RUnlock()
	if ok && time.Since(e.cachedAt) < c.ttl {
		return e.permissions, nil
	}

	perms, err := c.fetch(ctx, role)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[role] = entry{permissions: perms, cachedAt: time.Now()}
	c.mu.Unlock()
	return perms, nil
}

// Invalidate drops one role's cache entry, or all entries if role == "".
// Homework hook: wire this to a `rbac.permissions_changed` consumer handler
// (see docs/implementation-plan.md Phase 8) — not called anywhere in this
// slice.
func (c *Cache) Invalidate(role string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if role == "" {
		c.entries = make(map[string]entry)
		return
	}
	delete(c.entries, role)
}

func HasPermission(permissions []string, permission string) bool {
	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}
