package service

import (
	"context"
	"fmt"

	"github.com/upwifi/banking/internal/pix/domain"
	"github.com/upwifi/banking/internal/pix/repository"
	"github.com/upwifi/banking/internal/platform/providerregistry"

	pixfeature "github.com/upwifi/banking/internal/pix"
)

const featureName = "pix"

// Service orchestrates PIX use cases against whichever BaaS provider the
// caller selected, persisting our own bookkeeping rows alongside the
// provider's response — mirroring internal/checkout/service.
type Service struct {
	registry *providerregistry.Registry
	repo     *repository.Repository
}

func New(registry *providerregistry.Registry, repo *repository.Repository) *Service {
	return &Service{registry: registry, repo: repo}
}

func (s *Service) resolveProvider(baas domain.BaaS) (pixfeature.Provider, error) {
	return providerregistry.Get[pixfeature.Provider](s.registry, featureName, string(baas))
}

// CreateImmediateCharge handles items 7.1/7.2 of the homologation script:
// creates a "cobrança imediata" with a fixed expiration, optionally
// identifying the debtor by CPF/CNPJ and name.
func (s *Service) CreateImmediateCharge(ctx context.Context, req domain.CreateImmediateChargeRequest) (domain.ChargeResult, error) {
	if err := validateImmediateRequest(req); err != nil {
		return domain.ChargeResult{}, err
	}

	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return domain.ChargeResult{}, err
	}

	result, err := provider.CreateImmediate(ctx, req)
	if err != nil {
		return domain.ChargeResult{}, fmt.Errorf("pix: create immediate charge via provider: %w", err)
	}

	if _, err := s.repo.Create(ctx, toRepoCharge(req.BaaS, result, req.PayerRequest), result); err != nil {
		return domain.ChargeResult{}, fmt.Errorf("pix: persist immediate charge: %w", err)
	}
	return result, nil
}

// CreateChargeWithDueDate handles item 7.5: creates a "cobrança com
// vencimento", which always requires a fully identified debtor.
func (s *Service) CreateChargeWithDueDate(ctx context.Context, req domain.CreateChargeWithDueDateRequest) (domain.ChargeResult, error) {
	if err := validateDueDateRequest(req); err != nil {
		return domain.ChargeResult{}, err
	}

	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return domain.ChargeResult{}, err
	}

	result, err := provider.CreateWithDueDate(ctx, req)
	if err != nil {
		return domain.ChargeResult{}, fmt.Errorf("pix: create charge with due date via provider: %w", err)
	}

	if _, err := s.repo.Create(ctx, toRepoCharge(req.BaaS, result, req.PayerRequest), result); err != nil {
		return domain.ChargeResult{}, fmt.Errorf("pix: persist charge with due date: %w", err)
	}
	return result, nil
}

// GetImmediateCharge handles item 7.3: consults an immediate charge by
// txid, reconciling our local status copy with the provider's response.
func (s *Service) GetImmediateCharge(ctx context.Context, baas domain.BaaS, txID string) (domain.ChargeResult, error) {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return domain.ChargeResult{}, err
	}
	result, err := provider.GetByTxID(ctx, txID)
	if err != nil {
		return domain.ChargeResult{}, fmt.Errorf("pix: get immediate charge via provider: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, baas, txID, result.Status, result.Revision); err != nil {
		return domain.ChargeResult{}, fmt.Errorf("pix: reconcile status: %w", err)
	}
	return result, nil
}

// GetChargeWithDueDate handles item 7.7: consults a charge with a due date
// by txid.
func (s *Service) GetChargeWithDueDate(ctx context.Context, baas domain.BaaS, txID string) (domain.ChargeResult, error) {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return domain.ChargeResult{}, err
	}
	result, err := provider.GetWithDueDateByTxID(ctx, txID)
	if err != nil {
		return domain.ChargeResult{}, fmt.Errorf("pix: get charge with due date via provider: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, baas, txID, result.Status, result.Revision); err != nil {
		return domain.ChargeResult{}, fmt.Errorf("pix: reconcile status: %w", err)
	}
	return result, nil
}

// ListImmediateCharges handles item 7.4: lists immediate charges within a
// creation date range.
func (s *Service) ListImmediateCharges(ctx context.Context, req domain.ListChargesRequest) (domain.ListChargesResult, error) {
	if err := validateListRequest(req); err != nil {
		return domain.ListChargesResult{}, err
	}
	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return domain.ListChargesResult{}, err
	}
	result, err := provider.ListImmediate(ctx, req)
	if err != nil {
		return domain.ListChargesResult{}, fmt.Errorf("pix: list immediate charges via provider: %w", err)
	}
	return result, nil
}

// ListChargesWithDueDate handles item 7.6: lists charges with a due date
// within a creation date range.
func (s *Service) ListChargesWithDueDate(ctx context.Context, req domain.ListChargesRequest) (domain.ListChargesResult, error) {
	if err := validateListRequest(req); err != nil {
		return domain.ListChargesResult{}, err
	}
	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return domain.ListChargesResult{}, err
	}
	result, err := provider.ListWithDueDate(ctx, req)
	if err != nil {
		return domain.ListChargesResult{}, fmt.Errorf("pix: list charges with due date via provider: %w", err)
	}
	return result, nil
}

// RegisterWebhook handles item 7.8: configures the PIX product's own
// webhook for a given PIX key.
func (s *Service) RegisterWebhook(ctx context.Context, req domain.RegisterWebhookRequest) error {
	if req.PixKey == "" {
		return fmt.Errorf("pix: pix_key is required to register a webhook")
	}
	if req.URL == "" {
		return fmt.Errorf("pix: url is required to register a webhook")
	}
	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return err
	}
	if err := provider.RegisterWebhook(ctx, req); err != nil {
		return fmt.Errorf("pix: register webhook via provider: %w", err)
	}
	return nil
}

func validateImmediateRequest(req domain.CreateImmediateChargeRequest) error {
	if req.PixKey == "" {
		return fmt.Errorf("pix: pix_key is required")
	}
	if req.Amount.AmountCents <= 0 {
		return fmt.Errorf("pix: amount_cents must be greater than zero")
	}
	if req.ExpirationSeconds <= 0 {
		return fmt.Errorf("pix: expiration_seconds must be greater than zero")
	}
	// C6's devedor is oneOf PessoaFisica|PessoaJuridica: name plus exactly
	// one of CPF/CNPJ. Optional overall, but invalid if provided
	// incompletely or with both documents set.
	if req.Debtor != nil {
		if req.Debtor.Name == "" {
			return fmt.Errorf("pix: debtor.name is required when debtor is informed")
		}
		if req.Debtor.DocumentType != domain.DebtorCPF && req.Debtor.DocumentType != domain.DebtorCNPJ {
			return fmt.Errorf("pix: debtor.document_type must be CPF or CNPJ")
		}
		if req.Debtor.DocumentID == "" {
			return fmt.Errorf("pix: debtor.document_id is required when debtor is informed")
		}
	}
	return nil
}

func validateDueDateRequest(req domain.CreateChargeWithDueDateRequest) error {
	if req.TxID == "" {
		return fmt.Errorf("pix: txid is required for a charge with due date")
	}
	if req.PixKey == "" {
		return fmt.Errorf("pix: pix_key is required")
	}
	if req.Amount.AmountCents <= 0 {
		return fmt.Errorf("pix: amount_cents must be greater than zero")
	}
	if req.DueDate.IsZero() {
		return fmt.Errorf("pix: due_date is required")
	}
	// CobVGerada.devedor is always required (unlike /cob), including the
	// full postal address.
	if req.Debtor.Name == "" {
		return fmt.Errorf("pix: debtor.name is required")
	}
	if req.Debtor.DocumentType != domain.DebtorCPF && req.Debtor.DocumentType != domain.DebtorCNPJ {
		return fmt.Errorf("pix: debtor.document_type must be CPF or CNPJ")
	}
	if req.Debtor.DocumentID == "" {
		return fmt.Errorf("pix: debtor.document_id is required")
	}
	if req.Debtor.Street == "" || req.Debtor.City == "" || req.Debtor.State == "" || req.Debtor.ZipCode == "" {
		return fmt.Errorf("pix: debtor address (street, city, state, zip_code) is required for a charge with due date")
	}
	return nil
}

func validateListRequest(req domain.ListChargesRequest) error {
	if req.Start.IsZero() || req.End.IsZero() {
		return fmt.Errorf("pix: start and end are required to list charges")
	}
	if req.End.Before(req.Start) {
		return fmt.Errorf("pix: end must not be before start")
	}
	return nil
}

func toRepoCharge(baas domain.BaaS, result domain.ChargeResult, payerRequest string) repository.Charge {
	c := repository.Charge{
		BaaS:         baas,
		TxID:         result.TxID,
		Kind:         result.Kind,
		Amount:       result.Amount,
		PixKey:       result.PixKey,
		Status:       result.Status,
		Revision:     result.Revision,
		Location:     result.Location,
		DueDate:      result.DueDate,
		Debtor:       result.Debtor,
		PayerRequest: payerRequest,
	}
	if result.Kind == domain.ChargeImmediate {
		exp := result.ExpirationSeconds
		c.ExpirationSeconds = &exp
	}
	return c
}
