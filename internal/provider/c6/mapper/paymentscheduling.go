// Package mapper (this file) holds pure functions translating between the
// unified paymentscheduling domain model and C6's DTOs.
package mapper

import (
	"time"

	"github.com/upwifi/banking/internal/paymentscheduling/domain"
	"github.com/upwifi/banking/internal/provider/c6/dto"
)

func toC6Amount(cents int64) float64 {
	return float64(cents) / 100
}

func fromC6Amount(v float64) int64 {
	return int64(v*100 + 0.5)
}

func formatC6Date(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(c6DateLayout)
}

func parseC6Date(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(c6DateLayout, s)
	if err != nil {
		return nil
	}
	return &t
}

// ToC6PaymentInput translates one unified payment submission into C6's
// "payment" input shape.
func ToC6PaymentInput(p domain.PaymentInput) dto.PaymentInput {
	return dto.PaymentInput{
		Content:         p.Content,
		Amount:          toC6Amount(p.Amount.AmountCents),
		BankCode:        p.BankCode,
		BankName:        p.BankName,
		BeneficiaryName: p.BeneficiaryName,
		Description:     p.Description,
		PayerName:       p.PayerName,
		TransactionDate: formatC6Date(p.TransactionDate),
	}
}

// ToC6DecodeRequest builds the body for POST /decode.
func ToC6DecodeRequest(items []domain.PaymentInput) dto.DecodeRequest {
	out := make([]dto.PaymentInput, 0, len(items))
	for _, item := range items {
		out = append(out, ToC6PaymentInput(item))
	}
	return dto.DecodeRequest{Items: out}
}

// FromC6Bond translates one C6 "bonds" entry (GET /query) into our
// unified DDABond.
func FromC6Bond(b dto.Bond) domain.DDABond {
	bond := domain.DDABond{
		Amount:          domain.Money{AmountCents: fromC6Amount(b.Amount)},
		BankCode:        b.BankCode,
		BankName:        b.BankName,
		BeneficiaryName: b.BeneficiaryName,
		Content:         b.Content,
		Overdue:         b.Overdue,
		PayerName:       b.PayerName,
	}
	if d := parseC6Date(b.DueDate); d != nil {
		bond.DueDate = *d
	}
	return bond
}

// FromC6Bonds translates the GET /query response into a list of DDABond.
func FromC6Bonds(resp dto.QueryResponse) []domain.DDABond {
	out := make([]domain.DDABond, 0, len(resp.Items))
	for _, b := range resp.Items {
		out = append(out, FromC6Bond(b))
	}
	return out
}

// fromC6ProductType/fromC6Status are intentionally identity casts: C6's
// product_type and status enums are already the exact strings our unified
// enums use (BOLETO/PIX; READ_DATA/ERROR/DECODE_ERROR/PROCESSED/SCHEDULED/
// PROCESSING/SCHEDULING_CANCELLED), so no translation table is needed —
// kept as named functions so a future divergence has a single place to fix.
func fromC6ProductType(s string) domain.ProductType     { return domain.ProductType(s) }
func fromC6PaymentStatus(s string) domain.PaymentStatus { return domain.PaymentStatus(s) }

// FromC6PaymentItem translates one C6 "payment" entry (GET
// /{group_id}/items) into our unified PaymentItem.
func FromC6PaymentItem(p dto.PaymentItem) domain.PaymentItem {
	item := domain.PaymentItem{
		ID:              p.ID,
		GroupID:         p.GroupID,
		Content:         p.Content,
		Amount:          domain.Money{AmountCents: fromC6Amount(p.Amount)},
		BankCode:        p.BankCode,
		BankName:        p.BankName,
		BeneficiaryName: p.BeneficiaryName,
		Description:     p.Description,
		PayerName:       p.PayerName,
		Overdue:         p.Overdue,
		ProductType:     fromC6ProductType(p.ProductType),
		Status:          fromC6PaymentStatus(p.Status),
		ErrorMessage:    p.ErrorMessage,
	}
	item.DueDate = parseC6Date(p.DueDate)
	item.TransactionDate = parseC6Date(p.TransactionDate)
	return item
}

// FromC6GroupItems translates the GET /{group_id}/items response.
func FromC6GroupItems(resp dto.GroupItemsResponse) []domain.PaymentItem {
	out := make([]domain.PaymentItem, 0, len(resp.Items))
	for _, p := range resp.Items {
		out = append(out, FromC6PaymentItem(p))
	}
	return out
}

// ToC6DeleteItemRefs builds the array body for DELETE /{group_id}/items.
func ToC6DeleteItemRefs(itemIDs []string) []dto.DeleteItemRef {
	out := make([]dto.DeleteItemRef, 0, len(itemIDs))
	for _, id := range itemIDs {
		out = append(out, dto.DeleteItemRef{ID: id})
	}
	return out
}
