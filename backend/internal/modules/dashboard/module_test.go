package dashboard

import (
	"bytes"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/platform"
)

func TestBangkokDayRange(t *testing.T) {
	start, end, label, err := bangkokDayRange("2026-07-14")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if label != "2026-07-14" {
		t.Fatalf("unexpected label: %s", label)
	}
	wantStart := time.Date(2026, 7, 13, 17, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantStart.Add(24*time.Hour)) {
		t.Fatalf("unexpected UTC range: %s - %s", start, end)
	}
}

func TestBangkokDayRangeRejectsInvalidDate(t *testing.T) {
	if _, _, _, err := bangkokDayRange("14/07/2026"); err == nil {
		t.Fatal("expected invalid date to fail")
	}
}

func TestBangkokDateRangeUsesInclusiveEndDate(t *testing.T) {
	start, end, startLabel, endLabel, err := bangkokDateRange("2026-07-01", "2026-07-07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if startLabel != "2026-07-01" || endLabel != "2026-07-07" {
		t.Fatalf("unexpected labels: %s - %s", startLabel, endLabel)
	}
	if end.Sub(start) != 7*24*time.Hour {
		t.Fatalf("expected seven inclusive days, got %s", end.Sub(start))
	}
}

func TestBangkokDateRangeRejectsReversedDates(t *testing.T) {
	if _, _, _, _, err := bangkokDateRange("2026-07-14", "2026-07-01"); err == nil {
		t.Fatal("expected reversed range to fail")
	}
}

func TestSalesExportsProduceRealPDFAndXLSX(t *testing.T) {
	result := salesSummaryResult{
		StartLabel:   "2026-07-01",
		EndLabel:     "2026-07-07",
		TotalSales:   107,
		InvoiceCount: 1,
		CashReceived: 107,
		Invoices: []map[string]any{{
			"invoice_number": "BL-001", "customer_name": "ลูกค้าทดสอบ", "branch_name": "สาขาทดสอบ",
			"payment_status": "paid", "issued_at": time.Date(2026, 7, 1, 4, 0, 0, 0, time.UTC), "total_amount": 107.0,
		}},
	}
	user := platform.AuthUser{Name: "พนักงานทดสอบ"}
	pdf, err := buildSalesPDF(result, user)
	if err != nil {
		t.Fatalf("build PDF: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("expected PDF signature, got %q", pdf[:min(8, len(pdf))])
	}
	xlsx, err := buildSalesXLSX(result, user)
	if err != nil {
		t.Fatalf("build XLSX: %v", err)
	}
	if !bytes.HasPrefix(xlsx, []byte("PK")) {
		t.Fatalf("expected ZIP/XLSX signature, got %q", xlsx[:min(8, len(xlsx))])
	}
}
