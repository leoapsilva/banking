// Package client provides a minimal HTTP client for the InfinitePay Checkout
// API. InfinitePay uses no auth token — the handle (InfiniteTag) embedded in
// each request body is the only credential.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Client wraps a plain *http.Client for InfinitePay's checkout API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// New creates a client for the given InfinitePay base URL
// (default: "https://api.checkout.infinitepay.io").
func New(httpClient *http.Client, baseURL string) *Client {
	return &Client{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
	}
}

// APIError represents a non-2xx response from InfinitePay.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("infinitepay: unexpected status %d: %s", e.StatusCode, e.Body)
}

// Do performs a JSON request against path (relative to baseURL).
// reqBody may be nil; respBody may be nil when the caller ignores the response.
func (c *Client) Do(ctx context.Context, method, path string, reqBody, respBody any) error {
	var reqBodyBytes []byte
	var bodyReader io.Reader
	if reqBody != nil {
		var err error
		reqBodyBytes, err = json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("infinitepay/client: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(reqBodyBytes)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("infinitepay/client: build request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Error("infinitepay api call failed", "method", method, "url", url, "error", err)
		return fmt.Errorf("infinitepay/client: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)
	duration := time.Since(start)

	level := slog.LevelInfo
	switch {
	case resp.StatusCode >= 500:
		level = slog.LevelError
	case resp.StatusCode >= 400:
		level = slog.LevelWarn
	}
	slog.Log(ctx, level, "infinitepay api call",
		"method", method, "url", url, "status", resp.StatusCode, "duration_ms", duration.Milliseconds())
	slog.Debug("infinitepay api call body",
		"method", method, "url", url, "request_body", string(reqBodyBytes), "response_body", string(raw))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(raw)}
	}

	if respBody != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, respBody); err != nil {
			return fmt.Errorf("infinitepay/client: decode response: %w", err)
		}
	}
	return nil
}
