package service

import (
	"log/slog"

	checkoutrepo "github.com/upwifi/banking/internal/checkout/repository"
	webhookdomain "github.com/upwifi/banking/internal/webhook/domain"
)

// underpaidTolerance is how much less than the expected amount we still
// accept as payment, in cents.
//
// It is zero deliberately. A provider may report *more* than we asked (the
// InfinitePay card fee is added on top of the requested amount and shows up
// in paid_amount), but never less. Any shortfall is either a forged
// notification or a real discrepancy — both need a human, not an automatic
// activation.
const underpaidTolerance int64 = 0

// paymentAmountTrusted reports whether the amount in a PAID webhook is
// consistent with what the checkout was created for.
//
// Without this check, anyone who learns a pending checkout's provider id —
// it travels in the buyer's own URL — could POST a forged "paid" event and
// activate a subscription without paying. The path secret alone is not
// enough, because it does not change per checkout.
//
// Events that carry no amount (C6 sends payment details only in separate GET
// responses) are accepted: there is nothing to compare, and rejecting them
// would break the C6 flow.
func paymentAmountTrusted(checkout checkoutrepo.Checkout, event webhookdomain.InboundEvent) bool {
	if event.PaidAmountCents == nil {
		return true
	}
	expected := checkout.Amount.AmountCents
	return *event.PaidAmountCents >= expected-underpaidTolerance
}

// logRejectedPayment records why a PAID event was not honoured. The amounts
// are logged because reconciling them is exactly what an operator needs, and
// neither value is a secret.
func logRejectedPayment(checkout checkoutrepo.Checkout, event webhookdomain.InboundEvent) {
	var paid int64
	if event.PaidAmountCents != nil {
		paid = *event.PaidAmountCents
	}
	slog.Error("billing: refusing PAID webhook with amount below the checkout value",
		"provider_checkout_id", checkout.ProviderCheckoutID,
		"baas", string(checkout.BaaS),
		"expected_cents", checkout.Amount.AmountCents,
		"reported_cents", paid,
	)
}

// orderReferenceTrusted reports whether a webhook's order_nsu (InfinitePay's
// echo of the reference we supplied when creating the checkout, see
// https://www.infinitepay.io/checkout-documentacao) matches what we stored.
//
// This is a second, independent signal alongside the amount check: the two
// are set at different points (order_nsu at checkout creation, amount is the
// price) and a forger who fakes one has to fake both.
//
// Events with no order_nsu are accepted — the field wasn't documented as
// carried by the webhook until this check was added, and providers other
// than InfinitePay never populate it. Absence is not evidence of forgery;
// only a mismatched value is.
func orderReferenceTrusted(checkout checkoutrepo.Checkout, event webhookdomain.InboundEvent) bool {
	if event.OrderNSU == nil {
		return true
	}
	return checkout.ExternalReferenceID == "" || *event.OrderNSU == checkout.ExternalReferenceID
}

// logRejectedOrderReference records an order_nsu mismatch the same way
// logRejectedPayment records an amount mismatch — visible to an operator,
// nothing secret in either value.
func logRejectedOrderReference(checkout checkoutrepo.Checkout, event webhookdomain.InboundEvent) {
	var got string
	if event.OrderNSU != nil {
		got = *event.OrderNSU
	}
	slog.Error("billing: refusing PAID webhook with order_nsu mismatch",
		"provider_checkout_id", checkout.ProviderCheckoutID,
		"baas", string(checkout.BaaS),
		"expected_order_nsu", checkout.ExternalReferenceID,
		"reported_order_nsu", got,
	)
}
