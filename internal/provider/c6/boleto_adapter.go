package c6

import (
	"context"
	"net/http"

	boletodomain "github.com/upwifi/banking/internal/boleto/domain"
	"github.com/upwifi/banking/internal/provider/c6/client"
	"github.com/upwifi/banking/internal/provider/c6/dto"
	"github.com/upwifi/banking/internal/provider/c6/mapper"
)

// BoletoAdapter implements boleto.Provider against the C6 Bank Slip API.
type BoletoAdapter struct {
	client *client.Client
}

func NewBoletoAdapter(c *client.Client) *BoletoAdapter {
	return &BoletoAdapter{client: c}
}

func (a *BoletoAdapter) Create(ctx context.Context, req boletodomain.CreateBankSlipRequest) (boletodomain.BankSlipResult, error) {
	body, err := mapper.ToC6CreateBankSlipRequest(req)
	if err != nil {
		return boletodomain.BankSlipResult{}, err
	}
	var resp dto.CreateBankSlipResponse
	if err := a.client.Do(ctx, http.MethodPost, "/", body, &resp); err != nil {
		return boletodomain.BankSlipResult{}, err
	}
	return mapper.FromC6CreateBankSlipResponse(resp)
}

func (a *BoletoAdapter) Get(ctx context.Context, providerBankSlipID string) (boletodomain.BankSlipDetails, error) {
	var resp dto.BankSlipResponse
	if err := a.client.Do(ctx, http.MethodGet, "/"+providerBankSlipID, nil, &resp); err != nil {
		return boletodomain.BankSlipDetails{}, err
	}
	return mapper.FromC6BankSlipResponse(resp)
}

// GetPDF returns the raw PDF bytes for a bank slip. C6's /pdf endpoint
// streams the PDF directly (application/pdf) rather than a JSON base64
// wrapper, so we fetch the raw body.
func (a *BoletoAdapter) GetPDF(ctx context.Context, providerBankSlipID string) ([]byte, error) {
	return a.client.DoRaw(ctx, http.MethodGet, "/"+providerBankSlipID+"/pdf")
}

func (a *BoletoAdapter) Update(ctx context.Context, providerBankSlipID string, req boletodomain.UpdateBankSlipRequest) (boletodomain.BankSlipDetails, error) {
	body, err := mapper.ToC6AlterBankSlipRequest(req)
	if err != nil {
		return boletodomain.BankSlipDetails{}, err
	}
	var resp dto.BankSlipResponse
	if err := a.client.Do(ctx, http.MethodPut, "/"+providerBankSlipID, body, &resp); err != nil {
		return boletodomain.BankSlipDetails{}, err
	}
	return mapper.FromC6BankSlipResponse(resp)
}

func (a *BoletoAdapter) Cancel(ctx context.Context, providerBankSlipID string) error {
	return a.client.Do(ctx, http.MethodPut, "/"+providerBankSlipID+"/cancel", nil, nil)
}
