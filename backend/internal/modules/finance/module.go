package finance

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type CheckPreviewRequest struct {
	BranchID    string   `json:"branch_id"`
	CheckNumber string   `json:"check_number"`
	BankName    string   `json:"bank_name"`
	PayerName   string   `json:"payer_name"`
	Amount      float64  `json:"amount"`
	InvoiceIDs  []string `json:"invoice_ids"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) ListChecks(ctx context.Context, user platform.AuthUser) ([]map[string]any, error) {
	query := `
		SELECT c.id, c.check_number, c.bank_name, c.payer_name, c.amount, c.status, b.name, c.received_date
		FROM checks c
		INNER JOIN branches b ON b.id = c.branch_id
	`
	args := []any{}
	if user.BranchID != nil && user.RoleKey != "super_admin" {
		args = append(args, *user.BranchID)
		query += " WHERE c.branch_id = $1"
	}
	query += " ORDER BY c.created_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, checkNumber, bankName, payerName, status, branchName string
		var amount float64
		var receivedDate time.Time
		if err := rows.Scan(&id, &checkNumber, &bankName, &payerName, &amount, &status, &branchName, &receivedDate); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":            id,
			"check_number":  checkNumber,
			"bank_name":     bankName,
			"payer_name":    payerName,
			"amount":        amount,
			"status":        status,
			"branch_name":   branchName,
			"received_date": receivedDate,
		})
	}
	return items, rows.Err()
}

func (s *Service) ListOutstandingInvoices(ctx context.Context, user platform.AuthUser) ([]map[string]any, error) {
	query := `
		SELECT i.id, i.invoice_number, i.customer_name, i.total_amount, b.name
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
		WHERE i.payment_status = 'unpaid'
	`
	args := []any{}
	if user.BranchID != nil && user.RoleKey != "super_admin" {
		args = append(args, *user.BranchID)
		query += " AND i.branch_id = $1"
	}
	query += " ORDER BY i.issued_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, invoiceNumber, customerName, branchName string
		var totalAmount float64
		if err := rows.Scan(&id, &invoiceNumber, &customerName, &totalAmount, &branchName); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":             id,
			"invoice_number": invoiceNumber,
			"customer_name":  customerName,
			"total_amount":   totalAmount,
			"branch_name":    branchName,
		})
	}
	return items, rows.Err()
}

func (s *Service) PreviewApply(ctx context.Context, user platform.AuthUser, input CheckPreviewRequest) (map[string]any, error) {
	if err := validateBranch(user, input.BranchID); err != nil {
		return nil, err
	}
	invoices, totalInvoices, err := s.loadInvoices(ctx, s.db, input.BranchID, input.InvoiceIDs)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"check_amount":   platform.Round2(input.Amount),
		"invoices_total": totalInvoices,
		"is_match":       platform.Round2(input.Amount) == totalInvoices,
		"invoices":       invoices,
	}, nil
}

func (s *Service) CreateCheck(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input CheckPreviewRequest) (string, error) {
	if err := validateBranch(user, input.BranchID); err != nil {
		return "", err
	}
	var checkID string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		invoices, totalInvoices, err := s.loadInvoices(ctx, tx, input.BranchID, input.InvoiceIDs)
		if err != nil {
			return err
		}
		status := "pending"
		if len(invoices) > 0 {
			if !checkAmountMatches(platform.Round2(input.Amount), totalInvoices) {
				return platform.NewError(http.StatusConflict, "selected invoices must match the check amount exactly")
			}
			status = "applied"
		}

		checkID = platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO checks (id, branch_id, check_number, bank_name, payer_name, amount, status, received_date, created_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_DATE, $8, NOW(), NOW())
		`, checkID, input.BranchID, strings.TrimSpace(input.CheckNumber), strings.TrimSpace(input.BankName), strings.TrimSpace(input.PayerName), platform.Round2(input.Amount), status, user.ID); err != nil {
			return err
		}

		for _, invoice := range invoices {
			invoiceID := invoice["id"].(string)
			invoiceAmount := invoice["total_amount"].(float64)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO payment_invoice_map (id, check_id, invoice_id, applied_amount, created_at)
				VALUES ($1, $2, $3, $4, NOW())
			`, platform.MustUUID(), checkID, invoiceID, invoiceAmount); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO invoice_payments (id, invoice_id, payment_type, amount, reference_code, notes, created_by, created_at)
				VALUES ($1, $2, 'check', $3, $4, 'Matched by check reconciliation', $5, NOW())
			`, platform.MustUUID(), invoiceID, invoiceAmount, input.CheckNumber, user.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE invoices
				SET payment_status = 'paid', updated_at = NOW()
				WHERE id = $1
			`, invoiceID); err != nil {
				return err
			}
		}

		meta.EntityType = "check"
		meta.EntityID = &checkID
		meta.Action = "check.create"
		meta.After = map[string]any{"check_number": input.CheckNumber, "status": status, "invoice_count": len(invoices)}
		return s.audit.Log(ctx, tx, meta)
	})
	return checkID, err
}

func checkAmountMatches(checkAmount float64, invoicesTotal float64) bool {
	return platform.Round2(checkAmount) == platform.Round2(invoicesTotal)
}

func (s *Service) loadInvoices(ctx context.Context, db platform.DBTX, branchID string, invoiceIDs []string) ([]map[string]any, float64, error) {
	items := []map[string]any{}
	var total float64
	for _, invoiceID := range invoiceIDs {
		var id, invoiceNumber, customerName string
		var invoiceBranchID string
		var totalAmount float64
		var paymentStatus string
		if err := db.QueryRowContext(ctx, `
			SELECT id::text, branch_id::text, invoice_number, customer_name, total_amount, payment_status
			FROM invoices
			WHERE id = $1
			FOR UPDATE
		`, invoiceID).Scan(&id, &invoiceBranchID, &invoiceNumber, &customerName, &totalAmount, &paymentStatus); err != nil {
			if err == sql.ErrNoRows {
				return nil, 0, platform.NewError(http.StatusNotFound, "invoice not found")
			}
			return nil, 0, err
		}
		if invoiceBranchID != branchID {
			return nil, 0, platform.NewError(http.StatusForbidden, "invoice branch mismatch")
		}
		if paymentStatus != "unpaid" {
			return nil, 0, platform.NewError(http.StatusConflict, "invoice is already settled")
		}
		total = platform.Round2(total + totalAmount)
		items = append(items, map[string]any{
			"id":             id,
			"invoice_number": invoiceNumber,
			"customer_name":  customerName,
			"total_amount":   totalAmount,
		})
	}
	return items, total, nil
}

func validateBranch(user platform.AuthUser, branchID string) error {
	if user.BranchID != nil && user.RoleKey != "super_admin" && *user.BranchID != branchID {
		return platform.NewError(http.StatusForbidden, "branch scope mismatch")
	}
	return nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListChecks(c echo.Context) error {
	items, err := h.service.ListChecks(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load checks", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ListOutstandingInvoices(c echo.Context) error {
	items, err := h.service.ListOutstandingInvoices(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load outstanding invoices", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) PreviewApply(c echo.Context) error {
	var input CheckPreviewRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	result, err := h.service.PreviewApply(c.Request().Context(), user, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) CreateCheck(c echo.Context) error {
	var input CheckPreviewRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	meta := audit.MetaFromContext(c)
	id, err := h.service.CreateCheck(c.Request().Context(), user, meta, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "check saved"})
}
