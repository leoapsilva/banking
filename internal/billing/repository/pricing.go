package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/upwifi/banking/internal/billing/domain"
)

// GetPlan loads an active plan by its code. Inactive or unknown codes return
// domain.ErrPlanNotFound so the handler can answer 422 rather than 500.
func (r *Repository) GetPlan(ctx context.Context, code string) (domain.Plan, error) {
	const q = `
		SELECT code, description, frequency, amount_cents, currency, active
		FROM billing_plans
		WHERE code = $1 AND active`

	var p domain.Plan
	var freq string
	err := r.pool.QueryRow(ctx, q, code).Scan(
		&p.Code, &p.Description, &freq, &p.Amount.AmountCents, &p.Amount.Currency, &p.Active,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Plan{}, domain.ErrPlanNotFound
	}
	if err != nil {
		return domain.Plan{}, fmt.Errorf("billing: load plan: %w", err)
	}
	p.Frequency = domain.Frequency(freq)
	return p, nil
}

// ListActivePlans loads every active plan, cheapest first, so a storefront
// can render a price list without knowing plan codes in advance.
func (r *Repository) ListActivePlans(ctx context.Context) ([]domain.Plan, error) {
	const q = `
		SELECT code, description, frequency, amount_cents, currency, active
		FROM billing_plans
		WHERE active
		ORDER BY amount_cents ASC`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("billing: list plans: %w", err)
	}
	defer rows.Close()

	var plans []domain.Plan
	for rows.Next() {
		var p domain.Plan
		var freq string
		if err := rows.Scan(&p.Code, &p.Description, &freq, &p.Amount.AmountCents, &p.Amount.Currency, &p.Active); err != nil {
			return nil, fmt.Errorf("billing: scan plan: %w", err)
		}
		p.Frequency = domain.Frequency(freq)
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("billing: list plans: %w", err)
	}
	return plans, nil
}

// GetCoupon loads a coupon by code. Unknown codes return
// domain.ErrCouponNotFound. Validity is not checked here — that is the
// domain's job (Coupon.Validate), so the rules stay testable without a DB.
func (r *Repository) GetCoupon(ctx context.Context, code string) (domain.Coupon, error) {
	const q = `
		SELECT code, percent_off, amount_off_cents, applicable_frequencies,
		       duration, valid_from, valid_until, max_redemptions, redemptions_count
		FROM billing_coupons
		WHERE code = $1`

	var c domain.Coupon
	var freqs []string
	var duration string
	err := r.pool.QueryRow(ctx, q, code).Scan(
		&c.Code, &c.PercentOff, &c.AmountOffCents, &freqs,
		&duration, &c.ValidFrom, &c.ValidUntil, &c.MaxRedemptions, &c.RedemptionsCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Coupon{}, domain.ErrCouponNotFound
	}
	if err != nil {
		return domain.Coupon{}, fmt.Errorf("billing: load coupon: %w", err)
	}
	c.Duration = domain.CouponDuration(duration)
	for _, f := range freqs {
		c.ApplicableFrequencies = append(c.ApplicableFrequencies, domain.Frequency(f))
	}
	return c, nil
}

// CreateCoupon inserts a new coupon. Returns domain.ErrCouponCodeExists on a
// duplicate code (billing_coupons.code is UNIQUE) instead of a generic error,
// so a caller that generates codes itself (e.g. one per signup) can retry
// with a different code rather than surfacing a 502.
func (r *Repository) CreateCoupon(ctx context.Context, c domain.Coupon) error {
	const q = `
		INSERT INTO billing_coupons (code, percent_off, amount_off_cents, applicable_frequencies, duration, valid_from, valid_until, max_redemptions)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	freqs := make([]string, len(c.ApplicableFrequencies))
	for i, f := range c.ApplicableFrequencies {
		freqs[i] = string(f)
	}

	_, err := r.pool.Exec(ctx, q,
		c.Code, c.PercentOff, c.AmountOffCents, freqs, string(c.Duration),
		c.ValidFrom, c.ValidUntil, c.MaxRedemptions,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrCouponCodeExists
		}
		return fmt.Errorf("billing: create coupon: %w", err)
	}
	return nil
}

// RedeemCoupon increments the redemption counter, refusing to exceed
// max_redemptions. The check lives in the UPDATE predicate so two concurrent
// redemptions of the last available slot cannot both succeed.
//
// Returns false when the coupon was already exhausted.
func (r *Repository) RedeemCoupon(ctx context.Context, code string) (bool, error) {
	const q = `
		UPDATE billing_coupons
		SET redemptions_count = redemptions_count + 1
		WHERE code = $1
		  AND (max_redemptions IS NULL OR redemptions_count < max_redemptions)`

	tag, err := r.pool.Exec(ctx, q, code)
	if err != nil {
		return false, fmt.Errorf("billing: redeem coupon: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// SetPlanAndCoupon records which plan and coupon a subscription was created
// under, so a renewal can reproduce the amount.
func (r *Repository) SetPlanAndCoupon(ctx context.Context, subID any, planCode string, couponCode *string) error {
	const q = `UPDATE subscriptions SET plan_code = $2, coupon_code = $3 WHERE id = $1`
	if _, err := r.pool.Exec(ctx, q, subID, planCode, couponCode); err != nil {
		return fmt.Errorf("billing: set plan/coupon: %w", err)
	}
	return nil
}
