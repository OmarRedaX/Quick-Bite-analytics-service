package apperror

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Colocated unit test for Wrap — the middleware every controller method in
// app/analytics/controller is registered through. logger.FromContext falls
// back to slog.Default() when no request-scoped logger is attached, so
// httptest.NewRequest's plain background context is fine here.

func TestWrap_AppErrorRendersItsOwnCodeAndStatus(t *testing.T) {
	appErr := New("NOT_FOUND", http.StatusNotFound, "restaurant not found")
	handler := Wrap(func(w http.ResponseWriter, r *http.Request) error {
		return appErr
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v, body: %s", err, rec.Body.String())
	}
	if body.Success {
		t.Fatal("expected success:false")
	}
	if body.Error.Code != "NOT_FOUND" || body.Error.Message != "restaurant not found" {
		t.Fatalf("expected the AppError's own code/message rendered, got %+v", body.Error)
	}
}

func TestWrap_GenericErrorRendersGeneric500(t *testing.T) {
	handler := Wrap(func(w http.ResponseWriter, r *http.Request) error {
		return errors.New("some internal detail that must not leak")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v, body: %s", err, rec.Body.String())
	}
	if body.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected generic code INTERNAL_ERROR, got %s", body.Error.Code)
	}
	if body.Error.Message == "some internal detail that must not leak" {
		t.Fatal("expected the internal error message NOT to leak to the client")
	}
}

func TestWrap_NilError_DoesNotWriteAResponse(t *testing.T) {
	handler := Wrap(func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected the default 200 (no explicit WriteHeader call) when the handler succeeds, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected an empty body when the handler itself already wrote the success response, got %s", rec.Body.String())
	}
}
