package purchasing

import (
	"testing"
)

func TestCalculatePurchaseAmountsVATModes(t *testing.T) {
	tests := []struct {
		name  string
		mode  string
		tax   float64
		total float64
	}{
		{name: "exclusive", mode: "exclusive", tax: 17.5, total: 267.5},
		{name: "inclusive", mode: "inclusive", tax: 16.36, total: 250},
		{name: "none", mode: "none", tax: 0, total: 250},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines := []resolvedPurchaseLine{
				{Input: PurchaseOrderLineInput{Quantity: 2, UnitCost: 100, LineDiscount: 10}},
				{Input: PurchaseOrderLineInput{Quantity: 1, UnitCost: 50}},
			}
			summary, err := calculatePurchaseAmounts(lines, test.mode, 7, 10, 20)
			if err != nil {
				t.Fatalf("calculate amounts: %v", err)
			}
			if summary.Subtotal != 250 || summary.LineDiscount != 10 || summary.Tax != test.tax || summary.Total != test.total {
				t.Fatalf("unexpected summary: %#v", summary)
			}
		})
	}
}

func TestPurchaseValidationAndDates(t *testing.T) {
	mode, rate, err := normalizeVAT("", 0)
	if err != nil || mode != "exclusive" || rate != 7 {
		t.Fatalf("expected default VAT 7%% exclusive, got %q %.2f %v", mode, rate, err)
	}
	if _, _, err := normalizeVAT("invalid", 7); err == nil {
		t.Fatal("expected invalid VAT mode to fail")
	}
	if _, err := parseExpiry("2026/08/05"); err == nil {
		t.Fatal("expected invalid expiry format to fail")
	}
	expiry, err := parseExpiry("2026-09-01")
	if err != nil || !expiry.Valid || expiry.Time.Format("2006-01-02") != "2026-09-01" {
		t.Fatalf("unexpected parsed expiry: %#v %v", expiry, err)
	}
	if _, err := calculatePurchaseAmounts([]resolvedPurchaseLine{{Input: PurchaseOrderLineInput{Quantity: 1, UnitCost: 10, LineDiscount: 11}}}, "none", 0, 0, 0); err == nil {
		t.Fatal("expected excessive line discount to fail")
	}
}

func TestCursorPaginationHelpers(t *testing.T) {
	for _, offset := range []int{0, 20, 40, 1234} {
		if got := decodeOffset(encodeOffset(offset)); got != offset {
			t.Fatalf("cursor round trip: want %d, got %d", offset, got)
		}
	}
	if got := decodeOffset("not-a-cursor"); got != 0 {
		t.Fatalf("invalid cursor should start at zero, got %d", got)
	}
	// A caller may now ask for more than one page — the suppliers screen pages
	// client-side and needs the whole list — so only 0/negative falls back to
	// the default, and 500 is the ceiling.
	if normalizeLimit(0) != 20 || normalizeLimit(-5) != 20 || normalizeLimit(7) != 7 {
		t.Fatal("unexpected cursor limit normalization")
	}
	if normalizeLimit(500) != 500 || normalizeLimit(5000) != 500 {
		t.Fatal("cursor limit should allow a full-list request but cap at 500")
	}
}
