package checkout

import (
	"context"
	"errors"

	"github.com/upwifi/banking/internal/checkout/domain"
)

// ErrNotSupported is returned by provider adapters for operations that the
// BaaS does not support (e.g. InfinitePay has no Authorize or Cancel API).
// Callers can check with errors.Is(err, checkout.ErrNotSupported).
var ErrNotSupported = errors.New("operation not supported by this provider")

// Provider is implemented once per BaaS (e.g. internal/provider/c6) and
// translates the unified domain model to/from that provider's API.
type Provider interface {
	Create(ctx context.Context, req domain.CreateCheckoutRequest) (domain.CheckoutResult, error)
	Authorize(ctx context.Context, req domain.AuthorizeRequest) (domain.AuthorizeResult, error)
	Get(ctx context.Context, providerCheckoutID string) (domain.CheckoutDetails, error)
	Cancel(ctx context.Context, providerCheckoutID string) error
}
