package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/upwifi/banking/internal/billing/domain"
	"github.com/upwifi/banking/internal/billing/service"
)

// createCouponPayload accepts either PercentOff or AmountOffCents, mirroring
// the mutually-exclusive discount kinds enforced by the billing_coupons
// schema (see migration 0006). ValidUntil/ValidFrom are RFC3339 to match
// every other timestamp this API already returns.
type createCouponPayload struct {
	Code                  string   `json:"code"`
	PercentOff            *int     `json:"percent_off"`
	AmountOffCents        *int64   `json:"amount_off_cents"`
	ApplicableFrequencies []string `json:"applicable_frequencies"`
	Duration              string   `json:"duration"`
	ValidFrom             *string  `json:"valid_from"`
	ValidUntil            *string  `json:"valid_until"`
	MaxRedemptions        *int     `json:"max_redemptions"`
}

// createCoupon handles POST /v1/coupons — registers a new discount code.
// Same X-API-Key authentication as every other endpoint in this API; there
// is no separate "admin" tier today (only Cores holds a key).
func (h *Handler) createCoupon(w http.ResponseWriter, r *http.Request) {
	var p createCouponPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	validFrom, err := parseOptionalRFC3339(p.ValidFrom)
	if err != nil {
		writeError(w, http.StatusBadRequest, "valid_from must be RFC3339")
		return
	}
	validUntil, err := parseOptionalRFC3339(p.ValidUntil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "valid_until must be RFC3339")
		return
	}

	freqs := make([]domain.Frequency, len(p.ApplicableFrequencies))
	for i, f := range p.ApplicableFrequencies {
		freqs[i] = domain.Frequency(f)
	}

	err = h.svc.CreateCoupon(r.Context(), service.CreateCouponRequest{
		Code:                  p.Code,
		PercentOff:            p.PercentOff,
		AmountOffCents:        p.AmountOffCents,
		ApplicableFrequencies: freqs,
		Duration:              domain.CouponDuration(p.Duration),
		ValidFrom:             validFrom,
		ValidUntil:            validUntil,
		MaxRedemptions:        p.MaxRedemptions,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCouponRequest):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, domain.ErrCouponCodeExists):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusBadGateway, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"code": strings.ToUpper(p.Code)})
}

func parseOptionalRFC3339(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
