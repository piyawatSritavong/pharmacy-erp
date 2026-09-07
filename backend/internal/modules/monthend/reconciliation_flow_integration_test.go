package monthend

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	inventorymodule "pharmacy-erp/backend/internal/modules/inventory"
	salesmodule "pharmacy-erp/backend/internal/modules/sales"
	"pharmacy-erp/backend/internal/platform"
)

func TestGlobalWarehouseMonthEndFlow(t *testing.T) {
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

	var actorID, branchID, warehouseID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local'`).Scan(&actorID); err != nil {
		t.Fatalf("load actor: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE code='KNP' AND active=TRUE`).Scan(&branchID); err != nil {
		t.Fatalf("load branch: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE branch_type='main_warehouse' AND active=TRUE`).Scan(&warehouseID); err != nil {
		t.Fatalf("load warehouse: %v", err)
	}

	unique := strings.ReplaceAll(platform.MustUUID(), "-", "")[:10]
	productID := platform.MustUUID()
	branchLotID, warehouseRealLotID, warehouseGhostLotID := platform.MustUUID(), platform.MustUUID(), platform.MustUUID()
	invoiceIDs := []string{platform.MustUUID(), platform.MustUUID(), platform.MustUUID()}
	itemIDs := []string{platform.MustUUID(), platform.MustUUID(), platform.MustUUID()}
	movementIDs := []string{platform.MustUUID(), platform.MustUUID(), platform.MustUUID(), platform.MustUUID(), platform.MustUUID(), platform.MustUUID()}
	reconciliationID := ""

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if reconciliationID != "" {
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM stock_adjustment_notes WHERE reconciliation_id=$1`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM transfer_item_lot_allocations WHERE transfer_item_id IN (SELECT id FROM transfer_items WHERE transfer_id IN (SELECT id FROM transfers WHERE reconciliation_id=$1))`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM transfer_events WHERE transfer_id IN (SELECT id FROM transfers WHERE reconciliation_id=$1)`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM transfer_items WHERE transfer_id IN (SELECT id FROM transfers WHERE reconciliation_id=$1)`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM transfers WHERE reconciliation_id=$1`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM reconciliation_logs WHERE reconciliation_id=$1`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM reconciliation_item_snapshots WHERE reconciliation_id=$1`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM reconciliation_invoice_snapshots WHERE reconciliation_id=$1`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM month_end_reconciliation_scopes WHERE reconciliation_id=$1`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM audit_logs WHERE entity_id=$1`, reconciliationID)
			_, _ = db.ExecContext(cleanupCtx, `DELETE FROM month_end_reconciliations WHERE id=$1`, reconciliationID)
		}
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM invoice_payments WHERE invoice_id=ANY($1::uuid[])`, pqUUIDArray(invoiceIDs))
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM invoice_items WHERE invoice_id=ANY($1::uuid[])`, pqUUIDArray(invoiceIDs))
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM invoices WHERE id=ANY($1::uuid[])`, pqUUIDArray(invoiceIDs))
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM inventory_movement_lots WHERE inventory_movement_id IN (SELECT id FROM inventory_movements WHERE product_id=$1) OR inventory_lot_id IN (SELECT id FROM inventory_lots WHERE product_id=$1)`, productID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM inventory_movements WHERE product_id=$1`, productID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM inventory_lots WHERE product_id=$1`, productID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM inventory WHERE product_id=$1`, productID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM audit_logs WHERE entity_id=$1`, productID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM products WHERE id=$1`, productID)
	})

	if _, err := db.ExecContext(ctx, `
		INSERT INTO products (id,sku,name,description,cost_price,base_selling_price,unit_name,tax_exempt,active,
			max_discount_amount,low_stock_real_threshold,low_stock_ghost_threshold,tracks_expiry,expiry_warning_days,created_at,updated_at)
		VALUES ($1,$2,$3,'month-end integration fixture',50,100,'ชิ้น',FALSE,TRUE,0,0,0,FALSE,30,NOW(),NOW())
	`, productID, "MER-"+unique, "Month End Fixture "+unique); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory (id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at) VALUES
		($1,$2,$3,2,0,NOW(),NOW()),($4,$5,$3,100,20,NOW(),NOW())
	`, platform.MustUUID(), branchID, productID, platform.MustUUID(), warehouseID); err != nil {
		t.Fatalf("insert inventory: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_lots (id,branch_id,product_id,stock_bucket,lot_number,received_quantity,remaining_quantity,unit_cost,source_type,received_at,created_at,updated_at) VALUES
		($1,$2,$3,'real',$4,5,2,50,'purchase_order',NOW(),NOW(),NOW()),
		($5,$6,$3,'real',$7,100,100,50,'purchase_order',NOW(),NOW(),NOW()),
		($8,$6,$3,'ghost',$9,20,20,50,'purchase_order',NOW(),NOW(),NOW())
	`, branchLotID, branchID, productID, "A-"+unique, warehouseRealLotID, warehouseID, "WH-R-"+unique, warehouseGhostLotID, "WH-G-"+unique); err != nil {
		t.Fatalf("insert lots: %v", err)
	}

	created := []time.Time{
		time.Date(2098, 2, 1, 2, 0, 0, 0, time.UTC),
		time.Date(2098, 2, 1, 3, 0, 0, 0, time.UTC),
		time.Date(2098, 2, 1, 4, 0, 0, 0, time.UTC),
	}
	prefix := "KNP-MER-" + unique + "-"
	for index := range invoiceIDs {
		invoiceNumber := prefix + []string{"00001", "00002", "00003"}[index]
		taxInvoiceType := "abbreviated"
		requestFullTaxInvoice := false
		if index == 0 {
			taxInvoiceType = "full"
			requestFullTaxInvoice = true
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO invoices (id,branch_id,invoice_number,customer_name,payment_status,invoice_status,is_government_mode,
				tax_invoice_type,request_full_tax_invoice,subtotal,tax_rate,tax_amount,total_amount,created_by,issued_at,created_at,updated_at)
			VALUES ($1,$2,$3,$4,'paid','issued',FALSE,$7,$8,100,0,0,100,$5,$6,$6,$6)
		`, invoiceIDs[index], branchID, invoiceNumber, "fixture "+invoiceNumber, actorID, created[index], taxInvoiceType, requestFullTaxInvoice); err != nil {
			t.Fatalf("insert invoice %d: %v", index, err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO invoice_items (id,invoice_id,product_id,actual_product_name,display_name,quantity,stock_bucket,
				unit_price,line_subtotal,tax_rate,tax_amount,line_total,price_source,override_reason,cost_snapshot,
				inventory_lot_id,lot_number_snapshot,lot_received_at_snapshot,created_at)
			VALUES ($1,$2,$3,$4,$4,1,'real',100,100,0,0,100,'base','',50,$5,$6,NOW(),$7)
		`, itemIDs[index], invoiceIDs[index], productID, "Month End Fixture "+unique, branchLotID, "A-"+unique, created[index]); err != nil {
			t.Fatalf("insert invoice item %d: %v", index, err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at)
			VALUES ($1,$2,$3,'sale','real',-1,'invoice',$4,'',$5,$6)
		`, movementIDs[index+3], branchID, productID, invoiceIDs[index], actorID, created[index]); err != nil {
			t.Fatalf("insert sale movement %d: %v", index, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO inventory_movement_lots (id,inventory_movement_id,inventory_lot_id,quantity_delta,created_at) VALUES ($1,$2,$3,-1,$4)`, platform.MustUUID(), movementIDs[index+3], branchLotID, created[index]); err != nil {
			t.Fatalf("link sale movement %d: %v", index, err)
		}
	}
	for _, opening := range []struct {
		id, branch, bucket, lot string
		quantity                int
	}{{movementIDs[0], branchID, "real", branchLotID, 5}, {movementIDs[1], warehouseID, "real", warehouseRealLotID, 100}, {movementIDs[2], warehouseID, "ghost", warehouseGhostLotID, 20}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO inventory_movements (id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,note,performed_by,created_at) VALUES ($1,$2,$3,'opening_balance',$4,$5,'purchase_order','',$6,$7)`, opening.id, opening.branch, productID, opening.bucket, opening.quantity, actorID, created[0].Add(-time.Hour)); err != nil {
			t.Fatalf("insert opening movement: %v", err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO inventory_movement_lots (id,inventory_movement_id,inventory_lot_id,quantity_delta,created_at) VALUES ($1,$2,$3,$4,$5)`, platform.MustUUID(), opening.id, opening.lot, opening.quantity, created[0].Add(-time.Hour)); err != nil {
			t.Fatalf("link opening movement: %v", err)
		}
	}
	payments := [][]struct {
		kind   string
		amount float64
	}{
		{{"bank_transfer", 100}},
		{{"cash", 100}},
		{{"cash", 40}, {"bank_transfer", 60}},
	}
	for index, invoicePayments := range payments {
		for _, payment := range invoicePayments {
			if _, err := db.ExecContext(ctx, `INSERT INTO invoice_payments (id,invoice_id,payment_type,amount,reference_code,notes,created_by,created_at) VALUES ($1,$2,$3,$4,'','',$5,$6)`, platform.MustUUID(), invoiceIDs[index], payment.kind, payment.amount, actorID, created[index]); err != nil {
				t.Fatalf("insert payment %d: %v", index, err)
			}
		}
	}

	var originalUpdated []time.Time
	for _, invoiceID := range []string{invoiceIDs[0], invoiceIDs[2]} {
		var value time.Time
		if err := db.QueryRowContext(ctx, `SELECT updated_at FROM invoices WHERE id=$1`, invoiceID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		originalUpdated = append(originalUpdated, value)
	}

	service := NewService(db, audit.NewService(db))
	result, err := service.FinalizeReconciliation(ctx, platform.AuthUser{ID: actorID, RoleKey: "super_admin", Scope: "global", Portal: "backoffice"}, audit.LogEntry{}, ReconciliationInput{
		DateFrom: "2098-02-01", DateTo: "2098-02-28", BranchIDs: []string{branchID}, TargetRevenue: 999999, AdjustmentPercent: 5,
	})
	if err != nil {
		t.Fatalf("finalize reconciliation: %v", err)
	}
	reconciliationID = result["id"].(string)

	var branchReal, branchGhost, warehouseReal, warehouseGhost int
	if err := db.QueryRowContext(ctx, `SELECT qty_real,qty_ghost FROM inventory WHERE branch_id=$1 AND product_id=$2`, branchID, productID).Scan(&branchReal, &branchGhost); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT qty_real,qty_ghost FROM inventory WHERE branch_id=$1 AND product_id=$2`, warehouseID, productID).Scan(&warehouseReal, &warehouseGhost); err != nil {
		t.Fatal(err)
	}
	if branchReal != 2 || branchGhost != 0 || warehouseReal != 101 || warehouseGhost != 19 {
		t.Fatalf("unexpected inventory after reconciliation: branch=(%d,%d) warehouse=(%d,%d)", branchReal, branchGhost, warehouseReal, warehouseGhost)
	}

	for index, invoiceID := range []string{invoiceIDs[0], invoiceIDs[2]} {
		var updatedAt time.Time
		if err := db.QueryRowContext(ctx, `SELECT updated_at FROM invoices WHERE id=$1`, invoiceID).Scan(&updatedAt); err != nil {
			t.Fatal(err)
		}
		if !updatedAt.Equal(originalUpdated[index]) {
			t.Fatalf("renumber changed updated_at for invoice %s: before=%s after=%s", invoiceID, originalUpdated[index], updatedAt)
		}
	}

	salesService := salesmodule.NewService(db, audit.NewService(db))
	adminInvoices, err := salesService.ListInvoices(ctx, platform.AuthUser{RoleKey: "admin", Portal: "backoffice", Scope: "branch", BranchID: &branchID}, nil)
	if err != nil {
		t.Fatalf("list admin invoices: %v", err)
	}
	visibleFixture := []string{}
	for _, invoice := range adminInvoices {
		for _, invoiceID := range invoiceIDs {
			if invoice["id"] == invoiceID {
				visibleFixture = append(visibleFixture, invoice["invoice_number"].(string))
			}
		}
	}
	sort.Strings(visibleFixture)
	if len(visibleFixture) != 2 || visibleFixture[0] != prefix+"00001" || visibleFixture[1] != prefix+"00002" {
		t.Fatalf("unexpected admin invoice visibility: %v", visibleFixture)
	}

	inventoryService := inventorymodule.NewService(db, audit.NewService(db))
	adminMovements, err := inventoryService.ListMovementHistory(ctx, platform.AuthUser{RoleKey: "admin", Portal: "backoffice", Scope: "branch", BranchID: &branchID}, inventorymodule.MovementHistoryFilter{BranchID: branchID, ProductID: productID, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list admin movements: %v", err)
	}
	visibleTotal := 0
	for _, movement := range adminMovements.Items {
		visibleTotal += movement["quantity_delta"].(int)
		if movement["movement_type"] == "month_end_sale_source_reversal" || movement["stock_bucket"] == "ghost" {
			t.Fatalf("admin saw internal/Ghost movement: %+v", movement)
		}
	}
	if len(adminMovements.Items) != 4 || visibleTotal != 2 {
		t.Fatalf("unexpected admin movement history: count=%d total=%d items=%+v", len(adminMovements.Items), visibleTotal, adminMovements.Items)
	}

	adjustments, err := inventoryService.ListStockAdjustments(ctx, platform.AuthUser{RoleKey: "admin", Portal: "backoffice", Scope: "branch", BranchID: &branchID}, inventorymodule.StockAdjustmentFilter{BranchID: branchID, ProductID: productID, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list admin adjustments: %v", err)
	}
	if len(adjustments.Items) != 1 || adjustments.Items[0]["reason"] != "PRODUCT_RETURN_TO_WAREHOUSE" || adjustments.Items[0]["quantity"] != -1 {
		t.Fatalf("unexpected branch adjustment notes: %+v", adjustments.Items)
	}
	centralAdjustments, err := inventoryService.ListStockAdjustments(ctx, platform.AuthUser{RoleKey: "admin", Portal: "backoffice", Scope: "global"}, inventorymodule.StockAdjustmentFilter{ProductID: productID, ReconciliationID: reconciliationID, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list central adjustments: %v", err)
	}
	if len(centralAdjustments.Items) != 2 {
		t.Fatalf("central admin should see two Real adjustments, got %+v", centralAdjustments.Items)
	}
	for _, adjustment := range centralAdjustments.Items {
		if adjustment["stock_type"] != "REAL" {
			t.Fatalf("central admin saw non-Real adjustment: %+v", adjustment)
		}
	}
	superAdjustments, err := inventoryService.ListStockAdjustments(ctx, platform.AuthUser{RoleKey: "super_admin", Portal: "backoffice", Scope: "global"}, inventorymodule.StockAdjustmentFilter{ProductID: productID, ReconciliationID: reconciliationID, Page: 1, PageSize: 20})
	if err != nil || len(superAdjustments.Items) != 3 {
		t.Fatalf("superadmin adjustment visibility: count=%d err=%v items=%+v", len(superAdjustments.Items), err, superAdjustments.Items)
	}
	if _, err := inventoryService.ListStockAdjustments(ctx, platform.AuthUser{RoleKey: "branch_pos", Portal: "pos", Scope: "branch", BranchID: &branchID}, inventorymodule.StockAdjustmentFilter{}); err == nil {
		t.Fatal("expected POS stock-adjustment history to be forbidden")
	}

	report, err := service.MonthEndReport(ctx, platform.AuthUser{ID: actorID, RoleKey: "super_admin"}, reconciliationID, "", "", "", branchID, "", "", 1, 50)
	if err != nil {
		t.Fatalf("load reconciliation report: %v", err)
	}
	if report.Summary.ActiveInvoicesBefore != 3 || report.Summary.ActiveInvoicesAfter != 2 || report.Summary.RealQuantityDeducted != 2 || report.Summary.BranchRealReturned != 1 || report.Summary.WarehouseRealReceived != 1 || report.Summary.GhostQuantityDeducted != 1 || report.Summary.GhostDeficitQuantity != 0 {
		t.Fatalf("unexpected report summary: %+v", report.Summary)
	}
	hiddenRows := 0
	for _, row := range report.Rows {
		if row.InvoiceID == invoiceIDs[1] {
			hiddenRows++
			if row.Status != "hidden" || row.StockDeductionSource != "ghost" || row.CurrentInvoiceNo != nil || len(row.Movements) < 4 {
				t.Fatalf("unexpected hidden report row: %+v", row)
			}
		}
	}
	if hiddenRows != 1 {
		t.Fatalf("expected one hidden invoice row, got %d", hiddenRows)
	}

	if _, err := service.FinalizeReconciliation(ctx, platform.AuthUser{ID: actorID, RoleKey: "super_admin", Scope: "global", Portal: "backoffice"}, audit.LogEntry{}, ReconciliationInput{
		DateFrom: "2098-02-15", DateTo: "2098-03-15", BranchIDs: []string{branchID},
	}); err == nil || !strings.Contains(err.Error(), "ทับซ้อน") {
		t.Fatalf("expected an overlapping date-range rejection, got %v", err)
	}
}

// database/sql cannot pass []string as a PostgreSQL UUID array directly.
func pqUUIDArray(values []string) any {
	return "{" + strings.Join(values, ",") + "}"
}
