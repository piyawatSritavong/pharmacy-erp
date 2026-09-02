package dashboard

import (
	"context"
	"net/http"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

func bangkokDayBounds(now time.Time) (time.Time, time.Time) {
	location := time.FixedZone("Asia/Bangkok", 7*60*60)
	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}

// TodayBranchSales returns each selling branch's takings for today (Asia/Bangkok),
// so the dashboard can chart the day at a glance. Branches that have not sold yet
// still appear at zero, keeping the chart a stable shape through the day.
func (s *Service) TodayBranchSales(ctx context.Context) (map[string]any, error) {
	start, end := bangkokDayBounds(time.Now())
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.code, b.name,
		       COUNT(DISTINCT i.id),
		       COALESCE(SUM(i.total_amount), 0)
		FROM branches b
		LEFT JOIN invoices i
		       ON i.branch_id = b.id AND i.invoice_status = 'issued' AND i.deleted_at IS NULL
		      AND i.issued_at >= $1 AND i.issued_at < $2
		WHERE b.active = TRUE AND b.sales_enabled = TRUE AND b.branch_type <> 'main_warehouse'
		GROUP BY b.code, b.name
		ORDER BY b.name
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	branches := []map[string]any{}
	total := 0.0
	invoiceCount := 0
	for rows.Next() {
		var code, name string
		var count int
		var amount float64
		if err := rows.Scan(&code, &name, &count, &amount); err != nil {
			return nil, err
		}
		total += amount
		invoiceCount += count
		branches = append(branches, map[string]any{
			"branch_code": code, "branch_name": name,
			"invoice_count": count, "total_amount": platform.Round2(amount),
		})
	}
	return map[string]any{
		"date":          platform.InBangkok(start).Format("2006-01-02"),
		"total_amount":  platform.Round2(total),
		"invoice_count": invoiceCount,
		"branches":      branches,
	}, rows.Err()
}

// LowStock lists every selling branch's products at or below their reorder point,
// preferring a per-branch threshold over the product default. The dashboard
// splits these into a company-wide table and one table per branch.
func (s *Service) LowStock(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.code, b.name, p.name, p.sku, inv.qty_real,
		       COALESCE(bps.low_stock_real_threshold, p.low_stock_real_threshold) AS threshold
		FROM inventory inv
		INNER JOIN branches b ON b.id = inv.branch_id
		       AND b.active = TRUE AND b.sales_enabled = TRUE AND b.branch_type <> 'main_warehouse'
		INNER JOIN products p ON p.id = inv.product_id AND p.active = TRUE
		LEFT JOIN branch_product_settings bps
		       ON bps.branch_id = inv.branch_id AND bps.product_id = inv.product_id
		WHERE COALESCE(bps.low_stock_real_threshold, p.low_stock_real_threshold) > 0
		  AND inv.qty_real <= COALESCE(bps.low_stock_real_threshold, p.low_stock_real_threshold)
		ORDER BY b.name,
		         inv.qty_real - COALESCE(bps.low_stock_real_threshold, p.low_stock_real_threshold),
		         p.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var branchCode, branchName, productName, sku string
		var qtyReal, threshold int
		if err := rows.Scan(&branchCode, &branchName, &productName, &sku, &qtyReal, &threshold); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"branch_code": branchCode, "branch_name": branchName,
			"product_name": productName, "sku": sku,
			"qty_real": qtyReal, "threshold": threshold,
		})
	}
	return items, rows.Err()
}

// Notifications aggregates what needs attention across the back office: shelves
// running low, requisitions waiting to be reviewed, and returns waiting to be
// claimed. The bell shows the sum; each row links to where the work is done.
func (s *Service) Notifications(ctx context.Context) ([]map[string]any, error) {
	var lowStock, requisitions, claims int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM inventory inv
		INNER JOIN branches b ON b.id = inv.branch_id
		       AND b.active = TRUE AND b.sales_enabled = TRUE AND b.branch_type <> 'main_warehouse'
		INNER JOIN products p ON p.id = inv.product_id AND p.active = TRUE
		LEFT JOIN branch_product_settings bps
		       ON bps.branch_id = inv.branch_id AND bps.product_id = inv.product_id
		WHERE COALESCE(bps.low_stock_real_threshold, p.low_stock_real_threshold) > 0
		  AND inv.qty_real <= COALESCE(bps.low_stock_real_threshold, p.low_stock_real_threshold)
	`).Scan(&lowStock); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stock_transfer_requests WHERE status = 'pending'`).Scan(&requisitions); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_returns WHERE status = 'pending'`).Scan(&claims); err != nil {
		return nil, err
	}
	return []map[string]any{
		{"type": "low_stock", "label": "สินค้าใกล้หมด", "detail": "รายการที่ถึงจุดแจ้งเตือนสต๊อก", "count": lowStock, "href": "/dashboard"},
		{"type": "requisition", "label": "คำขอเบิกสินค้า", "detail": "ใบเบิกจากสาขาที่รอตรวจสอบ", "count": requisitions, "href": "/requisitions"},
		{"type": "claim", "label": "เคลม/คืนสินค้า", "detail": "คำขอคืนสินค้าที่รอดำเนินการ", "count": claims, "href": "/claims"},
	}, nil
}

func (h *Handler) TodayBranchSales(c echo.Context) error {
	result, err := h.service.TodayBranchSales(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load today's sales", err))
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) LowStock(c echo.Context) error {
	items, err := h.service.LowStock(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load low stock", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Notifications(c echo.Context) error {
	items, err := h.service.Notifications(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load notifications", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}
