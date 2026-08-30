package dashboard

import (
	"context"

	"pharmacy-erp/backend/internal/platform"
)

func (s *Service) refactoredGlobalSummary(ctx context.Context, user platform.AuthUser) (map[string]any, error) {
	var totalSales, governmentSales, unpaidTotal float64
	var governmentCount, unpaidCount, activeTransfers, pendingRequests, pendingMonthEnd int
	var qtyReal, qtyGhost int64

	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(total_amount), 0),
			COALESCE(SUM(CASE WHEN is_government_mode THEN total_amount ELSE 0 END), 0),
			COUNT(*) FILTER (WHERE is_government_mode)
		FROM invoices
		WHERE invoice_status = 'issued' AND deleted_at IS NULL
	`).Scan(&totalSales, &governmentSales, &governmentCount); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(total_amount), 0)
		FROM invoices
		WHERE payment_status = 'unpaid' AND invoice_status = 'issued' AND deleted_at IS NULL
	`).Scan(&unpaidCount, &unpaidTotal); err != nil {
		return nil, err
	}
	if user.RoleKey == "super_admin" {
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty_real),0),COALESCE(SUM(qty_ghost),0) FROM inventory`).Scan(&qtyReal, &qtyGhost); err != nil {
			return nil, err
		}
	} else if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty_real),0) FROM inventory`).Scan(&qtyReal); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transfers WHERE status IN ('requested', 'in_transit')
	`).Scan(&activeTransfers); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stock_transfer_requests WHERE status = 'pending'
	`).Scan(&pendingRequests); err != nil {
		return nil, err
	}
	if user.RoleKey == "super_admin" {
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM month_end_workpapers
			WHERE status IN ('DRAFT', 'CALCULATING', 'PENDING_APPROVAL', 'APPROVED')
		`).Scan(&pendingMonthEnd); err != nil {
			return nil, err
		}
	}
	recentInvoices, err := s.listRecentInvoices(ctx, "global", "", "")
	if err != nil {
		return nil, err
	}
	metrics := []map[string]any{
		{"key": "sales_total", "label": "ยอดขายรวม", "value": totalSales},
		{"key": "government_sales_total", "label": "ยอดขาย รพ.สต.", "value": governmentSales},
		{"key": "government_invoice_count", "label": "จำนวนใบขาย รพ.สต.", "value": governmentCount},
		{"key": "unpaid_count", "label": "ใบขายค้างชำระ", "value": unpaidCount},
		{"key": "unpaid_total", "label": "ยอดค้างชำระ", "value": unpaidTotal},
		{"key": "qty_real", "label": "สต๊อกจริงรวม", "value": qtyReal},
	}
	if user.RoleKey == "super_admin" {
		metrics = append(metrics, map[string]any{"key": "qty_ghost", "label": "สต๊อกผีรวม", "value": qtyGhost})
	}
	metrics = append(metrics,
		map[string]any{"key": "active_transfers", "label": "ใบโอนที่กำลังดำเนินการ", "value": activeTransfers},
		map[string]any{"key": "pending_stock_requests", "label": "คำขอสินค้ารอตรวจสอบ", "value": pendingRequests},
	)
	if user.RoleKey == "super_admin" {
		metrics = append(metrics, map[string]any{"key": "pending_month_end", "label": "รอบสิ้นเดือนที่ยังไม่ปิด", "value": pendingMonthEnd})
	}
	pending := map[string]any{"stock_transfers": pendingRequests}
	if user.RoleKey == "super_admin" {
		pending["month_end_workpapers"] = pendingMonthEnd
	}
	return map[string]any{
		"scope":            "global",
		"metrics":          metrics,
		"pending_requests": pending,
		"recent_invoices":  recentInvoices,
		"recent_items":     map[string]any{"invoices": recentInvoices},
	}, nil
}
