package infinitepay

import (
	"context"
	"testing"
)

func validPayload() []byte {
	return []byte(`{
		"invoice_slug": "abc123slug",
		"order_nsu": "ORDER-001",
		"amount": 29990,
		"paid_amount": 29990,
		"transaction_nsu": "txn-1"
	}`)
}

// ProviderCheckoutID must be order_nsu, not invoice_slug — it's the only
// value we know at checkout-creation time (see mapper.FromCreateLinkResponse),
// so it's the only value the webhook can be correlated against.
func TestParseInboundUsesOrderNSUAsProviderCheckoutID(t *testing.T) {
	event, err := NewWebhookAdapter().ParseInbound(context.Background(), validPayload())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.ProviderCheckoutID != "ORDER-001" {
		t.Errorf("ProviderCheckoutID = %q, want %q (order_nsu, not invoice_slug)", event.ProviderCheckoutID, "ORDER-001")
	}
}

// invoice_slug is still the identity InfinitePay uses, kept for dedup —
// just not usable as the correlation key against our stored checkout.
func TestParseInboundStillUsesInvoiceSlugForDedup(t *testing.T) {
	event, err := NewWebhookAdapter().ParseInbound(context.Background(), validPayload())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.ExternalID != "abc123slug:txn-1" {
		t.Errorf("ExternalID = %q, want %q", event.ExternalID, "abc123slug:txn-1")
	}
}

func TestParseInboundRejectsMissingInvoiceSlug(t *testing.T) {
	payload := []byte(`{"order_nsu": "ORDER-001"}`)
	if _, err := NewWebhookAdapter().ParseInbound(context.Background(), payload); err == nil {
		t.Error("expected error when invoice_slug is missing")
	}
}

// The attack/failure this guards against: without order_nsu there is no
// value to correlate against the stored checkout at all — accepting the
// event would either silently drop it or, worse, match nothing and get
// treated as an unrelated one-off checkout.
func TestParseInboundRejectsMissingOrderNSU(t *testing.T) {
	payload := []byte(`{"invoice_slug": "abc123slug"}`)
	if _, err := NewWebhookAdapter().ParseInbound(context.Background(), payload); err == nil {
		t.Error("expected error when order_nsu is missing")
	}
}

func TestParseInboundSetsOrderNSUField(t *testing.T) {
	event, err := NewWebhookAdapter().ParseInbound(context.Background(), validPayload())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.OrderNSU == nil || *event.OrderNSU != "ORDER-001" {
		t.Errorf("OrderNSU = %v, want pointer to %q", event.OrderNSU, "ORDER-001")
	}
}
