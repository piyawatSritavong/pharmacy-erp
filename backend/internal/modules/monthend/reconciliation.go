package monthend

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/stocklot"
	"pharmacy-erp/backend/internal/platform"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const reconciliationToleranceCents int64 = 1

type ReconciliationAdjustmentInput struct {
	InvoiceItemID string  `json:"invoice_item_id"`
	NewUnitPrice  float64 `json:"new_unit_price"`
}

type ReconciliationInput struct {
	Month             string                          `json:"month"`
	DateFrom          string                          `json:"date_from"`
	DateTo            string                          `json:"date_to"`
	BranchIDs         []string                        `json:"branch_ids"`
	TargetRevenue     float64                         `json:"target_revenue"`
	AdjustmentPercent float64                         `json:"adjustment_percent"`
	Adjustments       []ReconciliationAdjustmentInput `json:"adjustments"`
}

type reconciliationItem struct {
	ID             string       `json:"id"`
	InvoiceID      string       `json:"invoice_id"`
	ProductID      string       `json:"product_id"`
	SKU            string       `json:"sku"`
	ProductName    string       `json:"product_name"`
	Quantity       int          `json:"quantity"`
	UnitPrice      float64      `json:"unit_price"`
	TaxRate        float64      `json:"tax_rate"`
	LineSubtotal   float64      `json:"line_subtotal"`
	TaxAmount      float64      `json:"tax_amount"`
	LineTotal      float64      `json:"line_total"`
	StockBucket    string       `json:"-"`
	InventoryLotID string       `json:"-"`
	LotNumber      string       `json:"-"`
	LotExpiresOn   sql.NullTime `json:"-"`
	LotReceivedAt  time.Time    `json:"-"`
	LotUnitCost    float64      `json:"-"`
	NewUnitPrice   float64      `json:"new_unit_price"`
	NewLineTotal   float64      `json:"new_line_total"`
	Variance       float64      `json:"variance_amount"`
	DeductBucket   string       `json:"deduct_stock_bucket,omitempty"`
	GhostStock     int          `json:"ghost_stock_available"`
	RealStock      int          `json:"real_stock_available"`
}

type reconciliationInvoice struct {
	ID                    string                `json:"id"`
	BranchID              string                `json:"branch_id"`
	BranchCode            string                `json:"branch_code"`
	BranchName            string                `json:"branch_name"`
	InvoiceNumber         string                `json:"invoice_number"`
	CustomerName          string                `json:"customer_name"`
	IssuedAt              time.Time             `json:"issued_at"`
	CreatedAt             time.Time             `json:"created_at"`
	TotalAmount           float64               `json:"total_amount"`
	PaymentMethod         string                `json:"payment_method"`
	RequestFullTaxInvoice bool                  `json:"request_full_tax_invoice"`
	SuppressionCandidate  bool                  `json:"suppression_candidate"`
	AdjustmentEligible    bool                  `json:"adjustment_eligible"`
	WillSuppress          bool                  `json:"will_suppress"`
	Items                 []*reconciliationItem `json:"items"`
	FinalTotal            float64               `json:"final_total"`
}

type reconciliationSource struct {
	PeriodStart      time.Time
	PeriodEnd        time.Time
	BranchIDs        []string
	Invoices         []*reconciliationInvoice
	InvoiceByID      map[string]*reconciliationInvoice
	ItemByID         map[string]*reconciliationItem
	OriginalRevenue  int64
	SuppressedAmount int64
	SourceHash       string
}

type reconciliationOverviewGroup struct {
	InvoiceCount int     `json:"invoice_count"`
	Revenue      float64 `json:"revenue"`
}

func reconciliationPeriod(month string) (time.Time, time.Time, error) {
	location := time.FixedZone("Asia/Bangkok", 7*60*60)
	start, err := time.ParseInLocation("2006-01", strings.TrimSpace(month), location)
	if err != nil {
		return time.Time{}, time.Time{}, platform.NewError(http.StatusBadRequest, "กรุณาเลือกเดือนในรูปแบบ YYYY-MM")
	}
	return start.UTC(), start.AddDate(0, 1, 0).UTC(), nil
}

func reconciliationRange(input ReconciliationInput) (time.Time, time.Time, string, string, error) {
	dateFrom := strings.TrimSpace(input.DateFrom)
	dateTo := strings.TrimSpace(input.DateTo)
	if dateFrom == "" && dateTo == "" {
		start, end, err := reconciliationPeriod(input.Month)
		if err != nil {
			return time.Time{}, time.Time{}, "", "", err
		}
		return start, end, platform.InBangkok(start).Format("2006-01-02"), platform.InBangkok(end.Add(-time.Second)).Format("2006-01-02"), nil
	}
	if dateFrom == "" || dateTo == "" {
		return time.Time{}, time.Time{}, "", "", platform.NewError(http.StatusBadRequest, "กรุณาเลือกวันเริ่มต้นและวันสิ้นสุด")
	}
	location := time.FixedZone("Asia/Bangkok", 7*60*60)
	start, err := time.ParseInLocation("2006-01-02", dateFrom, location)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", platform.NewError(http.StatusBadRequest, "วันที่เริ่มต้นไม่ถูกต้อง")
	}
	last, err := time.ParseInLocation("2006-01-02", dateTo, location)
	if err != nil {
		return time.Time{}, time.Time{}, "", "", platform.NewError(http.StatusBadRequest, "วันที่สิ้นสุดไม่ถูกต้อง")
	}
	if last.Before(start) {
		return time.Time{}, time.Time{}, "", "", platform.NewError(http.StatusBadRequest, "วันสิ้นสุดต้องไม่ก่อนวันเริ่มต้น")
	}
	return start.UTC(), last.AddDate(0, 0, 1).UTC(), dateFrom, dateTo, nil
}

func canonicalBranchIDs(values []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := uuid.Parse(value)
		if err != nil {
			return nil, platform.NewError(http.StatusBadRequest, "รหัสสาขาไม่ถูกต้อง")
		}
		value = parsed.String()
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}

func (s *Service) loadReconciliationSource(ctx context.Context, db platform.DBTX, input ReconciliationInput) (reconciliationSource, error) {
	periodStart, periodEnd, _, _, err := reconciliationRange(input)
	if err != nil {
		return reconciliationSource{}, err
	}
	branchIDs, err := canonicalBranchIDs(input.BranchIDs)
	if err != nil {
		return reconciliationSource{}, err
	}
	if len(branchIDs) == 0 {
		rows, queryErr := db.QueryContext(ctx, `SELECT id::text FROM branches WHERE active=TRUE AND sales_enabled=TRUE AND branch_type<>'main_warehouse' ORDER BY id`)
		if queryErr != nil {
			return reconciliationSource{}, queryErr
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return reconciliationSource{}, err
			}
			branchIDs = append(branchIDs, id)
		}
		if err := rows.Close(); err != nil {
			return reconciliationSource{}, err
		}
	}
	if len(branchIDs) == 0 {
		return reconciliationSource{}, platform.NewError(http.StatusBadRequest, "ไม่พบสาขาที่เปิดขาย")
	}
	var validBranchCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM branches WHERE id=ANY($1::uuid[]) AND active=TRUE AND sales_enabled=TRUE AND branch_type<>'main_warehouse'`, pq.Array(branchIDs)).Scan(&validBranchCount); err != nil {
		return reconciliationSource{}, err
	}
	if validBranchCount != len(branchIDs) {
		return reconciliationSource{}, platform.NewError(http.StatusBadRequest, "มีสาขาที่เลือกไม่ถูกต้องหรือถูกปิดใช้งาน")
	}

	source := reconciliationSource{
		PeriodStart: periodStart, PeriodEnd: periodEnd, BranchIDs: branchIDs,
		Invoices: []*reconciliationInvoice{}, InvoiceByID: map[string]*reconciliationInvoice{},
		ItemByID: map[string]*reconciliationItem{},
	}
	rows, err := db.QueryContext(ctx, `
		SELECT i.id::text,i.branch_id::text,b.code,b.name,i.invoice_number,i.customer_name,
		       i.issued_at,i.created_at,i.total_amount,i.payment_method,
		       i.request_full_tax_invoice
		FROM invoices i
		INNER JOIN branches b ON b.id=i.branch_id
		WHERE i.deleted_at IS NULL
		  AND i.invoice_status='issued'
		  AND i.payment_status='paid'
		  AND i.created_at >= $1 AND i.created_at < $2
		  AND i.branch_id=ANY($3::uuid[])
		ORDER BY i.created_at,i.invoice_number,i.id
	`, periodStart, periodEnd, pq.Array(branchIDs))
	if err != nil {
		return reconciliationSource{}, err
	}
	invoiceIDs := []string{}
	for rows.Next() {
		invoice := &reconciliationInvoice{Items: []*reconciliationItem{}}
		if err := rows.Scan(&invoice.ID, &invoice.BranchID, &invoice.BranchCode, &invoice.BranchName,
			&invoice.InvoiceNumber, &invoice.CustomerName, &invoice.IssuedAt, &invoice.CreatedAt, &invoice.TotalAmount,
			&invoice.PaymentMethod, &invoice.RequestFullTaxInvoice); err != nil {
			rows.Close()
			return reconciliationSource{}, err
		}
		invoice.SuppressionCandidate = invoice.PaymentMethod == "cash" && !invoice.RequestFullTaxInvoice
		invoice.AdjustmentEligible = invoice.SuppressionCandidate
		invoice.WillSuppress = invoice.SuppressionCandidate
		invoice.FinalTotal = invoice.TotalAmount
		source.OriginalRevenue += centsFromFloat(invoice.TotalAmount)
		if invoice.SuppressionCandidate {
			source.SuppressedAmount += centsFromFloat(invoice.TotalAmount)
		}
		source.Invoices = append(source.Invoices, invoice)
		source.InvoiceByID[invoice.ID] = invoice
		invoiceIDs = append(invoiceIDs, invoice.ID)
	}
	if err := rows.Close(); err != nil {
		return reconciliationSource{}, err
	}
	if len(invoiceIDs) == 0 {
		return source, nil
	}

	itemRows, err := db.QueryContext(ctx, `
		SELECT ii.id::text,ii.invoice_id::text,ii.product_id::text,p.sku,ii.actual_product_name,
		       ii.quantity,ii.unit_price,ii.tax_rate,ii.line_subtotal,ii.tax_amount,ii.line_total,
		       ii.stock_bucket,COALESCE(ii.inventory_lot_id::text,''),COALESCE(lot.lot_number,''),
		       lot.expires_on,COALESCE(lot.received_at,i.created_at),COALESCE(lot.unit_cost,ii.cost_snapshot),
		       COALESCE(inv.qty_real,0),COALESCE(warehouse_inventory.qty_ghost,0)
		FROM invoice_items ii
		INNER JOIN products p ON p.id=ii.product_id
		INNER JOIN invoices i ON i.id=ii.invoice_id
		LEFT JOIN inventory_lots lot ON lot.id=ii.inventory_lot_id
		LEFT JOIN inventory inv ON inv.branch_id=i.branch_id AND inv.product_id=ii.product_id
		LEFT JOIN branches warehouse ON warehouse.branch_type='main_warehouse' AND warehouse.active=TRUE
		LEFT JOIN inventory warehouse_inventory ON warehouse_inventory.branch_id=warehouse.id AND warehouse_inventory.product_id=ii.product_id
		WHERE ii.invoice_id=ANY($1::uuid[])
		ORDER BY i.created_at,ii.created_at,ii.id
	`, pq.Array(invoiceIDs))
	if err != nil {
		return reconciliationSource{}, err
	}
	for itemRows.Next() {
		item := &reconciliationItem{}
		if err := itemRows.Scan(&item.ID, &item.InvoiceID, &item.ProductID, &item.SKU,
			&item.ProductName, &item.Quantity, &item.UnitPrice, &item.TaxRate,
			&item.LineSubtotal, &item.TaxAmount, &item.LineTotal, &item.StockBucket,
			&item.InventoryLotID, &item.LotNumber, &item.LotExpiresOn, &item.LotReceivedAt, &item.LotUnitCost,
			&item.RealStock, &item.GhostStock); err != nil {
			itemRows.Close()
			return reconciliationSource{}, err
		}
		item.NewUnitPrice = item.UnitPrice
		item.NewLineTotal = item.LineTotal
		source.ItemByID[item.ID] = item
		if invoice := source.InvoiceByID[item.InvoiceID]; invoice != nil {
			invoice.Items = append(invoice.Items, item)
		}
	}
	if err := itemRows.Close(); err != nil {
		return reconciliationSource{}, err
	}

	hash := sha256.New()
	for _, invoice := range source.Invoices {
		_, _ = fmt.Fprintf(hash, "%s|%s|%.2f|%s|%t|%s;", invoice.ID, invoice.InvoiceNumber,
			invoice.TotalAmount, invoice.PaymentMethod, invoice.RequestFullTaxInvoice,
			invoice.IssuedAt.UTC().Format(time.RFC3339Nano))
		for _, item := range invoice.Items {
			_, _ = fmt.Fprintf(hash, "%s|%s|%d|%.2f|%.2f|%d|%d;", item.ID, item.ProductID,
				item.Quantity, item.UnitPrice, item.LineTotal, item.RealStock, item.GhostStock)
		}
	}
	source.SourceHash = hex.EncodeToString(hash.Sum(nil))
	return source, nil
}

func reconciliationOverviewGroups(invoices []*reconciliationInvoice) map[string]reconciliationOverviewGroup {
	groupCents := map[string]int64{}
	groupCounts := map[string]int{}
	for _, invoice := range invoices {
		key := "unclassified"
		switch {
		case invoice.SuppressionCandidate:
			key = "cash_suppressed"
		case invoice.PaymentMethod == "cash" && invoice.RequestFullTaxInvoice:
			key = "cash_full_tax"
		case invoice.PaymentMethod == "bank_transfer":
			key = "bank_transfer"
		case invoice.PaymentMethod == "mixed":
			key = "mixed"
		}
		groupCounts[key]++
		groupCents[key] += centsFromFloat(invoice.TotalAmount)
	}

	result := map[string]reconciliationOverviewGroup{}
	for _, key := range []string{"cash_suppressed", "cash_full_tax", "bank_transfer", "mixed", "unclassified"} {
		result[key] = reconciliationOverviewGroup{
			InvoiceCount: groupCounts[key],
			Revenue:      centsToFloat(groupCents[key]),
		}
	}
	// The same cash-only/no-full-tax pool can either be hidden or retained for
	// automatic price adjustment. Expose it under both semantic keys so clients
	// do not have to reconstruct eligibility from payment fields.
	result["cash_adjustable"] = result["cash_suppressed"]
	return result
}

func applyReconciliationAdjustments(source *reconciliationSource, adjustments []ReconciliationAdjustmentInput) (int64, int, error) {
	seen := map[string]bool{}
	invoiceReduction := map[string]int64{}
	adjustmentReduction := int64(0)
	for _, adjustment := range adjustments {
		itemID := strings.TrimSpace(adjustment.InvoiceItemID)
		if itemID == "" || seen[itemID] {
			return 0, 0, platform.NewError(http.StatusBadRequest, "รายการปรับราคาซ้ำหรือไม่สมบูรณ์")
		}
		seen[itemID] = true
		item := source.ItemByID[itemID]
		if item == nil {
			return 0, 0, platform.NewError(http.StatusBadRequest, "ไม่พบรายการสินค้าที่เลือกปรับราคา")
		}
		invoice := source.InvoiceByID[item.InvoiceID]
		if invoice == nil || !invoice.AdjustmentEligible {
			return 0, 0, platform.NewError(http.StatusBadRequest, "ปรับราคาได้เฉพาะใบขายเงินสดที่ยังคงอยู่หลังการซ่อนบิล")
		}
		newCents, valid := checkoutMoney(adjustment.NewUnitPrice)
		if !valid {
			return 0, 0, platform.NewError(http.StatusBadRequest, "ราคาที่ปรับต้องไม่ติดลบและมีทศนิยมไม่เกิน 2 ตำแหน่ง")
		}
		oldCents := centsFromFloat(item.UnitPrice)
		if newCents >= oldCents {
			return 0, 0, platform.NewError(http.StatusBadRequest, "ราคาที่ปรับต้องต่ำกว่าราคาเดิม")
		}
		newSubtotal := newCents * int64(item.Quantity)
		taxBasisPoints := centsFromFloat(item.TaxRate)
		newTax := roundedRatio(newSubtotal*taxBasisPoints, 10000)
		newTotal := newSubtotal + newTax
		oldTotal := centsFromFloat(item.LineTotal)
		reduction := oldTotal - newTotal
		if reduction <= 0 {
			return 0, 0, platform.NewError(http.StatusBadRequest, "ราคาที่ปรับไม่ทำให้ยอดขายลดลง")
		}
		item.NewUnitPrice = centsToFloat(newCents)
		item.NewLineTotal = centsToFloat(newTotal)
		item.Variance = centsToFloat((oldCents - newCents) * int64(item.Quantity))
		item.DeductBucket = "real"
		if newCents == 0 {
			item.DeductBucket = "ghost"
		}
		invoiceReduction[invoice.ID] += reduction
		adjustmentReduction += reduction
	}
	for invoiceID, reduction := range invoiceReduction {
		invoice := source.InvoiceByID[invoiceID]
		invoice.FinalTotal = centsToFloat(centsFromFloat(invoice.TotalAmount) - reduction)
	}
	return adjustmentReduction, len(seen), nil
}

type automaticInvoicePlan struct {
	invoice      *reconciliationInvoice
	original     int64
	maximumCut   int64
	willSuppress bool
}

func adjustmentBasisPoints(percent float64) (int64, bool) {
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > 5 {
		return 0, false
	}
	value := math.Round(percent * 100)
	if math.Abs(percent*100-value) > 0.000001 {
		return 0, false
	}
	return int64(value), true
}

func ceilRatio(numerator, denominator int64) int64 {
	if numerator <= 0 {
		return 0
	}
	return (numerator + denominator - 1) / denominator
}

func reconciliationLineTotal(item *reconciliationItem, unitPrice int64) int64 {
	oldUnitPrice := centsFromFloat(item.UnitPrice)
	if unitPrice == oldUnitPrice {
		return centsFromFloat(item.LineTotal)
	}
	subtotal := unitPrice * int64(item.Quantity)
	tax := roundedRatio(subtotal*centsFromFloat(item.TaxRate), 10000)
	return subtotal + tax
}

func minimumUnitPrice(item *reconciliationItem, maximumPercentBasisPoints int64) int64 {
	oldUnitPrice := centsFromFloat(item.UnitPrice)
	return ceilRatio(oldUnitPrice*(10000-maximumPercentBasisPoints), 10000)
}

func maximumItemReduction(item *reconciliationItem, maximumPercentBasisPoints int64) int64 {
	oldTotal := centsFromFloat(item.LineTotal)
	minimumTotal := reconciliationLineTotal(item, minimumUnitPrice(item, maximumPercentBasisPoints))
	if minimumTotal >= oldTotal {
		return 0
	}
	return oldTotal - minimumTotal
}

// applyAutomaticReconciliationPlan splits the cash-only/no-full-tax pool into
// mutually exclusive actions. Whole invoices are hidden first when the target
// needs more reduction than the retained lines can absorb under the percentage
// cap. The remaining gap is then distributed over retained item prices without
// allowing any unit price to fall by more than the requested percentage.
func applyAutomaticReconciliationPlan(source *reconciliationSource, target int64, percent float64) (int64, int64, int, error) {
	maximumPercentBasisPoints, valid := adjustmentBasisPoints(percent)
	if !valid {
		return 0, 0, 0, platform.NewError(http.StatusBadRequest, "เปอร์เซ็นต์ปรับราคาต้องอยู่ระหว่าง 0 ถึง 5 และมีทศนิยมไม่เกิน 2 ตำแหน่ง")
	}
	if target > source.OriginalRevenue {
		return 0, 0, 0, platform.NewError(http.StatusBadRequest, "ยอดเป้าหมายต้องไม่สูงกว่ายอดขายทั้งหมด")
	}

	plans := []*automaticInvoicePlan{}
	totalMaximumCut := int64(0)
	for _, invoice := range source.Invoices {
		invoice.WillSuppress = false
		invoice.FinalTotal = invoice.TotalAmount
		for _, item := range invoice.Items {
			item.NewUnitPrice = item.UnitPrice
			item.NewLineTotal = item.LineTotal
			item.Variance = 0
			item.DeductBucket = ""
		}
		if !invoice.AdjustmentEligible {
			continue
		}
		plan := &automaticInvoicePlan{invoice: invoice, original: centsFromFloat(invoice.TotalAmount)}
		for _, item := range invoice.Items {
			plan.maximumCut += maximumItemReduction(item, maximumPercentBasisPoints)
		}
		totalMaximumCut += plan.maximumCut
		plans = append(plans, plan)
	}

	reductionNeeded := source.OriginalRevenue - target
	remainingReduction := reductionNeeded
	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].original == plans[j].original {
			if plans[i].invoice.IssuedAt.Equal(plans[j].invoice.IssuedAt) {
				return plans[i].invoice.ID < plans[j].invoice.ID
			}
			return plans[i].invoice.IssuedAt.Before(plans[j].invoice.IssuedAt)
		}
		return plans[i].original > plans[j].original
	})

	suppressedAmount := int64(0)
	remainingMaximumCut := totalMaximumCut
	for remainingReduction > remainingMaximumCut {
		selected := -1
		for index, plan := range plans {
			if plan.willSuppress || plan.original > remainingReduction {
				continue
			}
			selected = index
			break
		}
		if selected < 0 {
			break
		}
		plan := plans[selected]
		plan.willSuppress = true
		plan.invoice.WillSuppress = true
		suppressedAmount += plan.original
		remainingReduction -= plan.original
		remainingMaximumCut -= plan.maximumCut
	}

	adjustmentReduction := int64(0)
	adjustedItemCount := 0
	for _, plan := range plans {
		if plan.willSuppress || remainingReduction <= 0 {
			continue
		}
		invoiceReduction := int64(0)
		for _, item := range plan.invoice.Items {
			if remainingReduction <= 0 {
				break
			}
			oldUnitPrice := centsFromFloat(item.UnitPrice)
			minimumPrice := minimumUnitPrice(item, maximumPercentBasisPoints)
			maximumReduction := maximumItemReduction(item, maximumPercentBasisPoints)
			if maximumReduction <= 0 {
				continue
			}

			newUnitPrice := minimumPrice
			if maximumReduction > remainingReduction {
				low, high := minimumPrice, oldUnitPrice
				for low < high {
					mid := low + (high-low)/2
					reduction := centsFromFloat(item.LineTotal) - reconciliationLineTotal(item, mid)
					if reduction > remainingReduction {
						low = mid + 1
					} else {
						high = mid
					}
				}
				newUnitPrice = low
			}
			newLineTotal := reconciliationLineTotal(item, newUnitPrice)
			reduction := centsFromFloat(item.LineTotal) - newLineTotal
			if reduction <= 0 {
				continue
			}
			item.NewUnitPrice = centsToFloat(newUnitPrice)
			item.NewLineTotal = centsToFloat(newLineTotal)
			item.Variance = centsToFloat((oldUnitPrice - newUnitPrice) * int64(item.Quantity))
			item.DeductBucket = "real"
			invoiceReduction += reduction
			adjustmentReduction += reduction
			remainingReduction -= reduction
			adjustedItemCount++
		}
		plan.invoice.FinalTotal = centsToFloat(plan.original - invoiceReduction)
	}

	return suppressedAmount, adjustmentReduction, adjustedItemCount, nil
}

func checkoutMoney(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false
	}
	rounded := math.Round(value * 100)
	if math.Abs(value*100-rounded) > 0.000001 {
		return 0, false
	}
	return int64(rounded), true
}

func (s *Service) ReconciliationOverview(ctx context.Context, user platform.AuthUser, input ReconciliationInput) (map[string]any, error) {
	if user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	source, err := s.loadReconciliationSource(ctx, s.db, ReconciliationInput{Month: input.Month, DateFrom: input.DateFrom, DateTo: input.DateTo, BranchIDs: input.BranchIDs})
	if err != nil {
		return nil, err
	}
	suppressedCount := 0
	remainingCashCount := 0
	nonCashCount := 0
	for _, invoice := range source.Invoices {
		if invoice.SuppressionCandidate {
			suppressedCount++
		}
		if !invoice.AdjustmentEligible {
			nonCashCount++
		}
	}
	baseRevenue := source.OriginalRevenue - source.SuppressedAmount
	return map[string]any{
		"period_start":                 platform.InBangkok(source.PeriodStart).Format("2006-01-02"),
		"period_end":                   platform.InBangkok(source.PeriodEnd.Add(-time.Second)).Format("2006-01-02"),
		"branch_ids":                   source.BranchIDs,
		"original_revenue":             centsToFloat(source.OriginalRevenue),
		"suppressed_revenue":           centsToFloat(source.SuppressedAmount),
		"base_revenue":                 centsToFloat(baseRevenue),
		"invoice_count":                len(source.Invoices),
		"suppressed_invoice_count":     suppressedCount,
		"remaining_cash_invoice_count": remainingCashCount,
		"non_cash_invoice_count":       nonCashCount,
		"invoice_groups":               reconciliationOverviewGroups(source.Invoices),
		"maximum_adjustment_percent":   5,
		"legacy_target_ignored":        true,
		"reconciliation_mode":          "hide_all_cash_no_tax",
		"source_hash":                  source.SourceHash,
	}, nil
}

func (s *Service) PreviewReconciliation(ctx context.Context, user platform.AuthUser, input ReconciliationInput) (map[string]any, error) {
	if user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	source, err := s.loadReconciliationSource(ctx, s.db, input)
	if err != nil {
		return nil, err
	}
	suppressedAmount := source.SuppressedAmount
	adjustmentReduction := int64(0)
	adjustedCount := 0
	baseRevenue := source.OriginalRevenue - suppressedAmount
	finalRevenue := baseRevenue - adjustmentReduction
	suppressed := []*reconciliationInvoice{}
	remainingCash := []*reconciliationInvoice{}
	type ghostProjection struct {
		ProductID      string `json:"product_id"`
		ProductName    string `json:"product_name"`
		Quantity       int    `json:"quantity"`
		GhostBefore    int    `json:"warehouse_ghost_before"`
		GhostAfter     int    `json:"warehouse_ghost_after"`
		DeficitCreated int    `json:"deficit_created"`
	}
	projectionByProduct := map[string]*ghostProjection{}
	totalQuantity := 0
	for _, invoice := range source.Invoices {
		if invoice.SuppressionCandidate {
			invoice.WillSuppress = true
			suppressed = append(suppressed, invoice)
			for _, item := range invoice.Items {
				projection := projectionByProduct[item.ProductID]
				if projection == nil {
					projection = &ghostProjection{ProductID: item.ProductID, ProductName: item.ProductName, GhostBefore: item.GhostStock}
					projectionByProduct[item.ProductID] = projection
				}
				projection.Quantity += item.Quantity
				totalQuantity += item.Quantity
			}
		}
	}
	productProjection := make([]*ghostProjection, 0, len(projectionByProduct))
	ghostDeficit := 0
	for _, projection := range projectionByProduct {
		projection.GhostAfter = projection.GhostBefore - projection.Quantity
		if projection.GhostAfter < 0 {
			projection.DeficitCreated = -projection.GhostAfter
			ghostDeficit += projection.DeficitCreated
		}
		productProjection = append(productProjection, projection)
	}
	sort.Slice(productProjection, func(i, j int) bool {
		return productProjection[i].ProductName < productProjection[j].ProductName
	})
	return map[string]any{
		"period_start":                platform.InBangkok(source.PeriodStart).Format("2006-01-02"),
		"period_end":                  platform.InBangkok(source.PeriodEnd.Add(-time.Second)).Format("2006-01-02"),
		"branch_ids":                  source.BranchIDs,
		"original_revenue":            centsToFloat(source.OriginalRevenue),
		"suppressed_revenue":          centsToFloat(suppressedAmount),
		"base_revenue":                centsToFloat(baseRevenue),
		"adjustment_reduction":        centsToFloat(adjustmentReduction),
		"target_revenue":              input.TargetRevenue,
		"final_revenue":               centsToFloat(finalRevenue),
		"target_difference":           0,
		"target_matched":              true,
		"suppression_candidates":      suppressed,
		"remaining_cash_invoices":     remainingCash,
		"suppressed_invoice_count":    len(suppressed),
		"adjusted_item_count":         adjustedCount,
		"adjustment_percent":          input.AdjustmentPercent,
		"eligible_cash_invoice_count": len(suppressed) + len(remainingCash),
		"stock_projection": map[string]any{
			"branch_real_returned": totalQuantity, "warehouse_real_received": totalQuantity,
			"warehouse_ghost_deducted": totalQuantity, "ghost_deficit_created": ghostDeficit,
			"products": productProjection,
		},
		"legacy_target_ignored": true,
		"reconciliation_mode":   "hide_all_cash_no_tax",
		"source_hash":           source.SourceHash,
	}, nil
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func insertReconciliationLog(ctx context.Context, tx *sql.Tx, reconciliationID, logType string, user platform.AuthUser, values map[string]any) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO reconciliation_logs (
			id,reconciliation_id,log_type,invoice_id,invoice_item_id,branch_id,product_id,
			original_invoice_number,new_invoice_number,old_unit_price,new_unit_price,
			variance_amount,stock_bucket,stock_quantity,before_data,after_data,actor_id,
			adjustment_reason,movement_role,real_stock_deducted,ghost_stock_deducted,created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16::jsonb,$17,$18,$19,$20,$21,NOW())
	`, platform.MustUUID(), reconciliationID, logType,
		platform.NullUUID(stringPointer(stringValue(values["invoice_id"]))), platform.NullUUID(stringPointer(stringValue(values["invoice_item_id"]))),
		platform.NullUUID(stringPointer(stringValue(values["branch_id"]))), platform.NullUUID(stringPointer(stringValue(values["product_id"]))),
		stringValue(values["original_invoice_number"]), stringValue(values["new_invoice_number"]),
		nullableMoney(values["old_unit_price"]), nullableMoney(values["new_unit_price"]), moneyValue(values["variance_amount"]),
		nullableText(values["stock_bucket"]), intValue(values["stock_quantity"]),
		platform.MustJSON(mapValue(values["before_data"])), platform.MustJSON(mapValue(values["after_data"])), user.ID,
		stringValue(values["adjustment_reason"]), stringValue(values["movement_role"]),
		intValue(values["real_stock_deducted"]), intValue(values["ghost_stock_deducted"]))
	return err
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func moneyValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return platform.Round2(typed)
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func intValue(value any) int {
	if typed, ok := value.(int); ok {
		return typed
	}
	return 0
}

func nullableMoney(value any) any {
	if value == nil {
		return nil
	}
	return moneyValue(value)
}

func nullableText(value any) any {
	text := strings.TrimSpace(stringValue(value))
	if text == "" {
		return nil
	}
	return text
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func (s *Service) deductReconciliationStock(ctx context.Context, tx *sql.Tx, reconciliationID string, user platform.AuthUser, invoice *reconciliationInvoice, item *reconciliationItem, bucket string, movementType string) error {
	column := "qty_real"
	storageBucket := "real"
	if bucket == "ghost" {
		column = "qty_ghost"
		storageBucket = "ghost"
	}
	var available int
	if err := tx.QueryRowContext(ctx, `SELECT `+column+` FROM inventory WHERE branch_id=$1 AND product_id=$2 FOR UPDATE`, invoice.BranchID, item.ProductID).Scan(&available); err != nil {
		if err == sql.ErrNoRows {
			return platform.NewError(http.StatusConflict, "ไม่พบสต๊อกสำหรับสินค้า "+item.ProductName)
		}
		return err
	}
	if available < item.Quantity {
		return platform.NewError(http.StatusConflict, fmt.Sprintf("สต๊อก%sของ %s ไม่พอ ต้องการ %d มี %d", bucket, item.ProductName, item.Quantity, available))
	}
	allocations, err := stocklot.AllocateFEFO(ctx, tx, invoice.BranchID, item.ProductID, storageBucket, item.Quantity)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inventory SET `+column+`=`+column+`-$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, invoice.BranchID, item.ProductID, item.Quantity); err != nil {
		return err
	}
	movementID := platform.MustUUID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (
			id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,
			reference_type,reference_id,note,performed_by,created_at
		) VALUES ($1,$2,$3,$4,$5,$6,'month_end_reconciliation',$7,$8,$9,NOW())
	`, movementID, invoice.BranchID, item.ProductID, movementType, storageBucket, -item.Quantity,
		reconciliationID, "ปรับสต๊อกจากสรุปสิ้นเดือน "+invoice.InvoiceNumber, user.ID); err != nil {
		return err
	}
	if err := stocklot.AttachMovement(ctx, tx, movementID, allocations, -1); err != nil {
		return err
	}
	return insertReconciliationLog(ctx, tx, reconciliationID, "stock_deducted", user, map[string]any{
		"invoice_id": invoice.ID, "invoice_item_id": item.ID, "branch_id": invoice.BranchID,
		"product_id": item.ProductID, "original_invoice_number": invoice.InvoiceNumber,
		"stock_bucket": bucket, "stock_quantity": item.Quantity,
		"before_data": map[string]any{"available": available, "storage_bucket": storageBucket},
		"after_data":  map[string]any{"available": available - item.Quantity, "movement_id": movementID},
	})
}

func insertStockAdjustmentNote(ctx context.Context, tx *sql.Tx, user platform.AuthUser, reconciliationID, invoiceID, movementID, branchID, productID, stockType, reason string, quantity int) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO stock_adjustment_notes (
			id,branch_id,product_id,quantity,stock_type,reason,reference_invoice_id,
			reconciliation_id,inventory_movement_id,created_by,created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW())
	`, platform.MustUUID(), branchID, productID, quantity, stockType, reason, invoiceID, reconciliationID, movementID, user.ID)
	return err
}

func activeWarehouseID(ctx context.Context, tx *sql.Tx) (string, error) {
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE branch_type='main_warehouse' AND active=TRUE`).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return "", platform.NewError(http.StatusConflict, "ไม่พบโกดังหลักสำหรับสต๊อกผี")
		}
		return "", err
	}
	return id, nil
}

func restoreInvoiceRealLot(ctx context.Context, tx *sql.Tx, item *reconciliationItem) (stocklot.Lot, error) {
	if strings.TrimSpace(item.InventoryLotID) == "" {
		return stocklot.Lot{}, platform.NewError(http.StatusConflict, "ไม่พบ lot เดิมของ "+item.ProductName)
	}
	var lot stocklot.Lot
	if err := tx.QueryRowContext(ctx, `
		SELECT id::text,branch_id::text,product_id::text,stock_bucket,lot_number,expires_on,
		       received_quantity,remaining_quantity,unit_cost,received_at
		FROM inventory_lots WHERE id=$1 FOR UPDATE
	`, item.InventoryLotID).Scan(&lot.ID, &lot.BranchID, &lot.ProductID, &lot.StockBucket, &lot.LotNumber,
		&lot.ExpiresOn, &lot.ReceivedQuantity, &lot.RemainingQuantity, &lot.UnitCost, &lot.ReceivedAt); err != nil {
		if err == sql.ErrNoRows {
			return stocklot.Lot{}, platform.NewError(http.StatusConflict, "ไม่พบ lot เดิมของ "+item.ProductName)
		}
		return stocklot.Lot{}, err
	}
	if lot.ProductID != item.ProductID || lot.StockBucket != "real" || lot.RemainingQuantity+item.Quantity > lot.ReceivedQuantity {
		return stocklot.Lot{}, platform.NewError(http.StatusConflict, "ไม่สามารถย้อนสต๊อกจริงของ "+item.ProductName+" ได้")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inventory_lots SET remaining_quantity=remaining_quantity+$2,updated_at=NOW() WHERE id=$1`, lot.ID, item.Quantity); err != nil {
		return stocklot.Lot{}, err
	}
	lot.RemainingQuantity += item.Quantity
	return lot, nil
}

func (s *Service) reclassifySuppressedItem(ctx context.Context, tx *sql.Tx, reconciliationID, transferID, transferItemID, warehouseID string, user platform.AuthUser, invoice *reconciliationInvoice, item *reconciliationItem) error {
	const reason = "PRODUCT_RETURN_TO_WAREHOUSE"
	lot, err := restoreInvoiceRealLot(ctx, tx, item)
	if err != nil {
		return err
	}

	var branchRealBefore int
	if err := tx.QueryRowContext(ctx, `SELECT qty_real FROM inventory WHERE branch_id=$1 AND product_id=$2 FOR UPDATE`, invoice.BranchID, item.ProductID).Scan(&branchRealBefore); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inventory SET qty_real=qty_real+$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, invoice.BranchID, item.ProductID, item.Quantity); err != nil {
		return err
	}
	reversalMovementID := platform.MustUUID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at)
		VALUES ($1,$2,$3,'month_end_sale_source_reversal','real',$4,'month_end_reconciliation',$5,$6,$7,NOW())
	`, reversalMovementID, invoice.BranchID, item.ProductID, item.Quantity, reconciliationID, "ย้อนแหล่งตัดของบิล "+invoice.InvoiceNumber, user.ID); err != nil {
		return err
	}
	if err := stocklot.AttachMovement(ctx, tx, reversalMovementID, []stocklot.Allocation{{Lot: lot, Quantity: item.Quantity}}, 1); err != nil {
		return err
	}
	if err := insertReconciliationLog(ctx, tx, reconciliationID, "stock_reversed", user, map[string]any{
		"invoice_id": invoice.ID, "invoice_item_id": item.ID, "branch_id": invoice.BranchID, "product_id": item.ProductID,
		"original_invoice_number": invoice.InvoiceNumber, "stock_bucket": "real", "stock_quantity": item.Quantity,
		"adjustment_reason": reason, "movement_role": "sale_source_reversal",
		"before_data": map[string]any{"available": branchRealBefore, "lot_id": lot.ID},
		"after_data":  map[string]any{"available": branchRealBefore + item.Quantity, "movement_id": reversalMovementID},
	}); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE inventory_lots SET remaining_quantity=remaining_quantity-$2,updated_at=NOW() WHERE id=$1`, lot.ID, item.Quantity); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inventory SET qty_real=qty_real-$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, invoice.BranchID, item.ProductID, item.Quantity); err != nil {
		return err
	}
	dispatchMovementID := platform.MustUUID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at)
		VALUES ($1,$2,$3,'month_end_return_to_warehouse','real',$4,'transfer',$5,$6,$7,NOW())
	`, dispatchMovementID, invoice.BranchID, item.ProductID, -item.Quantity, transferID, reason, user.ID); err != nil {
		return err
	}
	if err := stocklot.AttachMovement(ctx, tx, dispatchMovementID, []stocklot.Allocation{{Lot: lot, Quantity: item.Quantity}}, -1); err != nil {
		return err
	}
	if err := insertStockAdjustmentNote(ctx, tx, user, reconciliationID, invoice.ID, dispatchMovementID, invoice.BranchID, item.ProductID, "REAL", reason, -item.Quantity); err != nil {
		return err
	}
	if err := insertReconciliationLog(ctx, tx, reconciliationID, "stock_deducted", user, map[string]any{
		"invoice_id": invoice.ID, "invoice_item_id": item.ID, "branch_id": invoice.BranchID, "product_id": item.ProductID,
		"original_invoice_number": invoice.InvoiceNumber, "stock_bucket": "real", "stock_quantity": item.Quantity,
		"adjustment_reason": reason, "movement_role": "branch_return_dispatch", "real_stock_deducted": item.Quantity,
		"before_data": map[string]any{"available": branchRealBefore + item.Quantity, "lot_id": lot.ID},
		"after_data":  map[string]any{"available": branchRealBefore, "movement_id": dispatchMovementID, "transfer_id": transferID},
	}); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory (id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at)
		VALUES ($1,$2,$3,0,0,NOW(),NOW()) ON CONFLICT(branch_id,product_id) DO NOTHING
	`, platform.MustUUID(), warehouseID, item.ProductID); err != nil {
		return err
	}
	var warehouseRealBefore, warehouseGhostBefore int
	if err := tx.QueryRowContext(ctx, `SELECT qty_real,qty_ghost FROM inventory WHERE branch_id=$1 AND product_id=$2 FOR UPDATE`, warehouseID, item.ProductID).Scan(&warehouseRealBefore, &warehouseGhostBefore); err != nil {
		return err
	}
	destinationLotID, err := stocklot.Create(ctx, tx, stocklot.Lot{
		BranchID: warehouseID, ProductID: item.ProductID, StockBucket: "real", LotNumber: lot.LotNumber,
		ExpiresOn: lot.ExpiresOn, ReceivedQuantity: item.Quantity, RemainingQuantity: item.Quantity,
		UnitCost: lot.UnitCost, ReceivedAt: time.Now().UTC(),
	}, "month_end_return", &transferID, &transferItemID, &lot.ID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inventory SET qty_real=qty_real+$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, warehouseID, item.ProductID, item.Quantity); err != nil {
		return err
	}
	receiveMovementID := platform.MustUUID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at)
		VALUES ($1,$2,$3,'month_end_return_received','real',$4,'transfer',$5,$6,$7,NOW())
	`, receiveMovementID, warehouseID, item.ProductID, item.Quantity, transferID, reason, user.ID); err != nil {
		return err
	}
	if err := stocklot.AttachMovement(ctx, tx, receiveMovementID, []stocklot.Allocation{{Lot: stocklot.Lot{ID: destinationLotID}, Quantity: item.Quantity}}, 1); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO transfer_item_lot_allocations (id,transfer_item_id,source_lot_id,destination_lot_id,dispatched_quantity,received_quantity,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$5,NOW(),NOW())
	`, platform.MustUUID(), transferItemID, lot.ID, destinationLotID, item.Quantity); err != nil {
		return err
	}
	if err := insertStockAdjustmentNote(ctx, tx, user, reconciliationID, invoice.ID, receiveMovementID, warehouseID, item.ProductID, "REAL", reason, item.Quantity); err != nil {
		return err
	}
	if err := insertReconciliationLog(ctx, tx, reconciliationID, "stock_received", user, map[string]any{
		"invoice_id": invoice.ID, "invoice_item_id": item.ID, "branch_id": warehouseID, "product_id": item.ProductID,
		"original_invoice_number": invoice.InvoiceNumber, "stock_bucket": "real", "stock_quantity": item.Quantity,
		"adjustment_reason": reason, "movement_role": "warehouse_return_receive",
		"before_data": map[string]any{"available": warehouseRealBefore},
		"after_data":  map[string]any{"available": warehouseRealBefore + item.Quantity, "movement_id": receiveMovementID, "transfer_id": transferID},
	}); err != nil {
		return err
	}

	var sellableGhostLots int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(remaining_quantity),0)::integer
		FROM inventory_lots
		WHERE branch_id=$1 AND product_id=$2 AND stock_bucket='ghost'
		  AND remaining_quantity>0
		  AND (expires_on IS NULL OR expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)
	`, warehouseID, item.ProductID).Scan(&sellableGhostLots); err != nil {
		return err
	}
	allocatable := warehouseGhostBefore
	if allocatable < 0 {
		allocatable = 0
	}
	if allocatable > sellableGhostLots {
		allocatable = sellableGhostLots
	}
	if allocatable > item.Quantity {
		allocatable = item.Quantity
	}
	ghostAllocations := []stocklot.Allocation{}
	if allocatable > 0 {
		ghostAllocations, err = stocklot.AllocateFEFO(ctx, tx, warehouseID, item.ProductID, "ghost", allocatable)
		if err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE inventory SET qty_ghost=qty_ghost-$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, warehouseID, item.ProductID, item.Quantity); err != nil {
		return err
	}
	ghostMovementID := platform.MustUUID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at)
		VALUES ($1,$2,$3,'month_end_ghost_deduction','ghost',$4,'month_end_reconciliation',$5,$6,$7,NOW())
	`, ghostMovementID, warehouseID, item.ProductID, -item.Quantity, reconciliationID, "ตัดสต๊อกผีแทนบิล "+invoice.InvoiceNumber, user.ID); err != nil {
		return err
	}
	if err := stocklot.AttachMovement(ctx, tx, ghostMovementID, ghostAllocations, -1); err != nil {
		return err
	}
	deficit := item.Quantity - allocatable
	if deficit > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_ghost_deficits (id,branch_id,product_id,quantity,reconciliation_id,invoice_id,invoice_item_id,inventory_movement_id,created_by,created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
		`, platform.MustUUID(), warehouseID, item.ProductID, deficit, reconciliationID, invoice.ID, item.ID, ghostMovementID, user.ID); err != nil {
			return err
		}
		if err := insertReconciliationLog(ctx, tx, reconciliationID, "stock_deficit", user, map[string]any{
			"invoice_id": invoice.ID, "invoice_item_id": item.ID, "branch_id": warehouseID, "product_id": item.ProductID,
			"original_invoice_number": invoice.InvoiceNumber, "stock_bucket": "ghost", "stock_quantity": deficit,
			"adjustment_reason": reason, "movement_role": "warehouse_ghost_deficit",
			"before_data": map[string]any{"available": warehouseGhostBefore, "lot_allocated": allocatable},
			"after_data":  map[string]any{"available": warehouseGhostBefore - item.Quantity, "movement_id": ghostMovementID, "deficit": deficit},
		}); err != nil {
			return err
		}
	}
	if err := insertStockAdjustmentNote(ctx, tx, user, reconciliationID, invoice.ID, ghostMovementID, warehouseID, item.ProductID, "GHOST", reason, -item.Quantity); err != nil {
		return err
	}
	if err := insertReconciliationLog(ctx, tx, reconciliationID, "stock_deducted", user, map[string]any{
		"invoice_id": invoice.ID, "invoice_item_id": item.ID, "branch_id": warehouseID, "product_id": item.ProductID,
		"original_invoice_number": invoice.InvoiceNumber, "stock_bucket": "ghost", "stock_quantity": item.Quantity,
		"adjustment_reason": reason, "movement_role": "invoice_ghost_source", "ghost_stock_deducted": item.Quantity,
		"before_data": map[string]any{"available": warehouseGhostBefore, "lot_allocated": allocatable},
		"after_data":  map[string]any{"available": warehouseGhostBefore - item.Quantity, "movement_id": ghostMovementID, "deficit": deficit},
	}); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE reconciliation_item_snapshots
		SET effective_stock_bucket='ghost'
		WHERE reconciliation_id=$1 AND invoice_item_id=$2
	`, reconciliationID, item.ID)
	return err
}

func compactedInvoiceNumber(original string, sequence int) string {
	if len(original) >= 5 {
		suffix := original[len(original)-5:]
		if _, err := strconv.Atoi(suffix); err == nil {
			return original[:len(original)-5] + fmt.Sprintf("%05d", sequence)
		}
	}
	return fmt.Sprintf("%s-%05d", original, sequence)
}

// snapshotReconciliationState persists the exact invoice and item state before
// any bill is hidden, renumbered, repriced, or deducted from Ghost Stock. The
// comparison report reads these normalized snapshots instead of reconstructing
// historical state from today's invoice rows.
func snapshotReconciliationState(ctx context.Context, tx *sql.Tx, reconciliationID string, periodStart, periodEnd time.Time, branchIDs []string) error {
	if _, err := tx.ExecContext(ctx, `
		WITH payments AS (
			SELECT invoice_id,
			       COUNT(*) AS payment_count,
			       BOOL_AND(payment_type='cash') AS cash_only,
			       BOOL_AND(payment_type='bank_transfer') AS transfer_only,
			       SUM(amount) AS paid_amount
			FROM invoice_payments
			GROUP BY invoice_id
		)
		INSERT INTO reconciliation_invoice_snapshots (
			reconciliation_id,invoice_id,branch_id,original_invoice_number,issued_at,
			payment_method,request_full_tax_invoice,original_subtotal,original_tax_amount,
			original_total_amount,original_invoice_data,invoice_created_at,created_at
		)
		SELECT $1,i.id,i.branch_id,i.invoice_number,i.issued_at,
		       CASE
		         WHEN i.payment_status<>'paid' OR COALESCE(payments.payment_count,0)=0
		              OR COALESCE(payments.paid_amount,0)<i.total_amount THEN 'unpaid'
		         WHEN payments.cash_only THEN 'cash'
		         WHEN payments.transfer_only THEN 'bank_transfer'
		         ELSE 'mixed'
		       END,
		       i.request_full_tax_invoice,i.subtotal,i.tax_amount,i.total_amount,
		       jsonb_build_object(
		         'invoice_number',i.invoice_number,
		         'subtotal',i.subtotal,
		         'tax_amount',i.tax_amount,
		         'total_amount',i.total_amount,
		         'invoice_status',i.invoice_status,
		         'payment_status',i.payment_status,
		         'request_full_tax_invoice',i.request_full_tax_invoice
		       ),i.created_at,NOW()
		FROM invoices i
		LEFT JOIN payments ON payments.invoice_id=i.id
		WHERE i.deleted_at IS NULL
		  AND i.invoice_status='issued'
		  AND i.created_at >= $2 AND i.created_at < $3
		  AND i.branch_id=ANY($4::uuid[])
		ON CONFLICT DO NOTHING
	`, reconciliationID, periodStart, periodEnd, pq.Array(branchIDs)); err != nil {
		return err
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO reconciliation_item_snapshots (
			reconciliation_id,invoice_item_id,invoice_id,product_id,product_name,
			quantity,original_unit_price,original_line_subtotal,original_tax_amount,
			original_line_total,original_stock_bucket,effective_stock_bucket,original_item_data,created_at
		)
		SELECT $1,ii.id,ii.invoice_id,ii.product_id,ii.actual_product_name,
		       ii.quantity,ii.unit_price,ii.line_subtotal,ii.tax_amount,ii.line_total,
		       ii.stock_bucket,ii.stock_bucket,
		       jsonb_build_object(
		         'product_name',ii.actual_product_name,
		         'quantity',ii.quantity,
		         'unit_price',ii.unit_price,
		         'line_subtotal',ii.line_subtotal,
		         'tax_amount',ii.tax_amount,
		         'line_total',ii.line_total,
		         'stock_bucket',ii.stock_bucket
		       ),NOW()
		FROM reconciliation_invoice_snapshots ris
		INNER JOIN invoice_items ii ON ii.invoice_id=ris.invoice_id
		WHERE ris.reconciliation_id=$1
		ON CONFLICT DO NOTHING
	`, reconciliationID)
	return err
}

func (s *Service) FinalizeReconciliation(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input ReconciliationInput) (map[string]any, error) {
	if user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	reconciliationID := platform.MustUUID()
	reconciliationNumber := platform.GenerateReadableCode("MER")
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		periodStart, periodEnd, periodStartDate, periodEndDate, err := reconciliationRange(input)
		if err != nil {
			return err
		}
		branchIDs, err := canonicalBranchIDs(input.BranchIDs)
		if err != nil {
			return err
		}
		lockKey := "month-end-reconciliation:" + periodStartDate + ":" + periodEndDate + ":" + strings.Join(branchIDs, ",")
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
			return err
		}
		source, err := s.loadReconciliationSource(ctx, tx, input)
		if err != nil {
			return err
		}
		if len(branchIDs) == 0 {
			branchIDs = source.BranchIDs
		}
		if len(branchIDs) == 0 {
			return platform.NewError(http.StatusBadRequest, "กรุณาเลือกอย่างน้อยหนึ่งสาขาขาย")
		}
		if _, err := tx.ExecContext(ctx, `
			SELECT id FROM invoices
			WHERE deleted_at IS NULL AND created_at >= $1 AND created_at < $2
			  AND branch_id=ANY($3::uuid[])
			ORDER BY id FOR UPDATE
		`, periodStart, periodEnd, pq.Array(branchIDs)); err != nil {
			return err
		}
		// Reload after locking so preview and mutation use the same rows.
		source, err = s.loadReconciliationSource(ctx, tx, ReconciliationInput{
			Month: input.Month, DateFrom: input.DateFrom, DateTo: input.DateTo, BranchIDs: branchIDs,
			TargetRevenue: input.TargetRevenue, AdjustmentPercent: input.AdjustmentPercent,
		})
		if err != nil {
			return err
		}
		var existing bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM month_end_reconciliation_scopes scope
				WHERE scope.branch_id=ANY($1::uuid[])
				  AND daterange(scope.date_from,scope.date_to,'[]') && daterange($2::date,$3::date,'[]')
			)
		`, pq.Array(branchIDs), periodStartDate, periodEndDate).Scan(&existing); err != nil {
			return err
		}
		if existing {
			return platform.NewError(http.StatusConflict, "ช่วงวันที่นี้ทับซ้อนกับรอบที่สรุปแล้ว")
		}
		suppressedAmount := source.SuppressedAmount
		adjustmentReduction := int64(0)
		adjustedCount := 0
		baseRevenue := source.OriginalRevenue - suppressedAmount
		finalRevenue := baseRevenue - adjustmentReduction
		suppressedCount := 0
		for _, invoice := range source.Invoices {
			if invoice.SuppressionCandidate {
				invoice.WillSuppress = true
				suppressedCount++
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO month_end_reconciliations (
				id,reconciliation_number,period_start,period_end,branch_ids,target_revenue,
				original_revenue,suppressed_revenue,adjustment_reduction,final_revenue,
				suppressed_invoice_count,adjusted_item_count,adjustment_percent,source_hash,finalized_by,
				reconciliation_mode,legacy_target_ignored,finalized_at,created_at
			) VALUES ($1,$2,$3,$4,$5::uuid[],$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'hide_all_cash_no_tax',TRUE,NOW(),NOW())
		`, reconciliationID, reconciliationNumber, periodStartDate, periodEndDate, pq.Array(branchIDs), centsToFloat(finalRevenue),
			centsToFloat(source.OriginalRevenue), centsToFloat(suppressedAmount), centsToFloat(adjustmentReduction),
			centsToFloat(finalRevenue), suppressedCount, adjustedCount, 0, source.SourceHash, user.ID); err != nil {
			return err
		}
		for _, branchID := range branchIDs {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO month_end_reconciliation_scopes (reconciliation_id,branch_id,date_from,date_to,created_at)
				VALUES ($1,$2,$3,$4,NOW())
			`, reconciliationID, branchID, periodStartDate, periodEndDate); err != nil {
				return err
			}
		}
		if err := snapshotReconciliationState(ctx, tx, reconciliationID, periodStart, periodEnd, branchIDs); err != nil {
			return err
		}
		warehouseID, err := activeWarehouseID(ctx, tx)
		if err != nil {
			return err
		}

		for _, invoice := range source.Invoices {
			if !invoice.WillSuppress {
				continue
			}
			transferID := platform.MustUUID()
			transferCode := platform.GenerateReadableCode("MER-RTN")
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO transfers (
					id,transfer_code,source_branch_id,destination_branch_id,status,request_note,
					requested_by,dispatched_by,received_by,pickup_name,courier_name,
					requested_at,dispatched_at,received_at,created_at,updated_at,
					reconciliation_id,reference_invoice_id,system_generated
				) VALUES ($1,$2,$3,$4,'completed','PRODUCT_RETURN_TO_WAREHOUSE',$5,$5,$5,'ระบบสรุปสิ้นเดือน','ระบบสรุปสิ้นเดือน',NOW(),NOW(),NOW(),NOW(),NOW(),$6,$7,TRUE)
			`, transferID, transferCode, invoice.BranchID, warehouseID, user.ID, reconciliationID, invoice.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO transfer_events (id,transfer_id,status,note,actor_id,event_at)
				VALUES ($1,$2,'completed','PRODUCT_RETURN_TO_WAREHOUSE',$3,NOW())
			`, platform.MustUUID(), transferID, user.ID); err != nil {
				return err
			}
			for _, item := range invoice.Items {
				if item.StockBucket != "real" {
					return platform.NewError(http.StatusConflict, "บิลที่จะสรุปต้องมีแหล่งตัดเดิมเป็นสต๊อกจริง")
				}
				transferItemID := platform.MustUUID()
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO transfer_items (id,transfer_id,product_id,quantity,stock_bucket,received_quantity,discrepancy_note,created_at)
					VALUES ($1,$2,$3,$4,'real',$4,'',NOW())
				`, transferItemID, transferID, item.ProductID, item.Quantity); err != nil {
					return err
				}
				if err := s.reclassifySuppressedItem(ctx, tx, reconciliationID, transferID, transferItemID, warehouseID, user, invoice, item); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE invoices SET original_invoice_number=COALESCE(original_invoice_number,invoice_number),deleted_at=NOW(),hidden_by_id=$2,updated_at=NOW() WHERE id=$1`, invoice.ID, user.ID); err != nil {
				return err
			}
			if err := insertReconciliationLog(ctx, tx, reconciliationID, "invoice_suppressed", user, map[string]any{
				"invoice_id": invoice.ID, "branch_id": invoice.BranchID,
				"original_invoice_number": invoice.InvoiceNumber,
				"variance_amount":         invoice.TotalAmount,
				"adjustment_reason":       "PRODUCT_RETURN_TO_WAREHOUSE",
				"movement_role":           "invoice_hidden",
				"before_data":             map[string]any{"deleted_at": nil, "request_full_tax_invoice": false, "payment_method": "cash", "total_amount": invoice.TotalAmount},
				"after_data":              map[string]any{"hidden": true, "hidden_by_id": user.ID, "transfer_id": transferID, "effective_stock_bucket": "ghost"},
			}); err != nil {
				return err
			}
		}

		// Compact every surviving number in chronological order, separately per
		// branch. A temporary value avoids unique-index collisions while numbers
		// move into gaps left by hidden invoices.
		type numberRow struct{ id, branchID, oldNumber string }
		numberRows, err := tx.QueryContext(ctx, `SELECT id::text,branch_id::text,invoice_number FROM invoices WHERE deleted_at IS NULL AND invoice_status='issued' AND created_at >= $1 AND created_at < $2 AND branch_id=ANY($3::uuid[]) ORDER BY branch_id,created_at,id FOR UPDATE`, periodStart, periodEnd, pq.Array(branchIDs))
		if err != nil {
			return err
		}
		numbers := []numberRow{}
		for numberRows.Next() {
			var row numberRow
			if err := numberRows.Scan(&row.id, &row.branchID, &row.oldNumber); err != nil {
				numberRows.Close()
				return err
			}
			numbers = append(numbers, row)
		}
		if err := numberRows.Close(); err != nil {
			return err
		}
		for index, row := range numbers {
			if _, err := tx.ExecContext(ctx, `UPDATE invoices SET original_invoice_number=COALESCE(original_invoice_number,invoice_number),invoice_number=$2 WHERE id=$1`, row.id, fmt.Sprintf("__MER_%s_%06d", reconciliationID, index)); err != nil {
				return err
			}
		}
		sequenceByBranch := map[string]int{}
		for _, row := range numbers {
			sequenceByBranch[row.branchID]++
			newNumber := compactedInvoiceNumber(row.oldNumber, sequenceByBranch[row.branchID])
			if _, err := tx.ExecContext(ctx, `UPDATE invoices SET invoice_number=$2 WHERE id=$1`, row.id, newNumber); err != nil {
				return err
			}
			if newNumber != row.oldNumber {
				if err := insertReconciliationLog(ctx, tx, reconciliationID, "invoice_renumbered", user, map[string]any{
					"invoice_id": row.id, "branch_id": row.branchID,
					"original_invoice_number": row.oldNumber, "new_invoice_number": newNumber,
					"before_data": map[string]any{"invoice_number": row.oldNumber},
					"after_data":  map[string]any{"invoice_number": newNumber, "sequence": sequenceByBranch[row.branchID]},
				}); err != nil {
					return err
				}
			}
		}

		meta.EntityType, meta.EntityID, meta.Action = "month_end_reconciliation", &reconciliationID, "month_end.reconcile"
		meta.After = map[string]any{
			"reconciliation_number": reconciliationNumber, "date_from": periodStartDate, "date_to": periodEndDate, "branch_ids": branchIDs,
			"original_revenue": centsToFloat(source.OriginalRevenue), "suppressed_revenue": centsToFloat(suppressedAmount),
			"adjustment_reduction": centsToFloat(adjustmentReduction), "final_revenue": centsToFloat(finalRevenue),
			"suppressed_invoice_count": suppressedCount, "adjusted_item_count": adjustedCount,
			"legacy_target_ignored": true, "reconciliation_mode": "hide_all_cash_no_tax",
		}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.GetReconciliation(ctx, user, reconciliationID)
}

func (s *Service) ListReconciliations(ctx context.Context, user platform.AuthUser) ([]map[string]any, error) {
	if user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id::text,r.reconciliation_number,r.period_start,r.period_end,r.branch_ids,
		       r.target_revenue,r.original_revenue,r.suppressed_revenue,r.adjustment_reduction,
		       r.final_revenue,r.suppressed_invoice_count,r.adjusted_item_count,r.adjustment_percent,u.full_name,r.finalized_at
		FROM month_end_reconciliations r
		INNER JOIN users u ON u.id=r.finalized_by
		ORDER BY r.finalized_at DESC LIMIT 200
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, actor string
		var start, end, finalized time.Time
		var branchIDs pq.StringArray
		var target, original, suppressed, reduction, final, adjustmentPercent float64
		var suppressedCount, adjustedCount int
		if err := rows.Scan(&id, &number, &start, &end, &branchIDs, &target, &original, &suppressed,
			&reduction, &final, &suppressedCount, &adjustedCount, &adjustmentPercent, &actor, &finalized); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id": id, "reconciliation_number": number, "period_start": start.Format("2006-01-02"),
			"period_end": end.Format("2006-01-02"), "branch_ids": []string(branchIDs), "target_revenue": target,
			"original_revenue": original, "suppressed_revenue": suppressed, "adjustment_reduction": reduction,
			"final_revenue": final, "suppressed_invoice_count": suppressedCount, "adjusted_item_count": adjustedCount,
			"adjustment_percent": adjustmentPercent,
			"finalized_by_name":  actor, "finalized_at": finalized,
		})
	}
	return items, rows.Err()
}

func (s *Service) GetReconciliation(ctx context.Context, user platform.AuthUser, id string) (map[string]any, error) {
	if user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	var number, actor, sourceHash string
	var start, end, finalized time.Time
	var branchIDs pq.StringArray
	var target, original, suppressed, reduction, final, adjustmentPercent float64
	var suppressedCount, adjustedCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT r.reconciliation_number,r.period_start,r.period_end,r.branch_ids,r.target_revenue,
		       r.original_revenue,r.suppressed_revenue,r.adjustment_reduction,r.final_revenue,
		       r.suppressed_invoice_count,r.adjusted_item_count,r.adjustment_percent,r.source_hash,u.full_name,r.finalized_at
		FROM month_end_reconciliations r INNER JOIN users u ON u.id=r.finalized_by WHERE r.id=$1
	`, id).Scan(&number, &start, &end, &branchIDs, &target, &original, &suppressed, &reduction,
		&final, &suppressedCount, &adjustedCount, &adjustmentPercent, &sourceHash, &actor, &finalized); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบรายการสรุปสิ้นเดือน")
		}
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id::text,l.log_type,COALESCE(l.invoice_id::text,''),COALESCE(l.invoice_item_id::text,''),
		       COALESCE(l.branch_id::text,''),COALESCE(b.name,''),COALESCE(l.product_id::text,''),COALESCE(p.name,''),
		       l.original_invoice_number,l.new_invoice_number,l.old_unit_price,l.new_unit_price,
		       l.variance_amount,COALESCE(l.stock_bucket,''),l.stock_quantity,l.before_data::text,l.after_data::text,
		       l.adjustment_reason,l.movement_role,l.real_stock_deducted,l.ghost_stock_deducted,l.created_at
		FROM reconciliation_logs l
		LEFT JOIN branches b ON b.id=l.branch_id LEFT JOIN products p ON p.id=l.product_id
		WHERE l.reconciliation_id=$1 ORDER BY l.created_at,l.id
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []map[string]any{}
	for rows.Next() {
		var logID, logType, invoiceID, itemID, branchID, branchName, productID, productName, oldNumber, newNumber, bucket, beforeRaw, afterRaw, reason, movementRole string
		var oldPrice, newPrice sql.NullFloat64
		var variance float64
		var stockQuantity, realDeducted, ghostDeducted int
		var created time.Time
		if err := rows.Scan(&logID, &logType, &invoiceID, &itemID, &branchID, &branchName, &productID, &productName,
			&oldNumber, &newNumber, &oldPrice, &newPrice, &variance, &bucket, &stockQuantity, &beforeRaw, &afterRaw,
			&reason, &movementRole, &realDeducted, &ghostDeducted, &created); err != nil {
			return nil, err
		}
		var before, after any
		_ = json.Unmarshal([]byte(beforeRaw), &before)
		_ = json.Unmarshal([]byte(afterRaw), &after)
		log := map[string]any{
			"id": logID, "log_type": logType, "invoice_id": invoiceID, "invoice_item_id": itemID,
			"branch_id": branchID, "branch_name": branchName, "product_id": productID, "product_name": productName,
			"original_invoice_number": oldNumber, "new_invoice_number": newNumber, "variance_amount": variance,
			"stock_bucket": bucket, "stock_quantity": stockQuantity, "before_data": before, "after_data": after,
			"adjustment_reason": reason, "movement_role": movementRole, "real_stock_deducted": realDeducted,
			"ghost_stock_deducted": ghostDeducted, "created_at": created,
		}
		if oldPrice.Valid {
			log["old_unit_price"] = oldPrice.Float64
		}
		if newPrice.Valid {
			log["new_unit_price"] = newPrice.Float64
		}
		logs = append(logs, log)
	}
	return map[string]any{
		"id": id, "reconciliation_number": number, "period_start": start.Format("2006-01-02"),
		"period_end": end.Format("2006-01-02"), "branch_ids": []string(branchIDs), "target_revenue": target,
		"original_revenue": original, "suppressed_revenue": suppressed, "adjustment_reduction": reduction,
		"final_revenue": final, "suppressed_invoice_count": suppressedCount, "adjusted_item_count": adjustedCount,
		"adjustment_percent": adjustmentPercent,
		"source_hash":        sourceHash, "finalized_by_name": actor, "finalized_at": finalized, "logs": logs,
	}, rows.Err()
}

func (h *Handler) PreviewReconciliation(c echo.Context) error {
	var input ReconciliationInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลสรุปสิ้นเดือนไม่ถูกต้อง"))
	}
	item, err := h.service.PreviewReconciliation(c.Request().Context(), platform.CurrentUser(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}

func (h *Handler) ReconciliationOverview(c echo.Context) error {
	var input ReconciliationInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลรอบสรุปสิ้นเดือนไม่ถูกต้อง"))
	}
	item, err := h.service.ReconciliationOverview(c.Request().Context(), platform.CurrentUser(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}

func (h *Handler) FinalizeReconciliation(c echo.Context) error {
	var input ReconciliationInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลสรุปสิ้นเดือนไม่ถูกต้อง"))
	}
	item, err := h.service.FinalizeReconciliation(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"item": item, "message": "สรุปสิ้นเดือนเสร็จสมบูรณ์"})
}

func (h *Handler) ListReconciliations(c echo.Context) error {
	items, err := h.service.ListReconciliations(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetReconciliation(c echo.Context) error {
	item, err := h.service.GetReconciliation(c.Request().Context(), platform.CurrentUser(c), c.Param("reconciliationID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}
