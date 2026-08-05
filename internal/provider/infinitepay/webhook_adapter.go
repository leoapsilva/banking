package infinitepay

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/provider/infinitepay/dto"
	webhookdomain "github.com/upwifi/banking/internal/webhook/domain"
)

// WebhookAdapter implements webhook.InboundParser for InfinitePay.
// InfinitePay does not have a webhook registration API (the webhook_url is
// sent per-checkout in POST /links), so this adapter only implements
// ParseInbound — not RegistrarProvider.
type WebhookAdapter struct{}

func NewWebhookAdapter() *WebhookAdapter { return &WebhookAdapter{} }

// ParseInbound decodes an InfinitePay payment notification.
// InfinitePay only fires webhooks on successful payment, so status is always
// PAID. The payload carries full payment details (capture_method, installments,
// paid_amount, receipt_url, transaction_nsu) which are propagated via the
// InboundEvent payment confirmation fields so the subscription service can
// persist them to the checkouts table without a separate API call.
func (a *WebhookAdapter) ParseInbound(_ context.Context, rawBody []byte) (webhookdomain.InboundEvent, error) {
	var p dto.WebhookPayload
	if err := json.Unmarshal(rawBody, &p); err != nil {
		return webhookdomain.InboundEvent{}, fmt.Errorf("infinitepay/webhook: decode payload: %w", err)
	}
	if p.InvoiceSlug == "" {
		return webhookdomain.InboundEvent{}, fmt.Errorf("infinitepay/webhook: missing invoice_slug")
	}
	// order_nsu is what we stored as ProviderCheckoutID at creation time (see
	// mapper.FromCreateLinkResponse) — InfinitePay's POST /links response
	// carries no usable identifier of its own (verified against a real
	// sandbox call: only `url` comes back, no `slug`). Without order_nsu
	// there is nothing to correlate this event against.
	if p.OrderNSU == "" {
		return webhookdomain.InboundEvent{}, fmt.Errorf("infinitepay/webhook: missing order_nsu")
	}

	now := time.Now().UTC()

	// Build typed pointers for payment confirmation fields.
	paidAmountCents := p.PaidAmount
	captureMethod := p.CaptureMethod
	installments := p.Installments
	receiptURL := p.ReceiptURL
	transactionID := p.TransactionNSU

	orderNSU := p.OrderNSU

	event := webhookdomain.InboundEvent{
		BaaS: string(checkoutdomain.BaaSInfinitePay),
		// ExternalID must be stable across retransmissions for deduplication.
		// invoice_slug + transaction_nsu is the most granular stable identity
		// (transaction_nsu is only present post-payment, so this combination
		// is unique per payment attempt).
		ExternalID:    p.InvoiceSlug + ":" + p.TransactionNSU,
		Service:       webhookdomain.ServiceCheckout,
		Status:        string(checkoutdomain.StatusPaid),
		EventDateTime: now,
		RawPayload:    rawBody,
		// ProviderCheckoutID must match what Create() stored — order_nsu, not
		// invoice_slug. invoice_slug still identifies the transaction to
		// InfinitePay (kept in ExternalID for dedup) but was never available
		// to us at checkout-creation time, so it can't be the correlation key.
		ProviderCheckoutID: orderNSU,
		// Set unconditionally: OrderNSU is guaranteed non-empty by the check
		// above. Kept as its own field (not just ProviderCheckoutID) so
		// orderReferenceTrusted can do an explicit second comparison against
		// the stored ExternalReferenceID — defence in depth, not just routing.
		OrderNSU:        &orderNSU,
		PaidAmountCents: &paidAmountCents,
		CaptureMethod:   &captureMethod,
		Installments:    &installments,
		ReceiptURL:      &receiptURL,
		TransactionID:   &transactionID,
		PaidAt:          &now,
	}

	return event, nil
}
