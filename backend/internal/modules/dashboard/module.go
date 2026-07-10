package dashboard

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Summary(ctx context.Context, user platform.AuthUser) (map[string]any, error) {
	switch user.RoleKey {
	case "super_admin":
		return s.globalSummary(ctx)
	case "branch_admin":
		return s.branchSummary(ctx, user)
	case "branch_pos":
		return s.DailySales(ctx, user, "")
	default:
		return s.globalSummary(ctx)
	}
}

func (s *Service) globalSummary(ctx context.Context) (map[string]any, error) {
	var totalSales, unpaidTotal float64
	var unpaidCount, transferCount, marketplaceCount int
	var qtyReal, qtyGhost int64

	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(total_amount), 0)
		FROM invoices
		WHERE invoice_status = 'issued'
	`).Scan(&totalSales); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(total_amount), 0)
		FROM invoices
		WHERE payment_status = 'unpaid' AND invoice_status = 'issued'
	`).Scan(&unpaidCount, &unpaidTotal); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transfers WHERE status <> 'completed'`).Scan(&transferCount); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM marketplace_orders WHERE status <> 'completed'`).Scan(&marketplaceCount); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(qty_real), 0), COALESCE(SUM(qty_ghost), 0) FROM inventory`).Scan(&qtyReal, &qtyGhost); err != nil {
		return nil, err
	}

	recentInvoices, err := s.listRecentInvoices(ctx, "global", "", "")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"scope": "global",
		"metrics": []map[string]any{
			{"key": "sales_total", "label": "ยอดขายรวม", "value": totalSales},
			{"key": "unpaid_count", "label": "บิลค้างชำระ", "value": unpaidCount},
			{"key": "unpaid_total", "label": "ยอดค้างชำระ", "value": unpaidTotal},
			{"key": "qty_real", "label": "สต็อกจริงรวม", "value": qtyReal},
			{"key": "qty_ghost", "label": "สต็อกผีรวม", "value": qtyGhost},
			{"key": "transfers", "label": "รายการโอนค้าง", "value": transferCount},
			{"key": "marketplace", "label": "Marketplace Queue", "value": marketplaceCount},
		},
		"recent_invoices": recentInvoices,
		"recent_items": map[string]any{
			"invoices": recentInvoices,
		},
	}, nil
}

func (s *Service) branchSummary(ctx context.Context, user platform.AuthUser) (map[string]any, error) {
	if user.BranchID == nil {
		return nil, platform.NewError(http.StatusForbidden, "branch scope is required")
	}

	var totalSales, unpaidTotal float64
	var unpaidCount, transferCount, marketplaceCount int
	var qtyReal, qtyGhost int64
	branchID := *user.BranchID

	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(total_amount), 0)
		FROM invoices
		WHERE invoice_status = 'issued' AND branch_id = $1
	`, branchID).Scan(&totalSales); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(total_amount), 0)
		FROM invoices
		WHERE payment_status = 'unpaid' AND invoice_status = 'issued' AND branch_id = $1
	`, branchID).Scan(&unpaidCount, &unpaidTotal); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM transfers
		WHERE status <> 'completed' AND (source_branch_id = $1 OR destination_branch_id = $1)
	`, branchID).Scan(&transferCount); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM marketplace_orders
		WHERE status <> 'completed' AND branch_id = $1
	`, branchID).Scan(&marketplaceCount); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(qty_real), 0), COALESCE(SUM(qty_ghost), 0)
		FROM inventory
		WHERE branch_id = $1
	`, branchID).Scan(&qtyReal, &qtyGhost); err != nil {
		return nil, err
	}

	recentInvoices, err := s.listRecentInvoices(ctx, "branch", branchID, "")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"scope": "branch",
		"metrics": []map[string]any{
			{"key": "sales_total", "label": "ยอดขายสาขา", "value": totalSales},
			{"key": "unpaid_count", "label": "บิลค้างชำระ", "value": unpaidCount},
			{"key": "unpaid_total", "label": "ยอดค้างชำระ", "value": unpaidTotal},
			{"key": "qty_real", "label": "สต็อกจริง", "value": qtyReal},
			{"key": "qty_ghost", "label": "สต็อกผี", "value": qtyGhost},
			{"key": "transfers", "label": "รายการโอนค้าง", "value": transferCount},
			{"key": "marketplace", "label": "Marketplace Queue", "value": marketplaceCount},
		},
		"recent_invoices": recentInvoices,
		"recent_items": map[string]any{
			"invoices": recentInvoices,
		},
	}, nil
}

func (s *Service) DailySales(ctx context.Context, user platform.AuthUser, dateInput string) (map[string]any, error) {
	start, end, label, err := bangkokDayRange(dateInput)
	if err != nil {
		return nil, err
	}

	var totalSales float64
	var invoiceCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(total_amount), 0)
		FROM invoices
		WHERE created_by = $1 AND invoice_status = 'issued' AND issued_at >= $2 AND issued_at < $3
	`, user.ID, start, end).Scan(&invoiceCount, &totalSales); err != nil {
		return nil, err
	}

	var cashReceived, bankReceived float64
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN payment_type = 'cash' THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN payment_type = 'bank_transfer' THEN amount ELSE 0 END), 0)
		FROM invoice_payments
		WHERE created_by = $1 AND created_at >= $2 AND created_at < $3
	`, user.ID, start, end).Scan(&cashReceived, &bankReceived); err != nil {
		return nil, err
	}

	recentInvoices, err := s.listRecentInvoices(ctx, "self_daily", "", user.ID, start, end)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"scope": "self_daily",
		"date":  label,
		"metrics": []map[string]any{
			{"key": "sales_total", "label": "ยอดขายวันนี้", "value": totalSales},
			{"key": "invoice_count", "label": "จำนวนบิลวันนี้", "value": invoiceCount},
			{"key": "cash_received", "label": "รับเงินสดวันนี้", "value": cashReceived},
			{"key": "bank_received", "label": "รับโอนวันนี้", "value": bankReceived},
		},
		"recent_invoices": recentInvoices,
		"recent_items": map[string]any{
			"invoices": recentInvoices,
		},
	}, nil
}

func bangkokDayRange(dateInput string) (time.Time, time.Time, string, error) {
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}

	day := time.Now().In(location)
	if trimmed := strings.TrimSpace(dateInput); trimmed != "" {
		parsed, err := time.ParseInLocation("2006-01-02", trimmed, location)
		if err != nil {
			return time.Time{}, time.Time{}, "", platform.NewError(http.StatusBadRequest, "date must be YYYY-MM-DD")
		}
		day = parsed
	}

	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	end := start.Add(24 * time.Hour)
	return start.UTC(), end.UTC(), start.Format("2006-01-02"), nil
}

func (s *Service) listRecentInvoices(ctx context.Context, scope string, branchID string, userID string, ranges ...time.Time) ([]map[string]any, error) {
	query := `
		SELECT i.id, i.invoice_number, i.customer_name, i.total_amount, i.payment_status, b.name, i.issued_at
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
		WHERE i.invoice_status = 'issued'
	`
	args := []any{}

	if scope == "branch" {
		args = append(args, branchID)
		query += " AND i.branch_id = $1"
	}
	if scope == "self_daily" {
		args = append(args, userID)
		query += " AND i.created_by = $" + strconvI(len(args))
		if len(ranges) == 2 {
			args = append(args, ranges[0], ranges[1])
			query += " AND i.issued_at >= $" + strconvI(len(args)-1) + " AND i.issued_at < $" + strconvI(len(args))
		}
	}

	query += " ORDER BY i.issued_at DESC LIMIT 8"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, invoiceNumber, customerName, paymentStatus, branchName string
		var totalAmount float64
		var issuedAt time.Time
		if err := rows.Scan(&id, &invoiceNumber, &customerName, &totalAmount, &paymentStatus, &branchName, &issuedAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":             id,
			"invoice_number": invoiceNumber,
			"customer_name":  customerName,
			"total_amount":   totalAmount,
			"payment_status": paymentStatus,
			"branch_name":    branchName,
			"issued_at":      issuedAt,
		})
	}
	return items, rows.Err()
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Summary(c echo.Context) error {
	data, err := h.service.Summary(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, data)
}

func (h *Handler) DailySales(c echo.Context) error {
	data, err := h.service.DailySales(c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("date")))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, data)
}

func strconvI(value int) string {
	return strconv.Itoa(value)
}
