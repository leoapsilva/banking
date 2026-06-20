package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/upwifi/banking/internal/paymentscheduling/domain"
	"github.com/upwifi/banking/internal/paymentscheduling/service"
)

const dateLayout = "2006-01-02"

// Handler exposes the unified DDA/payment-scheduling HTTP API. Every
// request carries a "baas" field (or query param for GET/DELETE) selecting
// which provider adapter handles it.
type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// Register wires routes onto mux using Go 1.22+ method-aware patterns.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/payment-scheduling/dda", h.getDDA)
	mux.HandleFunc("POST /v1/payment-scheduling/groups", h.submitForDecode)
	mux.HandleFunc("GET /v1/payment-scheduling/groups/{group_id}/items", h.getGroupItems)
	mux.HandleFunc("DELETE /v1/payment-scheduling/groups/{group_id}/items", h.removeItems)
	mux.HandleFunc("DELETE /v1/payment-scheduling/groups/{group_id}/items/{item_id}", h.removeItem)
	mux.HandleFunc("POST /v1/payment-scheduling/groups/{group_id}/submit", h.submitForApproval)
}

type ddaBondResponse struct {
	AmountCents     int64  `json:"amount_cents"`
	BankCode        string `json:"bank_code"`
	BankName        string `json:"bank_name"`
	BeneficiaryName string `json:"beneficiary_name"`
	Content         string `json:"content"`
	DueDate         string `json:"due_date"`
	Overdue         bool   `json:"overdue"`
	PayerName       string `json:"payer_name"`
}

func (h *Handler) getDDA(w http.ResponseWriter, r *http.Request) {
	baas := r.URL.Query().Get("baas")
	if baas == "" {
		writeError(w, http.StatusBadRequest, "baas query parameter is required")
		return
	}
	bonds, err := h.svc.GetDDA(r.Context(), domain.BaaS(baas))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	resp := make([]ddaBondResponse, 0, len(bonds))
	for _, b := range bonds {
		resp = append(resp, ddaBondResponse{
			AmountCents:     b.Amount.AmountCents,
			BankCode:        b.BankCode,
			BankName:        b.BankName,
			BeneficiaryName: b.BeneficiaryName,
			Content:         b.Content,
			DueDate:         b.DueDate.Format(dateLayout),
			Overdue:         b.Overdue,
			PayerName:       b.PayerName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": resp})
}

type paymentInputPayload struct {
	Content         string  `json:"content"`
	AmountCents     int64   `json:"amount_cents"`
	BankCode        string  `json:"bank_code"`
	BankName        string  `json:"bank_name"`
	BeneficiaryName string  `json:"beneficiary_name"`
	Description     string  `json:"description"`
	PayerName       string  `json:"payer_name"`
	TransactionDate *string `json:"transaction_date"`
}

func (p paymentInputPayload) toDomain() (domain.PaymentInput, error) {
	input := domain.PaymentInput{
		Content:         p.Content,
		Amount:          domain.Money{AmountCents: p.AmountCents},
		BankCode:        p.BankCode,
		BankName:        p.BankName,
		BeneficiaryName: p.BeneficiaryName,
		Description:     p.Description,
		PayerName:       p.PayerName,
	}
	if p.TransactionDate != nil {
		t, err := time.Parse(dateLayout, *p.TransactionDate)
		if err != nil {
			return domain.PaymentInput{}, err
		}
		input.TransactionDate = &t
	}
	return input, nil
}

type submitForDecodePayload struct {
	BaaS  string                `json:"baas"`
	Items []paymentInputPayload `json:"items"`
}

func (h *Handler) submitForDecode(w http.ResponseWriter, r *http.Request) {
	var payload submitForDecodePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.BaaS == "" {
		writeError(w, http.StatusBadRequest, "baas is required")
		return
	}

	items := make([]domain.PaymentInput, 0, len(payload.Items))
	for _, p := range payload.Items {
		item, err := p.toDomain()
		if err != nil {
			writeError(w, http.StatusBadRequest, "transaction_date must be in YYYY-MM-DD format")
			return
		}
		items = append(items, item)
	}

	result, err := h.svc.SubmitForDecode(r.Context(), domain.SubmitForDecodeRequest{
		BaaS:  domain.BaaS(payload.BaaS),
		Items: items,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"group_id": result.GroupID})
}

type paymentItemResponse struct {
	ID              string  `json:"id"`
	GroupID         string  `json:"group_id"`
	Content         string  `json:"content"`
	AmountCents     int64   `json:"amount_cents"`
	BankCode        string  `json:"bank_code"`
	BankName        string  `json:"bank_name"`
	BeneficiaryName string  `json:"beneficiary_name"`
	Description     string  `json:"description"`
	PayerName       string  `json:"payer_name"`
	DueDate         *string `json:"due_date,omitempty"`
	TransactionDate *string `json:"transaction_date,omitempty"`
	Overdue         *bool   `json:"overdue,omitempty"`
	ProductType     string  `json:"product_type,omitempty"`
	Status          string  `json:"status"`
	ErrorMessage    string  `json:"error_message,omitempty"`
}

func toItemResponse(item domain.PaymentItem) paymentItemResponse {
	resp := paymentItemResponse{
		ID:              item.ID,
		GroupID:         item.GroupID,
		Content:         item.Content,
		AmountCents:     item.Amount.AmountCents,
		BankCode:        item.BankCode,
		BankName:        item.BankName,
		BeneficiaryName: item.BeneficiaryName,
		Description:     item.Description,
		PayerName:       item.PayerName,
		ProductType:     string(item.ProductType),
		Status:          string(item.Status),
		ErrorMessage:    item.ErrorMessage,
	}
	if item.DueDate != nil {
		s := item.DueDate.Format(dateLayout)
		resp.DueDate = &s
	}
	if item.TransactionDate != nil {
		s := item.TransactionDate.Format(dateLayout)
		resp.TransactionDate = &s
	}
	resp.Overdue = item.Overdue
	return resp
}

func (h *Handler) getGroupItems(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("group_id")
	baas := r.URL.Query().Get("baas")
	if baas == "" {
		writeError(w, http.StatusBadRequest, "baas query parameter is required")
		return
	}
	items, err := h.svc.GetGroupItems(r.Context(), domain.BaaS(baas), groupID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	resp := make([]paymentItemResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toItemResponse(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": resp})
}

type removeItemsPayload struct {
	BaaS    string   `json:"baas"`
	ItemIDs []string `json:"item_ids"`
}

func (h *Handler) removeItems(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("group_id")
	var payload removeItemsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.BaaS == "" {
		writeError(w, http.StatusBadRequest, "baas is required")
		return
	}
	if err := h.svc.RemoveItems(r.Context(), domain.BaaS(payload.BaaS), groupID, payload.ItemIDs); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) removeItem(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("group_id")
	itemID := r.PathValue("item_id")
	baas := r.URL.Query().Get("baas")
	if baas == "" {
		writeError(w, http.StatusBadRequest, "baas query parameter is required")
		return
	}
	if err := h.svc.RemoveItem(r.Context(), domain.BaaS(baas), groupID, itemID); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type submitForApprovalPayload struct {
	BaaS         string `json:"baas"`
	UploaderName string `json:"uploader_name"`
}

func (h *Handler) submitForApproval(w http.ResponseWriter, r *http.Request) {
	groupID := r.PathValue("group_id")
	var payload submitForApprovalPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.BaaS == "" {
		writeError(w, http.StatusBadRequest, "baas is required")
		return
	}
	err := h.svc.SubmitForApproval(r.Context(), domain.SubmitForApprovalRequest{
		BaaS:         domain.BaaS(payload.BaaS),
		GroupID:      groupID,
		UploaderName: payload.UploaderName,
	})
	if err != nil {
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
