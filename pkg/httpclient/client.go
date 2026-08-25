// Package httpclient is a small net/http wrapper: fixed timeout, JSON
// encode/decode, and retry-on-5xx with backoff. It knows nothing about
// core-service, RBAC, or any app-specific endpoint — that lives in
// lib/coreclient.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a thin, reusable HTTP client with retry-on-5xx.
type Client struct {
	http       *http.Client
	maxRetries int
	baseDelay  time.Duration
}

// Config configures the underlying http.Client and retry behavior.
type Config struct {
	Timeout    time.Duration
	MaxRetries int           // number of retries after the first attempt; 0 = no retry
	BaseDelay  time.Duration // backoff base; doubles per attempt
}

func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	baseDelay := cfg.BaseDelay
	if baseDelay <= 0 {
		baseDelay = 50 * time.Millisecond
	}
	return &Client{
		http:       &http.Client{Timeout: timeout},
		maxRetries: cfg.MaxRetries,
		baseDelay:  baseDelay,
	}
}

// Request is a single JSON-in/JSON-out HTTP call.
type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    any // marshaled to JSON if non-nil
}

// Do sends req, retrying on 5xx and transport errors with exponential
// backoff. On success (2xx) it decodes the JSON body into out (if out is
// non-nil). Non-2xx, non-retryable responses return an *HTTPError.
func (c *Client) Do(ctx context.Context, req Request, out any) error {
	var bodyBytes []byte
	if req.Body != nil {
		b, err := json.Marshal(req.Body)
		if err != nil {
			return fmt.Errorf("httpclient: marshal body: %w", err)
		}
		bodyBytes = b
	}

	attempts := c.maxRetries + 1
	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.baseDelay * time.Duration(1<<uint(attempt-1))):
			}
		}

		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
		if err != nil {
			return fmt.Errorf("httpclient: build request: %w", err)
		}
		if bodyBytes != nil {
			httpReq.Header.Set("Content-Type", "application/json")
		}
		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}

		resp, err := c.http.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("httpclient: %w", err)
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("httpclient: read body: %w", readErr)
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody)}
			continue
		}
		if resp.StatusCode >= 400 {
			return &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody)}
		}

		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("httpclient: decode response: %w", err)
			}
		}
		return nil
	}

	return lastErr
}

// HTTPError is returned for non-2xx responses that exhausted retries (5xx)
// or were non-retryable (4xx).
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("httpclient: status %d: %s", e.StatusCode, e.Body)
}
