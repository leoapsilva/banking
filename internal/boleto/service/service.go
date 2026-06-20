package service

import (
	"context"
	"fmt"
	"time"

	"github.com/upwifi/banking/internal/boleto/domain"
	"github.com/upwifi/banking/internal/boleto/repository"
	"github.com/upwifi/banking/internal/platform/providerregistry"

	boletofeature "github.com/upwifi/banking/internal/boleto"
)

const featureName = "boleto"

// Service orchestrates bank slip use cases against whichever BaaS provider
// the caller selected, persisting our own bookkeeping rows alongside the
// provider's response.
type Service struct {
	registry *providerregistry.Registry
	repo     *repository.Repository
}

func New(registry *providerregistry.Registry, repo *repository.Repository) *Service {
	return &Service{registry: registry, repo: repo}
}

func (s *Service) resolveProvider(baas domain.BaaS) (boletofeature.Provider, error) {
	return providerregistry.Get[boletofeature.Provider](s.registry, featureName, string(baas))
}

// CreateBankSlip validates the request, delegates to the selected provider,
// and persists the resulting bank slip row.
func (s *Service) CreateBankSlip(ctx context.Context, req domain.CreateBankSlipRequest) (domain.BankSlipResult, error) {
	if err := validateCreate(req); err != nil {
		return domain.BankSlipResult{}, err
	}

	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return domain.BankSlipResult{}, err
	}

	result, err := provider.Create(ctx, req)
	if err != nil {
		return domain.BankSlipResult{}, fmt.Errorf("boleto: create via provider: %w", err)
	}

	row := repository.BankSlip{
		BaaS:                req.BaaS,
		ProviderBankSlipID:  result.ProviderBankSlipID,
		ExternalReferenceID: req.ExternalReferenceID,
		Amount:              result.Amount,
		Status:              result.Status,
		DueDate:             result.DueDate,
		PayerTaxID:          req.Payer.TaxID,
		PayerName:           req.Payer.Name,
	}
	if _, err := s.repo.Create(ctx, row, result); err != nil {
		return domain.BankSlipResult{}, fmt.Errorf("boleto: persist: %w", err)
	}

	return result, nil
}

// GetBankSlip fetches the latest bank slip state from the provider and
// reconciles our local status copy.
func (s *Service) GetBankSlip(ctx context.Context, baas domain.BaaS, providerBankSlipID string) (domain.BankSlipDetails, error) {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return domain.BankSlipDetails{}, err
	}
	details, err := provider.Get(ctx, providerBankSlipID)
	if err != nil {
		return domain.BankSlipDetails{}, fmt.Errorf("boleto: get via provider: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, baas, providerBankSlipID, details.Status); err != nil {
		return domain.BankSlipDetails{}, fmt.Errorf("boleto: reconcile status: %w", err)
	}
	return details, nil
}

// UpdateBankSlip alters due date, amount, discount, interest and/or fine on
// a previously issued bank slip.
func (s *Service) UpdateBankSlip(ctx context.Context, baas domain.BaaS, providerBankSlipID string, req domain.UpdateBankSlipRequest) (domain.BankSlipDetails, error) {
	if err := validateUpdate(req); err != nil {
		return domain.BankSlipDetails{}, err
	}

	provider, err := s.resolveProvider(baas)
	if err != nil {
		return domain.BankSlipDetails{}, err
	}
	details, err := provider.Update(ctx, providerBankSlipID, req)
	if err != nil {
		return domain.BankSlipDetails{}, fmt.Errorf("boleto: update via provider: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, baas, providerBankSlipID, details.Status); err != nil {
		return domain.BankSlipDetails{}, fmt.Errorf("boleto: reconcile status: %w", err)
	}
	return details, nil
}

// CancelBankSlip writes off an issued bank slip, paid or not, that has not
// already been settled.
func (s *Service) CancelBankSlip(ctx context.Context, baas domain.BaaS, providerBankSlipID string) error {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return err
	}
	if err := provider.Cancel(ctx, providerBankSlipID); err != nil {
		return fmt.Errorf("boleto: cancel via provider: %w", err)
	}
	return s.repo.UpdateStatus(ctx, baas, providerBankSlipID, domain.StatusCancelled)
}

func validateCreate(req domain.CreateBankSlipRequest) error {
	if req.ExternalReferenceID == "" {
		return fmt.Errorf("boleto: external_reference_id is required")
	}
	if req.Amount.AmountCents <= 0 {
		return fmt.Errorf("boleto: amount must be greater than zero")
	}
	if req.DueDate.Before(truncateToDate(time.Now())) {
		return fmt.Errorf("boleto: due_date must not be in the past")
	}
	if len(req.Instructions) > 4 {
		return fmt.Errorf("boleto: at most 4 instructions are supported")
	}
	for _, instr := range req.Instructions {
		if len(instr) > 80 {
			return fmt.Errorf("boleto: each instruction must be at most 80 characters")
		}
	}
	if req.Payer.Name == "" || req.Payer.TaxID == "" {
		return fmt.Errorf("boleto: payer name and tax_id are required")
	}
	if err := validateDiscount(req.Discount); err != nil {
		return err
	}
	if err := validateFine(req.Fine); err != nil {
		return err
	}
	if err := validateInterest(req.Interest); err != nil {
		return err
	}
	return nil
}

func validateUpdate(req domain.UpdateBankSlipRequest) error {
	if req.Amount == nil && req.DueDate == nil && req.Discount == nil && req.Interest == nil && req.Fine == nil {
		return fmt.Errorf("boleto: update requires at least one field")
	}
	if req.Amount != nil && req.Amount.AmountCents <= 0 {
		return fmt.Errorf("boleto: amount must be greater than zero")
	}
	if err := validateDiscount(req.Discount); err != nil {
		return err
	}
	if err := validateFine(req.Fine); err != nil {
		return err
	}
	if err := validateInterest(req.Interest); err != nil {
		return err
	}
	return nil
}

// validateFine enforces the roteiro's "percentual OU valor fixo" rule: the
// Type field is itself the XOR selector, so there is nothing further to
// reject beyond a recognized type and a positive value.
func validateFine(f *domain.Fine) error {
	if f == nil {
		return nil
	}
	if f.Type != domain.AmountTypeFixed && f.Type != domain.AmountTypePercentage {
		return fmt.Errorf("boleto: fine.type must be FIXED or PERCENTAGE")
	}
	if f.Value <= 0 {
		return fmt.Errorf("boleto: fine.value must be greater than zero")
	}
	if f.DeadlineDays < 0 {
		return fmt.Errorf("boleto: fine.deadline_days must not be negative")
	}
	return nil
}

func validateInterest(i *domain.Interest) error {
	if i == nil {
		return nil
	}
	if i.Type != domain.AmountTypeFixed && i.Type != domain.AmountTypePercentage {
		return fmt.Errorf("boleto: interest.type must be FIXED or PERCENTAGE")
	}
	if i.Value <= 0 {
		return fmt.Errorf("boleto: interest.value must be greater than zero")
	}
	if i.DeadlineDays < 0 {
		return fmt.Errorf("boleto: interest.deadline_days must not be negative")
	}
	return nil
}

// validateDiscount enforces C6's constraints: at most 3 tiers, all tiers
// share one discount_type, and deadlines strictly decrease from first to
// last (no ties, no ascending order).
func validateDiscount(d *domain.Discount) error {
	if d == nil {
		return nil
	}
	if len(d.Tiers) == 0 {
		return fmt.Errorf("boleto: discount must have at least one tier")
	}
	if len(d.Tiers) > 3 {
		return fmt.Errorf("boleto: at most 3 discount tiers are supported")
	}
	firstType := d.Tiers[0].Type
	prevDeadline := -1
	for i, tier := range d.Tiers {
		if tier.Type != domain.AmountTypeFixed && tier.Type != domain.AmountTypePercentage {
			return fmt.Errorf("boleto: discount.tiers[%d].type must be FIXED or PERCENTAGE", i)
		}
		if tier.Type != firstType {
			return fmt.Errorf("boleto: all discount tiers must share the same type")
		}
		if tier.Value <= 0 {
			return fmt.Errorf("boleto: discount.tiers[%d].value must be greater than zero", i)
		}
		if tier.DeadlineDays <= 0 {
			return fmt.Errorf("boleto: discount.tiers[%d].deadline_days must be greater than zero", i)
		}
		if i > 0 && tier.DeadlineDays >= prevDeadline {
			return fmt.Errorf("boleto: discount tiers must have strictly decreasing deadline_days")
		}
		prevDeadline = tier.DeadlineDays
	}
	return nil
}

func truncateToDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
