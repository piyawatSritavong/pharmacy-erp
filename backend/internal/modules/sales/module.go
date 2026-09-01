package sales

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/stocklot"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type LineInput struct {
	ProductID      string  `json:"product_id"`
	InventoryLotID string  `json:"inventory_lot_id"`
	AliasID        *string `json:"alias_id"`
	// Quantity is expressed in UnitID; stock is deducted in base units.
	Quantity          int      `json:"quantity"`
	UnitID            string   `json:"unit_id"`
	StockBucket       string   `json:"stock_bucket"`
	DiscountAmount    float64  `json:"discount_amount"`
	OverrideUnitPrice *float64 `json:"override_unit_price"`
	OverrideReason    string   `json:"override_reason"`
}

type QuoteRequest struct {
	BranchID      string      `json:"branch_id"`
	CustomerName  string      `json:"customer_name"`
	CustomerTaxID string      `json:"customer_tax_id"`
	IsGovernment  bool        `json:"is_government_mode"`
	Items              []LineInput `json:"items"`
	BillDiscountAmount float64     `json:"bill_discount_amount"`
	ExpiresAt          *time.Time  `json:"expires_at"`
}

type InvoiceRequest struct {
	BranchID          string      `json:"branch_id"`
	CustomerName      string      `json:"customer_name"`
	CustomerTaxID     string      `json:"customer_tax_id"`
	IsGovernment      bool        `json:"is_government_mode"`
	FullTaxInvoice    bool        `json:"full_tax_invoice"`
	Items              []LineInput `json:"items"`
	BillDiscountAmount float64     `json:"bill_discount_amount"`
	SourceQuotationID  *string     `json:"source_quotation_id"`
}

type PaymentRequest struct {
	PaymentType   string `json:"payment_type"`
	ReferenceCode string `json:"reference_code"`
	Notes         string `json:"notes"`
}

type CheckoutRequest struct {
	BranchID       string      `json:"branch_id"`
	CustomerName   string      `json:"customer_name"`
	CustomerTaxID  string      `json:"customer_tax_id"`
	IsGovernment   bool        `json:"is_government_mode"`
	FullTaxInvoice bool        `json:"full_tax_invoice"`
	Items              []LineInput `json:"items"`
	BillDiscountAmount float64     `json:"bill_discount_amount"`
	PaymentType        string      `json:"payment_type"`
	TenderedAmount float64     `json:"tendered_amount"`
	TransferAmount float64     `json:"transfer_amount"`
	ReferenceCode  string      `json:"reference_code"`
	Notes          string      `json:"notes"`
}

type checkoutSettlement struct {
	PaymentType    string
	CashAmount     float64
	TransferAmount float64
	TenderedAmount float64
	ChangeAmount   float64
}

type checkoutPaymentRow struct {
	PaymentType   string
	Amount        float64
	ReferenceCode string
}

func settlementPaymentRows(settlement checkoutSettlement) []checkoutPaymentRow {
	rows := make([]checkoutPaymentRow, 0, 2)
	if settlement.CashAmount > 0 {
		rows = append(rows, checkoutPaymentRow{PaymentType: "cash", Amount: settlement.CashAmount})
	}
	if settlement.TransferAmount > 0 {
		rows = append(rows, checkoutPaymentRow{
			PaymentType: "bank_transfer", Amount: settlement.TransferAmount,
		})
	}
	return rows
}

type DeleteDocumentRequest struct {
	Confirmation string `json:"confirmation"`
}

type QuotationLotAllocation struct {
	QuotationItemID string `json:"quotation_item_id"`
	InventoryLotID  string `json:"inventory_lot_id"`
	Quantity        int    `json:"quantity"`
}

type ConvertQuotationRequest struct {
	Allocations []QuotationLotAllocation `json:"allocations"`
}

type pricedLine struct {
	ProductID      string
	InventoryLotID string
	LotNumber      string
	LotReceivedAt  time.Time
	LotExpiresOn   sql.NullTime
	AliasID        *string
	ProductName    string
	DisplayName    string
	Quantity       int
	StockBucket    string
	UnitPrice      float64
	LineSubtotal   float64
	TaxRate        float64
	TaxAmount      float64
	LineTotal      float64
	CostSnapshot   float64
	PriceSource    string
	OverrideReason string

	// Selling unit and money adjustments. Quantity above is always base units.
	UnitID            string
	UnitName          string
	ConversionQty     int
	SoldQuantity      int
	SoldUnitPrice     float64
	GrossSubtotal     float64
	DiscountAmount    float64
	BillDiscountShare float64
	IsGiveaway        bool
	PromotionID       string
	PromotionName     string
	// Discount headroom left on this line after the cashier's own discount,
	// used to cap the bill-level discount.
	DiscountCeilingRemaining float64
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func canOverride(user platform.AuthUser) bool {
	return platform.HasPermission(user, "price.override.global") || platform.HasPermission(user, "price.override.pos")
}

func (s *Service) Preview(ctx context.Context, user platform.AuthUser, branchID string, isGovernment bool, items []LineInput, billDiscount float64) (map[string]any, error) {
	if err := validateBranchScope(user, branchID); err != nil {
		return nil, err
	}
	if err := validateSalesBranch(ctx, s.db, branchID); err != nil {
		return nil, err
	}
	vatRate, err := platform.GetSettingFloat(ctx, s.db, "vat_rate", 7)
	if err != nil {
		return nil, err
	}
	cart, err := s.priceCart(ctx, s.db, user, branchID, isGovernment, items, billDiscount, vatRate, false)
	if err != nil {
		return nil, err
	}
	preview := buildPreview(cart.Lines, cart.Subtotal, cart.TaxAmount, cart.TotalAmount, vatRate)
	addCartSummary(preview, cart)
	// D11: this is a portal/presentation concern (POS keeps its preview
	// simple), not a permission gate — matches any pos-portal role, not just
	// branch_pos by name.
	if user.RoleKey != "super_admin" {
		for _, line := range preview["lines"].([]map[string]any) {
			delete(line, "stock_bucket")
			delete(line, "cost_snapshot")
		}
	}
	return preview, nil
}

func (s *Service) PreviewSale(ctx context.Context, user platform.AuthUser, branchID string, isGovernment bool, items []LineInput, billDiscount float64) (map[string]any, error) {
	if err := validateBranchScope(user, branchID); err != nil {
		return nil, err
	}
	if err := validateSalesBranch(ctx, s.db, branchID); err != nil {
		return nil, err
	}
	vatRate, err := platform.GetSettingFloat(ctx, s.db, "vat_rate", 7)
	if err != nil {
		return nil, err
	}
	cart, err := s.priceCart(ctx, s.db, user, branchID, isGovernment, items, billDiscount, vatRate, true)
	if err != nil {
		return nil, err
	}
	lines, subtotal, taxAmount, totalAmount := cart.Lines, cart.Subtotal, cart.TaxAmount, cart.TotalAmount
	preview := buildPreview(lines, subtotal, taxAmount, totalAmount, vatRate)
	addCartSummary(preview, cart)
	if user.RoleKey != "super_admin" {
		for _, line := range preview["lines"].([]map[string]any) {
			delete(line, "stock_bucket")
			delete(line, "cost_snapshot")
		}
	}
	return preview, nil
}

// addCartSummary reports the discount and promotion breakdown next to the
// totals so the cashier screen can show what the customer saved.
func addCartSummary(preview map[string]any, cart cartResult) {
	summary, _ := preview["summary"].(map[string]any)
	if summary == nil {
		summary = map[string]any{}
		preview["summary"] = summary
	}
	summary["line_discount_total"] = cart.LineDiscount
	summary["bill_discount_amount"] = cart.BillDiscount
	summary["promotion_discount_total"] = cart.PromotionDiscount
	summary["discount_total"] = platform.Round2(cart.LineDiscount + cart.BillDiscount + cart.PromotionDiscount)
	summary["giveaway_cost_total"] = cart.GiveawayCost
	preview["applied_promotions"] = cart.AppliedPromotions
}

func buildPreview(lines []pricedLine, subtotal, taxAmount, totalAmount, vatRate float64) map[string]any {
	payloadLines := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		payloadLines = append(payloadLines, map[string]any{
			"product_id":       line.ProductID,
			"inventory_lot_id": line.InventoryLotID,
			"lot_number":       line.LotNumber,
			"lot_received_at":  nullableLotTime(line.LotReceivedAt),
			"lot_expires_on":   nullableSQLTime(line.LotExpiresOn),
			"alias_id":         line.AliasID,
			"actual_name":      line.ProductName,
			"display_name":     line.DisplayName,
			"quantity":         line.Quantity,
			"stock_bucket":     line.StockBucket,
			"unit_price":       line.UnitPrice,
			"line_subtotal":    line.LineSubtotal,
			"tax_rate":         line.TaxRate,
			"tax_amount":       line.TaxAmount,
			"line_total":       line.LineTotal,
			"cost_snapshot":    line.CostSnapshot,
			"price_source":     line.PriceSource,
			"override_reason":  line.OverrideReason,
			"unit_id":          line.UnitID,
			"unit_name":        line.UnitName,
			"conversion_qty":   line.ConversionQty,
			"sold_quantity":    line.SoldQuantity,
			"sold_unit_price":  line.SoldUnitPrice,
			"discount_amount":  platform.Round2(line.DiscountAmount + line.BillDiscountShare),
			"is_giveaway":      line.IsGiveaway,
			"promotion_name":   line.PromotionName,
		})
	}
	return map[string]any{
		"lines": payloadLines,
		"summary": map[string]any{
			"subtotal":     subtotal,
			"tax_rate":     vatRate,
			"tax_amount":   taxAmount,
			"total_amount": totalAmount,
		},
	}
}

func nullableLotTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullableSQLTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func (s *Service) CreateQuotation(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input QuoteRequest) (string, error) {
	if err := validateBranchScope(user, input.BranchID); err != nil {
		return "", err
	}
	if err := validateSalesBranch(ctx, s.db, input.BranchID); err != nil {
		return "", err
	}
	var quoteID string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		createdAt := time.Now().UTC()
		vatRate, err := platform.GetSettingFloat(ctx, tx, "vat_rate", 7)
		if err != nil {
			return err
		}
		cart, err := s.priceCart(ctx, tx, user, input.BranchID, input.IsGovernment, input.Items, input.BillDiscountAmount, vatRate, false)
		if err != nil {
			return err
		}
		lines, subtotal, taxAmount, totalAmount := cart.Lines, cart.Subtotal, cart.TaxAmount, cart.TotalAmount
		quoteNumber, err := nextDocumentNumber(ctx, tx, input.BranchID, "quotation", createdAt)
		if err != nil {
			return err
		}

		quoteID = platform.MustUUID()
		expiry := sql.NullTime{}
		if input.ExpiresAt != nil {
			expiry = sql.NullTime{Time: input.ExpiresAt.UTC(), Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO quotations (id, branch_id, quote_number, customer_name, customer_tax_id, status, is_government_mode, subtotal, tax_rate, tax_amount, total_amount, created_by, expires_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'draft', $6, $7, $8, $9, $10, $11, $12, $13, $13)
		`, quoteID, input.BranchID, quoteNumber, strings.TrimSpace(input.CustomerName), platform.NullString(input.CustomerTaxID), input.IsGovernment, subtotal, vatRate, taxAmount, totalAmount, user.ID, expiry, createdAt); err != nil {
			return err
		}

		for _, line := range lines {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO quotation_items (id, quotation_id, product_id, alias_id, display_name, quantity, stock_bucket, unit_price, line_subtotal, price_source, override_reason, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
			`, platform.MustUUID(), quoteID, line.ProductID, platform.NullUUID(line.AliasID), line.DisplayName, line.Quantity, line.StockBucket, line.UnitPrice, line.LineSubtotal, line.PriceSource, line.OverrideReason); err != nil {
				return err
			}
		}

		meta.EntityType = "quotation"
		meta.EntityID = &quoteID
		meta.Action = "quotation.create"
		meta.After = map[string]any{"quote_number": quoteNumber, "total_amount": totalAmount}
		return s.audit.Log(ctx, tx, meta)
	})
	return quoteID, err
}

func (s *Service) CreateInvoice(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input InvoiceRequest) (string, error) {
	if err := validateBranchScope(user, input.BranchID); err != nil {
		return "", err
	}
	if input.FullTaxInvoice && strings.TrimSpace(input.CustomerTaxID) == "" {
		return "", platform.NewError(http.StatusBadRequest, "ใบกำกับภาษีเต็มรูปต้องระบุเลขประจำตัวผู้เสียภาษี")
	}
	if err := validateSalesBranch(ctx, s.db, input.BranchID); err != nil {
		return "", err
	}
	var invoiceID string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		issuedAt := time.Now().UTC()
		vatRate, err := platform.GetSettingFloat(ctx, tx, "vat_rate", 7)
		if err != nil {
			return err
		}
		cart, err := s.priceCart(ctx, tx, user, input.BranchID, input.IsGovernment, input.Items, input.BillDiscountAmount, vatRate, true)
		if err != nil {
			return err
		}
		lines, subtotal, taxAmount, totalAmount := cart.Lines, cart.Subtotal, cart.TaxAmount, cart.TotalAmount
		invoiceID = platform.MustUUID()
		if err := s.lockAndApplyStock(ctx, tx, input.BranchID, invoiceID, user, lines, meta); err != nil {
			return err
		}

		invoiceNumber, err := nextDocumentNumber(ctx, tx, input.BranchID, "invoice", issuedAt)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invoices (id, branch_id, invoice_number, source_quote_id, customer_name, customer_tax_id, payment_status, invoice_status, is_government_mode, tax_invoice_type, request_full_tax_invoice, subtotal, tax_rate, tax_amount, total_amount, bill_discount_amount, line_discount_total, promotion_discount_total, giveaway_cost_total, created_by, issued_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'unpaid', 'issued', $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $19, $19)
		`, invoiceID, input.BranchID, invoiceNumber, platform.NullUUID(input.SourceQuotationID), strings.TrimSpace(input.CustomerName), platform.NullString(input.CustomerTaxID), input.IsGovernment, taxInvoiceType(input.FullTaxInvoice), input.FullTaxInvoice, subtotal, vatRate, taxAmount, totalAmount, cart.BillDiscount, cart.LineDiscount, cart.PromotionDiscount, cart.GiveawayCost, user.ID, issuedAt); err != nil {
			return err
		}

		for _, line := range lines {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO invoice_items (
					id,invoice_id,product_id,alias_id,actual_product_name,display_name,
					quantity,stock_bucket,unit_price,line_subtotal,tax_rate,tax_amount,
					line_total,price_source,override_reason,cost_snapshot,inventory_lot_id,
					lot_number_snapshot,lot_received_at_snapshot,lot_expires_on_snapshot,
					unit_id,unit_name_snapshot,unit_conversion_qty,sold_quantity,sold_unit_price,
					discount_amount,bill_discount_share,is_giveaway,promotion_id,promotion_name_snapshot,created_at
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
					$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,NOW())
			`, platform.MustUUID(), invoiceID, line.ProductID, platform.NullUUID(line.AliasID), line.ProductName, line.DisplayName,
				line.Quantity, line.StockBucket, line.UnitPrice, line.LineSubtotal, line.TaxRate, line.TaxAmount,
				line.LineTotal, line.PriceSource, line.OverrideReason, line.CostSnapshot, line.InventoryLotID,
				line.LotNumber, line.LotReceivedAt, line.LotExpiresOn,
				platform.NullUUID(&line.UnitID), line.UnitName, lineConversion(line), line.SoldQuantity, line.SoldUnitPrice,
				line.DiscountAmount, line.BillDiscountShare, line.IsGiveaway,
				platform.NullUUID(&line.PromotionID), line.PromotionName); err != nil {
				return err
			}
		}

		if input.SourceQuotationID != nil {
			if _, err := tx.ExecContext(ctx, `
				UPDATE quotations
				SET status = 'converted', converted_invoice_id = $2, updated_at = NOW()
				WHERE id = $1
			`, *input.SourceQuotationID, invoiceID); err != nil {
				return err
			}
		}

		meta.EntityType = "invoice"
		meta.EntityID = &invoiceID
		meta.Action = "invoice.create"
		meta.After = map[string]any{"invoice_number": invoiceNumber, "total_amount": totalAmount}
		return s.audit.Log(ctx, tx, meta)
	})
	return invoiceID, err
}

func checkoutMoneyCents(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false
	}
	rounded := math.Round(value * 100)
	if math.Abs(value*100-rounded) >= 0.000001 {
		return 0, false
	}
	return int64(rounded), true
}

func checkoutCentsToMoney(value int64) float64 {
	return float64(value) / 100
}

func validateCheckoutPayment(input CheckoutRequest, totalAmount float64) (checkoutSettlement, error) {
	settlement := checkoutSettlement{PaymentType: input.PaymentType}
	totalCents, totalValid := checkoutMoneyCents(totalAmount)
	tenderedCents, tenderedValid := checkoutMoneyCents(input.TenderedAmount)
	transferCents, transferValid := checkoutMoneyCents(input.TransferAmount)
	if !totalValid || !tenderedValid || !transferValid {
		return settlement, platform.NewError(http.StatusBadRequest, "ยอดชำระต้องเป็นจำนวนเงินไม่ติดลบและมีทศนิยมไม่เกิน 2 ตำแหน่ง")
	}
	if totalCents <= 0 {
		return settlement, platform.NewError(http.StatusBadRequest, "ยอดที่ต้องชำระต้องมากกว่าศูนย์")
	}
	switch input.PaymentType {
	case "cash":
		if tenderedCents < totalCents {
			return settlement, platform.NewError(http.StatusBadRequest, "ยอดรับเงินสดน้อยกว่ายอดชำระ")
		}
		settlement.CashAmount = checkoutCentsToMoney(totalCents)
		settlement.TenderedAmount = checkoutCentsToMoney(tenderedCents)
		settlement.ChangeAmount = checkoutCentsToMoney(tenderedCents - totalCents)
	case "bank_transfer":
		settlement.TransferAmount = checkoutCentsToMoney(totalCents)
	case "mixed":
		if transferCents <= 0 || transferCents >= totalCents {
			return settlement, platform.NewError(http.StatusBadRequest, "ยอดเงินโอนต้องมากกว่าศูนย์และน้อยกว่ายอดชำระ")
		}
		cashCents := totalCents - transferCents
		if tenderedCents < cashCents {
			return settlement, platform.NewError(http.StatusBadRequest, "ยอดรับเงินสดน้อยกว่ายอดเงินสดที่ต้องชำระ")
		}
		settlement.CashAmount = checkoutCentsToMoney(cashCents)
		settlement.TransferAmount = checkoutCentsToMoney(transferCents)
		settlement.TenderedAmount = checkoutCentsToMoney(tenderedCents)
		settlement.ChangeAmount = checkoutCentsToMoney(tenderedCents - cashCents)
	default:
		return settlement, platform.NewError(http.StatusBadRequest, "กรุณาเลือกชำระด้วยเงินสด เงินโอน หรือเงินสดผสมเงินโอน")
	}
	return settlement, nil
}

func (s *Service) PreviewCheckout(ctx context.Context, user platform.AuthUser, input CheckoutRequest) (map[string]any, error) {
	if user.Portal != "pos" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะพนักงานขายหน้าร้านเท่านั้น")
	}
	if input.FullTaxInvoice && strings.TrimSpace(input.CustomerTaxID) == "" {
		return nil, platform.NewError(http.StatusBadRequest, "ใบกำกับภาษีเต็มรูปต้องระบุเลขประจำตัวผู้เสียภาษี")
	}
	preview, err := s.PreviewSale(ctx, user, input.BranchID, input.IsGovernment, input.Items, input.BillDiscountAmount)
	if err != nil {
		return nil, err
	}
	summary := preview["summary"].(map[string]any)
	totalAmount := summary["total_amount"].(float64)
	settlement, err := validateCheckoutPayment(input, totalAmount)
	if err != nil {
		return nil, err
	}
	summary["payment_type"] = input.PaymentType
	summary["cash_amount"] = settlement.CashAmount
	summary["transfer_amount"] = settlement.TransferAmount
	summary["tendered_amount"] = settlement.TenderedAmount
	summary["change_amount"] = settlement.ChangeAmount
	return preview, nil
}

func (s *Service) Checkout(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input CheckoutRequest) (map[string]any, error) {
	if user.Portal != "pos" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะพนักงานขายหน้าร้านเท่านั้น")
	}
	if err := validateBranchScope(user, input.BranchID); err != nil {
		return nil, err
	}
	if err := validateSalesBranch(ctx, s.db, input.BranchID); err != nil {
		return nil, err
	}
	if input.FullTaxInvoice && strings.TrimSpace(input.CustomerTaxID) == "" {
		return nil, platform.NewError(http.StatusBadRequest, "ใบกำกับภาษีเต็มรูปต้องระบุเลขประจำตัวผู้เสียภาษี")
	}
	result := map[string]any{}
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		issuedAt := time.Now().UTC()
		vatRate, err := platform.GetSettingFloat(ctx, tx, "vat_rate", 7)
		if err != nil {
			return err
		}
		cart, err := s.priceCart(ctx, tx, user, input.BranchID, input.IsGovernment, input.Items, input.BillDiscountAmount, vatRate, true)
		if err != nil {
			return err
		}
		lines, subtotal, taxAmount, totalAmount := cart.Lines, cart.Subtotal, cart.TaxAmount, cart.TotalAmount
		settlement, err := validateCheckoutPayment(input, totalAmount)
		if err != nil {
			return err
		}
		invoiceID := platform.MustUUID()
		if err := s.lockAndApplyStock(ctx, tx, input.BranchID, invoiceID, user, lines, meta); err != nil {
			return err
		}
		invoiceNumber, err := nextDocumentNumber(ctx, tx, input.BranchID, "invoice", issuedAt)
		if err != nil {
			return err
		}
		customerName := strings.TrimSpace(input.CustomerName)
		if customerName == "" {
			customerName = "ลูกค้าหน้าร้าน"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invoices (
				id, branch_id, invoice_number, customer_name, customer_tax_id,
				payment_status, invoice_status, is_government_mode, tax_invoice_type, subtotal,
				tax_rate, tax_amount, total_amount, created_by, issued_at, created_at, updated_at,
				request_full_tax_invoice, bill_discount_amount, line_discount_total,
				promotion_discount_total, giveaway_cost_total
			) VALUES ($1, $2, $3, $4, $5, 'paid', 'issued', $6, $7, $8, $9, $10, $11, $12, $13, $13, $13, $14, $15, $16, $17, $18)
		`, invoiceID, input.BranchID, invoiceNumber, customerName,
			platform.NullString(input.CustomerTaxID), input.IsGovernment, taxInvoiceType(input.FullTaxInvoice), subtotal,
			vatRate, taxAmount, totalAmount, user.ID, issuedAt, input.FullTaxInvoice,
			cart.BillDiscount, cart.LineDiscount, cart.PromotionDiscount, cart.GiveawayCost); err != nil {
			return err
		}
		for _, line := range lines {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO invoice_items (
					id, invoice_id, product_id, alias_id, actual_product_name,
					display_name, quantity, stock_bucket, unit_price, line_subtotal,
					tax_rate, tax_amount, line_total, price_source, override_reason,
					cost_snapshot, inventory_lot_id, lot_number_snapshot,
					lot_received_at_snapshot, lot_expires_on_snapshot,
					unit_id, unit_name_snapshot, unit_conversion_qty, sold_quantity, sold_unit_price,
					discount_amount, bill_discount_share, is_giveaway, promotion_id, promotion_name_snapshot, created_at
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
					$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,NOW())
			`, platform.MustUUID(), invoiceID, line.ProductID, platform.NullUUID(line.AliasID),
				line.ProductName, line.DisplayName, line.Quantity, line.StockBucket,
				line.UnitPrice, line.LineSubtotal, line.TaxRate, line.TaxAmount,
				line.LineTotal, line.PriceSource, line.OverrideReason, line.CostSnapshot,
				line.InventoryLotID, line.LotNumber, line.LotReceivedAt, line.LotExpiresOn,
				platform.NullUUID(&line.UnitID), line.UnitName, lineConversion(line), line.SoldQuantity, line.SoldUnitPrice,
				line.DiscountAmount, line.BillDiscountShare, line.IsGiveaway,
				platform.NullUUID(&line.PromotionID), line.PromotionName); err != nil {
				return err
			}
		}
		for _, payment := range settlementPaymentRows(settlement) {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO invoice_payments (
					id, invoice_id, payment_type, amount, reference_code, notes, created_by, created_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
			`, platform.MustUUID(), invoiceID, payment.PaymentType, payment.Amount,
				payment.ReferenceCode, strings.TrimSpace(input.Notes), user.ID); err != nil {
				return err
			}
		}
		meta.EntityType = "invoice"
		meta.EntityID = &invoiceID
		meta.Action = "pos.checkout"
		meta.After = map[string]any{
			"invoice_number":  invoiceNumber,
			"total_amount":    totalAmount,
			"payment_type":    input.PaymentType,
			"cash_amount":     settlement.CashAmount,
			"transfer_amount": settlement.TransferAmount,
			"tendered_amount": settlement.TenderedAmount,
			"change_amount":   settlement.ChangeAmount,
		}
		if err := s.audit.Log(ctx, tx, meta); err != nil {
			return err
		}
		result = map[string]any{
			"invoice_id":      invoiceID,
			"invoice_number":  invoiceNumber,
			"total_amount":    totalAmount,
			"cash_amount":     settlement.CashAmount,
			"transfer_amount": settlement.TransferAmount,
			"tendered_amount": settlement.TenderedAmount,
			"change_amount":   settlement.ChangeAmount,
		}
		return nil
	})
	return result, err
}

func (s *Service) CollectPayment(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, invoiceID string, input PaymentRequest) error {
	if input.PaymentType != "cash" && input.PaymentType != "bank_transfer" {
		return platform.NewError(http.StatusBadRequest, "payment_type must be cash or bank_transfer")
	}
	if err := validatePaymentCollector(user); err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var branchID string
		var totalAmount float64
		var paymentStatus string
		if err := tx.QueryRowContext(ctx, `
			SELECT branch_id::text, total_amount, payment_status
			FROM invoices
			WHERE id = $1 AND deleted_at IS NULL
			FOR UPDATE
		`, invoiceID).Scan(&branchID, &totalAmount, &paymentStatus); err != nil {
			return err
		}
		if err := validateBranchScope(user, branchID); err != nil {
			return err
		}
		if paymentStatus == "paid" {
			return platform.NewError(http.StatusConflict, "invoice is already paid")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invoice_payments (id, invoice_id, payment_type, amount, reference_code, notes, created_by, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		`, platform.MustUUID(), invoiceID, input.PaymentType, totalAmount, "", strings.TrimSpace(input.Notes), user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE invoices
			SET payment_status = 'paid', updated_at = NOW()
			WHERE id = $1
		`, invoiceID); err != nil {
			return err
		}
		meta.EntityType = "invoice"
		meta.EntityID = &invoiceID
		meta.Action = "invoice.collect_payment"
		meta.After = map[string]any{"payment_type": input.PaymentType, "amount": totalAmount}
		return s.audit.Log(ctx, tx, meta)
	})
}

func validatePaymentCollector(user platform.AuthUser) error {
	if !platform.HasPermission(user, "payment.collect") {
		return platform.NewError(http.StatusForbidden, "บัญชีนี้ไม่มีสิทธิ์รับชำระเงิน")
	}
	return nil
}

func taxInvoiceType(full bool) string {
	if full {
		return "full"
	}
	return "abbreviated"
}

func (s *Service) ListQuotations(ctx context.Context, user platform.AuthUser, governmentMode *bool) ([]map[string]any, error) {
	query := `
		SELECT q.id, q.quote_number, q.customer_name, q.status, q.total_amount, q.is_government_mode, b.name, q.created_at
		FROM quotations q
		INNER JOIN branches b ON b.id = q.branch_id
	`
	args := []any{}
	conditions := []string{}
	if user.BranchID != nil && user.Scope != "global" {
		args = append(args, *user.BranchID)
		conditions = append(conditions, fmt.Sprintf("q.branch_id = $%d", len(args)))
	}
	if governmentMode != nil {
		args = append(args, *governmentMode)
		conditions = append(conditions, fmt.Sprintf("q.is_government_mode = $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY q.created_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, quoteNumber, customerName, status, branchName string
		var totalAmount float64
		var isGovernment bool
		var createdAt time.Time
		if err := rows.Scan(&id, &quoteNumber, &customerName, &status, &totalAmount, &isGovernment, &branchName, &createdAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":                 id,
			"quote_number":       quoteNumber,
			"customer_name":      customerName,
			"status":             status,
			"total_amount":       totalAmount,
			"is_government_mode": isGovernment,
			"branch_name":        branchName,
			"created_at":         createdAt,
		})
	}
	return items, rows.Err()
}

func (s *Service) ListInvoices(ctx context.Context, user platform.AuthUser, governmentMode *bool) ([]map[string]any, error) {
	query := `
		SELECT i.id, i.invoice_number, i.customer_name, i.payment_status, i.total_amount, i.is_government_mode, i.tax_invoice_type, i.request_full_tax_invoice, b.name, i.issued_at, i.deleted_at, COALESCE(i.hidden_by_id::text,''), COALESCE(i.original_invoice_number,'')
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
	`
	args := []any{}
	conditions := []string{}
	if user.RoleKey != "super_admin" {
		conditions = append(conditions, "i.deleted_at IS NULL")
	}
	if user.BranchID != nil && user.Scope != "global" {
		args = append(args, *user.BranchID)
		conditions = append(conditions, fmt.Sprintf("i.branch_id = $%d", len(args)))
	}
	if governmentMode != nil {
		args = append(args, *governmentMode)
		conditions = append(conditions, fmt.Sprintf("i.is_government_mode = $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY i.issued_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, invoiceNumber, customerName, paymentStatus, taxInvoiceType, branchName, hiddenByID, originalNumber string
		var totalAmount float64
		var isGovernment, requestFullTax bool
		var issuedAt time.Time
		var deletedAt sql.NullTime
		if err := rows.Scan(&id, &invoiceNumber, &customerName, &paymentStatus, &totalAmount, &isGovernment, &taxInvoiceType, &requestFullTax, &branchName, &issuedAt, &deletedAt, &hiddenByID, &originalNumber); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id":                       id,
			"invoice_number":           invoiceNumber,
			"customer_name":            customerName,
			"payment_status":           paymentStatus,
			"total_amount":             totalAmount,
			"is_government_mode":       isGovernment,
			"tax_invoice_type":         taxInvoiceType,
			"request_full_tax_invoice": requestFullTax,
			"branch_name":              branchName,
			"issued_at":                issuedAt,
		}
		if user.RoleKey == "super_admin" {
			item["original_invoice_number"] = originalNumber
			item["original_number"] = originalNumber
			if deletedAt.Valid {
				item["deleted_at"] = deletedAt.Time
				item["hidden_by_id"] = hiddenByID
				item["hidden_by"] = hiddenByID
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetQuotation(ctx context.Context, user platform.AuthUser, quotationID string) (map[string]any, error) {
	var id, branchID, quoteNumber, customerName, status string
	var totalAmount float64
	if err := s.db.QueryRowContext(ctx, `
		SELECT id::text,branch_id::text,quote_number,customer_name,status,total_amount
		FROM quotations WHERE id=$1
	`, quotationID).Scan(&id, &branchID, &quoteNumber, &customerName, &status, &totalAmount); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบใบเสนอราคา")
		}
		return nil, err
	}
	if err := validateBranchScope(user, branchID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT qi.id::text,qi.product_id::text,p.sku,p.name,qi.display_name,
		       qi.quantity,qi.stock_bucket,qi.unit_price
		FROM quotation_items qi
		INNER JOIN products p ON p.id=qi.product_id
		WHERE qi.quotation_id=$1 ORDER BY qi.created_at,qi.id
	`, quotationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var itemID, productID, sku, productName, displayName, bucket string
		var quantity int
		var unitPrice float64
		if err := rows.Scan(&itemID, &productID, &sku, &productName, &displayName, &quantity, &bucket, &unitPrice); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id": itemID, "product_id": productID, "sku": sku,
			"product_name": productName, "display_name": displayName,
			"quantity": quantity, "stock_bucket": bucket, "unit_price": unitPrice,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "branch_id": branchID, "quote_number": quoteNumber,
		"customer_name": customerName, "status": status,
		"total_amount": totalAmount, "items": items,
	}, nil
}

func (s *Service) GetInvoice(ctx context.Context, user platform.AuthUser, invoiceID string) (map[string]any, error) {
	var branchID string
	var id, invoiceNumber, customerName, customerTaxID, paymentStatus, taxInvoiceType, branchName, originalNumber, hiddenByID string
	var isGovernment bool
	var subtotal, taxRate, taxAmount, totalAmount float64
	var issuedAt time.Time
	var deletedAt sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
		SELECT i.id, i.branch_id::text, i.invoice_number, i.customer_name, COALESCE(i.customer_tax_id, ''), i.payment_status, i.is_government_mode, i.tax_invoice_type, i.subtotal, i.tax_rate, i.tax_amount, i.total_amount, b.name, i.issued_at,
		       COALESCE(i.original_invoice_number,''),COALESCE(i.hidden_by_id::text,''),i.deleted_at
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
		WHERE i.id = $1 AND (i.deleted_at IS NULL OR $2)
	`, invoiceID, user.RoleKey == "super_admin").Scan(
		&id, &branchID, &invoiceNumber, &customerName, &customerTaxID, &paymentStatus, &isGovernment, &taxInvoiceType, &subtotal, &taxRate, &taxAmount, &totalAmount, &branchName, &issuedAt,
		&originalNumber, &hiddenByID, &deletedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "invoice not found")
		}
		return nil, err
	}
	if err := validateBranchScope(user, branchID); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"id":                 id,
		"invoice_number":     invoiceNumber,
		"customer_name":      customerName,
		"customer_tax_id":    customerTaxID,
		"payment_status":     paymentStatus,
		"is_government_mode": isGovernment,
		"tax_invoice_type":   taxInvoiceType,
		"subtotal":           subtotal,
		"tax_rate":           taxRate,
		"tax_amount":         taxAmount,
		"total_amount":       totalAmount,
		"branch_name":        branchName,
		"issued_at":          issuedAt,
	}
	if user.RoleKey == "super_admin" {
		payload["original_invoice_number"] = originalNumber
		payload["original_number"] = originalNumber
		if deletedAt.Valid {
			payload["deleted_at"] = deletedAt.Time
			payload["hidden_by_id"] = hiddenByID
			payload["hidden_by"] = hiddenByID
		}
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text,product_id::text,COALESCE(alias_id::text,''),actual_product_name,display_name,
		       quantity,stock_bucket,unit_price,line_subtotal,tax_amount,line_total,
		       price_source,override_reason,cost_snapshot,reconciliation_discount_amount,COALESCE(inventory_lot_id::text,''),
		       COALESCE(lot_number_snapshot,''),lot_received_at_snapshot,lot_expires_on_snapshot,
		       COALESCE(unit_name_snapshot,''),unit_conversion_qty,sold_quantity,sold_unit_price,
		       discount_amount,bill_discount_share,is_giveaway,COALESCE(promotion_name_snapshot,'')
		FROM invoice_items
		WHERE invoice_id = $1
		ORDER BY created_at ASC
	`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var itemID, productID, aliasID, actualName, displayName, stockBucket, priceSource, overrideReason, lotID, lotNumber string
		var quantity int
		var unitPrice, lineSubtotal, taxAmount, lineTotal, costSnapshot, discountAmount float64
		var lotReceivedAt, lotExpiresOn sql.NullTime
		var unitName, promotionName string
		var conversionQty, soldQuantity int
		var soldUnitPrice, lineDiscount, billDiscountShare float64
		var isGiveaway bool
		if err := rows.Scan(&itemID, &productID, &aliasID, &actualName, &displayName, &quantity, &stockBucket, &unitPrice, &lineSubtotal, &taxAmount, &lineTotal, &priceSource, &overrideReason, &costSnapshot, &discountAmount, &lotID, &lotNumber, &lotReceivedAt, &lotExpiresOn,
			&unitName, &conversionQty, &soldQuantity, &soldUnitPrice, &lineDiscount, &billDiscountShare, &isGiveaway, &promotionName); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id":               itemID,
			"product_id":       productID,
			"actual_name":      actualName,
			"display_name":     displayName,
			"quantity":         quantity,
			"unit_price":       unitPrice,
			"line_subtotal":    lineSubtotal,
			"tax_amount":       taxAmount,
			"line_total":       lineTotal,
			"price_source":     priceSource,
			"override_reason":  overrideReason,
			"discount_amount":  discountAmount,
			"unit_name":        unitName,
			"conversion_qty":   conversionQty,
			"sold_quantity":    soldQuantity,
			"sold_unit_price":  soldUnitPrice,
			"line_discount":    platform.Round2(lineDiscount + billDiscountShare),
			"is_giveaway":      isGiveaway,
			"promotion_name":   promotionName,
			"inventory_lot_id": lotID,
			"lot_number":       lotNumber,
			"lot_received_at":  nullableSQLTime(lotReceivedAt),
			"lot_expires_on":   nullableSQLTime(lotExpiresOn),
		}
		// D11: internal detail hidden only from the POS portal (matches the
		// preview-trimming above), not literally "only super_admin" — any
		// back-office role (admin, office, branch_head, ...) sees it.
		if user.RoleKey == "super_admin" {
			item["stock_bucket"] = stockBucket
			item["cost_snapshot"] = costSnapshot
		}
		if aliasID != "" {
			item["alias_id"] = aliasID
		}
		items = append(items, item)
	}
	payload["items"] = items
	return payload, rows.Err()
}

func (s *Service) GetInvoicePrint(ctx context.Context, user platform.AuthUser, invoiceID string) (map[string]any, error) {
	var (
		id, branchID, branchCode, branchName, branchAddress      string
		invoiceNumber, customerName, customerTaxID               string
		paymentStatus, invoiceStatus, taxInvoiceType, sellerName string
		isGovernment                                             bool
		subtotal, taxRate, taxAmount, totalAmount                float64
		issuedAt                                                 time.Time
	)
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			i.id,
			i.branch_id::text,
			b.code,
			b.name,
			b.address,
			i.invoice_number,
			i.customer_name,
			COALESCE(i.customer_tax_id, ''),
			i.payment_status,
			i.invoice_status,
			i.is_government_mode,
			i.tax_invoice_type,
			i.subtotal,
			i.tax_rate,
			i.tax_amount,
			i.total_amount,
			u.full_name,
			i.issued_at
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
		INNER JOIN users u ON u.id = i.created_by
		WHERE i.id = $1 AND (i.deleted_at IS NULL OR $2)
	`, invoiceID, user.RoleKey == "super_admin").Scan(
		&id,
		&branchID,
		&branchCode,
		&branchName,
		&branchAddress,
		&invoiceNumber,
		&customerName,
		&customerTaxID,
		&paymentStatus,
		&invoiceStatus,
		&isGovernment,
		&taxInvoiceType,
		&subtotal,
		&taxRate,
		&taxAmount,
		&totalAmount,
		&sellerName,
		&issuedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "invoice not found")
		}
		return nil, err
	}
	if err := validateBranchScope(user, branchID); err != nil {
		return nil, err
	}

	companyName, err := platform.GetSettingString(ctx, s.db, "company_name", "Pharmacy ERP Demo")
	if err != nil {
		return nil, err
	}
	companyTaxID, err := platform.GetSettingString(ctx, s.db, "company_tax_id", "")
	if err != nil {
		return nil, err
	}
	companyAddress, err := platform.GetSettingString(ctx, s.db, "company_address", "")
	if err != nil {
		return nil, err
	}

	itemRows, err := s.db.QueryContext(ctx, `
		SELECT
			product_id::text,
			COALESCE(alias_id::text, ''),
			actual_product_name,
			display_name,
			quantity,
			stock_bucket,
			unit_price,
			line_subtotal,
			tax_rate,
			tax_amount,
			line_total,
			price_source,
			override_reason,
			cost_snapshot,
			reconciliation_discount_amount,
			COALESCE(inventory_lot_id::text,''),
			COALESCE(lot_number_snapshot,''),
			lot_received_at_snapshot,
			lot_expires_on_snapshot
		FROM invoice_items
		WHERE invoice_id = $1
		ORDER BY created_at ASC
	`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()

	items := []map[string]any{}
	for itemRows.Next() {
		var (
			productID, aliasID, actualName, displayName string
			stockBucket, priceSource, overrideReason    string
			lotID, lotNumber                            string
			quantity                                    int
			unitPrice, lineSubtotal, lineTaxRate        float64
			lineTaxAmount, lineTotal, costSnapshot      float64
			discountAmount                              float64
			lotReceivedAt, lotExpiresOn                 sql.NullTime
		)
		if err := itemRows.Scan(
			&productID,
			&aliasID,
			&actualName,
			&displayName,
			&quantity,
			&stockBucket,
			&unitPrice,
			&lineSubtotal,
			&lineTaxRate,
			&lineTaxAmount,
			&lineTotal,
			&priceSource,
			&overrideReason,
			&costSnapshot,
			&discountAmount,
			&lotID,
			&lotNumber,
			&lotReceivedAt,
			&lotExpiresOn,
		); err != nil {
			return nil, err
		}
		item := map[string]any{
			"product_id":       productID,
			"actual_name":      actualName,
			"display_name":     displayName,
			"quantity":         quantity,
			"unit_price":       unitPrice,
			"line_subtotal":    lineSubtotal,
			"tax_rate":         lineTaxRate,
			"tax_amount":       lineTaxAmount,
			"line_total":       lineTotal,
			"price_source":     priceSource,
			"override_reason":  overrideReason,
			"discount_amount":  discountAmount,
			"is_alias_display": aliasID != "",
			"inventory_lot_id": lotID,
			"lot_number":       lotNumber,
			"lot_received_at":  nullableSQLTime(lotReceivedAt),
			"lot_expires_on":   nullableSQLTime(lotExpiresOn),
		}
		// D11: internal detail hidden only from the POS portal (matches the
		// preview-trimming above), not literally "only super_admin" — any
		// back-office role (admin, office, branch_head, ...) sees it.
		if user.RoleKey == "super_admin" {
			item["stock_bucket"] = stockBucket
			item["cost_snapshot"] = costSnapshot
		}
		if aliasID != "" {
			item["alias_id"] = aliasID
		}
		items = append(items, item)
	}
	if err := itemRows.Err(); err != nil {
		return nil, err
	}

	paymentRows, err := s.db.QueryContext(ctx, `
		SELECT ip.payment_type, ip.amount, ip.reference_code, ip.notes, COALESCE(u.full_name, ''), ip.created_at
		FROM invoice_payments ip
		LEFT JOIN users u ON u.id = ip.created_by
		WHERE ip.invoice_id = $1
		ORDER BY ip.created_at ASC
	`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer paymentRows.Close()

	payments := []map[string]any{}
	for paymentRows.Next() {
		var paymentType, referenceCode, notes, collectedBy string
		var amount float64
		var createdAt time.Time
		if err := paymentRows.Scan(&paymentType, &amount, &referenceCode, &notes, &collectedBy, &createdAt); err != nil {
			return nil, err
		}
		payments = append(payments, map[string]any{
			"payment_type":   paymentType,
			"amount":         amount,
			"reference_code": referenceCode,
			"notes":          notes,
			"collected_by":   collectedBy,
			"created_at":     createdAt,
		})
	}
	if err := paymentRows.Err(); err != nil {
		return nil, err
	}

	return map[string]any{
		"document": map[string]any{
			"id":                 id,
			"invoice_number":     invoiceNumber,
			"issued_at":          issuedAt,
			"payment_status":     paymentStatus,
			"invoice_status":     invoiceStatus,
			"customer_name":      customerName,
			"customer_tax_id":    customerTaxID,
			"is_government_mode": isGovernment,
			"tax_invoice_type":   taxInvoiceType,
			"seller_name":        sellerName,
		},
		"company": map[string]any{
			"name":    companyName,
			"tax_id":  companyTaxID,
			"address": companyAddress,
		},
		"branch": map[string]any{
			"id":      branchID,
			"code":    branchCode,
			"name":    branchName,
			"address": branchAddress,
		},
		"summary": map[string]any{
			"subtotal":     subtotal,
			"tax_rate":     taxRate,
			"tax_amount":   taxAmount,
			"total_amount": totalAmount,
		},
		"items":    items,
		"payments": payments,
	}, nil
}

func (s *Service) ConvertQuotation(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, quotationID string, input ConvertQuotationRequest) (string, error) {
	var branchID, customerName, customerTaxID string
	var isGovernment bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT branch_id::text, customer_name, COALESCE(customer_tax_id, ''), is_government_mode
		FROM quotations
		WHERE id = $1 AND status = 'draft'
	`, quotationID).Scan(&branchID, &customerName, &customerTaxID, &isGovernment); err != nil {
		if err == sql.ErrNoRows {
			return "", platform.NewError(http.StatusNotFound, "draft quotation not found")
		}
		return "", err
	}
	if err := validateBranchScope(user, branchID); err != nil {
		return "", err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text,product_id::text,COALESCE(alias_id::text,''),quantity,stock_bucket,unit_price,override_reason
		FROM quotation_items
		WHERE quotation_id = $1
		ORDER BY created_at ASC
	`, quotationID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	type quoteLine struct {
		ID, ProductID, AliasID, StockBucket, OverrideReason string
		Quantity                                            int
		UnitPrice                                           float64
	}
	quoteLines := map[string]quoteLine{}
	quoteOrder := []string{}
	for rows.Next() {
		var line quoteLine
		if err := rows.Scan(&line.ID, &line.ProductID, &line.AliasID, &line.Quantity, &line.StockBucket, &line.UnitPrice, &line.OverrideReason); err != nil {
			return "", err
		}
		quoteLines[line.ID] = line
		quoteOrder = append(quoteOrder, line.ID)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(input.Allocations) == 0 {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเลือก lot สำหรับสินค้าทุกรายการก่อนออกใบขาย")
	}
	allocated := map[string]int{}
	items := []LineInput{}
	for _, allocation := range input.Allocations {
		line, ok := quoteLines[strings.TrimSpace(allocation.QuotationItemID)]
		if !ok || allocation.Quantity <= 0 || strings.TrimSpace(allocation.InventoryLotID) == "" {
			return "", platform.NewError(http.StatusBadRequest, "ข้อมูลการจัดสรร lot ไม่ถูกต้อง")
		}
		allocated[line.ID] += allocation.Quantity
		override := line.UnitPrice
		item := LineInput{
			ProductID:         line.ProductID,
			InventoryLotID:    strings.TrimSpace(allocation.InventoryLotID),
			Quantity:          allocation.Quantity,
			StockBucket:       line.StockBucket,
			OverrideUnitPrice: &override,
			OverrideReason:    line.OverrideReason,
		}
		if line.AliasID != "" {
			aliasID := line.AliasID
			item.AliasID = &aliasID
		}
		items = append(items, item)
	}
	for _, lineID := range quoteOrder {
		if allocated[lineID] != quoteLines[lineID].Quantity {
			return "", platform.NewError(http.StatusBadRequest, "จำนวนที่จัดสรรจาก lot ต้องเท่ากับจำนวนในใบเสนอราคา")
		}
	}

	return s.CreateInvoice(ctx, user, meta, InvoiceRequest{
		BranchID:          branchID,
		CustomerName:      customerName,
		CustomerTaxID:     customerTaxID,
		IsGovernment:      isGovernment,
		FullTaxInvoice:    strings.TrimSpace(customerTaxID) != "",
		Items:             items,
		SourceQuotationID: &quotationID,
	})
}

func requireDocumentDelete(user platform.AuthUser) error {
	if !platform.HasPermission(user, "quotation.manage") {
		return platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบเท่านั้นที่ลบเอกสารได้")
	}
	return nil
}

func deletionWarning(monthEndCount int) string {
	if monthEndCount > 0 {
		return "ใบขายนี้ถูกอ้างอิงในกระดาษทำการปิดเดือน จึงไม่สามารถลบได้"
	}
	return "ระบบจะคืนสต๊อกและซ่อนใบขายนี้ โดยเก็บข้อมูลเดิมไว้ตรวจสอบย้อนหลัง"
}

func (s *Service) InvoiceDeletionImpact(ctx context.Context, user platform.AuthUser, invoiceID string) (map[string]any, error) {
	if err := requireDocumentDelete(user); err != nil {
		return nil, err
	}
	var number string
	var itemCount, paymentCount, monthEndCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT i.invoice_number,
		       (SELECT COUNT(*) FROM invoice_items ii WHERE ii.invoice_id = i.id),
		       (SELECT COUNT(*) FROM invoice_payments ip WHERE ip.invoice_id = i.id),
		       (SELECT COUNT(DISTINCT mel.workpaper_id) FROM month_end_workpaper_lines mel WHERE mel.invoice_id = i.id)
		       + (SELECT COUNT(DISTINCT rl.reconciliation_id) FROM reconciliation_logs rl WHERE rl.invoice_id = i.id)
		FROM invoices i
		WHERE i.id = $1 AND i.deleted_at IS NULL
	`, invoiceID).Scan(&number, &itemCount, &paymentCount, &monthEndCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบใบขาย")
		}
		return nil, err
	}
	return map[string]any{
		"document_number": number,
		"confirmation":    "ลบ " + number,
		"related": map[string]any{
			"items": itemCount, "payments": paymentCount,
			"month_end_workpapers": monthEndCount,
		},
		"warning": deletionWarning(monthEndCount),
	}, nil
}

func (s *Service) QuotationDeletionImpact(ctx context.Context, user platform.AuthUser, quotationID string) (map[string]any, error) {
	if err := requireDocumentDelete(user); err != nil {
		return nil, err
	}
	var number, status string
	var itemCount, invoiceCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT q.quote_number, q.status,
		       (SELECT COUNT(*) FROM quotation_items qi WHERE qi.quotation_id = q.id),
		       (SELECT COUNT(*) FROM invoices i WHERE i.source_quote_id = q.id AND i.deleted_at IS NULL)
		FROM quotations q
		WHERE q.id = $1
	`, quotationID).Scan(&number, &status, &itemCount, &invoiceCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบใบเสนอราคา")
		}
		return nil, err
	}
	return map[string]any{
		"document_number": number,
		"status":          status,
		"confirmation":    "ลบ " + number,
		"related": map[string]any{
			"items": itemCount, "invoices": invoiceCount,
		},
		"warning": "หากออกใบขายจากเอกสารนี้ ระบบจะลบใบขาย คืนสต๊อก และลบข้อมูลที่เกี่ยวข้องทั้งหมด",
	}, nil
}

type invoiceStockLine struct {
	ProductID string
	Bucket    string
	Quantity  int
}

func (s *Service) deleteInvoiceTx(ctx context.Context, tx *sql.Tx, user platform.AuthUser, meta audit.LogEntry, invoiceID, confirmation string, restoreQuotation bool) (string, error) {
	var branchID, number string
	var sourceQuotationID, originalNumber sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT branch_id::text, invoice_number, source_quote_id::text, original_invoice_number
		FROM invoices
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, invoiceID).Scan(&branchID, &number, &sourceQuotationID, &originalNumber); err != nil {
		if err == sql.ErrNoRows {
			return "", platform.NewError(http.StatusNotFound, "ไม่พบใบขาย")
		}
		return "", err
	}
	if confirmation != "" && confirmation != "ลบ "+number {
		return "", platform.NewError(http.StatusBadRequest, "ข้อความยืนยันการลบไม่ถูกต้อง")
	}
	if originalNumber.Valid {
		return "", platform.NewError(http.StatusConflict, "ใบขายนี้เป็นส่วนหนึ่งของการสรุปสิ้นเดือนและไม่สามารถซ่อนได้")
	}
	var monthEndCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM month_end_workpaper_lines WHERE invoice_id = $1`, invoiceID).Scan(&monthEndCount); err != nil {
		return "", err
	}
	if monthEndCount > 0 {
		return "", platform.NewError(http.StatusConflict, "ใบขายนี้ถูกอ้างอิงในกระดาษทำการปิดเดือนและไม่สามารถลบได้")
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT product_id::text, stock_bucket, SUM(quantity)::int
		FROM invoice_items
		WHERE invoice_id = $1
		GROUP BY product_id, stock_bucket
		ORDER BY product_id, stock_bucket
	`, invoiceID)
	if err != nil {
		return "", err
	}
	stockLines := []invoiceStockLine{}
	for rows.Next() {
		var line invoiceStockLine
		if err := rows.Scan(&line.ProductID, &line.Bucket, &line.Quantity); err != nil {
			rows.Close()
			return "", err
		}
		stockLines = append(stockLines, line)
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	for _, line := range stockLines {
		if err := platform.EnforceGhostWritePolicy(user, line.Bucket == "ghost"); err != nil {
			return "", err
		}
	}

	for _, line := range stockLines {
		allocations := []stocklot.Allocation{}
		movementIDs := []string{}
		movementRows, err := tx.QueryContext(ctx, `
			SELECT id::text FROM inventory_movements
			WHERE reference_type='invoice' AND reference_id=$1 AND product_id=$2
			  AND stock_bucket=$3 AND movement_type='sale'
			ORDER BY created_at,id
		`, invoiceID, line.ProductID, line.Bucket)
		if err != nil {
			return "", err
		}
		for movementRows.Next() {
			var movementID string
			if err := movementRows.Scan(&movementID); err != nil {
				movementRows.Close()
				return "", err
			}
			movementIDs = append(movementIDs, movementID)
		}
		if err := movementRows.Close(); err != nil {
			return "", err
		}
		// A transaction owns one database connection. Close the movement cursor
		// before loading allocations so the nested query can use that connection.
		for _, movementID := range movementIDs {
			movementAllocations, err := stocklot.LoadMovementAllocations(ctx, tx, movementID)
			if err != nil {
				return "", err
			}
			allocations = append(allocations, movementAllocations...)
		}
		if len(allocations) > 0 {
			if err := stocklot.RestoreAllocations(ctx, tx, allocations); err != nil {
				return "", err
			}
		} else {
			var cost float64
			if err := tx.QueryRowContext(ctx, `SELECT cost_price FROM products WHERE id=$1`, line.ProductID).Scan(&cost); err != nil {
				return "", err
			}
			lotID, err := stocklot.Create(ctx, tx, stocklot.Lot{BranchID: branchID, ProductID: line.ProductID, StockBucket: line.Bucket, ReceivedQuantity: line.Quantity, RemainingQuantity: line.Quantity, UnitCost: cost}, "invoice_delete_restore", &invoiceID, nil, nil)
			if err != nil {
				return "", err
			}
			allocations = []stocklot.Allocation{{Lot: stocklot.Lot{ID: lotID}, Quantity: line.Quantity}}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory (id, branch_id, product_id, qty_real, qty_ghost, created_at, updated_at)
			VALUES ($1, $2, $3, 0, 0, NOW(), NOW())
			ON CONFLICT (branch_id, product_id) DO NOTHING
		`, platform.MustUUID(), branchID, line.ProductID); err != nil {
			return "", err
		}
		if line.Bucket == "real" {
			_, err = tx.ExecContext(ctx, `UPDATE inventory SET qty_real = qty_real + $3, updated_at = NOW() WHERE branch_id = $1 AND product_id = $2`, branchID, line.ProductID, line.Quantity)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE inventory SET qty_ghost = qty_ghost + $3, updated_at = NOW() WHERE branch_id = $1 AND product_id = $2`, branchID, line.ProductID, line.Quantity)
		}
		if err != nil {
			return "", err
		}
		restoreMovementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (
				id, branch_id, product_id, movement_type, stock_bucket, quantity_delta,
				reference_type, reference_id, note, performed_by, created_at
			) VALUES ($1, $2, $3, 'invoice_delete_restore', $4, $5, 'deleted_invoice', $6, $7, $8, NOW())
		`, restoreMovementID, branchID, line.ProductID, line.Bucket, line.Quantity, invoiceID, "คืนสต๊อกจากการลบ "+number, user.ID); err != nil {
			return "", err
		}
		if err := stocklot.AttachMovement(ctx, tx, restoreMovementID, allocations, 1); err != nil {
			return "", err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE invoices
		SET original_invoice_number=COALESCE(original_invoice_number,invoice_number),
		    deleted_at=NOW(),hidden_by_id=$2,updated_at=NOW(),
		    source_quote_id=CASE WHEN $3 THEN source_quote_id ELSE NULL END
		WHERE id=$1
	`, invoiceID, user.ID, restoreQuotation); err != nil {
		return "", err
	}
	if restoreQuotation && sourceQuotationID.Valid {
		if _, err := tx.ExecContext(ctx, `
			UPDATE quotations
			SET status = 'draft', converted_invoice_id = NULL, updated_at = NOW()
			WHERE id = $1
		`, sourceQuotationID.String); err != nil {
			return "", err
		}
	}
	meta.EntityType = "invoice"
	meta.EntityID = &invoiceID
	meta.Action = "invoice.delete"
	meta.Before = map[string]any{"invoice_number": number}
	meta.After = map[string]any{"soft_deleted": true, "stock_restored": true, "hidden_by_id": user.ID}
	if err := s.audit.Log(ctx, tx, meta); err != nil {
		return "", err
	}
	return number, nil
}

func (s *Service) DeleteInvoice(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, invoiceID, confirmation string) error {
	if err := requireDocumentDelete(user); err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		_, err := s.deleteInvoiceTx(ctx, tx, user, meta, invoiceID, strings.TrimSpace(confirmation), true)
		return err
	})
}

func (s *Service) DeleteQuotation(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, quotationID, confirmation string) error {
	if err := requireDocumentDelete(user); err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var number string
		if err := tx.QueryRowContext(ctx, `SELECT quote_number FROM quotations WHERE id = $1 FOR UPDATE`, quotationID).Scan(&number); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบใบเสนอราคา")
			}
			return err
		}
		if strings.TrimSpace(confirmation) != "ลบ "+number {
			return platform.NewError(http.StatusBadRequest, "ข้อความยืนยันการลบไม่ถูกต้อง")
		}
		rows, err := tx.QueryContext(ctx, `SELECT id::text FROM invoices WHERE source_quote_id = $1 AND deleted_at IS NULL ORDER BY created_at`, quotationID)
		if err != nil {
			return err
		}
		invoiceIDs := []string{}
		for rows.Next() {
			var invoiceID string
			if err := rows.Scan(&invoiceID); err != nil {
				rows.Close()
				return err
			}
			invoiceIDs = append(invoiceIDs, invoiceID)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, invoiceID := range invoiceIDs {
			if _, err := s.deleteInvoiceTx(ctx, tx, user, meta, invoiceID, "", false); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM quotations WHERE id = $1`, quotationID); err != nil {
			return err
		}
		meta.EntityType = "quotation"
		meta.EntityID = &quotationID
		meta.Action = "quotation.delete"
		meta.Before = map[string]any{"quote_number": number}
		meta.After = map[string]any{"deleted": true, "related_invoices": len(invoiceIDs)}
		return s.audit.Log(ctx, tx, meta)
	})
}

func validateBranchScope(user platform.AuthUser, branchID string) error {
	if user.BranchID != nil && user.Scope != "global" && *user.BranchID != branchID {
		return platform.NewError(http.StatusForbidden, "branch scope mismatch")
	}
	return nil
}

func validateSalesBranch(ctx context.Context, db platform.DBTX, branchID string) error {
	if strings.TrimSpace(branchID) == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณาเลือกสาขาขาย")
	}
	var salesEnabled bool
	if err := db.QueryRowContext(ctx, `SELECT sales_enabled FROM branches WHERE id=$1 AND active=TRUE`, branchID).Scan(&salesEnabled); err != nil {
		if err == sql.ErrNoRows {
			return platform.NewError(http.StatusNotFound, "ไม่พบสาขาขายที่เปิดใช้งาน")
		}
		return err
	}
	if !salesEnabled {
		return platform.NewError(http.StatusForbidden, "โกดังใช้สำหรับรับและกระจายสินค้า ไม่สามารถสร้างรายการขายได้")
	}
	return nil
}

func nextDocumentNumber(ctx context.Context, tx *sql.Tx, branchID string, docType string, issuedAt time.Time) (string, error) {
	var prefix, branchCode string
	var nextNumber int64
	if err := tx.QueryRowContext(ctx, `
		SELECT ds.prefix, ds.next_number, b.code
		FROM document_sequences ds
		INNER JOIN branches b ON b.id=ds.branch_id
		WHERE ds.branch_id = $1 AND ds.doc_type = $2
		FOR UPDATE OF ds
	`, branchID, docType).Scan(&prefix, &nextNumber, &branchCode); err != nil {
		return "", err
	}
	number := platform.FormatBranchDocumentNumber(branchCode, prefix, issuedAt, nextNumber)
	if _, err := tx.ExecContext(ctx, `
		UPDATE document_sequences
		SET next_number = next_number + 1, updated_at = NOW()
		WHERE branch_id = $1 AND doc_type = $2
	`, branchID, docType); err != nil {
		return "", err
	}
	return number, nil
}

func (s *Service) priceLines(ctx context.Context, db platform.DBTX, user platform.AuthUser, branchID string, isGovernment bool, items []LineInput, vatRate float64, requireLot bool) ([]pricedLine, float64, float64, float64, error) {
	if len(items) == 0 {
		return nil, 0, 0, 0, platform.NewError(http.StatusBadRequest, "at least one line is required")
	}
	if err := validatePOSCatalogRules(user, isGovernment, items); err != nil {
		return nil, 0, 0, 0, err
	}

	lines := make([]pricedLine, 0, len(items))
	var subtotal float64
	var taxAmount float64

	for _, item := range items {
		if item.Quantity <= 0 {
			return nil, 0, 0, 0, platform.NewError(http.StatusBadRequest, "quantity must be greater than zero")
		}
		if err := platform.EnforceGhostWritePolicy(user, item.StockBucket == "ghost"); err != nil {
			return nil, 0, 0, 0, err
		}
		if item.StockBucket != "real" {
			return nil, 0, 0, 0, platform.NewError(http.StatusBadRequest, "ใบขายและใบเสนอราคาใช้ได้เฉพาะสต๊อกจริง")
		}
		var (
			productID         string
			productName       string
			costPrice         float64
			basePrice         float64
			taxExempt         bool
			aliasID           sql.NullString
			aliasName         sql.NullString
			aliasDefaultPrice sql.NullFloat64
			maxDiscountAmount float64
		)
		if err := db.QueryRowContext(ctx, `
			SELECT p.id::text, p.name, p.cost_price, COALESCE(bpp.selling_price, p.base_selling_price), p.tax_exempt,
			       COALESCE(bps.max_discount_amount,p.max_discount_amount),
			       a.id::text, a.alias_name, a.default_government_price
			FROM products p
			LEFT JOIN branch_product_prices bpp ON bpp.product_id = p.id AND bpp.branch_id = $2
			LEFT JOIN branch_product_settings bps ON bps.product_id = p.id AND bps.branch_id = $2
			LEFT JOIN product_aliases a ON a.id = $3 AND a.product_id = p.id AND a.active = TRUE AND (a.branch_id IS NULL OR a.branch_id = $2)
			WHERE p.id = $1 AND p.active = TRUE
		`, item.ProductID, branchID, platform.NullUUID(item.AliasID)).Scan(
			&productID, &productName, &costPrice, &basePrice, &taxExempt, &maxDiscountAmount, &aliasID, &aliasName, &aliasDefaultPrice,
		); err != nil {
			if err == sql.ErrNoRows {
				return nil, 0, 0, 0, platform.NewError(http.StatusNotFound, "product not found")
			}
			return nil, 0, 0, 0, err
		}
		if err := validateAliasSelection(item.AliasID, aliasID); err != nil {
			return nil, 0, 0, 0, err
		}

		unit, err := resolveUnit(ctx, db, productID, item.UnitID, basePrice, productName)
		if err != nil {
			return nil, 0, 0, 0, err
		}
		soldQuantity := item.Quantity
		baseQuantity := soldQuantity * unit.Conversion

		var lotID, lotNumber string
		var lotReceivedAt time.Time
		var lotExpiresOn sql.NullTime
		if requireLot {
			lotID = strings.TrimSpace(item.InventoryLotID)
			if lotID == "" {
				return nil, 0, 0, 0, platform.NewError(http.StatusBadRequest, fmt.Sprintf("กรุณาเลือก lot ของ %s", productName))
			}
			var remaining int
			if err := db.QueryRowContext(ctx, `
				SELECT il.id::text,il.lot_number,il.received_at,il.expires_on,il.remaining_quantity,il.unit_cost
				FROM inventory_lots il
				WHERE il.id=$1 AND il.branch_id=$2 AND il.product_id=$3 AND il.stock_bucket=$4
				  AND il.remaining_quantity>0
				  AND (il.expires_on IS NULL OR il.expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)
			`, lotID, branchID, productID, item.StockBucket).Scan(&lotID, &lotNumber, &lotReceivedAt, &lotExpiresOn, &remaining, &costPrice); err != nil {
				if err == sql.ErrNoRows {
					return nil, 0, 0, 0, platform.NewError(http.StatusConflict, fmt.Sprintf("lot ที่เลือกของ %s ไม่พร้อมขายหรือหมดอายุแล้ว", productName))
				}
				return nil, 0, 0, 0, err
			}
			if remaining < baseQuantity {
				return nil, 0, 0, 0, platform.NewError(http.StatusConflict, fmt.Sprintf("จำนวนใน lot ที่เลือกของ %s ไม่เพียงพอ", productName))
			}
		}

		displayName, soldUnitPrice, priceSource, err := resolveLineDisplayAndPrice(productName, unit.Price, aliasID, aliasName, aliasDefaultPrice, isGovernment, item.OverrideUnitPrice, canOverride(user))
		if err != nil {
			return nil, 0, 0, 0, err
		}
		// The discount ceiling is stored per base unit, so scale it to the unit
		// actually being sold before comparing.
		unitFloor := platform.Round2(unit.Price - maxDiscountAmount*float64(unit.Conversion))
		if unitFloor < 0 {
			unitFloor = 0
		}
		hasGlobalOverride := platform.HasPermission(user, "price.override.global") && strings.TrimSpace(item.OverrideReason) != ""
		if item.OverrideUnitPrice != nil {
			if soldUnitPrice < unitFloor && !hasGlobalOverride {
				return nil, 0, 0, 0, platform.NewError(http.StatusForbidden, fmt.Sprintf("ราคาของ %s ต่ำกว่าราคาต่ำสุด %.2f บาท", productName, unitFloor))
			}
		}

		grossSubtotal := platform.Round2(soldUnitPrice * float64(soldQuantity))
		lineDiscount := platform.Round2(item.DiscountAmount)
		if lineDiscount < 0 {
			return nil, 0, 0, 0, platform.NewError(http.StatusBadRequest, "ส่วนลดต้องไม่ติดลบ")
		}
		if lineDiscount > 0 {
			if !platform.HasPermission(user, "sales.discount.line") {
				return nil, 0, 0, 0, platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์ให้ส่วนลด")
			}
			if lineDiscount > grossSubtotal {
				return nil, 0, 0, 0, platform.NewError(http.StatusBadRequest, fmt.Sprintf("ส่วนลดของ %s มากกว่ายอดสินค้า", productName))
			}
			maxLineDiscount := platform.Round2(maxDiscountAmount * float64(baseQuantity))
			if lineDiscount > maxLineDiscount && !hasGlobalOverride {
				return nil, 0, 0, 0, platform.NewError(http.StatusForbidden,
					fmt.Sprintf("ส่วนลดของ %s เกินเพดาน %.2f บาท", productName, maxLineDiscount))
			}
		}

		netSubtotal := platform.Round2(grossSubtotal - lineDiscount)
		unitPrice := soldUnitPrice
		if baseQuantity > 0 {
			unitPrice = platform.Round2(grossSubtotal / float64(baseQuantity))
		}
		lineTaxRate := vatRate
		if taxExempt {
			lineTaxRate = 0
		}
		lineTaxAmount := platform.Round2(netSubtotal * lineTaxRate / 100)
		lineTotal := platform.Round2(netSubtotal + lineTaxAmount)

		subtotal = platform.Round2(subtotal + netSubtotal)
		taxAmount = platform.Round2(taxAmount + lineTaxAmount)

		lines = append(lines, pricedLine{
			ProductID:      productID,
			InventoryLotID: lotID,
			LotNumber:      lotNumber,
			LotReceivedAt:  lotReceivedAt,
			LotExpiresOn:   lotExpiresOn,
			AliasID:        platform.StringPointer(aliasID),
			ProductName:    productName,
			DisplayName:    displayName,
			Quantity:       baseQuantity,
			StockBucket:    item.StockBucket,
			UnitPrice:      unitPrice,
			LineSubtotal:   netSubtotal,
			TaxRate:        lineTaxRate,
			TaxAmount:      lineTaxAmount,
			LineTotal:      lineTotal,
			CostSnapshot:   costPrice,
			PriceSource:    priceSource,
			OverrideReason: strings.TrimSpace(item.OverrideReason),

			UnitID:         unit.ID,
			UnitName:       unit.Name,
			ConversionQty:  unit.Conversion,
			SoldQuantity:   soldQuantity,
			SoldUnitPrice:  platform.Round2(soldUnitPrice),
			GrossSubtotal:  grossSubtotal,
			DiscountAmount: lineDiscount,
			DiscountCeilingRemaining: math.Max(0, platform.Round2(maxDiscountAmount*float64(baseQuantity)-lineDiscount)),
		})
	}

	totalAmount := platform.Round2(subtotal + taxAmount)
	return lines, subtotal, taxAmount, totalAmount, nil
}

func validateAliasSelection(requestedAliasID *string, resolvedAliasID sql.NullString) error {
	if requestedAliasID != nil && strings.TrimSpace(*requestedAliasID) != "" && !resolvedAliasID.Valid {
		return platform.NewError(http.StatusBadRequest, "alias does not match the selected product or branch")
	}
	return nil
}

func validatePOSCatalogRules(user platform.AuthUser, isGovernment bool, items []LineInput) error {
	if user.Portal != "pos" {
		return nil
	}
	if isGovernment {
		return platform.NewError(http.StatusForbidden, "งานขายราชการทำได้เฉพาะผู้ดูแลระบบผ่านเมนู รพ.สต.")
	}
	for _, item := range items {
		if item.AliasID != nil && strings.TrimSpace(*item.AliasID) != "" {
			return platform.NewError(http.StatusForbidden, "พนักงานขายหน้าร้านไม่สามารถใช้ชื่อสินค้าราชการ")
		}
	}
	return nil
}

func resolveLineDisplayAndPrice(productName string, basePrice float64, aliasID sql.NullString, aliasName sql.NullString, aliasDefaultPrice sql.NullFloat64, isGovernment bool, overrideUnitPrice *float64, allowOverride bool) (string, float64, string, error) {
	displayName := productName
	priceSource := "branch_price"
	unitPrice := basePrice

	if isGovernment && aliasID.Valid {
		displayName = aliasName.String
		if aliasDefaultPrice.Valid {
			unitPrice = aliasDefaultPrice.Float64
			priceSource = "government_alias_default"
		}
	}
	if overrideUnitPrice != nil {
		if !allowOverride {
			return "", 0, "", platform.NewError(http.StatusForbidden, "price override is not allowed")
		}
		unitPrice = *overrideUnitPrice
		priceSource = "override"
	}
	return displayName, unitPrice, priceSource, nil
}

func calculateLineAmounts(unitPrice float64, quantity int, vatRate float64, taxExempt bool) (float64, float64, float64, float64) {
	lineSubtotal := platform.Round2(unitPrice * float64(quantity))
	lineTaxRate := vatRate
	if taxExempt {
		lineTaxRate = 0
	}
	lineTaxAmount := platform.Round2(lineSubtotal * lineTaxRate / 100)
	lineTotal := platform.Round2(lineSubtotal + lineTaxAmount)
	return lineSubtotal, lineTaxRate, lineTaxAmount, lineTotal
}

func (s *Service) lockAndApplyStock(ctx context.Context, tx *sql.Tx, branchID, invoiceID string, user platform.AuthUser, lines []pricedLine, meta audit.LogEntry) error {
	type aggregateKey struct {
		ProductID   string
		StockBucket string
	}
	type inventoryState struct {
		BeforeReal  int
		BeforeGhost int
		AfterReal   int
		AfterGhost  int
	}
	required := map[aggregateKey]int{}
	lotRequired := map[string]int{}
	for _, line := range lines {
		if strings.TrimSpace(line.InventoryLotID) == "" {
			return platform.NewError(http.StatusBadRequest, fmt.Sprintf("กรุณาเลือก lot ของ %s", line.ProductName))
		}
		required[aggregateKey{ProductID: line.ProductID, StockBucket: line.StockBucket}] += line.Quantity
		lotRequired[line.InventoryLotID] += line.Quantity
	}

	keys := make([]aggregateKey, 0, len(required))
	for key := range required {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ProductID == keys[j].ProductID {
			return keys[i].StockBucket < keys[j].StockBucket
		}
		return keys[i].ProductID < keys[j].ProductID
	})

	inventoryStates := map[aggregateKey]inventoryState{}
	for _, key := range keys {
		var qtyReal, qtyGhost int
		if err := tx.QueryRowContext(ctx, `
			SELECT qty_real, qty_ghost
			FROM inventory
			WHERE branch_id = $1 AND product_id = $2
			FOR UPDATE
		`, branchID, key.ProductID).Scan(&qtyReal, &qtyGhost); err != nil {
			return err
		}

		needed := required[key]
		state := inventoryState{BeforeReal: qtyReal, BeforeGhost: qtyGhost, AfterReal: qtyReal, AfterGhost: qtyGhost}
		if key.StockBucket == "real" {
			if qtyReal < needed {
				return platform.NewError(http.StatusConflict, fmt.Sprintf("สต๊อกจริงไม่เพียงพอสำหรับสินค้า %s", key.ProductID))
			}
			state.AfterReal -= needed
		} else {
			if qtyGhost < needed {
				return platform.NewError(http.StatusConflict, fmt.Sprintf("สต๊อกผีไม่เพียงพอสำหรับสินค้า %s", key.ProductID))
			}
			state.AfterGhost -= needed
		}
		inventoryStates[key] = state
	}

	lotIDs := make([]string, 0, len(lotRequired))
	for lotID := range lotRequired {
		lotIDs = append(lotIDs, lotID)
	}
	sort.Strings(lotIDs)
	lockedLots := map[string]stocklot.Lot{}
	for _, lotID := range lotIDs {
		var lot stocklot.Lot
		var expired bool
		if err := tx.QueryRowContext(ctx, `
			SELECT id::text,branch_id::text,product_id::text,stock_bucket,lot_number,
			       expires_on,received_quantity,remaining_quantity,unit_cost,received_at,
			       COALESCE(expires_on < (NOW() AT TIME ZONE 'Asia/Bangkok')::date,FALSE)
			FROM inventory_lots WHERE id=$1 FOR UPDATE
		`, lotID).Scan(&lot.ID, &lot.BranchID, &lot.ProductID, &lot.StockBucket, &lot.LotNumber,
			&lot.ExpiresOn, &lot.ReceivedQuantity, &lot.RemainingQuantity, &lot.UnitCost, &lot.ReceivedAt, &expired); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusConflict, "ไม่พบ lot ที่เลือก")
			}
			return err
		}
		if lot.BranchID != branchID || expired {
			return platform.NewError(http.StatusConflict, "lot ที่เลือกไม่อยู่ในสาขานี้หรือหมดอายุแล้ว")
		}
		if lot.RemainingQuantity < lotRequired[lotID] {
			return platform.NewError(http.StatusConflict, "จำนวนใน lot ที่เลือกไม่เพียงพอ")
		}
		lockedLots[lotID] = lot
	}

	for index := range lines {
		lot := lockedLots[lines[index].InventoryLotID]
		if lot.ProductID != lines[index].ProductID || lot.StockBucket != lines[index].StockBucket {
			return platform.NewError(http.StatusConflict, fmt.Sprintf("lot ที่เลือกไม่ตรงกับสินค้า %s", lines[index].ProductName))
		}
		lines[index].LotNumber = lot.LotNumber
		lines[index].LotReceivedAt = lot.ReceivedAt
		lines[index].LotExpiresOn = lot.ExpiresOn
		lines[index].CostSnapshot = lot.UnitCost
	}

	for _, lotID := range lotIDs {
		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory_lots
			SET remaining_quantity=remaining_quantity-$2,updated_at=NOW()
			WHERE id=$1
		`, lotID, lotRequired[lotID]); err != nil {
			return err
		}
	}

	for _, key := range keys {
		state := inventoryStates[key]

		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory
			SET qty_real = $3, qty_ghost = $4, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, branchID, key.ProductID, state.AfterReal, state.AfterGhost); err != nil {
			return err
		}
		productEntity := key.ProductID
		logEntry := meta
		logEntry.EntityType = "inventory"
		logEntry.EntityID = &productEntity
		logEntry.Action = "inventory.deduct_for_sale"
		logEntry.Before = map[string]any{"qty_real": state.BeforeReal, "qty_ghost": state.BeforeGhost}
		logEntry.After = map[string]any{"qty_real": state.AfterReal, "qty_ghost": state.AfterGhost, "stock_bucket": key.StockBucket, "deducted": required[key]}
		if err := s.audit.Log(ctx, tx, logEntry); err != nil {
			return err
		}
	}

	for _, line := range lines {
		movementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'sale', $4, $5, 'invoice', $6, '', $7, NOW())
		`, movementID, branchID, line.ProductID, line.StockBucket, -line.Quantity, invoiceID, user.ID); err != nil {
			return err
		}
		allocation := stocklot.Allocation{Lot: lockedLots[line.InventoryLotID], Quantity: line.Quantity}
		if err := stocklot.AttachMovement(ctx, tx, movementID, []stocklot.Allocation{allocation}, -1); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) LotOptions(ctx context.Context, user platform.AuthUser, branchID, productID, bucket string) ([]map[string]any, error) {
	if err := validateBranchScope(user, branchID); err != nil {
		return nil, err
	}
	if err := validateSalesBranch(ctx, s.db, branchID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(productID) == "" {
		return nil, platform.NewError(http.StatusBadRequest, "กรุณาเลือกสินค้า")
	}
	if err := stocklot.ValidateBucket(bucket); err != nil {
		return nil, err
	}
	if bucket != "real" {
		return nil, platform.NewError(http.StatusBadRequest, "การขายใช้ได้เฉพาะ lot สต๊อกจริง")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT il.id::text,il.lot_number,il.received_at,il.expires_on,
		       COALESCE(bpp.selling_price,p.base_selling_price),il.remaining_quantity,
		       il.unit_cost,il.source_type,COALESCE(po.po_number,''),
		       COALESCE(po.supplier_name_snapshot,'')
		FROM inventory_lots il
		INNER JOIN products p ON p.id=il.product_id AND p.active=TRUE
		LEFT JOIN branch_product_prices bpp ON bpp.branch_id=il.branch_id AND bpp.product_id=il.product_id
		LEFT JOIN purchase_order_items poi ON poi.id=il.source_item_id
		LEFT JOIN purchase_orders po ON po.id=poi.purchase_order_id
		WHERE il.branch_id=$1 AND il.product_id=$2 AND il.stock_bucket=$3
		  AND il.remaining_quantity>0
		  AND (il.expires_on IS NULL OR il.expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)
		ORDER BY il.expires_on ASC NULLS LAST,il.received_at,il.id
	`, branchID, productID, bucket)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, lotNumber, sourceType, poNumber, supplierName string
		var receivedAt time.Time
		var expiresOn sql.NullTime
		var sellingPrice, unitCost float64
		var remaining int
		if err := rows.Scan(&id, &lotNumber, &receivedAt, &expiresOn, &sellingPrice, &remaining, &unitCost, &sourceType, &poNumber, &supplierName); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id": id, "lot_number": lotNumber, "received_at": receivedAt,
			"expires_on": nullableSQLTime(expiresOn), "selling_price": sellingPrice,
		}
		if user.RoleKey == "super_admin" {
			item["remaining_quantity"] = remaining
			item["unit_cost"] = unitCost
			item["stock_bucket"] = bucket
			item["source_type"] = sourceType
			item["po_number"] = poNumber
			item["supplier_name"] = supplierName
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) LotOptions(c echo.Context) error {
	user := platform.CurrentUser(c)
	branchID := strings.TrimSpace(c.QueryParam("branch_id"))
	if branchID == "" && user.BranchID != nil {
		branchID = *user.BranchID
	}
	items, err := h.service.LotOptions(c.Request().Context(), user, branchID, strings.TrimSpace(c.QueryParam("product_id")), strings.TrimSpace(c.QueryParam("stock_bucket")))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) PreviewQuotation(c echo.Context) error {
	var input QuoteRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	result, err := h.service.Preview(c.Request().Context(), user, input.BranchID, input.IsGovernment, input.Items, input.BillDiscountAmount)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) PreviewInvoice(c echo.Context) error {
	var input InvoiceRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	result, err := h.service.PreviewSale(c.Request().Context(), user, input.BranchID, input.IsGovernment, input.Items, input.BillDiscountAmount)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) PreviewCheckout(c echo.Context) error {
	var input CheckoutRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลการขายไม่ถูกต้อง"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	result, err := h.service.PreviewCheckout(c.Request().Context(), user, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) Checkout(c echo.Context) error {
	var input CheckoutRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลการขายไม่ถูกต้อง"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	result, err := h.service.Checkout(c.Request().Context(), user, audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	result["message"] = "ชำระเงินและออกใบเสร็จแล้ว"
	return platform.JSON(c, http.StatusCreated, result)
}

func (h *Handler) CreateQuotation(c echo.Context) error {
	var input QuoteRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	meta := audit.MetaFromContext(c)
	id, err := h.service.CreateQuotation(c.Request().Context(), user, meta, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "สร้างใบเสนอราคาแล้ว"})
}

func (h *Handler) ListQuotations(c echo.Context) error {
	governmentMode, err := optionalBooleanQuery(c.QueryParam("government_mode"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	items, err := h.service.ListQuotations(c.Request().Context(), platform.CurrentUser(c), governmentMode)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load quotations", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetQuotation(c echo.Context) error {
	item, err := h.service.GetQuotation(c.Request().Context(), platform.CurrentUser(c), c.Param("quotationID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}

func (h *Handler) ConvertQuotation(c echo.Context) error {
	var input ConvertQuotationRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลการจัดสรร lot ไม่ถูกต้อง"))
	}
	meta := audit.MetaFromContext(c)
	id, err := h.service.ConvertQuotation(c.Request().Context(), platform.CurrentUser(c), meta, c.Param("quotationID"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "แปลงใบเสนอราคาเป็นใบขายแล้ว"})
}

func (h *Handler) QuotationDeletionImpact(c echo.Context) error {
	impact, err := h.service.QuotationDeletionImpact(c.Request().Context(), platform.CurrentUser(c), c.Param("quotationID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, impact)
}

func (h *Handler) DeleteQuotation(c echo.Context) error {
	var input DeleteDocumentRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลยืนยันไม่ถูกต้อง"))
	}
	if err := h.service.DeleteQuotation(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("quotationID"), input.Confirmation); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบใบเสนอราคาและข้อมูลที่เกี่ยวข้องแล้ว")
}

func (h *Handler) CreateInvoice(c echo.Context) error {
	var input InvoiceRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	meta := audit.MetaFromContext(c)
	id, err := h.service.CreateInvoice(c.Request().Context(), user, meta, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "สร้างใบขายแล้ว"})
}

func (h *Handler) ListInvoices(c echo.Context) error {
	governmentMode, err := optionalBooleanQuery(c.QueryParam("government_mode"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	items, err := h.service.ListInvoices(c.Request().Context(), platform.CurrentUser(c), governmentMode)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load invoices", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func optionalBooleanQuery(raw string) (*bool, error) {
	switch strings.TrimSpace(raw) {
	case "":
		return nil, nil
	case "true":
		value := true
		return &value, nil
	case "false":
		value := false
		return &value, nil
	default:
		return nil, platform.NewError(http.StatusBadRequest, "government_mode must be true or false")
	}
}

func (h *Handler) InvoiceDeletionImpact(c echo.Context) error {
	impact, err := h.service.InvoiceDeletionImpact(c.Request().Context(), platform.CurrentUser(c), c.Param("invoiceID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, impact)
}

func (h *Handler) DeleteInvoice(c echo.Context) error {
	var input DeleteDocumentRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลยืนยันไม่ถูกต้อง"))
	}
	if err := h.service.DeleteInvoice(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("invoiceID"), input.Confirmation); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบใบขาย คืนสต๊อก และลบข้อมูลที่เกี่ยวข้องแล้ว")
}

func (h *Handler) GetInvoice(c echo.Context) error {
	item, err := h.service.GetInvoice(c.Request().Context(), platform.CurrentUser(c), c.Param("invoiceID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}

func (h *Handler) GetInvoicePrint(c echo.Context) error {
	item, err := h.service.GetInvoicePrint(c.Request().Context(), platform.CurrentUser(c), c.Param("invoiceID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}

func (h *Handler) CollectPayment(c echo.Context) error {
	var input PaymentRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.CollectPayment(c.Request().Context(), platform.CurrentUser(c), meta, c.Param("invoiceID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "รับชำระเงินแล้ว")
}
