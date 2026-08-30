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

func TestMonthEndCreatesImmutableGhostDeficit(t *testing.T) {
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

	service := NewService(db, audit.NewService(db))
	result, err := service.FinalizeReconciliation(ctx, platform.AuthUser{
		ID: "20000000-0000-4000-8000-000000000001", RoleKey: "super_admin", Portal: "backoffice", Scope: "global",
	}, audit.LogEntry{}, ReconciliationInput{
		DateFrom: "2097-03-01", DateTo: "2097-03-31", BranchIDs: []string{"30000000-0000-4000-8000-000000000002"},
	})
	if err != nil {
		t.Fatalf("finalize zero-Ghost reconciliation: %v", err)
	}
	reconciliationID, ok := result["id"].(string)
	if !ok || reconciliationID == "" {
		t.Fatalf("missing reconciliation ID: %+v", result)
	}

	var warehouseReal, warehouseGhost int
	if err := db.QueryRowContext(ctx, `
		SELECT qty_real,qty_ghost FROM inventory
		WHERE branch_id='30000000-0000-4000-8000-000000000001'
		  AND product_id='40000000-0000-4000-8000-000000000001'
	`).Scan(&warehouseReal, &warehouseGhost); err != nil {
		t.Fatal(err)
	}
	if warehouseReal != 1 || warehouseGhost != -1 {
		t.Fatalf("warehouse inventory = real %d / Ghost %d, want 1 / -1", warehouseReal, warehouseGhost)
	}

	var deficitQuantity, deficitCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(quantity),0)::integer,COUNT(*)::integer
		FROM inventory_ghost_deficits WHERE reconciliation_id=$1
	`, reconciliationID).Scan(&deficitQuantity, &deficitCount); err != nil {
		t.Fatal(err)
	}
	if deficitQuantity != 1 || deficitCount != 1 {
		t.Fatalf("deficit ledger = quantity %d / rows %d, want 1 / 1", deficitQuantity, deficitCount)
	}
	if _, err := db.ExecContext(ctx, `UPDATE inventory_ghost_deficits SET quantity=2 WHERE reconciliation_id=$1`, reconciliationID); err == nil {
		t.Fatal("expected immutable deficit ledger to reject updates")
	}

	report, err := service.MonthEndReport(ctx, platform.AuthUser{ID: "20000000-0000-4000-8000-000000000001", RoleKey: "super_admin"}, reconciliationID, "", "", "", "30000000-0000-4000-8000-000000000002", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.GhostQuantityDeducted != 1 || report.Summary.GhostDeficitQuantity != 1 || report.Summary.WarehouseRealReceived != 1 {
		t.Fatalf("unexpected deficit report summary: %+v", report.Summary)
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
