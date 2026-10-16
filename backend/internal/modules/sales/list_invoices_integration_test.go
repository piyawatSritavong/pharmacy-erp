package sales

import (
	"context"
	"os"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"
)

// The sales history pages and filters on the server: a filtered page must
// agree with the bills themselves, and dates are Bangkok business days.
func TestListInvoicesPageFiltersOnTheServer(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.OpenTest(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	service := NewService(db, audit.NewService(db))
	admin := platform.AuthUser{RoleKey: "super_admin", Portal: "backoffice", Scope: "global"}

	all, total, err := service.ListInvoicesPage(ctx, admin, nil, InvoiceListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Skip("no bills to list; the sales integration tests create them")
	}
	if total != len(all) {
		t.Fatalf("unpaged total %d, rows %d", total, len(all))
	}

	page, pageTotal, err := service.ListInvoicesPage(ctx, admin, nil, InvoiceListFilter{Page: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || pageTotal != total {
		t.Fatalf("one-row page: got %d rows of %d, want 1 of %d", len(page), pageTotal, total)
	}
	if page[0]["id"] != all[0]["id"] {
		t.Fatal("the first page must start with the newest bill")
	}

	target := all[0]
	number := target["invoice_number"].(string)
	found, foundTotal, err := service.ListInvoicesPage(ctx, admin, nil, InvoiceListFilter{Search: number, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if foundTotal < 1 || found[0]["invoice_number"] != number {
		t.Fatalf("searching %s found %v", number, found)
	}

	bangkok := time.FixedZone("Asia/Bangkok", 7*60*60)
	day := target["issued_at"].(time.Time).In(bangkok)
	sameDay, _, err := service.ListInvoicesPage(ctx, admin, nil, InvoiceListFilter{Search: number, DateFrom: day.Format("2006-01-02"), DateTo: day.Format("2006-01-02"), PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(sameDay) == 0 {
		t.Fatal("a bill must be inside its own Bangkok day")
	}
	nextDay := day.AddDate(0, 0, 1).Format("2006-01-02")
	later, _, err := service.ListInvoicesPage(ctx, admin, nil, InvoiceListFilter{Search: number, DateFrom: nextDay, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range later {
		if row["id"] == target["id"] {
			t.Fatal("a bill must not appear in the days after it")
		}
	}

	if _, _, err := service.ListInvoicesPage(ctx, admin, nil, InvoiceListFilter{DateFrom: "17/10/2026"}); err == nil {
		t.Fatal("a malformed date must be refused")
	}
	literal, _, err := service.ListInvoicesPage(ctx, admin, nil, InvoiceListFilter{Search: "%", PageSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range literal {
		if !containsPercent(row) {
			t.Fatalf("a %% search must match a literal %%, matched %v", row["invoice_number"])
		}
	}
}

func containsPercent(row map[string]any) bool {
	for _, key := range []string{"invoice_number", "customer_name", "original_invoice_number", "customer_code"} {
		if value, _ := row[key].(string); len(value) > 0 {
			for _, r := range value {
				if r == '%' {
					return true
				}
			}
		}
	}
	return false
}
