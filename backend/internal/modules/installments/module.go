package installments

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type CreatePlanRequest struct {
	InvoiceID string `json:"invoice_id"`
	Months    int    `json:"months"`
	StartDate string `json:"start_date"`
}

type RecordPaymentRequest struct {
	Amount        float64 `json:"amount"`
	PaymentType   string  `json:"payment_type"`
	ReferenceCode string  `json:"reference_code"`
	Notes         string  `json:"notes"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) CreatePlan(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input CreatePlanRequest) (string, error) {
	if strings.TrimSpace(input.InvoiceID) == "" {
		return "", platform.NewError(http.StatusBadRequest, "invoice_id is required")
	}
	if input.Months <= 0 || input.Months > 60 {
		return "", platform.NewError(http.StatusBadRequest, "months must be between 1 and 60")
	}

	firstDue := time.Now().UTC().AddDate(0, 1, 0)
	if strings.TrimSpace(input.StartDate) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(input.StartDate))
		if err != nil {
			return "", platform.NewError(http.StatusBadRequest, "start_date must be in YYYY-MM-DD format")
		}
		firstDue = parsed
	}

	var planID string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var branchID, paymentStatus, invoiceStatus string
		var totalAmount float64
		if err := tx.QueryRowContext(ctx, `
			SELECT branch_id::text, total_amount, payment_status, invoice_status
			FROM invoices
			WHERE id = $1
			FOR UPDATE
		`, input.InvoiceID).Scan(&branchID, &totalAmount, &paymentStatus, &invoiceStatus); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "invoice not found")
			}
			return err
		}
		if err := validateBranchScope(user, branchID); err != nil {
			return err
		}
		if invoiceStatus != "issued" {
			return platform.NewError(http.StatusConflict, "invoice is not in issued status")
		}
		if paymentStatus != "unpaid" {
			return platform.NewError(http.StatusConflict, "invoice already has payments or an installment plan")
		}

		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM installment_plans WHERE invoice_id = $1)`, input.InvoiceID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return platform.NewError(http.StatusConflict, "installment plan already exists for this invoice")
		}

		monthly, lastAmount := splitInstallments(totalAmount, input.Months)
		if monthly <= 0 || lastAmount <= 0 {
			return platform.NewError(http.StatusBadRequest, "invoice total is too small for this number of months")
		}

		planID = platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO installment_plans (id, invoice_id, branch_id, months, monthly_amount, total_amount, status, created_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, NOW(), NOW())
		`, planID, input.InvoiceID, branchID, input.Months, monthly, totalAmount, user.ID); err != nil {
			return err
		}

		for seq := 1; seq <= input.Months; seq++ {
			amount := monthly
			if seq == input.Months {
				amount = lastAmount
			}
			dueDate := firstDue.AddDate(0, seq-1, 0)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO installment_payments (id, plan_id, seq_number, due_date, amount, paid_amount, status, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, 0, 'pending', NOW(), NOW())
			`, platform.MustUUID(), planID, seq, dueDate.Format("2006-01-02"), amount); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE invoices
			SET payment_status = 'installment', updated_at = NOW()
			WHERE id = $1
		`, input.InvoiceID); err != nil {
			return err
		}

		meta.EntityType = "installment_plan"
		meta.EntityID = &planID
		meta.Action = "installment.plan_create"
		meta.After = map[string]any{"invoice_id": input.InvoiceID, "months": input.Months, "monthly_amount": monthly, "total_amount": totalAmount}
		return s.audit.Log(ctx, tx, meta)
	})
	return planID, err
}

func (s *Service) RecordPayment(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, paymentID string, input RecordPaymentRequest) error {
	if input.PaymentType != "cash" && input.PaymentType != "bank_transfer" {
		return platform.NewError(http.StatusBadRequest, "payment_type must be cash or bank_transfer")
	}
	if input.Amount <= 0 {
		return platform.NewError(http.StatusBadRequest, "amount must be greater than zero")
	}
	amount := platform.Round2(input.Amount)

	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var planID, invoiceID, branchID, planStatus, paymentStatus string
		var seqNumber int
		var dueAmount, paidAmount float64
		if err := tx.QueryRowContext(ctx, `
			SELECT ip.plan_id::text, ip.seq_number, ip.amount, ip.paid_amount, ip.status,
			       pl.invoice_id::text, pl.branch_id::text, pl.status
			FROM installment_payments ip
			INNER JOIN installment_plans pl ON pl.id = ip.plan_id
			WHERE ip.id = $1
			FOR UPDATE OF ip, pl
		`, paymentID).Scan(&planID, &seqNumber, &dueAmount, &paidAmount, &paymentStatus, &invoiceID, &branchID, &planStatus); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "installment payment not found")
			}
			return err
		}
		if err := validateBranchScope(user, branchID); err != nil {
			return err
		}
		if planStatus != "active" {
			return platform.NewError(http.StatusConflict, "installment plan is not active")
		}
		if paymentStatus == "paid" {
			return platform.NewError(http.StatusConflict, "installment is already paid")
		}
		remaining := platform.Round2(dueAmount - paidAmount)
		if amount > remaining {
			return platform.NewError(http.StatusBadRequest, "amount exceeds the remaining balance of this installment")
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invoice_payments (id, invoice_id, payment_type, amount, reference_code, notes, created_by, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		`, platform.MustUUID(), invoiceID, input.PaymentType, amount, strings.TrimSpace(input.ReferenceCode), strings.TrimSpace(input.Notes), user.ID); err != nil {
			return err
		}

		newPaid := platform.Round2(paidAmount + amount)
		fullyPaid := newPaid >= dueAmount
		if fullyPaid {
			if _, err := tx.ExecContext(ctx, `
				UPDATE installment_payments
				SET paid_amount = $2, status = 'paid', paid_at = NOW(), received_by = $3, updated_at = NOW()
				WHERE id = $1
			`, paymentID, newPaid, user.ID); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `
				UPDATE installment_payments
				SET paid_amount = $2, received_by = $3, updated_at = NOW()
				WHERE id = $1
			`, paymentID, newPaid, user.ID); err != nil {
				return err
			}
		}

		var pendingCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM installment_payments WHERE plan_id = $1 AND status = 'pending'
		`, planID).Scan(&pendingCount); err != nil {
			return err
		}
		if pendingCount == 0 {
			if _, err := tx.ExecContext(ctx, `
				UPDATE installment_plans SET status = 'completed', updated_at = NOW() WHERE id = $1
			`, planID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE invoices SET payment_status = 'paid', updated_at = NOW() WHERE id = $1
			`, invoiceID); err != nil {
				return err
			}
		}

		meta.EntityType = "installment_payment"
		meta.EntityID = &paymentID
		meta.Action = "installment.collect_payment"
		meta.Before = map[string]any{"paid_amount": paidAmount, "status": paymentStatus}
		meta.After = map[string]any{"seq_number": seqNumber, "amount": amount, "paid_amount": newPaid, "payment_type": input.PaymentType, "plan_completed": pendingCount == 0}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) List(ctx context.Context, user platform.AuthUser, branchID string, status string) (map[string]any, error) {
	if branchID == "" && user.BranchID != nil {
		branchID = *user.BranchID
	}
	if err := validateBranchScope(user, branchID); err != nil {
		return nil, err
	}

	query := `
		SELECT pl.id, pl.invoice_id::text, i.invoice_number, i.customer_name, pl.branch_id::text, b.name,
		       pl.months, pl.monthly_amount, pl.total_amount, pl.status, pl.created_at,
		       ip.id, ip.seq_number, ip.due_date, ip.amount, ip.paid_amount, ip.status, ip.paid_at
		FROM installment_plans pl
		INNER JOIN invoices i ON i.id = pl.invoice_id
		INNER JOIN branches b ON b.id = pl.branch_id
		INNER JOIN installment_payments ip ON ip.plan_id = pl.id
	`
	args := []any{}
	conditions := []string{}
	if branchID != "" {
		args = append(args, branchID)
		conditions = append(conditions, fmt.Sprintf("pl.branch_id = $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("pl.status = $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY pl.created_at DESC, ip.seq_number ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	today := time.Now().UTC().Format("2006-01-02")
	plans := []map[string]any{}
	planIndex := map[string]int{}
	var outstandingTotal float64
	overdueCount := 0

	for rows.Next() {
		var planID, invoiceID, invoiceNumber, customerName, planBranchID, branchName, planStatus string
		var months, seqNumber int
		var monthlyAmount, totalAmount, dueAmount, paidAmount float64
		var createdAt, dueDate time.Time
		var paymentID, paymentStatus string
		var paidAt sql.NullTime
		if err := rows.Scan(
			&planID, &invoiceID, &invoiceNumber, &customerName, &planBranchID, &branchName,
			&months, &monthlyAmount, &totalAmount, &planStatus, &createdAt,
			&paymentID, &seqNumber, &dueDate, &dueAmount, &paidAmount, &paymentStatus, &paidAt,
		); err != nil {
			return nil, err
		}

		index, ok := planIndex[planID]
		if !ok {
			plans = append(plans, map[string]any{
				"id":             planID,
				"invoice_id":     invoiceID,
				"invoice_number": invoiceNumber,
				"customer_name":  customerName,
				"branch_id":      planBranchID,
				"branch_name":    branchName,
				"months":         months,
				"monthly_amount": monthlyAmount,
				"total_amount":   totalAmount,
				"status":         planStatus,
				"created_at":     createdAt,
				"paid_total":     0.0,
				"outstanding":    0.0,
				"payments":       []map[string]any{},
			})
			index = len(plans) - 1
			planIndex[planID] = index
		}

		displayStatus := paymentStatus
		dueDateText := dueDate.Format("2006-01-02")
		if paymentStatus == "pending" && dueDateText < today {
			displayStatus = "overdue"
			if planStatus == "active" {
				overdueCount++
			}
		}
		remaining := platform.Round2(dueAmount - paidAmount)
		if paymentStatus == "pending" && planStatus == "active" {
			outstandingTotal = platform.Round2(outstandingTotal + remaining)
		}

		payment := map[string]any{
			"id":          paymentID,
			"seq_number":  seqNumber,
			"due_date":    dueDateText,
			"amount":      dueAmount,
			"paid_amount": paidAmount,
			"remaining":   remaining,
			"status":      displayStatus,
		}
		if paidAt.Valid {
			payment["paid_at"] = paidAt.Time
		}

		plan := plans[index]
		plan["payments"] = append(plan["payments"].([]map[string]any), payment)
		plan["paid_total"] = platform.Round2(plan["paid_total"].(float64) + paidAmount)
		plan["outstanding"] = platform.Round2(plan["outstanding"].(float64) + remaining)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return map[string]any{
		"items": plans,
		"summary": map[string]any{
			"plan_count":        len(plans),
			"outstanding_total": outstandingTotal,
			"overdue_count":     overdueCount,
		},
	}, nil
}

// splitInstallments divides a total into equal monthly amounts; the final
// installment absorbs the rounding difference so the sum matches the total.
func splitInstallments(totalAmount float64, months int) (float64, float64) {
	monthly := platform.Round2(totalAmount / float64(months))
	last := platform.Round2(totalAmount - monthly*float64(months-1))
	return monthly, last
}

func validateBranchScope(user platform.AuthUser, branchID string) error {
	if branchID == "" {
		return nil
	}
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

func (h *Handler) List(c echo.Context) error {
	result, err := h.service.List(c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("branch_id")), strings.TrimSpace(c.QueryParam("status")))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load installment plans", err))
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) CreatePlan(c echo.Context) error {
	var input CreatePlanRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	planID, err := h.service.CreatePlan(c.Request().Context(), platform.CurrentUser(c), meta, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": planID, "message": "installment plan created"})
}

func (h *Handler) RecordPayment(c echo.Context) error {
	var input RecordPaymentRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.RecordPayment(c.Request().Context(), platform.CurrentUser(c), meta, c.Param("paymentID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "installment payment recorded")
}
