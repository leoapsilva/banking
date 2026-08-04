package service

import (
	"testing"

	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	checkoutrepo "github.com/upwifi/banking/internal/checkout/repository"
	webhookdomain "github.com/upwifi/banking/internal/webhook/domain"
)

func checkoutOf(cents int64) checkoutrepo.Checkout {
	return checkoutrepo.Checkout{
		ProviderCheckoutID: "abc123",
		Amount:             checkoutdomain.Money{AmountCents: cents, Currency: "BRL"},
	}
}

func paidEvent(cents *int64) webhookdomain.InboundEvent {
	return webhookdomain.InboundEvent{
		Status:          string(checkoutdomain.StatusPaid),
		PaidAmountCents: cents,
	}
}

func cents(v int64) *int64 { return &v }

func TestExactAmountAccepted(t *testing.T) {
	if !paymentAmountTrusted(checkoutOf(29990), paidEvent(cents(29990))) {
		t.Error("exact amount must be accepted")
	}
}

// Providers may add their fee on top of the requested amount, so paying more
// than expected is legitimate.
func TestOverpaymentAccepted(t *testing.T) {
	if !paymentAmountTrusted(checkoutOf(29990), paidEvent(cents(30590))) {
		t.Error("overpayment (provider fee) must be accepted")
	}
}

// The attack this guard exists for: a forged notification claiming a token
// payment against a real pending checkout.
func TestUnderpaymentRejected(t *testing.T) {
	if paymentAmountTrusted(checkoutOf(29990), paidEvent(cents(1))) {
		t.Error("underpayment must be rejected")
	}
}

func TestSlightUnderpaymentRejected(t *testing.T) {
	if paymentAmountTrusted(checkoutOf(29990), paidEvent(cents(29989))) {
		t.Error("any shortfall must be rejected — tolerance is zero by design")
	}
}

// C6 does not carry amounts in the webhook; rejecting those would break it.
func TestEventWithoutAmountAccepted(t *testing.T) {
	if !paymentAmountTrusted(checkoutOf(29990), paidEvent(nil)) {
		t.Error("event without amount must be accepted (nothing to compare)")
	}
}

func TestZeroReportedAmountRejected(t *testing.T) {
	if paymentAmountTrusted(checkoutOf(29990), paidEvent(cents(0))) {
		t.Error("explicit zero must be rejected, unlike a nil amount")
	}
}
