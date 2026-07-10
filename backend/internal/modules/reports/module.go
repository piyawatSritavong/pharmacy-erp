package reports

import (
	"context"
	"database/sql"
	"net/http"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Tax(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.name, COUNT(i.id), COALESCE(SUM(i.subtotal), 0), COALESCE(SUM(i.tax_amount), 0), COALESCE(SUM(i.total_amount), 0)
		FROM branches b
		LEFT JOIN invoices i ON i.branch_id = b.id AND i.invoice_status = 'issued'
		GROUP BY b.name
		ORDER BY b.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var branchName string
		var invoiceCount int
		var subtotal, taxAmount, totalAmount float64
		if err := rows.Scan(&branchName, &invoiceCount, &subtotal, &taxAmount, &totalAmount); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"branch_name":   branchName,
			"invoice_count": invoiceCount,
			"subtotal":      subtotal,
			"tax_amount":    taxAmount,
			"total_amount":  totalAmount,
		})
	}
	return items, rows.Err()
}

func (s *Service) ProfitLoss(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.name, COALESCE(SUM(ii.line_subtotal), 0), COALESCE(SUM(ii.cost_snapshot * ii.quantity), 0), COALESCE(SUM(ii.line_subtotal - (ii.cost_snapshot * ii.quantity)), 0)
		FROM branches b
		LEFT JOIN invoices i ON i.branch_id = b.id AND i.invoice_status = 'issued'
		LEFT JOIN invoice_items ii ON ii.invoice_id = i.id
		GROUP BY b.name
		ORDER BY b.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var branchName string
		var revenue, cost, profit float64
		if err := rows.Scan(&branchName, &revenue, &cost, &profit); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"branch_name": branchName,
			"revenue":     revenue,
			"cost":        cost,
			"profit":      profit,
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

func (h *Handler) Tax(c echo.Context) error {
	items, err := h.service.Tax(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load tax report", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ProfitLoss(c echo.Context) error {
	items, err := h.service.ProfitLoss(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load profit/loss report", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}
