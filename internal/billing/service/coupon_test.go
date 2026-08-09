package service

import (
	"context"
	"errors"
	"testing"

	"github.com/upwifi/banking/internal/billing/domain"
)

func TestCreateCoupon_RejectsMissingCode(t *testing.T) {
	s := &Service{}
	percentOff := 30
	err := s.CreateCoupon(context.Background(), CreateCouponRequest{PercentOff: &percentOff})
	if !errors.Is(err, ErrInvalidCouponRequest) {
		t.Fatalf("expected ErrInvalidCouponRequest, got %v", err)
	}
}

func TestCreateCoupon_RejectsNoDiscount(t *testing.T) {
	s := &Service{}
	err := s.CreateCoupon(context.Background(), CreateCouponRequest{Code: "TESTE30"})
	if !errors.Is(err, ErrInvalidCouponRequest) {
		t.Fatalf("expected ErrInvalidCouponRequest, got %v", err)
	}
}

func TestCreateCoupon_RejectsBothDiscountKinds(t *testing.T) {
	s := &Service{}
	percentOff := 30
	amountOff := int64(1000)
	err := s.CreateCoupon(context.Background(), CreateCouponRequest{
		Code: "TESTE30", PercentOff: &percentOff, AmountOffCents: &amountOff,
	})
	if !errors.Is(err, ErrInvalidCouponRequest) {
		t.Fatalf("expected ErrInvalidCouponRequest, got %v", err)
	}
}

func TestCreateCoupon_RejectsPercentOffOutOfRange(t *testing.T) {
	s := &Service{}
	for _, invalid := range []int{0, -5, 101} {
		invalid := invalid
		err := s.CreateCoupon(context.Background(), CreateCouponRequest{Code: "TESTE30", PercentOff: &invalid})
		if !errors.Is(err, ErrInvalidCouponRequest) {
			t.Fatalf("percent_off=%d: expected ErrInvalidCouponRequest, got %v", invalid, err)
		}
	}
}

func TestCreateCoupon_RejectsNonPositiveAmountOff(t *testing.T) {
	s := &Service{}
	amountOff := int64(0)
	err := s.CreateCoupon(context.Background(), CreateCouponRequest{Code: "TESTE30", AmountOffCents: &amountOff})
	if !errors.Is(err, ErrInvalidCouponRequest) {
		t.Fatalf("expected ErrInvalidCouponRequest, got %v", err)
	}
}

func TestCreateCoupon_RejectsNonPositiveMaxRedemptions(t *testing.T) {
	s := &Service{}
	percentOff := 30
	maxRedemptions := 0
	err := s.CreateCoupon(context.Background(), CreateCouponRequest{
		Code: "TESTE30", PercentOff: &percentOff, MaxRedemptions: &maxRedemptions,
	})
	if !errors.Is(err, ErrInvalidCouponRequest) {
		t.Fatalf("expected ErrInvalidCouponRequest, got %v", err)
	}
}

func TestCreateCoupon_RejectsInvalidDuration(t *testing.T) {
	s := &Service{}
	percentOff := 30
	err := s.CreateCoupon(context.Background(), CreateCouponRequest{
		Code: "TESTE30", PercentOff: &percentOff, Duration: domain.CouponDuration("WHENEVER"),
	})
	if !errors.Is(err, ErrInvalidCouponRequest) {
		t.Fatalf("expected ErrInvalidCouponRequest, got %v", err)
	}
}
