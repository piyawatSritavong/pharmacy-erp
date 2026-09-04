package dashboard

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

// BranchSales powers the dashboard's per-branch sales cards: one row per
// selling branch with the totals split by how the money actually arrived
// (cash vs bank transfer, from invoice_payments) plus when that branch last
// rang up a sale. Branches with no sales still appear, at zero, so the grid
// stays a stable shape.
func (s *Service) BranchSales(ctx context.Context, filter SalesFilter) ([]map[string]any, error) {
	// The filter rides in the LEFT JOIN's ON clause, not a WHERE: a branch with
	// no sales in the chosen window must still appear, at zero, so the grid keeps
	// a stable shape as the operator narrows the range.
	args := []any{}
	joinClauses := append([]string{
		"i.branch_id = b.id",
		"i.invoice_status = 'issued'",
		"i.deleted_at IS NULL",
	}, filter.predicates("i", &args)...)

	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id::text, b.code, b.name,
		       COUNT(DISTINCT i.id),
		       COALESCE(SUM(i.total_amount), 0),
		       COALESCE(SUM(pay.cash), 0),
		       COALESCE(SUM(pay.transfer), 0),
		       MAX(i.issued_at)
		FROM branches b
		LEFT JOIN invoices i
		       ON `+strings.Join(joinClauses, " AND ")+`
		LEFT JOIN LATERAL (
		       SELECT
		           COALESCE(SUM(p.amount) FILTER (WHERE p.payment_type = 'cash'), 0) AS cash,
		           COALESCE(SUM(p.amount) FILTER (WHERE p.payment_type = 'bank_transfer'), 0) AS transfer
		       FROM invoice_payments p
		       WHERE p.invoice_id = i.id
		) pay ON TRUE
		WHERE b.active = TRUE AND b.sales_enabled = TRUE
		GROUP BY b.id, b.code, b.name
		ORDER BY COALESCE(SUM(i.total_amount), 0) DESC, b.name
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, code, name string
		var invoiceCount int
		var total, cash, transfer float64
		var lastSaleAt sql.NullTime
		if err := rows.Scan(&id, &code, &name, &invoiceCount, &total, &cash, &transfer, &lastSaleAt); err != nil {
			return nil, err
		}
		item := map[string]any{
			"branch_id":       id,
			"branch_code":     code,
			"branch_name":     name,
			"invoice_count":   invoiceCount,
			"total_amount":    platform.Round2(total),
			"cash_amount":     platform.Round2(cash),
			"transfer_amount": platform.Round2(transfer),
			"last_sale_at":    nil,
		}
		if lastSaleAt.Valid {
			item["last_sale_at"] = lastSaleAt.Time.Format(time.RFC3339)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *Handler) BranchSales(c echo.Context) error {
	items, err := h.service.BranchSales(c.Request().Context(), salesFilterFrom(c))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load branch sales", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}
