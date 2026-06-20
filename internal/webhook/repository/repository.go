package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) SaveRegistration(ctx context.Context, baas, service, url string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO webhook_registrations (baas, service, url)
		VALUES ($1, $2, $3)
		ON CONFLICT (baas, service) DO UPDATE SET url = EXCLUDED.url, registered_at = now()
	`, baas, service, url)
	return err
}

func (r *Repository) DeleteRegistration(ctx context.Context, baas, service string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM webhook_registrations WHERE baas = $1 AND service = $2`, baas, service)
	return err
}

// EventInsertResult tells the caller whether this exact event was already
// processed before, so handlers can skip duplicate side effects.
type EventInsertResult struct {
	ID            string
	AlreadyExists bool
}

// InsertEventIfNew records a received webhook payload. The unique
// constraint on (baas, provider_external_id, status, event_date_time) is
// what makes redelivery from C6 idempotent: a conflict means we've already
// seen and (presumably) processed this exact event.
func (r *Repository) InsertEventIfNew(ctx context.Context, baas, externalID, service, status string, eventDateTime time.Time, rawPayload []byte) (EventInsertResult, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO webhook_events (baas, provider_external_id, service, status, event_date_time, raw_payload)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (baas, provider_external_id, status, event_date_time) DO NOTHING
		RETURNING id
	`, baas, externalID, service, status, eventDateTime, rawPayload).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING suppressed the insert: we've already
		// recorded this exact event.
		return EventInsertResult{AlreadyExists: true}, nil
	}
	if err != nil {
		return EventInsertResult{}, err
	}
	return EventInsertResult{ID: id}, nil
}

func (r *Repository) MarkProcessed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE webhook_events SET processing_status = 'PROCESSED', processed_at = now() WHERE id = $1
	`, id)
	return err
}

func (r *Repository) MarkFailed(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE webhook_events SET processing_status = 'FAILED_PROCESSING', processed_at = now() WHERE id = $1
	`, id)
	return err
}
