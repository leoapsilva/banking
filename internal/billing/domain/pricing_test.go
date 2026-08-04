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

func TestFixedAmountCouponNeverGoesNegative(t *testing.T) {
	coupon := &Coupon{
		AmountOffCents: ptrI64(99999), Duration: CouponOnce,
		ApplicableFrequencies: []Frequency{FrequencyAnnual},
	}
	got, err := ResolvePrice(annualPlan(), coupon, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Amount.AmountCents != 0 {
		t.Errorf("amount = %d, want 0 (clamped)", got.Amount.AmountCents)
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
