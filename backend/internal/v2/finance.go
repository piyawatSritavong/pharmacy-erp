package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type CustomerInput struct {
	ID               string `json:"id"`
	BranchID         string `json:"branch_id"`
	Code             string `json:"code"`
	Type             string `json:"type"`
	Name             string `json:"name"`
	TaxID            string `json:"tax_id"`
	BillingAddress   string `json:"billing_address"`
	ShippingAddress  string `json:"shipping_address"`
	Phone            string `json:"phone"`
	Email            string `json:"email"`
	CreditDays       int    `json:"credit_days"`
	CreditLimitCents int64  `json:"credit_limit_cents"`
	Active           bool   `json:"active"`
	Revision         int    `json:"revision"`
}
type DocumentLine struct {
	ProductID      string `json:"product_id"`
	UnitID         string `json:"unit_id"`
	Quantity       int64  `json:"quantity"`
	DiscountCents  int64  `json:"discount_cents"`
	VATBPS         int64  `json:"vat_bps"`
	UnitPriceCents *int64 `json:"unit_price_cents,omitempty"`
}
type DocumentInput struct {
	BranchID    string         `json:"branch_id"`
	Kind        string         `json:"kind"`
	CustomerID  string         `json:"customer_id"`
	SupplierID  string         `json:"supplier_id"`
	IssuedOn    string         `json:"issued_on"`
	DueOn       string         `json:"due_on"`
	Notes       string         `json:"notes"`
	PromotionID string         `json:"promotion_id"`
	ApprovalID  string         `json:"approval_id"`
	Lines       []DocumentLine `json:"lines"`
}
type TransitionInput struct {
	BranchID    string `json:"branch_id"`
	ID          string `json:"id"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	EffectiveOn string `json:"effective_on"`
	Revision    int    `json:"revision"`
}
type CreditLine struct {
	SourceLineID string `json:"source_line_id"`
	Quantity     int64  `json:"quantity"`
}
type CreditInput struct {
	BranchID   string       `json:"branch_id"`
	DocumentID string       `json:"document_id"`
	IssuedOn   string       `json:"issued_on"`
	Reason     string       `json:"reason"`
	Lines      []CreditLine `json:"lines"`
}
type PaymentAllocation struct {
	DocumentID  string `json:"document_id"`
	AmountCents int64  `json:"amount_cents"`
}
type PaymentInput struct {
	BranchID     string              `json:"branch_id"`
	CustomerID   string              `json:"customer_id"`
	SupplierID   string              `json:"supplier_id"`
	Direction    string              `json:"direction"`
	Method       string              `json:"method"`
	AmountCents  int64               `json:"amount_cents"`
	Reference    string              `json:"reference"`
	PaidOn       string              `json:"paid_on"`
	DrawerID     string              `json:"drawer_id"`
	ChequeNumber string              `json:"cheque_number"`
	Bank         string              `json:"bank"`
	ChequeDueOn  string              `json:"cheque_due_on"`
	Allocations  []PaymentAllocation `json:"allocations"`
}
type AllocationInput struct {
	BranchID    string              `json:"branch_id"`
	PaymentID   string              `json:"payment_id"`
	Allocations []PaymentAllocation `json:"allocations"`
}
type PriceRuleInput struct {
	BranchID            string `json:"branch_id"`
	CustomerID          string `json:"customer_id"`
	ProductID           string `json:"product_id"`
	UnitID              string `json:"unit_id"`
	MinimumBaseQuantity int64  `json:"min_base_quantity"`
	UnitPriceCents      int64  `json:"unit_price_cents"`
	StartsOn            string `json:"starts_on"`
	EndsOn              string `json:"ends_on"`
}
type pricedLine struct {
	ID, ProductID, Description, PriceSource string
	Unit                                    UnitSnapshot
	Price, Discount, VATBPS, VAT, Total     int64
}

const documentsQuery = `SELECT jsonb_build_object('items',COALESCE(jsonb_agg(to_jsonb(x)||jsonb_build_object('payment_status',CASE WHEN x.status='cancelled' THEN 'cancelled' WHEN x.kind NOT IN ('ar_invoice','ap_bill') THEN NULL WHEN x.balance_cents<=0 THEN 'paid' WHEN x.balance_cents>=x.total_cents THEN 'unpaid' ELSE 'partial' END) ORDER BY x.created_at DESC),'[]'::jsonb)) FROM (SELECT d.*,COALESCE((SELECT SUM(e.amount_cents) FROM v2_money_events e WHERE e.document_id=d.id),0)::bigint AS balance_cents FROM v2_documents d WHERE branch_id=$1)x`
const financeQuery = `SELECT jsonb_build_object(
 'documents',COALESCE((SELECT jsonb_agg(row_to_json(x)) FROM (
 SELECT d.id,d.document_number,d.kind,d.customer_id,d.supplier_id,d.counterparty_snapshot,d.due_on,d.total_cents,
 COALESCE((SELECT SUM(e.amount_cents) FROM v2_money_events e WHERE e.document_id=d.id),0)::bigint AS balance_cents,
 GREATEST(0,(NOW() AT TIME ZONE 'Asia/Bangkok')::date-d.due_on) AS overdue_days
 FROM v2_documents d WHERE branch_id=$1 AND kind IN ('ar_invoice','ap_bill') AND status='issued'
 )x),'[]'::jsonb),
 'payments',COALESCE((SELECT jsonb_agg(row_to_json(x)) FROM (
 SELECT p.*,p.amount_cents-COALESCE((SELECT SUM(a.amount_cents) FROM v2_allocations a WHERE a.payment_id=p.id),0)::bigint AS unallocated_cents,
 c.id AS cheque_id,c.cheque_number,c.bank,c.due_on AS cheque_due_on,c.status AS cheque_status,c.revision AS cheque_revision
 FROM v2_payments p LEFT JOIN v2_cheques c ON c.payment_id=p.id WHERE p.branch_id=$1 ORDER BY p.created_at DESC LIMIT 200
 )x),'[]'::jsonb))`

func (s *Service) saveCustomer(ctx context.Context, tx *sql.Tx, a Actor, in CustomerInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.TaxID = strings.TrimSpace(in.TaxID)
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Code) == "" || (in.Type != "person" && in.Type != "business") || in.CreditDays < 0 || in.CreditDays > 3650 || in.CreditLimitCents < 0 || in.CreditLimitCents > maxInteger {
		return nil, bad("ข้อมูลลูกค้าหรือเงื่อนไขเครดิตไม่ถูกต้อง")
	}
	if in.ID == "" {
		in.ID = newID()
		_, err = tx.ExecContext(ctx, `INSERT INTO v2_customers(id,branch_id,customer_code,customer_type,name,tax_id,billing_address,shipping_address,phone,email,credit_days,credit_limit_cents,active,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, in.ID, b, in.Code, in.Type, in.Name, in.TaxID, in.BillingAddress, in.ShippingAddress, in.Phone, in.Email, in.CreditDays, in.CreditLimitCents, in.Active, a.User.ID)
	} else {
		result, e := tx.ExecContext(ctx, `UPDATE v2_customers SET customer_code=$3,customer_type=$4,name=$5,tax_id=$6,billing_address=$7,shipping_address=$8,phone=$9,email=$10,credit_days=$11,credit_limit_cents=$12,active=$13,revision=revision+1,updated_at=NOW() WHERE id=$1 AND branch_id=$2 AND revision=$14`, in.ID, b, in.Code, in.Type, in.Name, in.TaxID, in.BillingAddress, in.ShippingAddress, in.Phone, in.Email, in.CreditDays, in.CreditLimitCents, in.Active, in.Revision)
		if e != nil {
			return nil, e
		}
		n, e := result.RowsAffected()
		if e != nil || n != 1 {
			return nil, conflict("ข้อมูลลูกค้ามีการแก้ไขแล้ว กรุณาโหลดใหม่")
		}
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": in.ID}, emit(ctx, tx, a, "customer.changed", map[string]string{"id": in.ID, "branch_id": b})
}

func counterparty(ctx context.Context, tx *sql.Tx, b, customer, supplier string) (json.RawMessage, int, error) {
	var snapshot []byte
	var days int
	if (customer == "") == (supplier == "") {
		return nil, 0, bad("เลือกลูกค้าหรือ supplier อย่างใดอย่างหนึ่ง")
	}
	if customer != "" {
		if err := tx.QueryRowContext(ctx, `SELECT row_to_json(c),credit_days FROM v2_customers c WHERE id=$1 AND branch_id=$2 AND active=TRUE FOR UPDATE`, customer, b).Scan(&snapshot, &days); err != nil {
			return nil, 0, conflict("ลูกค้าไม่พร้อมใช้งานในสาขานี้")
		}
	} else {
		if err := tx.QueryRowContext(ctx, `SELECT row_to_json(s),payment_terms_days FROM suppliers s WHERE id=$1 AND active=TRUE FOR UPDATE`, supplier).Scan(&snapshot, &days); err != nil {
			return nil, 0, conflict("supplier ไม่พร้อมใช้งาน")
		}
	}
	return snapshot, days, nil
}

func ensureCredit(ctx context.Context, tx *sql.Tx, b, customer string, total int64) error {
	var limit, balance int64
	if err := tx.QueryRowContext(ctx, `SELECT credit_limit_cents FROM v2_customers WHERE id=$1 AND branch_id=$2 FOR UPDATE`, customer, b).Scan(&limit); err != nil {
		return err
	}
	// A pending refund is not a credit allocation against another invoice.
	// Count positive debt per invoice so refund liabilities cannot mask exposure.
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(GREATEST(x.balance,0)),0)::bigint FROM (SELECT COALESCE(SUM(e.amount_cents),0)::bigint AS balance FROM v2_documents d LEFT JOIN v2_money_events e ON e.document_id=d.id WHERE d.customer_id=$1 AND d.branch_id=$2 AND d.kind='ar_invoice' GROUP BY d.id)x`, customer, b).Scan(&balance); err != nil {
		return err
	}
	if balance < 0 {
		balance = 0
	}
	next, err := add(balance, total)
	if err != nil {
		return err
	}
	if next > limit {
		return conflict("ยอดค้างรวมบิลใหม่นี้เกินวงเงินเครดิต")
	}
	return nil
}

func moneyEvent(ctx context.Context, tx *sql.Tx, a Actor, b, doc, payment, cheque, kind, effective, reason string, amount int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO v2_money_events(id,operation_id,branch_id,document_id,payment_id,cheque_id,event_type,amount_cents,effective_on,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::date,$10,$11)`, newID(), a.OperationID, b, optionalID(doc), optionalID(payment), optionalID(cheque), kind, amount, effective, reason, a.User.ID)
	return err
}

func (s *Service) createDocument(ctx context.Context, tx *sql.Tx, a Actor, in DocumentInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	approvalInput := in
	if in.Kind != "quotation" && in.Kind != "ar_invoice" && in.Kind != "ap_bill" {
		return nil, bad("ประเภทเอกสารไม่ถูกต้อง")
	}
	if len(in.Lines) == 0 || len(in.Lines) > 200 {
		return nil, bad("เอกสารต้องมีสินค้า 1–200 รายการ")
	}
	if (in.Kind == "ap_bill") != (in.SupplierID != "") {
		return nil, bad("ใบซื้อใช้ supplier และใบขายใช้ customer")
	}
	if in.IssuedOn == "" {
		in.IssuedOn = today()
	}
	if err = date(in.IssuedOn); err != nil {
		return nil, err
	}
	if in.IssuedOn != today() {
		return nil, bad("เอกสาร V2 รอบนี้ออกได้เฉพาะวันที่ปัจจุบัน")
	}
	snapshot, days, err := counterparty(ctx, tx, b, in.CustomerID, in.SupplierID)
	if err != nil {
		return nil, err
	}
	pricingItems := []StockLine{}
	for _, l := range in.Lines {
		pricingItems = append(pricingItems, StockLine{ProductID: l.ProductID})
	}
	if err = lockStock(ctx, tx, []string{b}, pricingItems); err != nil {
		return nil, err
	}
	if in.ApprovalID != "" {
		if in.Kind == "ap_bill" {
			return nil, bad("การอนุมัติส่วนลดขายไม่ใช้กับใบซื้อ")
		}
		if err = consumeDiscountApproval(ctx, tx, a, b, approvalInput); err != nil {
			return nil, err
		}
	}
	if in.DueOn == "" {
		issued, _ := time.Parse("2006-01-02", in.IssuedOn)
		in.DueOn = issued.AddDate(0, 0, days).Format("2006-01-02")
	}
	if err = date(in.DueOn); err != nil {
		return nil, err
	}
	if in.DueOn < in.IssuedOn {
		return nil, bad("วันครบกำหนดต้องไม่ก่อนวันออกเอกสาร")
	}
	var discountLimit int64
	if err = tx.QueryRowContext(ctx, `SELECT max_discount_bps FROM v2_branch_policies WHERE branch_id=$1`, b).Scan(&discountLimit); err != nil {
		return nil, conflict("ตั้งนโยบาย V2 ของสาขาก่อนออกเอกสาร")
	}
	units := make([]UnitSnapshot, len(in.Lines))
	quantityByProduct := map[string]int64{}
	for i, line := range in.Lines {
		units[i], err = resolveUnit(ctx, tx, line.ProductID, line.UnitID, line.Quantity)
		if err != nil {
			return nil, err
		}
		quantityByProduct[line.ProductID], err = add(quantityByProduct[line.ProductID], units[i].BaseQuantity)
		if err != nil {
			return nil, err
		}
	}
	priced := []pricedLine{}
	var total, tax, discount int64
	for i, line := range in.Lines {
		if line.DiscountCents < 0 || line.VATBPS < 0 || line.VATBPS > 10000 {
			return nil, bad("ส่วนลดหรือภาษีไม่ถูกต้อง")
		}
		u := units[i]
		price := u.DefaultPriceCents
		priceSource := "catalog"
		if in.Kind == "ap_bill" {
			if line.UnitPriceCents == nil || *line.UnitPriceCents < 0 || *line.UnitPriceCents > maxInteger {
				return nil, bad("ใบซื้อจำเป็นต้องระบุต้นทุนต่อหน่วย")
			}
			price = *line.UnitPriceCents
			priceSource = "supplier_bill"
		} else if line.UnitPriceCents != nil {
			return nil, bad("ราคาขายต้องมาจาก catalog หรือ price rule")
		}
		var ruleID string
		e := sql.ErrNoRows
		if in.Kind != "ap_bill" {
			e = tx.QueryRowContext(ctx, `SELECT id::text,unit_price_cents FROM v2_price_rules WHERE branch_id=$1 AND product_id=$2 AND unit_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid AND (customer_id IS NULL OR customer_id=NULLIF($4,'')::uuid) AND active=TRUE AND starts_on<=$5::date AND (ends_on IS NULL OR ends_on>=$5::date) AND min_base_quantity<=$6 ORDER BY (customer_id IS NOT NULL) DESC,min_base_quantity DESC,id LIMIT 1`, b, line.ProductID, line.UnitID, in.CustomerID, in.IssuedOn, quantityByProduct[line.ProductID]).Scan(&ruleID, &price)
		}
		if e != nil && e != sql.ErrNoRows {
			return nil, e
		}
		if e == nil {
			priceSource = "rule:" + ruleID
		}
		gross, e := multiply(price, line.Quantity)
		if e != nil {
			return nil, e
		}
		allowed := gross
		if in.Kind != "ap_bill" && in.ApprovalID == "" {
			allowed, e = proportion(gross, discountLimit, 10000)
			if e != nil {
				return nil, e
			}
		}
		if line.DiscountCents > allowed {
			return nil, conflict("ส่วนลดเกินเพดานที่กำหนด")
		}
		net := gross - line.DiscountCents
		vat, e := proportion(net, line.VATBPS, 10000)
		if e != nil {
			return nil, e
		}
		lineTotal, e := add(net, vat)
		if e != nil {
			return nil, e
		}
		var name string
		if e = tx.QueryRowContext(ctx, `SELECT name FROM products WHERE id=$1`, line.ProductID).Scan(&name); e != nil {
			return nil, e
		}
		priced = append(priced, pricedLine{ID: newID(), ProductID: line.ProductID, Description: name, PriceSource: priceSource, Unit: u, Price: price, Discount: line.DiscountCents, VATBPS: line.VATBPS, VAT: vat, Total: lineTotal})
		total, err = add(total, lineTotal)
		if err != nil {
			return nil, err
		}
		tax, err = add(tax, vat)
		if err != nil {
			return nil, err
		}
		discount, err = add(discount, line.DiscountCents)
		if err != nil {
			return nil, err
		}
	}
	var promotionSnapshot json.RawMessage
	if in.PromotionID != "" {
		if in.Kind == "ap_bill" {
			return nil, bad("โปรขายใช้กับใบซื้อไม่ได้")
		}
		priced, promotionSnapshot, err = applyPromotion(ctx, tx, b, in.PromotionID, in.IssuedOn, priced)
		if err != nil {
			return nil, err
		}
		total, tax, discount = 0, 0, 0
		for _, l := range priced {
			total, err = add(total, l.Total)
			if err != nil {
				return nil, err
			}
			tax, err = add(tax, l.VAT)
			if err != nil {
				return nil, err
			}
			discount, err = add(discount, l.Discount)
			if err != nil {
				return nil, err
			}
		}
	}
	if total <= 0 {
		return nil, bad("ยอดเอกสารต้องมากกว่าศูนย์")
	}
	if in.Kind == "ar_invoice" {
		if err = ensureCredit(ctx, tx, b, in.CustomerID, total); err != nil {
			return nil, err
		}
	}
	id := newID()
	number := "V2-" + strings.ToUpper(in.Kind) + "-" + strings.ReplaceAll(id, "-", "")
	status := "issued"
	if in.Kind == "quotation" {
		status = "draft"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_documents(id,operation_id,branch_id,document_number,kind,status,customer_id,supplier_id,counterparty_snapshot,total_cents,vat_cents,discount_cents,issued_on,due_on,notes,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13::date,$14::date,$15,$16)`, id, a.OperationID, b, number, in.Kind, status, optionalID(in.CustomerID), optionalID(in.SupplierID), string(snapshot), total, tax, discount, in.IssuedOn, in.DueOn, in.Notes, a.User.ID)
	if err != nil {
		return nil, err
	}
	stockLines := []StockLine{}
	if len(promotionSnapshot) > 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO v2_document_promotions(id,document_id,promotion_id,snapshot) VALUES($1,$2,$3,$4::jsonb)`, newID(), id, in.PromotionID, string(promotionSnapshot)); err != nil {
			return nil, err
		}
	}
	for _, line := range priced {
		_, err = tx.ExecContext(ctx, `INSERT INTO v2_document_lines(id,document_id,product_id,description,quantity,base_quantity,unit_snapshot,unit_price_cents,discount_cents,vat_bps,vat_cents,total_cents,price_source) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12,$13)`, line.ID, id, line.ProductID, line.Description, line.Unit.Quantity, line.Unit.BaseQuantity, rawJSON(line.Unit), line.Price, line.Discount, line.VATBPS, line.VAT, line.Total, line.PriceSource)
		if err != nil {
			return nil, err
		}
		stockLines = append(stockLines, StockLine{ProductID: line.ProductID, Quantity: line.Unit.BaseQuantity})
	}
	if in.Kind == "ar_invoice" {
		if err = lockStock(ctx, tx, []string{b}, stockLines); err != nil {
			return nil, err
		}
		for _, line := range priced {
			lots, e := availableLots(ctx, tx, b, line.ProductID, "", line.Unit.BaseQuantity)
			if e != nil {
				return nil, e
			}
			if _, e = issueAllocations(ctx, tx, a, b, line.ProductID, "invoice.issue", id, "", line.Unit, lots); e != nil {
				return nil, e
			}
		}
	}
	if status == "issued" {
		if err = moneyEvent(ctx, tx, a, b, id, "", "", "document.issued", in.IssuedOn, "", total); err != nil {
			return nil, err
		}
	}
	return map[string]any{"id": id, "document_number": number, "total_cents": total, "status": status}, emit(ctx, tx, a, "document.created", map[string]string{"id": id, "branch_id": b})
}

func (s *Service) documentTransition(ctx context.Context, tx *sql.Tx, a Actor, in TransitionInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	var kind, status, customer string
	var total int64
	if err = tx.QueryRowContext(ctx, `SELECT kind,status,COALESCE(customer_id::text,''),total_cents FROM v2_documents WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.ID, b).Scan(&kind, &status, &customer, &total); err != nil {
		return nil, err
	}
	if kind != "quotation" || status != "draft" {
		return nil, conflict("เปลี่ยนได้เฉพาะใบเสนอราคาที่ยังเป็นฉบับร่าง")
	}
	if in.Action == "cancel" {
		if strings.TrimSpace(in.Reason) == "" {
			return nil, bad("กรุณาระบุเหตุผลยกเลิก")
		}
		_, err = tx.ExecContext(ctx, `UPDATE v2_documents SET status='cancelled' WHERE id=$1`, in.ID)
		return map[string]any{"id": in.ID, "status": "cancelled"}, err
	}
	if in.Action != "convert" {
		return nil, bad("เลือก convert หรือ cancel")
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT active FROM v2_customers WHERE id=$1 AND branch_id=$2 FOR UPDATE`, customer, b).Scan(&active); err != nil {
		return nil, err
	}
	if !active {
		return nil, conflict("ลูกค้าถูกปิดใช้งาน")
	}
	if err = ensureCredit(ctx, tx, b, customer, total); err != nil {
		return nil, err
	}
	id := newID()
	number := "V2-AR-" + strings.ReplaceAll(id, "-", "")
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_documents(id,operation_id,branch_id,document_number,kind,status,customer_id,counterparty_snapshot,source_document_id,total_cents,vat_cents,discount_cents,issued_on,due_on,notes,created_by) SELECT $2,$3,branch_id,$4,'ar_invoice','issued',customer_id,counterparty_snapshot,id,total_cents,vat_cents,discount_cents,$5::date,GREATEST(due_on,$5::date),notes,$6 FROM v2_documents WHERE id=$1`, in.ID, id, a.OperationID, number, today(), a.User.ID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id::text,product_id::text,base_quantity,unit_snapshot FROM v2_document_lines WHERE document_id=$1 ORDER BY product_id,id`, in.ID)
	if err != nil {
		return nil, err
	}
	type quoteLine struct {
		id, p string
		qty   int64
		unit  []byte
	}
	lines := []quoteLine{}
	stock := []StockLine{}
	for rows.Next() {
		var l quoteLine
		if err = rows.Scan(&l.id, &l.p, &l.qty, &l.unit); err != nil {
			rows.Close()
			return nil, err
		}
		lines = append(lines, l)
		stock = append(stock, StockLine{ProductID: l.p})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = lockStock(ctx, tx, []string{b}, stock); err != nil {
		return nil, err
	}
	for _, l := range lines {
		var u UnitSnapshot
		if err = json.Unmarshal(l.unit, &u); err != nil {
			return nil, err
		}
		items, e := availableLots(ctx, tx, b, l.p, "", l.qty)
		if e != nil {
			return nil, e
		}
		if _, e = issueAllocations(ctx, tx, a, b, l.p, "invoice.issue", id, "quotation conversion", u, items); e != nil {
			return nil, e
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO v2_document_lines(id,document_id,product_id,source_line_id,description,quantity,base_quantity,unit_snapshot,unit_price_cents,discount_cents,vat_bps,vat_cents,total_cents,price_source) SELECT $2,$3,product_id,id,description,quantity,base_quantity,unit_snapshot,unit_price_cents,discount_cents,vat_bps,vat_cents,total_cents,price_source FROM v2_document_lines WHERE id=$1`, l.id, newID(), id)
		if err != nil {
			return nil, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE v2_documents SET status='converted' WHERE id=$1`, in.ID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_document_promotions(id,document_id,promotion_id,snapshot) SELECT $2,$3,promotion_id,snapshot FROM v2_document_promotions WHERE document_id=$1`, in.ID, newID(), id); err != nil {
		return nil, err
	}
	if err = moneyEvent(ctx, tx, a, b, id, "", "", "document.issued", today(), "converted quotation", total); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "document_number": number, "status": "issued"}, emit(ctx, tx, a, "quotation.converted", map[string]string{"source_id": in.ID, "invoice_id": id})
}

func (s *Service) creditNote(ctx context.Context, tx *sql.Tx, a Actor, in CreditInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if len(in.Lines) == 0 || len(in.Lines) > 200 || strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุรายการและเหตุผลลดหนี้")
	}
	if in.IssuedOn == "" {
		in.IssuedOn = today()
	}
	if in.IssuedOn != today() {
		return nil, bad("ใบลดหนี้ V2 รอบนี้ออกได้เฉพาะวันนี้")
	}
	var kind, status string
	var customer, supplier sql.NullString
	var snapshot []byte
	if err = tx.QueryRowContext(ctx, `SELECT kind,status,customer_id::text,supplier_id::text,counterparty_snapshot FROM v2_documents WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.DocumentID, b).Scan(&kind, &status, &customer, &supplier, &snapshot); err != nil {
		return nil, err
	}
	if status != "issued" || (kind != "ar_invoice" && kind != "ap_bill") {
		return nil, conflict("ใบเดิมต้องเป็นใบขายหรือใบซื้อที่ออกแล้ว")
	}
	creditKind := "ar_credit"
	if kind == "ap_bill" {
		creditKind = "ap_credit"
	}
	id := newID()
	number := "V2-CN-" + strings.ReplaceAll(id, "-", "")
	type credit struct {
		source, p, description, priceSource            string
		unit                                           []byte
		qty, base, price, discount, vatBPS, vat, total int64
	}
	credits := []credit{}
	seen := map[string]bool{}
	var total, tax int64
	for _, item := range in.Lines {
		if item.Quantity <= 0 || seen[item.SourceLineID] {
			return nil, bad("รายการลดหนี้ซ้ำหรือจำนวนไม่ถูกต้อง")
		}
		seen[item.SourceLineID] = true
		var c credit
		var sold, base, originalTotal, originalVAT, originalDiscount int64
		if err = tx.QueryRowContext(ctx, `SELECT product_id::text,description,quantity,base_quantity,unit_snapshot,unit_price_cents,discount_cents,vat_bps,vat_cents,total_cents,price_source FROM v2_document_lines WHERE id=$1 AND document_id=$2`, item.SourceLineID, in.DocumentID).Scan(&c.p, &c.description, &sold, &base, &c.unit, &c.price, &originalDiscount, &c.vatBPS, &originalVAT, &originalTotal, &c.priceSource); err != nil {
			return nil, err
		}
		var priorQty, priorTotal, priorVAT, priorDiscount int64
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(l.quantity),0)::bigint,COALESCE(SUM(l.total_cents),0)::bigint,COALESCE(SUM(l.vat_cents),0)::bigint,COALESCE(SUM(l.discount_cents),0)::bigint FROM v2_document_lines l JOIN v2_documents d ON d.id=l.document_id WHERE l.source_line_id=$1 AND d.kind IN ('ar_credit','ap_credit') AND d.status='issued'`, item.SourceLineID).Scan(&priorQty, &priorTotal, &priorVAT, &priorDiscount); err != nil {
			return nil, err
		}
		if item.Quantity > sold-priorQty {
			return nil, conflict("จำนวนลดหนี้สะสมเกินจำนวนในใบเดิม")
		}
		c.source = item.SourceLineID
		c.qty = item.Quantity
		c.base, err = proportion(base, item.Quantity, sold)
		if err != nil {
			return nil, err
		}
		if item.Quantity == sold-priorQty {
			c.total = originalTotal - priorTotal
			c.vat = originalVAT - priorVAT
			c.discount = originalDiscount - priorDiscount
		} else {
			c.total, err = proportion(originalTotal, item.Quantity, sold)
			if err != nil {
				return nil, err
			}
			c.vat, err = proportion(originalVAT, item.Quantity, sold)
			if err != nil {
				return nil, err
			}
			c.discount, err = proportion(originalDiscount, item.Quantity, sold)
			if err != nil {
				return nil, err
			}
		}
		total, err = add(total, c.total)
		if err != nil {
			return nil, err
		}
		tax, err = add(tax, c.vat)
		if err != nil {
			return nil, err
		}
		credits = append(credits, c)
	}
	if total <= 0 {
		return nil, bad("ยอดลดหนี้ต้องมากกว่าศูนย์")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_documents(id,operation_id,branch_id,document_number,kind,status,customer_id,supplier_id,counterparty_snapshot,source_document_id,total_cents,vat_cents,issued_on,due_on,notes,created_by) VALUES($1,$2,$3,$4,$5,'issued',$6,$7,$8::jsonb,$9,$10,$11,$12::date,$12::date,$13,$14)`, id, a.OperationID, b, number, creditKind, optionalID(customer.String), optionalID(supplier.String), string(snapshot), in.DocumentID, total, tax, in.IssuedOn, in.Reason, a.User.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range credits {
		_, err = tx.ExecContext(ctx, `INSERT INTO v2_document_lines(id,document_id,product_id,source_line_id,description,quantity,base_quantity,unit_snapshot,unit_price_cents,discount_cents,vat_bps,vat_cents,total_cents,price_source) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12,$13,$14)`, newID(), id, c.p, c.source, c.description, c.qty, c.base, string(c.unit), c.price, c.discount, c.vatBPS, c.vat, c.total, c.priceSource)
		if err != nil {
			return nil, err
		}
	}
	if err = moneyEvent(ctx, tx, a, b, in.DocumentID, "", "", "credit_note.issued", in.IssuedOn, in.Reason, -total); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "document_number": number, "total_cents": total, "stock_returned": false}, emit(ctx, tx, a, "credit_note.issued", map[string]string{"id": id, "source_id": in.DocumentID})
}

func (s *Service) createPayment(ctx context.Context, tx *sql.Tx, a Actor, in PaymentInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.AmountCents <= 0 || in.AmountCents > maxInteger || (in.Method != "cash" && in.Method != "bank_transfer" && in.Method != "cheque") {
		return nil, bad("จำนวนเงินหรือวิธีชำระไม่ถูกต้อง")
	}
	if in.Direction != "receive" && in.Direction != "pay" {
		return nil, bad("ระบุทิศทาง receive หรือ pay")
	}
	if (in.Direction == "receive") != (in.CustomerID != "") {
		return nil, bad("รับเงินใช้ลูกค้า จ่ายเงินใช้ supplier")
	}
	if _, _, err = counterparty(ctx, tx, b, in.CustomerID, in.SupplierID); err != nil {
		return nil, err
	}
	if in.PaidOn == "" {
		in.PaidOn = today()
	}
	if in.PaidOn != today() {
		return nil, bad("ลงรับ/จ่ายเงิน V2 รอบนี้ได้เฉพาะวันนี้")
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_payments(id,operation_id,branch_id,direction,customer_id,supplier_id,method,amount_cents,reference,paid_on,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::date,$11)`, id, a.OperationID, b, in.Direction, optionalID(in.CustomerID), optionalID(in.SupplierID), in.Method, in.AmountCents, in.Reference, in.PaidOn, a.User.ID); err != nil {
		return nil, err
	}
	chequeID := ""
	if in.Method == "cheque" {
		if in.ChequeNumber == "" || in.Bank == "" {
			return nil, bad("กรุณาระบุเลขเช็คและธนาคาร")
		}
		if err = date(in.ChequeDueOn); err != nil {
			return nil, err
		}
		chequeID = newID()
		if _, err = tx.ExecContext(ctx, `INSERT INTO v2_cheques(id,branch_id,payment_id,cheque_number,bank,due_on) VALUES($1,$2,$3,$4,$5,$6::date)`, chequeID, b, id, strings.TrimSpace(in.ChequeNumber), strings.TrimSpace(in.Bank), in.ChequeDueOn); err != nil {
			return nil, err
		}
	} else {
		if err = moneyEvent(ctx, tx, a, b, "", id, "", "payment."+in.Direction, in.PaidOn, in.Reference, in.AmountCents); err != nil {
			return nil, err
		}
	}
	if in.Method == "cash" {
		if err = drawerEntry(ctx, tx, a, b, in.DrawerID, id, in.Direction, in.AmountCents); err != nil {
			return nil, err
		}
	}
	if err = s.applyAllocations(ctx, tx, a, b, id, in.Allocations, in.Method != "cheque", in.PaidOn); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "cheque_id": chequeID, "settled": in.Method != "cheque"}, emit(ctx, tx, a, "payment.created", map[string]string{"id": id, "branch_id": b})
}

func (s *Service) applyAllocations(ctx context.Context, tx *sql.Tx, a Actor, b, payment string, items []PaymentAllocation, settled bool, effective string) error {
	items = append([]PaymentAllocation(nil), items...)
	sort.Slice(items, func(i, j int) bool { return items[i].DocumentID < items[j].DocumentID })
	var customer, supplier, direction, method string
	var amount, used int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(customer_id::text,''),COALESCE(supplier_id::text,''),direction,method,amount_cents FROM v2_payments WHERE id=$1 AND branch_id=$2 FOR UPDATE`, payment, b).Scan(&customer, &supplier, &direction, &method, &amount); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0)::bigint FROM v2_allocations WHERE payment_id=$1`, payment).Scan(&used); err != nil {
		return err
	}
	if len(items) > 200 {
		return bad("จัดสรรได้สูงสุด 200 เอกสารต่อครั้ง")
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.AmountCents <= 0 || item.AmountCents > amount-used || seen[item.DocumentID] {
			return bad("ยอดจัดสรรเกินเงินคงเหลือ หรือรายการซ้ำ")
		}
		seen[item.DocumentID] = true
		var docCustomer, docSupplier, kind, status string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(customer_id::text,''),COALESCE(supplier_id::text,''),kind,status FROM v2_documents WHERE id=$1 AND branch_id=$2 FOR UPDATE`, item.DocumentID, b).Scan(&docCustomer, &docSupplier, &kind, &status); err != nil {
			return err
		}
		if customer != docCustomer || supplier != docSupplier || status != "issued" || (direction == "receive" && kind != "ar_invoice") || (direction == "pay" && kind != "ap_bill") {
			return conflict("เอกสารและเงินต้องเป็นคู่ค้าและสาขาเดียวกัน")
		}
		var balance, pending int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0)::bigint FROM v2_money_events WHERE document_id=$1`, item.DocumentID).Scan(&balance); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(a.amount_cents),0)::bigint FROM v2_allocations a JOIN v2_cheques c ON c.payment_id=a.payment_id WHERE a.document_id=$1 AND c.status IN ('received','deposited')`, item.DocumentID).Scan(&pending); err != nil {
			return err
		}
		if item.AmountCents > balance-pending {
			return conflict("ยอดจัดสรรเกินยอดหนี้ที่ยังไม่ได้กันเช็ค")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO v2_allocations(id,operation_id,payment_id,document_id,amount_cents) VALUES($1,$2,$3,$4,$5)`, newID(), a.OperationID, payment, item.DocumentID, item.AmountCents); err != nil {
			return err
		}
		if settled {
			if err := moneyEvent(ctx, tx, a, b, item.DocumentID, payment, "", "payment.allocated", effective, "", -item.AmountCents); err != nil {
				return err
			}
		}
		used += item.AmountCents
	}
	return nil
}

func (s *Service) allocatePayment(ctx context.Context, tx *sql.Tx, a Actor, in AllocationInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if len(in.Allocations) == 0 {
		return nil, bad("ระบุเอกสารจัดสรรเงิน")
	}
	var method string
	if err = tx.QueryRowContext(ctx, `SELECT method FROM v2_payments WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.PaymentID, b).Scan(&method); err != nil {
		return nil, err
	}
	settled := method != "cheque"
	if method == "cheque" {
		var status string
		if err = tx.QueryRowContext(ctx, `SELECT status FROM v2_cheques WHERE payment_id=$1 FOR UPDATE`, in.PaymentID).Scan(&status); err != nil {
			return nil, err
		}
		if status == "bounced" || status == "cancelled" {
			return nil, conflict("เช็คนี้ไม่พร้อมจัดสรร")
		}
		settled = status == "cleared"
	}
	err = s.applyAllocations(ctx, tx, a, b, in.PaymentID, in.Allocations, settled, today())
	return map[string]string{"payment_id": in.PaymentID}, err
}

func (s *Service) chequeTransition(ctx context.Context, tx *sql.Tx, a Actor, in TransitionInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	// Payment first is the common lock order for allocation and cheque updates.
	var payment string
	if err = tx.QueryRowContext(ctx, `SELECT c.payment_id::text FROM v2_cheques c JOIN v2_payments p ON p.id=c.payment_id WHERE c.id=$1 AND p.branch_id=$2`, in.ID, b).Scan(&payment); err != nil {
		return nil, err
	}
	var amount int64
	var direction string
	if err = tx.QueryRowContext(ctx, `SELECT amount_cents,direction FROM v2_payments WHERE id=$1 FOR UPDATE`, payment).Scan(&amount, &direction); err != nil {
		return nil, err
	}
	var old, due string
	var revision int
	if err = tx.QueryRowContext(ctx, `SELECT status,due_on::text,revision FROM v2_cheques WHERE id=$1 FOR UPDATE`, in.ID).Scan(&old, &due, &revision); err != nil {
		return nil, err
	}
	if revision != in.Revision {
		return nil, conflict("สถานะเช็คเปลี่ยนแล้ว กรุณาโหลดใหม่")
	}
	valid := (old == "received" && (in.Action == "deposited" || in.Action == "cancelled")) || (old == "deposited" && (in.Action == "cleared" || in.Action == "bounced" || in.Action == "cancelled")) || (old == "cleared" && in.Action == "bounced")
	if !valid {
		return nil, conflict("เปลี่ยนสถานะเช็คตามลำดับนี้ไม่ได้")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุหลักฐานหรือเหตุผลเปลี่ยนสถานะเช็ค")
	}
	if in.EffectiveOn == "" {
		in.EffectiveOn = today()
	}
	if in.EffectiveOn != today() {
		return nil, bad("ลงสถานะเช็คได้เฉพาะวันนี้")
	}
	if in.Action == "cleared" && in.EffectiveOn < due {
		return nil, conflict("ยังไม่ถึงวันเช็ค")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE v2_cheques SET status=$2,revision=revision+1 WHERE id=$1`, in.ID, in.Action); err != nil {
		return nil, err
	}
	if in.Action == "cleared" || old == "cleared" {
		factor := int64(-1)
		kind := "cheque.cleared"
		if old == "cleared" {
			factor = 1
			kind = "cheque.bounced"
		}
		rows, e := tx.QueryContext(ctx, `SELECT document_id::text,amount_cents FROM v2_allocations WHERE payment_id=$1 ORDER BY document_id`, payment)
		if e != nil {
			return nil, e
		}
		allocations := []PaymentAllocation{}
		for rows.Next() {
			var item PaymentAllocation
			if e = rows.Scan(&item.DocumentID, &item.AmountCents); e != nil {
				rows.Close()
				return nil, e
			}
			allocations = append(allocations, item)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		for _, item := range allocations {
			var ignored string
			if e = tx.QueryRowContext(ctx, `SELECT id::text FROM v2_documents WHERE id=$1 FOR UPDATE`, item.DocumentID).Scan(&ignored); e != nil {
				return nil, e
			}
			if e = moneyEvent(ctx, tx, a, b, item.DocumentID, payment, in.ID, kind, in.EffectiveOn, in.Reason, factor*item.AmountCents); e != nil {
				return nil, e
			}
		}
		if err = moneyEvent(ctx, tx, a, b, "", payment, in.ID, "payment."+direction+"."+kind, in.EffectiveOn, in.Reason, -factor*amount); err != nil {
			return nil, err
		}
	} else {
		if err = moneyEvent(ctx, tx, a, b, "", payment, in.ID, "cheque."+in.Action, in.EffectiveOn, in.Reason, 0); err != nil {
			return nil, err
		}
	}
	return map[string]any{"id": in.ID, "status": in.Action, "revision": revision + 1}, emit(ctx, tx, a, "cheque.changed", map[string]string{"id": in.ID, "status": in.Action})
}

func (s *Service) priceRule(ctx context.Context, tx *sql.Tx, a Actor, in PriceRuleInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.MinimumBaseQuantity <= 0 || in.UnitPriceCents < 0 || in.UnitPriceCents > maxInteger {
		return nil, bad("จำนวนขั้นต่ำหรือราคาไม่ถูกต้อง")
	}
	if err = date(in.StartsOn); err != nil {
		return nil, err
	}
	if in.EndsOn != "" {
		if err = date(in.EndsOn); err != nil {
			return nil, err
		}
		if in.EndsOn < in.StartsOn {
			return nil, bad("วันสิ้นสุดก่อนวันเริ่ม")
		}
	}
	if _, err = resolveUnit(ctx, tx, in.ProductID, in.UnitID, 1); err != nil {
		return nil, err
	}
	if in.CustomerID != "" {
		if _, _, err = counterparty(ctx, tx, b, in.CustomerID, ""); err != nil {
			return nil, err
		}
	}
	if err = lockStock(ctx, tx, []string{b}, []StockLine{{ProductID: in.ProductID}}); err != nil {
		return nil, err
	}
	id := newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_price_rules(id,branch_id,customer_id,product_id,unit_id,min_base_quantity,unit_price_cents,starts_on,ends_on,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8::date,NULLIF($9,'')::date,$10)`, id, b, optionalID(in.CustomerID), in.ProductID, optionalID(in.UnitID), in.MinimumBaseQuantity, in.UnitPriceCents, in.StartsOn, in.EndsOn, a.User.ID)
	return map[string]string{"id": id}, err
}

func (s *Service) documentDetail(c echo.Context) error {
	ctx := c.Request().Context()
	b, err := branch(ctx, s.db, platform.CurrentUser(c), c.QueryParam("branch_id"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	var raw []byte
	err = s.db.QueryRowContext(ctx, `SELECT jsonb_build_object('document',row_to_json(d),'promotion',(SELECT row_to_json(pr) FROM v2_document_promotions pr WHERE pr.document_id=d.id),'lines',COALESCE((SELECT jsonb_agg(row_to_json(l) ORDER BY l.id) FROM v2_document_lines l WHERE document_id=d.id),'[]'::jsonb),'events',COALESCE((SELECT jsonb_agg(row_to_json(e) ORDER BY e.created_at) FROM v2_money_events e WHERE document_id=d.id),'[]'::jsonb)) FROM v2_documents d WHERE id=$1 AND branch_id=$2`, c.Param("id"), b).Scan(&raw)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(404, "ไม่พบเอกสาร V2", err))
	}
	return c.JSONBlob(200, raw)
}
