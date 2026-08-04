package config

import "testing"

func cfgWith(url, token string) *Config {
	return &Config{
		InfinitePayWebhookURL:        url,
		InfinitePayWebhookPathSecret: token,
	}
}

func TestWebhookURLEndingWithTokenIsValid(t *testing.T) {
	c := cfgWith("https://exemplo.com.br/webhooks/infinitepay/abc123", "abc123")
	if err := validateInfinitePayWebhook(c); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// A trailing slash is a plausible typo and should not be treated as a mismatch.
func TestTrailingSlashTolerated(t *testing.T) {
	c := cfgWith("https://exemplo.com.br/webhooks/infinitepay/abc123/", "abc123")
	if err := validateInfinitePayWebhook(c); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// The failure this guard exists for: the URL looks right but omits the token,
// so InfinitePay would POST to a route the service never registered.
func TestWebhookURLWithoutTokenRejected(t *testing.T) {
	c := cfgWith("https://exemplo.com.br/webhooks/infinitepay", "abc123")
	if err := validateInfinitePayWebhook(c); err == nil {
		t.Error("expected error when the URL does not end with the token")
	}
}

func TestWebhookURLWithWrongTokenRejected(t *testing.T) {
	c := cfgWith("https://exemplo.com.br/webhooks/infinitepay/outrovalor", "abc123")
	if err := validateInfinitePayWebhook(c); err == nil {
		t.Error("expected error when the URL ends with a different token")
	}
}

// A token that merely appears somewhere in the URL is not enough — it has to
// be the final segment, since that is what the route registration uses.
func TestTokenInTheMiddleRejected(t *testing.T) {
	c := cfgWith("https://exemplo.com.br/abc123/webhooks/infinitepay", "abc123")
	if err := validateInfinitePayWebhook(c); err == nil {
		t.Error("expected error when the token is not the last path segment")
	}
}

func TestBothEmptyIsValid(t *testing.T) {
	if err := validateInfinitePayWebhook(cfgWith("", "")); err != nil {
		t.Errorf("InfinitePay is optional; unexpected error: %v", err)
	}
}

func TestOnlyURLSetRejected(t *testing.T) {
	c := cfgWith("https://exemplo.com.br/webhooks/infinitepay/abc123", "")
	if err := validateInfinitePayWebhook(c); err == nil {
		t.Error("expected error when only the URL is set")
	}
}

func TestOnlyTokenSetRejected(t *testing.T) {
	if err := validateInfinitePayWebhook(cfgWith("", "abc123")); err == nil {
		t.Error("expected error when only the token is set")
	}
}
