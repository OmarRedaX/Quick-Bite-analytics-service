package coreclient

import (
	"context"
	"fmt"
	"net/url"
)

// GetPermissionsByRole calls core's GET /api/internal/rbac/permissions.
// Signature matches rbac.PermissionsFetcher exactly, so lib/boot wires it in
// directly as `rbac.NewCache(ttl, coreClient.GetPermissionsByRole)` — no
// adapter needed.
func (c *Client) GetPermissionsByRole(ctx context.Context, role string) ([]string, error) {
	var env Envelope[RolePermissionsResponse]
	path := "/api/internal/rbac/permissions?role=" + url.QueryEscape(role)
	if err := c.get(ctx, path, &env); err != nil {
		return nil, fmt.Errorf("get permissions for role %q: %w", role, err)
	}
	return env.Data.Permissions, nil
}
