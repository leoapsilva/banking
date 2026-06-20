package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/upwifi/banking/internal/pix/domain"
	"github.com/upwifi/banking/internal/pix/service"
)

// Handler exposes the unified PIX HTTP API. Every request carries a "baas"
// field selecting which provider adapter handles it, matching the
// convention in internal/checkout/handler.
type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// Register wires routes onto mux using Go 1.22+ method-aware patterns.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/pix/charges", h.create)
	mux.HandleFunc("GET /v1/pix/charges/{txid}", h.get)
	mux.HandleFunc("GET /v1/pix/charges", h.list)
	mux.HandleFunc("POST /v1/pix/webhook", h.registerWebhook)
}

type debtorPayload struct {
	DocumentType string `json:"document_type"` // "CPF" or "CNPJ"
	DocumentID   string `json:"document_id"`
	Name         string `json:"name"`
	Street       string `json:"street"`
	City         string `json:"city"`
	State        string `json:"state"`
	ZipCode      string `json:"zip_code"`
}

func (p *debtorPayload) toDomain() *domain.Debtor {
	if p == nil {
		return nil
	}
	return &domain.Debtor{
		DocumentType: domain.DebtorDocumentType(p.DocumentType),
		DocumentID:   p.DocumentID,
		Name:         p.Name,
		Street:       p.Street,
		City:         p.City,
		State:        p.State,
		ZipCode:      p.ZipCode,
	}
}

// createChargePayload covers both creation flows (items 7.1/7.2/7.5): when
// due_date is informed, the request is routed to "cobrança com vencimento"
// (POST /cobv-equivalent); otherwise it's an immediate charge.
type createChargePayload struct {
	BaaS              string         `json:"baas"`
	TxID              string         `json:"txid"`
	AmountCents       int64          `json:"amount_cents"`
	PixKey            string         `json:"pix_key"`
	ExpirationSeconds int            `json:"expiration_seconds"`
	DueDate           string         `json:"due_date"` // YYYY-MM-DD; presence selects the cobv flow
	DaysValidAfterDue int            `json:"days_valid_after_due_date"`
	Debtor            *debtorPayload `json:"debtor"`
	PayerRequest      string         `json:"payer_request"`
}

type chargeResponse struct {
	TxID              string  `json:"txid"`
	Kind              string  `json:"kind"`
	Status            string  `json:"status"`
	AmountCents       int64   `json:"amount_cents"`
	PixKey            string  `json:"pix_key"`
	Revision          int     `json:"revision"`
	Location          string  `json:"location,omitempty"`
	ExpirationSeconds int     `json:"expiration_seconds,omitempty"`
	DueDate           *string `json:"due_date,omitempty"`
}

func toChargeResponse(c domain.ChargeResult) chargeResponse {
	resp := chargeResponse{
		TxID:              c.TxID,
		Kind:              string(c.Kind),
		Status:            string(c.Status),
		AmountCents:       c.Amount.AmountCents,
		PixKey:            c.PixKey,
		Revision:          c.Revision,
		Location:          c.Location,
		ExpirationSeconds: c.ExpirationSeconds,
	}
	if c.DueDate != nil {
		s := c.DueDate.Format("2006-01-02")
		resp.DueDate = &s
	}
	return resp
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var payload createChargePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.BaaS == "" {
		writeError(w, http.StatusBadRequest, "baas is required")
		return
	}

	if payload.DueDate != "" {
		dueDate, err := time.Parse("2006-01-02", payload.DueDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "due_date must be in YYYY-MM-DD format")
			return
		}
		var debtor domain.Debtor
		if payload.Debtor != nil {
			debtor = *payload.Debtor.toDomain()
		}
		req := domain.CreateChargeWithDueDateRequest{
			BaaS:                  domain.BaaS(payload.BaaS),
			TxID:                  payload.TxID,
			Amount:                domain.Money{AmountCents: payload.AmountCents},
			PixKey:                payload.PixKey,
			DueDate:               dueDate,
			DaysValidAfterDueDate: payload.DaysValidAfterDue,
			Debtor:                debtor,
			PayerRequest:          payload.PayerRequest,
		}
		result, err := h.svc.CreateChargeWithDueDate(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, toChargeResponse(result))
		return
	}

	req := domain.CreateImmediateChargeRequest{
		BaaS:              domain.BaaS(payload.BaaS),
		TxID:              payload.TxID,
		Amount:            domain.Money{AmountCents: payload.AmountCents},
		PixKey:            payload.PixKey,
		ExpirationSeconds: payload.ExpirationSeconds,
		Debtor:            payload.Debtor.toDomain(),
		PayerRequest:      payload.PayerRequest,
	}
	result, err := h.svc.CreateImmediateCharge(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toChargeResponse(result))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	txID := r.PathValue("txid")
	baas := r.URL.Query().Get("baas")
	if baas == "" {
		writeError(w, http.StatusBadRequest, "baas query parameter is required")
		return
	}

	// "kind" selects whether to consult /cob or /cobv for this txid (items
	// 7.3 vs 7.7). Defaults to immediate, the more common case.
	kind := r.URL.Query().Get("kind")
	var (
		result domain.ChargeResult
		err    error
	)
	if kind == "with_due_date" {
		result, err = h.svc.GetChargeWithDueDate(r.Context(), domain.BaaS(baas), txID)
	} else {
		result, err = h.svc.GetImmediateCharge(r.Context(), domain.BaaS(baas), txID)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toChargeResponse(result))
}

type listChargesResponse struct {
	Charges []chargeResponse `json:"charges"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	baas := q.Get("baas")
	if baas == "" {
		writeError(w, http.StatusBadRequest, "baas query parameter is required")
		return
	}
	start, err := time.Parse(time.RFC3339, q.Get("start"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "start must be an RFC3339 timestamp")
		return
	}
	end, err := time.Parse(time.RFC3339, q.Get("end"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "end must be an RFC3339 timestamp")
		return
	}

	req := domain.ListChargesRequest{BaaS: domain.BaaS(baas), Start: start, End: end}

	// "kind" selects /cob vs /cobv listing (items 7.4 vs 7.6).
	var result domain.ListChargesResult
	if q.Get("kind") == "with_due_date" {
		result, err = h.svc.ListChargesWithDueDate(r.Context(), req)
	} else {
		result, err = h.svc.ListImmediateCharges(r.Context(), req)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := listChargesResponse{Charges: make([]chargeResponse, 0, len(result.Charges))}
	for _, c := range result.Charges {
		resp.Charges = append(resp.Charges, toChargeResponse(c))
	}
	writeJSON(w, http.StatusOK, resp)
}

type registerWebhookPayload struct {
	BaaS   string `json:"baas"`
	PixKey string `json:"pix_key"`
	URL    string `json:"url"`
}

// registerWebhook handles item 7.8: configures the PIX-specific webhook
// (PUT /webhook/{chave} on C6), not the generic internal/webhook feature.
func (h *Handler) registerWebhook(w http.ResponseWriter, r *http.Request) {
	var payload registerWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.BaaS == "" {
		writeError(w, http.StatusBadRequest, "baas is required")
		return
	}

	err := h.svc.RegisterWebhook(r.Context(), domain.RegisterWebhookRequest{
		BaaS:   domain.BaaS(payload.BaaS),
		PixKey: payload.PixKey,
		URL:    payload.URL,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
