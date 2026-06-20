package pix

import (
	"context"

	"github.com/upwifi/banking/internal/pix/domain"
)

// Provider is implemented once per BaaS (e.g. internal/provider/c6) and
// translates the unified PIX domain model to/from that provider's API.
type Provider interface {
	CreateImmediate(ctx context.Context, req domain.CreateImmediateChargeRequest) (domain.ChargeResult, error)
	CreateWithDueDate(ctx context.Context, req domain.CreateChargeWithDueDateRequest) (domain.ChargeResult, error)
	GetByTxID(ctx context.Context, txID string) (domain.ChargeResult, error)
	GetWithDueDateByTxID(ctx context.Context, txID string) (domain.ChargeResult, error)
	ListImmediate(ctx context.Context, req domain.ListChargesRequest) (domain.ListChargesResult, error)
	ListWithDueDate(ctx context.Context, req domain.ListChargesRequest) (domain.ListChargesResult, error)
	RegisterWebhook(ctx context.Context, req domain.RegisterWebhookRequest) error
}
