package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/upwifi/banking/internal/pix/domain"
)

// Repository persists PIX charges locally for history/audit purposes — the
// source of truth for status is always C6, mirroring the checkout feature's
// pattern (internal/checkout/repository).
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Charge is the persisted row shape.
type Charge struct {
	ID                uuid.UUID
	BaaS              domain.BaaS
	TxID              string
	Kind              domain.ChargeKind
	Amount            domain.Money
	PixKey            string
	Status            domain.Status
	Revision          int
	Location          string
	ExpirationSeconds *int
	DueDate           *time.Time
	Debtor            *domain.Debtor
	PayerRequest      string
}

func (r *Repository) Create(ctx context.Context, c Charge, rawResponse any) (uuid.UUID, error) {
	raw, err := json.Marshal(rawResponse)
	if err != nil {
		return uuid.Nil, err
	}

	var debtorDocType, debtorDocID, debtorName *string
	if c.Debtor != nil {
		dt := string(c.Debtor.DocumentType)
		debtorDocType = &dt
		debtorDocID = &c.Debtor.DocumentID
		debtorName = &c.Debtor.Name
	}

	var id uuid.UUID
	err = r.pool.QueryRow(ctx, `
		INSERT INTO pix_charges (baas, txid, kind, amount_cents, pix_key, status, revision,
			location, expiration_seconds, due_date, debtor_document_type, debtor_document_id,
			debtor_name, payer_request, raw_create_response)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (baas, txid) DO UPDATE SET
			status = EXCLUDED.status, revision = EXCLUDED.revision, location = EXCLUDED.location,
			updated_at = now()
		RETURNING id
	`, c.BaaS, c.TxID, c.Kind, c.Amount.AmountCents, c.PixKey, c.Status, c.Revision,
		nullableString(c.Location), c.ExpirationSeconds, c.DueDate, debtorDocType, debtorDocID,
		debtorName, nullableString(c.PayerRequest), raw,
	).Scan(&id)
	return id, err
}

func (r *Repository) UpdateStatus(ctx context.Context, baas domain.BaaS, txID string, status domain.Status, revision int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE pix_charges SET status = $1, revision = $2, updated_at = now()
		WHERE baas = $3 AND txid = $4
	`, status, revision, baas, txID)
	return err
}

func (r *Repository) GetByTxID(ctx context.Context, baas domain.BaaS, txID string) (Charge, error) {
	var c Charge
	var debtorDocType, debtorDocID, debtorName *string
	var expirationSeconds *int
	err := r.pool.QueryRow(ctx, `
		SELECT id, baas, txid, kind, amount_cents, pix_key, status, revision,
			COALESCE(location, ''), expiration_seconds, due_date,
			debtor_document_type, debtor_document_id, debtor_name, COALESCE(payer_request, '')
		FROM pix_charges WHERE baas = $1 AND txid = $2
	`, baas, txID).Scan(
		&c.ID, &c.BaaS, &c.TxID, &c.Kind, &c.Amount.AmountCents, &c.PixKey, &c.Status, &c.Revision,
		&c.Location, &expirationSeconds, &c.DueDate,
		&debtorDocType, &debtorDocID, &debtorName, &c.PayerRequest,
	)
	if err == pgx.ErrNoRows {
		return Charge{}, ErrNotFound
	}
	if err != nil {
		return Charge{}, err
	}
	c.ExpirationSeconds = expirationSeconds
	if debtorDocType != nil {
		c.Debtor = &domain.Debtor{DocumentType: domain.DebtorDocumentType(*debtorDocType)}
		if debtorDocID != nil {
			c.Debtor.DocumentID = *debtorDocID
		}
		if debtorName != nil {
			c.Debtor.Name = *debtorName
		}
	}
	return c, nil
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

var ErrNotFound = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "pix: not found" }
