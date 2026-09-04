package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Colocated unit test against httptest.Server — real HTTP round trips, no
// mocking of net/http itself. Mirrors order-service's CoreClient test
// (same retry-on-5xx-only, never-on-4xx contract) — see
// testing-implementation-plan.md Phase 2.

func TestClient_Do_RetriesOn5xxThenSucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := New(Config{MaxRetries: 3, BaseDelay: time.Millisecond})
	var out struct {
		OK bool `json:"ok"`
	}
	err := client.Do(context.Background(), Request{Method: "GET", URL: srv.URL}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.OK {
		t.Fatal("expected decoded response body")
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected 3 attempts (2 failures + 1 success), got %d", attempts)
	}
}

func TestClient_Do_ExhaustsRetriesOn5xx(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := New(Config{MaxRetries: 2, BaseDelay: time.Millisecond})
	err := client.Do(context.Background(), Request{Method: "GET", URL: srv.URL}, nil)
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("expected maxRetries+1 = 3 attempts, got %d", attempts)
	}
	var httpErr *HTTPError
	if !asHTTPError(err, &httpErr) || httpErr.StatusCode != 500 {
		t.Fatalf("expected an *HTTPError with status 500, got %v", err)
	}
}

func TestClient_Do_NeverRetriesOn4xx(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	client := New(Config{MaxRetries: 3, BaseDelay: time.Millisecond})
	err := client.Do(context.Background(), Request{Method: "GET", URL: srv.URL}, nil)
	if err == nil {
		t.Fatal("expected an error for a 400 response")
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Fatalf("expected exactly 1 attempt (4xx is never retried), got %d", attempts)
	}
}

func TestClient_Do_TimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := New(Config{Timeout: 5 * time.Millisecond})
	err := client.Do(context.Background(), Request{Method: "GET", URL: srv.URL}, nil)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
}

func TestClient_Do_SendsHeadersAndJSONBody(t *testing.T) {
	var gotAPIKey string
	var gotBody struct {
		Name string `json:"name"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("api-key")
		_ = decodeJSON(r, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := New(Config{})
	err := client.Do(context.Background(), Request{
		Method:  "POST",
		URL:     srv.URL,
		Headers: map[string]string{"api-key": "secret-123"},
		Body:    map[string]string{"name": "quickbite"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAPIKey != "secret-123" {
		t.Fatalf("expected api-key header forwarded, got %q", gotAPIKey)
	}
	if gotBody.Name != "quickbite" {
		t.Fatalf("expected JSON body forwarded, got %+v", gotBody)
	}
}

// asHTTPError is a tiny errors.As shim kept local to this test file so it
// doesn't need to import stdlib errors just for one assertion.
func asHTTPError(err error, target **HTTPError) bool {
	he, ok := err.(*HTTPError)
	if !ok {
		return false
	}
	*target = he
	return true
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(out)
}
