package httpserver

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
)

// APIKeyHeader is the header callers must present.
const APIKeyHeader = "X-API-Key"

// APIKeyAuth rejects requests that do not present a configured API key.
//
// Until now every route was anonymous, including subscription cancellation
// and webhook registration. Even on a private network this is defence in
// depth: it stops anything else on that network from driving the API, and
// gives each caller a distinguishable identity in the logs.
//
// Paths listed in exempt are served without a key. Two things belong there
// and nothing else:
//   - /healthz, which must answer before any credential is provisioned;
//   - /webhooks/*, which is called by the payment providers, who cannot hold
//     our key. Those paths carry their own secret in the URL and are
//     validated separately.
//
// Comparison is constant-time so a caller cannot recover a key byte by byte
// from response timing.
func APIKeyAuth(keys map[string]string, exempt []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isExempt(r.URL.Path, exempt) {
				next.ServeHTTP(w, r)
				return
			}

			presented := r.Header.Get(APIKeyHeader)
			if presented == "" {
				unauthorized(w, r, "missing api key")
				return
			}

			name, ok := lookupKey(keys, presented)
			if !ok {
				unauthorized(w, r, "invalid api key")
				return
			}

			// Carry the caller's identity so handlers can scope ownership
			// checks to it rather than trusting an id from the request body.
			next.ServeHTTP(w, r.WithContext(WithCaller(r.Context(), name)))
		})
	}
}

// lookupKey compares the presented key against every configured key in
// constant time. Every candidate is checked even after a match so the work
// done does not depend on which key matched, or whether any did.
func lookupKey(keys map[string]string, presented string) (string, bool) {
	var matched string
	var found bool
	for name, key := range keys {
		if subtle.ConstantTimeCompare([]byte(presented), []byte(key)) == 1 {
			matched = name
			found = true
		}
	}
	return matched, found
}

func isExempt(path string, exempt []string) bool {
	for _, prefix := range exempt {
		if path == prefix || strings.HasPrefix(path, strings.TrimSuffix(prefix, "*")) {
			return true
		}
	}
	return false
}

// unauthorized answers without echoing the presented key or revealing whether
// the key was absent or merely wrong to anyone but our own logs.
func unauthorized(w http.ResponseWriter, r *http.Request, reason string) {
	slog.Warn("api key rejected", "reason", reason, "path", r.URL.Path, "request_id", RequestID(r.Context()))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}
