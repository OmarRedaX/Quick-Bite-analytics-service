package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"analytics-service/lib/coreevents"
)

// permissionsChangedPayload is the only field this handler needs off
// core-service's rbac.permissions_changed event — an empty/missing role
// means "invalidate everything" (mirrors Cache.Invalidate's own "" case).
type permissionsChangedPayload struct {
	Role string `json:"role"`
}

// HandlePermissionsChanged builds the coreevents.EventHandler for
// rbac.permissions_changed — Go analogue of order-service's
// PermissionCacheService.handlePermissionsChanged
// (lib/rbac/permission-cache.service.ts). Registered on a second consumer
// bound to core.events (see lib/boot/boot.go), not the order.events
// consumer every other handler in this service uses.
func (c *Cache) HandlePermissionsChanged(log *slog.Logger) coreevents.EventHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p permissionsChangedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("rbac.permissions_changed: unmarshal payload: %w", err)
		}

		c.Invalidate(p.Role)

		role := p.Role
		if role == "" {
			role = "*"
		}
		log.Info("rbac.permissions_changed -> permission cache invalidated", "role", role)
		return nil
	}
}
