// Package client provides a shared HTTP client for every C6 API
// (checkout, webhooks, and future boleto/pix adapters): mTLS transport,
// bearer token injection with single-retry-on-401, partner headers, and
// request/response logging.
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

	"github.com/upwifi/banking/internal/provider/c6/auth"
)

// Client wraps an mTLS-enabled *http.Client with C6 auth and headers.
type Client struct {
	httpClient     *http.Client
	tokenManager   *auth.TokenManager
	baseURL        string
	partnerName    string
	partnerVersion string
}

// New creates a client for a given C6 API base URL (e.g.
// "https://baas-api-sandbox.c6bank.info/v1/checkouts").
func New(httpClient *http.Client, tokenManager *auth.TokenManager, baseURL, partnerName, partnerVersion string) *Client {
	return &Client{
		httpClient:     httpClient,
		tokenManager:   tokenManager,
		baseURL:        strings.TrimRight(baseURL, "/"),
		partnerName:    partnerName,
		partnerVersion: partnerVersion,
	}
}

// APIError represents a non-2xx response from C6. Body is populated
// whenever C6 returned one, including on 401 — earlier versions of this
// client silently discarded the body on 401, which hid the real reason
// for auth failures (e.g. a product/scope not yet activated on the
// account) behind an empty error message.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("c6: unexpected status %d: %s", e.StatusCode, e.Body)
}

// Do performs a JSON request against path (relative to baseURL), retrying
// once with a forced token refresh if the first attempt returns 401.
// reqBody may be nil; respBody may be nil if the caller doesn't need the
// decoded response (e.g. 204 responses).
func (c *Client) Do(ctx context.Context, method, path string, reqBody, respBody any) error {
	status, body, err := c.doOnce(ctx, method, path, reqBody, respBody, false)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized {
		status, body, err = c.doOnce(ctx, method, path, reqBody, respBody, true)
		if err != nil {
			return err
		}
	}
	if status < 200 || status >= 300 {
		return &APIError{StatusCode: status, Body: string(body)}
	}
	return nil
}

func (c *Client) doOnce(ctx context.Context, method, path string, reqBody, respBody any, forceRefresh bool) (status int, rawBody []byte, err error) {
	var token string
	if forceRefresh {
		token, err = c.tokenManager.ForceRefresh(ctx)
	} else {
		token, err = c.tokenManager.GetToken(ctx)
	}
	if err != nil {
		return 0, nil, fmt.Errorf("c6/client: get token: %w", err)
	}

	var reqBodyBytes []byte
	var bodyReader io.Reader
	if reqBody != nil {
		reqBodyBytes, err = json.Marshal(reqBody)
		if err != nil {
			return 0, nil, fmt.Errorf("c6/client: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(reqBodyBytes)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return 0, nil, fmt.Errorf("c6/client: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.partnerName != "" {
		req.Header.Set("partner-software-name", c.partnerName)
	}
	if c.partnerVersion != "" {
		req.Header.Set("partner-software-version", c.partnerVersion)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Error("c6 api call failed", "method", method, "url", url, "error", err)
		return 0, nil, fmt.Errorf("c6/client: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	duration := time.Since(start)

	level := slog.LevelInfo
	switch {
	case resp.StatusCode >= 500:
		level = slog.LevelError
	case resp.StatusCode >= 400:
		level = slog.LevelWarn
	}
	slog.Log(ctx, level, "c6 api call",
		"method", method, "url", url, "status", resp.StatusCode, "duration_ms", duration.Milliseconds())

	// Never logs headers (so the Authorization bearer token never reaches
	// logs); bodies are debug-only since they may carry payer PII.
	slog.Debug("c6 api call body",
		"method", method, "url", url, "request_body", string(reqBodyBytes), "response_body", string(raw))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, raw, nil
	}

	if respBody != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, respBody); err != nil {
			return resp.StatusCode, raw, fmt.Errorf("c6/client: decode response: %w", err)
		}
	}

	return resp.StatusCode, raw, nil
}
