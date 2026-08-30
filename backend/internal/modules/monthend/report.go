package monthend

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

type MonthEndReportSummary struct {
	ActiveInvoicesBefore  int     `json:"active_invoices_before"`
	ActiveInvoicesAfter   int     `json:"active_invoices_after"`
	RevenueBefore         float64 `json:"revenue_before"`
	RevenueAfter          float64 `json:"revenue_after"`
	RealQuantityDeducted  int     `json:"real_quantity_deducted"`
	GhostQuantityDeducted int     `json:"ghost_quantity_deducted"`
	BranchRealReturned    int     `json:"branch_real_returned"`
	WarehouseRealReceived int     `json:"warehouse_real_received"`
	GhostDeficitQuantity  int     `json:"ghost_deficit_quantity"`
}

type MonthEndReportMovement struct {
	Role       string `json:"role"`
	BranchID   string `json:"branch_id"`
	BranchName string `json:"branch_name"`
	StockType  string `json:"stock_type"`
	Quantity   int    `json:"quantity"`
	Reason     string `json:"reason"`
}

type MonthEndReportRow struct {
	ReconciliationID     string                   `json:"reconciliation_id"`
	BranchID             string                   `json:"branch_id"`
	BranchName           string                   `json:"branch_name"`
	InvoiceID            string                   `json:"invoice_id"`
	InvoiceItemID        string                   `json:"invoice_item_id"`
	OriginalInvoiceNo    string                   `json:"original_invoice_no"`
	CurrentInvoiceNo     *string                  `json:"current_invoice_no"`
	ProductName          string                   `json:"product_name"`
	Quantity             int                      `json:"quantity"`
	OriginalPrice        float64                  `json:"original_price"`
	AdjustedPrice        *float64                 `json:"adjusted_price"`
	DiscountAmount       float64                  `json:"discount_amount"`
	PaymentMethod        string                   `json:"payment_method"`
	Status               string                   `json:"status"`
	StockDeductionSource string                   `json:"stock_deduction_source"`
	Movements            []MonthEndReportMovement `json:"movements"`
}

type MonthEndReportResult struct {
	ReconciliationID  string                `json:"reconciliation_id,omitempty"`
	ReconciliationIDs []string              `json:"reconciliation_ids"`
	Period            string                `json:"period,omitempty"`
	DateFrom          string                `json:"date_from"`
	DateTo            string                `json:"date_to"`
	BranchID          string                `json:"branch_id,omitempty"`
	Summary           MonthEndReportSummary `json:"summary"`
	Rows              []MonthEndReportRow   `json:"rows"`
	Pagination        map[string]int        `json:"pagination"`
}

func normalizeMonthEndReportPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

type monthEndReportFilter struct {
	reconciliationArg any
	dateFromArg       any
	dateToArg         any
	branchArg         any
	branchID          string
}

func normalizeMonthEndReportFilter(reconciliationID, period, dateFrom, dateTo, branchID string) (monthEndReportFilter, error) {
	filter := monthEndReportFilter{}
	reconciliationID = strings.TrimSpace(reconciliationID)
	if reconciliationID != "" {
		parsed, err := uuid.Parse(reconciliationID)
		if err != nil {
			return filter, platform.NewError(http.StatusBadRequest, "รหัสรอบสรุปสิ้นเดือนไม่ถูกต้อง")
		}
		filter.reconciliationArg = parsed.String()
	} else {
		input := ReconciliationInput{Month: strings.TrimSpace(period), DateFrom: strings.TrimSpace(dateFrom), DateTo: strings.TrimSpace(dateTo)}
		_, _, normalizedFrom, normalizedTo, err := reconciliationRange(input)
		if err != nil {
			return filter, err
		}
		filter.dateFromArg = normalizedFrom
		filter.dateToArg = normalizedTo
	}
	branchID = strings.TrimSpace(branchID)
	if branchID != "" {
		parsed, err := uuid.Parse(branchID)
		if err != nil {
			return filter, platform.NewError(http.StatusBadRequest, "รหัสสาขาไม่ถูกต้อง")
		}
		filter.branchID = parsed.String()
		filter.branchArg = filter.branchID
	}
	return filter, nil
}

const selectedReconciliationsSQL = `
	SELECT r.id
	FROM month_end_reconciliations r
	WHERE (($1::uuid IS NOT NULL AND r.id=$1::uuid)
	    OR ($1::uuid IS NULL AND r.period_start=$2::date AND r.period_end=$3::date))
	  AND ($4::uuid IS NULL OR $4::uuid=ANY(r.branch_ids))`

func (s *Service) MonthEndReport(ctx context.Context, user platform.AuthUser, reconciliationID, period, dateFrom, dateTo, branchID string, page, pageSize int) (MonthEndReportResult, error) {
	if user.RoleKey != "super_admin" {
		return MonthEndReportResult{}, platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบสูงสุดเท่านั้น")
	}
	filter, err := normalizeMonthEndReportFilter(reconciliationID, period, dateFrom, dateTo, branchID)
	if err != nil {
		return MonthEndReportResult{}, err
	}
	page, pageSize = normalizeMonthEndReportPage(page, pageSize)
	args := []any{filter.reconciliationArg, filter.dateFromArg, filter.dateToArg, filter.branchArg}

	var reconciliationIDs pq.StringArray
	var periodStart, periodEnd sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
		WITH selected AS (`+selectedReconciliationsSQL+`)
		SELECT COALESCE(ARRAY_AGG(r.id::text ORDER BY r.finalized_at,r.id),ARRAY[]::text[]),
		       MIN(r.period_start)::timestamptz,MAX(r.period_end)::timestamptz
		FROM month_end_reconciliations r INNER JOIN selected s ON s.id=r.id
	`, args...).Scan(&reconciliationIDs, &periodStart, &periodEnd); err != nil {
		return MonthEndReportResult{}, err
	}
	if len(reconciliationIDs) == 0 || !periodStart.Valid || !periodEnd.Valid {
		return MonthEndReportResult{}, platform.NewError(http.StatusNotFound, "ไม่พบรอบสรุปสิ้นเดือนที่เลือก")
	}

	result := MonthEndReportResult{
		ReconciliationIDs: []string(reconciliationIDs),
		DateFrom:          periodStart.Time.Format("2006-01-02"),
		DateTo:            periodEnd.Time.Format("2006-01-02"),
		BranchID:          filter.branchID,
		Rows:              []MonthEndReportRow{},
	}
	if len(reconciliationIDs) == 1 {
		result.ReconciliationID = reconciliationIDs[0]
	}
	if result.DateFrom[:7] == result.DateTo[:7] && result.DateFrom[8:] == "01" {
		lastOfMonth, _ := time.Parse("2006-01-02", result.DateTo)
		if lastOfMonth.AddDate(0, 0, 1).Day() == 1 {
			result.Period = result.DateFrom[:7]
		}
	}

	if err := s.db.QueryRowContext(ctx, `
		WITH selected AS (`+selectedReconciliationsSQL+`), invoice_scope AS (
			SELECT snapshot.reconciliation_id,snapshot.invoice_id,snapshot.payment_method,
			       snapshot.original_total_amount,invoice.total_amount,invoice.deleted_at
			FROM reconciliation_invoice_snapshots snapshot
			INNER JOIN selected s ON s.id=snapshot.reconciliation_id
			INNER JOIN invoices invoice ON invoice.id=snapshot.invoice_id
			WHERE $4::uuid IS NULL OR snapshot.branch_id=$4::uuid
		), item_totals AS (
			SELECT COALESCE(SUM(item.quantity) FILTER (WHERE item.effective_stock_bucket='real'),0)::integer AS real_deducted,
			       COALESCE(SUM(item.quantity) FILTER (WHERE item.effective_stock_bucket='ghost'),0)::integer AS ghost_deducted
			FROM reconciliation_item_snapshots item
			INNER JOIN reconciliation_invoice_snapshots snapshot
			        ON snapshot.reconciliation_id=item.reconciliation_id AND snapshot.invoice_id=item.invoice_id
			INNER JOIN selected s ON s.id=item.reconciliation_id
			WHERE $4::uuid IS NULL OR snapshot.branch_id=$4::uuid
		), stock_totals AS (
			SELECT COALESCE(SUM(log.stock_quantity) FILTER (WHERE log.movement_role='branch_return_dispatch'),0)::integer AS branch_returned,
			       COALESCE(SUM(log.stock_quantity) FILTER (WHERE log.movement_role='warehouse_return_receive'),0)::integer AS warehouse_received,
			       COALESCE(SUM(log.ghost_stock_deducted) FILTER (WHERE log.movement_role='invoice_ghost_source'),0)::integer AS ghost_deducted,
			       COALESCE(SUM(log.stock_quantity) FILTER (WHERE log.movement_role='warehouse_ghost_deficit'),0)::integer AS ghost_deficit
			FROM reconciliation_logs log
			INNER JOIN selected s ON s.id=log.reconciliation_id
			LEFT JOIN reconciliation_invoice_snapshots snapshot
			       ON snapshot.reconciliation_id=log.reconciliation_id AND snapshot.invoice_id=log.invoice_id
			WHERE $4::uuid IS NULL OR snapshot.branch_id=$4::uuid
		)
		SELECT COUNT(DISTINCT invoice_id)::integer,
		       COUNT(DISTINCT invoice_id) FILTER (WHERE deleted_at IS NULL)::integer,
		       COALESCE(SUM(original_total_amount) FILTER (WHERE payment_method<>'unpaid'),0),
		       COALESCE(SUM(total_amount) FILTER (WHERE payment_method<>'unpaid' AND deleted_at IS NULL),0),
		       COALESCE((SELECT real_deducted FROM item_totals),0),
		       CASE WHEN COALESCE((SELECT ghost_deducted FROM stock_totals),0)>0
		            THEN (SELECT ghost_deducted FROM stock_totals)
		            ELSE COALESCE((SELECT ghost_deducted FROM item_totals),0) END,
		       COALESCE((SELECT branch_returned FROM stock_totals),0),
		       COALESCE((SELECT warehouse_received FROM stock_totals),0),
		       COALESCE((SELECT ghost_deficit FROM stock_totals),0)
		FROM invoice_scope
	`, args...).Scan(
		&result.Summary.ActiveInvoicesBefore,
		&result.Summary.ActiveInvoicesAfter,
		&result.Summary.RevenueBefore,
		&result.Summary.RevenueAfter,
		&result.Summary.RealQuantityDeducted,
		&result.Summary.GhostQuantityDeducted,
		&result.Summary.BranchRealReturned,
		&result.Summary.WarehouseRealReceived,
		&result.Summary.GhostDeficitQuantity,
	); err != nil {
		return MonthEndReportResult{}, err
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `
		WITH selected AS (`+selectedReconciliationsSQL+`)
		SELECT COUNT(*)
		FROM reconciliation_item_snapshots item
		INNER JOIN reconciliation_invoice_snapshots invoice
		        ON invoice.reconciliation_id=item.reconciliation_id AND invoice.invoice_id=item.invoice_id
		INNER JOIN selected s ON s.id=item.reconciliation_id
		WHERE $4::uuid IS NULL OR invoice.branch_id=$4::uuid
	`, args...).Scan(&total); err != nil {
		return MonthEndReportResult{}, err
	}

	queryArgs := append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `
		WITH selected AS (`+selectedReconciliationsSQL+`), price_changes AS (
			SELECT DISTINCT ON (log.reconciliation_id,log.invoice_item_id)
			       log.reconciliation_id,log.invoice_item_id,log.new_unit_price,log.variance_amount
			FROM reconciliation_logs log INNER JOIN selected s ON s.id=log.reconciliation_id
			WHERE log.log_type='price_adjusted' AND log.invoice_item_id IS NOT NULL
			ORDER BY log.reconciliation_id,log.invoice_item_id,log.created_at DESC,log.id DESC
		), movement_details AS (
			SELECT log.reconciliation_id,log.invoice_item_id,
			       JSONB_AGG(JSONB_BUILD_OBJECT(
			           'role',log.movement_role,
			           'branch_id',COALESCE(log.branch_id::text,''),
			           'branch_name',COALESCE(branch.name,''),
			           'stock_type',UPPER(COALESCE(log.stock_bucket,'')),
			           'quantity',CASE
			               WHEN log.movement_role IN ('branch_return_dispatch','invoice_ghost_source') THEN -log.stock_quantity
			               WHEN log.movement_role IN ('sale_source_reversal','warehouse_return_receive') THEN log.stock_quantity
			               ELSE 0 END,
			           'reason',log.adjustment_reason
			       ) ORDER BY log.created_at,log.id) AS movements
			FROM reconciliation_logs log
			INNER JOIN selected s ON s.id=log.reconciliation_id
			LEFT JOIN branches branch ON branch.id=log.branch_id
			WHERE log.invoice_item_id IS NOT NULL AND log.movement_role<>''
			GROUP BY log.reconciliation_id,log.invoice_item_id
		)
		SELECT item.reconciliation_id::text,invoice.branch_id::text,branch.name,
		       item.invoice_id::text,item.invoice_item_id::text,invoice.original_invoice_number,
		       CASE WHEN current_invoice.deleted_at IS NULL THEN current_invoice.invoice_number ELSE NULL END,
		       item.product_name,item.quantity,item.original_unit_price,
		       price.new_unit_price,COALESCE(price.variance_amount,0),invoice.payment_method,
		       CASE WHEN current_invoice.deleted_at IS NOT NULL THEN 'hidden'
		            WHEN price.invoice_item_id IS NOT NULL THEN 'adjusted' ELSE 'active' END,
		       COALESCE(item.effective_stock_bucket,'none'),COALESCE(movement.movements,'[]'::jsonb)
		FROM reconciliation_item_snapshots item
		INNER JOIN reconciliation_invoice_snapshots invoice
		        ON invoice.reconciliation_id=item.reconciliation_id AND invoice.invoice_id=item.invoice_id
		INNER JOIN selected s ON s.id=item.reconciliation_id
		INNER JOIN invoices current_invoice ON current_invoice.id=item.invoice_id
		INNER JOIN branches branch ON branch.id=invoice.branch_id
		LEFT JOIN price_changes price
		       ON price.reconciliation_id=item.reconciliation_id AND price.invoice_item_id=item.invoice_item_id
		LEFT JOIN movement_details movement
		       ON movement.reconciliation_id=item.reconciliation_id AND movement.invoice_item_id=item.invoice_item_id
		WHERE $4::uuid IS NULL OR invoice.branch_id=$4::uuid
		ORDER BY branch.name,invoice.invoice_created_at,item.invoice_id,item.invoice_item_id
		LIMIT $5 OFFSET $6
	`, queryArgs...)
	if err != nil {
		return MonthEndReportResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item MonthEndReportRow
		var currentNumber sql.NullString
		var adjustedPrice sql.NullFloat64
		var movementsJSON []byte
		if err := rows.Scan(
			&item.ReconciliationID, &item.BranchID, &item.BranchName,
			&item.InvoiceID, &item.InvoiceItemID, &item.OriginalInvoiceNo,
			&currentNumber, &item.ProductName, &item.Quantity, &item.OriginalPrice,
			&adjustedPrice, &item.DiscountAmount, &item.PaymentMethod, &item.Status,
			&item.StockDeductionSource, &movementsJSON,
		); err != nil {
			return MonthEndReportResult{}, err
		}
		if currentNumber.Valid {
			item.CurrentInvoiceNo = &currentNumber.String
		}
		if adjustedPrice.Valid {
			item.AdjustedPrice = &adjustedPrice.Float64
		}
		item.Movements = []MonthEndReportMovement{}
		if err := json.Unmarshal(movementsJSON, &item.Movements); err != nil {
			return MonthEndReportResult{}, err
		}
		result.Rows = append(result.Rows, item)
	}
	if err := rows.Err(); err != nil {
		return MonthEndReportResult{}, err
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	result.Pagination = map[string]int{
		"page": page, "page_size": pageSize, "total": total, "total_pages": totalPages,
	}
	return result, nil
}

func (h *Handler) MonthEndReport(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	result, err := h.service.MonthEndReport(
		c.Request().Context(),
		platform.CurrentUser(c),
		c.QueryParam("reconciliation_id"),
		c.QueryParam("period"),
		c.QueryParam("date_from"),
		c.QueryParam("date_to"),
		c.QueryParam("branch_id"),
		page,
		pageSize,
	)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}
