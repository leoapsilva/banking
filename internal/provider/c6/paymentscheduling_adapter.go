package c6

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	psdomain "github.com/upwifi/banking/internal/paymentscheduling/domain"
	"github.com/upwifi/banking/internal/provider/c6/client"
	"github.com/upwifi/banking/internal/provider/c6/dto"
	"github.com/upwifi/banking/internal/provider/c6/mapper"
)

// PaymentSchedulingAdapter implements paymentscheduling.Provider against
// C6's "Agendamento de Pagamentos" API
// (docs/c6/swagger/agendamento-de-pagamentos.yaml). The client passed in
// must be configured with a base URL of ".../v1/schedule_payments".
type PaymentSchedulingAdapter struct {
	client *client.Client
}

func NewPaymentSchedulingAdapter(c *client.Client) *PaymentSchedulingAdapter {
	return &PaymentSchedulingAdapter{client: c}
}

func (a *PaymentSchedulingAdapter) SubmitForDecode(ctx context.Context, req psdomain.SubmitForDecodeRequest) (psdomain.SubmitForDecodeResult, error) {
	body := mapper.ToC6DecodeRequest(req.Items)
	var resp dto.DecodeResponse
	if err := a.client.Do(ctx, http.MethodPost, "/decode", body, &resp); err != nil {
		return psdomain.SubmitForDecodeResult{}, err
	}
	return psdomain.SubmitForDecodeResult{GroupID: resp.GroupID}, nil
}

func (a *PaymentSchedulingAdapter) GetDDA(ctx context.Context, baas psdomain.BaaS) ([]psdomain.DDABond, error) {
	var resp dto.QueryResponse
	if err := a.client.Do(ctx, http.MethodGet, "/query", nil, &resp); err != nil {
		return nil, err
	}
	return mapper.FromC6Bonds(resp), nil
}

// GetGroupItems is retried while C6 reports the group's items are still
// being asynchronously decoded — POST /decode returns the group_id
// immediately, but C6 finishes validating each item (bar code integrity /
// DICT lookup) in the background, so a query made too soon after
// submission returns 422 rather than the items.
func (a *PaymentSchedulingAdapter) GetGroupItems(ctx context.Context, baas psdomain.BaaS, groupID string) ([]psdomain.PaymentItem, error) {
	var items []psdomain.PaymentItem
	err := retryWhileDecoding(ctx, func() error {
		var resp dto.GroupItemsResponse
		if err := a.client.Do(ctx, http.MethodGet, "/"+groupID+"/items", nil, &resp); err != nil {
			return err
		}
		items = mapper.FromC6GroupItems(resp)
		return nil
	})
	return items, err
}

func (a *PaymentSchedulingAdapter) RemoveItems(ctx context.Context, baas psdomain.BaaS, groupID string, itemIDs []string) error {
	body := mapper.ToC6DeleteItemRefs(itemIDs)
	return retryWhileDecoding(ctx, func() error {
		return a.client.Do(ctx, http.MethodDelete, "/"+groupID+"/items", body, nil)
	})
}

func (a *PaymentSchedulingAdapter) RemoveItem(ctx context.Context, baas psdomain.BaaS, groupID, itemID string) error {
	return retryWhileDecoding(ctx, func() error {
		return a.client.Do(ctx, http.MethodDelete, "/"+groupID+"/items/"+itemID, nil, nil)
	})
}

func (a *PaymentSchedulingAdapter) SubmitForApproval(ctx context.Context, req psdomain.SubmitForApprovalRequest) error {
	body := dto.SubmitForApprovalRequest{GroupID: req.GroupID, UploaderName: req.UploaderName}
	return retryWhileDecoding(ctx, func() error {
		return a.client.Do(ctx, http.MethodPost, "/submit", body, nil)
	})
}

// retryWhileDecoding retries fn while it fails with C6's transient
// "items still being decoded" 422 (confirmed live in sandbox:
// {"status":422,"detail":"Alguns itens no grupo ainda estão no processo de
// decodificação."}), backing off briefly between attempts. Any other error
// (including a 422 for a different reason) is returned immediately. Bounded
// to ~30s total — if decoding hasn't finished by then, something else is
// likely wrong and the caller should see the real error rather than hang.
func retryWhileDecoding(ctx context.Context, fn func() error) error {
	const maxAttempts = 8
	const delay = 4 * time.Second

	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil || !isItemsStillDecoding(err) || attempt == maxAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

func isItemsStillDecoding(err error) bool {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == 422 && strings.Contains(apiErr.Body, "processo de decodifica")
}
