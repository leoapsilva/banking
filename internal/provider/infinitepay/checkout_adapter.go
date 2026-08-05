// Package infinitepay implements the checkout.Provider port against
// InfinitePay's hosted-link Checkout API. All InfinitePay-specific HTTP
// concerns are isolated here; feature packages only ever see the unified
// domain model.
package infinitepay

import (
	"context"
	"net/http"

	checkoutfeature "github.com/upwifi/banking/internal/checkout"
	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/provider/infinitepay/client"
	"github.com/upwifi/banking/internal/provider/infinitepay/dto"
	"github.com/upwifi/banking/internal/provider/infinitepay/mapper"
)

// CheckoutAdapter implements checkout.Provider against the InfinitePay
// Checkout API (POST /links). Authorize, Get, and Cancel are not supported
// by this provider and return checkout.ErrNotSupported.
type CheckoutAdapter struct {
	client     *client.Client
	handle     string
	webhookURL string
}

// NewCheckoutAdapter creates the adapter. handle is the InfiniteTag (without
// the leading '$'). webhookURL is our fixed inbound endpoint that InfinitePay
// will POST payment notifications to.
func NewCheckoutAdapter(c *client.Client, handle, webhookURL string) *CheckoutAdapter {
	return &CheckoutAdapter{client: c, handle: handle, webhookURL: webhookURL}
}

func (a *CheckoutAdapter) Create(ctx context.Context, req checkoutdomain.CreateCheckoutRequest) (checkoutdomain.CheckoutResult, error) {
	body := mapper.ToCreateLinkRequest(a.handle, a.webhookURL, req)
	var resp dto.CreateLinkResponse
	if err := a.client.Do(ctx, http.MethodPost, "/links", body, &resp); err != nil {
		return checkoutdomain.CheckoutResult{}, err
	}
	return mapper.FromCreateLinkResponse(resp, req.ExternalReferenceID), nil
}

// Authorize is not supported: InfinitePay has no tokenized-card charge API.
func (a *CheckoutAdapter) Authorize(_ context.Context, _ checkoutdomain.AuthorizeRequest) (checkoutdomain.AuthorizeResult, error) {
	return checkoutdomain.AuthorizeResult{}, checkoutfeature.ErrNotSupported
}

// Get is not supported: InfinitePay has no GET-by-id endpoint.
// The checkout service falls back to reading status from the local DB
// (updated by the InfinitePay inbound webhook) when it receives this error.
func (a *CheckoutAdapter) Get(_ context.Context, _ string) (checkoutdomain.CheckoutDetails, error) {
	return checkoutdomain.CheckoutDetails{}, checkoutfeature.ErrNotSupported
}

// Cancel is not supported: InfinitePay has no cancel-link API.
func (a *CheckoutAdapter) Cancel(_ context.Context, _ string) error {
	return checkoutfeature.ErrNotSupported
}
