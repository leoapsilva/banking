package service

import (
	"context"
	"fmt"

	"github.com/upwifi/banking/internal/paymentscheduling/domain"
	"github.com/upwifi/banking/internal/paymentscheduling/repository"
	"github.com/upwifi/banking/internal/platform/providerregistry"

	paymentschedulingfeature "github.com/upwifi/banking/internal/paymentscheduling"
)

const featureName = "paymentscheduling"

// Service orchestrates DDA/payment-scheduling use cases against whichever
// BaaS provider the caller selected, persisting our own bookkeeping rows
// alongside the provider's response.
type Service struct {
	registry *providerregistry.Registry
	repo     *repository.Repository
}

func New(registry *providerregistry.Registry, repo *repository.Repository) *Service {
	return &Service{registry: registry, repo: repo}
}

func (s *Service) resolveProvider(baas domain.BaaS) (paymentschedulingfeature.Provider, error) {
	return providerregistry.Get[paymentschedulingfeature.Provider](s.registry, featureName, string(baas))
}

// GetDDA handles roteiro item 8.1: lists boletos pending payment.
func (s *Service) GetDDA(ctx context.Context, baas domain.BaaS) ([]domain.DDABond, error) {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return nil, err
	}
	bonds, err := provider.GetDDA(ctx, baas)
	if err != nil {
		return nil, fmt.Errorf("paymentscheduling: get dda via provider: %w", err)
	}
	return bonds, nil
}

// SubmitForDecode handles roteiro item 8.2: submits a batch of payments for
// pre-processing and persists the resulting group locally.
func (s *Service) SubmitForDecode(ctx context.Context, req domain.SubmitForDecodeRequest) (domain.SubmitForDecodeResult, error) {
	if err := validateSubmitForDecode(req); err != nil {
		return domain.SubmitForDecodeResult{}, err
	}

	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return domain.SubmitForDecodeResult{}, err
	}

	result, err := provider.SubmitForDecode(ctx, req)
	if err != nil {
		return domain.SubmitForDecodeResult{}, fmt.Errorf("paymentscheduling: submit for decode via provider: %w", err)
	}

	if _, err := s.repo.SaveGroup(ctx, req.BaaS, result.GroupID, ""); err != nil {
		return domain.SubmitForDecodeResult{}, fmt.Errorf("paymentscheduling: persist group: %w", err)
	}

	return result, nil
}

// GetGroupItems handles roteiro item 8.3: lists every item in a group and
// reconciles our local copy with the authoritative status from C6.
func (s *Service) GetGroupItems(ctx context.Context, baas domain.BaaS, groupID string) ([]domain.PaymentItem, error) {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return nil, err
	}
	items, err := provider.GetGroupItems(ctx, baas, groupID)
	if err != nil {
		return nil, fmt.Errorf("paymentscheduling: get group items via provider: %w", err)
	}
	if err := s.repo.SyncItems(ctx, baas, groupID, items); err != nil {
		return nil, fmt.Errorf("paymentscheduling: sync items: %w", err)
	}
	return items, nil
}

// RemoveItems handles roteiro item 8.4: removes a batch of items from a
// group before it's submitted for approval.
func (s *Service) RemoveItems(ctx context.Context, baas domain.BaaS, groupID string, itemIDs []string) error {
	if len(itemIDs) == 0 {
		return fmt.Errorf("paymentscheduling: item_ids is required")
	}
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return err
	}
	if err := provider.RemoveItems(ctx, baas, groupID, itemIDs); err != nil {
		return fmt.Errorf("paymentscheduling: remove items via provider: %w", err)
	}
	return s.repo.DeleteItems(ctx, baas, groupID, itemIDs)
}

// RemoveItem handles roteiro item 8.5: removes a single item from a group.
func (s *Service) RemoveItem(ctx context.Context, baas domain.BaaS, groupID, itemID string) error {
	provider, err := s.resolveProvider(baas)
	if err != nil {
		return err
	}
	if err := provider.RemoveItem(ctx, baas, groupID, itemID); err != nil {
		return fmt.Errorf("paymentscheduling: remove item via provider: %w", err)
	}
	return s.repo.DeleteItems(ctx, baas, groupID, []string{itemID})
}

// SubmitForApproval handles roteiro item 8.6: locks the group and sends it
// to web banking for human approval.
func (s *Service) SubmitForApproval(ctx context.Context, req domain.SubmitForApprovalRequest) error {
	if req.GroupID == "" {
		return fmt.Errorf("paymentscheduling: group_id is required")
	}
	if req.UploaderName == "" {
		return fmt.Errorf("paymentscheduling: uploader_name is required")
	}
	provider, err := s.resolveProvider(req.BaaS)
	if err != nil {
		return err
	}
	if err := provider.SubmitForApproval(ctx, req); err != nil {
		return fmt.Errorf("paymentscheduling: submit for approval via provider: %w", err)
	}
	return s.repo.UpdateGroupStatus(ctx, req.BaaS, req.GroupID, "APPROVAL_REQUESTED")
}

func validateSubmitForDecode(req domain.SubmitForDecodeRequest) error {
	if len(req.Items) == 0 {
		return fmt.Errorf("paymentscheduling: items must not be empty")
	}
	for i, item := range req.Items {
		if item.Content == "" {
			return fmt.Errorf("paymentscheduling: items[%d].content is required", i)
		}
		if item.Amount.AmountCents <= 0 {
			return fmt.Errorf("paymentscheduling: items[%d].amount must be greater than zero", i)
		}
	}
	return nil
}
