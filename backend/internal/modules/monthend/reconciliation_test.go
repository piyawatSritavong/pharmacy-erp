package monthend

import (
	"context"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/platform"
)

func TestReconciliationOverviewRejectsNonSuperadminBeforeDatabaseAccess(t *testing.T) {
	service := &Service{}
	if _, err := service.ReconciliationOverview(context.Background(), platform.AuthUser{RoleKey: "admin"}, ReconciliationInput{Month: "2026-08"}); err == nil {
		t.Fatal("expected non-superadmin overview access to be rejected")
	}
}

func TestApplyReconciliationAdjustmentsSelectsStockBucket(t *testing.T) {
	invoice := &reconciliationInvoice{ID: "invoice-1", AdjustmentEligible: true, TotalAmount: 19000}
	realItem := &reconciliationItem{ID: "item-real", InvoiceID: invoice.ID, Quantity: 1, UnitPrice: 16000, LineTotal: 16000}
	ghostItem := &reconciliationItem{ID: "item-ghost", InvoiceID: invoice.ID, Quantity: 2, UnitPrice: 1500, LineTotal: 3000}
	invoice.Items = []*reconciliationItem{realItem, ghostItem}
	source := reconciliationSource{
		InvoiceByID: map[string]*reconciliationInvoice{invoice.ID: invoice},
		ItemByID: map[string]*reconciliationItem{
			realItem.ID:  realItem,
			ghostItem.ID: ghostItem,
		},
	}

	reduction, count, err := applyReconciliationAdjustments(&source, []ReconciliationAdjustmentInput{
		{InvoiceItemID: realItem.ID, NewUnitPrice: 9000},
		{InvoiceItemID: ghostItem.ID, NewUnitPrice: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || reduction != 1_000_000 {
		t.Fatalf("unexpected adjustment count/reduction: %d/%d", count, reduction)
	}
	if realItem.DeductBucket != "real" || ghostItem.DeductBucket != "ghost" {
		t.Fatalf("unexpected stock buckets: %q/%q", realItem.DeductBucket, ghostItem.DeductBucket)
	}
	if invoice.FinalTotal != 9000 {
		t.Fatalf("unexpected final invoice total: %.2f", invoice.FinalTotal)
	}
}

func TestApplyReconciliationAdjustmentsRejectsIneligibleAndPriceIncrease(t *testing.T) {
	item := &reconciliationItem{ID: "item-1", InvoiceID: "invoice-1", Quantity: 1, UnitPrice: 100, LineTotal: 100}
	source := reconciliationSource{
		InvoiceByID: map[string]*reconciliationInvoice{"invoice-1": {ID: "invoice-1"}},
		ItemByID:    map[string]*reconciliationItem{item.ID: item},
	}
	if _, _, err := applyReconciliationAdjustments(&source, []ReconciliationAdjustmentInput{{InvoiceItemID: item.ID, NewUnitPrice: 90}}); err == nil {
		t.Fatal("expected an ineligible invoice error")
	}
	source.InvoiceByID["invoice-1"].AdjustmentEligible = true
	if _, _, err := applyReconciliationAdjustments(&source, []ReconciliationAdjustmentInput{{InvoiceItemID: item.ID, NewUnitPrice: 110}}); err == nil {
		t.Fatal("expected a price increase error")
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
		{TotalAmount: 20319.30, PaymentMethod: "cash", RequestFullTaxInvoice: true},
		{TotalAmount: 90259.39, PaymentMethod: "bank_transfer"},
		{TotalAmount: 13020.35, PaymentMethod: "mixed"},
		{TotalAmount: 10.01, PaymentMethod: "unpaid"},
	}

	groups := reconciliationOverviewGroups(invoices)
	tests := map[string]reconciliationOverviewGroup{
		"cash_suppressed": {InvoiceCount: 1, Revenue: 244298.87},
		"cash_adjustable": {InvoiceCount: 1, Revenue: 244298.87},
		"cash_full_tax":   {InvoiceCount: 1, Revenue: 20319.30},
		"bank_transfer":   {InvoiceCount: 1, Revenue: 90259.39},
		"mixed":           {InvoiceCount: 1, Revenue: 13020.35},
		"unclassified":    {InvoiceCount: 1, Revenue: 10.01},
	}
	for key, want := range tests {
		got := groups[key]
		if got != want {
			t.Errorf("group %s = %+v, want %+v", key, got, want)
		}
	}
}

func TestAutomaticReconciliationPlanSplitsEligiblePoolAndCapsDiscount(t *testing.T) {
	hidden := &reconciliationInvoice{
		ID: "eligible-100", TotalAmount: 100, PaymentMethod: "cash",
		SuppressionCandidate: true, AdjustmentEligible: true,
		Items: []*reconciliationItem{{ID: "item-100", InvoiceID: "eligible-100", Quantity: 1, UnitPrice: 100, LineTotal: 100}},
	}
	adjusted := &reconciliationInvoice{
		ID: "eligible-200", TotalAmount: 200, PaymentMethod: "cash",
		SuppressionCandidate: true, AdjustmentEligible: true,
		Items: []*reconciliationItem{{ID: "item-200", InvoiceID: "eligible-200", Quantity: 1, UnitPrice: 200, LineTotal: 200}},
	}
	protected := &reconciliationInvoice{ID: "full-tax", TotalAmount: 500, PaymentMethod: "cash", RequestFullTaxInvoice: true}
	source := reconciliationSource{OriginalRevenue: 80000, Invoices: []*reconciliationInvoice{hidden, adjusted, protected}}

	suppressed, reduction, adjustedCount, err := applyAutomaticReconciliationPlan(&source, 69000, 5)
	if err != nil {
		t.Fatal(err)
	}
	if suppressed != 10000 || reduction != 1000 || adjustedCount != 1 {
		t.Fatalf("unexpected plan amounts/count: suppressed=%d reduction=%d adjusted=%d", suppressed, reduction, adjustedCount)
	}
	if !hidden.WillSuppress || adjusted.WillSuppress || protected.WillSuppress {
		t.Fatalf("eligible pool was not split exclusively: hidden=%t adjusted=%t protected=%t", hidden.WillSuppress, adjusted.WillSuppress, protected.WillSuppress)
	}
	if adjusted.Items[0].NewUnitPrice != 190 {
		t.Fatalf("adjusted unit price = %.2f, want 190.00", adjusted.Items[0].NewUnitPrice)
	}
	if effective := (adjusted.Items[0].UnitPrice - adjusted.Items[0].NewUnitPrice) / adjusted.Items[0].UnitPrice * 100; effective > 5.000001 {
		t.Fatalf("effective discount %.4f exceeds five percent", effective)
	}
}

func TestAutomaticReconciliationPlanRejectsPercentOverFive(t *testing.T) {
	source := reconciliationSource{}
	if _, _, _, err := applyAutomaticReconciliationPlan(&source, 0, 5.01); err == nil {
		t.Fatal("expected adjustment percent above five to be rejected")
	}
}

func TestCompactedInvoiceNumberPreservesDocumentPrefix(t *testing.T) {
	if got, want := compactedInvoiceNumber("MES-BL2026080500317", 2), "MES-BL2026080500002"; got != want {
		t.Fatalf("compacted number = %q, want %q", got, want)
	}
}
