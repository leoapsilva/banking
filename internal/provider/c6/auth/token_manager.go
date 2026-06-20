// Package auth implements C6's mTLS + OAuth2 client_credentials handshake
// and caches the resulting access token so callers don't reauthenticate on
// every request.
package auth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// expiryMargin is subtracted from the token's reported expires_in so we
// refresh slightly before the C6 side would consider it stale.
const expiryMargin = 30 * time.Second

// TokenManager fetches and caches a C6 OAuth2 access token. Safe for
// concurrent use; refreshes are serialized so concurrent callers don't
// trigger redundant POST /v1/auth/ calls.
type TokenManager struct {
	httpClient   *http.Client
	authURL      string
	clientID     string
	clientSecret string

	mu        sync.RWMutex
	token     string
	expiresAt time.Time
}

// NewTLSHTTPClient builds an *http.Client configured for mTLS using the
// client certificate/key pair C6 issues per credential.
func NewTLSHTTPClient(certPath, keyPath string) (*http.Client, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("c6/auth: load mTLS keypair: %w", err)
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		},
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
}

// New creates a TokenManager. httpClient must already be configured with
// the mTLS certificate (see NewTLSHTTPClient) since the auth endpoint also
// requires mutual TLS.
func New(httpClient *http.Client, baseURL, clientID, clientSecret string) *TokenManager {
	return &TokenManager{
		httpClient:   httpClient,
		authURL:      strings.TrimRight(baseURL, "/") + "/v1/auth/",
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// GetToken returns a cached valid token or fetches a new one.
func (tm *TokenManager) GetToken(ctx context.Context) (string, error) {
	tm.mu.RLock()
	if tm.tokenValidLocked() {
		tok := tm.token
		tm.mu.RUnlock()
		return tok, nil
	}
	tm.mu.RUnlock()
	return tm.refresh(ctx)
}

// ForceRefresh discards any cached token and fetches a new one. Callers
// should invoke this once after receiving an HTTP 401, to handle the case
// where C6 revoked the token early due to inactivity, then retry.
func (tm *TokenManager) ForceRefresh(ctx context.Context) (string, error) {
	tm.mu.Lock()
	tm.expiresAt = time.Time{}
	tm.mu.Unlock()
	return tm.refresh(ctx)
}

func (tm *TokenManager) tokenValidLocked() bool {
	return tm.token != "" && time.Now().Before(tm.expiresAt)
}

func (tm *TokenManager) refresh(ctx context.Context) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check after acquiring the write lock: another goroutine may
	// have already refreshed while we were waiting.
	if tm.tokenValidLocked() {
		return tm.token, nil
	}

	form := url.Values{}
	form.Set("client_id", tm.clientID)
	form.Set("client_secret", tm.clientSecret)
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tm.authURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("c6/auth: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	start := time.Now()
	resp, err := tm.httpClient.Do(req)
	if err != nil {
		slog.Error("c6 auth call failed", "url", tm.authURL, "error", err)
		return "", fmt.Errorf("c6/auth: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Intentionally never logs the request body (client_id/client_secret)
	// or the response body (contains the access_token itself).
	slog.Info("c6 auth call", "url", tm.authURL, "status", resp.StatusCode, "duration_ms", time.Since(start).Milliseconds())

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("c6/auth: unexpected status %d", resp.StatusCode)
	}

	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("c6/auth: decode response: %w", err)
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("c6/auth: empty access_token in response")
	}

	tm.token = body.AccessToken
	ttl := time.Duration(body.ExpiresIn) * time.Second
	if ttl <= expiryMargin {
		ttl = expiryMargin * 2 // defensive floor in case C6 ever returns a very short TTL
	}
	tm.expiresAt = time.Now().Add(ttl - expiryMargin)

	return tm.token, nil
}
