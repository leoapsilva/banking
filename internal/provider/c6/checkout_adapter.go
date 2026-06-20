// Package c6 implements the checkout.Provider and webhook.WebhookRegistrarProvider
// ports against C6 Bank's BaaS APIs. All C6-specific HTTP/auth concerns are
// isolated here; feature packages only ever see the unified domain model.
package c6

import (
	"context"
	"net/http"

	checkoutdomain "github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/provider/c6/client"
	"github.com/upwifi/banking/internal/provider/c6/dto"
	"github.com/upwifi/banking/internal/provider/c6/mapper"
)

// CheckoutAdapter implements checkout.Provider against the C6 Checkout API.
type CheckoutAdapter struct {
	client *client.Client
}

func NewCheckoutAdapter(c *client.Client) *CheckoutAdapter {
	return &CheckoutAdapter{client: c}
}

func (a *CheckoutAdapter) Create(ctx context.Context, req checkoutdomain.CreateCheckoutRequest) (checkoutdomain.CheckoutResult, error) {
	body, err := mapper.ToC6CreateCheckoutRequest(req)
	if err != nil {
		return checkoutdomain.CheckoutResult{}, err
	}
	var resp dto.CreateCheckoutResponse
	if err := a.client.Do(ctx, http.MethodPost, "/", body, &resp); err != nil {
		return checkoutdomain.CheckoutResult{}, err
	}
	return mapper.FromC6CreateCheckoutResponse(resp, checkoutdomain.StatusCreated), nil
}

func (a *CheckoutAdapter) Authorize(ctx context.Context, req checkoutdomain.AuthorizeRequest) (checkoutdomain.AuthorizeResult, error) {
	body, err := mapper.ToC6AuthorizeRequest(req)
	if err != nil {
		return checkoutdomain.AuthorizeResult{}, err
	}
	var resp dto.AuthorizeResponse
	if err := a.client.Do(ctx, http.MethodPost, "/authorize", body, &resp); err != nil {
		return checkoutdomain.AuthorizeResult{}, err
	}
	return mapper.FromC6AuthorizeResponse(resp), nil
}

func (a *CheckoutAdapter) Get(ctx context.Context, providerCheckoutID string) (checkoutdomain.CheckoutDetails, error) {
	var resp dto.GetCheckoutResponse
	if err := a.client.Do(ctx, http.MethodGet, "/"+providerCheckoutID, nil, &resp); err != nil {
		return checkoutdomain.CheckoutDetails{}, err
	}
	return mapper.FromC6GetCheckoutResponse(resp), nil
}

func (a *CheckoutAdapter) Cancel(ctx context.Context, providerCheckoutID string) error {
	return a.client.Do(ctx, http.MethodPut, "/"+providerCheckoutID+"/cancel", nil, nil)
}
