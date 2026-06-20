// Package mapper (this file) holds pure functions translating between our
// unified PIX domain model and C6's DTOs. Kept side-effect free and
// isolated so they can be table-driven tested without any HTTP/DB
// dependency, matching the convention in mapper/checkout.go.
package mapper

import (
	"fmt"
	"strconv"
	"time"

	"github.com/upwifi/banking/internal/pix/domain"
	"github.com/upwifi/banking/internal/provider/c6/dto"
)

const (
	c6DateLayout     = "2006-01-02"
	c6DateTimeLayout = time.RFC3339
)

// ToC6Amount renders cents as C6's required decimal string format, e.g.
// 10000 -> "100.00". C6 validates this against the pattern \d{1,10}\.\d{2}
// and rejects a bare JSON number for PIX endpoints (unlike checkout, which
// takes a float).
func ToC6Amount(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// FromC6Amount parses C6's decimal amount string back into cents.
func FromC6Amount(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("c6/mapper: parse amount %q: %w", s, err)
	}
	return int64(f*100 + 0.5), nil
}

// FromC6PixStatus translates C6's CobrancaStatus enum (PIX charges) to our
// unified pix/domain.Status. Named distinctly from checkout's FromC6Status
// since both live in this shared mapper package but translate different
// status vocabularies.
func FromC6PixStatus(s string) domain.Status {
	switch s {
	case "ATIVA":
		return domain.StatusActive
	case "CONCLUIDA":
		return domain.StatusCompleted
	case "REMOVIDA_PELO_USUARIO_RECEBEDOR":
		return domain.StatusRemovedByReceiver
	case "REMOVIDA_PELO_PSP":
		return domain.StatusRemovedByPSP
	default:
		return domain.Status(s)
	}
}

// ToC6CobDevedor builds the optional "devedor" object for an immediate
// charge. CPF and CNPJ are mutually exclusive per C6's PessoaFisica/
// PessoaJuridica oneOf.
func ToC6CobDevedor(d *domain.Debtor) (*dto.CobDevedor, error) {
	if d == nil {
		return nil, nil
	}
	cd := &dto.CobDevedor{Nome: d.Name}
	switch d.DocumentType {
	case domain.DebtorCPF:
		cd.CPF = d.DocumentID
	case domain.DebtorCNPJ:
		cd.CNPJ = d.DocumentID
	default:
		return nil, fmt.Errorf("c6/mapper: unknown debtor document type %q", d.DocumentType)
	}
	return cd, nil
}

func fromC6CobDevedor(d *dto.CobDevedor) *domain.Debtor {
	if d == nil {
		return nil
	}
	debtor := &domain.Debtor{Name: d.Nome}
	if d.CPF != "" {
		debtor.DocumentType = domain.DebtorCPF
		debtor.DocumentID = d.CPF
	} else if d.CNPJ != "" {
		debtor.DocumentType = domain.DebtorCNPJ
		debtor.DocumentID = d.CNPJ
	}
	return debtor
}

// ToC6CobVDevedor builds the required "devedor" object for a charge with a
// due date, which additionally requires the postal address fields.
func ToC6CobVDevedor(d domain.Debtor) (dto.CobVDevedor, error) {
	cd := dto.CobVDevedor{
		Nome:       d.Name,
		Logradouro: d.Street,
		Cidade:     d.City,
		UF:         d.State,
		CEP:        d.ZipCode,
	}
	switch d.DocumentType {
	case domain.DebtorCPF:
		cd.CPF = d.DocumentID
	case domain.DebtorCNPJ:
		cd.CNPJ = d.DocumentID
	default:
		return dto.CobVDevedor{}, fmt.Errorf("c6/mapper: unknown debtor document type %q", d.DocumentType)
	}
	return cd, nil
}

func fromC6CobVDevedor(d dto.CobVDevedor) domain.Debtor {
	debtor := domain.Debtor{
		Name:    d.Nome,
		Street:  d.Logradouro,
		City:    d.Cidade,
		State:   d.UF,
		ZipCode: d.CEP,
	}
	if d.CPF != "" {
		debtor.DocumentType = domain.DebtorCPF
		debtor.DocumentID = d.CPF
	} else if d.CNPJ != "" {
		debtor.DocumentType = domain.DebtorCNPJ
		debtor.DocumentID = d.CNPJ
	}
	return debtor
}

// ToC6CreateCobRequest builds the body for PUT /cob/{txid} or POST /cob.
func ToC6CreateCobRequest(req domain.CreateImmediateChargeRequest) (dto.CreateCobRequest, error) {
	devedor, err := ToC6CobDevedor(req.Debtor)
	if err != nil {
		return dto.CreateCobRequest{}, err
	}
	return dto.CreateCobRequest{
		Calendario:         dto.CobCalendario{Expiracao: req.ExpirationSeconds},
		Devedor:            devedor,
		Valor:              dto.CobValor{Original: ToC6Amount(req.Amount.AmountCents)},
		Chave:              req.PixKey,
		SolicitacaoPagador: req.PayerRequest,
	}, nil
}

// FromC6CobResponse builds our unified charge result from C6's CobGerada/
// CobCompleta response shape.
func FromC6CobResponse(resp dto.CobResponse) (domain.ChargeResult, error) {
	result := domain.ChargeResult{
		TxID:              resp.TxID,
		Kind:              domain.ChargeImmediate,
		Status:            FromC6PixStatus(resp.Status),
		PixKey:            resp.Chave,
		Revision:          resp.Revisao,
		Location:          resp.Location,
		ExpirationSeconds: resp.Calendario.Expiracao,
		Debtor:            fromC6CobDevedor(resp.Devedor),
	}
	cents, err := FromC6Amount(resp.Valor.Original)
	if err != nil {
		return domain.ChargeResult{}, err
	}
	result.Amount = domain.Money{AmountCents: cents}
	if resp.Calendario.Criacao != "" {
		t, err := time.Parse(c6DateTimeLayout, resp.Calendario.Criacao)
		if err == nil {
			result.CreatedAt = t
		}
	}
	return result, nil
}

// ToC6CreateCobVRequest builds the body for PUT /cobv/{txid}.
func ToC6CreateCobVRequest(req domain.CreateChargeWithDueDateRequest) (dto.CreateCobVRequest, error) {
	devedor, err := ToC6CobVDevedor(req.Debtor)
	if err != nil {
		return dto.CreateCobVRequest{}, err
	}
	return dto.CreateCobVRequest{
		Calendario: dto.CobVCalendario{
			DataDeVencimento:       req.DueDate.Format(c6DateLayout),
			ValidadeAposVencimento: req.DaysValidAfterDueDate,
		},
		Devedor:            devedor,
		Valor:              dto.CobValor{Original: ToC6Amount(req.Amount.AmountCents)},
		Chave:              req.PixKey,
		SolicitacaoPagador: req.PayerRequest,
	}, nil
}

// FromC6CobVResponse builds our unified charge result from C6's CobVGerada/
// CobVCompleta response shape.
func FromC6CobVResponse(resp dto.CobVResponse) (domain.ChargeResult, error) {
	result := domain.ChargeResult{
		TxID:     resp.TxID,
		Kind:     domain.ChargeWithDueDate,
		Status:   FromC6PixStatus(resp.Status),
		PixKey:   resp.Chave,
		Revision: resp.Revisao,
		Location: resp.Location,
	}
	debtor := fromC6CobVDevedor(resp.Devedor)
	result.Debtor = &debtor

	cents, err := FromC6Amount(resp.Valor.Original)
	if err != nil {
		return domain.ChargeResult{}, err
	}
	result.Amount = domain.Money{AmountCents: cents}

	if resp.Calendario.Criacao != "" {
		if t, err := time.Parse(c6DateTimeLayout, resp.Calendario.Criacao); err == nil {
			result.CreatedAt = t
		}
	}
	if resp.Calendario.DataDeVencimento != "" {
		if t, err := time.Parse(c6DateLayout, resp.Calendario.DataDeVencimento); err == nil {
			result.DueDate = &t
		}
	}
	return result, nil
}

// FromC6CobsConsultadas translates a GET /cob list response.
func FromC6CobsConsultadas(resp dto.CobsConsultadasResponse) (domain.ListChargesResult, error) {
	out := domain.ListChargesResult{Charges: make([]domain.ChargeResult, 0, len(resp.Cobs))}
	for _, c := range resp.Cobs {
		charge, err := FromC6CobResponse(c)
		if err != nil {
			return domain.ListChargesResult{}, err
		}
		out.Charges = append(out.Charges, charge)
	}
	return out, nil
}

// FromC6CobsVConsultadas translates a GET /cobv list response.
func FromC6CobsVConsultadas(resp dto.CobsVConsultadasResponse) (domain.ListChargesResult, error) {
	out := domain.ListChargesResult{Charges: make([]domain.ChargeResult, 0, len(resp.Cobs))}
	for _, c := range resp.Cobs {
		charge, err := FromC6CobVResponse(c)
		if err != nil {
			return domain.ListChargesResult{}, err
		}
		out.Charges = append(out.Charges, charge)
	}
	return out, nil
}

// ToC6WebhookSolicitado builds the body for PUT /webhook/{chave}.
func ToC6WebhookSolicitado(req domain.RegisterWebhookRequest) dto.WebhookSolicitado {
	return dto.WebhookSolicitado{WebhookURL: req.URL}
}
