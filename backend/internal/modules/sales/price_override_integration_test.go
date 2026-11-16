package sales

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/purchasing"
	"pharmacy-erp/backend/internal/platform"
)

// The owner phones a branch: "sell it at 399". The till changes the price of
// a 450 item with a reason, even past the branch's discount floor; without a
// reason, or without the right to change prices, it cannot.
func TestPOSPriceOverrideAgainstConfiguredDatabase(t *testing.T) {
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
		Permissions: []string{"price.override.global"}}
	till := platform.AuthUser{ID: userID, RoleKey: "branch_pos", Portal: "pos", Scope: "branch", BranchID: &branchID,
		Permissions: []string{"invoice.create.pos", "price.override.pos"}}
	plainTill := till
	plainTill.Permissions = []string{"invoice.create.pos"}
	meta := audit.LogEntry{ActorID: &userID}
	auditService := audit.NewService(db)
	purchaseService := purchasing.NewService(db, auditService)
	salesService := NewService(db, auditService)

	unique := platform.MustUUID()
	supplierID, err := purchaseService.CreateSupplier(ctx, meta, purchasing.SupplierInput{SupplierCode: "OVR-" + unique[:8], LegalName: "Override " + unique})
	if err != nil {
		t.Fatal(err)
	}
	// No discount allowed on the shelf: the floor is the shelf price itself.
	poID, err := purchaseService.CreatePurchaseOrder(ctx, admin, meta, purchasing.PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, PurchasedAt: time.Now().UTC().Format(time.RFC3339), VATMode: "none",
		Items: []purchasing.PurchaseOrderLineInput{{NewProduct: &purchasing.NewProductInput{SKU: "OVR-" + unique[:8], Name: "Override diaper",
			UnitName: "แพ็ค", BaseSellingPrice: 450}, StockBucket: "real", Quantity: 10, UnitCost: 300, LotNumber: "OVR-" + unique[:8]}},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := purchaseService.GetPurchaseOrder(ctx, admin, poID)
	if err != nil {
		t.Fatal(err)
	}
	productID := detail["items"].([]map[string]any)[0]["product_id"].(string)
	if _, err := db.ExecContext(ctx, `DELETE FROM branch_product_prices WHERE product_id=$1`, productID); err != nil {
		t.Fatal(err)
	}
	var lotID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM inventory_lots WHERE product_id=$1 AND branch_id=$2`, productID, branchID).Scan(&lotID); err != nil {
		t.Fatal(err)
	}

	price := 399.0
	line := func(reason string) []LineInput {
		return []LineInput{{ProductID: productID, InventoryLotID: lotID, Quantity: 1, StockBucket: "real",
			OverrideUnitPrice: &price, OverrideReason: reason}}
	}
	preview := func(user platform.AuthUser, reason string) (map[string]any, error) {
		return salesService.PreviewSale(ctx, user, branchID, false, line(reason), 0, cartOptions{})
	}

	if _, err := preview(till, ""); err == nil || !strings.Contains(err.Error(), "เหตุผล") {
		t.Fatalf("a till changing a price without a reason: %v, want a reason demanded", err)
	}
	if _, err := preview(plainTill, "เจ้าของโทรสั่ง"); err == nil {
		t.Fatal("a till without price.override.pos must not change a price")
	}
	got, err := preview(till, "เจ้าของโทรสั่ง")
	if err != nil {
		t.Fatalf("a reasoned override past the floor: %v", err)
	}
	lines := got["lines"].([]map[string]any)
	if lines[0]["sold_unit_price"].(float64) != 399 || lines[0]["price_source"] != "override" {
		t.Fatalf("line priced %v via %v, want 399 via override", lines[0]["sold_unit_price"], lines[0]["price_source"])
	}
	if _, ok := lines[0]["cost_snapshot"]; ok {
		t.Fatal("the till must not see cost")
	}
	// Selling above the shelf is a price change too (a dearer size).
	price = 499
	if got, err := preview(till, "ราคาตามขนาด/ไซซ์"); err != nil || got["lines"].([]map[string]any)[0]["sold_unit_price"].(float64) != 499 {
		t.Fatalf("override above the shelf: %v", err)
	}
	price = -1
	if _, err := preview(till, "ผิด"); err == nil {
		t.Fatal("a negative price must be refused")
	}
}
