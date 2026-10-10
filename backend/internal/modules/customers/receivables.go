package customers

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

// Receivables are the credit bills still owed. Nothing here is a separate
// ledger: a bill's balance is its total less the invoice_payments taken
// against it, the same rows every sales and money report already reads.

type ReceivableFilter struct {
	BranchID   string
	CustomerID string
	Status     string // "", "overdue", "current"
	Search     string
}

type ReceivePaymentInput struct {
	CustomerID    string   `json:"customer_id"`
	PaymentType   string   `json:"payment_type"`
	Amount        float64  `json:"amount"`
	ReferenceCode string   `json:"reference_code"`
	Note          string   `json:"note"`
	InvoiceIDs    []string `json:"invoice_ids"`
}

func agingBucket(daysOverdue int) string {
	switch {
	case daysOverdue <= 0:
		return "current"
	case daysOverdue <= 30:
		return "1_30"
	case daysOverdue <= 60:
		return "31_60"
	case daysOverdue <= 90:
		return "61_90"
	default:
		return "over_90"
	}
}

// receivableScope decides which branches' bills a caller may see. A branch
// till sees its own branch's bills, but once it is looking at one customer it
// sees all of that customer's bills: a member of the chain may come to any
// branch to settle up.
func receivableScope(user platform.AuthUser, filter ReceivableFilter) (string, error) {
	if !platform.IsGlobalScope(user) && strings.TrimSpace(filter.CustomerID) != "" {
		return "", nil
	}
	return platform.BranchFilter(user, filter.BranchID)
}

func (s *Service) ListReceivables(ctx context.Context, user platform.AuthUser, filter ReceivableFilter) (map[string]any, error) {
	branchID, err := receivableScope(user, filter)
	if err != nil {
		return nil, err
	}
	args := []any{}
	conditions := []string{"o.outstanding > 0"}
	if branchID != "" {
		args = append(args, branchID)
		conditions = append(conditions, fmt.Sprintf("o.branch_id = $%d", len(args)))
	}
	if customerID := strings.TrimSpace(filter.CustomerID); customerID != "" {
		args = append(args, customerID)
		conditions = append(conditions, fmt.Sprintf("o.customer_id = $%d", len(args)))
	}
	if search := strings.ToLower(strings.TrimSpace(filter.Search)); search != "" {
		args = append(args, "%"+search+"%")
		conditions = append(conditions, fmt.Sprintf("(LOWER(c.name) LIKE $%[1]d OR LOWER(o.invoice_number) LIKE $%[1]d OR c.phone LIKE $%[1]d OR LOWER(c.customer_code) LIKE $%[1]d)", len(args)))
	}
	switch filter.Status {
	case "overdue":
		conditions = append(conditions, "o.due_date < "+bangkokToday)
	case "current":
		conditions = append(conditions, "(o.due_date IS NULL OR o.due_date >= "+bangkokToday+")")
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH o AS (`+openBillsSQL+`)
		SELECT o.id, o.invoice_number, o.issued_at, o.due_date, o.total_amount, o.paid_amount, o.outstanding,
		       c.id, c.customer_code, c.name, c.phone, b.id, b.name,
		       COALESCE(`+bangkokToday+` - o.due_date, 0)
		FROM o
		INNER JOIN customers c ON c.id = o.customer_id
		INNER JOIN branches b ON b.id = o.branch_id
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY o.due_date NULLS LAST, o.issued_at, o.id
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	totals := map[string]float64{"current": 0, "1_30": 0, "31_60": 0, "61_90": 0, "over_90": 0}
	type customerAging struct {
		ID, Code, Name, Phone string
		Buckets               map[string]float64
		Total                 float64
		Bills                 int
		OldestOverdue         int
	}
	byCustomer := map[string]*customerAging{}
	var grandTotal, overdueTotal float64
	for rows.Next() {
		var invoiceID, number, customerID, code, name, phone, branchID, branchName string
		var issuedAt time.Time
		var dueDate sql.NullTime
		var total, paid, outstanding float64
		var daysOverdue int
		if err := rows.Scan(&invoiceID, &number, &issuedAt, &dueDate, &total, &paid, &outstanding,
			&customerID, &code, &name, &phone, &branchID, &branchName, &daysOverdue); err != nil {
			return nil, err
		}
		bucket := agingBucket(daysOverdue)
		item := map[string]any{
			"invoice_id": invoiceID, "invoice_number": number, "issued_at": issuedAt,
			"total_amount": total, "paid_amount": platform.Round2(paid), "outstanding": platform.Round2(outstanding),
			"customer_id": customerID, "customer_code": code, "customer_name": name, "customer_phone": phone,
			"branch_id": branchID, "branch_name": branchName, "days_overdue": int(math.Max(0, float64(daysOverdue))),
			"aging_bucket": bucket,
		}
		if dueDate.Valid {
			item["due_date"] = dueDate.Time.Format("2006-01-02")
		}
		items = append(items, item)
		totals[bucket] = platform.Round2(totals[bucket] + outstanding)
		grandTotal = platform.Round2(grandTotal + outstanding)
		if bucket != "current" {
			overdueTotal = platform.Round2(overdueTotal + outstanding)
		}
		aging := byCustomer[customerID]
		if aging == nil {
			aging = &customerAging{ID: customerID, Code: code, Name: name, Phone: phone, Buckets: map[string]float64{}}
			byCustomer[customerID] = aging
		}
		aging.Buckets[bucket] = platform.Round2(aging.Buckets[bucket] + outstanding)
		aging.Total = platform.Round2(aging.Total + outstanding)
		aging.Bills++
		if daysOverdue > aging.OldestOverdue {
			aging.OldestOverdue = daysOverdue
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	customers := make([]map[string]any, 0, len(byCustomer))
	for _, aging := range byCustomer {
		customers = append(customers, map[string]any{
			"customer_id": aging.ID, "customer_code": aging.Code, "customer_name": aging.Name,
			"customer_phone": aging.Phone, "buckets": aging.Buckets, "total": aging.Total,
			"bill_count": aging.Bills, "oldest_days_overdue": aging.OldestOverdue,
		})
	}
	sort.Slice(customers, func(i, j int) bool {
		return customers[i]["total"].(float64) > customers[j]["total"].(float64)
	})
	return map[string]any{
		"items": items, "customers": customers, "aging_totals": totals,
		"total_outstanding": grandTotal, "overdue_outstanding": overdueTotal,
	}, nil
}

// ReceivePayment takes money from a customer and settles their bills, oldest
// due first unless the cashier picked the bills. Each settled slice is an
// ordinary invoice_payments row, so the money lands in the same daily cash
// and transfer figures as a sale taken at the till.
func (s *Service) ReceivePayment(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input ReceivePaymentInput) (map[string]any, error) {
	if !platform.HasPermission(user, "payment.collect") {
		return nil, platform.NewError(http.StatusForbidden, "บัญชีนี้ไม่มีสิทธิ์รับชำระเงิน")
	}
	if input.PaymentType != "cash" && input.PaymentType != "bank_transfer" {
		return nil, platform.NewError(http.StatusBadRequest, "กรุณาเลือกรับชำระด้วยเงินสดหรือเงินโอน")
	}
	amountCents := int64(math.Round(input.Amount * 100))
	if amountCents <= 0 || math.Abs(input.Amount*100-float64(amountCents)) > 0.000001 {
		return nil, platform.NewError(http.StatusBadRequest, "ยอดรับชำระต้องมากกว่าศูนย์และมีทศนิยมไม่เกิน 2 ตำแหน่ง")
	}
	if strings.TrimSpace(input.CustomerID) == "" {
		return nil, platform.NewError(http.StatusBadRequest, "กรุณาเลือกลูกค้า")
	}
	receivedBranch := platform.OwnBranchID(user)
	chosen := map[string]bool{}
	for _, id := range input.InvoiceIDs {
		if id = strings.TrimSpace(id); id != "" {
			chosen[id] = true
		}
	}

	type allocation struct {
		InvoiceID, InvoiceNumber, BranchID string
		Amount, Remaining                  float64
		Status                             string
	}
	allocations := []allocation{}
	var customerName string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT name FROM customers WHERE id=$1 FOR UPDATE`, input.CustomerID).Scan(&customerName); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบลูกค้า")
			}
			return err
		}
		// Lock the open bills themselves, oldest due first, so two tills
		// taking money from the same customer cannot both settle one bill.
		rows, err := tx.QueryContext(ctx, `
			SELECT i.id, i.invoice_number, i.branch_id, i.total_amount,
			       i.total_amount - COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id = i.id), 0)
			FROM invoices i
			WHERE i.customer_id = $1 AND i.payment_status <> 'paid'
			  AND i.deleted_at IS NULL AND i.invoice_status = 'issued'
			ORDER BY i.due_date NULLS LAST, i.issued_at, i.id
			FOR UPDATE
		`, input.CustomerID)
		if err != nil {
			return err
		}
		type openBill struct {
			ID, Number, BranchID string
			Total, Outstanding   float64
		}
		bills := []openBill{}
		for rows.Next() {
			var bill openBill
			if err := rows.Scan(&bill.ID, &bill.Number, &bill.BranchID, &bill.Total, &bill.Outstanding); err != nil {
				rows.Close()
				return err
			}
			if len(chosen) > 0 && !chosen[bill.ID] {
				continue
			}
			if bill.Outstanding > 0 {
				bills = append(bills, bill)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(bills) == 0 {
			return platform.NewError(http.StatusBadRequest, "ไม่มีบิลค้างชำระที่เลือกไว้")
		}
		remainingCents := amountCents
		var owedCents int64
		for _, bill := range bills {
			owedCents += int64(math.Round(bill.Outstanding * 100))
		}
		if amountCents > owedCents {
			return platform.NewError(http.StatusBadRequest, fmt.Sprintf("ยอดรับชำระมากกว่ายอดค้าง %.2f บาท", float64(owedCents)/100))
		}
		for _, bill := range bills {
			if remainingCents <= 0 {
				break
			}
			billCents := int64(math.Round(bill.Outstanding * 100))
			take := billCents
			if take > remainingCents {
				take = remainingCents
			}
			remainingCents -= take
			status := "partial"
			if take == billCents {
				status = "paid"
			}
			// A head-office user takes the money in the bill's own branch's name.
			branch := receivedBranch
			if branch == "" {
				branch = bill.BranchID
			}
			reference := ""
			if input.PaymentType == "bank_transfer" {
				reference = strings.TrimSpace(input.ReferenceCode)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO invoice_payments (id, invoice_id, payment_type, amount, reference_code, notes, created_by, created_at, received_branch_id)
				VALUES ($1,$2,$3,$4,$5,$6,$7,NOW(),$8)
			`, platform.MustUUID(), bill.ID, input.PaymentType, float64(take)/100, reference,
				strings.TrimSpace(input.Note), user.ID, branch); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE invoices SET payment_status=$2, updated_at=NOW() WHERE id=$1`, bill.ID, status); err != nil {
				return err
			}
			allocations = append(allocations, allocation{
				InvoiceID: bill.ID, InvoiceNumber: bill.Number, BranchID: bill.BranchID,
				Amount: float64(take) / 100, Remaining: float64(billCents-take) / 100, Status: status,
			})
		}
		meta.EntityType = "customer"
		meta.EntityID = &input.CustomerID
		meta.Action = "receivable.payment"
		meta.After = map[string]any{"amount": float64(amountCents) / 100, "payment_type": input.PaymentType,
			"allocations": allocations}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(allocations))
	for _, a := range allocations {
		result = append(result, map[string]any{"invoice_id": a.InvoiceID, "invoice_number": a.InvoiceNumber,
			"amount": a.Amount, "remaining": a.Remaining, "payment_status": a.Status})
	}
	return map[string]any{"customer_name": customerName, "amount": float64(amountCents) / 100,
		"payment_type": input.PaymentType, "allocations": result}, nil
}

func (h *Handler) ListReceivables(c echo.Context) error {
	result, err := h.service.ListReceivables(c.Request().Context(), platform.CurrentUser(c), ReceivableFilter{
		BranchID: c.QueryParam("branch_id"), CustomerID: c.QueryParam("customer_id"),
		Status: c.QueryParam("status"), Search: c.QueryParam("search"),
	})
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดลูกหนี้ค้างชำระไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) ReceivePayment(c echo.Context) error {
	var input ReceivePaymentInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	result, err := h.service.ReceivePayment(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	result["message"] = "รับชำระหนี้แล้ว"
	return platform.JSON(c, http.StatusCreated, result)
}
