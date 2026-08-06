package service

import (
	"context"
	"errors"
	"fmt"

	checkoutfeature "github.com/upwifi/banking/internal/checkout"
	"github.com/upwifi/banking/internal/checkout/domain"
	"github.com/upwifi/banking/internal/checkout/repository"
	"github.com/upwifi/banking/internal/platform/providerregistry"
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
		row.PayerEmail = req.Payer.Email
		row.PayerPhone = req.Payer.PhoneNumber
		row.PayerAddress = req.Payer.Address
	}
	if _, err := s.repo.Create(ctx, row, result); err != nil {
		return domain.CheckoutResult{}, fmt.Errorf("checkout: persist: %w", err)
	}

	return result, nil
}

// GetCheckout fetches the latest checkout state. For providers that support
// a GET endpoint (e.g. C6), it calls the provider and reconciles the local
// status. For providers that return ErrNotSupported from Get() (e.g.
// InfinitePay — DB-first, webhook-driven), it reads the already-persisted
// state from the repository without any outbound API call.
func (s *Service) GetCheckout(ctx context.Context, baas domain.BaaS, providerCheckoutID string) (domain.CheckoutDetails, error) {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return domain.CheckoutDetails{}, err
	}

	details, err := provider.Get(ctx, providerCheckoutID)
	if errors.Is(err, checkoutfeature.ErrNotSupported) {
		row, repoErr := s.repo.GetByProviderID(ctx, baas, providerCheckoutID)
		if repoErr != nil {
			return domain.CheckoutDetails{}, fmt.Errorf("checkout: get from repository: %w", repoErr)
		}
		return rowToCheckoutDetails(row), nil
	}
	if err != nil {
		return domain.CheckoutDetails{}, fmt.Errorf("checkout: get via provider: %w", err)
	}

	if err := s.repo.UpdateStatus(ctx, baas, providerCheckoutID, details.Status); err != nil {
		return domain.CheckoutDetails{}, fmt.Errorf("checkout: reconcile status: %w", err)
	}
	return details, nil
}

// CheckPayment asks the selected provider to actively confirm payment
// status (see domain.CheckPaymentRequest). Unlike GetCheckout, it does not
// fall back to the repository on ErrNotSupported: a caller reaching for
// this method specifically wants a fresh provider-side confirmation, and
// silently returning stale local state would defeat that.
func (s *Service) CheckPayment(ctx context.Context, baas domain.BaaS, req domain.CheckPaymentRequest) (domain.CheckoutDetails, error) {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return domain.CheckoutDetails{}, err
	}

	details, err := provider.CheckPayment(ctx, req)
	if err != nil {
		return domain.CheckoutDetails{}, fmt.Errorf("checkout: check payment via provider: %w", err)
	}
	return details, nil
}

// rowToCheckoutDetails converts a repository row to the domain details shape,
// used for DB-first providers (InfinitePay) where Get() is not available.
func rowToCheckoutDetails(row repository.Checkout) domain.CheckoutDetails {
	d := domain.CheckoutDetails{
		ProviderCheckoutID: row.ProviderCheckoutID,
		Status:             row.Status,
		Amount:             row.Amount,
		CaptureMethod:      row.CaptureMethod,
		Installments:       row.Installments,
		ReceiptURL:         row.ReceiptURL,
		TransactionID:      row.TransactionID,
		PaidAt:             row.PaidAt,
	}
	if row.PaidAmountCents != nil {
		d.PaidAmount = &domain.Money{AmountCents: *row.PaidAmountCents, Currency: row.Amount.Currency}
	}
	return d
}

// CancelCheckout aborts a pending checkout or reverses a paid one when the
// payment method allows it. For providers that don't support cancellation via
// API (ErrNotSupported), the local status is still updated to CANCELLED so
// the subscription cancel flow completes cleanly.
func (s *Service) CancelCheckout(ctx context.Context, baas domain.BaaS, providerCheckoutID string) error {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return err
	}
	if err := provider.Cancel(ctx, providerCheckoutID); err != nil && !errors.Is(err, checkoutfeature.ErrNotSupported) {
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
