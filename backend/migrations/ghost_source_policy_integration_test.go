package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestGhostSourcePolicyPreservesHistoryAndRejectsOperationalWrites(t *testing.T) {
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

	schema := fmt.Sprintf("ghost_source_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() { _, _ = adminDB.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) })

	schemaURL, err := databaseURLWithSchema(databaseURL, schema)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	names, err := migrationNamesBefore("045_ghost_po_month_end_only.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := applyMigrationFile(ctx, db, name); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}

	if _, err := db.ExecContext(ctx, ghostSourceFixtureSQL); err != nil {
		t.Fatalf("create pre-cut-over history: %v", err)
	}
	if err := applyMigrationFile(ctx, db, "045_ghost_po_month_end_only.sql"); err != nil {
		t.Fatalf("apply Ghost source policy: %v", err)
	}

	assertCount(t, db, `SELECT COUNT(*) FROM inventory_movements WHERE stock_bucket='ghost' AND reference_type='inventory.receive'`, 7, "historical manual Ghost movements")
	assertCount(t, db, `SELECT COALESCE(SUM(quantity_delta),0) FROM inventory_movements WHERE stock_bucket='ghost' AND reference_type='inventory.receive'`, 51, "historical manual Ghost quantity")
	assertCount(t, db, `SELECT COUNT(*) FROM inventory_lots WHERE stock_bucket='ghost' AND source_type='inventory_receive'`, 1, "historical manual Ghost lot")

	expectExecError(t, db, `INSERT INTO inventory_movements(id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,performed_by) VALUES(gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','receive','ghost',1,'inventory.receive','20000000-0000-4000-8000-000000000001')`, "manual Ghost movement")
	expectExecError(t, db, `INSERT INTO inventory_lots(id,branch_id,product_id,stock_bucket,lot_number,received_quantity,remaining_quantity,source_type) VALUES(gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','ghost','MANUAL-NEW',1,1,'inventory_receive')`, "manual Ghost lot")

	if _, err := db.ExecContext(ctx, `INSERT INTO inventory_movements(id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,performed_by) VALUES(gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','purchase_receive','ghost',1,'purchase_order','20000000-0000-4000-8000-000000000001'),(gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','month_end_ghost_deduction','ghost',-1,'month_end_reconciliation','20000000-0000-4000-8000-000000000001'),(gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','manual_adjust','real',1,'inventory.adjust','20000000-0000-4000-8000-000000000001')`); err != nil {
		t.Fatalf("allowed PO, Month-End, or Real movement was rejected: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO inventory_lots(id,branch_id,product_id,stock_bucket,lot_number,received_quantity,remaining_quantity,source_type) VALUES(gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','ghost','PO-NEW',1,1,'purchase_order'),(gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','real','REAL-NEW',1,1,'inventory_receive')`); err != nil {
		t.Fatalf("allowed PO Ghost or Real lot was rejected: %v", err)
	}

	assertCount(t, db, `SELECT COUNT(*) FROM pg_trigger trigger JOIN pg_class relation ON relation.oid=trigger.tgrelid JOIN pg_namespace namespace ON namespace.oid=relation.relnamespace WHERE namespace.nspname=current_schema() AND NOT trigger.tgisinternal AND trigger.tgname IN ('trg_invoice_items_no_direct_ghost','trg_quotation_items_no_direct_ghost','trg_transfer_items_no_direct_ghost','trg_product_returns_no_direct_ghost')`, 4, "operational Ghost document guards")
}

func expectExecError(t *testing.T, db *sql.DB, statement, label string) {
	t.Helper()
	if _, err := db.Exec(statement); err == nil {
		t.Fatalf("expected %s to be rejected", label)
	}
}

const ghostSourceFixtureSQL = `
INSERT INTO roles (id,role_key,name,active,is_system,portal,scope)
VALUES ('10000000-0000-4000-8000-000000000001','super_admin','Superadmin',TRUE,TRUE,'backoffice','global');
INSERT INTO branches (id,code,name,branch_type,parent_branch_id,active,sales_enabled,online_sales_enabled)
VALUES ('30000000-0000-4000-8000-000000000001','WH','Warehouse','main_warehouse',NULL,TRUE,TRUE,FALSE);
INSERT INTO users (id,role_id,full_name,email,password_hash,active)
VALUES ('20000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','Policy Actor','policy@example.test','test',TRUE);
INSERT INTO products (id,sku,name,description,cost_price,base_selling_price,unit_name,tax_exempt,active)
VALUES ('40000000-0000-4000-8000-000000000001','POLICY-001','Policy Fixture','',10,20,'ชิ้น',FALSE,TRUE);
INSERT INTO inventory (id,branch_id,product_id,qty_real,qty_ghost)
VALUES (gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001',0,51);
INSERT INTO inventory_lots(id,branch_id,product_id,stock_bucket,lot_number,received_quantity,remaining_quantity,source_type)
VALUES (gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','ghost','LEGACY-MANUAL',51,51,'inventory_receive');
INSERT INTO inventory_movements(id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,performed_by)
SELECT gen_random_uuid(),'30000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','receive','ghost',quantity,'inventory.receive','20000000-0000-4000-8000-000000000001'
FROM unnest(ARRAY[10,9,8,7,6,5,6]) AS quantity;
`
