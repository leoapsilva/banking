package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/billing/service"
)

type Handler struct {
	svc            *service.Service
	monthlyPlanURL string // INFINITEPAY_PLAN_MONTHLY_URL — returned as-is, no API call
}

func New(svc *service.Service, monthlyPlanURL string) *Handler {
	return &Handler{svc: svc, monthlyPlanURL: monthlyPlanURL}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/subscriptions/monthly", h.createMonthly)
	mux.HandleFunc("POST /v1/subscriptions/annual-installment", h.createAnnualInstallment)
	mux.HandleFunc("POST /v1/subscriptions/annual", h.createAnnualCheckout)
	mux.HandleFunc("GET /v1/subscriptions/infinitepay/plans/monthly", h.getMonthlyPlanLink)
	mux.HandleFunc("PUT /v1/subscriptions/{id}/cancel", h.cancel)
}

type createMonthlyPayload struct {
	BaaS          string `json:"baas"`
	CustomerName  string `json:"customer_name"`
	CustomerTaxID string `json:"customer_tax_id"`
	CustomerEmail string `json:"customer_email"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	RedirectURL   string `json:"redirect_url"`
}

func (h *Handler) createMonthly(w http.ResponseWriter, r *http.Request) {
	var p createMonthlyPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	result, subID, err := h.svc.CreateMonthlySubscription(r.Context(), service.CreateMonthlySubscriptionRequest{
		BaaS:          checkoutdomain.BaaS(p.BaaS),
		CustomerName:  p.CustomerName,
		CustomerTaxID: p.CustomerTaxID,
		CustomerEmail: p.CustomerEmail,
		Amount:        checkoutdomain.Money{AmountCents: p.AmountCents, Currency: p.Currency},
		RedirectURL:   p.RedirectURL,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"subscription_id": subID.String(),
		"checkout_id":     result.ProviderCheckoutID,
		"checkout_url":    result.CheckoutURL,
	})
}

type createAnnualInstallmentPayload struct {
	BaaS          string `json:"baas"`
	CustomerName  string `json:"customer_name"`
	CustomerTaxID string `json:"customer_tax_id"`
	CustomerEmail string `json:"customer_email"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	Installments  int    `json:"installments"`
	RedirectURL   string `json:"redirect_url"`
}

func (h *Handler) createAnnualInstallment(w http.ResponseWriter, r *http.Request) {
	var p createAnnualInstallmentPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	result, subID, err := h.svc.CreateAnnualInstallment(r.Context(), service.CreateAnnualInstallmentRequest{
		BaaS:          checkoutdomain.BaaS(p.BaaS),
		CustomerName:  p.CustomerName,
		CustomerTaxID: p.CustomerTaxID,
		CustomerEmail: p.CustomerEmail,
		Amount:        checkoutdomain.Money{AmountCents: p.AmountCents, Currency: p.Currency},
		Installments:  p.Installments,
		RedirectURL:   p.RedirectURL,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"subscription_id": subID.String(),
		"checkout_id":     result.ProviderCheckoutID,
		"checkout_url":    result.CheckoutURL,
	})
}

type createAnnualCheckoutPayload struct {
	BaaS          string `json:"baas"`
	CustomerName  string `json:"customer_name"`
	CustomerTaxID string `json:"customer_tax_id"`
	CustomerEmail string `json:"customer_email"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	RedirectURL   string `json:"redirect_url"`
}

// createAnnualCheckout handles POST /v1/subscriptions/annual.
// Designed for InfinitePay: no installments or card in the payload — the
// buyer chooses payment method (Pix, card up to 12x) on the hosted page.
func (h *Handler) createAnnualCheckout(w http.ResponseWriter, r *http.Request) {
	var p createAnnualCheckoutPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	result, subID, err := h.svc.CreateAnnualCheckout(r.Context(), service.CreateAnnualCheckoutRequest{
		BaaS:          checkoutdomain.BaaS(p.BaaS),
		CustomerName:  p.CustomerName,
		CustomerTaxID: p.CustomerTaxID,
		CustomerEmail: p.CustomerEmail,
		Amount:        checkoutdomain.Money{AmountCents: p.AmountCents, Currency: p.Currency},
		RedirectURL:   p.RedirectURL,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"subscription_id": subID.String(),
		"checkout_id":     result.ProviderCheckoutID,
		"checkout_url":    result.CheckoutURL,
	})
}

// getMonthlyPlanLink handles GET /v1/subscriptions/infinitepay/plans/monthly.
// Returns the pre-created InfinitePay plan link from config. No API call is
// made — the link is static (created once in the InfinitePay app/dashboard).
func (h *Handler) getMonthlyPlanLink(w http.ResponseWriter, r *http.Request) {
	if h.monthlyPlanURL == "" {
		writeError(w, http.StatusServiceUnavailable, "monthly plan not configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": h.monthlyPlanURL})
}

type cancelPayload struct {
	Reason             string `json:"reason"`
	RefundCurrentCycle bool   `json:"refund_current_cycle"`
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid subscription id")
		return
	}

	var p cancelPayload
	_ = json.NewDecoder(r.Body).Decode(&p) // body is optional

	err = h.svc.CancelSubscription(r.Context(), service.CancelSubscriptionRequest{
		SubscriptionID:     id,
		Reason:             p.Reason,
		RefundCurrentCycle: p.RefundCurrentCycle,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
