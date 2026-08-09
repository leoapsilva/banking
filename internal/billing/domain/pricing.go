package domain

import (
	"errors"
	"fmt"
	"slices"
	"time"

	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
)

// Errors returned by pricing resolution. Handlers map these to 422 so the
// caller can tell a bad coupon from an infrastructure failure.
var (
	ErrPlanNotFound     = errors.New("billing: plan not found or inactive")
	ErrCouponNotFound   = errors.New("billing: coupon not found")
	ErrCouponExpired    = errors.New("billing: coupon outside its validity window")
	ErrCouponExhausted  = errors.New("billing: coupon redemption limit reached")
	ErrCouponNotForPlan = errors.New("billing: coupon does not apply to this plan frequency")

	// ErrCouponExceedsPlan guards against a fixed-amount coupon worth as much
	// as (or more than) the plan it is applied to — e.g. a R$199,91 discount
	// on a R$99,99 plan.
	//
	// Charging zero silently would be worse than refusing: it hides an
	// operator mistake behind a successful-looking purchase, and providers
	// reject zero-value charges anyway. Free access is a deliberate act, not
	// the accidental result of arithmetic — see grantComplimentary.
	ErrCouponExceedsPlan = errors.New("billing: coupon discount is not smaller than the plan price")

	// ErrCouponCodeExists guards the UNIQUE constraint on billing_coupons.code
	// — surfaced as 409 instead of a generic 502 so a caller generating codes
	// (e.g. a per-signup coupon) can tell "pick another code" from "we're down".
	ErrCouponCodeExists = errors.New("billing: coupon code already exists")
)

// Plan is a priced entry in the catalogue. The amount lives here rather than
// in the API request so that pricing has a single owner.
type Plan struct {
	Code        string
	Description string
	Frequency   Frequency
	Amount      checkoutdomain.Money
	Active      bool
}

// CouponDuration controls whether a discount applies only to the first cycle
// or keeps applying on renewals.
type CouponDuration string

const (
	CouponOnce      CouponDuration = "ONCE"
	CouponRepeating CouponDuration = "REPEATING"
	CouponForever   CouponDuration = "FOREVER"
)

// Coupon is a generic discount rule. Either PercentOff or AmountOffCents is
// set, never both — enforced by a CHECK constraint in the schema.
type Coupon struct {
	Code                  string
	PercentOff            *int
	AmountOffCents        *int64
	ApplicableFrequencies []Frequency
	Duration              CouponDuration
	ValidFrom             *time.Time
	ValidUntil            *time.Time
	MaxRedemptions        *int
	RedemptionsCount      int
}

// Validate reports whether the coupon may be applied to the given plan at the
// given moment. It does not mutate the coupon.
func (c Coupon) Validate(plan Plan, now time.Time) error {
	if c.ValidFrom != nil && now.Before(*c.ValidFrom) {
		return ErrCouponExpired
	}
	if c.ValidUntil != nil && now.After(*c.ValidUntil) {
		return ErrCouponExpired
	}
	if c.MaxRedemptions != nil && c.RedemptionsCount >= *c.MaxRedemptions {
		return ErrCouponExhausted
	}
	if len(c.ApplicableFrequencies) > 0 && !slices.Contains(c.ApplicableFrequencies, plan.Frequency) {
		return ErrCouponNotForPlan
	}
	return nil
}

// apply returns the discounted amount in cents.
//
// It refuses rather than clamps: a discount that wipes out the plan price is
// an operator error (wrong coupon attached to a cheaper plan), and turning it
// into a zero charge would hide that error behind what looks like a
// successful purchase.
func (c Coupon) apply(amountCents int64) (int64, error) {
	var discounted int64
	switch {
	case c.PercentOff != nil:
		// Integer arithmetic on cents: no float rounding drift.
		// PercentOff is capped at 100 by the schema, so this cannot go
		// negative — but 100% still means a zero charge, which is refused
		// below like any other.
		discounted = amountCents - (amountCents * int64(*c.PercentOff) / 100)
	case c.AmountOffCents != nil:
		discounted = amountCents - *c.AmountOffCents
	default:
		return amountCents, nil
	}
	if discounted <= 0 {
		return 0, ErrCouponExceedsPlan
	}
	return discounted, nil
}

// PricedCharge is the outcome of resolving a plan (and optional coupon) into
// the amount to send to the PSP.
type PricedCharge struct {
	Plan       Plan
	Coupon     *Coupon
	Amount     checkoutdomain.Money // amount actually charged now
	ListAmount checkoutdomain.Money // amount without discount, for display
}

// DiscountApplied describes the discount in words, for the API response.
// Empty when no coupon was used.
func (p PricedCharge) DiscountApplied() string {
	if p.Coupon == nil {
		return ""
	}
	switch {
	case p.Coupon.PercentOff != nil:
		return fmt.Sprintf("%d%% de desconto (%s)", *p.Coupon.PercentOff, p.Coupon.Duration)
	case p.Coupon.AmountOffCents != nil:
		return fmt.Sprintf("desconto de %d centavos (%s)", *p.Coupon.AmountOffCents, p.Coupon.Duration)
	}
	return ""
}

// ResolvePrice computes what to charge for a plan, applying the coupon when
// one is supplied and valid. A nil coupon means list price.
func ResolvePrice(plan Plan, coupon *Coupon, now time.Time) (PricedCharge, error) {
	if !plan.Active {
		return PricedCharge{}, ErrPlanNotFound
	}
	priced := PricedCharge{
		Plan:       plan,
		Amount:     plan.Amount,
		ListAmount: plan.Amount,
	}
	if coupon == nil {
		return priced, nil
	}
	if err := coupon.Validate(plan, now); err != nil {
		return PricedCharge{}, err
	}
	discounted, err := coupon.apply(plan.Amount.AmountCents)
	if err != nil {
		return PricedCharge{}, err
	}
	priced.Coupon = coupon
	priced.Amount = checkoutdomain.Money{
		AmountCents: discounted,
		Currency:    plan.Amount.Currency,
	}
	return priced, nil
}

// RenewalAmount is what the next cycle costs. A ONCE coupon stops applying
// after the first charge; REPEATING/FOREVER keep applying.
//
// This is the distinction that makes "50% off the first cycle" behave
// correctly on renewals instead of silently discounting forever.
func (p PricedCharge) RenewalAmount() checkoutdomain.Money {
	if p.Coupon == nil || p.Coupon.Duration == CouponOnce {
		return p.ListAmount
	}
	return p.Amount
}
