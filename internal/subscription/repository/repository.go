package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/subscription/domain"
)

var ErrNotFound = errors.New("subscription: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, s domain.Subscription) (uuid.UUID, error) {
	var id uuid.UUID
	var interestType *string
	if s.InterestType != nil {
		v := string(*s.InterestType)
		interestType = &v
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO subscriptions (baas, customer_tax_id, customer_name, customer_email,
			amount_cents, currency, frequency, installments_total, interest_type, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id
	`, s.BaaS, s.CustomerTaxID, s.CustomerName, s.CustomerEmail,
		s.Amount.AmountCents, s.Amount.Currency, s.Frequency, s.InstallmentsTotal, interestType, s.Status,
	).Scan(&id)
	return id, err
}

func (r *Repository) SetInitialCheckout(ctx context.Context, id, checkoutID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE subscriptions SET initial_checkout_id = $1, updated_at = now() WHERE id = $2`, checkoutID, id)
	return err
}

func (r *Repository) ActivateWithToken(ctx context.Context, id, cardTokenID uuid.UUID, nextChargeDate *time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subscriptions
		SET status = $1, card_token_id = $2, next_charge_date = $3, updated_at = now()
		WHERE id = $4
	`, domain.StatusActive, cardTokenID, nextChargeDate, id)
	return err
}

func (r *Repository) Complete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subscriptions SET status = $1, next_charge_date = NULL, updated_at = now() WHERE id = $2
	`, domain.StatusCompleted, id)
	return err
}

func (r *Repository) Cancel(ctx context.Context, id uuid.UUID, reason string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subscriptions
		SET status = $1, cancelled_at = now(), cancelled_reason = $2, updated_at = now()
		WHERE id = $3
	`, domain.StatusCancelled, reason, id)
	return err
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (domain.Subscription, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT s.id, s.baas, s.customer_tax_id, s.customer_name, COALESCE(s.customer_email,''),
			s.amount_cents, s.currency, s.frequency, s.installments_total, s.installments_done,
			s.interest_type, s.card_token_id, s.initial_checkout_id, s.status, s.next_charge_date,
			s.consecutive_failures
		FROM subscriptions s WHERE s.id = $1
	`, id)
	s, err := scanSubscription(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, ErrNotFound
	}
	return s, err
}

func scanSubscription(row pgx.Row) (domain.Subscription, error) {
	var s domain.Subscription
	var interestType *string
	err := row.Scan(
		&s.ID, &s.BaaS, &s.CustomerTaxID, &s.CustomerName, &s.CustomerEmail,
		&s.Amount.AmountCents, &s.Amount.Currency, &s.Frequency, &s.InstallmentsTotal, &s.InstallmentsDone,
		&interestType, &s.CardTokenID, &s.InitialCheckoutID, &s.Status, &s.NextChargeDate,
		&s.ConsecutiveFailures,
	)
	if interestType != nil {
		it := checkoutdomain.InterestType(*interestType)
		s.InterestType = &it
	}
	return s, err
}

// FindByInitialCheckoutID looks up the subscription created alongside a
// given initial checkout, used when reconciling checkout webhook events.
func (r *Repository) FindByInitialCheckoutID(ctx context.Context, checkoutID uuid.UUID) (*domain.Subscription, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, baas, customer_tax_id, customer_name, COALESCE(customer_email,''),
			amount_cents, currency, frequency, installments_total, installments_done,
			interest_type, card_token_id, initial_checkout_id, status, next_charge_date,
			consecutive_failures
		FROM subscriptions WHERE initial_checkout_id = $1
	`, checkoutID)
	s, err := scanSubscription(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// DueForCharge returns ACTIVE subscriptions whose next_charge_date has
// arrived, locking the rows (FOR UPDATE SKIP LOCKED) so multiple worker
// instances could safely run this query concurrently in the future even
// though today there's only one.
func (r *Repository) DueForCharge(ctx context.Context, now time.Time) ([]domain.Subscription, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, baas, customer_tax_id, customer_name, COALESCE(customer_email,''),
			amount_cents, currency, frequency, installments_total, installments_done,
			interest_type, card_token_id, initial_checkout_id, status, next_charge_date,
			consecutive_failures
		FROM subscriptions
		WHERE status = $1 AND next_charge_date <= $2
		FOR UPDATE SKIP LOCKED
	`, domain.StatusActive, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.Subscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *Repository) RecordChargeAttempt(ctx context.Context, subscriptionID uuid.UUID, cycleDate time.Time, externalReferenceID string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO subscription_charges (subscription_id, cycle_date, external_reference_id, status)
		VALUES ($1, $2, $3, 'IN_PROGRESS')
		ON CONFLICT (subscription_id, cycle_date) DO NOTHING
		RETURNING id
	`, subscriptionID, cycleDate, externalReferenceID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil // already attempted this cycle; caller should skip
	}
	return id, err
}

func (r *Repository) MarkChargeResult(ctx context.Context, chargeID uuid.UUID, status, responseCode, message string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subscription_charges
		SET status = $1, provider_response_code = $2, provider_message = $3, attempted_at = now()
		WHERE id = $4
	`, status, responseCode, message, chargeID)
	return err
}

func (r *Repository) AdvanceNextChargeDate(ctx context.Context, id uuid.UUID, next time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subscriptions
		SET next_charge_date = $1, consecutive_failures = 0, updated_at = now()
		WHERE id = $2
	`, next, id)
	return err
}

// RegisterFailure increments the failure counter and, once it reaches
// domain.MaxConsecutiveFailures, marks the subscription PAST_DUE so the
// worker stops retrying it automatically.
func (r *Repository) RegisterFailure(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subscriptions
		SET consecutive_failures = consecutive_failures + 1,
		    status = CASE WHEN consecutive_failures + 1 >= $1 THEN $2 ELSE status END,
		    updated_at = now()
		WHERE id = $3
	`, domain.MaxConsecutiveFailures, domain.StatusPastDue, id)
	return err
}
