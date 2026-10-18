package sales

import (
	"context"
	"os"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/purchasing"
	"pharmacy-erp/backend/internal/platform"
)

// A shop that prices VAT-inclusive charges exactly the shelf price less the
// discounts typed, and the invoice still records before-VAT amounts.
func TestVATInclusivePricingAgainstConfiguredDatabase(t *testing.T) {
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

	var userID, branchID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE active AND sales_enabled ORDER BY created_at, id LIMIT 1`).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	admin := platform.AuthUser{ID: userID, RoleKey: "super_admin", Portal: "backoffice", Scope: "global",
		Permissions: []string{"sales.discount.line", "price.override.global"}}
	meta := audit.LogEntry{ActorID: &userID}
	auditService := audit.NewService(db)
	purchaseService := purchasing.NewService(db, auditService)
	salesService := NewService(db, auditService)

	unique := platform.MustUUID()
	supplierID, err := purchaseService.CreateSupplier(ctx, meta, purchasing.SupplierInput{SupplierCode: "VAT-" + unique[:8], LegalName: "VAT " + unique})
	if err != nil {
		t.Fatal(err)
	}
	newLine := func(sku string, price float64) purchasing.PurchaseOrderLineInput {
		return purchasing.PurchaseOrderLineInput{NewProduct: &purchasing.NewProductInput{SKU: sku, Name: "VAT " + sku, UnitName: "ชิ้น",
			BaseSellingPrice: price, MaxDiscountAmount: 1000}, StockBucket: "real", Quantity: 10, UnitCost: 100, LotNumber: sku}
	}
	poID, err := purchaseService.CreatePurchaseOrder(ctx, admin, meta, purchasing.PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, PurchasedAt: time.Now().UTC().Format(time.RFC3339), VATMode: "none",
		Items: []purchasing.PurchaseOrderLineInput{newLine("VATA-"+unique[:6], 6499), newLine("VATB-"+unique[:6], 3500)},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := purchaseService.GetPurchaseOrder(ctx, admin, poID)
	if err != nil {
		t.Fatal(err)
	}
	lines := []LineInput{}
	for _, item := range detail["items"].([]map[string]any) {
		productID := item["product_id"].(string)
		if _, err := db.ExecContext(ctx, `DELETE FROM branch_product_prices WHERE product_id=$1`, productID); err != nil {
			t.Fatal(err)
		}
		var lotID string
		if err := db.QueryRowContext(ctx, `SELECT id::text FROM inventory_lots WHERE product_id=$1 AND branch_id=$2`, productID, branchID).Scan(&lotID); err != nil {
			t.Fatal(err)
		}
		line := LineInput{ProductID: productID, InventoryLotID: lotID, Quantity: 1, StockBucket: "real"}
		var price float64
		if err := db.QueryRowContext(ctx, `SELECT base_selling_price FROM products WHERE id=$1`, productID).Scan(&price); err != nil {
			t.Fatal(err)
		}
		// The 6,499 item leads, whichever order the order lists them in.
		if price == 6499 {
			lines = append([]LineInput{line}, lines...)
		} else {
			lines = append(lines, line)
		}
	}

	var before string
	if err := db.QueryRowContext(ctx, `SELECT setting_value FROM app_settings WHERE setting_key='prices_include_vat'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), `UPDATE app_settings SET setting_value=$1 WHERE setting_key='prices_include_vat'`, before); err != nil {
			t.Errorf("restore setting: %v", err)
		}
	}()
	if _, err := db.ExecContext(ctx, `UPDATE app_settings SET setting_value='true' WHERE setting_key='prices_include_vat'`); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		lines    []LineInput
		discount float64
		total    float64
	}{
		{"one item at its shelf price", lines[:1], 0, 6499},
		{"a 500 baht bill discount comes off the price paid", lines[:1], 500, 5999},
		{"two items less 100", lines, 100, 9899},
	} {
		preview, err := salesService.PreviewSale(ctx, admin, branchID, false, tc.lines, tc.discount, cartOptions{})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		summary := preview["summary"].(map[string]any)
		total, subtotal, tax := summary["total_amount"].(float64), summary["subtotal"].(float64), summary["tax_amount"].(float64)
		if total != tc.total {
			t.Fatalf("%s: total %.2f, want %.2f", tc.name, total, tc.total)
		}
		if platform.Round2(subtotal+tax) != total {
			t.Fatalf("%s: subtotal %.2f + VAT %.2f does not make %.2f", tc.name, subtotal, tax, total)
		}
		if want := platform.Round2(total * 7 / 107); tax < want-0.02 || tax > want+0.02 {
			t.Fatalf("%s: VAT %.2f, want about %.2f", tc.name, tax, want)
		}
		if got := summary["discount_total"].(float64); got != tc.discount {
			t.Fatalf("%s: the till shows a discount of %.2f, the cashier typed %.2f", tc.name, got, tc.discount)
		}
		if summary["prices_include_vat"] != true {
			t.Fatalf("%s: preview must say prices include VAT", tc.name)
		}
	}
}
