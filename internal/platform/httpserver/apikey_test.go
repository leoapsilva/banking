package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(Caller(r.Context())))
	})
}

func TestAPIKeyAcceptsValidKey(t *testing.T) {
	mw := APIKeyAuth(map[string]string{"cores": "s3cret"}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	req.Header.Set(APIKeyHeader, "s3cret")
	rec := httptest.NewRecorder()

	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "cores" {
		t.Errorf("caller = %q, want %q", body, "cores")
	}
}

func TestAPIKeyRejectsMissingKey(t *testing.T) {
	mw := APIKeyAuth(map[string]string{"cores": "s3cret"}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	rec := httptest.NewRecorder()

	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAPIKeyRejectsWrongKey(t *testing.T) {
	mw := APIKeyAuth(map[string]string{"cores": "s3cret"}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	req.Header.Set(APIKeyHeader, "wrong")
	rec := httptest.NewRecorder()

	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// The response must not disclose which key was expected, nor echo what was
// presented — a 401 body that varies gives an attacker a probing oracle.
func TestUnauthorizedBodyLeaksNothing(t *testing.T) {
	mw := APIKeyAuth(map[string]string{"cores": "s3cret"}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	req.Header.Set(APIKeyHeader, "guess")
	rec := httptest.NewRecorder()

	mw(okHandler()).ServeHTTP(rec, req)

	body := rec.Body.String()
	if body != `{"error":"unauthorized"}` {
		t.Errorf("body = %q, want a fixed opaque message", body)
	}
}

func TestHealthzIsExempt(t *testing.T) {
	mw := APIKeyAuth(map[string]string{"cores": "s3cret"}, []string{"/healthz"})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (healthz must answer without a key)", rec.Code)
	}
}

// Providers call the webhook paths and cannot hold our key.
func TestWebhookPrefixIsExempt(t *testing.T) {
	mw := APIKeyAuth(map[string]string{"cores": "s3cret"}, []string{"/webhooks/"})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/infinitepay/somesecret", nil)
	rec := httptest.NewRecorder()

	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

// An exempt prefix must not accidentally open unrelated paths.
func TestExemptPrefixDoesNotOverreach(t *testing.T) {
	mw := APIKeyAuth(map[string]string{"cores": "s3cret"}, []string{"/webhooks/"})
	req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
	rec := httptest.NewRecorder()

	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMultipleCallersGetDistinctIdentities(t *testing.T) {
	keys := map[string]string{"cores": "key-a", "outro": "key-b"}
	mw := APIKeyAuth(keys, nil)

	for name, key := range keys {
		req := httptest.NewRequest(http.MethodGet, "/v1/subscriptions", nil)
		req.Header.Set(APIKeyHeader, key)
		rec := httptest.NewRecorder()

		mw(okHandler()).ServeHTTP(rec, req)

		if got := rec.Body.String(); got != name {
			t.Errorf("caller = %q, want %q", got, name)
		}
	}
}
