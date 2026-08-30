// Package fda implements Part B Rule 1 — the อย. (Thai FDA) submission
// report: pick products, see their movement over a date range, print a
// formal document. It's read-only (no state to manage beyond the products
// table's own requires_fda_report/fda_registration_no flags, which the
// products module already handles) — one endpoint drives both the live
// preview and the final print, exactly like the PO print flow (D4) reads
// off already-fetched detail data rather than a separate "generate" call.
package fda

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

type ReportFilter struct {
	DateFrom   string
	DateTo     string
	BranchID   string
	CategoryID string
	ProductIDs []string
}

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// Summary returns the company/license header plus one line per matching
// product with its อย. registration number and movement in the window. When
// ProductIDs is empty, it defaults to every requires_fda_report=true product
// (optionally narrowed by category) — the "select products" step the master
// prompt describes is choosing which of these to keep before printing, done
// client-side against this same response.
func (s *Service) Summary(ctx context.Context, filter ReportFilter) (map[string]any, error) {
	dateFrom, dateTo, err := parseRange(filter.DateFrom, filter.DateTo)
	if err != nil {
		return nil, err
	}

	companyName, _ := platform.GetSettingString(ctx, s.db, "company_name", "")
	companyAddress, _ := platform.GetSettingString(ctx, s.db, "company_address", "")
	companyTaxID, _ := platform.GetSettingString(ctx, s.db, "company_tax_id", "")
	licenseNo, _ := platform.GetSettingString(ctx, s.db, "fda_license_no", "")

	args := []any{dateFrom, dateTo}
	conditions := []string{}
	if len(filter.ProductIDs) > 0 {
		args = append(args, pq.Array(filter.ProductIDs))
		conditions = append(conditions, "p.id = ANY($3)")
	} else {
		conditions = append(conditions, "p.requires_fda_report = TRUE")
		if strings.TrimSpace(filter.CategoryID) != "" {
			args = append(args, strings.TrimSpace(filter.CategoryID))
			conditions = append(conditions, "p.category_id = $"+strconv.Itoa(len(args)))
		}
	}
	branchFilterSold := ""
	branchFilterReceived := ""
	if strings.TrimSpace(filter.BranchID) != "" {
		args = append(args, strings.TrimSpace(filter.BranchID))
		branchIdx := "$" + strconv.Itoa(len(args))
		branchFilterSold = " AND i.branch_id = " + branchIdx
		branchFilterReceived = " AND m.branch_id = " + branchIdx
	}

	query := `
		SELECT p.id, p.sku, p.name, COALESCE(p.fda_registration_no, ''),
		       COALESCE((
		           SELECT SUM(ii.quantity) FROM invoice_items ii
		           INNER JOIN invoices i ON i.id = ii.invoice_id AND i.deleted_at IS NULL
		           WHERE ii.product_id = p.id AND i.invoice_status = 'issued'
		             AND i.issued_at >= $1 AND i.issued_at < $2` + branchFilterSold + `
		       ), 0) AS qty_sold,
		       COALESCE((
		           SELECT SUM(m.quantity_delta) FROM inventory_movements m
		           WHERE m.product_id = p.id AND m.movement_type = 'receive'
		             AND m.created_at >= $1 AND m.created_at < $2` + branchFilterReceived + `
		       ), 0) AS qty_received
		FROM products p
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY p.name
	`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, sku, name, registrationNo string
		var qtySold, qtyReceived int
		if err := rows.Scan(&id, &sku, &name, &registrationNo, &qtySold, &qtyReceived); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"product_id":          id,
			"sku":                 sku,
			"name":                name,
			"fda_registration_no": registrationNo,
			"quantity_sold":       qtySold,
			"quantity_received":   qtyReceived,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return map[string]any{
		"company_name":    companyName,
		"company_address": companyAddress,
		"company_tax_id":  companyTaxID,
		"fda_license_no":  licenseNo,
		"date_from":       dateFrom.Format("2006-01-02"),
		"date_to":         dateTo.AddDate(0, 0, -1).Format("2006-01-02"),
		"items":           items,
	}, nil
}

func parseRange(fromInput, toInput string) (time.Time, time.Time, error) {
	location := time.FixedZone("Asia/Bangkok", 7*60*60)
	today := time.Now().In(location).Format("2006-01-02")
	if strings.TrimSpace(fromInput) == "" {
		fromInput = today
	}
	if strings.TrimSpace(toInput) == "" {
		toInput = fromInput
	}
	from, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(fromInput), location)
	if err != nil {
		return time.Time{}, time.Time{}, platform.NewError(http.StatusBadRequest, "วันที่เริ่มต้นต้องอยู่ในรูปแบบ YYYY-MM-DD")
	}
	to, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(toInput), location)
	if err != nil {
		return time.Time{}, time.Time{}, platform.NewError(http.StatusBadRequest, "วันที่สิ้นสุดต้องอยู่ในรูปแบบ YYYY-MM-DD")
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, platform.NewError(http.StatusBadRequest, "วันที่สิ้นสุดต้องไม่น้อยกว่าวันที่เริ่มต้น")
	}
	return from.UTC(), to.AddDate(0, 0, 1).UTC(), nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Summary(c echo.Context) error {
	var productIDs []string
	if raw := c.QueryParams()["product_id"]; len(raw) > 0 {
		productIDs = raw
	}
	filter := ReportFilter{
		DateFrom:   c.QueryParam("date_from"),
		DateTo:     c.QueryParam("date_to"),
		BranchID:   c.QueryParam("branch_id"),
		CategoryID: c.QueryParam("category_id"),
		ProductIDs: productIDs,
	}
	result, err := h.service.Summary(c.Request().Context(), filter)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}
