package coreclient

// Envelope mirrors core-service's response shape ({"success", "data"}) —
// same as order-service's CoreEnvelope<T> in lib/core-client/types.ts,
// expressed with a Go generic instead of a TS generic.
type Envelope[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

// RolePermissionsResponse mirrors core's GET /api/internal/rbac/permissions
// response body.
type RolePermissionsResponse struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}
