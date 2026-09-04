package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

// SalesFilter is the scope the dashboard is being asked about. A zero time means
// "unbounded on that end", and an empty PaymentStatus means "every bill, paid or
// not" — the two screens disagreed for months because one of them silently
// counted unpaid bills and the other did not, so the choice is now the
// operator's and is applied identically to every figure on the page.
type SalesFilter struct {
	DateFrom      time.Time
	DateTo        time.Time
	PaymentStatus string
}

// predicates renders the filter as SQL fragments against an invoice alias,
// appending to args. DateTo is inclusive of the whole day.
func (f SalesFilter) predicates(alias string, args *[]any) []string {
	clauses := []string{}
	if !f.DateFrom.IsZero() {
		*args = append(*args, f.DateFrom)
		clauses = append(clauses, fmt.Sprintf("%s.issued_at >= $%d", alias, len(*args)))
	}
	if !f.DateTo.IsZero() {
		*args = append(*args, f.DateTo.AddDate(0, 0, 1))
		clauses = append(clauses, fmt.Sprintf("%s.issued_at < $%d", alias, len(*args)))
	}
	if f.PaymentStatus == "paid" || f.PaymentStatus == "unpaid" {
		*args = append(*args, f.PaymentStatus)
		clauses = append(clauses, fmt.Sprintf("%s.payment_status = $%d", alias, len(*args)))
	}
	return clauses
}

func salesFilterFrom(c echo.Context) SalesFilter {
	filter := SalesFilter{PaymentStatus: strings.TrimSpace(c.QueryParam("payment_status"))}
	if raw := strings.TrimSpace(c.QueryParam("date_from")); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			filter.DateFrom = parsed
		}
	}
	if raw := strings.TrimSpace(c.QueryParam("date_to")); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			filter.DateTo = parsed
		}
	}
	return filter
}

// RevenueComparison answers "what did this period look like before the month-end
// close touched it, and what does it look like now" — overall and per branch.
//
// A close is destructive by design: it soft-deletes the bills Ghost Stock could
// cover and rewrites total_amount on the bills it reprices. The only surviving
// record of the original is reconciliation_invoice_snapshots, which holds a row
// for EVERY issued bill in a closed period (not just the ones it changed), so
// "before" is the snapshot when there is one and the live row when there is not.
//
// Bills deleted by hand are excluded from both sides: they were voided, not
// adjusted, and counting them as "before" revenue would overstate what was ever
// really sold. A bill the close hid is recognised by the snapshot it left.
func (s *Service) RevenueComparison(ctx context.Context, filter SalesFilter) (map[string]any, error) {
	args := []any{}
	clauses := append([]string{
		// A bill voided and reissued (an abbreviated tax invoice swapped for a
		// full one) is not revenue on either side — its replacement is.
		"i.invoice_status = 'issued'",
		// Live bills, plus the ones a close hid. The snapshot is what proves a
		// close hid it: hidden_by_id is also stamped by an ordinary manual
		// deletion, and counting those as "before" revenue overstates what was
		// ever really sold.
		"(i.deleted_at IS NULL OR snap.invoice_id IS NOT NULL)",
	}, filter.predicates("i", &args)...)

	query := `
		WITH scope AS (
			SELECT i.branch_id,
			       i.deleted_at,
			       i.total_amount,
			       COALESCE(snap.original_total_amount, i.total_amount) AS before_amount,
			       -- What the close struck off a bill it kept. Counted as hidden,
			       -- not as a repricing, so the split reads the same here as in
			       -- the close's own record.
			       COALESCE((
			           SELECT SUM(ii.line_total) FROM invoice_items ii
			           WHERE ii.invoice_id = i.id AND ii.reconciliation_removed_at IS NOT NULL
			       ), 0) AS struck_amount
			FROM invoices i
			LEFT JOIN reconciliation_invoice_snapshots snap ON snap.invoice_id = i.id
			WHERE ` + strings.Join(clauses, " AND ") + `
		)
		SELECT b.id::text, b.code, b.name,
		       COUNT(scope.branch_id),
		       COALESCE(SUM(scope.before_amount), 0),
		       COUNT(scope.branch_id) FILTER (WHERE scope.deleted_at IS NULL),
		       COALESCE(SUM(scope.total_amount) FILTER (WHERE scope.deleted_at IS NULL), 0),
		       COUNT(scope.branch_id) FILTER (WHERE scope.deleted_at IS NOT NULL),
		       COALESCE(SUM(scope.before_amount) FILTER (WHERE scope.deleted_at IS NOT NULL), 0)
		           + COALESCE(SUM(scope.struck_amount) FILTER (WHERE scope.deleted_at IS NULL), 0)
		FROM branches b
		LEFT JOIN scope ON scope.branch_id = b.id
		WHERE b.active = TRUE AND b.sales_enabled = TRUE
		GROUP BY b.id, b.code, b.name
		ORDER BY COALESCE(SUM(scope.before_amount), 0) DESC, b.name
	`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	branches := []map[string]any{}
	var beforeTotal, afterTotal, hiddenTotal float64
	var beforeCount, afterCount, hiddenCount int
	for rows.Next() {
		var id, code, name string
		var bCount, aCount, hCount int
		var bAmount, aAmount, hAmount float64
		if err := rows.Scan(&id, &code, &name, &bCount, &bAmount, &aCount, &aAmount, &hCount, &hAmount); err != nil {
			return nil, err
		}
		// Whatever the close took off a bill it kept, rather than removing the
		// bill outright.
		repriced := platform.Round2(bAmount - aAmount - hAmount)
		branches = append(branches, map[string]any{
			"branch_id":            id,
			"branch_code":          code,
			"branch_name":          name,
			"before_invoice_count": bCount,
			"before_amount":        platform.Round2(bAmount),
			"after_invoice_count":  aCount,
			"after_amount":         platform.Round2(aAmount),
			"hidden_invoice_count": hCount,
			"hidden_amount":        platform.Round2(hAmount),
			"repriced_reduction":   repriced,
		})
		beforeTotal += bAmount
		afterTotal += aAmount
		hiddenTotal += hAmount
		beforeCount += bCount
		afterCount += aCount
		hiddenCount += hCount
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return map[string]any{
		"before_invoice_count": beforeCount,
		"before_amount":        platform.Round2(beforeTotal),
		"after_invoice_count":  afterCount,
		"after_amount":         platform.Round2(afterTotal),
		"hidden_invoice_count": hiddenCount,
		"hidden_amount":        platform.Round2(hiddenTotal),
		"repriced_reduction":   platform.Round2(beforeTotal - afterTotal - hiddenTotal),
		"branches":             branches,
	}, nil
}

// RevenueComparison is superadmin-only: it is the one place the pre-close
// figures are visible, and admin.central is meant to work from the adjusted
// books alone.
func (h *Handler) RevenueComparison(c echo.Context) error {
	if platform.CurrentUser(c).RoleKey != "super_admin" {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น"))
	}
	result, err := h.service.RevenueComparison(c.Request().Context(), salesFilterFrom(c))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดยอดก่อน/หลังปรับไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, result)
}
