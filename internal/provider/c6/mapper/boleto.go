// Package mapper (boleto) holds pure functions translating between our
// unified bank slip domain model and C6's DTOs. Kept side-effect free and
// isolated so they can be table-driven tested without any HTTP/DB
// dependency.
package mapper

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/upwifi/banking/internal/boleto/domain"
	"github.com/upwifi/banking/internal/provider/c6/dto"
)

const dateLayout = "2006-01-02"

// ToC6AmountType translates our neutral fixed/percentage vocabulary to
// C6's single-letter codes ("V" = valor/fixed, "P" = percentual).
func ToC6AmountType(t domain.AmountType) (string, error) {
	switch t {
	case domain.AmountTypeFixed:
		return "V", nil
	case domain.AmountTypePercentage:
		return "P", nil
	default:
		return "", fmt.Errorf("c6/mapper: unknown amount type %q", t)
	}
}

func fromC6AmountType(s string) domain.AmountType {
	switch s {
	case "P":
		return domain.AmountTypePercentage
	default:
		// C6 defaults absent/unrecognized codes to fixed value ("V").
		return domain.AmountTypeFixed
	}
}

// boletoAmountToFloat converts a domain Value (cents for Fixed, whole
// percentage units for Percentage) into the decimal number C6 expects.
// Fixed amounts are cents-to-reais like the checkout mapper; percentage
// values are already in the units C6's "value" field expects (e.g. 10 means
// 10%), so they pass through unscaled.
func boletoAmountToFloat(t domain.AmountType, value int64) float64 {
	if t == domain.AmountTypeFixed {
		return float64(value) / 100
	}
	return float64(value)
}

func boletoAmountFromFloat(t domain.AmountType, value float64) int64 {
	if t == domain.AmountTypeFixed {
		return int64(value*100 + 0.5)
	}
	return int64(value)
}

func toBoletoAmount(m domain.Money) float64 {
	return float64(m.AmountCents) / 100
}

func fromBoletoAmount(v float64) domain.Money {
	return domain.Money{AmountCents: int64(v*100 + 0.5), Currency: "BRL"}
}

// ToC6Discount builds the C6 discount object. Returns nil for a nil input or
// one with no tiers. C6 only exposes a single discount_type shared across
// all tiers, so we use the first tier's type for that field — the service
// layer is responsible for rejecting mixed-type tiers before this is called.
func ToC6Discount(d *domain.Discount) (*dto.Discount, error) {
	if d == nil || len(d.Tiers) == 0 {
		return nil, nil
	}
	discountType, err := ToC6AmountType(d.Tiers[0].Type)
	if err != nil {
		return nil, err
	}
	out := &dto.Discount{DiscountType: discountType}
	slots := []**dto.DiscountDetails{&out.First, &out.Second, &out.Third}
	for i, tier := range d.Tiers {
		if i >= len(slots) {
			return nil, fmt.Errorf("c6/mapper: at most 3 discount tiers are supported, got %d", len(d.Tiers))
		}
		*slots[i] = &dto.DiscountDetails{
			Value:    boletoAmountToFloat(tier.Type, tier.Value),
			DeadLine: tier.DeadlineDays,
		}
	}
	return out, nil
}

// FromC6Discount builds our unified discount from C6's response shape.
func FromC6Discount(d *dto.Discount) *domain.Discount {
	if d == nil {
		return nil
	}
	t := fromC6AmountType(d.DiscountType)
	var tiers []domain.DiscountTier
	for _, details := range []*dto.DiscountDetails{d.First, d.Second, d.Third} {
		if details == nil {
			continue
		}
		tiers = append(tiers, domain.DiscountTier{
			Type:         t,
			Value:        boletoAmountFromFloat(t, details.Value),
			DeadlineDays: details.DeadLine,
		})
	}
	if len(tiers) == 0 {
		return nil
	}
	return &domain.Discount{Tiers: tiers}
}

// ToC6Fine builds the C6 fine object.
func ToC6Fine(f *domain.Fine) (*dto.Fine, error) {
	if f == nil {
		return nil, nil
	}
	t, err := ToC6AmountType(f.Type)
	if err != nil {
		return nil, err
	}
	return &dto.Fine{
		Type:     t,
		Value:    boletoAmountToFloat(f.Type, f.Value),
		DeadLine: f.DeadlineDays,
	}, nil
}

// FromC6Fine builds our unified fine from C6's response shape.
func FromC6Fine(f *dto.Fine) *domain.Fine {
	if f == nil || f.Type == "" {
		return nil
	}
	t := fromC6AmountType(f.Type)
	return &domain.Fine{
		Type:         t,
		Value:        boletoAmountFromFloat(t, f.Value),
		DeadlineDays: f.DeadLine,
	}
}

// ToC6Interest builds the C6 interest object.
func ToC6Interest(i *domain.Interest) (*dto.Interest, error) {
	if i == nil {
		return nil, nil
	}
	t, err := ToC6AmountType(i.Type)
	if err != nil {
		return nil, err
	}
	return &dto.Interest{
		Type:     t,
		Value:    boletoAmountToFloat(i.Type, i.Value),
		DeadLine: i.DeadlineDays,
	}, nil
}

// FromC6Interest builds our unified interest from C6's response shape.
func FromC6Interest(i *dto.Interest) *domain.Interest {
	if i == nil || i.Type == "" {
		return nil
	}
	t := fromC6AmountType(i.Type)
	return &domain.Interest{
		Type:         t,
		Value:        boletoAmountFromFloat(t, i.Value),
		DeadlineDays: i.DeadLine,
	}
}

func toBoletoAddress(a domain.Address) dto.BoletoAddress {
	return dto.BoletoAddress{
		Street: a.Street, Number: json.Number(a.Number), Complement: a.Complement,
		City: a.City, State: a.State, ZipCode: a.ZipCode,
	}
}

func fromBoletoAddress(a dto.BoletoAddress) domain.Address {
	return domain.Address{
		Street: a.Street, Number: a.Number.String(), Complement: a.Complement,
		City: a.City, State: a.State, ZipCode: a.ZipCode,
	}
}

func toBoletoPayer(p domain.Payer) dto.BoletoPayer {
	return dto.BoletoPayer{
		Name: p.Name, TaxID: p.TaxID, Email: p.Email,
		Address: toBoletoAddress(p.Address),
	}
}

func fromBoletoPayer(p dto.BoletoPayer) domain.Payer {
	return domain.Payer{
		Name: p.Name, TaxID: p.TaxID, Email: p.Email,
		Address: fromBoletoAddress(p.Address),
	}
}

// FromC6BoletoStatus translates C6 bank slip statuses to our unified Status enum.
func FromC6BoletoStatus(s string) domain.Status {
	switch s {
	case "PAID":
		return domain.StatusPaid
	case "CANCELLED":
		return domain.StatusCancelled
	default:
		return domain.StatusCreated
	}
}

// ToC6CreateBankSlipRequest builds the C6 POST /v1/bank_slips/ body.
func ToC6CreateBankSlipRequest(req domain.CreateBankSlipRequest) (dto.CreateBankSlipRequest, error) {
	discount, err := ToC6Discount(req.Discount)
	if err != nil {
		return dto.CreateBankSlipRequest{}, err
	}
	interest, err := ToC6Interest(req.Interest)
	if err != nil {
		return dto.CreateBankSlipRequest{}, err
	}
	fine, err := ToC6Fine(req.Fine)
	if err != nil {
		return dto.CreateBankSlipRequest{}, err
	}
	return dto.CreateBankSlipRequest{
		ExternalReferenceID: req.ExternalReferenceID,
		Amount:              toBoletoAmount(req.Amount),
		DueDate:             req.DueDate.Format(dateLayout),
		Instructions:        req.Instructions,
		Discount:            discount,
		Interest:            interest,
		Fine:                fine,
		Payer:               toBoletoPayer(req.Payer),
	}, nil
}

// FromC6CreateBankSlipResponse builds our unified result from C6's create
// response. C6's create response carries no status field, so a freshly
// issued slip is always StatusCreated.
func FromC6CreateBankSlipResponse(resp dto.CreateBankSlipResponse) (domain.BankSlipResult, error) {
	dueDate, err := time.Parse(dateLayout, resp.DueDate)
	if err != nil {
		return domain.BankSlipResult{}, fmt.Errorf("c6/mapper: parse due_date %q: %w", resp.DueDate, err)
	}
	return domain.BankSlipResult{
		ProviderBankSlipID: resp.ID,
		OurNumber:          resp.OurNumber,
		BarCode:            resp.BarCode,
		DigitableLine:      resp.DigitableLine,
		Amount:             fromBoletoAmount(resp.Amount),
		DueDate:            dueDate,
		Status:             domain.StatusCreated,
	}, nil
}

// FromC6BankSlipResponse builds our unified bank slip details from C6's
// "bank_slip_response" shape, used by both GET and PUT (alter) responses.
func FromC6BankSlipResponse(resp dto.BankSlipResponse) (domain.BankSlipDetails, error) {
	dueDate, err := time.Parse(dateLayout, resp.DueDate)
	if err != nil {
		return domain.BankSlipDetails{}, fmt.Errorf("c6/mapper: parse due_date %q: %w", resp.DueDate, err)
	}
	details := domain.BankSlipDetails{
		ProviderBankSlipID:  resp.ID,
		ExternalReferenceID: resp.ExternalReferenceID,
		Status:              FromC6BoletoStatus(resp.Status),
		Amount:              fromBoletoAmount(resp.Amount),
		DueDate:             dueDate,
		Instructions:        resp.Instructions,
		Discount:            FromC6Discount(resp.Discount),
		Interest:            FromC6Interest(resp.Interest),
		Fine:                FromC6Fine(resp.Fine),
		Payer:               fromBoletoPayer(resp.Payer),
		OurNumber:           resp.OurNumber,
		BarCode:             resp.BarCode,
		DigitableLine:       resp.DigitableLine,
	}
	for _, p := range resp.Payments {
		payment := domain.PaymentInfo{Amount: fromBoletoAmount(p.Amount)}
		if p.Date != "" {
			if d, err := time.Parse(dateLayout, p.Date); err == nil {
				payment.Date = d
			}
		}
		details.Payments = append(details.Payments, payment)
	}
	return details, nil
}

// ToC6AlterBankSlipRequest builds the C6 PUT /v1/bank_slips/{id} body from
// the subset of fields the caller wants to change.
func ToC6AlterBankSlipRequest(req domain.UpdateBankSlipRequest) (dto.AlterBankSlipRequest, error) {
	var out dto.AlterBankSlipRequest
	if req.Amount != nil {
		v := toBoletoAmount(*req.Amount)
		out.Amount = &v
	}
	if req.DueDate != nil {
		v := req.DueDate.Format(dateLayout)
		out.DueDate = &v
	}
	if req.Discount != nil {
		discount, err := ToC6Discount(req.Discount)
		if err != nil {
			return dto.AlterBankSlipRequest{}, err
		}
		out.Discount = discount
	}
	if req.Interest != nil {
		interest, err := ToC6Interest(req.Interest)
		if err != nil {
			return dto.AlterBankSlipRequest{}, err
		}
		out.Interest = interest
	}
	if req.Fine != nil {
		fine, err := ToC6Fine(req.Fine)
		if err != nil {
			return dto.AlterBankSlipRequest{}, err
		}
		out.Fine = fine
	}
	return out, nil
}
