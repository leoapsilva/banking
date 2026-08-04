package domain

import (
	"errors"
	"testing"
	"time"

	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
)

func annualPlan() Plan {
	return Plan{
		Code:      "cores-annual",
		Frequency: FrequencyAnnual,
		Amount:    checkoutdomain.Money{AmountCents: 29990, Currency: "BRL"},
		Active:    true,
	}
}

func ptrInt(v int) *int              { return &v }
func ptrI64(v int64) *int64          { return &v }
func ptrTime(t time.Time) *time.Time { return &t }

func TestResolvePriceWithoutCoupon(t *testing.T) {
	got, err := ResolvePrice(annualPlan(), nil, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Amount.AmountCents != 29990 {
		t.Errorf("amount = %d, want 29990", got.Amount.AmountCents)
	}
	if got.DiscountApplied() != "" {
		t.Errorf("discount = %q, want empty", got.DiscountApplied())
	}
}

func TestResolvePricePercentCoupon(t *testing.T) {
	coupon := &Coupon{
		Code:                  "LANCAMENTO50",
		PercentOff:            ptrInt(50),
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
		Duration:              CouponOnce,
	}
	got, err := ResolvePrice(annualPlan(), coupon, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Amount.AmountCents != 14995 {
		t.Errorf("amount = %d, want 14995", got.Amount.AmountCents)
	}
	if got.ListAmount.AmountCents != 29990 {
		t.Errorf("list = %d, want 29990", got.ListAmount.AmountCents)
	}
}

// A ONCE coupon must not leak into renewals — otherwise a launch discount
// would silently apply forever.
func TestRenewalDropsOnceCoupon(t *testing.T) {
	coupon := &Coupon{
		PercentOff: ptrInt(50), Duration: CouponOnce,
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
	}
	got, _ := ResolvePrice(annualPlan(), coupon, time.Now())
	if renewal := got.RenewalAmount(); renewal.AmountCents != 29990 {
		t.Errorf("renewal = %d, want 29990 (list price)", renewal.AmountCents)
	}
}

func TestRenewalKeepsForeverCoupon(t *testing.T) {
	coupon := &Coupon{
		PercentOff: ptrInt(50), Duration: CouponForever,
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
	}
	got, _ := ResolvePrice(annualPlan(), coupon, time.Now())
	if renewal := got.RenewalAmount(); renewal.AmountCents != 14995 {
		t.Errorf("renewal = %d, want 14995 (discount persists)", renewal.AmountCents)
	}
}

// A discount worth more than the plan is an operator error — a coupon meant
// for an expensive plan attached to a cheap one. Refuse it instead of
// charging zero, which would hide the mistake behind a successful purchase.
func TestFixedCouponLargerThanPlanRejected(t *testing.T) {
	cheapPlan := annualPlan()
	cheapPlan.Amount.AmountCents = 9999 // R$ 99,99

	coupon := &Coupon{
		AmountOffCents: ptrI64(19991), Duration: CouponOnce, // R$ 199,91
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
	}
	_, err := ResolvePrice(cheapPlan, coupon, time.Now())
	if !errors.Is(err, ErrCouponExceedsPlan) {
		t.Errorf("err = %v, want ErrCouponExceedsPlan", err)
	}
}

// Exactly zeroing the plan is refused too: free access must be granted
// deliberately, not fall out of arithmetic.
func TestCouponZeroingPlanRejected(t *testing.T) {
	coupon := &Coupon{
		AmountOffCents: ptrI64(29990), Duration: CouponOnce,
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
	}
	_, err := ResolvePrice(annualPlan(), coupon, time.Now())
	if !errors.Is(err, ErrCouponExceedsPlan) {
		t.Errorf("err = %v, want ErrCouponExceedsPlan", err)
	}
}

func TestHundredPercentCouponRejected(t *testing.T) {
	coupon := &Coupon{
		PercentOff: ptrInt(100), Duration: CouponOnce,
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
	}
	_, err := ResolvePrice(annualPlan(), coupon, time.Now())
	if !errors.Is(err, ErrCouponExceedsPlan) {
		t.Errorf("err = %v, want ErrCouponExceedsPlan (use a courtesy grant instead)", err)
	}
}

// The CORESVIP case: R$ 199,91 off the R$ 299,90 plan → R$ 99,99.
func TestFixedCouponSmallerThanPlanAccepted(t *testing.T) {
	coupon := &Coupon{
		AmountOffCents: ptrI64(19991), Duration: CouponOnce,
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
	}
	got, err := ResolvePrice(annualPlan(), coupon, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Amount.AmountCents != 9999 {
		t.Errorf("amount = %d, want 9999 (R$ 99,99)", got.Amount.AmountCents)
	}
}

func TestCouponRejectedForWrongFrequency(t *testing.T) {
	coupon := &Coupon{
		PercentOff: ptrInt(50), Duration: CouponOnce,
		ApplicableFrequencies: []Frequency{FrequencyMonthly},
	}
	_, err := ResolvePrice(annualPlan(), coupon, time.Now())
	if !errors.Is(err, ErrCouponNotForPlan) {
		t.Errorf("err = %v, want ErrCouponNotForPlan", err)
	}
}

func TestCouponRejectedWhenExpired(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	coupon := &Coupon{
		PercentOff: ptrInt(50), Duration: CouponOnce,
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
		ValidUntil:            ptrTime(past),
	}
	_, err := ResolvePrice(annualPlan(), coupon, time.Now())
	if !errors.Is(err, ErrCouponExpired) {
		t.Errorf("err = %v, want ErrCouponExpired", err)
	}
}

func TestCouponRejectedBeforeValidFrom(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	coupon := &Coupon{
		PercentOff: ptrInt(50), Duration: CouponOnce,
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
		ValidFrom:             ptrTime(future),
	}
	_, err := ResolvePrice(annualPlan(), coupon, time.Now())
	if !errors.Is(err, ErrCouponExpired) {
		t.Errorf("err = %v, want ErrCouponExpired", err)
	}
}

func TestCouponRejectedWhenExhausted(t *testing.T) {
	coupon := &Coupon{
		PercentOff: ptrInt(50), Duration: CouponOnce,
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
		MaxRedemptions:        ptrInt(10),
		RedemptionsCount:      10,
	}
	_, err := ResolvePrice(annualPlan(), coupon, time.Now())
	if !errors.Is(err, ErrCouponExhausted) {
		t.Errorf("err = %v, want ErrCouponExhausted", err)
	}
}

func TestInactivePlanRejected(t *testing.T) {
	p := annualPlan()
	p.Active = false
	_, err := ResolvePrice(p, nil, time.Now())
	if !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("err = %v, want ErrPlanNotFound", err)
	}
}
