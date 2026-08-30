package purchasing

import (
	"context"
	"os"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"
)

func TestPurchaseOrderAtomicStockLifecycleAgainstConfiguredDatabase(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.Open(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	var userID, branchID, productID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local'`).Scan(&userID); err != nil {
		t.Fatalf("load super admin: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT i.branch_id::text, p.id::text
		FROM inventory i INNER JOIN products p ON p.id=i.product_id
		WHERE p.active=TRUE ORDER BY i.branch_id,p.id LIMIT 1
	`).Scan(&branchID, &productID); err != nil {
		t.Fatalf("load branch product membership: %v", err)
	}

	service := NewService(db, audit.NewService(db))
	user := platform.AuthUser{ID: userID, RoleKey: "super_admin"}
	meta := audit.LogEntry{ActorID: &userID}
	unique := platform.MustUUID()
	supplierID, err := service.CreateSupplier(ctx, meta, SupplierInput{SupplierCode: "TEST-" + unique[:8], LegalName: "Integration Supplier " + unique})
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	if _, err := service.CreateSupplier(ctx, meta, SupplierInput{SupplierCode: "OTHER-" + unique[:8], LegalName: "Integration Supplier " + unique}); err == nil {
		t.Fatal("expected case-insensitive supplier name uniqueness")
	}

	var before int
	if err := db.QueryRowContext(ctx, `SELECT qty_real FROM inventory WHERE branch_id=$1 AND product_id=$2`, branchID, productID).Scan(&before); err != nil {
		t.Fatalf("load starting inventory: %v", err)
	}
	poID, err := service.CreatePurchaseOrder(ctx, user, meta, PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, PurchasedAt: time.Now().UTC().Format(time.RFC3339), VATMode: "exclusive", VATRate: 7,
		Items: []PurchaseOrderLineInput{{ProductID: productID, StockBucket: "real", Quantity: 2, UnitCost: 12.5, LotNumber: "INT-" + unique[:8], ExpiresOn: time.Now().AddDate(1, 0, 0).Format("2006-01-02")}},
	})
	if err != nil {
		t.Fatalf("create purchase order: %v", err)
	}
	detail, err := service.GetPurchaseOrder(ctx, platform.AuthUser{RoleKey: "super_admin"}, poID)
	if err != nil {
		t.Fatalf("get purchase order: %v", err)
	}
	lines := detail["items"].([]map[string]any)
	if len(lines) != 1 || lines[0]["remaining_quantity"].(int) != 2 {
		t.Fatalf("unexpected posted lot: %#v", lines)
	}
	var afterCreate, lotCount, movementCount int
	if err := db.QueryRowContext(ctx, `SELECT qty_real FROM inventory WHERE branch_id=$1 AND product_id=$2`, branchID, productID).Scan(&afterCreate); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inventory_lots WHERE source_id=$1 AND source_type='purchase_order'`, poID).Scan(&lotCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inventory_movements WHERE reference_type='purchase_order' AND reference_id=$1`, poID).Scan(&movementCount); err != nil {
		t.Fatal(err)
	}
	if afterCreate != before+2 || lotCount != 1 || movementCount != 1 {
		t.Fatalf("PO was not posted atomically: before=%d after=%d lots=%d movements=%d", before, afterCreate, lotCount, movementCount)
	}

	lineID := lines[0]["id"].(string)
	if err := service.UpdatePurchaseOrder(ctx, user, meta, poID, PurchaseOrderInput{VATMode: "exclusive", VATRate: 7, CorrectionReason: "integration correction", Items: []PurchaseOrderLineInput{{ID: lineID, Quantity: 3, UnitCost: 12.5, LotNumber: "INT-" + unique[:8], ExpiresOn: time.Now().AddDate(1, 0, 0).Format("2006-01-02")}}}); err != nil {
		t.Fatalf("correct purchase order: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT qty_real FROM inventory WHERE branch_id=$1 AND product_id=$2`, branchID, productID).Scan(&afterCreate); err != nil || afterCreate != before+3 {
		t.Fatalf("correction did not update inventory: qty=%d err=%v", afterCreate, err)
	}
	if err := service.CancelPurchaseOrder(ctx, user, meta, poID, "integration cleanup"); err != nil {
		t.Fatalf("cancel purchase order: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT qty_real FROM inventory WHERE branch_id=$1 AND product_id=$2`, branchID, productID).Scan(&afterCreate); err != nil || afterCreate != before {
		t.Fatalf("cancellation did not restore inventory: qty=%d err=%v", afterCreate, err)
	}

	rollbackSKU := "ROLLBACK-" + unique[:8]
	_, err = service.CreatePurchaseOrder(ctx, user, meta, PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, VATMode: "none",
		Items: []PurchaseOrderLineInput{
			{NewProduct: &NewProductInput{SKU: rollbackSKU, Name: "Rollback product", UnitName: "ชิ้น", BaseSellingPrice: 20}, StockBucket: "real", Quantity: 1, UnitCost: 10},
			{ProductID: platform.MustUUID(), StockBucket: "real", Quantity: 1, UnitCost: 10},
		},
	})
	if err == nil {
		t.Fatal("expected invalid second line to roll back the PO")
	}
	var rolledBackProducts int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE sku=$1`, rollbackSKU).Scan(&rolledBackProducts); err != nil || rolledBackProducts != 0 {
		t.Fatalf("inline product was not rolled back: count=%d err=%v", rolledBackProducts, err)
	}
}
