package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/upwifi/banking/internal/boleto/domain"
	"github.com/upwifi/banking/internal/boleto/service"
)

const dateLayout = "2006-01-02"

// Handler exposes the unified bank slip (boleto) HTTP API. Every request
// carries a "baas" field (or query param for GET/PUT) selecting which
// provider adapter handles it.
type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// Register wires routes onto mux using Go 1.22+ method-aware patterns.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/bank-slips", h.create)
	mux.HandleFunc("GET /v1/bank-slips/{id}", h.get)
	mux.HandleFunc("PUT /v1/bank-slips/{id}", h.update)
	mux.HandleFunc("PUT /v1/bank-slips/{id}/cancel", h.cancel)
}

type addressPayload struct {
	Street     string `json:"street"`
	Number     string `json:"number"`
	Complement string `json:"complement"`
	City       string `json:"city"`
	State      string `json:"state"`
	ZipCode    string `json:"zip_code"`
}

type payerPayload struct {
	Name    string         `json:"name"`
	TaxID   string         `json:"tax_id"`
	Email   string         `json:"email"`
	Address addressPayload `json:"address"`
}

type discountTierPayload struct {
	Type         string `json:"type"`
	Value        int64  `json:"value"`
	DeadlineDays int    `json:"deadline_days"`
}

type discountPayload struct {
	Tiers []discountTierPayload `json:"tiers"`
}

type finePayload struct {
	Type         string `json:"type"`
	Value        int64  `json:"value"`
	DeadlineDays int    `json:"deadline_days"`
}

type interestPayload struct {
	Type         string `json:"type"`
	Value        int64  `json:"value"`
	DeadlineDays int    `json:"deadline_days"`
}

func (p *discountPayload) toDomain() *domain.Discount {
	if p == nil || len(p.Tiers) == 0 {
		return nil
	}
	d := &domain.Discount{}
	for _, t := range p.Tiers {
		d.Tiers = append(d.Tiers, domain.DiscountTier{
			Type:         domain.AmountType(t.Type),
			Value:        t.Value,
			DeadlineDays: t.DeadlineDays,
		})
	}
	return d
}

func fromDomainDiscount(d *domain.Discount) *discountPayload {
	if d == nil {
		return nil
	}
	out := &discountPayload{}
	for _, t := range d.Tiers {
		out.Tiers = append(out.Tiers, discountTierPayload{
			Type: string(t.Type), Value: t.Value, DeadlineDays: t.DeadlineDays,
		})
	}
	return out
}

func (p *finePayload) toDomain() *domain.Fine {
	if p == nil {
		return nil
	}
	return &domain.Fine{Type: domain.AmountType(p.Type), Value: p.Value, DeadlineDays: p.DeadlineDays}
}

func fromDomainFine(f *domain.Fine) *finePayload {
	if f == nil {
		return nil
	}
	return &finePayload{Type: string(f.Type), Value: f.Value, DeadlineDays: f.DeadlineDays}
}

func (p *interestPayload) toDomain() *domain.Interest {
	if p == nil {
		return nil
	}
	return &domain.Interest{Type: domain.AmountType(p.Type), Value: p.Value, DeadlineDays: p.DeadlineDays}
}

func fromDomainInterest(i *domain.Interest) *interestPayload {
	if i == nil {
		return nil
	}
	return &interestPayload{Type: string(i.Type), Value: i.Value, DeadlineDays: i.DeadlineDays}
}

func (p payerPayload) toDomain() domain.Payer {
	return domain.Payer{
		Name: p.Name, TaxID: p.TaxID, Email: p.Email,
		Address: domain.Address{
			Street: p.Address.Street, Number: p.Address.Number, Complement: p.Address.Complement,
			City: p.Address.City, State: p.Address.State, ZipCode: p.Address.ZipCode,
		},
	}
}

type createBankSlipPayload struct {
	BaaS                string           `json:"baas"`
	ExternalReferenceID string           `json:"external_reference_id"`
	AmountCents         int64            `json:"amount_cents"`
	Currency            string           `json:"currency"`
	DueDate             string           `json:"due_date"`
	Instructions        []string         `json:"instructions"`
	Discount            *discountPayload `json:"discount"`
	Interest            *interestPayload `json:"interest"`
	Fine                *finePayload     `json:"fine"`
	Payer               payerPayload     `json:"payer"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var payload createBankSlipPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.BaaS == "" {
		writeError(w, http.StatusBadRequest, "baas is required")
		return
	}
	dueDate, err := time.Parse(dateLayout, payload.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_date must be in YYYY-MM-DD format")
		return
	}

	currency := payload.Currency
	if currency == "" {
		currency = "BRL"
	}

	req := domain.CreateBankSlipRequest{
		BaaS:                domain.BaaS(payload.BaaS),
		ExternalReferenceID: payload.ExternalReferenceID,
		Amount:              domain.Money{AmountCents: payload.AmountCents, Currency: currency},
		DueDate:             dueDate,
		Instructions:        payload.Instructions,
		Discount:            payload.Discount.toDomain(),
		Interest:            payload.Interest.toDomain(),
		Fine:                payload.Fine.toDomain(),
		Payer:               payload.Payer.toDomain(),
	}

	result, err := h.svc.CreateBankSlip(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, createBankSlipResponse{
		ID:            result.ProviderBankSlipID,
		OurNumber:     result.OurNumber,
		BarCode:       result.BarCode,
		DigitableLine: result.DigitableLine,
		AmountCents:   result.Amount.AmountCents,
		Currency:      result.Amount.Currency,
		DueDate:       result.DueDate.Format(dateLayout),
		Status:        string(result.Status),
	})
}

type createBankSlipResponse struct {
	ID            string `json:"id"`
	OurNumber     string `json:"our_number"`
	BarCode       string `json:"bar_code"`
	DigitableLine string `json:"digitable_line"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	DueDate       string `json:"due_date"`
	Status        string `json:"status"`
}

type paymentPayload struct {
	Date        string `json:"date"`
	AmountCents int64  `json:"amount_cents"`
}

type bankSlipDetailsResponse struct {
	ID                  string           `json:"id"`
	ExternalReferenceID string           `json:"external_reference_id"`
	Status              string           `json:"status"`
	AmountCents         int64            `json:"amount_cents"`
	Currency            string           `json:"currency"`
	DueDate             string           `json:"due_date"`
	Instructions        []string         `json:"instructions,omitempty"`
	Discount            *discountPayload `json:"discount,omitempty"`
	Interest            *interestPayload `json:"interest,omitempty"`
	Fine                *finePayload     `json:"fine,omitempty"`
	OurNumber           string           `json:"our_number"`
	BarCode             string           `json:"bar_code"`
	DigitableLine       string           `json:"digitable_line"`
	Payments            []paymentPayload `json:"payments,omitempty"`
}

func detailsToResponse(details domain.BankSlipDetails) bankSlipDetailsResponse {
	resp := bankSlipDetailsResponse{
		ID:                  details.ProviderBankSlipID,
		ExternalReferenceID: details.ExternalReferenceID,
		Status:              string(details.Status),
		AmountCents:         details.Amount.AmountCents,
		Currency:            details.Amount.Currency,
		DueDate:             details.DueDate.Format(dateLayout),
		Instructions:        details.Instructions,
		Discount:            fromDomainDiscount(details.Discount),
		Interest:            fromDomainInterest(details.Interest),
		Fine:                fromDomainFine(details.Fine),
		OurNumber:           details.OurNumber,
		BarCode:             details.BarCode,
		DigitableLine:       details.DigitableLine,
	}
	for _, p := range details.Payments {
		resp.Payments = append(resp.Payments, paymentPayload{
			Date:        p.Date.Format(dateLayout),
			AmountCents: p.Amount.AmountCents,
		})
	}
	return resp
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	baas := r.URL.Query().Get("baas")
	if baas == "" {
		writeError(w, http.StatusBadRequest, "baas query parameter is required")
		return
	}

	details, err := h.svc.GetBankSlip(r.Context(), domain.BaaS(baas), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detailsToResponse(details))
}

type updateBankSlipPayload struct {
	BaaS        string           `json:"baas"`
	AmountCents *int64           `json:"amount_cents"`
	Currency    string           `json:"currency"`
	DueDate     *string          `json:"due_date"`
	Discount    *discountPayload `json:"discount"`
	Interest    *interestPayload `json:"interest"`
	Fine        *finePayload     `json:"fine"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var payload updateBankSlipPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.BaaS == "" {
		writeError(w, http.StatusBadRequest, "baas is required")
		return
	}

	req := domain.UpdateBankSlipRequest{
		Discount: payload.Discount.toDomain(),
		Interest: payload.Interest.toDomain(),
		Fine:     payload.Fine.toDomain(),
	}
	if payload.AmountCents != nil {
		currency := payload.Currency
		if currency == "" {
			currency = "BRL"
		}
		req.Amount = &domain.Money{AmountCents: *payload.AmountCents, Currency: currency}
	}
	if payload.DueDate != nil {
		dueDate, err := time.Parse(dateLayout, *payload.DueDate)
		if err != nil {
			writeError(w, http.StatusBadRequest, "due_date must be in YYYY-MM-DD format")
			return
		}
		req.DueDate = &dueDate
	}

	details, err := h.svc.UpdateBankSlip(r.Context(), domain.BaaS(payload.BaaS), id, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detailsToResponse(details))
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	baas := r.URL.Query().Get("baas")
	if baas == "" {
		writeError(w, http.StatusBadRequest, "baas query parameter is required")
		return
	}

	if err := h.svc.CancelBankSlip(r.Context(), domain.BaaS(baas), id); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
