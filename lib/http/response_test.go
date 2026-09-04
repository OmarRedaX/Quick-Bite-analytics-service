package response

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"testing"
)

// Colocated unit test for the response envelope renderers, via
// httptest.NewRecorder — no real server needed.

type fakeCodedError struct {
	code       string
	statusCode int
	message    string
}

func (e *fakeCodedError) Error() string   { return e.message }
func (e *fakeCodedError) Code() string    { return e.code }
func (e *fakeCodedError) StatusCode() int { return e.statusCode }

func TestSendSuccess_RendersEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	SendSuccess(rec, 200, map[string]int{"count": 3})

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var body struct {
		Success bool           `json:"success"`
		Data    map[string]int `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Success || body.Data["count"] != 3 {
		t.Fatalf("expected success:true, data.count:3, got %+v", body)
	}
}

func TestSendPaginated_IncludesMeta(t *testing.T) {
	rec := httptest.NewRecorder()
	SendPaginated(rec, []int{1, 2}, map[string]bool{"hasMore": true})

	var body struct {
		Success bool            `json:"success"`
		Data    []int           `json:"data"`
		Meta    map[string]bool `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Success || len(body.Data) != 2 || !body.Meta["hasMore"] {
		t.Fatalf("expected success:true, data, and meta.hasMore:true, got %+v", body)
	}
}

func TestSendError_CodedErrorRendersOwnCodeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	SendError(rec, slog.Default(), &fakeCodedError{code: "FORBIDDEN", statusCode: 403, message: "nope"})

	if rec.Code != 403 {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Success || body.Error.Code != "FORBIDDEN" || body.Error.Message != "nope" {
		t.Fatalf("expected the CodedError's own code/message, got %+v", body)
	}
}

func TestSendError_GenericErrorRendersGeneric500WithoutLeaking(t *testing.T) {
	rec := httptest.NewRecorder()
	SendError(rec, slog.Default(), errors.New("db connection string: user:pass@host"))

	if rec.Code != 500 {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected code INTERNAL_ERROR, got %s", body.Error.Code)
	}
	if body.Error.Message != "Something went wrong" {
		t.Fatalf("expected the generic message, not the raw error, got %q", body.Error.Message)
	}
}
