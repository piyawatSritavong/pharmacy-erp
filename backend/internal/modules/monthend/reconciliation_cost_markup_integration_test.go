package monthend

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"
)

// TestCostMarkupMonthEndFlow finalizes a round where the warehouse holds no
// Ghost Stock for the sold product: the cash bill stays, recorded at
// cost × 1.05, while the bank-transfer bill and all stock are untouched.
func TestCostMarkupMonthEndFlow(t *testing.T) {
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
	invoiceIDs := []string{platform.MustUUID(), platform.MustUUID()}
	itemIDs := []string{platform.MustUUID(), platform.MustUUID()}
	reconciliationID := ""

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if reconciliationID != "" {
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
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM inventory WHERE product_id=$1`, productID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM audit_logs WHERE entity_id=$1`, productID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM products WHERE id=$1`, productID)
	})

	if _, err := db.ExecContext(ctx, `
		INSERT INTO products (id,sku,name,description,cost_price,base_selling_price,unit_name,tax_exempt,active,
			max_discount_amount,low_stock_real_threshold,low_stock_ghost_threshold,tracks_expiry,expiry_warning_days,created_at,updated_at)
		VALUES ($1,$2,$3,'cost-markup integration fixture',50,100,'ชิ้น',FALSE,TRUE,0,0,0,FALSE,30,NOW(),NOW())
	`, productID, "MEC-"+unique, "Cost Markup Fixture "+unique); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	// No Ghost Stock anywhere for this product → the cash bill cannot hide.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory (id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at) VALUES
		($1,$2,$3,2,0,NOW(),NOW()),($4,$5,$3,0,0,NOW(),NOW())
	`, platform.MustUUID(), branchID, productID, platform.MustUUID(), warehouseID); err != nil {
		t.Fatalf("insert inventory: %v", err)
	}

	created := []time.Time{time.Date(2097, 3, 1, 2, 0, 0, 0, time.UTC), time.Date(2097, 3, 1, 3, 0, 0, 0, time.UTC)}
	prefix := "KNP-MEC-" + unique + "-"
	for index := range invoiceIDs {
		invoiceNumber := prefix + []string{"00001", "00002"}[index]
		if _, err := db.ExecContext(ctx, `
			INSERT INTO invoices (id,branch_id,invoice_number,customer_name,payment_status,invoice_status,is_government_mode,
				tax_invoice_type,request_full_tax_invoice,subtotal,tax_rate,tax_amount,total_amount,created_by,issued_at,created_at,updated_at)
			VALUES ($1,$2,$3,$4,'paid','issued',FALSE,'abbreviated',FALSE,100,0,0,100,$5,$6,$6,$6)
		`, invoiceIDs[index], branchID, invoiceNumber, "fixture "+invoiceNumber, actorID, created[index]); err != nil {
			t.Fatalf("insert invoice %d: %v", index, err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO invoice_items (id,invoice_id,product_id,actual_product_name,display_name,quantity,stock_bucket,
				unit_price,line_subtotal,tax_rate,tax_amount,line_total,price_source,override_reason,cost_snapshot,
				sold_quantity,sold_unit_price,unit_conversion_qty,created_at)
			VALUES ($1,$2,$3,$4,$4,1,'real',100,100,0,0,100,'base','',50,1,100,1,$5)
		`, itemIDs[index], invoiceIDs[index], productID, "Cost Markup Fixture "+unique, created[index]); err != nil {
			t.Fatalf("insert invoice item %d: %v", index, err)
		}
		paymentType := []string{"cash", "bank_transfer"}[index]
		if _, err := db.ExecContext(ctx, `INSERT INTO invoice_payments (id,invoice_id,payment_type,amount,reference_code,notes,created_by,created_at) VALUES ($1,$2,$3,100,'','',$4,$5)`, platform.MustUUID(), invoiceIDs[index], paymentType, actorID, created[index]); err != nil {
			t.Fatalf("insert payment %d: %v", index, err)
		}
	}

	service := NewService(db, audit.NewService(db))
	superadmin := platform.AuthUser{ID: actorID, RoleKey: "super_admin", Scope: "global", Portal: "backoffice"}
	scope := ReconciliationInput{DateFrom: "2097-03-01", DateTo: "2097-03-31", BranchIDs: []string{branchID}}

	rejected := scope
	rejected.AdjustmentPercent = 4
	if _, err := service.FinalizeReconciliation(ctx, superadmin, audit.LogEntry{}, rejected); err == nil || !strings.Contains(err.Error(), "5 ถึง 10") {
		t.Fatalf("expected a markup below five percent to be rejected, got %v", err)
	}

	preview, err := service.PreviewReconciliation(ctx, superadmin, scope)
	if err != nil {
		t.Fatalf("preview reconciliation: %v", err)
	}
	if preview["final_revenue"] != 152.5 || preview["target_revenue"] != 152.5 || preview["adjustment_percent"] != 5.0 || preview["repriced_invoice_count"] != 1 || preview["hidden_invoice_count"] != 0 {
		t.Fatalf("unexpected preview: final=%v target=%v percent=%v repriced=%v hidden=%v", preview["final_revenue"], preview["target_revenue"], preview["adjustment_percent"], preview["repriced_invoice_count"], preview["hidden_invoice_count"])
	}

	scope.AdjustmentPercent = 5
	result, err := service.FinalizeReconciliation(ctx, superadmin, audit.LogEntry{}, scope)
	if err != nil {
		t.Fatalf("finalize reconciliation: %v", err)
	}
	reconciliationID = result["id"].(string)
	if result["original_revenue"] != 200.0 || result["suppressed_revenue"] != 0.0 || result["adjustment_reduction"] != 47.5 ||
		result["final_revenue"] != 152.5 || result["target_revenue"] != 152.5 || result["adjustment_percent"] != 5.0 ||
		result["adjusted_item_count"] != 1 || result["suppressed_invoice_count"] != 0 || result["reconciliation_mode"] != reconciliationModeCostMarkup {
		t.Fatalf("unexpected reconciliation record: %+v", result)
	}

	var deleted *time.Time
	var invoiceNumber string
	var subtotal, taxAmount, total, paid float64
	if err := db.QueryRowContext(ctx, `
		SELECT i.deleted_at,i.invoice_number,i.subtotal,i.tax_amount,i.total_amount,
		       (SELECT COALESCE(SUM(amount),0) FROM invoice_payments WHERE invoice_id=i.id)
		FROM invoices i WHERE i.id=$1
	`, invoiceIDs[0]).Scan(&deleted, &invoiceNumber, &subtotal, &taxAmount, &total, &paid); err != nil {
		t.Fatal(err)
	}
	if deleted != nil || invoiceNumber != prefix+"00001" || subtotal != 52.5 || taxAmount != 0 || total != 52.5 || paid != 52.5 {
		t.Fatalf("cash bill should stay at 52.50 with matching payment: deleted=%v number=%s subtotal=%.2f tax=%.2f total=%.2f paid=%.2f", deleted, invoiceNumber, subtotal, taxAmount, total, paid)
	}
	var unitPrice, soldUnitPrice, lineTotal, reconciliationDiscount float64
	var reconciledAt *time.Time
	if err := db.QueryRowContext(ctx, `SELECT unit_price,sold_unit_price,line_total,reconciliation_discount_amount,reconciled_at FROM invoice_items WHERE id=$1`, itemIDs[0]).Scan(&unitPrice, &soldUnitPrice, &lineTotal, &reconciliationDiscount, &reconciledAt); err != nil {
		t.Fatal(err)
	}
	if unitPrice != 52.5 || soldUnitPrice != 52.5 || lineTotal != 52.5 || reconciliationDiscount != 47.5 || reconciledAt == nil {
		t.Fatalf("unexpected repriced line: unit=%.2f sold=%.2f total=%.2f discount=%.2f reconciled=%v", unitPrice, soldUnitPrice, lineTotal, reconciliationDiscount, reconciledAt)
	}
	if err := db.QueryRowContext(ctx, `SELECT i.invoice_number,i.total_amount,(SELECT COALESCE(SUM(amount),0) FROM invoice_payments WHERE invoice_id=i.id) FROM invoices i WHERE i.id=$1`, invoiceIDs[1]).Scan(&invoiceNumber, &total, &paid); err != nil {
		t.Fatal(err)
	}
	if invoiceNumber != prefix+"00002" || total != 100 || paid != 100 {
		t.Fatalf("transfer bill must stay untouched: number=%s total=%.2f paid=%.2f", invoiceNumber, total, paid)
	}

	var branchReal, branchGhost, warehouseReal, warehouseGhost int
	if err := db.QueryRowContext(ctx, `SELECT qty_real,qty_ghost FROM inventory WHERE branch_id=$1 AND product_id=$2`, branchID, productID).Scan(&branchReal, &branchGhost); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT qty_real,qty_ghost FROM inventory WHERE branch_id=$1 AND product_id=$2`, warehouseID, productID).Scan(&warehouseReal, &warehouseGhost); err != nil {
		t.Fatal(err)
	}
	if branchReal != 2 || branchGhost != 0 || warehouseReal != 0 || warehouseGhost != 0 {
		t.Fatalf("repricing must not move stock: branch=(%d,%d) warehouse=(%d,%d)", branchReal, branchGhost, warehouseReal, warehouseGhost)
	}

	var priceLogs, invoiceLogs int
	var oldPrice, newPrice, variance float64
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FILTER (WHERE log_type='price_adjusted'),COUNT(*) FILTER (WHERE log_type='invoice_repriced'),
		       COALESCE(MAX(old_unit_price) FILTER (WHERE log_type='price_adjusted'),0),
		       COALESCE(MAX(new_unit_price) FILTER (WHERE log_type='price_adjusted'),0),
		       COALESCE(MAX(variance_amount) FILTER (WHERE log_type='invoice_repriced'),0)
		FROM reconciliation_logs WHERE reconciliation_id=$1
	`, reconciliationID).Scan(&priceLogs, &invoiceLogs, &oldPrice, &newPrice, &variance); err != nil {
		t.Fatal(err)
	}
	if priceLogs != 1 || invoiceLogs != 1 || oldPrice != 100 || newPrice != 52.5 || variance != 47.5 {
		t.Fatalf("unexpected logs: price=%d invoice=%d old=%.2f new=%.2f variance=%.2f", priceLogs, invoiceLogs, oldPrice, newPrice, variance)
	}

	report, err := service.MonthEndReport(ctx, superadmin, reconciliationID, "", "", "", branchID, "", "", 1, 50)
	if err != nil {
		t.Fatalf("load reconciliation report: %v", err)
	}
	if report.Summary.ActiveInvoicesBefore != 2 || report.Summary.ActiveInvoicesAfter != 2 || report.Summary.RevenueBefore != 200 || report.Summary.RevenueAfter != 152.5 || report.Summary.GhostQuantityDeducted != 0 {
		t.Fatalf("unexpected report summary: %+v", report.Summary)
	}
	adjustedRows := 0
	for _, row := range report.Rows {
		if row.InvoiceID != invoiceIDs[0] {
			continue
		}
		adjustedRows++
		if row.Status != "adjusted" || row.AdjustedPrice == nil || *row.AdjustedPrice != 52.5 || row.DiscountAmount != 47.5 || row.CurrentInvoiceNo == nil || row.StockDeductionSource != "real" {
			t.Fatalf("unexpected adjusted report row: %+v", row)
		}
	}
	if adjustedRows != 1 {
		t.Fatalf("expected one adjusted report row, got %d", adjustedRows)
	}
}
