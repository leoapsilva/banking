package checkout

import (
	"context"

	"github.com/upwifi/banking/internal/checkout/domain"
)

// Provider is implemented once per BaaS (e.g. internal/provider/c6) and
// translates the unified domain model to/from that provider's API.
type Provider interface {
	Create(ctx context.Context, req domain.CreateCheckoutRequest) (domain.CheckoutResult, error)
	Authorize(ctx context.Context, req domain.AuthorizeRequest) (domain.AuthorizeResult, error)
	Get(ctx context.Context, providerCheckoutID string) (domain.CheckoutDetails, error)
	Cancel(ctx context.Context, providerCheckoutID string) error
}
