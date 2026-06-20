package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/upwifi/banking/internal/paymentscheduling/domain"
)

// Repository persists submitted payment groups/items locally for our own
// audit trail. C6 is always the source of truth for status; these rows
// exist so we have history even after a group is approved/settled.
//
// POST /decode only returns a group_id — individual item ids and
// read-only fields (status, due_date, etc.) only become known once we
// fetch GET /{group_id}/items, so items are populated exclusively via
// SyncItems rather than guessed at submission time.
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) SaveGroup(ctx context.Context, baas domain.BaaS, providerGroupID, uploaderName string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO payment_groups (baas, provider_group_id, uploader_name, status)
		VALUES ($1, $2, $3, 'SUBMITTED')
		ON CONFLICT (baas, provider_group_id) DO UPDATE SET provider_group_id = EXCLUDED.provider_group_id
		RETURNING id
	`, baas, providerGroupID, uploaderName).Scan(&id)
	return id, err
}

func (r *Repository) UpdateGroupStatus(ctx context.Context, baas domain.BaaS, providerGroupID, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE payment_groups SET status = $1, updated_at = now() WHERE baas = $2 AND provider_group_id = $3
	`, status, baas, providerGroupID)
	return err
}

// SyncItems upserts our local copy of a group's items with the
// authoritative list just fetched from C6 (GET /{group_id}/items), keyed
// on the provider's own item id.
func (r *Repository) SyncItems(ctx context.Context, baas domain.BaaS, providerGroupID string, items []domain.PaymentItem) error {
	var groupRowID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM payment_groups WHERE baas = $1 AND provider_group_id = $2
	`, baas, providerGroupID).Scan(&groupRowID)
	if err != nil {
		return err
	}

	for _, item := range items {
		_, err := r.pool.Exec(ctx, `
			INSERT INTO payment_group_items (group_id, provider_item_id, content, amount_cents,
				bank_code, bank_name, beneficiary_name, description, payer_name, due_date,
				transaction_date, overdue, product_type, status, error_message, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now())
			ON CONFLICT (group_id, provider_item_id) DO UPDATE SET
				status = EXCLUDED.status,
				due_date = EXCLUDED.due_date,
				overdue = EXCLUDED.overdue,
				product_type = EXCLUDED.product_type,
				error_message = EXCLUDED.error_message,
				updated_at = now()
		`, groupRowID, item.ID, item.Content, item.Amount.AmountCents, item.BankCode, item.BankName,
			item.BeneficiaryName, item.Description, item.PayerName, item.DueDate, item.TransactionDate,
			item.Overdue, string(item.ProductType), string(item.Status), item.ErrorMessage)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) DeleteItems(ctx context.Context, baas domain.BaaS, providerGroupID string, providerItemIDs []string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM payment_group_items
		WHERE provider_item_id = ANY($3)
		AND group_id = (SELECT id FROM payment_groups WHERE baas = $1 AND provider_group_id = $2)
	`, baas, providerGroupID, providerItemIDs)
	return err
}
