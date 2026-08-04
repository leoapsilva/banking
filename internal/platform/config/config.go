package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all environment-driven configuration for the service.
type Config struct {
	HTTPAddr string

	DatabaseURL string

	C6Env            string // "sandbox" or "production"
	C6ClientID       string
	C6ClientSecret   string
	C6MTLSCertPath   string
	C6MTLSKeyPath    string
	C6PartnerName    string
	C6PartnerVersion string

	WebhookPathSecret   string
	C6ExpectedClientID  string
	C6ExpectedPartnerID string

	// InfinitePay provider (all optional — deployment may use only C6)
	InfinitePayBaseURL           string // default: https://api.checkout.infinitepay.io
	InfinitePayHandle            string // InfiniteTag without leading '$'
	InfinitePayWebhookURL        string // public URL of our POST /webhooks/infinitepay/{secret} endpoint
	InfinitePayWebhookPathSecret string // path token for minimal obscurity on the inbound route
	InfinitePayPlanMonthlyURL    string // pre-created monthly plan link from InfinitePay app

	CronChargeInterval string // cron expression, default hourly

	LogLevel    string // debug, info, warn, error
	LogHTTPBody bool   // log request/response bodies (debug only; never logs Authorization/secrets)

	// APIKeys maps a caller name to its secret, parsed from API_KEYS in the
	// form "name:secret,other:secret". Callers present the secret in the
	// X-API-Key header; the name is what appears in logs and scopes
	// ownership. Empty disables authentication entirely — only acceptable
	// for local development, and logged loudly at startup.
	APIKeys map[string]string

	// MaxRequestBodyBytes caps request bodies so a single caller cannot
	// exhaust memory. Webhook payloads are the largest legitimate body.
	MaxRequestBodyBytes int64
}

// Load reads configuration from environment variables. Required fields
// return an error if missing so the process fails fast at startup.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:                     getEnvDefault("HTTP_ADDR", ":8080"),
		DatabaseURL:                  os.Getenv("DATABASE_URL"),
		C6Env:                        getEnvDefault("C6_ENV", "sandbox"),
		C6ClientID:                   os.Getenv("C6_CLIENT_ID"),
		C6ClientSecret:               os.Getenv("C6_CLIENT_SECRET"),
		C6MTLSCertPath:               os.Getenv("C6_MTLS_CERT_PATH"),
		C6MTLSKeyPath:                os.Getenv("C6_MTLS_KEY_PATH"),
		C6PartnerName:                getEnvDefault("C6_PARTNER_SOFTWARE_NAME", "banking"),
		C6PartnerVersion:             getEnvDefault("C6_PARTNER_SOFTWARE_VERSION", "0.1.0"),
		WebhookPathSecret:            os.Getenv("C6_WEBHOOK_PATH_SECRET"),
		C6ExpectedClientID:           os.Getenv("C6_EXPECTED_CLIENT_ID"),
		C6ExpectedPartnerID:          os.Getenv("C6_EXPECTED_PARTNER_ID"),
		InfinitePayBaseURL:           getEnvDefault("INFINITEPAY_BASE_URL", "https://api.checkout.infinitepay.io"),
		InfinitePayHandle:            os.Getenv("INFINITEPAY_HANDLE"),
		InfinitePayWebhookURL:        os.Getenv("INFINITEPAY_WEBHOOK_URL"),
		InfinitePayWebhookPathSecret: os.Getenv("INFINITEPAY_WEBHOOK_PATH_SECRET"),
		InfinitePayPlanMonthlyURL:    os.Getenv("INFINITEPAY_PLAN_MONTHLY_URL"),
		CronChargeInterval:           getEnvDefault("CRON_CHARGE_INTERVAL", "@hourly"),
		LogLevel:                     getEnvDefault("LOG_LEVEL", "info"),
		LogHTTPBody:                  getEnvDefault("LOG_HTTP_BODY", "false") == "true",
		APIKeys:                      parseAPIKeys(os.Getenv("API_KEYS")),
		MaxRequestBodyBytes:          parseBytes(getEnvDefault("MAX_REQUEST_BODY_BYTES", "1048576")),
	}

	required := map[string]string{
		"DATABASE_URL":           cfg.DatabaseURL,
		"C6_CLIENT_ID":           cfg.C6ClientID,
		"C6_CLIENT_SECRET":       cfg.C6ClientSecret,
		"C6_MTLS_CERT_PATH":      cfg.C6MTLSCertPath,
		"C6_MTLS_KEY_PATH":       cfg.C6MTLSKeyPath,
		"C6_WEBHOOK_PATH_SECRET": cfg.WebhookPathSecret,
	}
	for name, val := range required {
		if val == "" {
			return nil, fmt.Errorf("missing required environment variable %s", name)
		}
	}

	return cfg, nil
}

// C6BaseURL returns the host for the configured environment.
func (c *Config) C6BaseURL() string {
	if c.C6Env == "production" {
		return "https://baas-api.c6bank.info"
	}
	return "https://baas-api-sandbox.c6bank.info"
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseAPIKeys reads "name:secret,other:secret" into a map of caller name to
// secret. Malformed entries are skipped rather than failing startup, so one
// bad entry cannot take the service down — but every skip is visible because
// the resulting key simply will not authenticate.
//
// Returns nil for an empty value, which callers treat as "authentication
// disabled".
func parseAPIKeys(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	keys := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		name, secret, ok := strings.Cut(strings.TrimSpace(pair), ":")
		name, secret = strings.TrimSpace(name), strings.TrimSpace(secret)
		if !ok || name == "" || secret == "" {
			continue
		}
		keys[name] = secret
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

// parseBytes parses a byte count, falling back to 1 MiB when the value is
// absent or unparseable.
func parseBytes(raw string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || v <= 0 {
		return 1 << 20
	}
	return v
}

// MustAtoi parses an int env var, returning def on error/empty.
func MustAtoi(val string, def int) int {
	if val == "" {
		return def
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return n
}
