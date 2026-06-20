// Package paymentscheduling covers DDA consultation and payment-group
// scheduling/approval — item 8 of the C6 conformity roteiro. See
// internal/paymentscheduling/domain for the model and
// docs/c6/swagger/agendamento-de-pagamentos.yaml for the C6 contract.
package paymentscheduling

import (
	"context"

	"github.com/upwifi/banking/internal/paymentscheduling/domain"
)

// Provider is implemented once per BaaS (e.g. internal/provider/c6) and
// translates the unified domain model to/from that provider's API.
type Provider interface {
	// SubmitForDecode handles item 8.2: submits a batch of payments
	// (boleto bar codes / PIX keys or BR Codes) for initial processing,
	// returning a group_id.
	SubmitForDecode(ctx context.Context, req domain.SubmitForDecodeRequest) (domain.SubmitForDecodeResult, error)

	// GetDDA handles item 8.1: lists boletos pending payment, registered
	// against the authenticated PJ's DDA.
	GetDDA(ctx context.Context, baas domain.BaaS) ([]domain.DDABond, error)

	// GetGroupItems handles item 8.3: lists every payment item inside a
	// previously submitted group, with their current decode/payment status.
	GetGroupItems(ctx context.Context, baas domain.BaaS, groupID string) ([]domain.PaymentItem, error)

	// RemoveItems handles item 8.4: removes a list of items from a group
	// before it's submitted for approval.
	RemoveItems(ctx context.Context, baas domain.BaaS, groupID string, itemIDs []string) error

	// RemoveItem handles item 8.5: removes a single item from a group.
	RemoveItem(ctx context.Context, baas domain.BaaS, groupID, itemID string) error

	// SubmitForApproval handles item 8.6: locks the group and sends it to
	// web banking for human approval. The group can no longer be edited
	// via API after this call.
	SubmitForApproval(ctx context.Context, req domain.SubmitForApprovalRequest) error
}
