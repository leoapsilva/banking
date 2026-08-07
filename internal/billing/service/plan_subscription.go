package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/upwifi/banking/internal/billing/domain"
	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
)

// CreateFromPlanRequest creates a subscription priced by the catalogue rather
// than by an amount supplied by the caller. This is the contract API clients
// should use: they name a plan and optionally a coupon, and billing decides
// what to charge.
type CreateFromPlanRequest struct {
	BaaS          checkoutdomain.BaaS
	CustomerName  string
	CustomerTaxID string
	CustomerEmail string
	PlanCode      string
	CouponCode    string // optional
	RedirectURL   string
}

// CreateFromPlanResult carries both the charged and the list amount so the
// caller can show "de X por Y" without recomputing anything.
type CreateFromPlanResult struct {
	Checkout        checkoutdomain.CheckoutResult
	SubscriptionID  uuid.UUID
	AmountCents     int64
	ListAmountCents int64
	Currency        string
	DiscountApplied string
}

// CreateFromPlan resolves the price for a plan/coupon pair and opens the
// corresponding subscription.
//
// Pricing errors (unknown plan, invalid coupon) are returned as the sentinel
// errors from billing/domain so the handler can answer 422 instead of 502.
func (s *Service) CreateFromPlan(ctx context.Context, req CreateFromPlanRequest) (CreateFromPlanResult, error) {
	plan, err := s.repo.GetPlan(ctx, req.PlanCode)
	if err != nil {
		return CreateFromPlanResult{}, err
	}

	var coupon *domain.Coupon
	if req.CouponCode != "" {
		c, err := s.repo.GetCoupon(ctx, req.CouponCode)
		if err != nil {
			return CreateFromPlanResult{}, err
		}
		coupon = &c
	}

	priced, err := domain.ResolvePrice(plan, coupon, time.Now())
	if err != nil {
		return CreateFromPlanResult{}, err
	}

	// The frequency of the plan decides the shape of the subscription: an
	// annual plan is a single hosted checkout, a monthly one needs a saved
	// card for the recurring cycle.
	var (
		result checkoutdomain.CheckoutResult
		subID  uuid.UUID
	)
	switch plan.Frequency {
	case domain.FrequencyAnnual:
		result, subID, err = s.CreateAnnualCheckout(ctx, CreateAnnualCheckoutRequest{
			BaaS:          req.BaaS,
			CustomerName:  req.CustomerName,
			CustomerTaxID: req.CustomerTaxID,
			CustomerEmail: req.CustomerEmail,
			Amount:        priced.Amount,
			Description:   plan.Description,
			RedirectURL:   req.RedirectURL,
		})
	case domain.FrequencyMonthly:
		result, subID, err = s.CreateMonthlySubscription(ctx, CreateMonthlySubscriptionRequest{
			BaaS:          req.BaaS,
			CustomerName:  req.CustomerName,
			CustomerTaxID: req.CustomerTaxID,
			CustomerEmail: req.CustomerEmail,
			Amount:        priced.Amount,
			RedirectURL:   req.RedirectURL,
		})
	default:
		return CreateFromPlanResult{}, fmt.Errorf("billing: unsupported plan frequency %q", plan.Frequency)
	}
	if err != nil {
		return CreateFromPlanResult{}, err
	}

	// Record what the subscription was priced under, so a renewal can
	// reproduce the amount and honour the coupon's duration.
	if err := s.repo.SetPlanAndCoupon(ctx, subID, plan.Code, couponCodePtr(coupon)); err != nil {
		slog.Error("billing: could not record plan/coupon on subscription", "error", err, "subscription_id", subID)
	}

	// Redeem only after the checkout exists, so a provider failure does not
	// burn a redemption slot. A lost redemption on a later crash is
	// preferable to charging a coupon that was never used.
	if coupon != nil {
		redeemed, err := s.repo.RedeemCoupon(ctx, coupon.Code)
		if err != nil {
			slog.Error("billing: redeem coupon failed", "error", err, "coupon", coupon.Code)
		} else if !redeemed {
			// Lost a race for the last slot: the charge already carries the
			// discount, so we honour it and record the anomaly.
			slog.Warn("billing: coupon exhausted between validation and redemption",
				"coupon", coupon.Code, "subscription_id", subID)
		}
	}

	return CreateFromPlanResult{
		Checkout:        result,
		SubscriptionID:  subID,
		AmountCents:     priced.Amount.AmountCents,
		ListAmountCents: priced.ListAmount.AmountCents,
		Currency:        priced.Amount.Currency,
		DiscountApplied: priced.DiscountApplied(),
	}, nil
}

func couponCodePtr(c *domain.Coupon) *string {
	if c == nil {
		return nil
	}
	return &c.Code
}

// GetSubscription returns the current state of a subscription. This is what
// lets an API client mirror the status instead of keeping its own state
// machine — without it, the client would have to infer the subscription
// lifecycle from checkout events.
func (s *Service) GetSubscription(ctx context.Context, id uuid.UUID) (domain.Subscription, error) {
	return s.repo.GetByID(ctx, id)
}

// ListPlans returns every active plan, cheapest first, so a storefront can
// render a price list without hardcoding plan codes.
func (s *Service) ListPlans(ctx context.Context) ([]domain.Plan, error) {
	return s.repo.ListActivePlans(ctx)
}

// ValidateCoupon checks a coupon against a plan without creating anything, so
// a storefront can show the discount before the buyer commits.
func (s *Service) ValidateCoupon(ctx context.Context, planCode, couponCode string) (domain.PricedCharge, error) {
	plan, err := s.repo.GetPlan(ctx, planCode)
	if err != nil {
		return domain.PricedCharge{}, err
	}
	coupon, err := s.repo.GetCoupon(ctx, couponCode)
	if err != nil {
		return domain.PricedCharge{}, err
	}
	return domain.ResolvePrice(plan, &coupon, time.Now())
}
