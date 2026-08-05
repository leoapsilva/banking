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

func checkoutWithReference(ref string) checkoutrepo.Checkout {
	c := checkoutOf(29990)
	c.ExternalReferenceID = ref
	return c
}

func eventWithOrderNSU(nsu *string) webhookdomain.InboundEvent {
	return webhookdomain.InboundEvent{
		Status:   string(checkoutdomain.StatusPaid),
		OrderNSU: nsu,
	}
}

func nsu(v string) *string { return &v }

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

func TestMatchingOrderNSUAccepted(t *testing.T) {
	if !orderReferenceTrusted(checkoutWithReference("REF123"), eventWithOrderNSU(nsu("REF123"))) {
		t.Error("matching order_nsu must be accepted")
	}
}

// The attack this guard exists for: a forger who fakes the amount but does
// not know (or guesses wrong) the order_nsu we generated at checkout creation.
func TestMismatchedOrderNSURejected(t *testing.T) {
	if orderReferenceTrusted(checkoutWithReference("REF123"), eventWithOrderNSU(nsu("WRONG"))) {
		t.Error("mismatched order_nsu must be rejected")
	}
}

// order_nsu was undocumented in our webhook payload until this check was
// added; a nil value means "provider didn't send it", not "forged" — must
// not break existing/other-provider flows.
func TestEventWithoutOrderNSUAccepted(t *testing.T) {
	if !orderReferenceTrusted(checkoutWithReference("REF123"), eventWithOrderNSU(nil)) {
		t.Error("event without order_nsu must be accepted (nothing to compare)")
	}
}

// A checkout created before this field existed (or by a path that never set
// it) has no reference to compare against — must not reject on absence of
// our own data.
func TestCheckoutWithoutStoredReferenceAccepted(t *testing.T) {
	if !orderReferenceTrusted(checkoutWithReference(""), eventWithOrderNSU(nsu("ANYTHING"))) {
		t.Error("checkout with no stored reference must be accepted (nothing to compare against)")
	}
}
