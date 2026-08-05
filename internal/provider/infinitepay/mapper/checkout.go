// Package mapper holds pure functions translating between the unified checkout
// domain model and InfinitePay's DTOs. Side-effect free and isolated so they
// can be table-driven tested without any HTTP/DB dependency.
package mapper

import (
	"github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/provider/infinitepay/dto"
)

// ToCreateLinkRequest builds the InfinitePay POST /links body.
// handle and webhookURL are injected from config (not from the caller).
func ToCreateLinkRequest(handle, webhookURL string, req domain.CreateCheckoutRequest) dto.CreateLinkRequest {
	description := req.Description
	if description == "" {
		description = "Pagamento"
	}

	linkReq := dto.CreateLinkRequest{
		Handle: handle,
		Items: []dto.Item{
			{
				Quantity:    1,
				Price:       req.Amount.AmountCents,
				Description: description,
			},
		},
		OrderNSU:    req.ExternalReferenceID,
		RedirectURL: req.RedirectURL,
		WebhookURL:  webhookURL,
	}

	if req.Payer != nil {
		linkReq.Customer = &dto.Customer{
			Name:        req.Payer.Name,
			Email:       req.Payer.Email,
			PhoneNumber: req.Payer.PhoneNumber,
		}
		if req.Payer.Address != nil {
			linkReq.Address = &dto.Address{
				Cep:          req.Payer.Address.ZipCode,
				Street:       req.Payer.Address.Street,
				Neighborhood: req.Payer.Address.Neighborhood,
				Number:       req.Payer.Address.Number,
				Complement:   req.Payer.Address.Complement,
			}
		}
	}

	return linkReq
}

// FromCreateLinkResponse builds the unified CheckoutResult from InfinitePay's
// POST /links response. Status is always CREATED at this point.
//
// orderNSU is the reference we generated and sent as order_nsu on the
// request (see ToCreateLinkRequest) — NOT resp.Slug. Verified against a real
// sandbox call: InfinitePay's actual response to POST /links carries only
// `url`, no `slug` field, despite the documentation describing one. Using
// resp.Slug here made ProviderCheckoutID always empty, which both broke
// webhook correlation and violated the (baas, provider_checkout_id) unique
// constraint on every checkout after the first. order_nsu is the only
// identifier we can rely on: we choose it at creation time, and InfinitePay
// echoes it back in the webhook payload (see webhook_adapter.go), so the
// same value is available on both ends of the correlation.
func FromCreateLinkResponse(resp dto.CreateLinkResponse, orderNSU string) domain.CheckoutResult {
	return domain.CheckoutResult{
		ProviderCheckoutID: orderNSU,
		CheckoutURL:        resp.URL,
		Status:             domain.StatusCreated,
	}
}
