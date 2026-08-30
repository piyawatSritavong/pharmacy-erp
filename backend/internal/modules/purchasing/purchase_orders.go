package purchasing

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/stocklot"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type NewProductInput struct {
	SKU                    string  `json:"sku"`
	Barcode                string  `json:"barcode"`
	CategoryID             *string `json:"category_id"`
	Name                   string  `json:"name"`
	Description            string  `json:"description"`
	BaseSellingPrice       float64 `json:"base_selling_price"`
	UnitName               string  `json:"unit_name"`
	TaxExempt              bool    `json:"tax_exempt"`
	MaxDiscountAmount      float64 `json:"max_discount_amount"`
	LowStockRealThreshold  int     `json:"low_stock_real_threshold"`
	LowStockGhostThreshold int     `json:"low_stock_ghost_threshold"`
	TracksExpiry           bool    `json:"tracks_expiry"`
	ExpiryWarningDays      int     `json:"expiry_warning_days"`
}

type PurchaseOrderLineInput struct {
	ID           string           `json:"id"`
	ProductID    string           `json:"product_id"`
	NewProduct   *NewProductInput `json:"new_product"`
	StockBucket  string           `json:"stock_bucket"`
	Quantity     int              `json:"quantity"`
	UnitCost     float64          `json:"unit_cost"`
	LineDiscount float64          `json:"line_discount"`
	LotNumber    string           `json:"lot_number"`
	ExpiresOn    string           `json:"expires_on"`
}

type PurchaseOrderInput struct {
	BranchID                string                   `json:"branch_id"`
	SupplierID              string                   `json:"supplier_id"`
	PurchasedAt             string                   `json:"purchased_at"`
	DueDate                 string                   `json:"due_date"`
	JobName                 string                   `json:"job_name"`
	DeliveryTerms           string                   `json:"delivery_terms"`
	SupplierDocumentNumber  string                   `json:"supplier_document_number"`
	SupplierAddressSnapshot string                   `json:"supplier_address_snapshot"`
	VATMode                 string                   `json:"vat_mode"`
	VATRate                 float64                  `json:"vat_rate"`
	HeaderDiscount          float64                  `json:"header_discount"`
	ShippingAmount          float64                  `json:"shipping_amount"`
	Notes                   string                   `json:"notes"`
	CorrectionReason        string                   `json:"correction_reason"`
	Items                   []PurchaseOrderLineInput `json:"items"`
}

type resolvedPurchaseLine struct {
	Input          PurchaseOrderLineInput
	ID             string
	ProductID      string
	SKU            string
	Name           string
	UnitName       string
	TracksExpiry   bool
	ExpiresOn      sql.NullTime
	LineSubtotal   float64
	LineNet        float64
	LineTax        float64
	LineTotal      float64
	InventoryLotID string
}

type amountSummary struct {
	Subtotal       float64
	LineDiscount   float64
	HeaderDiscount float64
	Shipping       float64
	Tax            float64
	Total          float64
	VATMode        string
	VATRate        float64
}

func parsePurchasedAt(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Now().UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, platform.NewError(http.StatusBadRequest, "วันเวลาซื้อไม่ถูกต้อง")
	}
	return parsed.UTC(), nil
}

func parseExpiry(value string) (sql.NullTime, error) {
	if strings.TrimSpace(value) == "" {
		return sql.NullTime{}, nil
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return sql.NullTime{}, platform.NewError(http.StatusBadRequest, "วันหมดอายุต้องเป็นรูปแบบ YYYY-MM-DD")
	}
	return sql.NullTime{Time: parsed, Valid: true}, nil
}

func normalizeVAT(mode string, rate float64) (string, float64, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "exclusive"
	}
	if mode != "none" && mode != "exclusive" && mode != "inclusive" {
		return "", 0, platform.NewError(http.StatusBadRequest, "รูปแบบ VAT ไม่ถูกต้อง")
	}
	if mode == "none" {
		return mode, 0, nil
	}
	if rate == 0 {
		rate = 7
	}
	if rate < 0 || rate > 100 {
		return "", 0, platform.NewError(http.StatusBadRequest, "อัตรา VAT ไม่ถูกต้อง")
	}
	return mode, platform.Round2(rate), nil
}

func calculatePurchaseAmounts(lines []resolvedPurchaseLine, mode string, rate, headerDiscount, shipping float64) (amountSummary, error) {
	if headerDiscount < 0 || shipping < 0 {
		return amountSummary{}, platform.NewError(http.StatusBadRequest, "ส่วนลดและค่าขนส่งต้องไม่ติดลบ")
	}
	summary := amountSummary{VATMode: mode, VATRate: rate, HeaderDiscount: platform.Round2(headerDiscount), Shipping: platform.Round2(shipping)}
	for index := range lines {
		line := &lines[index]
		line.LineSubtotal = platform.Round2(line.Input.UnitCost * float64(line.Input.Quantity))
		if line.Input.LineDiscount < 0 || line.Input.LineDiscount > line.LineSubtotal {
			return amountSummary{}, platform.NewError(http.StatusBadRequest, "ส่วนลดรายการไม่ถูกต้อง")
		}
		line.LineNet = platform.Round2(line.LineSubtotal - line.Input.LineDiscount)
		summary.Subtotal = platform.Round2(summary.Subtotal + line.LineSubtotal)
		summary.LineDiscount = platform.Round2(summary.LineDiscount + line.Input.LineDiscount)
		if mode == "exclusive" {
			line.LineTax = platform.Round2(line.LineNet * rate / 100)
			line.LineTotal = platform.Round2(line.LineNet + line.LineTax)
		} else if mode == "inclusive" {
			line.LineTax = platform.Round2(line.LineNet * rate / (100 + rate))
			line.LineTotal = line.LineNet
		} else {
			line.LineTotal = line.LineNet
		}
	}
	base := platform.Round2(summary.Subtotal - summary.LineDiscount)
	if summary.HeaderDiscount > base {
		return amountSummary{}, platform.NewError(http.StatusBadRequest, "ส่วนลดท้ายบิลมากกว่ายอดสินค้า")
	}
	taxable := platform.Round2(base - summary.HeaderDiscount + summary.Shipping)
	switch mode {
	case "exclusive":
		summary.Tax = platform.Round2(taxable * rate / 100)
		summary.Total = platform.Round2(taxable + summary.Tax)
	case "inclusive":
		summary.Tax = platform.Round2(taxable * rate / (100 + rate))
		summary.Total = taxable
	default:
		summary.Total = taxable
	}
	return summary, nil
}

func supplierAddress(parts ...string) string {
	values := []string{}
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return strings.Join(values, " ")
}

func (s *Service) nextPONumber(ctx context.Context, tx *sql.Tx, branchID string, purchasedAt time.Time) (string, error) {
	var branchCode string
	if err := tx.QueryRowContext(ctx, `SELECT code FROM branches WHERE id=$1`, branchID).Scan(&branchCode); err != nil {
		if err == sql.ErrNoRows {
			return "", platform.NewError(http.StatusNotFound, "ไม่พบสาขารับสินค้า")
		}
		return "", err
	}
	var prefix string
	var next int64
	if err := tx.QueryRowContext(ctx, `
		SELECT prefix,next_number FROM document_sequences
		WHERE branch_id=$1 AND doc_type='purchase_order' FOR UPDATE
	`, branchID).Scan(&prefix, &next); err != nil {
		if err != sql.ErrNoRows {
			return "", err
		}
		prefix, next = "PO", 1
		if _, err := tx.ExecContext(ctx, `INSERT INTO document_sequences(id,branch_id,doc_type,prefix,next_number,is_locked,created_at,updated_at) VALUES($1,$2,'purchase_order',$3,1,FALSE,NOW(),NOW())`, platform.MustUUID(), branchID, prefix); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE document_sequences SET next_number=$2,updated_at=NOW() WHERE branch_id=$1 AND doc_type='purchase_order'`, branchID, next+1); err != nil {
		return "", err
	}
	return platform.FormatBranchDocumentNumber(branchCode, prefix, purchasedAt, next), nil
}

func (s *Service) resolvePurchaseLine(ctx context.Context, tx *sql.Tx, purchasedAt time.Time, line PurchaseOrderLineInput) (resolvedPurchaseLine, error) {
	if line.Quantity <= 0 || line.UnitCost < 0 {
		return resolvedPurchaseLine{}, platform.NewError(http.StatusBadRequest, "จำนวนต้องมากกว่าศูนย์และราคาซื้อต้องไม่ติดลบ")
	}
	if err := stocklot.ValidateBucket(line.StockBucket); err != nil {
		return resolvedPurchaseLine{}, err
	}
	resolved := resolvedPurchaseLine{Input: line, ID: platform.MustUUID()}
	if line.NewProduct != nil {
		product := line.NewProduct
		if strings.TrimSpace(product.Name) == "" || strings.TrimSpace(product.UnitName) == "" {
			return resolvedPurchaseLine{}, platform.NewError(http.StatusBadRequest, "สินค้าใหม่ต้องมีชื่อและหน่วยนับ")
		}
		if product.BaseSellingPrice < 0 || product.MaxDiscountAmount < 0 || product.LowStockRealThreshold < 0 || product.LowStockGhostThreshold < 0 {
			return resolvedPurchaseLine{}, platform.NewError(http.StatusBadRequest, "ราคา ส่วนลด และจุดเตือนสินค้าใหม่ต้องไม่ติดลบ")
		}
		warningDays := product.ExpiryWarningDays
		if warningDays == 0 {
			warningDays = 30
		}
		if warningDays < 0 || warningDays > 3650 {
			return resolvedPurchaseLine{}, platform.NewError(http.StatusBadRequest, "จำนวนวันเตือนหมดอายุไม่ถูกต้อง")
		}
		resolved.ProductID = platform.MustUUID()
		resolved.SKU = strings.ToUpper(strings.TrimSpace(product.SKU))
		if resolved.SKU == "" {
			resolved.SKU = platform.GenerateReadableCode("PRD")
		}
		resolved.Name = strings.TrimSpace(product.Name)
		resolved.UnitName = strings.TrimSpace(product.UnitName)
		resolved.TracksExpiry = product.TracksExpiry
		_, err := tx.ExecContext(ctx, `
			INSERT INTO products (
				id,sku,barcode,category_id,name,description,cost_price,base_selling_price,
				unit_name,tax_exempt,active,max_discount_amount,
				low_stock_real_threshold,low_stock_ghost_threshold,tracks_expiry,
				expiry_warning_days,created_at,updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,TRUE,$11,$12,$13,$14,$15,NOW(),NOW())
		`, resolved.ProductID, resolved.SKU, platform.NullString(product.Barcode), platform.NullUUID(product.CategoryID),
			resolved.Name, strings.TrimSpace(product.Description), platform.Round2(line.UnitCost), platform.Round2(product.BaseSellingPrice),
			resolved.UnitName, product.TaxExempt, platform.Round2(product.MaxDiscountAmount),
			product.LowStockRealThreshold, product.LowStockGhostThreshold, product.TracksExpiry, warningDays)
		if err != nil {
			return resolvedPurchaseLine{}, platform.MapUniqueViolation(err, "SKU หรือบาร์โค้ดสินค้าใหม่นี้มีอยู่แล้ว")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory(id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at)
			SELECT $1,b.id,$2,0,0,NOW(),NOW()
			FROM branches b
			WHERE b.branch_type='main_warehouse' AND b.active=TRUE
			ON CONFLICT(branch_id,product_id) DO NOTHING
		`, platform.MustUUID(), resolved.ProductID); err != nil {
			return resolvedPurchaseLine{}, fmt.Errorf("เพิ่มสินค้าใหม่เข้า catalog โกดัง: %w", err)
		}
	} else {
		if strings.TrimSpace(line.ProductID) == "" {
			return resolvedPurchaseLine{}, platform.NewError(http.StatusBadRequest, "กรุณาเลือกสินค้าหรือกรอกสินค้าใหม่")
		}
		if err := tx.QueryRowContext(ctx, `SELECT id::text,sku,name,unit_name,tracks_expiry FROM products WHERE id=$1 AND active=TRUE`, line.ProductID).Scan(&resolved.ProductID, &resolved.SKU, &resolved.Name, &resolved.UnitName, &resolved.TracksExpiry); err != nil {
			if err == sql.ErrNoRows {
				return resolvedPurchaseLine{}, platform.NewError(http.StatusNotFound, "product not found")
			}
			return resolvedPurchaseLine{}, err
		}
	}
	expires, err := parseExpiry(line.ExpiresOn)
	if err != nil {
		return resolvedPurchaseLine{}, err
	}
	if resolved.TracksExpiry && !expires.Valid {
		return resolvedPurchaseLine{}, platform.NewError(http.StatusBadRequest, fmt.Sprintf("กรุณาระบุวันหมดอายุของ %s", resolved.Name))
	}
	if expires.Valid {
		purchaseDate := platform.InBangkok(purchasedAt)
		if !expires.Time.After(time.Date(purchaseDate.Year(), purchaseDate.Month(), purchaseDate.Day(), 0, 0, 0, 0, purchaseDate.Location())) {
			return resolvedPurchaseLine{}, platform.NewError(http.StatusBadRequest, "วันหมดอายุต้องอยู่หลังวันที่ซื้อ")
		}
	}
	resolved.ExpiresOn = expires
	return resolved, nil
}

func (s *Service) CreatePurchaseOrder(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input PurchaseOrderInput) (string, error) {
	if strings.TrimSpace(input.BranchID) == "" || strings.TrimSpace(input.SupplierID) == "" {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเลือกสาขารับสินค้าและบริษัทคู่ค้า")
	}
	if len(input.Items) == 0 || len(input.Items) > 100 {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเพิ่มสินค้า 1–100 รายการ")
	}
	if user.RoleKey != "super_admin" {
		for _, line := range input.Items {
			if line.StockBucket == "ghost" {
				return "", platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้นที่จัดสรรสต๊อกผีได้")
			}
			if line.NewProduct != nil && line.NewProduct.LowStockGhostThreshold != 0 {
				return "", platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้นที่กำหนดค่าสต๊อกผีได้")
			}
		}
	}
	purchasedAt, err := parsePurchasedAt(input.PurchasedAt)
	if err != nil {
		return "", err
	}
	dueDate, err := parseExpiry(input.DueDate)
	if err != nil {
		return "", platform.NewError(http.StatusBadRequest, "วันครบกำหนดไม่ถูกต้อง")
	}
	vatMode, vatRate, err := normalizeVAT(input.VATMode, input.VATRate)
	if err != nil {
		return "", err
	}
	poID := platform.MustUUID()
	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var branchExists, branchIsWarehouse bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS(SELECT 1 FROM branches WHERE id=$1 AND active=TRUE),
			       EXISTS(SELECT 1 FROM branches WHERE id=$1 AND active=TRUE AND branch_type='main_warehouse')
		`, input.BranchID).Scan(&branchExists, &branchIsWarehouse); err != nil {
			return err
		}
		if !branchExists {
			return platform.NewError(http.StatusBadRequest, "ไม่พบสาขารับสินค้าที่เปิดใช้งาน")
		}
		for _, line := range input.Items {
			if line.StockBucket == "ghost" && !branchIsWarehouse {
				return platform.NewError(http.StatusBadRequest, "สั่งซื้อเข้าสต๊อกผีได้เฉพาะโกดัง WH")
			}
		}
		var supplierCode, supplierName, supplierTaxID, addressLine, subdistrict, district, province, postalCode, contact, phone, email string
		if err := tx.QueryRowContext(ctx, `
			SELECT supplier_code,legal_name,COALESCE(tax_id,''),address_line,subdistrict,district,province,postal_code,contact_name,phone,email
			FROM suppliers WHERE id=$1 AND active=TRUE
		`, input.SupplierID).Scan(&supplierCode, &supplierName, &supplierTaxID, &addressLine, &subdistrict, &district, &province, &postalCode, &contact, &phone, &email); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusBadRequest, "ไม่พบบริษัทคู่ค้าที่เปิดใช้งาน")
			}
			return err
		}
		address := strings.TrimSpace(input.SupplierAddressSnapshot)
		if address == "" {
			address = supplierAddress(addressLine, subdistrict, district, province, postalCode)
		}
		poNumber, err := s.nextPONumber(ctx, tx, input.BranchID, purchasedAt)
		if err != nil {
			return err
		}
		lines := make([]resolvedPurchaseLine, 0, len(input.Items))
		for _, item := range input.Items {
			resolved, err := s.resolvePurchaseLine(ctx, tx, purchasedAt, item)
			if err != nil {
				return err
			}
			lines = append(lines, resolved)
		}
		summary, err := calculatePurchaseAmounts(lines, vatMode, vatRate, input.HeaderDiscount, input.ShippingAmount)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_orders (
				id,branch_id,supplier_id,po_number,status,revision,purchased_at,due_date,posted_at,
				supplier_document_number,supplier_code_snapshot,supplier_name_snapshot,
				supplier_tax_id_snapshot,supplier_address_snapshot,supplier_contact_snapshot,
				supplier_phone_snapshot,supplier_email_snapshot,job_name,delivery_terms,
				vat_mode,vat_rate,subtotal,line_discount_total,header_discount,shipping_amount,
				tax_amount,total_amount,notes,created_by,updated_by,created_at,updated_at
			) VALUES ($1,$2,$3,$4,'posted',1,$5,$6,NOW(),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$26,NOW(),NOW())
		`, poID, input.BranchID, input.SupplierID, poNumber, purchasedAt, dueDate, strings.TrimSpace(input.SupplierDocumentNumber), supplierCode, supplierName,
			supplierTaxID, address, contact, phone, email, strings.TrimSpace(input.JobName), strings.TrimSpace(input.DeliveryTerms),
			summary.VATMode, summary.VATRate, summary.Subtotal, summary.LineDiscount,
			summary.HeaderDiscount, summary.Shipping, summary.Tax, summary.Total, strings.TrimSpace(input.Notes), user.ID); err != nil {
			return err
		}
		for index := range lines {
			line := &lines[index]
			lotNumber := strings.TrimSpace(line.Input.LotNumber)
			if lotNumber == "" {
				lotNumber = fmt.Sprintf("%s-%02d", poNumber, index+1)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO purchase_order_items (
					id,purchase_order_id,product_id,product_sku_snapshot,product_name_snapshot,
					unit_name_snapshot,stock_bucket,received_quantity,unit_cost,line_discount,
					line_subtotal,tax_amount,line_total,lot_number,expires_on,created_at,updated_at
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,NOW(),NOW())
			`, line.ID, poID, line.ProductID, line.SKU, line.Name, line.UnitName, line.Input.StockBucket,
				line.Input.Quantity, platform.Round2(line.Input.UnitCost), platform.Round2(line.Input.LineDiscount), line.LineSubtotal,
				line.LineTax, line.LineTotal, lotNumber, line.ExpiresOn); err != nil {
				return err
			}
			lotID, err := stocklot.Create(ctx, tx, stocklot.Lot{
				BranchID: input.BranchID, ProductID: line.ProductID, StockBucket: line.Input.StockBucket,
				LotNumber: lotNumber, ExpiresOn: line.ExpiresOn, ReceivedQuantity: line.Input.Quantity,
				RemainingQuantity: line.Input.Quantity, UnitCost: line.Input.UnitCost, ReceivedAt: purchasedAt,
			}, "purchase_order", &poID, &line.ID, nil)
			if err != nil {
				return err
			}
			line.InventoryLotID = lotID
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory(id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at)
				VALUES($1,$2,$3,0,0,NOW(),NOW()) ON CONFLICT(branch_id,product_id) DO NOTHING
			`, platform.MustUUID(), input.BranchID, line.ProductID); err != nil {
				return err
			}
			column := "qty_real"
			if line.Input.StockBucket == "ghost" {
				column = "qty_ghost"
			}
			if _, err := tx.ExecContext(ctx, `UPDATE inventory SET `+column+`=`+column+`+$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, input.BranchID, line.ProductID, line.Input.Quantity); err != nil {
				return err
			}
			movementID := platform.MustUUID()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_movements(id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at)
				VALUES($1,$2,$3,'purchase_receive',$4,$5,'purchase_order',$6,$7,$8,NOW())
			`, movementID, input.BranchID, line.ProductID, line.Input.StockBucket, line.Input.Quantity, poID, poNumber, user.ID); err != nil {
				return err
			}
			if err := stocklot.AttachMovement(ctx, tx, movementID, []stocklot.Allocation{{Lot: stocklot.Lot{ID: lotID}, Quantity: line.Input.Quantity}}, 1); err != nil {
				return err
			}
			if err := recomputeLatestProductCost(ctx, tx, line.ProductID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO purchase_order_events(id,purchase_order_id,event_type,revision,note,actor_id,event_at) VALUES($1,$2,'posted',1,$3,$4,NOW())`, platform.MustUUID(), poID, strings.TrimSpace(input.Notes), user.ID); err != nil {
			return err
		}
		meta.EntityType = "purchase_order"
		meta.EntityID = &poID
		meta.Action = "purchase_order.post"
		meta.After = map[string]any{"po_number": poNumber, "branch_id": input.BranchID, "supplier_id": input.SupplierID, "total_amount": summary.Total, "item_count": len(lines)}
		return s.audit.Log(ctx, tx, meta)
	})
	return poID, err
}

func recomputeLatestProductCost(ctx context.Context, tx *sql.Tx, productID string) error {
	var cost float64
	err := tx.QueryRowContext(ctx, `
		SELECT poi.unit_cost
		FROM purchase_order_items poi
		INNER JOIN purchase_orders po ON po.id=poi.purchase_order_id AND po.status='posted'
		WHERE poi.product_id=$1
		ORDER BY po.purchased_at DESC,po.created_at DESC,poi.created_at DESC
		LIMIT 1
	`, productID).Scan(&cost)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `SELECT unit_cost FROM inventory_lots WHERE product_id=$1 AND remaining_quantity>0 ORDER BY received_at DESC,id DESC LIMIT 1`, productID).Scan(&cost)
	}
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE products SET cost_price=$2,updated_at=NOW() WHERE id=$1`, productID, platform.Round2(cost))
	return err
}

type PurchaseOrderFilter struct {
	Search, SupplierID, BranchID, Status, DateFrom, DateTo string
	Page, PageSize                                         int
}

func (s *Service) ListPurchaseOrders(ctx context.Context, user platform.AuthUser, filter PurchaseOrderFilter) (map[string]any, error) {
	args := []any{}
	conditions := []string{}
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}
	if value := strings.TrimSpace(filter.Search); value != "" {
		add("(LOWER(po.po_number) LIKE $%[1]d OR LOWER(po.supplier_name_snapshot) LIKE $%[1]d OR LOWER(po.supplier_document_number) LIKE $%[1]d)", "%"+strings.ToLower(value)+"%")
	}
	if filter.SupplierID != "" {
		add("po.supplier_id = $%d", filter.SupplierID)
	}
	if filter.BranchID != "" {
		add("po.branch_id = $%d", filter.BranchID)
	}
	if filter.Status == "posted" || filter.Status == "cancelled" {
		add("po.status = $%d", filter.Status)
	}
	if filter.DateFrom != "" {
		add("po.purchased_at >= $%d::date", filter.DateFrom)
	}
	if filter.DateTo != "" {
		add("po.purchased_at < ($%d::date + INTERVAL '1 day')", filter.DateTo)
	}
	lineVisibility := ""
	if user.RoleKey != "super_admin" {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM purchase_order_items visible_poi WHERE visible_poi.purchase_order_id=po.id AND visible_poi.stock_bucket='real')")
		lineVisibility = " AND poi.stock_bucket='real'"
	}
	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM purchase_orders po `+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT po.id::text,po.po_number,po.status,po.revision,po.purchased_at,po.posted_at,
		       po.supplier_id::text,po.supplier_name_snapshot,po.supplier_document_number,
		       po.branch_id::text,b.name,po.vat_mode,po.vat_rate,po.total_amount,COALESCE(SUM(poi.line_total),0),
		       COUNT(poi.id)::bigint,COALESCE(SUM(poi.received_quantity),0)::bigint,
		       u.full_name,po.updated_at
		FROM purchase_orders po INNER JOIN branches b ON b.id=po.branch_id
		INNER JOIN users u ON u.id=po.created_by LEFT JOIN purchase_order_items poi ON poi.purchase_order_id=po.id`+lineVisibility+`
		`+where+` GROUP BY po.id,b.id,u.id ORDER BY po.purchased_at DESC,po.id DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, status, supplierID, supplierName, supplierDoc, branchID, branchName, vatMode, createdBy string
		var revision int
		var vatRate, totalAmount, visibleTotal float64
		var itemCount, totalQty int64
		var purchasedAt, postedAt, updatedAt time.Time
		if err := rows.Scan(&id, &number, &status, &revision, &purchasedAt, &postedAt, &supplierID, &supplierName, &supplierDoc, &branchID, &branchName, &vatMode, &vatRate, &totalAmount, &visibleTotal, &itemCount, &totalQty, &createdBy, &updatedAt); err != nil {
			return nil, err
		}
		if user.RoleKey != "super_admin" {
			totalAmount = visibleTotal
		}
		items = append(items, map[string]any{"id": id, "po_number": number, "status": status, "revision": revision, "purchased_at": purchasedAt, "posted_at": postedAt, "supplier_id": supplierID, "supplier_name": supplierName, "supplier_document_number": supplierDoc, "branch_id": branchID, "branch_name": branchName, "vat_mode": vatMode, "vat_rate": vatRate, "total_amount": totalAmount, "item_count": itemCount, "total_quantity": totalQty, "created_by_name": createdBy, "updated_at": updatedAt})
	}
	pages := (total + pageSize - 1) / pageSize
	if pages < 1 {
		pages = 1
	}
	return map[string]any{"items": items, "pagination": map[string]any{"page": page, "page_size": pageSize, "total": total, "total_pages": pages}}, rows.Err()
}

func (s *Service) GetPurchaseOrder(ctx context.Context, user platform.AuthUser, id string) (map[string]any, error) {
	item := map[string]any{}
	var poID, number, status, branchID, branchName, branchAddress, branchTaxID, supplierID, supplierCode, supplierName, taxID, address, contact, phone, email, supplierDoc, jobName, deliveryTerms, vatMode, notes, correctionReason, createdBy, updatedBy string
	var revision int
	var vatRate, subtotal, lineDiscount, headerDiscount, shipping, tax, total float64
	var purchasedAt, postedAt, createdAt, updatedAt time.Time
	var dueDate, cancelledAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT po.id::text,po.po_number,po.status,po.revision,po.branch_id::text,b.name,b.address,COALESCE(b.tax_id,''),
		po.supplier_id::text,po.supplier_code_snapshot,po.supplier_name_snapshot,po.supplier_tax_id_snapshot,
		po.supplier_address_snapshot,po.supplier_contact_snapshot,po.supplier_phone_snapshot,po.supplier_email_snapshot,
		po.supplier_document_number,po.job_name,po.delivery_terms,
		po.purchased_at,po.due_date,po.posted_at,po.vat_mode,po.vat_rate,po.subtotal,po.line_discount_total,
		po.header_discount,po.shipping_amount,po.tax_amount,po.total_amount,po.notes,po.correction_reason,
		cu.full_name,uu.full_name,po.cancelled_at,po.created_at,po.updated_at
		FROM purchase_orders po INNER JOIN branches b ON b.id=po.branch_id
		INNER JOIN users cu ON cu.id=po.created_by INNER JOIN users uu ON uu.id=po.updated_by WHERE po.id=$1
	`, id).Scan(&poID, &number, &status, &revision, &branchID, &branchName, &branchAddress, &branchTaxID, &supplierID, &supplierCode, &supplierName, &taxID, &address, &contact, &phone, &email, &supplierDoc, &jobName, &deliveryTerms, &purchasedAt, &dueDate, &postedAt, &vatMode, &vatRate, &subtotal, &lineDiscount, &headerDiscount, &shipping, &tax, &total, &notes, &correctionReason, &createdBy, &updatedBy, &cancelledAt, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบใบสั่งซื้อเข้า")
		}
		return nil, err
	}
	item = map[string]any{"id": poID, "po_number": number, "status": status, "revision": revision, "branch_id": branchID, "branch_name": branchName, "branch_address": branchAddress, "branch_tax_id": branchTaxID, "supplier_id": supplierID, "supplier_code": supplierCode, "supplier_name": supplierName, "supplier_tax_id": taxID, "supplier_address": address, "supplier_contact": contact, "supplier_phone": phone, "supplier_email": email, "supplier_document_number": supplierDoc, "job_name": jobName, "delivery_terms": deliveryTerms, "purchased_at": purchasedAt, "due_date": nullableTime(dueDate), "posted_at": postedAt, "vat_mode": vatMode, "vat_rate": vatRate, "subtotal": subtotal, "line_discount_total": lineDiscount, "header_discount": headerDiscount, "shipping_amount": shipping, "tax_amount": tax, "total_amount": total, "notes": notes, "correction_reason": correctionReason, "created_by_name": createdBy, "updated_by_name": updatedBy, "cancelled_at": nullableTime(cancelledAt), "created_at": createdAt, "updated_at": updatedAt}
	rows, err := s.db.QueryContext(ctx, `
		SELECT poi.id::text,poi.product_id::text,poi.product_sku_snapshot,poi.product_name_snapshot,
		poi.unit_name_snapshot,poi.stock_bucket,poi.received_quantity,poi.unit_cost,poi.line_discount,
		poi.line_subtotal,poi.tax_amount,poi.line_total,poi.lot_number,poi.expires_on,
		COALESCE(il.id::text,''),COALESCE(il.remaining_quantity,0),COALESCE(il.unit_cost,poi.unit_cost)
		FROM purchase_order_items poi LEFT JOIN inventory_lots il ON il.source_item_id=poi.id
		WHERE poi.purchase_order_id=$1 ORDER BY poi.created_at,poi.id
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []map[string]any{}
	var visibleSubtotal, visibleDiscount, visibleTax, visibleTotal float64
	hadHiddenLine := false
	for rows.Next() {
		var itemID, productID, sku, name, unit, bucket, lotNumber, lotID string
		var qty, remaining int
		var unitCost, discount, lineSubtotal, lineTax, lineTotal, lotCost float64
		var expiry sql.NullTime
		if err := rows.Scan(&itemID, &productID, &sku, &name, &unit, &bucket, &qty, &unitCost, &discount, &lineSubtotal, &lineTax, &lineTotal, &lotNumber, &expiry, &lotID, &remaining, &lotCost); err != nil {
			return nil, err
		}
		if user.RoleKey != "super_admin" && bucket == "ghost" {
			hadHiddenLine = true
			continue
		}
		visibleSubtotal = platform.Round2(visibleSubtotal + lineSubtotal)
		visibleDiscount = platform.Round2(visibleDiscount + discount)
		visibleTax = platform.Round2(visibleTax + lineTax)
		visibleTotal = platform.Round2(visibleTotal + lineTotal)
		line := map[string]any{"id": itemID, "product_id": productID, "sku": sku, "product_name": name, "unit_name": unit, "received_quantity": qty, "remaining_quantity": remaining, "unit_cost": unitCost, "line_discount": discount, "line_subtotal": lineSubtotal, "tax_amount": lineTax, "line_total": lineTotal, "lot_number": lotNumber, "expires_on": nullableTime(expiry), "inventory_lot_id": lotID, "lot_unit_cost": lotCost}
		if user.RoleKey == "super_admin" {
			line["stock_bucket"] = bucket
		}
		lines = append(lines, line)
	}
	if user.RoleKey != "super_admin" {
		if len(lines) == 0 && hadHiddenLine {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบใบสั่งซื้อเข้า")
		}
		item["subtotal"] = visibleSubtotal
		item["line_discount_total"] = visibleDiscount
		item["header_discount"] = 0
		item["shipping_amount"] = 0
		item["tax_amount"] = visibleTax
		item["total_amount"] = visibleTotal
		item["items"] = lines
		item["events"] = []map[string]any{}
		delete(item, "correction_reason")
		return item, rows.Err()
	}
	eventRows, err := s.db.QueryContext(ctx, `SELECT pe.id::text,pe.event_type,pe.revision,pe.note,u.full_name,pe.event_at FROM purchase_order_events pe INNER JOIN users u ON u.id=pe.actor_id WHERE pe.purchase_order_id=$1 ORDER BY pe.event_at DESC,pe.id DESC`, id)
	if err != nil {
		return nil, err
	}
	defer eventRows.Close()
	events := []map[string]any{}
	for eventRows.Next() {
		var eventID, eventType, note, actor string
		var rev int
		var at time.Time
		if err := eventRows.Scan(&eventID, &eventType, &rev, &note, &actor, &at); err != nil {
			return nil, err
		}
		events = append(events, map[string]any{"id": eventID, "event_type": eventType, "revision": rev, "note": note, "actor_name": actor, "event_at": at})
	}
	item["items"] = lines
	item["events"] = events
	return item, nil
}

func (s *Service) ProductOptions(ctx context.Context, user platform.AuthUser, branchID, bucket, query, cursor string, limit int) (CursorResult, error) {
	if strings.TrimSpace(branchID) == "" {
		return CursorResult{}, platform.NewError(http.StatusBadRequest, "branch_id is required")
	}
	if err := stocklot.ValidateBucket(bucket); err != nil {
		return CursorResult{}, err
	}
	if bucket == "ghost" && user.RoleKey != "super_admin" {
		return CursorResult{}, platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์เข้าถึงสต๊อกผี")
	}
	if bucket == "ghost" {
		var isWarehouse bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM branches WHERE id=$1 AND active=TRUE AND branch_type='main_warehouse')`, branchID).Scan(&isWarehouse); err != nil {
			return CursorResult{}, err
		}
		if !isWarehouse {
			return CursorResult{}, platform.NewError(http.StatusBadRequest, "สต๊อกผีมีเฉพาะโกดัง WH")
		}
	}
	limit = normalizeLimit(limit)
	offset := decodeOffset(cursor)
	args := []any{branchID, bucket}
	where := ""
	if keyword := strings.TrimSpace(query); keyword != "" {
		args = append(args, "%"+strings.ToLower(keyword)+"%")
		where = `WHERE p.active=TRUE AND (LOWER(p.name) LIKE $3 OR LOWER(p.sku) LIKE $3 OR LOWER(COALESCE(p.barcode,'')) LIKE $3 OR EXISTS (SELECT 1 FROM product_source_aliases psa WHERE psa.product_id=p.id AND LOWER(psa.alias_name) LIKE $3))`
	} else {
		where = `WHERE p.active=TRUE AND COALESCE(ls.sellable_quantity,0)=0`
	}
	args = append(args, limit+1, offset)
	rows, err := s.db.QueryContext(ctx, `
		WITH lot_summary AS (
		 SELECT product_id,SUM(remaining_quantity) FILTER(WHERE expires_on IS NULL OR expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)::bigint AS sellable_quantity,
		 SUM(remaining_quantity) FILTER(WHERE expires_on < (NOW() AT TIME ZONE 'Asia/Bangkok')::date)::bigint AS expired_quantity,
		 MIN(expires_on) FILTER(WHERE remaining_quantity>0 AND expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date) AS nearest_expiry
		 FROM inventory_lots WHERE branch_id=$1 AND stock_bucket=$2 AND remaining_quantity>0 GROUP BY product_id
		)
		SELECT p.id::text,p.sku,COALESCE(p.barcode,''),p.name,p.unit_name,p.cost_price,p.base_selling_price,
		COALESCE(bpp.selling_price,p.base_selling_price),COALESCE(bps.max_discount_amount,p.max_discount_amount),
		CASE WHEN $2='real' THEN COALESCE(bps.low_stock_real_threshold,p.low_stock_real_threshold) ELSE COALESCE(bps.low_stock_ghost_threshold,p.low_stock_ghost_threshold) END,
		p.tracks_expiry,p.expiry_warning_days,COALESCE(ls.sellable_quantity,0),COALESCE(ls.expired_quantity,0),ls.nearest_expiry,
		COALESCE(pc.name,'ไม่มีหมวดหมู่'),COALESCE(p.image_storage_key,''),
		(SELECT COUNT(*) FROM product_images pi WHERE pi.product_id=p.id)
		FROM products p
		INNER JOIN inventory warehouse_inventory ON warehouse_inventory.product_id=p.id
		INNER JOIN branches warehouse ON warehouse.id=warehouse_inventory.branch_id
		 AND warehouse.branch_type='main_warehouse' AND warehouse.active=TRUE
		LEFT JOIN branch_product_prices bpp ON bpp.product_id=p.id AND bpp.branch_id=$1
		LEFT JOIN branch_product_settings bps ON bps.product_id=p.id AND bps.branch_id=$1 LEFT JOIN product_categories pc ON pc.id=p.category_id
		LEFT JOIN lot_summary ls ON ls.product_id=p.id `+where+` ORDER BY p.name,p.id LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return CursorResult{}, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, sku, barcode, name, unit, category string
		var cost, base, effective, maxDiscount float64
		var threshold, warning, sellable, expired, imageCount int
		var imageKey string
		var tracks bool
		var expiry sql.NullTime
		if err := rows.Scan(&id, &sku, &barcode, &name, &unit, &cost, &base, &effective, &maxDiscount, &threshold, &tracks, &warning, &sellable, &expired, &expiry, &category, &imageKey, &imageCount); err != nil {
			return CursorResult{}, err
		}
		items = append(items, map[string]any{"id": id, "sku": sku, "barcode": barcode, "name": name, "unit_name": unit, "cost_price": cost, "base_selling_price": base, "effective_price": effective, "max_discount_amount": maxDiscount, "low_stock_threshold": threshold, "tracks_expiry": tracks, "expiry_warning_days": warning, "sellable_quantity": sellable, "expired_quantity": expired, "nearest_expiry": nullableTime(expiry), "category_name": category, "stock_bucket": bucket, "image_available": imageKey != "", "image_count": imageCount})
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	result := CursorResult{Items: items, HasMore: hasMore}
	if hasMore {
		result.NextCursor = encodeOffset(offset + limit)
	}
	return result, rows.Err()
}

func nullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}

func (s *Service) UpdatePurchaseOrder(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id string, input PurchaseOrderInput) error {
	if user.RoleKey != "super_admin" {
		for _, line := range input.Items {
			if line.StockBucket == "ghost" {
				return platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้นที่แก้สต๊อกผีได้")
			}
		}
	}
	reason := strings.TrimSpace(input.CorrectionReason)
	if reason == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณาระบุเหตุผลการแก้ไข")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status, branchID, vatMode string
		var branchIsWarehouse bool
		var revision int
		var vatRate float64
		if err := tx.QueryRowContext(ctx, `
			SELECT po.status,po.branch_id::text,po.revision,po.vat_mode,po.vat_rate,
			       branch.branch_type='main_warehouse' AND branch.active=TRUE
			FROM purchase_orders po INNER JOIN branches branch ON branch.id=po.branch_id
			WHERE po.id=$1 FOR UPDATE OF po
		`, id).Scan(&status, &branchID, &revision, &vatMode, &vatRate, &branchIsWarehouse); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบใบสั่งซื้อเข้า")
			}
			return err
		}
		if status != "posted" {
			return platform.NewError(http.StatusConflict, "ใบสั่งซื้อที่ยกเลิกแล้วแก้ไขไม่ได้")
		}
		if input.VATMode != "" {
			var err error
			vatMode, vatRate, err = normalizeVAT(input.VATMode, input.VATRate)
			if err != nil {
				return err
			}
		}
		for _, change := range input.Items {
			if strings.TrimSpace(change.ID) == "" {
				return platform.NewError(http.StatusBadRequest, "ไม่พบรหัสรายการที่ต้องการแก้ไข")
			}
			var productID, bucket, lotNumber string
			var oldQty, remaining int
			var oldCost, oldDiscount float64
			var tracks bool
			var lotID string
			var oldExpiry sql.NullTime
			if err := tx.QueryRowContext(ctx, `SELECT poi.product_id::text,poi.stock_bucket,poi.received_quantity,poi.unit_cost,poi.line_discount,poi.lot_number,poi.expires_on,p.tracks_expiry,il.id::text,il.remaining_quantity FROM purchase_order_items poi INNER JOIN products p ON p.id=poi.product_id INNER JOIN inventory_lots il ON il.source_item_id=poi.id WHERE poi.id=$1 AND poi.purchase_order_id=$2 FOR UPDATE OF poi,il`, change.ID, id).Scan(&productID, &bucket, &oldQty, &oldCost, &oldDiscount, &lotNumber, &oldExpiry, &tracks, &lotID, &remaining); err != nil {
				if err == sql.ErrNoRows {
					return platform.NewError(http.StatusNotFound, "ไม่พบรายการสินค้าในใบสั่งซื้อ")
				}
				return err
			}
			if bucket == "ghost" && user.RoleKey != "super_admin" {
				return platform.NewError(http.StatusForbidden, "รายการนี้เป็นสต๊อกผีและแก้ไขได้เฉพาะผู้ดูแลระบบสูงสุด")
			}
			newQty := change.Quantity
			if newQty == 0 {
				newQty = oldQty
			}
			if newQty <= 0 {
				return platform.NewError(http.StatusBadRequest, "จำนวนต้องมากกว่าศูนย์")
			}
			consumed := oldQty - remaining
			if newQty < consumed {
				return platform.NewError(http.StatusConflict, fmt.Sprintf("ลดจำนวนต่ำกว่า %d ไม่ได้ เพราะสินค้าถูกขายหรือโอนไปแล้ว", consumed))
			}
			targetProductID := productID
			if strings.TrimSpace(change.ProductID) != "" {
				targetProductID = strings.TrimSpace(change.ProductID)
			}
			targetBucket := bucket
			if strings.TrimSpace(change.StockBucket) != "" {
				targetBucket = strings.TrimSpace(change.StockBucket)
				if err := stocklot.ValidateBucket(targetBucket); err != nil {
					return err
				}
			}
			if targetBucket == "ghost" && !branchIsWarehouse {
				return platform.NewError(http.StatusBadRequest, "สต๊อกผีของใบสั่งซื้ออยู่ได้เฉพาะโกดัง WH")
			}
			var targetSKU, targetName, targetUnit string
			var targetTracks bool
			if err := tx.QueryRowContext(ctx, `SELECT sku,name,unit_name,tracks_expiry FROM products WHERE id=$1 AND active=TRUE`, targetProductID).Scan(&targetSKU, &targetName, &targetUnit, &targetTracks); err != nil {
				if err == sql.ErrNoRows {
					return platform.NewError(http.StatusNotFound, "ไม่พบสินค้าที่ต้องการเปลี่ยน")
				}
				return err
			}
			newCost := change.UnitCost
			if newCost == 0 && oldCost != 0 {
				newCost = oldCost
			}
			if newCost < 0 {
				return platform.NewError(http.StatusBadRequest, "ราคาซื้อต้องไม่ติดลบ")
			}
			newDiscount := change.LineDiscount
			if newDiscount == 0 && oldDiscount != 0 {
				newDiscount = oldDiscount
			}
			if newDiscount < 0 || newDiscount > newCost*float64(newQty) {
				return platform.NewError(http.StatusBadRequest, "ส่วนลดรายการไม่ถูกต้อง")
			}
			newExpiry := oldExpiry
			if strings.TrimSpace(change.ExpiresOn) != "" {
				parsed, err := parseExpiry(change.ExpiresOn)
				if err != nil {
					return err
				}
				newExpiry = parsed
			}
			newLot := strings.TrimSpace(change.LotNumber)
			if newLot == "" {
				newLot = lotNumber
			}
			productOrBucketChanged := targetProductID != productID || targetBucket != bucket
			lotIdentityChanged := (newExpiry.Valid != oldExpiry.Valid) || (newExpiry.Valid && !newExpiry.Time.Equal(oldExpiry.Time)) || newLot != lotNumber
			if consumed > 0 && (productOrBucketChanged || lotIdentityChanged) {
				return platform.NewError(http.StatusConflict, "เปลี่ยนสินค้า ประเภทสต๊อก lot หรือวันหมดอายุไม่ได้หลังสินค้าเริ่มถูกใช้")
			}
			tracks = targetTracks
			if tracks && !newExpiry.Valid {
				return platform.NewError(http.StatusBadRequest, "สินค้านี้ต้องระบุวันหมดอายุ")
			}
			delta := newQty - oldQty
			if productOrBucketChanged {
				oldColumn := "qty_real"
				if bucket == "ghost" {
					oldColumn = "qty_ghost"
				}
				result, err := tx.ExecContext(ctx, `UPDATE inventory SET `+oldColumn+`=`+oldColumn+`-$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2 AND `+oldColumn+` >= $3`, branchID, productID, oldQty)
				if err != nil {
					return err
				}
				if affected, _ := result.RowsAffected(); affected != 1 {
					return platform.NewError(http.StatusConflict, "ยอดสต๊อกเดิมไม่สอดคล้องกับ lot กรุณาตรวจสอบก่อนแก้ไข")
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO inventory(id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at) VALUES($1,$2,$3,0,0,NOW(),NOW()) ON CONFLICT(branch_id,product_id) DO NOTHING`, platform.MustUUID(), branchID, targetProductID); err != nil {
					return err
				}
				targetColumn := "qty_real"
				if targetBucket == "ghost" {
					targetColumn = "qty_ghost"
				}
				if _, err := tx.ExecContext(ctx, `UPDATE inventory SET `+targetColumn+`=`+targetColumn+`+$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, branchID, targetProductID, newQty); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE inventory_lots SET product_id=$2,stock_bucket=$3,received_quantity=$4,remaining_quantity=$4,lot_number=$5,expires_on=$6,unit_cost=$7,updated_at=NOW() WHERE id=$1`, lotID, targetProductID, targetBucket, newQty, newLot, newExpiry, platform.Round2(newCost)); err != nil {
					return err
				}
				for _, movement := range []struct {
					productID, stockBucket string
					quantity               int
				}{{productID, bucket, -oldQty}, {targetProductID, targetBucket, newQty}} {
					movementID := platform.MustUUID()
					if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_movements(id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at) VALUES($1,$2,$3,'purchase_correction',$4,$5,'purchase_order',$6,$7,$8,NOW())`, movementID, branchID, movement.productID, movement.stockBucket, movement.quantity, id, reason, user.ID); err != nil {
						return err
					}
					sign := 1
					if movement.quantity < 0 {
						sign = -1
					}
					if err := stocklot.AttachMovement(ctx, tx, movementID, []stocklot.Allocation{{Lot: stocklot.Lot{ID: lotID}, Quantity: absInt(movement.quantity)}}, sign); err != nil {
						return err
					}
				}
			} else if delta != 0 {
				column := "qty_real"
				if targetBucket == "ghost" {
					column = "qty_ghost"
				}
				if _, err := tx.ExecContext(ctx, `UPDATE inventory SET `+column+`=`+column+`+$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, branchID, productID, delta); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE inventory_lots SET received_quantity=$2,remaining_quantity=remaining_quantity+$3,lot_number=$4,expires_on=$5,unit_cost=$6,updated_at=NOW() WHERE id=$1`, lotID, newQty, delta, newLot, newExpiry, platform.Round2(newCost)); err != nil {
					return err
				}
				movementID := platform.MustUUID()
				if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_movements(id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at) VALUES($1,$2,$3,'purchase_correction',$4,$5,'purchase_order',$6,$7,$8,NOW())`, movementID, branchID, targetProductID, targetBucket, delta, id, reason, user.ID); err != nil {
					return err
				}
				sign := 1
				if delta < 0 {
					sign = -1
				}
				if err := stocklot.AttachMovement(ctx, tx, movementID, []stocklot.Allocation{{Lot: stocklot.Lot{ID: lotID}, Quantity: absInt(delta)}}, sign); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `UPDATE inventory_lots SET lot_number=$2,expires_on=$3,unit_cost=$4,updated_at=NOW() WHERE id=$1`, lotID, newLot, newExpiry, platform.Round2(newCost)); err != nil {
					return err
				}
			}
			lineSubtotal := platform.Round2(newCost * float64(newQty))
			lineNet := platform.Round2(lineSubtotal - newDiscount)
			lineTax := 0.0
			lineTotal := lineNet
			if vatMode == "exclusive" {
				lineTax = platform.Round2(lineNet * vatRate / 100)
				lineTotal = platform.Round2(lineNet + lineTax)
			} else if vatMode == "inclusive" {
				lineTax = platform.Round2(lineNet * vatRate / (100 + vatRate))
			}
			if _, err := tx.ExecContext(ctx, `UPDATE purchase_order_items SET product_id=$2,product_sku_snapshot=$3,product_name_snapshot=$4,unit_name_snapshot=$5,stock_bucket=$6,received_quantity=$7,unit_cost=$8,line_discount=$9,line_subtotal=$10,tax_amount=$11,line_total=$12,lot_number=$13,expires_on=$14,updated_at=NOW() WHERE id=$1`, change.ID, targetProductID, targetSKU, targetName, targetUnit, targetBucket, newQty, platform.Round2(newCost), platform.Round2(newDiscount), lineSubtotal, lineTax, lineTotal, newLot, newExpiry); err != nil {
				return err
			}
			if err := recomputeLatestProductCost(ctx, tx, productID); err != nil {
				return err
			}
			if targetProductID != productID {
				if err := recomputeLatestProductCost(ctx, tx, targetProductID); err != nil {
					return err
				}
			}
		}
		var subtotal, lineDiscount float64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(line_subtotal),0),COALESCE(SUM(line_discount),0) FROM purchase_order_items WHERE purchase_order_id=$1`, id).Scan(&subtotal, &lineDiscount); err != nil {
			return err
		}
		headerDiscount := input.HeaderDiscount
		shipping := input.ShippingAmount
		if headerDiscount < 0 || shipping < 0 || headerDiscount > subtotal-lineDiscount {
			return platform.NewError(http.StatusBadRequest, "ยอดส่วนลดหรือค่าขนส่งไม่ถูกต้อง")
		}
		taxable := platform.Round2(subtotal - lineDiscount - headerDiscount + shipping)
		tax := 0.0
		total := taxable
		if vatMode == "exclusive" {
			tax = platform.Round2(taxable * vatRate / 100)
			total = platform.Round2(taxable + tax)
		} else if vatMode == "inclusive" {
			tax = platform.Round2(taxable * vatRate / (100 + vatRate))
		}
		nextRevision := revision + 1
		if _, err := tx.ExecContext(ctx, `UPDATE purchase_orders SET vat_mode=$2,vat_rate=$3,subtotal=$4,line_discount_total=$5,header_discount=$6,shipping_amount=$7,tax_amount=$8,total_amount=$9,notes=CASE WHEN $10='' THEN notes ELSE $10 END,correction_reason=$11,revision=$12,updated_by=$13,updated_at=NOW() WHERE id=$1`, id, vatMode, vatRate, subtotal, lineDiscount, headerDiscount, shipping, tax, total, strings.TrimSpace(input.Notes), reason, nextRevision, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO purchase_order_events(id,purchase_order_id,event_type,revision,note,actor_id,event_at) VALUES($1,$2,'corrected',$3,$4,$5,NOW())`, platform.MustUUID(), id, nextRevision, reason, user.ID); err != nil {
			return err
		}
		meta.EntityType = "purchase_order"
		meta.EntityID = &id
		meta.Action = "purchase_order.correct"
		meta.After = map[string]any{"revision": nextRevision, "reason": reason, "total_amount": total}
		return s.audit.Log(ctx, tx, meta)
	})
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (s *Service) CancelPurchaseOrder(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณาระบุเหตุผลการยกเลิก")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status, branchID, number string
		var revision int
		if err := tx.QueryRowContext(ctx, `SELECT status,branch_id::text,po_number,revision FROM purchase_orders WHERE id=$1 FOR UPDATE`, id).Scan(&status, &branchID, &number, &revision); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบใบสั่งซื้อเข้า")
			}
			return err
		}
		if status != "posted" {
			return platform.NewError(http.StatusConflict, "ใบสั่งซื้อนี้ยกเลิกแล้ว")
		}
		if user.RoleKey != "super_admin" {
			var hasGhost bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM purchase_order_items WHERE purchase_order_id=$1 AND stock_bucket='ghost')`, id).Scan(&hasGhost); err != nil {
				return err
			}
			if hasGhost {
				return platform.NewError(http.StatusForbidden, "ใบสั่งซื้อนี้มีสต๊อกผีและยกเลิกได้เฉพาะผู้ดูแลระบบสูงสุด")
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT poi.product_id::text,poi.stock_bucket,poi.received_quantity,il.id::text,il.remaining_quantity FROM purchase_order_items poi INNER JOIN inventory_lots il ON il.source_item_id=poi.id WHERE poi.purchase_order_id=$1 ORDER BY poi.product_id,poi.id FOR UPDATE OF il`, id)
		if err != nil {
			return err
		}
		type line struct {
			product, bucket, lot string
			qty, remaining       int
		}
		lines := []line{}
		for rows.Next() {
			var l line
			if err := rows.Scan(&l.product, &l.bucket, &l.qty, &l.lot, &l.remaining); err != nil {
				rows.Close()
				return err
			}
			if l.remaining != l.qty {
				rows.Close()
				return platform.NewError(http.StatusConflict, "ยกเลิกไม่ได้ เพราะมีสินค้าจากใบสั่งซื้อนี้ถูกขายหรือโอนไปแล้ว")
			}
			lines = append(lines, l)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, l := range lines {
			column := "qty_real"
			if l.bucket == "ghost" {
				column = "qty_ghost"
			}
			if _, err := tx.ExecContext(ctx, `UPDATE inventory SET `+column+`=`+column+`-$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2 AND `+column+` >= $3`, branchID, l.product, l.qty); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE inventory_lots SET remaining_quantity=0,updated_at=NOW() WHERE id=$1`, l.lot); err != nil {
				return err
			}
			movementID := platform.MustUUID()
			if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_movements(id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at) VALUES($1,$2,$3,'purchase_cancel',$4,$5,'purchase_order',$6,$7,$8,NOW())`, movementID, branchID, l.product, l.bucket, -l.qty, id, reason, user.ID); err != nil {
				return err
			}
			if err := stocklot.AttachMovement(ctx, tx, movementID, []stocklot.Allocation{{Lot: stocklot.Lot{ID: l.lot}, Quantity: l.qty}}, -1); err != nil {
				return err
			}
			if err := recomputeLatestProductCost(ctx, tx, l.product); err != nil {
				return err
			}
		}
		nextRevision := revision + 1
		if _, err := tx.ExecContext(ctx, `UPDATE purchase_orders SET status='cancelled',revision=$2,correction_reason=$3,cancelled_by=$4,cancelled_at=NOW(),updated_by=$4,updated_at=NOW() WHERE id=$1`, id, nextRevision, reason, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO purchase_order_events(id,purchase_order_id,event_type,revision,note,actor_id,event_at) VALUES($1,$2,'cancelled',$3,$4,$5,NOW())`, platform.MustUUID(), id, nextRevision, reason, user.ID); err != nil {
			return err
		}
		meta.EntityType = "purchase_order"
		meta.EntityID = &id
		meta.Action = "purchase_order.cancel"
		meta.After = map[string]any{"po_number": number, "reason": reason, "revision": nextRevision}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (h *Handler) ListPurchaseOrders(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	result, err := h.service.ListPurchaseOrders(c.Request().Context(), platform.CurrentUser(c), PurchaseOrderFilter{Search: c.QueryParam("search"), SupplierID: c.QueryParam("supplier_id"), BranchID: c.QueryParam("branch_id"), Status: c.QueryParam("status"), DateFrom: c.QueryParam("date_from"), DateTo: c.QueryParam("date_to"), Page: page, PageSize: pageSize})
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}
func (h *Handler) GetPurchaseOrder(c echo.Context) error {
	item, err := h.service.GetPurchaseOrder(c.Request().Context(), platform.CurrentUser(c), c.Param("purchaseOrderID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}
func (h *Handler) CreatePurchaseOrder(c echo.Context) error {
	var input PurchaseOrderInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	id, err := h.service.CreatePurchaseOrder(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "บันทึกใบสั่งซื้อและเพิ่มสินค้าเข้าสต๊อกแล้ว"})
}
func (h *Handler) UpdatePurchaseOrder(c echo.Context) error {
	var input PurchaseOrderInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	if err := h.service.UpdatePurchaseOrder(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("purchaseOrderID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "แก้ไขใบสั่งซื้อและปรับสต๊อกแล้ว")
}
func (h *Handler) CancelPurchaseOrder(c echo.Context) error {
	var input struct {
		Reason string `json:"reason"`
	}
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	if err := h.service.CancelPurchaseOrder(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("purchaseOrderID"), input.Reason); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ยกเลิกใบสั่งซื้อและย้อนสต๊อกแล้ว")
}
func (h *Handler) ProductOptions(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	result, err := h.service.ProductOptions(c.Request().Context(), platform.CurrentUser(c), c.QueryParam("branch_id"), c.QueryParam("stock_bucket"), c.QueryParam("query"), c.QueryParam("cursor"), limit)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}
