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

// A sale brought in from the shop's previous system lands on the day it was
// sold: the bill's date and the money's received time are that day, while
// created_at stays the moment the row was written. Only superadmin may do it,
// and a branch's own level price beats the catalog's.
func TestHistoricalImportAndBranchLevelPricesAgainstConfiguredDatabase(t *testing.T) {
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
		Permissions: []string{"invoice.create.remote", "price.override.global"}}
	till := platform.AuthUser{ID: userID, RoleKey: "branch_pos", Portal: "pos", Scope: "branch", BranchID: &branchID,
		Permissions: []string{"invoice.create.pos"}}
	meta := audit.LogEntry{ActorID: &userID}
	auditService := audit.NewService(db)
	purchaseService := purchasing.NewService(db, auditService)
	salesService := NewService(db, auditService)

	unique := platform.MustUUID()
	supplierID, err := purchaseService.CreateSupplier(ctx, meta, purchasing.SupplierInput{SupplierCode: "HIS-" + unique[:8], LegalName: "History " + unique})
	if err != nil {
		t.Fatal(err)
	}
	poID, err := purchaseService.CreatePurchaseOrder(ctx, admin, meta, purchasing.PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, PurchasedAt: time.Now().UTC().Format(time.RFC3339), VATMode: "none",
		Items: []purchasing.PurchaseOrderLineInput{{NewProduct: &purchasing.NewProductInput{SKU: "HIS-" + unique[:8], Name: "History product",
			UnitName: "ชิ้น", BaseSellingPrice: 100}, StockBucket: "real", Quantity: 10, UnitCost: 50, LotNumber: "HIS-" + unique[:8]}},
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

	// Level 2 at 95 in the catalog, 90 at this branch: a level-1 member here
	// pays the branch's 90.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO product_level_prices (id, product_id, branch_id, unit_id, price_level, unit_price, created_at, updated_at)
		VALUES ($1,$2,NULL,NULL,2,95,NOW(),NOW()), ($3,$2,$4,NULL,2,90,NOW(),NOW())
	`, platform.MustUUID(), productID, platform.MustUUID(), branchID); err != nil {
		t.Fatal(err)
	}
	unit := unitInfo{Conversion: 1, Price: 100, IsBase: true}
	if price, level, ok, err := memberLevelPrice(ctx, db, productID, branchID, unit, 100, 2); err != nil || !ok || price != 90 || level != 2 {
		t.Fatalf("branch level price: %.2f L%d ok=%v err=%v, want 90 at L2", price, level, ok, err)
	}
	var otherBranch string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE id<>$1 ORDER BY created_at, id LIMIT 1`, branchID).Scan(&otherBranch); err != nil {
		t.Fatal(err)
	}
	if price, _, ok, err := memberLevelPrice(ctx, db, productID, otherBranch, unit, 100, 2); err != nil || !ok || price != 95 {
		t.Fatalf("another branch falls back to the catalog: %.2f ok=%v err=%v, want 95", price, ok, err)
	}

	soldAt := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	sale := CheckoutRequest{BranchID: branchID, PaymentType: "cash", TenderedAmount: 1000, SoldAt: soldAt.Format(time.RFC3339),
		Items: []LineInput{{ProductID: productID, InventoryLotID: lotID, Quantity: 1, StockBucket: "real"}}}
	if _, err := salesService.Checkout(ctx, till, meta, sale); err == nil {
		t.Fatal("a till must not bring in a past sale")
	}
	future := sale
	future.SoldAt = time.Now().Add(time.Hour).Format(time.RFC3339)
	if _, err := salesService.Checkout(ctx, admin, meta, future); err == nil {
		t.Fatal("a sale in the future must be refused")
	}
	result, err := salesService.Checkout(ctx, admin, meta, sale)
	if err != nil {
		t.Fatalf("import a past sale: %v", err)
	}
	invoiceID := result["invoice_id"].(string)
	var issuedAt, createdAt, receivedAt, paymentCreatedAt time.Time
	if err := db.QueryRowContext(ctx, `
		SELECT i.issued_at, i.created_at, p.received_at, p.created_at
		FROM invoices i JOIN invoice_payments p ON p.invoice_id=i.id WHERE i.id=$1
	`, invoiceID).Scan(&issuedAt, &createdAt, &receivedAt, &paymentCreatedAt); err != nil {
		t.Fatal(err)
	}
	if !issuedAt.Equal(soldAt) || !receivedAt.Equal(soldAt) {
		t.Fatalf("bill dated %v, money received %v, want both %v", issuedAt, receivedAt, soldAt)
	}
	if time.Since(createdAt) > time.Minute || time.Since(paymentCreatedAt) > time.Minute {
		t.Fatalf("created_at must stay the time the rows were written: %v / %v", createdAt, paymentCreatedAt)
	}
	var action string
	if err := db.QueryRowContext(ctx, `SELECT action FROM audit_logs WHERE entity_id=$1 ORDER BY created_at DESC LIMIT 1`, invoiceID).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != "pos.checkout.import" {
		t.Fatalf("audit action %q, want pos.checkout.import", action)
	}
}
