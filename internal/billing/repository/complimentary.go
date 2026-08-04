package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ComplimentaryGrant describes a decision to let a subscription run without
// charging for it.
//
// Either Until is set (exemption with an end date) or Forever is true
// (open-ended). Reason and GrantedBy are required by the service, not by the
// schema, because an unexplained free subscription is impossible to audit
// later.
type ComplimentaryGrant struct {
	Until     *time.Time
	Forever   bool
	Reason    string
	GrantedBy string
}

// GrantComplimentary marks a subscription as not billable and clears its next
// charge date, so the recurring worker stops selecting it.
func (r *Repository) GrantComplimentary(ctx context.Context, subID uuid.UUID, g ComplimentaryGrant) error {
	const q = `
		UPDATE subscriptions
		SET complimentary_until      = $2,
		    complimentary_forever    = $3,
		    complimentary_reason     = $4,
		    complimentary_granted_by = $5,
		    complimentary_granted_at = now(),
		    next_charge_date         = NULL,
		    status                   = 'ACTIVE',
		    updated_at               = now()
		WHERE id = $1`

	tag, err := r.pool.Exec(ctx, q, subID, g.Until, g.Forever, g.Reason, g.GrantedBy)
	if err != nil {
		return fmt.Errorf("billing: grant complimentary: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("billing: subscription %s not found", subID)
	}
	return nil
}

// RevokeComplimentary ends an exemption. It deliberately does not resume
// billing on its own: the subscription is left without a next charge date, so
// someone has to decide what happens next rather than a charge appearing
// unannounced on a customer who believed they were exempt.
func (r *Repository) RevokeComplimentary(ctx context.Context, subID uuid.UUID) error {
	const q = `
		UPDATE subscriptions
		SET complimentary_until      = NULL,
		    complimentary_forever    = FALSE,
		    complimentary_granted_at = NULL,
		    updated_at               = now()
		WHERE id = $1`

	if _, err := r.pool.Exec(ctx, q, subID); err != nil {
		return fmt.Errorf("billing: revoke complimentary: %w", err)
	}
	return nil
}
