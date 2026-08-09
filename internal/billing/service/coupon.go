package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/upwifi/banking/internal/billing/domain"
)

// ErrInvalidCouponRequest reports a malformed CreateCoupon request — a
// caller error (400), distinct from ErrCouponCodeExists (409, the code is
// well-formed but taken) and infra failures (502).
var ErrInvalidCouponRequest = errors.New("billing: invalid coupon request")

// CreateCouponRequest mirrors domain.Coupon's fields the caller may set.
// RedemptionsCount is deliberately absent — a coupon is always created
// unused.
type CreateCouponRequest struct {
	Code                  string
	PercentOff            *int
	AmountOffCents        *int64
	ApplicableFrequencies []domain.Frequency
	Duration              domain.CouponDuration
	ValidFrom             *time.Time
	ValidUntil            *time.Time
	MaxRedemptions        *int
}

// CreateCoupon registers a new discount code. Used both for hand-cadastrated
// campaign coupons and for codes generated per event (e.g. one per signup,
// see Cores' welcome-coupon email) — either way, this is the single place
// that enforces the coupon is well-formed before it reaches the database.
func (s *Service) CreateCoupon(ctx context.Context, req CreateCouponRequest) error {
	if req.Code == "" {
		return fmt.Errorf("%w: code is required", ErrInvalidCouponRequest)
	}
	if req.PercentOff == nil && req.AmountOffCents == nil {
		return fmt.Errorf("%w: percent_off or amount_off_cents is required", ErrInvalidCouponRequest)
	}
	if req.PercentOff != nil && req.AmountOffCents != nil {
		return fmt.Errorf("%w: percent_off and amount_off_cents are mutually exclusive", ErrInvalidCouponRequest)
	}
	if req.PercentOff != nil && (*req.PercentOff <= 0 || *req.PercentOff > 100) {
		return fmt.Errorf("%w: percent_off must be between 1 and 100", ErrInvalidCouponRequest)
	}
	if req.AmountOffCents != nil && *req.AmountOffCents <= 0 {
		return fmt.Errorf("%w: amount_off_cents must be positive", ErrInvalidCouponRequest)
	}
	if req.MaxRedemptions != nil && *req.MaxRedemptions <= 0 {
		return fmt.Errorf("%w: max_redemptions must be positive", ErrInvalidCouponRequest)
	}

	duration := req.Duration
	if duration == "" {
		duration = domain.CouponOnce
	}
	if duration != domain.CouponOnce && duration != domain.CouponRepeating && duration != domain.CouponForever {
		return fmt.Errorf("%w: duration must be ONCE, REPEATING or FOREVER", ErrInvalidCouponRequest)
	}

	return s.repo.CreateCoupon(ctx, domain.Coupon{
		Code:                  strings.ToUpper(req.Code),
		PercentOff:            req.PercentOff,
		AmountOffCents:        req.AmountOffCents,
		ApplicableFrequencies: req.ApplicableFrequencies,
		Duration:              duration,
		ValidFrom:             req.ValidFrom,
		ValidUntil:            req.ValidUntil,
		MaxRedemptions:        req.MaxRedemptions,
	})
}
