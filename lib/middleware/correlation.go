// Package middleware holds cross-cutting HTTP middleware: correlation id
// propagation and access logging. Go analogue of order-service's
// lib/correlation/correlationId.ts.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"analytics-service/lib/appcontext"
	"analytics-service/lib/logger"
)

const correlationHeader = "X-CorrelationId"

// Correlation reads X-CorrelationId (or generates one), stashes it on the
// context, echoes it on the response, and binds a request-scoped logger
// (with correlation_id pre-attached) into the context so every downstream
// log line carries it without repeating the field by hand.
func Correlation(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(correlationHeader)
			if id == "" {
				id = uuid.NewString()
			}
			w.Header().Set(correlationHeader, id)

			ctx := appcontext.WithCorrelationID(r.Context(), id)
			ctx = logger.WithContext(ctx, base.With("correlation_id", id))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// AccessLog logs one structured line per request: method, path, status,
// duration. Uses the request-scoped logger from context so it inherits
// correlation_id automatically.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		logger.FromContext(r.Context()).Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
