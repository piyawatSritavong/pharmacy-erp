package returns

import (
	"context"
	"os"
	"strings"
	"testing"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"
)

func ghostClaimFixture(t *testing.T) (*Service, context.Context, platform.AuthUser, string, string) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.Open(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	var actorID, warehouseID, productID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local'`).Scan(&actorID); err != nil {
		t.Fatalf("load actor: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE branch_type='main_warehouse' AND active=TRUE`).Scan(&warehouseID); err != nil {
		t.Fatalf("load warehouse: %v", err)
	}
	// Ghost lives only at the warehouse, so stage a product there rather than
	// depending on whatever the seed happens to hold.
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM products ORDER BY sku LIMIT 1`).Scan(&productID); err != nil {
		t.Fatalf("load product: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory (id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at)
		VALUES (gen_random_uuid(),$1,$2,0,0,NOW(),NOW())
		ON CONFLICT (branch_id,product_id) DO NOTHING
	`, warehouseID, productID); err != nil {
		t.Fatalf("seed inventory row: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE inventory SET qty_ghost=qty_ghost+50, qty_real=qty_real+50, updated_at=NOW()
		WHERE branch_id=$1 AND product_id=$2
	`, warehouseID, productID); err != nil {
		t.Fatalf("stage ghost stock: %v", err)
	}
	// inventory.qty_* must always equal the sum of its movements — the ledger
	// invariant TestIntegrationHarness enforces — so staged stock is booked in
	// rather than conjured onto the row.
	for _, bucket := range []string{"real", "ghost"} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at)
			VALUES (gen_random_uuid(),$1,$2,'opening_balance',$3,50,'operational_reset',NULL,'ตั้งค่าสต๊อกสำหรับเทสเคลม',$4,NOW())
		`, warehouseID, productID, bucket, actorID); err != nil {
			t.Fatalf("book staged %s stock: %v", bucket, err)
		}
		// A claim takes goods off lots, FEFO, so the staged quantity has to sit
		// on one — inventory and lots are two views of the same shelf.
		if _, err := db.ExecContext(ctx, `
			INSERT INTO inventory_lots (id,branch_id,product_id,stock_bucket,lot_number,received_quantity,remaining_quantity,unit_cost,source_type,received_at,created_at,updated_at)
			VALUES (gen_random_uuid(),$1,$2,$3,'CLAIM-FIXTURE',50,50,10,'operational_reset',NOW(),NOW(),NOW())
		`, warehouseID, productID, bucket); err != nil {
			t.Fatalf("stage %s lot: %v", bucket, err)
		}
	}

	service := NewService(db, audit.NewService(db))
	superadmin := platform.AuthUser{
		ID: actorID, RoleKey: "super_admin", Scope: "global", Portal: "backoffice",
		Permissions: []string{"returns.manage", "invoice.view"},
	}
	return service, ctx, superadmin, warehouseID, productID
}

func ghostQty(t *testing.T, service *Service, ctx context.Context, branchID, productID string) (int, int) {
	t.Helper()
	var ghost, real int
	if err := service.db.QueryRowContext(ctx,
		`SELECT qty_ghost, qty_real FROM inventory WHERE branch_id=$1 AND product_id=$2`,
		branchID, productID).Scan(&ghost, &real); err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	return ghost, real
}

// inventory.qty_ghost is what the month-end close reads for coverage; the lots
// are what the shelf actually holds. A claim that moved one without the other
// would leave the two disagreeing.
func assertGhostLotsMatchInventory(t *testing.T, service *Service, ctx context.Context, branchID, productID, stage string) {
	t.Helper()
	var onRow, onLots int
	if err := service.db.QueryRowContext(ctx, `
		SELECT i.qty_ghost, COALESCE((
			SELECT SUM(l.remaining_quantity) FROM inventory_lots l
			WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='ghost'
		), 0)
		FROM inventory i WHERE i.branch_id=$1 AND i.product_id=$2
	`, branchID, productID).Scan(&onRow, &onLots); err != nil {
		t.Fatalf("read ghost lots: %v", err)
	}
	if onRow != onLots {
		t.Fatalf("%s: inventory holds %d ghost but lots hold %d", stage, onRow, onLots)
	}
}

// The whole point of the feature: a defective Ghost unit can leave inventory on
// a claim, and the supplier's replacement comes back into Ghost — never into
// real stock, which would quietly mint stock the books do not know about.
func TestGhostClaimDeductsGhostAndRestocksGhost(t *testing.T) {
	service, ctx, superadmin, warehouseID, productID := ghostClaimFixture(t)
	ghostBefore, realBefore := ghostQty(t, service, ctx, warehouseID, productID)

	returnID, err := service.InitiateStockClaim(ctx, superadmin, audit.LogEntry{}, InitiateStockClaimInput{
		BranchID: warehouseID, ProductID: productID, StockBucket: "ghost", Quantity: 6,
		Reason: "ล็อตที่รับเข้ามาชำรุด",
	})
	if err != nil {
		t.Fatalf("initiate ghost claim: %v", err)
	}
	assertGhostLotsMatchInventory(t, service, ctx, warehouseID, productID, "หลังตัดสต๊อกส่งเคลม")
	ghostAfter, realAfter := ghostQty(t, service, ctx, warehouseID, productID)
	if ghostAfter != ghostBefore-6 {
		t.Fatalf("ghost should fall by 6: %d -> %d", ghostBefore, ghostAfter)
	}
	if realAfter != realBefore {
		t.Fatalf("real stock must not move: %d -> %d", realBefore, realAfter)
	}

	var supplierID string
	if err := service.db.QueryRowContext(ctx, `SELECT id::text FROM suppliers LIMIT 1`).Scan(&supplierID); err != nil {
		t.Fatalf("load supplier: %v", err)
	}
	if err := service.SendToSupplier(ctx, superadmin, audit.LogEntry{}, returnID, SendToSupplierInput{SupplierID: supplierID}); err != nil {
		t.Fatalf("send to supplier: %v", err)
	}
	if err := service.ResolveCaseA(ctx, superadmin, audit.LogEntry{}, returnID); err != nil {
		t.Fatalf("resolve case A: %v", err)
	}
	assertGhostLotsMatchInventory(t, service, ctx, warehouseID, productID, "หลังรับคืนจากคู่ค้า")
	ghostRestocked, realRestocked := ghostQty(t, service, ctx, warehouseID, productID)
	if ghostRestocked != ghostBefore {
		t.Fatalf("ghost should return to %d, got %d", ghostBefore, ghostRestocked)
	}
	if realRestocked != realBefore {
		t.Fatalf("real stock must still not move: %d -> %d", realBefore, realRestocked)
	}
}

// Both directions of the audit trail: the claim names its stock movements, and
// each movement names the claim through reference_id.
func TestGhostClaimIsTraceableInBothDirections(t *testing.T) {
	service, ctx, superadmin, warehouseID, productID := ghostClaimFixture(t)
	returnID, err := service.InitiateStockClaim(ctx, superadmin, audit.LogEntry{}, InitiateStockClaimInput{
		BranchID: warehouseID, ProductID: productID, StockBucket: "ghost", Quantity: 2, Reason: "แตกระหว่างขนส่ง",
	})
	if err != nil {
		t.Fatalf("initiate ghost claim: %v", err)
	}

	trace, err := service.Trace(ctx, superadmin, returnID)
	if err != nil {
		t.Fatalf("trace: %v", err)
	}
	claim := trace["claim"].(map[string]any)
	if claim["origin"] != "stock_claim" || claim["stock_bucket"] != "ghost" {
		t.Fatalf("claim should be a ghost stock claim, got %#v", claim)
	}
	movements := trace["movements"].([]map[string]any)
	if len(movements) != 1 || movements[0]["quantity_delta"].(int) != -2 || movements[0]["stock_bucket"] != "ghost" {
		t.Fatalf("expected one ghost withdrawal of 2, got %#v", movements)
	}
	if events := trace["events"].([]map[string]any); len(events) != 1 || events[0]["status"] != "pending_claim" {
		t.Fatalf("expected the opening event, got %#v", events)
	}

	var backReference string
	if err := service.db.QueryRowContext(ctx, `
		SELECT reference_id::text FROM inventory_movements
		WHERE reference_type='product_return' AND reference_id=$1 AND stock_bucket='ghost'
	`, returnID).Scan(&backReference); err != nil {
		t.Fatalf("movement should point back at the claim: %v", err)
	}
	if backReference != returnID {
		t.Fatalf("expected the movement to name claim %s, got %s", returnID, backReference)
	}
}

// central_admin holds real stock and only real stock — a Ghost claim must not
// be creatable, readable, or even confirmable as existing.
func TestGhostClaimIsInvisibleToCentralAdmin(t *testing.T) {
	service, ctx, superadmin, warehouseID, productID := ghostClaimFixture(t)
	returnID, err := service.InitiateStockClaim(ctx, superadmin, audit.LogEntry{}, InitiateStockClaimInput{
		BranchID: warehouseID, ProductID: productID, StockBucket: "ghost", Quantity: 1, Reason: "ชำรุด",
	})
	if err != nil {
		t.Fatalf("initiate ghost claim: %v", err)
	}
	central := platform.AuthUser{
		ID: superadmin.ID, RoleKey: "central_admin", Scope: "global", Portal: "backoffice",
		Permissions: []string{"returns.manage", "invoice.view"},
	}

	if _, err := service.InitiateStockClaim(ctx, central, audit.LogEntry{}, InitiateStockClaimInput{
		BranchID: warehouseID, ProductID: productID, StockBucket: "ghost", Quantity: 1, Reason: "ชำรุด",
	}); err == nil || !strings.Contains(err.Error(), "สต๊อกผี") {
		t.Fatalf("central_admin must not raise a ghost claim, got %v", err)
	}
	if _, err := service.Trace(ctx, central, returnID); err == nil {
		t.Fatalf("central_admin must not be able to trace a ghost claim")
	}
	items, err := service.List(ctx, central, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, item := range items {
		if item["id"] == returnID {
			t.Fatalf("ghost claim leaked into the central_admin list")
		}
		if _, ok := item["stock_bucket"]; ok {
			t.Fatalf("central_admin must not be told which bucket a claim sits in: %#v", item)
		}
	}

	// And real stock stays its ordinary business.
	if _, err := service.InitiateStockClaim(ctx, central, audit.LogEntry{}, InitiateStockClaimInput{
		BranchID: warehouseID, ProductID: productID, StockBucket: "real", Quantity: 1, Reason: "กล่องบุบ",
	}); err != nil {
		t.Fatalf("central_admin should be able to claim real stock: %v", err)
	}
}

// Ghost is a warehouse-only bucket, so a branch claim on it is turned away with
// a reason rather than surfacing as a trigger failure.
func TestGhostClaimIsRefusedAwayFromTheWarehouse(t *testing.T) {
	service, ctx, superadmin, _, productID := ghostClaimFixture(t)
	var branchID string
	if err := service.db.QueryRowContext(ctx,
		`SELECT id::text FROM branches WHERE branch_type='branch' AND active=TRUE LIMIT 1`).Scan(&branchID); err != nil {
		t.Fatalf("load branch: %v", err)
	}
	_, err := service.InitiateStockClaim(ctx, superadmin, audit.LogEntry{}, InitiateStockClaimInput{
		BranchID: branchID, ProductID: productID, StockBucket: "ghost", Quantity: 1, Reason: "ชำรุด",
	})
	if err == nil || !strings.Contains(err.Error(), "โกดังใหญ่") {
		t.Fatalf("expected a warehouse-only refusal, got %v", err)
	}
}
