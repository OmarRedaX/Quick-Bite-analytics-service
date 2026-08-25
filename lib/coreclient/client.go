// Package coreclient is the sync HTTP client to core-service — the Go
// analogue of order-service's lib/core-client/core-client.ts. It knows the
// base URL, the internal api-key header, and core's specific endpoints
// (today: just RBAC permissions). Built on pkg/httpclient, which knows
// nothing about core-service at all.
package coreclient

import (
	"context"
	"fmt"

	"analytics-service/lib/appcontext"
	"analytics-service/pkg/httpclient"
)

type Client struct {
	http    *httpclient.Client
	baseURL string
	apiKey  string
}

func New(http *httpclient.Client, baseURL, apiKey string) *Client {
	return &Client{http: http, baseURL: baseURL, apiKey: apiKey}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req := httpclient.Request{
		Method:  "GET",
		URL:     c.baseURL + path,
		Headers: map[string]string{"api-key": c.apiKey},
	}
	if id := appcontext.CorrelationIDFromContext(ctx); id != "" {
		req.Headers["X-CorrelationId"] = id
	}
	if err := c.http.Do(ctx, req, out); err != nil {
		return fmt.Errorf("coreclient: %w", err)
	}
	return nil
}
