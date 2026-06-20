// Package dto (this file) mirrors the JSON shapes of C6's PIX API
// (docs/c6/swagger/pix.yaml), kept in C6's own Portuguese vocabulary since
// that's what the wire format uses. Only internal/provider/c6/mapper is
// allowed to know both this vocabulary and the unified pix/domain one.
package dto

// PessoaFisica/PessoaJuridica are mutually exclusive shapes for "devedor":
// C6 requires exactly one of cpf or cnpj plus nome.
type PessoaFisica struct {
	CPF  string `json:"cpf"`
	Nome string `json:"nome"`
}

type PessoaJuridica struct {
	CNPJ string `json:"cnpj"`
	Nome string `json:"nome"`
}

// CobDevedor is the "devedor" shape on immediate charges: optional, and
// either a CPF or a CNPJ holder (never both).
type CobDevedor struct {
	CPF  string `json:"cpf,omitempty"`
	CNPJ string `json:"cnpj,omitempty"`
	Nome string `json:"nome,omitempty"`
}

// CobVDevedor additionally requires the postal address, per C6's
// CobVGerada.devedor schema (logradouro/cidade/uf/cep are required there,
// unlike the immediate-charge devedor).
type CobVDevedor struct {
	CPF        string `json:"cpf,omitempty"`
	CNPJ       string `json:"cnpj,omitempty"`
	Nome       string `json:"nome"`
	Logradouro string `json:"logradouro"`
	Cidade     string `json:"cidade"`
	UF         string `json:"uf"`
	CEP        string `json:"cep"`
}

// CobCalendario is the immediate-charge calendar: expiracao is the only
// field we send (criacao is server-assigned and only appears in responses).
type CobCalendario struct {
	Criacao   string `json:"criacao,omitempty"` // RFC3339, response-only
	Expiracao int    `json:"expiracao,omitempty"`
}

// CobVCalendario is the due-date charge calendar.
type CobVCalendario struct {
	Criacao                string `json:"criacao,omitempty"` // RFC3339, response-only
	DataDeVencimento       string `json:"dataDeVencimento"`  // YYYY-MM-DD
	ValidadeAposVencimento int    `json:"validadeAposVencimento,omitempty"`
}

// CobValor carries the charge amount. "original" is a decimal string per
// the pattern \d{1,10}\.\d{2} (e.g. "100.00") — C6 does not accept a
// floating point number here.
type CobValor struct {
	Original string `json:"original"`
}

// CreateCobRequest is the body for PUT /cob/{txid} and POST /cob.
type CreateCobRequest struct {
	Calendario         CobCalendario `json:"calendario"`
	Devedor            *CobDevedor   `json:"devedor,omitempty"`
	Valor              CobValor      `json:"valor"`
	Chave              string        `json:"chave"`
	SolicitacaoPagador string        `json:"solicitacaoPagador,omitempty"`
}

// CobResponse mirrors C6's CobGerada/CobCompleta — the response shape for
// create, get, and list-item entries alike.
type CobResponse struct {
	Calendario         CobCalendario `json:"calendario"`
	TxID               string        `json:"txid"`
	Revisao            int           `json:"revisao"`
	Location           string        `json:"location,omitempty"`
	Status             string        `json:"status"`
	Devedor            *CobDevedor   `json:"devedor,omitempty"`
	Valor              CobValor      `json:"valor"`
	Chave              string        `json:"chave"`
	SolicitacaoPagador string        `json:"solicitacaoPagador,omitempty"`
}

// CobsConsultadasResponse is the response for GET /cob (list).
type CobsConsultadasResponse struct {
	Parametros ParametrosConsultaCob `json:"parametros"`
	Cobs       []CobResponse         `json:"cobs"`
}

// CreateCobVRequest is the body for PUT /cobv/{txid}. Devedor and chave are
// always required for charges with a due date (unlike /cob).
type CreateCobVRequest struct {
	Calendario         CobVCalendario `json:"calendario"`
	Devedor            CobVDevedor    `json:"devedor"`
	Valor              CobValor       `json:"valor"`
	Chave              string         `json:"chave"`
	SolicitacaoPagador string         `json:"solicitacaoPagador,omitempty"`
}

// CobVResponse mirrors C6's CobVGerada/CobVCompleta.
type CobVResponse struct {
	Calendario         CobVCalendario `json:"calendario"`
	TxID               string         `json:"txid"`
	Revisao            int            `json:"revisao"`
	Location           string         `json:"location,omitempty"`
	Status             string         `json:"status"`
	Devedor            CobVDevedor    `json:"devedor"`
	Valor              CobValor       `json:"valor"`
	Chave              string         `json:"chave"`
	SolicitacaoPagador string         `json:"solicitacaoPagador,omitempty"`
}

// CobsVConsultadasResponse is the response for GET /cobv (list).
type CobsVConsultadasResponse struct {
	Parametros ParametrosConsultaCob `json:"parametros"`
	Cobs       []CobVResponse        `json:"cobs"`
}

// ParametrosConsultaCob echoes the query parameters C6 applied to a list
// request, including server-side pagination info.
type ParametrosConsultaCob struct {
	Inicio    string        `json:"inicio"`
	Fim       string        `json:"fim"`
	Paginacao PaginacaoEcho `json:"paginacao"`
}

type PaginacaoEcho struct {
	PaginaAtual            int `json:"paginaAtual"`
	ItensPorPagina         int `json:"itensPorPagina"`
	QuantidadeDePaginas    int `json:"quantidadeDePaginas"`
	QuantidadeTotalDeItens int `json:"quantidadeTotalDeItens"`
}

// WebhookSolicitado is the body for PUT /webhook/{chave}.
type WebhookSolicitado struct {
	WebhookURL string `json:"webhookUrl"`
}

// WebhookCompleto is the response for GET /webhook/{chave}.
type WebhookCompleto struct {
	WebhookURL string `json:"webhookUrl"`
	Chave      string `json:"chave"`
	Criacao    string `json:"criacao"`
}
