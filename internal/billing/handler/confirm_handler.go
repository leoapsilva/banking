package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/upwifi/banking/internal/billing/service"
)

// confirmInfinitePayPaymentPayload carries only identifiers, never a
// payment fact — "paid" and "amount" are deliberately absent. The caller
// (the Cores backend, itself proxying a buyer's untrusted redirect) cannot
// assert payment here; only InfinitePay's own payment_check endpoint can.
type confirmInfinitePayPaymentPayload struct {
	OrderNSU       string `json:"order_nsu"`
	Slug           string `json:"slug"`
	TransactionNSU string `json:"transaction_nsu"`
	ReceiptURL     string `json:"receipt_url"`
}

// confirmInfinitePayPayment handles POST /v1/checkouts/infinitepay/confirm.
// See service.ConfirmInfinitePayPayment for the fraud-prevention rationale.
func (h *Handler) confirmInfinitePayPayment(w http.ResponseWriter, r *http.Request) {
	var p confirmInfinitePayPaymentPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if p.OrderNSU == "" || p.Slug == "" || p.TransactionNSU == "" {
		writeError(w, http.StatusBadRequest, "order_nsu, slug and transaction_nsu are all required")
		return
	}

	result, err := h.svc.ConfirmInfinitePayPayment(r.Context(), service.ConfirmInfinitePayPaymentRequest{
		OrderNSU:       p.OrderNSU,
		Slug:           p.Slug,
		TransactionNSU: p.TransactionNSU,
		ReceiptURL:     p.ReceiptURL,
	})
	if errors.Is(err, service.ErrCheckoutNotFound) {
		writeError(w, http.StatusNotFound, "checkout not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": string(result)})
}
