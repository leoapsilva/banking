package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/upwifi/banking/internal/boleto/domain"
)

// Repository persists bank slips issued through the unified API.
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// BankSlip is the persisted row shape, distinct from domain.BankSlipResult
// because it also tracks our own bookkeeping fields.
type BankSlip struct {
	ID                  uuid.UUID
	BaaS                domain.BaaS
	ProviderBankSlipID  string
	ExternalReferenceID string
	Amount              domain.Money
	Status              domain.Status
	DueDate             time.Time
	PayerTaxID          string
	PayerName           string
}

func (r *Repository) Create(ctx context.Context, b BankSlip, rawResponse any) (uuid.UUID, error) {
	raw, err := json.Marshal(rawResponse)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = r.pool.QueryRow(ctx, `
		INSERT INTO bank_slips (baas, provider_bank_slip_id, external_reference_id, amount_cents,
			currency, status, due_date, payer_tax_id, payer_name, raw_create_response)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id
	`, b.BaaS, b.ProviderBankSlipID, b.ExternalReferenceID, b.Amount.AmountCents,
		b.Amount.Currency, b.Status, b.DueDate, b.PayerTaxID, b.PayerName, raw,
	).Scan(&id)
	return id, err
}

func (r *Repository) UpdateStatus(ctx context.Context, baas domain.BaaS, providerBankSlipID string, status domain.Status) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE bank_slips SET status = $1, updated_at = now()
		WHERE baas = $2 AND provider_bank_slip_id = $3
	`, status, baas, providerBankSlipID)
	return err
}

func (r *Repository) GetByProviderID(ctx context.Context, baas domain.BaaS, providerBankSlipID string) (BankSlip, error) {
	var b BankSlip
	err := r.pool.QueryRow(ctx, `
		SELECT id, baas, provider_bank_slip_id, COALESCE(external_reference_id, ''),
			amount_cents, currency, status, due_date,
			COALESCE(payer_tax_id, ''), COALESCE(payer_name, '')
		FROM bank_slips WHERE baas = $1 AND provider_bank_slip_id = $2
	`, baas, providerBankSlipID).Scan(
		&b.ID, &b.BaaS, &b.ProviderBankSlipID, &b.ExternalReferenceID,
		&b.Amount.AmountCents, &b.Amount.Currency, &b.Status, &b.DueDate,
		&b.PayerTaxID, &b.PayerName,
	)
	if err == pgx.ErrNoRows {
		return BankSlip{}, ErrNotFound
	}
	return b, err
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (BankSlip, error) {
	var b BankSlip
	err := r.pool.QueryRow(ctx, `
		SELECT id, baas, provider_bank_slip_id, COALESCE(external_reference_id, ''),
			amount_cents, currency, status, due_date,
			COALESCE(payer_tax_id, ''), COALESCE(payer_name, '')
		FROM bank_slips WHERE id = $1
	`, id).Scan(
		&b.ID, &b.BaaS, &b.ProviderBankSlipID, &b.ExternalReferenceID,
		&b.Amount.AmountCents, &b.Amount.Currency, &b.Status, &b.DueDate,
		&b.PayerTaxID, &b.PayerName,
	)
	if err == pgx.ErrNoRows {
		return BankSlip{}, ErrNotFound
	}
	return b, err
}

var ErrNotFound = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "boleto: not found" }
