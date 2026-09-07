package monthend

import (
	"context"
	"os"
	"testing"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/lib/pq"
)

func TestMonthEndReportCombinesSnapshotsInvoicesAndGhostLogs(t *testing.T) {
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

	var actorID, branchID, productID, productName string
	var cost float64
	if err := db.QueryRowContext(ctx, `
		SELECT u.id::text,b.id::text,p.id::text,p.name,p.cost_price
		FROM branches b
		CROSS JOIN LATERAL (SELECT id,name,cost_price FROM products WHERE active=TRUE ORDER BY id LIMIT 1) p
		CROSS JOIN LATERAL (SELECT id FROM users WHERE email='superadmin@erp.local' LIMIT 1) u
		WHERE b.active=TRUE AND b.branch_type<>'main_warehouse'
		ORDER BY b.id
		LIMIT 1
	`).Scan(&actorID, &branchID, &productID, &productName, &cost); err != nil {
		t.Fatalf("load fixture source: %v", err)
	}
	invoiceID, itemID := platform.MustUUID(), platform.MustUUID()
	invoiceNumber := "REPORT-" + invoiceID[:8] + "-00001"
	quantity := 1
	unitPrice, lineSubtotal, lineTax, lineTotal := 100.0, 100.0, 0.0, 100.0
	invoiceSubtotal, invoiceTax, invoiceTotal := 100.0, 0.0, 100.0
	if _, err := db.ExecContext(ctx, `
		INSERT INTO invoices (id,branch_id,invoice_number,customer_name,payment_status,invoice_status,
			is_government_mode,tax_invoice_type,request_full_tax_invoice,subtotal,tax_rate,tax_amount,total_amount,
			created_by,issued_at,created_at,updated_at,payment_method)
		VALUES ($1,$2,$3,'report fixture','paid','issued',FALSE,'abbreviated',FALSE,$4,0,$5,$6,$7,NOW(),NOW(),NOW(),'cash')
	`, invoiceID, branchID, invoiceNumber, invoiceSubtotal, invoiceTax, invoiceTotal, actorID); err != nil {
		t.Fatalf("insert fixture invoice: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO invoice_items (id,invoice_id,product_id,actual_product_name,display_name,quantity,stock_bucket,
			unit_price,line_subtotal,tax_rate,tax_amount,line_total,price_source,override_reason,cost_snapshot,created_at)
		VALUES ($1,$2,$3,$4,$4,$5,'real',$6,$7,0,$8,$9,'base','',$10,NOW())
	`, itemID, invoiceID, productID, productName, quantity, unitPrice, lineSubtotal, lineTax, lineTotal, cost); err != nil {
		t.Fatalf("insert fixture item: %v", err)
	}

	reconciliationID := platform.MustUUID()
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM reconciliation_logs WHERE reconciliation_id=$1`, reconciliationID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM reconciliation_item_snapshots WHERE reconciliation_id=$1`, reconciliationID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM reconciliation_invoice_snapshots WHERE reconciliation_id=$1`, reconciliationID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM month_end_reconciliations WHERE id=$1`, reconciliationID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM invoice_items WHERE id=$1`, itemID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM invoices WHERE id=$1`, invoiceID)
	})

	if _, err := db.ExecContext(ctx, `
		INSERT INTO month_end_reconciliations (
			id,reconciliation_number,period_start,period_end,branch_ids,target_revenue,
			original_revenue,suppressed_revenue,adjustment_reduction,final_revenue,
			suppressed_invoice_count,adjusted_item_count,source_hash,finalized_by
		) VALUES ($1,$2,'2099-01-01','2099-01-31',$3,0,$4,0,$4,0,0,1,'integration-test',$5)
	`, reconciliationID, "MER-TEST-"+reconciliationID, pq.Array([]string{branchID}), invoiceTotal, actorID); err != nil {
		t.Fatalf("insert reconciliation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO reconciliation_invoice_snapshots (
			reconciliation_id,invoice_id,branch_id,original_invoice_number,issued_at,
			payment_method,request_full_tax_invoice,original_subtotal,original_tax_amount,
			original_total_amount,original_invoice_data,invoice_created_at
		) VALUES ($1,$2,$3,$4,NOW(),'cash',FALSE,$5,$6,$7,'{}',NOW())
	`, reconciliationID, invoiceID, branchID, invoiceNumber, invoiceSubtotal, invoiceTax, invoiceTotal); err != nil {
		t.Fatalf("insert invoice snapshot: %v", err)
	}
	originalPrice := unitPrice + 1
	if _, err := db.ExecContext(ctx, `
		INSERT INTO reconciliation_item_snapshots (
			reconciliation_id,invoice_item_id,invoice_id,product_id,product_name,quantity,
			original_unit_price,original_line_subtotal,original_tax_amount,original_line_total,
			original_stock_bucket,effective_stock_bucket,original_item_data
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'ghost','{}')
	`, reconciliationID, itemID, invoiceID, productID, productName, quantity, originalPrice,
		lineSubtotal+float64(quantity), lineTax, lineTotal+float64(quantity), "real"); err != nil {
		t.Fatalf("insert item snapshot: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO reconciliation_logs (
			id,reconciliation_id,log_type,invoice_id,invoice_item_id,branch_id,product_id,
			original_invoice_number,old_unit_price,new_unit_price,variance_amount,
			stock_bucket,stock_quantity,before_data,after_data,actor_id,movement_role,ghost_stock_deducted
		) VALUES
		($1,$2,'price_adjusted',$3,$4,$5,$6,$7,$8,$9,$10,NULL,0,'{}','{}',$11,'',0),
		($12,$2,'stock_deducted',$3,$4,$5,$6,$7,NULL,NULL,0,'ghost',2,'{}','{}',$11,'invoice_ghost_source',2)
	`, platform.MustUUID(), reconciliationID, invoiceID, itemID, branchID, productID,
		invoiceNumber, originalPrice, unitPrice, float64(quantity), actorID, platform.MustUUID()); err != nil {
		t.Fatalf("insert reconciliation logs: %v", err)
	}

	service := NewService(db, audit.NewService(db))
	report, err := service.MonthEndReport(ctx, platform.AuthUser{ID: actorID, RoleKey: "super_admin"}, "", "2099-01", "", "", branchID, "", "", 1, 50)
	if err != nil {
		t.Fatalf("load report: %v", err)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("expected one report row, got %d", len(report.Rows))
	}
	row := report.Rows[0]
	if row.OriginalInvoiceNo != invoiceNumber || row.AdjustedPrice == nil || row.StockDeductionSource != "ghost" || row.Status != "adjusted" {
		t.Fatalf("unexpected report row: %+v", row)
	}
	if report.Summary.GhostQuantityDeducted != 2 || report.Summary.RealQuantityDeducted != 0 {
		t.Fatalf("unexpected stock summary: %+v", report.Summary)
	}
}
