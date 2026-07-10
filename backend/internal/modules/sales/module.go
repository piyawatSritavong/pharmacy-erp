package sales

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type LineInput struct {
	ProductID         string   `json:"product_id"`
	AliasID           *string  `json:"alias_id"`
	Quantity          int      `json:"quantity"`
	StockBucket       string   `json:"stock_bucket"`
	OverrideUnitPrice *float64 `json:"override_unit_price"`
	OverrideReason    string   `json:"override_reason"`
}

type QuoteRequest struct {
	BranchID      string      `json:"branch_id"`
	CustomerName  string      `json:"customer_name"`
	CustomerTaxID string      `json:"customer_tax_id"`
	IsGovernment  bool        `json:"is_government_mode"`
	Items         []LineInput `json:"items"`
	ExpiresAt     *time.Time  `json:"expires_at"`
}

type InvoiceRequest struct {
	BranchID          string      `json:"branch_id"`
	CustomerName      string      `json:"customer_name"`
	CustomerTaxID     string      `json:"customer_tax_id"`
	IsGovernment      bool        `json:"is_government_mode"`
	Items             []LineInput `json:"items"`
	SourceQuotationID *string     `json:"source_quotation_id"`
}

type PaymentRequest struct {
	PaymentType   string `json:"payment_type"`
	ReferenceCode string `json:"reference_code"`
	Notes         string `json:"notes"`
}

type pricedLine struct {
	ProductID      string
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
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func canOverride(user platform.AuthUser) bool {
	return platform.HasPermission(user, "price.override.global") || platform.HasPermission(user, "price.override.branch") || platform.HasPermission(user, "price.override.pos")
}

func (s *Service) Preview(ctx context.Context, user platform.AuthUser, branchID string, isGovernment bool, items []LineInput) (map[string]any, error) {
	if err := validateBranchScope(user, branchID); err != nil {
		return nil, err
	}
	vatRate, err := platform.GetSettingFloat(ctx, s.db, "vat_rate", 7)
	if err != nil {
		return nil, err
	}
	lines, subtotal, taxAmount, totalAmount, err := s.priceLines(ctx, s.db, user, branchID, isGovernment, items, vatRate)
	if err != nil {
		return nil, err
	}
	return buildPreview(lines, subtotal, taxAmount, totalAmount, vatRate), nil
}

func buildPreview(lines []pricedLine, subtotal, taxAmount, totalAmount, vatRate float64) map[string]any {
	payloadLines := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		payloadLines = append(payloadLines, map[string]any{
			"product_id":      line.ProductID,
			"alias_id":        line.AliasID,
			"actual_name":     line.ProductName,
			"display_name":    line.DisplayName,
			"quantity":        line.Quantity,
			"stock_bucket":    line.StockBucket,
			"unit_price":      line.UnitPrice,
			"line_subtotal":   line.LineSubtotal,
			"tax_rate":        line.TaxRate,
			"tax_amount":      line.TaxAmount,
			"line_total":      line.LineTotal,
			"cost_snapshot":   line.CostSnapshot,
			"price_source":    line.PriceSource,
			"override_reason": line.OverrideReason,
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

func (s *Service) CreateQuotation(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input QuoteRequest) (string, error) {
	if err := validateBranchScope(user, input.BranchID); err != nil {
		return "", err
	}
	var quoteID string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		createdAt := time.Now().UTC()
		vatRate, err := platform.GetSettingFloat(ctx, tx, "vat_rate", 7)
		if err != nil {
			return err
		}
		lines, subtotal, taxAmount, totalAmount, err := s.priceLines(ctx, tx, user, input.BranchID, input.IsGovernment, input.Items, vatRate)
		if err != nil {
			return err
		}
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
			INSERT INTO quotations (id, branch_id, quote_number, customer_name, customer_tax_id, status, subtotal, tax_rate, tax_amount, total_amount, created_by, expires_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'draft', $6, $7, $8, $9, $10, $11, $12, $12)
		`, quoteID, input.BranchID, quoteNumber, strings.TrimSpace(input.CustomerName), platform.NullString(input.CustomerTaxID), subtotal, vatRate, taxAmount, totalAmount, user.ID, expiry, createdAt); err != nil {
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
	var invoiceID string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		issuedAt := time.Now().UTC()
		vatRate, err := platform.GetSettingFloat(ctx, tx, "vat_rate", 7)
		if err != nil {
			return err
		}
		lines, subtotal, taxAmount, totalAmount, err := s.priceLines(ctx, tx, user, input.BranchID, input.IsGovernment, input.Items, vatRate)
		if err != nil {
			return err
		}
		if err := s.lockAndApplyStock(ctx, tx, input.BranchID, user, lines, meta); err != nil {
			return err
		}

		invoiceNumber, err := nextDocumentNumber(ctx, tx, input.BranchID, "invoice", issuedAt)
		if err != nil {
			return err
		}
		invoiceID = platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invoices (id, branch_id, invoice_number, source_quote_id, customer_name, customer_tax_id, payment_status, invoice_status, is_government_mode, subtotal, tax_rate, tax_amount, total_amount, created_by, issued_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'unpaid', 'issued', $7, $8, $9, $10, $11, $12, $13, $13, $13)
		`, invoiceID, input.BranchID, invoiceNumber, platform.NullUUID(input.SourceQuotationID), strings.TrimSpace(input.CustomerName), platform.NullString(input.CustomerTaxID), input.IsGovernment, subtotal, vatRate, taxAmount, totalAmount, user.ID, issuedAt); err != nil {
			return err
		}

		for _, line := range lines {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO invoice_items (id, invoice_id, product_id, alias_id, actual_product_name, display_name, quantity, stock_bucket, unit_price, line_subtotal, tax_rate, tax_amount, line_total, price_source, override_reason, cost_snapshot, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW())
			`, platform.MustUUID(), invoiceID, line.ProductID, platform.NullUUID(line.AliasID), line.ProductName, line.DisplayName, line.Quantity, line.StockBucket, line.UnitPrice, line.LineSubtotal, line.TaxRate, line.TaxAmount, line.LineTotal, line.PriceSource, line.OverrideReason, line.CostSnapshot); err != nil {
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
			WHERE id = $1
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
		`, platform.MustUUID(), invoiceID, input.PaymentType, totalAmount, strings.TrimSpace(input.ReferenceCode), strings.TrimSpace(input.Notes), user.ID); err != nil {
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
	if user.RoleKey != "branch_pos" {
		return platform.NewError(http.StatusForbidden, "only branch POS can collect direct cash or bank payments")
	}
	return nil
}

func (s *Service) ListQuotations(ctx context.Context, user platform.AuthUser) ([]map[string]any, error) {
	query := `
		SELECT q.id, q.quote_number, q.customer_name, q.status, q.total_amount, b.name, q.created_at
		FROM quotations q
		INNER JOIN branches b ON b.id = q.branch_id
	`
	args := []any{}
	if user.BranchID != nil && user.RoleKey != "super_admin" {
		args = append(args, *user.BranchID)
		query += " WHERE q.branch_id = $1"
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
		var createdAt time.Time
		if err := rows.Scan(&id, &quoteNumber, &customerName, &status, &totalAmount, &branchName, &createdAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":            id,
			"quote_number":  quoteNumber,
			"customer_name": customerName,
			"status":        status,
			"total_amount":  totalAmount,
			"branch_name":   branchName,
			"created_at":    createdAt,
		})
	}
	return items, rows.Err()
}

func (s *Service) ListInvoices(ctx context.Context, user platform.AuthUser) ([]map[string]any, error) {
	query := `
		SELECT i.id, i.invoice_number, i.customer_name, i.payment_status, i.total_amount, i.is_government_mode, b.name, i.issued_at
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
	`
	args := []any{}
	if user.BranchID != nil && user.RoleKey != "super_admin" {
		args = append(args, *user.BranchID)
		query += " WHERE i.branch_id = $1"
	}
	query += " ORDER BY i.issued_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, invoiceNumber, customerName, paymentStatus, branchName string
		var totalAmount float64
		var isGovernment bool
		var issuedAt time.Time
		if err := rows.Scan(&id, &invoiceNumber, &customerName, &paymentStatus, &totalAmount, &isGovernment, &branchName, &issuedAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":                 id,
			"invoice_number":     invoiceNumber,
			"customer_name":      customerName,
			"payment_status":     paymentStatus,
			"total_amount":       totalAmount,
			"is_government_mode": isGovernment,
			"branch_name":        branchName,
			"issued_at":          issuedAt,
		})
	}
	return items, rows.Err()
}

func (s *Service) GetInvoice(ctx context.Context, user platform.AuthUser, invoiceID string) (map[string]any, error) {
	var branchID string
	var id, invoiceNumber, customerName, customerTaxID, paymentStatus, branchName string
	var isGovernment bool
	var subtotal, taxRate, taxAmount, totalAmount float64
	var issuedAt time.Time
	if err := s.db.QueryRowContext(ctx, `
		SELECT i.id, i.branch_id::text, i.invoice_number, i.customer_name, COALESCE(i.customer_tax_id, ''), i.payment_status, i.is_government_mode, i.subtotal, i.tax_rate, i.tax_amount, i.total_amount, b.name, i.issued_at
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
		WHERE i.id = $1
	`, invoiceID).Scan(
		&id, &branchID, &invoiceNumber, &customerName, &customerTaxID, &paymentStatus, &isGovernment, &subtotal, &taxRate, &taxAmount, &totalAmount, &branchName, &issuedAt,
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
		"subtotal":           subtotal,
		"tax_rate":           taxRate,
		"tax_amount":         taxAmount,
		"total_amount":       totalAmount,
		"branch_name":        branchName,
		"issued_at":          issuedAt,
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT product_id::text, COALESCE(alias_id::text, ''), actual_product_name, display_name, quantity, stock_bucket, unit_price, line_subtotal, tax_amount, line_total, price_source, override_reason, cost_snapshot
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
		var productID, aliasID, actualName, displayName, stockBucket, priceSource, overrideReason string
		var quantity int
		var unitPrice, lineSubtotal, taxAmount, lineTotal, costSnapshot float64
		if err := rows.Scan(&productID, &aliasID, &actualName, &displayName, &quantity, &stockBucket, &unitPrice, &lineSubtotal, &taxAmount, &lineTotal, &priceSource, &overrideReason, &costSnapshot); err != nil {
			return nil, err
		}
		item := map[string]any{
			"product_id":      productID,
			"actual_name":     actualName,
			"display_name":    displayName,
			"quantity":        quantity,
			"stock_bucket":    stockBucket,
			"unit_price":      unitPrice,
			"line_subtotal":   lineSubtotal,
			"tax_amount":      taxAmount,
			"line_total":      lineTotal,
			"price_source":    priceSource,
			"override_reason": overrideReason,
			"cost_snapshot":   costSnapshot,
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
		id, branchID, branchCode, branchName, branchAddress string
		invoiceNumber, customerName, customerTaxID          string
		paymentStatus, invoiceStatus, sellerName            string
		isGovernment                                         bool
		subtotal, taxRate, taxAmount, totalAmount           float64
		issuedAt                                             time.Time
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
			i.subtotal,
			i.tax_rate,
			i.tax_amount,
			i.total_amount,
			u.full_name,
			i.issued_at
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
		INNER JOIN users u ON u.id = i.created_by
		WHERE i.id = $1
	`, invoiceID).Scan(
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
			cost_snapshot
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
			quantity                                    int
			unitPrice, lineSubtotal, lineTaxRate        float64
			lineTaxAmount, lineTotal, costSnapshot      float64
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
		); err != nil {
			return nil, err
		}
		item := map[string]any{
			"product_id":       productID,
			"actual_name":      actualName,
			"display_name":     displayName,
			"quantity":         quantity,
			"stock_bucket":     stockBucket,
			"unit_price":       unitPrice,
			"line_subtotal":    lineSubtotal,
			"tax_rate":         lineTaxRate,
			"tax_amount":       lineTaxAmount,
			"line_total":       lineTotal,
			"price_source":     priceSource,
			"override_reason":  overrideReason,
			"cost_snapshot":    costSnapshot,
			"is_alias_display": aliasID != "",
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

func (s *Service) ConvertQuotation(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, quotationID string) (string, error) {
	var branchID, customerName, customerTaxID string
	var isGovernment bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT branch_id::text, customer_name, COALESCE(customer_tax_id, ''), FALSE
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
		SELECT product_id::text, COALESCE(alias_id::text, ''), quantity, stock_bucket, unit_price, override_reason
		FROM quotation_items
		WHERE quotation_id = $1
		ORDER BY created_at ASC
	`, quotationID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	items := []LineInput{}
	for rows.Next() {
		var productID, aliasID, stockBucket, overrideReason string
		var quantity int
		var unitPrice float64
		if err := rows.Scan(&productID, &aliasID, &quantity, &stockBucket, &unitPrice, &overrideReason); err != nil {
			return "", err
		}
		override := unitPrice
		item := LineInput{
			ProductID:         productID,
			Quantity:          quantity,
			StockBucket:       stockBucket,
			OverrideUnitPrice: &override,
			OverrideReason:    overrideReason,
		}
		if aliasID != "" {
			item.AliasID = &aliasID
			isGovernment = true
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	return s.CreateInvoice(ctx, user, meta, InvoiceRequest{
		BranchID:          branchID,
		CustomerName:      customerName,
		CustomerTaxID:     customerTaxID,
		IsGovernment:      isGovernment,
		Items:             items,
		SourceQuotationID: &quotationID,
	})
}

func validateBranchScope(user platform.AuthUser, branchID string) error {
	if user.BranchID != nil && user.RoleKey != "super_admin" && *user.BranchID != branchID {
		return platform.NewError(http.StatusForbidden, "branch scope mismatch")
	}
	return nil
}

func nextDocumentNumber(ctx context.Context, tx *sql.Tx, branchID string, docType string, issuedAt time.Time) (string, error) {
	var prefix string
	var nextNumber int64
	if err := tx.QueryRowContext(ctx, `
		SELECT prefix, next_number
		FROM document_sequences
		WHERE branch_id = $1 AND doc_type = $2
		FOR UPDATE
	`, branchID, docType).Scan(&prefix, &nextNumber); err != nil {
		return "", err
	}
	number := platform.FormatSalesDocNumber(prefix, issuedAt, nextNumber)
	if _, err := tx.ExecContext(ctx, `
		UPDATE document_sequences
		SET next_number = next_number + 1, updated_at = NOW()
		WHERE branch_id = $1 AND doc_type = $2
	`, branchID, docType); err != nil {
		return "", err
	}
	return number, nil
}

func (s *Service) priceLines(ctx context.Context, db platform.DBTX, user platform.AuthUser, branchID string, isGovernment bool, items []LineInput, vatRate float64) ([]pricedLine, float64, float64, float64, error) {
	if len(items) == 0 {
		return nil, 0, 0, 0, platform.NewError(http.StatusBadRequest, "at least one line is required")
	}

	lines := make([]pricedLine, 0, len(items))
	var subtotal float64
	var taxAmount float64

	for _, item := range items {
		if item.Quantity <= 0 {
			return nil, 0, 0, 0, platform.NewError(http.StatusBadRequest, "quantity must be greater than zero")
		}
		if item.StockBucket != "real" && item.StockBucket != "ghost" {
			return nil, 0, 0, 0, platform.NewError(http.StatusBadRequest, "stock bucket must be real or ghost")
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
		)
		if err := db.QueryRowContext(ctx, `
			SELECT p.id::text, p.name, p.cost_price, COALESCE(bpp.selling_price, p.base_selling_price), p.tax_exempt,
			       a.id::text, a.alias_name, a.default_government_price
			FROM products p
			LEFT JOIN branch_product_prices bpp ON bpp.product_id = p.id AND bpp.branch_id = $2
			LEFT JOIN product_aliases a ON a.id = $3 AND a.product_id = p.id AND a.active = TRUE AND (a.branch_id IS NULL OR a.branch_id = $2)
			WHERE p.id = $1 AND p.active = TRUE
		`, item.ProductID, branchID, platform.NullUUID(item.AliasID)).Scan(
			&productID, &productName, &costPrice, &basePrice, &taxExempt, &aliasID, &aliasName, &aliasDefaultPrice,
		); err != nil {
			if err == sql.ErrNoRows {
				return nil, 0, 0, 0, platform.NewError(http.StatusNotFound, "product not found")
			}
			return nil, 0, 0, 0, err
		}
		if err := validateAliasSelection(item.AliasID, aliasID); err != nil {
			return nil, 0, 0, 0, err
		}

		displayName, unitPrice, priceSource, err := resolveLineDisplayAndPrice(productName, basePrice, aliasID, aliasName, aliasDefaultPrice, isGovernment, item.OverrideUnitPrice, canOverride(user))
		if err != nil {
			return nil, 0, 0, 0, err
		}
		lineSubtotal, lineTaxRate, lineTaxAmount, lineTotal := calculateLineAmounts(unitPrice, item.Quantity, vatRate, taxExempt)

		subtotal = platform.Round2(subtotal + lineSubtotal)
		taxAmount = platform.Round2(taxAmount + lineTaxAmount)

		lines = append(lines, pricedLine{
			ProductID:      productID,
			AliasID:        platform.StringPointer(aliasID),
			ProductName:    productName,
			DisplayName:    displayName,
			Quantity:       item.Quantity,
			StockBucket:    item.StockBucket,
			UnitPrice:      platform.Round2(unitPrice),
			LineSubtotal:   lineSubtotal,
			TaxRate:        lineTaxRate,
			TaxAmount:      lineTaxAmount,
			LineTotal:      lineTotal,
			CostSnapshot:   costPrice,
			PriceSource:    priceSource,
			OverrideReason: strings.TrimSpace(item.OverrideReason),
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

func (s *Service) lockAndApplyStock(ctx context.Context, tx *sql.Tx, branchID string, user platform.AuthUser, lines []pricedLine, meta audit.LogEntry) error {
	type aggregateKey struct {
		ProductID   string
		StockBucket string
	}
	required := map[aggregateKey]int{}
	for _, line := range lines {
		required[aggregateKey{ProductID: line.ProductID, StockBucket: line.StockBucket}] += line.Quantity
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
		before := map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost}
		if key.StockBucket == "real" {
			if qtyReal < needed {
				return platform.NewError(http.StatusConflict, fmt.Sprintf("insufficient real stock for product %s", key.ProductID))
			}
			qtyReal -= needed
		} else {
			if qtyGhost < needed {
				return platform.NewError(http.StatusConflict, fmt.Sprintf("insufficient ghost stock for product %s", key.ProductID))
			}
			qtyGhost -= needed
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory
			SET qty_real = $3, qty_ghost = $4, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, branchID, key.ProductID, qtyReal, qtyGhost); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'sale', $4, $5, 'invoice', '', $6, NOW())
		`, platform.MustUUID(), branchID, key.ProductID, key.StockBucket, -needed, user.ID); err != nil {
			return err
		}

		productEntity := key.ProductID
		logEntry := meta
		logEntry.EntityType = "inventory"
		logEntry.EntityID = &productEntity
		logEntry.Action = "inventory.deduct_for_sale"
		logEntry.Before = before
		logEntry.After = map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost, "stock_bucket": key.StockBucket, "deducted": needed}
		if err := s.audit.Log(ctx, tx, logEntry); err != nil {
			return err
		}
	}
	return nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
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
	result, err := h.service.Preview(c.Request().Context(), user, input.BranchID, input.IsGovernment, input.Items)
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
	result, err := h.service.Preview(c.Request().Context(), user, input.BranchID, input.IsGovernment, input.Items)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
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
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "quotation created"})
}

func (h *Handler) ListQuotations(c echo.Context) error {
	items, err := h.service.ListQuotations(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load quotations", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ConvertQuotation(c echo.Context) error {
	meta := audit.MetaFromContext(c)
	id, err := h.service.ConvertQuotation(c.Request().Context(), platform.CurrentUser(c), meta, c.Param("quotationID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "quotation converted"})
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
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "invoice created"})
}

func (h *Handler) ListInvoices(c echo.Context) error {
	items, err := h.service.ListInvoices(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load invoices", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
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
	return platform.JSONMessage(c, http.StatusOK, "payment collected")
}
