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

// ReconciliationInput scopes a month-end round. TargetRevenue and Adjustments
// are accepted for client compatibility only: under the cost-markup rule the
// target is an output of the round, never something the operator types.
type ReconciliationInput struct {
	Month     string   `json:"month"`
	DateFrom  string   `json:"date_from"`
	DateTo    string   `json:"date_to"`
	BranchIDs []string `json:"branch_ids"`
	// Ignored; kept so older clients keep working.
	TargetRevenue float64 `json:"target_revenue"`
	// Markup over cost (5–10, two decimals) applied to cash bills that Ghost
	// Stock cannot cover. Zero selects the 5% floor.
	AdjustmentPercent float64                         `json:"adjustment_percent"`
	Adjustments       []ReconciliationAdjustmentInput `json:"adjustments"`
}

const (
	// A cash bill Ghost Stock can cover in full is hidden and returned to WH.
	classificationHiddenGhost = "hidden_ghost"
	// A cash bill Ghost Stock cannot cover stays, recorded at cost × (1 + markup).
	classificationRepriced = "repriced_cost_markup"
	// Bank transfer, mixed tender, and full-tax-invoice bills are untouched.
	classificationUnchanged = "unchanged"

	reconciliationModeCostMarkup = "hide_ghost_covered_cost_markup"
	minimumMarkupPercent         = 5
	maximumMarkupPercent         = 10
)

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
	CostBasis      float64      `json:"cost_basis"`
	NewUnitPrice   float64      `json:"new_unit_price"`
	NewLineTotal   float64      `json:"new_line_total"`
	Variance       float64      `json:"variance_amount"`
	Repriced       bool         `json:"repriced"`
	// Suppressed marks a line Ghost Stock could cover: its goods go back to the
	// warehouse, the Ghost is deducted, and the line leaves the bill.
	Suppressed  bool `json:"suppressed"`
	MissingCost bool `json:"missing_cost"`
	GhostStock  int  `json:"ghost_stock_available"`
	RealStock   int  `json:"real_stock_available"`
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
	Classification        string                `json:"classification"`
	WillSuppress          bool                  `json:"will_suppress"`
	WillReprice           bool                  `json:"will_reprice"`
	Items                 []*reconciliationItem `json:"items"`
	// SuppressedItemCount is how many of this bill's lines Ghost Stock covered.
	// Equal to len(Items) means the whole bill goes; anything between 1 and
	// len(Items)-1 is a bill that survives with fewer lines.
	SuppressedItemCount int     `json:"suppressed_item_count"`
	FinalTotal          float64 `json:"final_total"`
	VarianceAmount      float64 `json:"variance_amount"`
}

type reconciliationSource struct {
	PeriodStart     time.Time
	PeriodEnd       time.Time
	BranchIDs       []string
	Invoices        []*reconciliationInvoice
	InvoiceByID     map[string]*reconciliationInvoice
	ItemByID        map[string]*reconciliationItem
	OriginalRevenue int64
	// CandidateAmount is the whole cash-only/no-full-tax pool before the plan
	// splits it into hidden and repriced bills.
	CandidateAmount int64
	SourceHash      string
}

// reconciliationPlan is the outcome of applyCostMarkupPlan, in satang.
type reconciliationPlan struct {
	MarkupPercent         float64
	MarkupBasisPoints     int64
	OriginalRevenue       int64
	HiddenRevenue         int64
	RepricedOriginal      int64
	RepricedFinal         int64
	UnchangedRevenue      int64
	HiddenInvoiceCount    int
	RepricedInvoiceCount  int
	UnchangedInvoiceCount int
	AdjustedItemCount     int
	MissingCostItemCount  int
	// PartialInvoiceCount is bills that lost some lines to Ghost Stock but kept
	// others; SuppressedItemCount counts every line removed, in whole bills and
	// partial ones alike.
	PartialInvoiceCount int
	SuppressedItemCount int
}

// AdjustmentReduction is the difference between what the repriced bills sold
// for and what the books record for them (10,000 − 5,775 = 4,225).
func (p reconciliationPlan) AdjustmentReduction() int64 { return p.RepricedOriginal - p.RepricedFinal }

// FinalRevenue is the target: untouched bills plus repriced bills.
func (p reconciliationPlan) FinalRevenue() int64 { return p.UnchangedRevenue + p.RepricedFinal }

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
		invoice.Classification = classificationUnchanged
		invoice.FinalTotal = invoice.TotalAmount
		source.OriginalRevenue += centsFromFloat(invoice.TotalAmount)
		if invoice.SuppressionCandidate {
			source.CandidateAmount += centsFromFloat(invoice.TotalAmount)
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
		WHERE ii.invoice_id=ANY($1::uuid[]) AND ii.reconciliation_removed_at IS NULL
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

// reconciliationOverviewGroups buckets the bills in scope by tender and by
// the action the cost-markup rule takes on them. Run applyCostMarkupPlan first
// so cash bills carry their classification.
func reconciliationOverviewGroups(invoices []*reconciliationInvoice) map[string]reconciliationOverviewGroup {
	groupCents := map[string]int64{}
	groupCounts := map[string]int{}
	for _, invoice := range invoices {
		key := "unclassified"
		switch {
		case invoice.SuppressionCandidate && invoice.WillReprice:
			key = "cash_repriced"
		case invoice.SuppressionCandidate:
			key = "cash_hidden_ghost"
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
	for _, key := range []string{"cash_hidden_ghost", "cash_repriced", "cash_full_tax", "bank_transfer", "mixed", "unclassified"} {
		result[key] = reconciliationOverviewGroup{
			InvoiceCount: groupCounts[key],
			Revenue:      centsToFloat(groupCents[key]),
		}
	}
	// The whole cash-only/no-full-tax pool, plus the legacy keys older clients
	// still read: "suppressed" is what gets hidden, "adjustable" what gets
	// repriced.
	result["cash_no_tax"] = reconciliationOverviewGroup{
		InvoiceCount: groupCounts["cash_hidden_ghost"] + groupCounts["cash_repriced"],
		Revenue:      centsToFloat(groupCents["cash_hidden_ghost"] + groupCents["cash_repriced"]),
	}
	result["cash_suppressed"] = result["cash_hidden_ghost"]
	result["cash_adjustable"] = result["cash_repriced"]
	return result
}

// markupBasisPoints validates the markup over cost. Zero selects the 5% floor
// so clients that still send 0 keep working; anything else must sit inside
// 5–10 with at most two decimals. Returns basis points and the percent used.
func markupBasisPoints(percent float64) (int64, float64, error) {
	if percent == 0 {
		percent = minimumMarkupPercent
	}
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < minimumMarkupPercent || percent > maximumMarkupPercent {
		return 0, 0, platform.NewError(http.StatusBadRequest, "เปอร์เซ็นต์กำไรเหนือต้นทุนต้องอยู่ระหว่าง 5 ถึง 10")
	}
	value := math.Round(percent * 100)
	if math.Abs(percent*100-value) > 0.000001 {
		return 0, 0, platform.NewError(http.StatusBadRequest, "เปอร์เซ็นต์กำไรเหนือต้นทุนต้องมีทศนิยมไม่เกิน 2 ตำแหน่ง")
	}
	return int64(value), percent, nil
}

// costMarkupUnitPrice is cost × (1 + markup) in satang, rounded half up:
// 5,500 × 1.05 = 5,775.
func costMarkupUnitPrice(costCents, markupBasisPoints int64) int64 {
	return roundedRatio(costCents*(10000+markupBasisPoints), 10000)
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

// allocateGhostToLines hands the warehouse's remaining Ghost Stock out line by
// line, in the order the lines were sold, and reports how many lines it covered.
//
// A line is covered whole or not at all: a line is one lot allocation, and
// splitting it would leave half a line returned to the warehouse and half of it
// repriced — two different treatments of the same sale. Bills are no longer
// all-or-nothing, though: a bill selling a wheelchair the warehouse has Ghost
// for alongside an IV pole it does not now loses the wheelchair line and keeps
// the IV pole, repriced.
func allocateGhostToLines(invoice *reconciliationInvoice, ghostRemaining map[string]int) int {
	covered := 0
	for _, item := range invoice.Items {
		if item.Quantity <= 0 || ghostRemaining[item.ProductID] < item.Quantity {
			continue
		}
		ghostRemaining[item.ProductID] -= item.Quantity
		item.Suppressed = true
		covered++
	}
	return covered
}

// applyCostMarkupPlan classifies every bill in scope exactly once:
//
//   - on a cash-only bill without a full tax invoice, every line the warehouse
//     has Ghost Stock for is removed: its goods go back to the warehouse and the
//     Ghost is deducted. Lines left standing are recorded at cost × (1 + markup);
//   - a bill whose lines are ALL covered disappears entirely (the existing hidden
//     path); one that keeps at least one line survives with fewer lines;
//   - every other bill (bank transfer, mixed tender, full tax invoice) is untouched.
//
// Ghost Stock is handed out line by line in created_at order, so an earlier bill
// wins the last units of a product. The target revenue is untouched bills plus
// what the surviving lines are re-recorded at; it is an output of the rule, not
// an input.
func applyCostMarkupPlan(source *reconciliationSource, percent float64) (reconciliationPlan, error) {
	basisPoints, normalized, err := markupBasisPoints(percent)
	if err != nil {
		return reconciliationPlan{}, err
	}
	plan := reconciliationPlan{MarkupPercent: normalized, MarkupBasisPoints: basisPoints}

	ghostRemaining := map[string]int{}
	for _, invoice := range source.Invoices {
		for _, item := range invoice.Items {
			if _, seen := ghostRemaining[item.ProductID]; !seen {
				ghostRemaining[item.ProductID] = item.GhostStock
			}
		}
	}

	for _, invoice := range source.Invoices {
		original := centsFromFloat(invoice.TotalAmount)
		plan.OriginalRevenue += original
		invoice.WillSuppress, invoice.WillReprice = false, false
		invoice.FinalTotal, invoice.VarianceAmount = invoice.TotalAmount, 0
		for _, item := range invoice.Items {
			item.NewUnitPrice, item.NewLineTotal, item.Variance = item.UnitPrice, item.LineTotal, 0
			item.CostBasis = item.LotUnitCost
			item.Repriced, item.MissingCost = false, false
		}
		if !invoice.SuppressionCandidate {
			invoice.Classification = classificationUnchanged
			plan.UnchangedRevenue += original
			plan.UnchangedInvoiceCount++
			continue
		}
		covered := allocateGhostToLines(invoice, ghostRemaining)
		invoice.SuppressedItemCount = covered
		plan.SuppressedItemCount += covered

		// Every line covered: nothing is left to bill for, so the bill goes.
		if covered > 0 && covered == len(invoice.Items) {
			invoice.Classification = classificationHiddenGhost
			invoice.WillSuppress = true
			plan.HiddenRevenue += original
			plan.HiddenInvoiceCount++
			continue
		}

		invoice.Classification = classificationRepriced
		invoice.WillReprice = true
		if covered > 0 {
			plan.PartialInvoiceCount++
		}
		// Lines the warehouse covered leave the bill; what they sold for is
		// hidden revenue exactly as a whole hidden bill's would be, so the books
		// still add up as untouched + hidden + retained.
		suppressedValue := int64(0)
		for _, item := range invoice.Items {
			if item.Suppressed {
				suppressedValue += centsFromFloat(item.LineTotal)
			}
		}
		plan.HiddenRevenue += suppressedValue
		retainedOriginal := original - suppressedValue

		reduction := int64(0)
		for _, item := range invoice.Items {
			if item.Suppressed {
				continue
			}
			costCents := centsFromFloat(item.LotUnitCost)
			if costCents <= 0 {
				// No cost on the lot or the sale snapshot: leave the line at its
				// sold price rather than recording it at zero, and say so.
				item.MissingCost = true
				plan.MissingCostItemCount++
				continue
			}
			oldUnitPrice := centsFromFloat(item.UnitPrice)
			newUnitPrice := costMarkupUnitPrice(costCents, basisPoints)
			if newUnitPrice >= oldUnitPrice {
				// Already sold at or below cost + markup (giveaways, promotions).
				continue
			}
			newLineTotal := reconciliationLineTotal(item, newUnitPrice)
			lineReduction := centsFromFloat(item.LineTotal) - newLineTotal
			if lineReduction <= 0 {
				continue
			}
			item.NewUnitPrice = centsToFloat(newUnitPrice)
			item.NewLineTotal = centsToFloat(newLineTotal)
			item.Variance = centsToFloat(lineReduction)
			item.Repriced = true
			reduction += lineReduction
			plan.AdjustedItemCount++
		}
		invoice.FinalTotal = centsToFloat(retainedOriginal - reduction)
		// The whole drop the bill takes — lines removed plus the repricing — so
		// the log and the payment shrink below describe the same number.
		invoice.VarianceAmount = centsToFloat(original - (retainedOriginal - reduction))
		plan.RepricedOriginal += retainedOriginal
		plan.RepricedFinal += retainedOriginal - reduction
		plan.RepricedInvoiceCount++
	}
	return plan, nil
}

func reconciliationPlanSummary(source reconciliationSource, plan reconciliationPlan) map[string]any {
	finalRevenue := plan.FinalRevenue()
	return map[string]any{
		"period_start":               platform.InBangkok(source.PeriodStart).Format("2006-01-02"),
		"period_end":                 platform.InBangkok(source.PeriodEnd.Add(-time.Second)).Format("2006-01-02"),
		"branch_ids":                 source.BranchIDs,
		"original_revenue":           centsToFloat(plan.OriginalRevenue),
		"cash_no_tax_revenue":        centsToFloat(source.CandidateAmount),
		"hidden_revenue":             centsToFloat(plan.HiddenRevenue),
		"suppressed_revenue":         centsToFloat(plan.HiddenRevenue),
		"base_revenue":               centsToFloat(plan.OriginalRevenue - plan.HiddenRevenue),
		"repriced_original_revenue":  centsToFloat(plan.RepricedOriginal),
		"repriced_final_revenue":     centsToFloat(plan.RepricedFinal),
		"adjustment_reduction":       centsToFloat(plan.AdjustmentReduction()),
		"unchanged_revenue":          centsToFloat(plan.UnchangedRevenue),
		"final_revenue":              centsToFloat(finalRevenue),
		"target_revenue":             centsToFloat(finalRevenue),
		"invoice_count":              len(source.Invoices),
		"suppressed_invoice_count":   plan.HiddenInvoiceCount,
		"hidden_invoice_count":       plan.HiddenInvoiceCount,
		"repriced_invoice_count":     plan.RepricedInvoiceCount,
		"unchanged_invoice_count":    plan.UnchangedInvoiceCount,
		"adjusted_item_count":        plan.AdjustedItemCount,
		"partial_invoice_count":      plan.PartialInvoiceCount,
		"suppressed_item_count":      plan.SuppressedItemCount,
		"missing_cost_item_count":    plan.MissingCostItemCount,
		"adjustment_percent":         plan.MarkupPercent,
		"minimum_adjustment_percent": minimumMarkupPercent,
		"maximum_adjustment_percent": maximumMarkupPercent,
		"legacy_target_ignored":      true,
		"reconciliation_mode":        reconciliationModeCostMarkup,
		"source_hash":                source.SourceHash,
	}
}

func (s *Service) ReconciliationOverview(ctx context.Context, user platform.AuthUser, input ReconciliationInput) (map[string]any, error) {
	if user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	source, err := s.loadReconciliationSource(ctx, s.db, input)
	if err != nil {
		return nil, err
	}
	plan, err := applyCostMarkupPlan(&source, input.AdjustmentPercent)
	if err != nil {
		return nil, err
	}
	result := reconciliationPlanSummary(source, plan)
	result["remaining_cash_invoice_count"] = plan.RepricedInvoiceCount
	result["non_cash_invoice_count"] = plan.UnchangedInvoiceCount
	result["invoice_groups"] = reconciliationOverviewGroups(source.Invoices)
	return result, nil
}

func (s *Service) PreviewReconciliation(ctx context.Context, user platform.AuthUser, input ReconciliationInput) (map[string]any, error) {
	if user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	source, err := s.loadReconciliationSource(ctx, s.db, input)
	if err != nil {
		return nil, err
	}
	plan, err := applyCostMarkupPlan(&source, input.AdjustmentPercent)
	if err != nil {
		return nil, err
	}
	hidden := []*reconciliationInvoice{}
	repriced := []*reconciliationInvoice{}
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
		// Ghost moves for every removed line, whether its bill disappears or
		// merely gets shorter — counting only whole hidden bills understated the
		// deduction the moment a bill could be partly covered.
		for _, item := range invoice.Items {
			if !item.Suppressed {
				continue
			}
			projection := projectionByProduct[item.ProductID]
			if projection == nil {
				projection = &ghostProjection{ProductID: item.ProductID, ProductName: item.ProductName, GhostBefore: item.GhostStock}
				projectionByProduct[item.ProductID] = projection
			}
			projection.Quantity += item.Quantity
			totalQuantity += item.Quantity
		}
		switch {
		case invoice.WillSuppress:
			hidden = append(hidden, invoice)
		case invoice.WillReprice:
			repriced = append(repriced, invoice)
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
	result := reconciliationPlanSummary(source, plan)
	result["target_difference"] = 0
	result["target_matched"] = true
	result["suppression_candidates"] = hidden
	result["repriced_invoices"] = repriced
	result["remaining_cash_invoices"] = repriced
	result["eligible_cash_invoice_count"] = len(hidden) + len(repriced)
	result["stock_projection"] = map[string]any{
		"branch_real_returned": totalQuantity, "warehouse_real_received": totalQuantity,
		"warehouse_ghost_deducted": totalQuantity, "ghost_deficit_created": ghostDeficit,
		"products": productProjection,
	}
	return result, nil
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

// repriceRetainedInvoice records a cash bill that Ghost Stock could not cover
// at cost × (1 + markup). The bill keeps its number and its Real stock
// movements; only the money columns move. The cash payment shrinks with the
// total so the payment ledger never settles more cash than the bill shows.
func (s *Service) repriceRetainedInvoice(ctx context.Context, tx *sql.Tx, reconciliationID string, user platform.AuthUser, plan reconciliationPlan, invoice *reconciliationInvoice) error {
	const reason = "COST_MARKUP_NO_GHOST_STOCK"
	var subtotalDelta, taxDelta, totalDelta int64
	repricedLines := 0

	// Lines the warehouse had Ghost Stock for have already been returned to it
	// upstream; here they leave the bill. The customer is refunded their share
	// through the payment shrink below, so the bill states only what remains.
	removedLines := 0
	for _, item := range invoice.Items {
		if !item.Suppressed {
			continue
		}
		subtotalDelta += centsFromFloat(item.LineSubtotal)
		taxDelta += centsFromFloat(item.TaxAmount)
		totalDelta += centsFromFloat(item.LineTotal)
		// Flagged, not deleted: reconciliation_item_snapshots and
		// reconciliation_logs hold RESTRICT keys onto this row so the trail of
		// what the close did cannot be destroyed. Every screen that shows a bill
		// filters removed lines out, so the bill reads as one line shorter.
		if _, err := tx.ExecContext(ctx, `
			UPDATE invoice_items SET reconciliation_removed_at=NOW(),reconciled_at=NOW() WHERE id=$1
		`, item.ID); err != nil {
			return err
		}
		if err := insertReconciliationLog(ctx, tx, reconciliationID, "line_suppressed", user, map[string]any{
			"invoice_id": invoice.ID, "invoice_item_id": item.ID, "branch_id": invoice.BranchID, "product_id": item.ProductID,
			"original_invoice_number": invoice.InvoiceNumber,
			"variance_amount":         item.LineTotal,
			"adjustment_reason":       "PRODUCT_RETURN_TO_WAREHOUSE",
			"movement_role":           "line_hidden",
			"before_data":             map[string]any{"product_name": item.ProductName, "quantity": item.Quantity, "unit_price": item.UnitPrice, "line_subtotal": item.LineSubtotal, "tax_amount": item.TaxAmount, "line_total": item.LineTotal, "warehouse_ghost_available": item.GhostStock},
			"after_data":              map[string]any{"removed_from_invoice": true, "effective_stock_bucket": "ghost"},
		}); err != nil {
			return err
		}
		removedLines++
	}

	for _, item := range invoice.Items {
		if !item.Repriced || item.Suppressed {
			continue
		}
		newUnitPrice := centsFromFloat(item.NewUnitPrice)
		newSubtotal := newUnitPrice * int64(item.Quantity)
		newTotal := centsFromFloat(item.NewLineTotal)
		newTax := newTotal - newSubtotal
		subtotalDelta += centsFromFloat(item.LineSubtotal) - newSubtotal
		taxDelta += centsFromFloat(item.TaxAmount) - newTax
		totalDelta += centsFromFloat(item.LineTotal) - newTotal
		if _, err := tx.ExecContext(ctx, `
			UPDATE invoice_items
			SET unit_price=$2::numeric,sold_unit_price=$2::numeric*unit_conversion_qty,line_subtotal=$3,tax_amount=$4,line_total=$5,
			    reconciliation_discount_amount=reconciliation_discount_amount+$6,reconciled_at=NOW()
			WHERE id=$1
		`, item.ID, item.NewUnitPrice, centsToFloat(newSubtotal), centsToFloat(newTax), item.NewLineTotal, item.Variance); err != nil {
			return err
		}
		if err := insertReconciliationLog(ctx, tx, reconciliationID, "price_adjusted", user, map[string]any{
			"invoice_id": invoice.ID, "invoice_item_id": item.ID, "branch_id": invoice.BranchID, "product_id": item.ProductID,
			"original_invoice_number": invoice.InvoiceNumber,
			"old_unit_price":          item.UnitPrice, "new_unit_price": item.NewUnitPrice, "variance_amount": item.Variance,
			"adjustment_reason": reason,
			"before_data":       map[string]any{"unit_price": item.UnitPrice, "line_subtotal": item.LineSubtotal, "tax_amount": item.TaxAmount, "line_total": item.LineTotal, "cost_basis": item.CostBasis, "warehouse_ghost_available": item.GhostStock},
			"after_data":        map[string]any{"unit_price": item.NewUnitPrice, "line_subtotal": centsToFloat(newSubtotal), "tax_amount": centsToFloat(newTax), "line_total": item.NewLineTotal, "markup_percent": plan.MarkupPercent},
		}); err != nil {
			return err
		}
		repricedLines++
	}
	if repricedLines == 0 && removedLines == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE invoices SET subtotal=subtotal-$2,tax_amount=tax_amount-$3,total_amount=total_amount-$4,updated_at=NOW() WHERE id=$1
	`, invoice.ID, centsToFloat(subtotalDelta), centsToFloat(taxDelta), centsToFloat(totalDelta)); err != nil {
		return err
	}

	type paymentRow struct {
		id     string
		amount int64
	}
	paymentRows, err := tx.QueryContext(ctx, `SELECT id::text,amount FROM invoice_payments WHERE invoice_id=$1 ORDER BY created_at DESC,id DESC FOR UPDATE`, invoice.ID)
	if err != nil {
		return err
	}
	payments := []paymentRow{}
	for paymentRows.Next() {
		var row paymentRow
		var amount float64
		if err := paymentRows.Scan(&row.id, &amount); err != nil {
			paymentRows.Close()
			return err
		}
		row.amount = centsFromFloat(amount)
		payments = append(payments, row)
	}
	if err := paymentRows.Close(); err != nil {
		return err
	}
	remaining := totalDelta
	beforePayments := []map[string]any{}
	afterPayments := []map[string]any{}
	for _, payment := range payments {
		cut := remaining
		if cut > payment.amount {
			cut = payment.amount
		}
		if cut < 0 {
			cut = 0
		}
		if cut > 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE invoice_payments SET amount=amount-$2 WHERE id=$1`, payment.id, centsToFloat(cut)); err != nil {
				return err
			}
			remaining -= cut
		}
		beforePayments = append(beforePayments, map[string]any{"id": payment.id, "amount": centsToFloat(payment.amount)})
		afterPayments = append(afterPayments, map[string]any{"id": payment.id, "amount": centsToFloat(payment.amount - cut)})
	}
	return insertReconciliationLog(ctx, tx, reconciliationID, "invoice_repriced", user, map[string]any{
		"invoice_id": invoice.ID, "branch_id": invoice.BranchID, "original_invoice_number": invoice.InvoiceNumber,
		"variance_amount": invoice.VarianceAmount, "adjustment_reason": reason,
		"before_data": map[string]any{"total_amount": invoice.TotalAmount, "payments": beforePayments},
		"after_data":  map[string]any{"total_amount": invoice.FinalTotal, "payments": afterPayments, "markup_percent": plan.MarkupPercent, "repriced_line_count": repricedLines, "removed_line_count": removedLines, "remaining_line_count": len(invoice.Items) - removedLines},
	})
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
		plan, err := applyCostMarkupPlan(&source, input.AdjustmentPercent)
		if err != nil {
			return err
		}
		finalRevenue := plan.FinalRevenue()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO month_end_reconciliations (
				id,reconciliation_number,period_start,period_end,branch_ids,target_revenue,
				original_revenue,suppressed_revenue,adjustment_reduction,final_revenue,
				suppressed_invoice_count,adjusted_item_count,adjustment_percent,source_hash,finalized_by,
				reconciliation_mode,legacy_target_ignored,finalized_at,created_at
			) VALUES ($1,$2,$3,$4,$5::uuid[],$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,TRUE,NOW(),NOW())
		`, reconciliationID, reconciliationNumber, periodStartDate, periodEndDate, pq.Array(branchIDs), centsToFloat(finalRevenue),
			centsToFloat(plan.OriginalRevenue), centsToFloat(plan.HiddenRevenue), centsToFloat(plan.AdjustmentReduction()),
			centsToFloat(finalRevenue), plan.HiddenInvoiceCount, plan.AdjustedItemCount, plan.MarkupPercent, source.SourceHash, user.ID,
			reconciliationModeCostMarkup); err != nil {
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

		// Any bill that lost lines to Ghost Stock returns those goods, whether it
		// lost all of them (and disappears) or only some (and survives shorter).
		for _, invoice := range source.Invoices {
			if invoice.SuppressedItemCount == 0 {
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
				if !item.Suppressed {
					continue
				}
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
			// A bill that kept lines stays in the books; removing those lines and
			// repricing what is left happens in the retained pass below.
			if !invoice.WillSuppress {
				continue
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

		// Cash bills Ghost Stock could not cover in full stay in the books: the
		// covered lines are struck off and whatever is left is recorded at
		// cost × (1 + markup). Runs after the returns above so the goods have
		// already moved, and before renumbering so it sees the survivors.
		for _, invoice := range source.Invoices {
			if !invoice.WillReprice {
				continue
			}
			if err := s.repriceRetainedInvoice(ctx, tx, reconciliationID, user, plan, invoice); err != nil {
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
			"original_revenue": centsToFloat(plan.OriginalRevenue), "suppressed_revenue": centsToFloat(plan.HiddenRevenue),
			"repriced_original_revenue": centsToFloat(plan.RepricedOriginal), "repriced_final_revenue": centsToFloat(plan.RepricedFinal),
			"adjustment_reduction": centsToFloat(plan.AdjustmentReduction()), "final_revenue": centsToFloat(finalRevenue),
			"suppressed_invoice_count": plan.HiddenInvoiceCount, "repriced_invoice_count": plan.RepricedInvoiceCount,
			"adjusted_item_count": plan.AdjustedItemCount, "adjustment_percent": plan.MarkupPercent,
			"legacy_target_ignored": true, "reconciliation_mode": reconciliationModeCostMarkup,
		}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.GetReconciliation(ctx, user, reconciliationID)
}

// ReconciliationQuery pages the closed rounds for the dashboard's round picker:
// the newest twenty, then twenty more each time the operator reaches the end of
// the list, with an optional substring match on the round number or its period.
type ReconciliationQuery struct {
	Search string
	Limit  int
	Offset int
}

func reconciliationQueryFrom(c echo.Context) ReconciliationQuery {
	query := ReconciliationQuery{Search: strings.TrimSpace(c.QueryParam("search")), Limit: 20}
	if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 && value <= 100 {
			query.Limit = value
		}
	}
	if raw := strings.TrimSpace(c.QueryParam("offset")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			query.Offset = value
		}
	}
	return query
}

func (s *Service) ListReconciliations(ctx context.Context, user platform.AuthUser, query ReconciliationQuery) ([]map[string]any, error) {
	if user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	if query.Limit <= 0 {
		query.Limit = 20
	}
	// Matching the period as text lets "2026-09" find every round that closed
	// that month, which is how an operator actually remembers them.
	args := []any{query.Limit, query.Offset}
	where := ""
	if query.Search != "" {
		args = append(args, "%"+strings.ToLower(query.Search)+"%")
		where = `WHERE LOWER(r.reconciliation_number) LIKE $3
		       OR r.period_start::text LIKE $3
		       OR r.period_end::text LIKE $3`
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id::text,r.reconciliation_number,r.period_start,r.period_end,r.branch_ids,
		       r.target_revenue,r.original_revenue,r.suppressed_revenue,r.adjustment_reduction,
		       r.final_revenue,r.suppressed_invoice_count,r.adjusted_item_count,r.adjustment_percent,r.reconciliation_mode,u.full_name,r.finalized_at
		FROM month_end_reconciliations r
		INNER JOIN users u ON u.id=r.finalized_by
		`+where+`
		ORDER BY r.finalized_at DESC LIMIT $1 OFFSET $2
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, actor, mode string
		var start, end, finalized time.Time
		var branchIDs pq.StringArray
		var target, original, suppressed, reduction, final, adjustmentPercent float64
		var suppressedCount, adjustedCount int
		if err := rows.Scan(&id, &number, &start, &end, &branchIDs, &target, &original, &suppressed,
			&reduction, &final, &suppressedCount, &adjustedCount, &adjustmentPercent, &mode, &actor, &finalized); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id": id, "reconciliation_number": number, "period_start": start.Format("2006-01-02"),
			"period_end": end.Format("2006-01-02"), "branch_ids": []string(branchIDs), "target_revenue": target,
			"original_revenue": original, "suppressed_revenue": suppressed, "adjustment_reduction": reduction,
			"final_revenue": final, "suppressed_invoice_count": suppressedCount, "adjusted_item_count": adjustedCount,
			"adjustment_percent": adjustmentPercent, "reconciliation_mode": mode,
			"finalized_by_name": actor, "finalized_at": finalized,
		})
	}
	return items, rows.Err()
}

func (s *Service) GetReconciliation(ctx context.Context, user platform.AuthUser, id string) (map[string]any, error) {
	if user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	var number, actor, sourceHash, mode string
	var start, end, finalized time.Time
	var branchIDs pq.StringArray
	var target, original, suppressed, reduction, final, adjustmentPercent float64
	var suppressedCount, adjustedCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT r.reconciliation_number,r.period_start,r.period_end,r.branch_ids,r.target_revenue,
		       r.original_revenue,r.suppressed_revenue,r.adjustment_reduction,r.final_revenue,
		       r.suppressed_invoice_count,r.adjusted_item_count,r.adjustment_percent,r.reconciliation_mode,r.source_hash,u.full_name,r.finalized_at
		FROM month_end_reconciliations r INNER JOIN users u ON u.id=r.finalized_by WHERE r.id=$1
	`, id).Scan(&number, &start, &end, &branchIDs, &target, &original, &suppressed, &reduction,
		&final, &suppressedCount, &adjustedCount, &adjustmentPercent, &mode, &sourceHash, &actor, &finalized); err != nil {
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
		"adjustment_percent": adjustmentPercent, "reconciliation_mode": mode,
		"source_hash": sourceHash, "finalized_by_name": actor, "finalized_at": finalized, "logs": logs,
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
	items, err := h.service.ListReconciliations(c.Request().Context(), platform.CurrentUser(c), reconciliationQueryFrom(c))
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
