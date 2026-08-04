package boleto

import (
	"context"

	"github.com/upwifi/banking/internal/boleto/domain"
)

// Provider is implemented once per BaaS (e.g. internal/provider/c6) and
// translates the unified domain model to/from that provider's bank slip API.
type Provider interface {
	Create(ctx context.Context, req domain.CreateBankSlipRequest) (domain.BankSlipResult, error)
	Get(ctx context.Context, providerBankSlipID string) (domain.BankSlipDetails, error)
	// GetPDF returns the bank slip rendered as raw PDF bytes. The provider
	// (e.g. C6) generates the PDF; we only proxy it.
	GetPDF(ctx context.Context, providerBankSlipID string) ([]byte, error)
	Update(ctx context.Context, providerBankSlipID string, req domain.UpdateBankSlipRequest) (domain.BankSlipDetails, error)
	Cancel(ctx context.Context, providerBankSlipID string) error
}
