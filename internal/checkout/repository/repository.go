package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/upwifi/banking/internal/checkout/domain"
)

// Repository persists checkouts and the card tokens captured from them.
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Checkout is the persisted row shape, distinct from domain.CheckoutResult
// because it also tracks our own bookkeeping fields.
type Checkout struct {
	ID                  uuid.UUID
	BaaS                domain.BaaS
	ProviderCheckoutID  string
	ExternalReferenceID string
	Amount              domain.Money
	Status              domain.Status
	CheckoutURL         string
	PayerTaxID          string
	PayerName           string
}

func (r *Repository) Create(ctx context.Context, c Checkout, rawResponse any) (uuid.UUID, error) {
	raw, err := json.Marshal(rawResponse)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = r.pool.QueryRow(ctx, `
		INSERT INTO checkouts (baas, provider_checkout_id, external_reference_id, amount_cents,
			currency, status, checkout_url, payer_tax_id, payer_name, raw_create_response)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id
	`, c.BaaS, c.ProviderCheckoutID, c.ExternalReferenceID, c.Amount.AmountCents,
		c.Amount.Currency, c.Status, c.CheckoutURL, c.PayerTaxID, c.PayerName, raw,
	).Scan(&id)
	return id, err
}

func (r *Repository) UpdateStatus(ctx context.Context, baas domain.BaaS, providerCheckoutID string, status domain.Status) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE checkouts SET status = $1, updated_at = now()
		WHERE baas = $2 AND provider_checkout_id = $3
	`, status, baas, providerCheckoutID)
	return err
}

func (r *Repository) GetByProviderID(ctx context.Context, baas domain.BaaS, providerCheckoutID string) (Checkout, error) {
	var c Checkout
	err := r.pool.QueryRow(ctx, `
		SELECT id, baas, provider_checkout_id, COALESCE(external_reference_id, ''),
			amount_cents, currency, status, COALESCE(checkout_url, ''),
			COALESCE(payer_tax_id, ''), COALESCE(payer_name, '')
		FROM checkouts WHERE baas = $1 AND provider_checkout_id = $2
	`, baas, providerCheckoutID).Scan(
		&c.ID, &c.BaaS, &c.ProviderCheckoutID, &c.ExternalReferenceID,
		&c.Amount.AmountCents, &c.Amount.Currency, &c.Status, &c.CheckoutURL,
		&c.PayerTaxID, &c.PayerName,
	)
	if err == pgx.ErrNoRows {
		return Checkout{}, ErrNotFound
	}
	return c, err
}

// CardToken is a saved card token captured from a successful save_card
// checkout, reusable for subsequent recurring/parceled charges.
type CardToken struct {
	ID               uuid.UUID
	BaaS             domain.BaaS
	ProviderToken    string
	CustomerTaxID    string
	SourceCheckoutID uuid.UUID
}

func (r *Repository) SaveCardToken(ctx context.Context, t CardToken) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO card_tokens (baas, provider_token, customer_tax_id, source_checkout_id)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (baas, provider_token) DO UPDATE SET provider_token = EXCLUDED.provider_token
		RETURNING id
	`, t.BaaS, t.ProviderToken, t.CustomerTaxID, t.SourceCheckoutID).Scan(&id)
	return id, err
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (Checkout, error) {
	var c Checkout
	err := r.pool.QueryRow(ctx, `
		SELECT id, baas, provider_checkout_id, COALESCE(external_reference_id, ''),
			amount_cents, currency, status, COALESCE(checkout_url, ''),
			COALESCE(payer_tax_id, ''), COALESCE(payer_name, '')
		FROM checkouts WHERE id = $1
	`, id).Scan(
		&c.ID, &c.BaaS, &c.ProviderCheckoutID, &c.ExternalReferenceID,
		&c.Amount.AmountCents, &c.Amount.Currency, &c.Status, &c.CheckoutURL,
		&c.PayerTaxID, &c.PayerName,
	)
	if err == pgx.ErrNoRows {
		return Checkout{}, ErrNotFound
	}
	return c, err
}

// GetCardTokenValue returns the raw provider token value for a saved card,
// used by the subscription cron worker to authorize recurring charges.
func (r *Repository) GetCardTokenValue(ctx context.Context, id uuid.UUID) (string, error) {
	var token string
	err := r.pool.QueryRow(ctx, `SELECT provider_token FROM card_tokens WHERE id = $1`, id).Scan(&token)
	if err == pgx.ErrNoRows {
		return "", ErrNotFound
	}
	return token, err
}

var ErrNotFound = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "checkout: not found" }
