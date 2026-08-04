// Package billing owns the commercial side of subscriptions: lifecycle,
// pricing, coupons and delinquency policy. It is deliberately separated from
// the PSP integration layer (internal/checkout, internal/provider/*), which
// owns moving money.
//
// The boundary rule: billing never touches checkout's tables or concrete
// types-with-behaviour. It reaches the payment layer only through the
// interfaces declared here, which internal/checkout satisfies. That is what
// makes this package extractable into its own service once a second product
// consumes it, without a rewrite.
package billing

import (
	"context"

	"github.com/google/uuid"

	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	checkoutrepo "github.com/upwifi/banking/internal/checkout/repository"
)

// PaymentGateway is the outbound port billing uses to move money. It is
// satisfied by *internal/checkout/service.Service.
//
// Note that the request/response types still come from checkout/domain. That
// is shared vocabulary, not shared implementation: billing depends on the
// payment layer's language but not on its storage or wiring. Translating to
// billing-owned DTOs is a larger change and is deliberately not done here.
type PaymentGateway interface {
	// CreateCheckout opens a hosted payment for the first (or only) charge.
	CreateCheckout(ctx context.Context, req checkoutdomain.CreateCheckoutRequest) (checkoutdomain.CheckoutResult, error)

	// Authorize charges a saved card token directly, without a hosted page.
	// Not every provider supports it — InfinitePay returns ErrNotSupported.
	Authorize(ctx context.Context, req checkoutdomain.AuthorizeRequest) (checkoutdomain.AuthorizeResult, error)

	// CancelCheckout aborts a pending hosted checkout at the provider.
	CancelCheckout(ctx context.Context, baas checkoutdomain.BaaS, providerCheckoutID string) error
}

// CheckoutStore is the outbound port billing uses to read and annotate the
// checkout records backing a subscription. It is satisfied by
// *internal/checkout/repository.Repository.
//
// Billing reads these records but does not own them: it never creates or
// deletes checkouts, only correlates them to subscriptions and records the
// outcome reported by the provider.
type CheckoutStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (checkoutrepo.Checkout, error)
	GetByProviderID(ctx context.Context, baas checkoutdomain.BaaS, providerCheckoutID string) (checkoutrepo.Checkout, error)
	UpdateStatus(ctx context.Context, baas checkoutdomain.BaaS, providerCheckoutID string, status checkoutdomain.Status) error
	UpdatePaymentDetails(ctx context.Context, checkoutID uuid.UUID, details checkoutrepo.PaymentDetails) error
	SaveCardToken(ctx context.Context, token checkoutrepo.CardToken) (uuid.UUID, error)
	GetCardTokenValue(ctx context.Context, tokenID uuid.UUID) (string, error)
}
