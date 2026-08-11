package domain

import (
	"testing"
	"time"
)

func TestCurrentPeriodEnd_AnnualPaid_ReturnsOneYearLater(t *testing.T) {
	paidAt := time.Date(2026, 8, 8, 20, 3, 34, 0, time.UTC)
	sub := Subscription{Frequency: FrequencyAnnual, PaidAt: &paidAt}

	got := sub.CurrentPeriodEnd()

	if got == nil {
		t.Fatal("CurrentPeriodEnd() = nil, want non-nil")
	}
	want := time.Date(2027, 8, 8, 20, 3, 34, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("CurrentPeriodEnd() = %v, want %v", got, want)
	}
}

func TestCurrentPeriodEnd_AnnualNeverPaid_ReturnsNil(t *testing.T) {
	sub := Subscription{Frequency: FrequencyAnnual, PaidAt: nil}

	if got := sub.CurrentPeriodEnd(); got != nil {
		t.Errorf("CurrentPeriodEnd() = %v, want nil (never paid)", got)
	}
}

// Monthly subscriptions bill on NextChargeDate — CurrentPeriodEnd is
// meaningless there even if PaidAt were somehow set.
func TestCurrentPeriodEnd_Monthly_ReturnsNil(t *testing.T) {
	paidAt := time.Now()
	sub := Subscription{Frequency: FrequencyMonthly, PaidAt: &paidAt}

	if got := sub.CurrentPeriodEnd(); got != nil {
		t.Errorf("CurrentPeriodEnd() = %v, want nil (monthly)", got)
	}
}
