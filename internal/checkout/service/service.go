package service

import (
	"context"
	"fmt"

	"github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/checkout/repository"
	"github.com/upwifi/banking/internal/platform/providerregistry"

	checkoutfeature "github.com/upwifi/banking/internal/checkout"
)

const featureName = "checkout"

// Service orchestrates checkout use cases against whichever BaaS provider
// the caller selected, persisting our own bookkeeping rows alongside the
// provider's response.
type Service struct {
	registry *providerregistry.Registry
	repo     *repository.Repository
}

func New(registry *providerregistry.Registry, repo *repository.Repository) *Service {
	return &Service{registry: registry, repo: repo}
}

func (s *Service) resolveProvider(baas domain.BaaS) (checkoutfeature.Provider, error) {
	return providerregistry.Get[checkoutfeature.Provider](s.registry, featureName, string(baas))
}

// CreateCheckout validates the request, delegates to the selected provider,
// and persists the resulting checkout row.
func (s *Service) CreateCheckout(ctx context.Context, req domain.CreateCheckoutRequest) (domain.CheckoutResult, error) {
	if err := validateCardPayment(req.Card); err != nil {
		return domain.CheckoutResult{}, err
	}

	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return domain.CheckoutResult{}, err
	}

	result, err := provider.Create(ctx, req)
	if err != nil {
		return domain.CheckoutResult{}, fmt.Errorf("checkout: create via provider: %w", err)
	}

	row := repository.Checkout{
		BaaS:                req.BaaS,
		ProviderCheckoutID:  result.ProviderCheckoutID,
		ExternalReferenceID: req.ExternalReferenceID,
		Amount:              req.Amount,
		Status:              result.Status,
		CheckoutURL:         result.CheckoutURL,
	}
	if req.Payer != nil {
		row.PayerTaxID = req.Payer.TaxID
		row.PayerName = req.Payer.Name
	}
	if _, err := s.repo.Create(ctx, row, result); err != nil {
		return domain.CheckoutResult{}, fmt.Errorf("checkout: persist: %w", err)
	}

	return result, nil
}

// GetCheckout fetches the latest checkout state from the provider and
// reconciles our local status copy.
func (s *Service) GetCheckout(ctx context.Context, baas domain.BaaS, providerCheckoutID string) (domain.CheckoutDetails, error) {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return domain.CheckoutDetails{}, err
	}
	details, err := provider.Get(ctx, providerCheckoutID)
	if err != nil {
		return domain.CheckoutDetails{}, fmt.Errorf("checkout: get via provider: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, baas, providerCheckoutID, details.Status); err != nil {
		return domain.CheckoutDetails{}, fmt.Errorf("checkout: reconcile status: %w", err)
	}
	return details, nil
}

// CancelCheckout aborts a pending checkout or reverses a paid one when the
// payment method allows it.
func (s *Service) CancelCheckout(ctx context.Context, baas domain.BaaS, providerCheckoutID string) error {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return err
	}
	if err := provider.Cancel(ctx, providerCheckoutID); err != nil {
		return fmt.Errorf("checkout: cancel via provider: %w", err)
	}
	return s.repo.UpdateStatus(ctx, baas, providerCheckoutID, domain.StatusCancelled)
}

// Authorize charges a previously tokenized card directly. Used both for the
// annual installment flow (single authorize with installments>1) and by the
// subscription cron worker for recurring monthly cycles.
func (s *Service) Authorize(ctx context.Context, req domain.AuthorizeRequest) (domain.AuthorizeResult, error) {
	if err := validateCardPayment(req.Card); err != nil {
		return domain.AuthorizeResult{}, err
	}
	if req.Card == nil || req.Card.CardInfo == nil || req.Card.CardInfo.Token == nil {
		return domain.AuthorizeResult{}, fmt.Errorf("checkout: authorize requires a saved card token")
	}

	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return domain.AuthorizeResult{}, err
	}
	result, err := provider.Authorize(ctx, req)
	if err != nil {
		return domain.AuthorizeResult{}, fmt.Errorf("checkout: authorize via provider: %w", err)
	}
	return result, nil
}

func validateCardPayment(card *domain.CardPayment) error {
	if card == nil {
		return nil
	}
	if card.Installments > 1 && card.InterestType == nil {
		return fmt.Errorf("checkout: interest_type is required when installments > 1")
	}
	if card.Installments < 1 || card.Installments > 12 {
		return fmt.Errorf("checkout: installments must be between 1 and 12")
	}
	return nil
}
