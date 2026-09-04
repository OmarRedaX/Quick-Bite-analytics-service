package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"analytics-service/lib/appcontext"
	"analytics-service/lib/logger"
)

func TestCorrelation_ExistingHeader_IsPropagatedNotRegenerated(t *testing.T) {
	var gotID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = appcontext.CorrelationIDFromContext(r.Context())
	})
	handler := Correlation(slog.Default())(next)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-CorrelationId", "caller-supplied-id")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotID != "caller-supplied-id" {
		t.Fatalf("expected the caller-supplied id propagated into context, got %q", gotID)
	}
	if got := rec.Header().Get("X-CorrelationId"); got != "caller-supplied-id" {
		t.Fatalf("expected the same id echoed on the response, got %q", got)
	}
}

func TestCorrelation_NoHeader_GeneratesOne(t *testing.T) {
	var gotID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = appcontext.CorrelationIDFromContext(r.Context())
	})
	handler := Correlation(slog.Default())(next)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotID == "" {
		t.Fatal("expected a generated correlation id when the request had none")
	}
	if rec.Header().Get("X-CorrelationId") != gotID {
		t.Fatalf("expected the generated id echoed on the response, got header=%q context=%q", rec.Header().Get("X-CorrelationId"), gotID)
	}
}

func TestCorrelation_BindsRequestScopedLoggerIntoContext(t *testing.T) {
	var boundLogger *slog.Logger
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		boundLogger = logger.FromContext(r.Context())
	})
	handler := Correlation(slog.Default())(next)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if boundLogger == nil {
		t.Fatal("expected a non-nil request-scoped logger bound into context")
	}
}

func TestAccessLog_CapturesNonDefaultStatusCode(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	handler := AccessLog(next)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected the underlying handler's status to pass through untouched, got %d", rec.Code)
	}
}
