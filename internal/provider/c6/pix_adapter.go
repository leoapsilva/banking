package c6

import (
	"context"
	"net/http"
	"net/url"

	pixdomain "github.com/upwifi/banking/internal/pix/domain"
	"github.com/upwifi/banking/internal/provider/c6/client"
	"github.com/upwifi/banking/internal/provider/c6/dto"
	"github.com/upwifi/banking/internal/provider/c6/mapper"
)

const c6DateTimeQueryLayout = "2006-01-02T15:04:05Z"

// PixAdapter implements pix.Provider against the C6 PIX API
// (docs/c6/swagger/pix.yaml). The client passed in must be configured with
// a base URL of ".../v2/pix" (see servers: in the swagger).
type PixAdapter struct {
	client *client.Client
}

func NewPixAdapter(c *client.Client) *PixAdapter {
	return &PixAdapter{client: c}
}

// CreateImmediate creates a "cobrança imediata". When req.TxID is empty, it
// uses POST /cob and lets C6 generate the txid; otherwise it uses
// PUT /cob/{txid}, which is idempotent for retries.
func (a *PixAdapter) CreateImmediate(ctx context.Context, req pixdomain.CreateImmediateChargeRequest) (pixdomain.ChargeResult, error) {
	body, err := mapper.ToC6CreateCobRequest(req)
	if err != nil {
		return pixdomain.ChargeResult{}, err
	}
	var resp dto.CobResponse
	if req.TxID == "" {
		if err := a.client.Do(ctx, http.MethodPost, "/cob", body, &resp); err != nil {
			return pixdomain.ChargeResult{}, err
		}
	} else {
		if err := a.client.Do(ctx, http.MethodPut, "/cob/"+req.TxID, body, &resp); err != nil {
			return pixdomain.ChargeResult{}, err
		}
	}
	return mapper.FromC6CobResponse(resp)
}

// CreateWithDueDate creates a "cobrança com vencimento" via PUT /cobv/{txid}.
// Unlike /cob, C6 has no POST variant for cobv: txid is always caller-supplied.
func (a *PixAdapter) CreateWithDueDate(ctx context.Context, req pixdomain.CreateChargeWithDueDateRequest) (pixdomain.ChargeResult, error) {
	body, err := mapper.ToC6CreateCobVRequest(req)
	if err != nil {
		return pixdomain.ChargeResult{}, err
	}
	var resp dto.CobVResponse
	if err := a.client.Do(ctx, http.MethodPut, "/cobv/"+req.TxID, body, &resp); err != nil {
		return pixdomain.ChargeResult{}, err
	}
	return mapper.FromC6CobVResponse(resp)
}

// GetByTxID fetches an immediate charge (GET /cob/{txid}).
func (a *PixAdapter) GetByTxID(ctx context.Context, txID string) (pixdomain.ChargeResult, error) {
	var resp dto.CobResponse
	if err := a.client.Do(ctx, http.MethodGet, "/cob/"+txID, nil, &resp); err != nil {
		return pixdomain.ChargeResult{}, err
	}
	return mapper.FromC6CobResponse(resp)
}

// GetWithDueDateByTxID fetches a charge with a due date (GET /cobv/{txid}).
func (a *PixAdapter) GetWithDueDateByTxID(ctx context.Context, txID string) (pixdomain.ChargeResult, error) {
	var resp dto.CobVResponse
	if err := a.client.Do(ctx, http.MethodGet, "/cobv/"+txID, nil, &resp); err != nil {
		return pixdomain.ChargeResult{}, err
	}
	return mapper.FromC6CobVResponse(resp)
}

// ListImmediate lists immediate charges by creation date range (GET /cob).
// inicio/fim are required query parameters per the swagger.
func (a *PixAdapter) ListImmediate(ctx context.Context, req pixdomain.ListChargesRequest) (pixdomain.ListChargesResult, error) {
	q := url.Values{
		"inicio": {req.Start.UTC().Format(c6DateTimeQueryLayout)},
		"fim":    {req.End.UTC().Format(c6DateTimeQueryLayout)},
	}
	var resp dto.CobsConsultadasResponse
	if err := a.client.Do(ctx, http.MethodGet, "/cob?"+q.Encode(), nil, &resp); err != nil {
		return pixdomain.ListChargesResult{}, err
	}
	return mapper.FromC6CobsConsultadas(resp)
}

// ListWithDueDate lists charges with a due date by creation date range
// (GET /cobv).
func (a *PixAdapter) ListWithDueDate(ctx context.Context, req pixdomain.ListChargesRequest) (pixdomain.ListChargesResult, error) {
	q := url.Values{
		"inicio": {req.Start.UTC().Format(c6DateTimeQueryLayout)},
		"fim":    {req.End.UTC().Format(c6DateTimeQueryLayout)},
	}
	var resp dto.CobsVConsultadasResponse
	if err := a.client.Do(ctx, http.MethodGet, "/cobv?"+q.Encode(), nil, &resp); err != nil {
		return pixdomain.ListChargesResult{}, err
	}
	return mapper.FromC6CobsVConsultadas(resp)
}

// RegisterWebhook configures the PIX product's own webhook for a given PIX
// key (PUT /webhook/{chave}) — distinct from the generic webhook feature's
// provider registration used by checkout/boleto.
func (a *PixAdapter) RegisterWebhook(ctx context.Context, req pixdomain.RegisterWebhookRequest) error {
	body := mapper.ToC6WebhookSolicitado(req)
	return a.client.Do(ctx, http.MethodPut, "/webhook/"+url.PathEscape(req.PixKey), body, nil)
}
