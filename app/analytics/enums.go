package analytics

// PermAnalyticsRead is the permission string checked by rbac.Require on the
// read endpoints. Namespaced "analytics:*" per CLAUDE.md, resolved through
// core-service's permissions catalog — this service does not own it.
const PermAnalyticsRead = "analytics:read"
