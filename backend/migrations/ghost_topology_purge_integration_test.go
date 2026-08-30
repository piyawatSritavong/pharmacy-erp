package migrations_test

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

	"pharmacy-erp/backend/migrations"

	_ "github.com/lib/pq"
)

func TestGlobalWarehouseGhostPurgeRebasesMixedDocuments(t *testing.T) {
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

	schema := fmt.Sprintf("ghost_purge_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() { _, _ = adminDB.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) })

	schemaURL, err := databaseURLWithSchema(databaseURL, schema)
	if err != nil {
		t.Fatalf("build isolated database URL: %v", err)
	}
	db, err := sql.Open("postgres", schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	names, err := migrationNamesBefore("044_global_warehouse_ghost_reconciliation.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := applyMigrationFile(ctx, db, name); err != nil {
			t.Fatalf("apply pre-cut-over migration %s: %v", name, err)
		}
	}

	if _, err := db.ExecContext(ctx, legacyGhostFixtureSQL); err != nil {
		t.Fatalf("create legacy Ghost fixture: %v", err)
	}
	if err := applyMigrationFile(ctx, db, "044_global_warehouse_ghost_reconciliation.sql"); err != nil {
		t.Fatalf("apply Ghost cut-over: %v", err)
	}

	assertCount(t, db, `SELECT COUNT(*) FROM purchase_orders WHERE id='60000000-0000-4000-8000-000000000001'`, 0, "mixed purchase order")
	assertCount(t, db, `SELECT COUNT(*) FROM invoices WHERE id='90000000-0000-4000-8000-000000000001'`, 0, "Ghost invoice")
	assertCount(t, db, `SELECT COUNT(*) FROM invoice_payments WHERE invoice_id='90000000-0000-4000-8000-000000000001'`, 0, "invoice payments")
	assertCount(t, db, `SELECT COUNT(*) FROM month_end_reconciliations WHERE id='a0000000-0000-4000-8000-000000000001'`, 0, "reconciliation")
	assertCount(t, db, `SELECT COUNT(*) FROM reconciliation_invoice_snapshots WHERE reconciliation_id='a0000000-0000-4000-8000-000000000001'`, 0, "invoice snapshots")
	assertCount(t, db, `SELECT COUNT(*) FROM reconciliation_item_snapshots WHERE reconciliation_id='a0000000-0000-4000-8000-000000000001'`, 0, "item snapshots")
	assertCount(t, db, `SELECT COUNT(*) FROM audit_logs WHERE entity_id IN ('60000000-0000-4000-8000-000000000001','90000000-0000-4000-8000-000000000001','a0000000-0000-4000-8000-000000000001')`, 0, "related audit logs")

	var real, ghost int
	if err := db.QueryRowContext(ctx, `SELECT qty_real,qty_ghost FROM inventory WHERE branch_id='30000000-0000-4000-8000-000000000002' AND product_id='40000000-0000-4000-8000-000000000001'`).Scan(&real, &ghost); err != nil {
		t.Fatal(err)
	}
	if real != 7 || ghost != 0 {
		t.Fatalf("rebuilt branch inventory = real %d / Ghost %d, want 7 / 0", real, ghost)
	}

	var lotSource, movementReference string
	var lotSourceID, movementReferenceID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT source_type,source_id::text FROM inventory_lots WHERE id='70000000-0000-4000-8000-000000000001'`).Scan(&lotSource, &lotSourceID); err != nil {
		t.Fatal(err)
	}
	if lotSource != "ghost_topology_rebase" || lotSourceID.Valid {
		t.Fatalf("Real lot was not rebased: source=%s id=%v", lotSource, lotSourceID)
	}
	if err := db.QueryRowContext(ctx, `SELECT reference_type,reference_id::text FROM inventory_movements WHERE id='80000000-0000-4000-8000-000000000001'`).Scan(&movementReference, &movementReferenceID); err != nil {
		t.Fatal(err)
	}
	if movementReference != "ghost_topology_rebase" || movementReferenceID.Valid {
		t.Fatalf("Real movement was not rebased: reference=%s id=%v", movementReference, movementReferenceID)
	}

	assertCount(t, db, `SELECT COUNT(*) FROM inventory_lots l JOIN branches b ON b.id=l.branch_id WHERE l.stock_bucket='ghost' AND b.branch_type<>'main_warehouse'`, 0, "non-WH Ghost lots")
	assertCount(t, db, `SELECT COUNT(*) FROM inventory_movements m JOIN branches b ON b.id=m.branch_id WHERE m.stock_bucket='ghost' AND b.branch_type<>'main_warehouse'`, 0, "non-WH Ghost movements")
	assertCount(t, db, `SELECT COUNT(*) FROM inventory_movement_lots ml LEFT JOIN inventory_movements m ON m.id=ml.inventory_movement_id LEFT JOIN inventory_lots l ON l.id=ml.inventory_lot_id WHERE m.id IS NULL OR l.id IS NULL`, 0, "orphan movement-lot links")
	assertCount(t, db, `SELECT COUNT(*) FROM inventory i WHERE i.qty_real<>COALESCE((SELECT SUM(l.remaining_quantity)::integer FROM inventory_lots l WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='real'),0) OR i.qty_ghost<>COALESCE((SELECT SUM(l.remaining_quantity)::integer FROM inventory_lots l WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='ghost'),0)`, 0, "inventory aggregate mismatches")

	if _, err := db.ExecContext(ctx, `UPDATE inventory SET qty_ghost=1 WHERE branch_id='30000000-0000-4000-8000-000000000002' AND product_id='40000000-0000-4000-8000-000000000001'`); err == nil {
		t.Fatal("expected database trigger to reject non-WH Ghost balance")
	}
}

func databaseURLWithSchema(rawURL, schema string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" {
		return "", fmt.Errorf("TEST_DATABASE_URL must use postgres:// URL form")
	}
	query := parsed.Query()
	query.Set("options", "-csearch_path="+schema+",public")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func migrationNamesBefore(stop string) ([]string, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") && entry.Name() < stop {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func applyMigrationFile(ctx context.Context, db *sql.DB, name string) error {
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
		return err
	}
	return tx.Commit()
}

func assertCount(t *testing.T, db *sql.DB, query string, want int, label string) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", label, got, want)
	}
}

const legacyGhostFixtureSQL = `
INSERT INTO roles (id,role_key,name,active,is_system,portal,scope)
VALUES ('10000000-0000-4000-8000-000000000001','super_admin','Superadmin',TRUE,TRUE,'backoffice','global');
INSERT INTO branches (id,code,name,branch_type,parent_branch_id,active,sales_enabled,online_sales_enabled)
VALUES
 ('30000000-0000-4000-8000-000000000001','WH','Warehouse','main_warehouse',NULL,TRUE,TRUE,FALSE),
 ('30000000-0000-4000-8000-000000000002','LEG','Legacy Branch','branch','30000000-0000-4000-8000-000000000001',TRUE,TRUE,FALSE);
INSERT INTO users (id,role_id,full_name,email,password_hash,active)
VALUES ('20000000-0000-4000-8000-000000000001','10000000-0000-4000-8000-000000000001','Migration Actor','migration@example.test','test',TRUE);
INSERT INTO products (id,sku,name,description,cost_price,base_selling_price,unit_name,tax_exempt,active)
VALUES ('40000000-0000-4000-8000-000000000001','PURGE-001','Purge Fixture','',10,20,'ชิ้น',FALSE,TRUE);
INSERT INTO suppliers (id,supplier_code,legal_name,active)
VALUES ('50000000-0000-4000-8000-000000000001','SUP-PURGE','Purge Supplier',TRUE);
INSERT INTO purchase_orders (
 id,branch_id,supplier_id,po_number,status,revision,purchased_at,posted_at,
 supplier_code_snapshot,supplier_name_snapshot,subtotal,total_amount,created_by,updated_by
) VALUES (
 '60000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002',
 '50000000-0000-4000-8000-000000000001','PO-PURGE-001','posted',1,NOW(),NOW(),
 'SUP-PURGE','Purge Supplier',120,120,'20000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001'
);
INSERT INTO purchase_order_items (
 id,purchase_order_id,product_id,product_sku_snapshot,product_name_snapshot,unit_name_snapshot,
 stock_bucket,received_quantity,unit_cost,line_subtotal,line_total,lot_number
) VALUES
 ('61000000-0000-4000-8000-000000000001','60000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','PURGE-001','Purge Fixture','ชิ้น','real',7,10,70,70,'REAL-PURGE'),
 ('61000000-0000-4000-8000-000000000002','60000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','PURGE-001','Purge Fixture','ชิ้น','ghost',5,10,50,50,'GHOST-PURGE');
INSERT INTO purchase_order_events (id,purchase_order_id,event_type,revision,actor_id)
VALUES (gen_random_uuid(),'60000000-0000-4000-8000-000000000001','posted',1,'20000000-0000-4000-8000-000000000001');
INSERT INTO inventory (id,branch_id,product_id,qty_real,qty_ghost)
VALUES ('62000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001',7,4);
INSERT INTO inventory_lots (
 id,branch_id,product_id,stock_bucket,lot_number,received_quantity,remaining_quantity,unit_cost,
 source_type,source_id,source_item_id,received_at
) VALUES
 ('70000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001','real','REAL-PURGE',7,7,10,'purchase_order','60000000-0000-4000-8000-000000000001','61000000-0000-4000-8000-000000000001',NOW()),
 ('70000000-0000-4000-8000-000000000002','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001','ghost','GHOST-PURGE',5,4,10,'purchase_order','60000000-0000-4000-8000-000000000001','61000000-0000-4000-8000-000000000002',NOW());
INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,performed_by)
VALUES
 ('80000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001','purchase_receive','real',7,'purchase_order','60000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001'),
 ('80000000-0000-4000-8000-000000000002','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001','purchase_receive','ghost',5,'purchase_order','60000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001');
INSERT INTO inventory_movement_lots (id,inventory_movement_id,inventory_lot_id,quantity_delta)
VALUES
 (gen_random_uuid(),'80000000-0000-4000-8000-000000000001','70000000-0000-4000-8000-000000000001',7),
 (gen_random_uuid(),'80000000-0000-4000-8000-000000000002','70000000-0000-4000-8000-000000000002',5);
INSERT INTO invoices (
 id,branch_id,invoice_number,customer_name,payment_status,invoice_status,is_government_mode,
 subtotal,tax_rate,tax_amount,total_amount,created_by,issued_at,request_full_tax_invoice
) VALUES (
 '90000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','INV-PURGE-001',
 'Legacy Ghost Customer','paid','issued',FALSE,20,0,0,20,'20000000-0000-4000-8000-000000000001',NOW(),FALSE
);
INSERT INTO invoice_items (
 id,invoice_id,product_id,actual_product_name,display_name,quantity,stock_bucket,unit_price,
 line_subtotal,tax_rate,tax_amount,line_total,price_source,cost_snapshot,inventory_lot_id,lot_number_snapshot,lot_received_at_snapshot
) VALUES (
 '91000000-0000-4000-8000-000000000001','90000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001',
 'Purge Fixture','Purge Fixture',1,'ghost',20,20,0,0,20,'base',10,'70000000-0000-4000-8000-000000000002','GHOST-PURGE',NOW()
);
INSERT INTO invoice_payments (id,invoice_id,payment_type,amount,created_by)
VALUES (gen_random_uuid(),'90000000-0000-4000-8000-000000000001','cash',20,'20000000-0000-4000-8000-000000000001');
INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,performed_by)
VALUES ('80000000-0000-4000-8000-000000000003','30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001','sale','ghost',-1,'invoice','90000000-0000-4000-8000-000000000001','20000000-0000-4000-8000-000000000001');
INSERT INTO inventory_movement_lots (id,inventory_movement_id,inventory_lot_id,quantity_delta)
VALUES (gen_random_uuid(),'80000000-0000-4000-8000-000000000003','70000000-0000-4000-8000-000000000002',-1);
INSERT INTO month_end_reconciliations (
 id,reconciliation_number,period_start,period_end,branch_ids,target_revenue,original_revenue,
 suppressed_revenue,adjustment_reduction,final_revenue,suppressed_invoice_count,adjusted_item_count,
 source_hash,finalized_by,adjustment_percent
) VALUES (
 'a0000000-0000-4000-8000-000000000001','MER-PURGE-001',CURRENT_DATE,CURRENT_DATE,
 ARRAY['30000000-0000-4000-8000-000000000002'::uuid],0,20,20,0,0,1,0,'fixture',
 '20000000-0000-4000-8000-000000000001',0
);
INSERT INTO reconciliation_logs (
 id,reconciliation_id,log_type,invoice_id,invoice_item_id,branch_id,product_id,
 original_invoice_number,stock_bucket,stock_quantity,actor_id
) VALUES (
 'a1000000-0000-4000-8000-000000000001','a0000000-0000-4000-8000-000000000001','stock_deducted',
 '90000000-0000-4000-8000-000000000001','91000000-0000-4000-8000-000000000001',
 '30000000-0000-4000-8000-000000000002','40000000-0000-4000-8000-000000000001',
 'INV-PURGE-001','ghost',1,'20000000-0000-4000-8000-000000000001'
);
INSERT INTO reconciliation_invoice_snapshots (
 reconciliation_id,invoice_id,branch_id,original_invoice_number,issued_at,payment_method,
 request_full_tax_invoice,original_subtotal,original_tax_amount,original_total_amount
) VALUES (
 'a0000000-0000-4000-8000-000000000001','90000000-0000-4000-8000-000000000001',
 '30000000-0000-4000-8000-000000000002','INV-PURGE-001',NOW(),'cash',FALSE,20,0,20
);
INSERT INTO reconciliation_item_snapshots (
 reconciliation_id,invoice_item_id,invoice_id,product_id,product_name,quantity,original_unit_price,
 original_line_subtotal,original_tax_amount,original_line_total,original_stock_bucket
) VALUES (
 'a0000000-0000-4000-8000-000000000001','91000000-0000-4000-8000-000000000001',
 '90000000-0000-4000-8000-000000000001','40000000-0000-4000-8000-000000000001','Purge Fixture',
 1,20,20,0,20,'ghost'
);
INSERT INTO audit_logs (id,actor_id,branch_id,entity_type,entity_id,action)
VALUES
 (gen_random_uuid(),'20000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','purchase_order','60000000-0000-4000-8000-000000000001','fixture'),
 (gen_random_uuid(),'20000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','invoice','90000000-0000-4000-8000-000000000001','fixture'),
 (gen_random_uuid(),'20000000-0000-4000-8000-000000000001','30000000-0000-4000-8000-000000000002','month_end_reconciliation','a0000000-0000-4000-8000-000000000001','fixture');
`
