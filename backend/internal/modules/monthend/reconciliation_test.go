package monthend

import (
	"context"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/platform"
)

func TestReconciliationOverviewRejectsNonSuperadminBeforeDatabaseAccess(t *testing.T) {
	service := &Service{}
	if _, err := service.ReconciliationOverview(context.Background(), platform.AuthUser{RoleKey: "central_admin"}, ReconciliationInput{Month: "2026-08"}); err == nil {
		t.Fatal("expected non-superadmin overview access to be rejected")
	}
}

// cashBill is a paid, cash-only bill without a full tax invoice: one line,
// quantity 1, no VAT. ghost is the warehouse Ghost Stock of its product.
func cashBill(id, productID string, price, cost float64, ghost int) *reconciliationInvoice {
	invoice := &reconciliationInvoice{ID: id, InvoiceNumber: id, TotalAmount: price, PaymentMethod: "cash", SuppressionCandidate: true}
	invoice.Items = []*reconciliationItem{{
		ID: id + "-1", InvoiceID: id, ProductID: productID, Quantity: 1,
		UnitPrice: price, LineSubtotal: price, LineTotal: price, LotUnitCost: cost, GhostStock: ghost,
	}}
	return invoice
}

func transferBill(id string, amount float64) *reconciliationInvoice {
	return &reconciliationInvoice{ID: id, InvoiceNumber: id, TotalAmount: amount, PaymentMethod: "bank_transfer"}
}

func TestCostMarkupPlanMatchesBusinessExample(t *testing.T) {
	// Branch A: TA001 transfer 100, CA002 cash 100 covered by Ghost Stock,
	// CA003 cash 100 with no Ghost Stock. Branch B: CB001 cash 100 with no
	// Ghost Stock, TB002/TB003 transfer 100 each. 600 sold → 450 recorded.
	ta001 := transferBill("TA001", 100)
	ca002 := cashBill("CA002", "P-GHOST", 100, 71.43, 1)
	ca003 := cashBill("CA003", "P-NONE", 100, 71.43, 0)
	cb001 := cashBill("CB001", "P-NONE", 100, 71.43, 0)
	tb002 := transferBill("TB002", 100)
	tb003 := transferBill("TB003", 100)
	source := reconciliationSource{Invoices: []*reconciliationInvoice{ta001, ca002, ca003, cb001, tb002, tb003}}

	plan, err := applyCostMarkupPlan(&source, 5)
	if err != nil {
		t.Fatal(err)
	}
	if plan.OriginalRevenue != 60000 || plan.HiddenRevenue != 10000 || plan.UnchangedRevenue != 30000 ||
		plan.RepricedOriginal != 20000 || plan.RepricedFinal != 15000 {
		t.Fatalf("unexpected plan amounts: %+v", plan)
	}
	if plan.FinalRevenue() != 45000 || plan.AdjustmentReduction() != 5000 {
		t.Fatalf("target = %d, reduction = %d; want 45000 / 5000", plan.FinalRevenue(), plan.AdjustmentReduction())
	}
	if plan.HiddenInvoiceCount != 1 || plan.RepricedInvoiceCount != 2 || plan.UnchangedInvoiceCount != 3 || plan.AdjustedItemCount != 2 {
		t.Fatalf("unexpected plan counts: %+v", plan)
	}
	if !ca002.WillSuppress || ca002.WillReprice || ca002.Classification != classificationHiddenGhost {
		t.Fatalf("CA002 should be hidden: %+v", ca002)
	}
	for _, invoice := range []*reconciliationInvoice{ca003, cb001} {
		if invoice.WillSuppress || !invoice.WillReprice || invoice.Classification != classificationRepriced ||
			invoice.FinalTotal != 75 || invoice.VarianceAmount != 25 || invoice.Items[0].NewUnitPrice != 75 || !invoice.Items[0].Repriced {
			t.Fatalf("%s should be recorded at 75: %+v item=%+v", invoice.ID, invoice, invoice.Items[0])
		}
	}
	for _, invoice := range []*reconciliationInvoice{ta001, tb002, tb003} {
		if invoice.WillSuppress || invoice.WillReprice || invoice.Classification != classificationUnchanged || invoice.FinalTotal != 100 {
			t.Fatalf("%s should be untouched: %+v", invoice.ID, invoice)
		}
	}
}

func TestCostMarkupUnitPriceRecordsCostTimesOneHundredFivePercent(t *testing.T) {
	// 10,000 sold on a 5,500 cost is recorded at 5,775; the difference is 4,225.
	invoice := cashBill("INV", "P", 10000, 5500, 0)
	source := reconciliationSource{Invoices: []*reconciliationInvoice{invoice}}
	plan, err := applyCostMarkupPlan(&source, 5)
	if err != nil {
		t.Fatal(err)
	}
	item := invoice.Items[0]
	if item.NewUnitPrice != 5775 || item.NewLineTotal != 5775 || item.Variance != 4225 || invoice.FinalTotal != 5775 {
		t.Fatalf("unexpected repricing: item=%+v invoice=%+v", item, invoice)
	}
	if plan.FinalRevenue() != 577500 || plan.AdjustmentReduction() != 422500 {
		t.Fatalf("target = %d, reduction = %d; want 577500 / 422500", plan.FinalRevenue(), plan.AdjustmentReduction())
	}
	// 10% is the ceiling: 5,500 × 1.10 = 6,050.
	if _, err := applyCostMarkupPlan(&source, 10); err != nil {
		t.Fatal(err)
	}
	if invoice.Items[0].NewUnitPrice != 6050 || invoice.FinalTotal != 6050 {
		t.Fatalf("10%% markup = %.2f, want 6050.00", invoice.Items[0].NewUnitPrice)
	}
}

func TestCostMarkupPlanAppliesVATOnTheRepricedLine(t *testing.T) {
	invoice := cashBill("INV", "P", 107, 50, 0)
	invoice.Items[0].TaxRate = 7
	invoice.Items[0].LineSubtotal, invoice.Items[0].UnitPrice, invoice.Items[0].TaxAmount = 100, 100, 7
	if _, err := applyCostMarkupPlan(&reconciliationSource{Invoices: []*reconciliationInvoice{invoice}}, 5); err != nil {
		t.Fatal(err)
	}
	// 50 × 1.05 = 52.50 + 7% VAT = 56.18 (56.175 rounded half up).
	if invoice.Items[0].NewUnitPrice != 52.5 || invoice.Items[0].NewLineTotal != 56.18 || invoice.FinalTotal != 56.18 {
		t.Fatalf("unexpected VAT handling: %+v", invoice.Items[0])
	}
}

func TestCostMarkupPlanHandsOutGhostStockInBillOrder(t *testing.T) {
	// Both bills sell the same product and the warehouse holds one Ghost unit:
	// the earlier bill hides, the later one is recorded at cost + markup.
	first := cashBill("C1", "P", 100, 50, 1)
	second := cashBill("C2", "P", 100, 50, 1)
	source := reconciliationSource{Invoices: []*reconciliationInvoice{first, second}}
	plan, err := applyCostMarkupPlan(&source, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !first.WillSuppress || second.WillSuppress || !second.WillReprice {
		t.Fatalf("ghost allocation order broken: first=%+v second=%+v", first, second)
	}
	if second.FinalTotal != 52.5 || plan.FinalRevenue() != 5250 || plan.HiddenRevenue != 10000 {
		t.Fatalf("unexpected plan: second=%.2f plan=%+v", second.FinalTotal, plan)
	}
}

// addLine appends one more line to a bill and grows its total, so a test can
// build the multi-line bills the per-line rule exists for.
func addLine(invoice *reconciliationInvoice, suffix, productID string, price, cost float64, ghost int) {
	invoice.Items = append(invoice.Items, &reconciliationItem{
		ID: invoice.ID + "-" + suffix, InvoiceID: invoice.ID, ProductID: productID, ProductName: productID, Quantity: 1,
		UnitPrice: price, LineSubtotal: price, LineTotal: price, LotUnitCost: cost, GhostStock: ghost,
	})
	invoice.TotalAmount += price
}

func TestCostMarkupPlanRemovesCoveredLinesAndRepricesTheRest(t *testing.T) {
	// One line is coverable, the other is not. The covered line leaves the bill
	// (its goods go back to the warehouse); the bill survives holding only the
	// uncovered line, recorded at cost + markup. Ghost is consumed by the line
	// that used it, so a later bill for the same product can still hide.
	mixed := cashBill("C1", "P-GHOST", 100, 50, 5)
	addLine(mixed, "2", "P-NONE", 100, 50, 0)
	later := cashBill("C2", "P-GHOST", 100, 50, 5)
	source := reconciliationSource{Invoices: []*reconciliationInvoice{mixed, later}}
	plan, err := applyCostMarkupPlan(&source, 5)
	if err != nil {
		t.Fatal(err)
	}
	if mixed.WillSuppress || !mixed.WillReprice {
		t.Fatalf("a bill that keeps a line must survive: %+v", mixed)
	}
	if mixed.SuppressedItemCount != 1 || !mixed.Items[0].Suppressed || mixed.Items[1].Suppressed {
		t.Fatalf("wrong line suppressed: %+v / %+v", mixed.Items[0], mixed.Items[1])
	}
	// 200 sold - 100 removed - 47.50 repriced = 52.50 left on the bill.
	if mixed.FinalTotal != 52.5 || mixed.VarianceAmount != 147.5 {
		t.Fatalf("mixed bill money wrong: final=%.2f variance=%.2f", mixed.FinalTotal, mixed.VarianceAmount)
	}
	if !later.WillSuppress {
		t.Fatalf("later bill should still hide on the Ghost the mixed bill left: %+v", later)
	}
	// Hidden revenue is the removed line (100) plus the whole hidden bill (100);
	// only the retained 100 is counted as repriced.
	if plan.HiddenRevenue != 20000 || plan.RepricedOriginal != 10000 || plan.RepricedFinal != 5250 {
		t.Fatalf("unexpected plan money: %+v", plan)
	}
	if plan.PartialInvoiceCount != 1 || plan.SuppressedItemCount != 2 || plan.HiddenInvoiceCount != 1 {
		t.Fatalf("unexpected plan counts: %+v", plan)
	}
	// The books still add up: untouched + hidden + retained = what was sold.
	if plan.UnchangedRevenue+plan.HiddenRevenue+plan.RepricedOriginal != plan.OriginalRevenue {
		t.Fatalf("revenue does not reconcile: %+v", plan)
	}
	if plan.FinalRevenue() != 5250 {
		t.Fatalf("final revenue should be the retained line only, got %d", plan.FinalRevenue())
	}
}

func TestCostMarkupPlanSplitsOneBillOfFiveLines(t *testing.T) {
	// The case the owner described: one cash bill, no full tax invoice, holding
	// a wheelchair the warehouse has Ghost Stock for and an IV pole it does not.
	// The wheelchair line disappears; the bill is left with the IV pole, priced
	// at cost + markup. Three more lines make it five, mixed both ways.
	bill := cashBill("C1", "wheelchair", 5000, 4000, 2) // covered
	addLine(bill, "2", "iv-pole", 1000, 400, 0)         // no Ghost -> repriced
	addLine(bill, "3", "gloves", 200, 100, 10)          // covered
	addLine(bill, "4", "syringe", 300, 100, 0)          // no Ghost -> repriced
	addLine(bill, "5", "mask", 500, 200, 0)             // no Ghost -> repriced
	source := reconciliationSource{Invoices: []*reconciliationInvoice{bill}}
	plan, err := applyCostMarkupPlan(&source, 5)
	if err != nil {
		t.Fatal(err)
	}
	if bill.WillSuppress || !bill.WillReprice {
		t.Fatalf("bill keeps three lines, so it must survive: %+v", bill)
	}
	if bill.SuppressedItemCount != 2 {
		t.Fatalf("expected the wheelchair and the gloves to go, got %d", bill.SuppressedItemCount)
	}
	suppressed := map[string]bool{}
	for _, item := range bill.Items {
		suppressed[item.ProductID] = item.Suppressed
	}
	if !suppressed["wheelchair"] || !suppressed["gloves"] {
		t.Fatalf("covered lines not removed: %+v", suppressed)
	}
	if suppressed["iv-pole"] || suppressed["syringe"] || suppressed["mask"] {
		t.Fatalf("uncovered lines must stay: %+v", suppressed)
	}
	// Retained 1000 + 300 + 500 = 1800, re-recorded at cost x 1.05:
	// 420 + 105 + 210 = 735.
	if bill.FinalTotal != 735 {
		t.Fatalf("retained lines should be recorded at cost + 5%%, got %.2f", bill.FinalTotal)
	}
	if plan.HiddenRevenue != 520000 || plan.RepricedOriginal != 180000 || plan.RepricedFinal != 73500 {
		t.Fatalf("unexpected plan money: %+v", plan)
	}
	if plan.UnchangedRevenue+plan.HiddenRevenue+plan.RepricedOriginal != plan.OriginalRevenue {
		t.Fatalf("revenue does not reconcile: %+v", plan)
	}
}

func TestCostMarkupPlanHidesABillOnlyWhenEveryLineIsCovered(t *testing.T) {
	// Both lines covered: nothing is left to bill for, so the bill itself goes.
	whole := cashBill("C1", "P-A", 100, 50, 1)
	addLine(whole, "2", "P-B", 100, 50, 1)
	source := reconciliationSource{Invoices: []*reconciliationInvoice{whole}}
	plan, err := applyCostMarkupPlan(&source, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !whole.WillSuppress || whole.WillReprice {
		t.Fatalf("a fully covered bill must disappear: %+v", whole)
	}
	if plan.HiddenInvoiceCount != 1 || plan.PartialInvoiceCount != 0 || plan.RepricedInvoiceCount != 0 {
		t.Fatalf("unexpected plan counts: %+v", plan)
	}
	if plan.HiddenRevenue != 20000 || plan.FinalRevenue() != 0 {
		t.Fatalf("unexpected plan money: %+v", plan)
	}
}

func TestCostMarkupPlanNeverRaisesAPriceAndFlagsMissingCost(t *testing.T) {
	giveaway := cashBill("C1", "P", 0, 10, 0)   // sold at 0: cost + markup is higher
	belowCost := cashBill("C2", "P", 40, 50, 0) // sold below cost + markup
	noCost := cashBill("C3", "P", 100, 0, 0)    // no cost known
	source := reconciliationSource{Invoices: []*reconciliationInvoice{giveaway, belowCost, noCost}}
	plan, err := applyCostMarkupPlan(&source, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, invoice := range source.Invoices {
		if !invoice.WillReprice || invoice.FinalTotal != invoice.TotalAmount || invoice.Items[0].Repriced || invoice.Items[0].NewUnitPrice != invoice.Items[0].UnitPrice {
			t.Fatalf("%s must keep its sold price: %+v item=%+v", invoice.ID, invoice, invoice.Items[0])
		}
	}
	if plan.AdjustedItemCount != 0 || plan.MissingCostItemCount != 1 || !noCost.Items[0].MissingCost {
		t.Fatalf("unexpected counts: %+v", plan)
	}
	if plan.FinalRevenue() != 14000 || plan.AdjustmentReduction() != 0 {
		t.Fatalf("target = %d, want 14000", plan.FinalRevenue())
	}
}

func TestMarkupBasisPointsAcceptsFiveToTenOnly(t *testing.T) {
	cases := []struct {
		percent     float64
		basisPoints int64
		used        float64
		ok          bool
	}{
		{0, 500, 5, true}, {5, 500, 5, true}, {7.5, 750, 7.5, true}, {10, 1000, 10, true},
		{4.99, 0, 0, false}, {10.01, 0, 0, false}, {5.005, 0, 0, false}, {-1, 0, 0, false},
	}
	for _, tc := range cases {
		basisPoints, used, err := markupBasisPoints(tc.percent)
		if (err == nil) != tc.ok || basisPoints != tc.basisPoints || used != tc.used {
			t.Errorf("markupBasisPoints(%v) = (%d, %v, %v), want (%d, %v, ok=%t)", tc.percent, basisPoints, used, err, tc.basisPoints, tc.used, tc.ok)
		}
	}
	if _, err := applyCostMarkupPlan(&reconciliationSource{}, 4); err == nil {
		t.Fatal("expected a markup below five percent to be rejected")
	}
}

func TestReconciliationPeriodUsesBangkokMonthBoundary(t *testing.T) {
	start, end, err := reconciliationPeriod("2026-08")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := start.Format(time.RFC3339), "2026-07-31T17:00:00Z"; got != want {
		t.Fatalf("start = %s, want %s", got, want)
	}
	if got, want := end.Format(time.RFC3339), "2026-08-31T17:00:00Z"; got != want {
		t.Fatalf("end = %s, want %s", got, want)
	}
}

func TestReconciliationOverviewGroupsReturnsCountsAndInvoiceRevenue(t *testing.T) {
	invoices := []*reconciliationInvoice{
		{TotalAmount: 244298.87, PaymentMethod: "cash", SuppressionCandidate: true},
		{TotalAmount: 1000, PaymentMethod: "cash", SuppressionCandidate: true, WillReprice: true},
		{TotalAmount: 20319.30, PaymentMethod: "cash", RequestFullTaxInvoice: true},
		{TotalAmount: 90259.39, PaymentMethod: "bank_transfer"},
		{TotalAmount: 13020.35, PaymentMethod: "mixed"},
		{TotalAmount: 10.01, PaymentMethod: "unpaid"},
	}

	groups := reconciliationOverviewGroups(invoices)
	tests := map[string]reconciliationOverviewGroup{
		"cash_hidden_ghost": {InvoiceCount: 1, Revenue: 244298.87},
		"cash_suppressed":   {InvoiceCount: 1, Revenue: 244298.87},
		"cash_repriced":     {InvoiceCount: 1, Revenue: 1000},
		"cash_adjustable":   {InvoiceCount: 1, Revenue: 1000},
		"cash_no_tax":       {InvoiceCount: 2, Revenue: 245298.87},
		"cash_full_tax":     {InvoiceCount: 1, Revenue: 20319.30},
		"bank_transfer":     {InvoiceCount: 1, Revenue: 90259.39},
		"mixed":             {InvoiceCount: 1, Revenue: 13020.35},
		"unclassified":      {InvoiceCount: 1, Revenue: 10.01},
	}
	for key, want := range tests {
		got := groups[key]
		if got != want {
			t.Errorf("group %s = %+v, want %+v", key, got, want)
		}
	}
}

func TestCompactedInvoiceNumberPreservesDocumentPrefix(t *testing.T) {
	if got, want := compactedInvoiceNumber("MES-BL2026080500317", 2), "MES-BL2026080500002"; got != want {
		t.Fatalf("compacted number = %q, want %q", got, want)
	}
}
