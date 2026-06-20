package config

import (
	"fmt"
	"os"
	"strconv"
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

	CronChargeInterval string // cron expression, default hourly

	LogLevel    string // debug, info, warn, error
	LogHTTPBody bool   // log request/response bodies (debug only; never logs Authorization/secrets)
}

// Load reads configuration from environment variables. Required fields
// return an error if missing so the process fails fast at startup.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:            getEnvDefault("HTTP_ADDR", ":8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		C6Env:               getEnvDefault("C6_ENV", "sandbox"),
		C6ClientID:          os.Getenv("C6_CLIENT_ID"),
		C6ClientSecret:      os.Getenv("C6_CLIENT_SECRET"),
		C6MTLSCertPath:      os.Getenv("C6_MTLS_CERT_PATH"),
		C6MTLSKeyPath:       os.Getenv("C6_MTLS_KEY_PATH"),
		C6PartnerName:       getEnvDefault("C6_PARTNER_SOFTWARE_NAME", "banking"),
		C6PartnerVersion:    getEnvDefault("C6_PARTNER_SOFTWARE_VERSION", "0.1.0"),
		WebhookPathSecret:   os.Getenv("C6_WEBHOOK_PATH_SECRET"),
		C6ExpectedClientID:  os.Getenv("C6_EXPECTED_CLIENT_ID"),
		C6ExpectedPartnerID: os.Getenv("C6_EXPECTED_PARTNER_ID"),
		CronChargeInterval:  getEnvDefault("CRON_CHARGE_INTERVAL", "@hourly"),
		LogLevel:            getEnvDefault("LOG_LEVEL", "info"),
		LogHTTPBody:         getEnvDefault("LOG_HTTP_BODY", "false") == "true",
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
