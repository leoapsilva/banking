package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/upwifi/banking/internal/billing/domain"
	"github.com/upwifi/banking/internal/billing/service"
	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
)

// createFromPlanPayload is the preferred way to open a subscription: the
// caller names a plan and optionally a coupon, and never sends an amount.
// Pricing has a single owner, and a client cannot charge itself a price we
// did not authorise.
type createFromPlanPayload struct {
	BaaS          string `json:"baas"`
	CustomerName  string `json:"customer_name"`
	CustomerTaxID string `json:"customer_tax_id"`
	CustomerEmail string `json:"customer_email"`
	Plan          string `json:"plan"`
	CouponCode    string `json:"coupon_code"`
	RedirectURL   string `json:"redirect_url"`
}

func (h *Handler) createFromPlan(w http.ResponseWriter, r *http.Request) {
	var p createFromPlanPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if p.Plan == "" {
		writeError(w, http.StatusBadRequest, "plan is required")
		return
	}

	result, err := h.svc.CreateFromPlan(r.Context(), service.CreateFromPlanRequest{
		BaaS:          checkoutdomain.BaaS(p.BaaS),
		CustomerName:  p.CustomerName,
		CustomerTaxID: p.CustomerTaxID,
		CustomerEmail: p.CustomerEmail,
		PlanCode:      p.Plan,
		CouponCode:    p.CouponCode,
		RedirectURL:   p.RedirectURL,
	})
	if err != nil {
		writePricingError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"subscription_id":   result.SubscriptionID.String(),
		"checkout_id":       result.Checkout.ProviderCheckoutID,
		"checkout_url":      result.Checkout.CheckoutURL,
		"amount_cents":      result.AmountCents,
		"list_amount_cents": result.ListAmountCents,
		"currency":          result.Currency,
		"discount_applied":  result.DiscountApplied,
	})
}

// listPlans lets a storefront render a price list without hardcoding plan
// codes — the amount displayed always comes from here, never from the
// caller (RN-02 in the Cores REFINAMENTO: the Cores app never calculates
// price).
func (h *Handler) listPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.ListPlans(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	body := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		body = append(body, map[string]any{
			"code":         p.Code,
			"description":  p.Description,
			"frequency":    string(p.Frequency),
			"amount_cents": p.Amount.AmountCents,
			"currency":     p.Amount.Currency,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": body})
}

// get exposes the subscription state so an API client can mirror it instead
// of maintaining a parallel state machine.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid subscription id")
		return
	}

	sub, err := h.svc.GetSubscription(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}

	body := map[string]any{
		"id":           sub.ID.String(),
		"baas":         string(sub.BaaS),
		"status":       string(sub.Status),
		"frequency":    string(sub.Frequency),
		"amount_cents": sub.Amount.AmountCents,
		"currency":     sub.Amount.Currency,
	}
	if sub.NextChargeDate != nil {
		body["next_charge_date"] = sub.NextChargeDate.Format("2006-01-02")
	}
	writeJSON(w, http.StatusOK, body)
}

// validateCoupon lets a storefront show the discount before the buyer
// commits, without creating anything.
func (h *Handler) validateCoupon(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	plan := r.URL.Query().Get("plan")
	if plan == "" {
		writeError(w, http.StatusBadRequest, "plan query parameter is required")
		return
	}

	priced, err := h.svc.ValidateCoupon(r.Context(), plan, code)
	if err != nil {
		writePricingError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":             true,
		"amount_cents":      priced.Amount.AmountCents,
		"list_amount_cents": priced.ListAmount.AmountCents,
		"currency":          priced.Amount.Currency,
		"discount_applied":  priced.DiscountApplied(),
	})
}

// writePricingError maps pricing rejections to 422 (the request was
// well-formed but the plan/coupon is not usable) and everything else to 502,
// so callers can tell "your coupon is bad" from "our provider is down".
//
// The message is the sentinel's text, which never contains provider detail.
func writePricingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrPlanNotFound),
		errors.Is(err, domain.ErrCouponNotFound),
		errors.Is(err, domain.ErrCouponExpired),
		errors.Is(err, domain.ErrCouponExhausted),
		errors.Is(err, domain.ErrCouponNotForPlan):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusBadGateway, err.Error())
	}
}
