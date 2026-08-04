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
func FromCreateLinkResponse(resp dto.CreateLinkResponse) domain.CheckoutResult {
	return domain.CheckoutResult{
		ProviderCheckoutID: resp.Slug,
		CheckoutURL:        resp.URL,
		Status:             domain.StatusCreated,
	}
}
