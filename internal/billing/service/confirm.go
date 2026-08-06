package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	checkoutfeature "github.com/upwifi/banking/internal/checkout"
	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	webhookdomain "github.com/upwifi/banking/internal/webhook/domain"
)

// ConfirmInfinitePayPaymentRequest carries the identifiers a buyer's browser
// returns with after paying on InfinitePay's hosted checkout page.
//
// They are NOT trusted as proof of payment — a buyer can edit any query
// string parameter. OrderNSU only locates the checkout; Slug and
// TransactionNSU are handed to the provider's own payment_check endpoint,
// which is the actual source of truth. ReceiptURL is display-only and is
// never validated (it has no bearing on activation).
type ConfirmInfinitePayPaymentRequest struct {
	OrderNSU       string
	Slug           string
	TransactionNSU string
	ReceiptURL     string
}

// ConfirmationResult reports what happened, distinguishing "already active"
// and "not paid yet" from a genuine new activation — a caller (the HTTP
// handler) needs this to answer correctly rather than always saying "ok".
type ConfirmationResult string

const (
	ConfirmationActivated   ConfirmationResult = "ACTIVATED"
	ConfirmationAlreadyPaid ConfirmationResult = "ALREADY_PAID"
	ConfirmationNotPaidYet  ConfirmationResult = "NOT_PAID_YET"
)

// ErrCheckoutNotFound is returned when OrderNSU does not match any checkout
// we created — either a forged value or a genuinely unknown order.
var ErrCheckoutNotFound = errors.New("billing: checkout not found for order_nsu")

// ConfirmInfinitePayPayment implements the /conta/concluido confirmation
// flow: instead of trusting redirect query params, it looks up the checkout
// by OrderNSU and asks InfinitePay's own payment_check endpoint whether it
// was actually paid.
//
// On confirmed payment, it converges on HandleCheckoutEvent — the same
// activation path the webhook uses — so no confirmation trigger
// reimplements activation logic or its fraud checks (payment amount,
// order reference).
func (s *Service) ConfirmInfinitePayPayment(ctx context.Context, req ConfirmInfinitePayPaymentRequest) (ConfirmationResult, error) {
	if req.OrderNSU == "" || req.Slug == "" || req.TransactionNSU == "" {
		return "", fmt.Errorf("billing: confirm payment: order_nsu, slug and transaction_nsu are all required")
	}

	checkoutRow, err := s.checkoutRepo.GetByProviderID(ctx, checkoutdomain.BaaSInfinitePay, req.OrderNSU)
	if err != nil {
		return "", ErrCheckoutNotFound
	}
	if checkoutRow.Status == checkoutdomain.StatusPaid {
		// Idempotent: the webhook, a previous redirect, or the
		// reconciliation job may have already activated this checkout.
		return ConfirmationAlreadyPaid, nil
	}

	details, err := s.checkoutSvc.CheckPayment(ctx, checkoutdomain.BaaSInfinitePay, checkoutdomain.CheckPaymentRequest{
		ProviderCheckoutID: req.OrderNSU,
		Slug:               req.Slug,
		TransactionNSU:     req.TransactionNSU,
	})
	if errors.Is(err, checkoutfeature.ErrNotSupported) {
		// Cannot happen in practice (this flow is InfinitePay-only, which
		// does support CheckPayment), but treated as "not confirmed" rather
		// than a hard error so a future wiring mistake fails soft here.
		return ConfirmationNotPaidYet, nil
	}
	if err != nil {
		return "", fmt.Errorf("billing: confirm payment: check with provider: %w", err)
	}
	if details.Status != checkoutdomain.StatusPaid {
		return ConfirmationNotPaidYet, nil
	}

	var paidAmountCents *int64
	if details.PaidAmount != nil {
		paidAmountCents = &details.PaidAmount.AmountCents
	}
	transactionID := req.TransactionNSU
	orderNSU := req.OrderNSU
	confirmedAt := time.Now()

	var receiptURL *string
	if req.ReceiptURL != "" {
		receiptURL = &req.ReceiptURL
	}

	event := webhookdomain.InboundEvent{
		BaaS:               string(checkoutdomain.BaaSInfinitePay),
		Status:             string(checkoutdomain.StatusPaid),
		ProviderCheckoutID: req.OrderNSU,
		OrderNSU:           &orderNSU,
		PaidAmountCents:    paidAmountCents,
		CaptureMethod:      details.CaptureMethod,
		Installments:       details.Installments,
		ReceiptURL:         receiptURL,
		TransactionID:      &transactionID,
		PaidAt:             &confirmedAt,
	}
	if err := s.HandleCheckoutEvent(ctx, event); err != nil {
		return "", fmt.Errorf("billing: confirm payment: activate: %w", err)
	}
	return ConfirmationActivated, nil
}
