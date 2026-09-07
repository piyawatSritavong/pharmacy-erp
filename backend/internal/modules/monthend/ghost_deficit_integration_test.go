package monthend

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"
	"pharmacy-erp/backend/migrations"

	_ "github.com/lib/pq"
)

// TestMonthEndRecordsCashBillWithoutGhostStockAtCostMarkup runs the fixture
// whose warehouse holds no Ghost Stock at all. Under the cost-markup rule the
// cash bill is neither hidden nor written as a deficit: it stays in the books
// at cost × 1.05 (10 → 10.50) and no stock moves. The deficit ledger itself
// stays append-only.
func TestMonthEndRecordsCashBillWithoutGhostStockAtCostMarkup(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	adminDB, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer adminDB.Close()

	schema := fmt.Sprintf("month_end_deficit_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = adminDB.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) })

	schemaURL, err := monthEndTestDatabaseURL(databaseURL, schema)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	if err := applyAllMonthEndTestMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx, monthEndDeficitFixtureSQL); err != nil {
		t.Fatalf("create deficit fixture: %v", err)
	}

	const actorID = "20000000-0000-4000-8000-000000000001"
	const warehouseID = "30000000-0000-4000-8000-000000000001"
	const branchID = "30000000-0000-4000-8000-000000000002"
	const productID = "40000000-0000-4000-8000-000000000001"
	const invoiceID = "70000000-0000-4000-8000-000000000001"
	const itemID = "71000000-0000-4000-8000-000000000001"
	service := NewService(db, audit.NewService(db))
	result, err := service.FinalizeReconciliation(ctx, platform.AuthUser{
		ID: actorID, RoleKey: "super_admin", Portal: "backoffice", Scope: "global",
	}, audit.LogEntry{}, ReconciliationInput{
		DateFrom: "2097-03-01", DateTo: "2097-03-31", BranchIDs: []string{branchID}, AdjustmentPercent: 5,
	})
	if err != nil {
		t.Fatalf("finalize zero-Ghost reconciliation: %v", err)
	}
	reconciliationID, ok := result["id"].(string)
	if !ok || reconciliationID == "" {
		t.Fatalf("missing reconciliation ID: %+v", result)
	}
	if result["suppressed_invoice_count"] != 0 || result["adjusted_item_count"] != 1 || result["final_revenue"] != 10.5 || result["adjustment_reduction"] != 9.5 || result["reconciliation_mode"] != reconciliationModeCostMarkup {
		t.Fatalf("unexpected reconciliation record: %+v", result)
	}

	var warehouseReal, warehouseGhost int
	if err := db.QueryRowContext(ctx, `SELECT qty_real,qty_ghost FROM inventory WHERE branch_id=$1 AND product_id=$2`, warehouseID, productID).Scan(&warehouseReal, &warehouseGhost); err != nil {
		t.Fatal(err)
	}
	if warehouseReal != 0 || warehouseGhost != 0 {
		t.Fatalf("warehouse inventory = real %d / Ghost %d, want 0 / 0 (nothing returned, no deficit)", warehouseReal, warehouseGhost)
	}
	var deleted *time.Time
	var total, paid, unitPrice float64
	if err := db.QueryRowContext(ctx, `
		SELECT i.deleted_at,i.total_amount,(SELECT COALESCE(SUM(amount),0) FROM invoice_payments WHERE invoice_id=i.id),ii.unit_price
		FROM invoices i INNER JOIN invoice_items ii ON ii.id=$2 WHERE i.id=$1
	`, invoiceID, itemID).Scan(&deleted, &total, &paid, &unitPrice); err != nil {
		t.Fatal(err)
	}
	if deleted != nil || total != 10.5 || paid != 10.5 || unitPrice != 10.5 {
		t.Fatalf("cash bill should stay at cost × 1.05: deleted=%v total=%.2f paid=%.2f unit=%.2f", deleted, total, paid, unitPrice)
	}

	var deficitCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*)::integer FROM inventory_ghost_deficits WHERE reconciliation_id=$1`, reconciliationID).Scan(&deficitCount); err != nil {
		t.Fatal(err)
	}
	if deficitCount != 0 {
		t.Fatalf("deficit ledger rows = %d, want 0", deficitCount)
	}

	// The ledger stays append-only for rounds that do write it.
	movementID := platform.MustUUID()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,performed_by,created_at)
		VALUES ($1,$2,$3,'month_end_ghost_deduction','ghost',-1,'month_end_reconciliation',$4,$5,NOW())
	`, movementID, warehouseID, productID, reconciliationID, actorID); err != nil {
		t.Fatalf("insert ledger movement: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_ghost_deficits (id,branch_id,product_id,quantity,reconciliation_id,invoice_id,invoice_item_id,inventory_movement_id,created_by,created_at)
		VALUES ($1,$2,$3,1,$4,$5,$6,$7,$8,NOW())
	`, platform.MustUUID(), warehouseID, productID, reconciliationID, invoiceID, itemID, movementID, actorID); err != nil {
		t.Fatalf("insert ledger row: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE inventory_ghost_deficits SET quantity=2 WHERE reconciliation_id=$1`, reconciliationID); err == nil {
		t.Fatal("expected immutable deficit ledger to reject updates")
	}

	report, err := service.MonthEndReport(ctx, platform.AuthUser{ID: actorID, RoleKey: "super_admin"}, reconciliationID, "", "", "", branchID, "", "", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.GhostQuantityDeducted != 0 || report.Summary.GhostDeficitQuantity != 0 || report.Summary.WarehouseRealReceived != 0 || report.Summary.RevenueBefore != 20 || report.Summary.RevenueAfter != 10.5 {
		t.Fatalf("unexpected cost-markup report summary: %+v", report.Summary)
	}
	if len(report.Rows) != 1 || report.Rows[0].Status != "adjusted" || report.Rows[0].AdjustedPrice == nil || *report.Rows[0].AdjustedPrice != 10.5 || report.Rows[0].DiscountAmount != 9.5 {
		t.Fatalf("unexpected report rows: %+v", report.Rows)
	}
}

func monthEndTestDatabaseURL(rawURL, schema string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" {
		return "", fmt.Errorf("TEST_DATABASE_URL must use postgres:// URL form")
	}
	query := parsed.Query()
	query.Set("options", "-csearch_path="+schema+",public")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func applyAllMonthEndTestMigrations(ctx context.Context, db *sql.DB) error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return err
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(raw)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

const monthEndDeficitFixtureSQL = `
INSERT INTO roles (id,role_key,name,active,is_system,portal,scope)
VALUES ('10000000-0000-4000-8000-000000000001','super_admin','Superadmin',TRUE,TRUE,'backoffice','global');
INSERT INTO branches (id,code,name,branch_type,parent_branch_id,active,sales_enabled,online_sales_enabled)
VALUES
 ('30000000-0000-4000-8000-000000000001','WH','Warehouse','main_warehouse',NULL,TRUE,TRUE,FALSE),
 ('30000000-0000-4000-8000-000000000002','DEF','Deficit Branch','branch','30000000-0000-4000-8000-000000000001',TRUE,TRUE,FALSE);
INSERT INTO users (id,role_id,full_name,email,password_hash,active)
VALUES ('20000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','Deficit Actor','deficit@example.test','test',TRUE);
INSERT INTO products (id,sku,name,description,cost_price,base_selling_price,unit_name,tax_exempt,active)
VALUES ('40000000-0000-4000-8000-000000000001','DEFICIT-001','Deficit Fixture','',10,20,'ชิ้น',FALSE,TRUE);
INSERT INTO inventory (id,branch_id,product_id,qty_real,qty_ghost)
VALUES
 ('50000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001',0,0),
 ('50000000-0000-4000-8000-000000000002','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001',0,0);
INSERT INTO inventory_lots (
 id,branch_id,product_id,stock_bucket,lot_number,received_quantity,remaining_quantity,unit_cost,source_type,received_at
) VALUES (
 '60000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001',
 'real','DEFICIT-REAL',1,0,10,'deficit_fixture','2097-02-28T00:00:00Z'
);
INSERT INTO invoices (
 id,branch_id,invoice_number,customer_name,payment_status,invoice_status,is_government_mode,
 tax_invoice_type,request_full_tax_invoice,subtotal,tax_rate,tax_amount,total_amount,created_by,issued_at,created_at,updated_at
) VALUES (
 '70000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','DEF-BL2097030100001',
 'Deficit Customer','paid','issued',FALSE,'abbreviated',FALSE,20,0,0,20,
 '20000000-0000-4000-8000-000000000001','2097-03-01T03:00:00Z','2097-03-01T03:00:00Z','2097-03-01T03:00:00Z'
);
INSERT INTO invoice_items (
 id,invoice_id,product_id,actual_product_name,display_name,quantity,stock_bucket,unit_price,
 line_subtotal,tax_rate,tax_amount,line_total,price_source,cost_snapshot,inventory_lot_id,
 lot_number_snapshot,lot_received_at_snapshot,created_at
) VALUES (
 '71000000-0000-4000-8000-000000000001','70000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001',
 'Deficit Fixture','Deficit Fixture',1,'real',20,20,0,0,20,'base',10,
 '60000000-0000-4000-8000-000000000001','DEFICIT-REAL','2097-02-28T00:00:00Z','2097-03-01T03:00:00Z'
);
INSERT INTO invoice_payments (id,invoice_id,payment_type,amount,created_by,created_at)
VALUES (gen_random_uuid(),'70000000-0000-4000-8000-000000000001','cash',20,'20000000-0000-4000-8000-000000000001','2097-03-01T03:00:00Z');
INSERT INTO inventory_movements (
 id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,performed_by,created_at
) VALUES
 ('80000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001','opening_balance','real',1,'deficit_fixture',NULL,'20000000-0000-4000-8000-000000000001','2097-02-28T00:00:00Z'),
 ('80000000-0000-4000-8000-000000000002','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001','sale','real',-1,'invoice','70000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001','2097-03-01T03:00:00Z');
INSERT INTO inventory_movement_lots (id,inventory_movement_id,inventory_lot_id,quantity_delta,created_at)
VALUES
 (gen_random_uuid(),'80000000-0000-4000-8000-000000000001','60000000-0000-4000-8000-000000000001',1,'2097-02-28T00:00:00Z'),
 (gen_random_uuid(),'80000000-0000-4000-8000-000000000002','60000000-0000-4000-8000-000000000001',-1,'2097-03-01T03:00:00Z');
`
