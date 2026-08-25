// Package logger wraps stdlib log/slog with a context-carrying convention:
// boot stores one *slog.Logger (with correlation_id etc. attached per
// request) in context.Context, and every layer pulls it back out via
// FromContext instead of threading a logger parameter through every call.
// This is the Go analogue of order-service's singleton `logger` — except
// request-scoped fields (correlationId) are attached per-request instead of
// passed as a metadata object on every call site.
package logger

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey struct{}

// New builds the process-wide JSON logger. Structured JSON on stdout is
// what "boot logs structured JSON" in the acceptance checklist refers to.
func New(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}

// WithContext attaches a logger (typically one with request-scoped fields
// already bound via .With()) to ctx.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the request-scoped logger, or slog.Default() if none
// was attached (e.g. background code running outside a request/message).
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
