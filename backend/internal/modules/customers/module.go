// Package customers keeps the chain's customer register: members who collect
// and spend loyalty points, wholesale buyers who get tier prices, and accounts
// allowed to buy on credit. Points are earned and spent inside the sales
// checkout transaction; this package owns the register, the points ledger
// outside a sale (manual adjustments) and the receivables built on credit bills.
package customers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type Input struct {
	CustomerType string   `json:"customer_type"`
	Name         string   `json:"name"`
	Phone        string   `json:"phone"`
	TaxID        string   `json:"tax_id"`
	Address      string   `json:"address"`
	Note         string   `json:"note"`
	PriceTier    string   `json:"price_tier"`
	CreditLimit  *float64 `json:"credit_limit"`
	CreditDays   *int     `json:"credit_days"`
	HomeBranchID string   `json:"home_branch_id"`
	Active       *bool    `json:"active"`
}

type PointsAdjustment struct {
	Points int    `json:"points"`
	Note   string `json:"note"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

var nonDigits = regexp.MustCompile(`\D`)

// NormalizePhone keeps the digits only, so "081-234 5678" and "0812345678"
// find the same member at the till.
func NormalizePhone(value string) string {
	return nonDigits.ReplaceAllString(value, "")
}

// openBillsSQL is every issued, live bill of a customer that still owes money,
// with what has been paid against it so far. Shared by the register, the
// receivables screen and the credit check at checkout.
const openBillsSQL = `
	SELECT i.id, i.customer_id, i.branch_id, i.invoice_number, i.issued_at, i.due_date,
	       i.total_amount, COALESCE(paid.amount, 0) AS paid_amount,
	       i.total_amount - COALESCE(paid.amount, 0) AS outstanding
	FROM invoices i
	LEFT JOIN LATERAL (SELECT SUM(amount) AS amount FROM invoice_payments WHERE invoice_id = i.id) paid ON TRUE
	WHERE i.customer_id IS NOT NULL AND i.payment_status <> 'paid'
	  AND i.deleted_at IS NULL AND i.invoice_status = 'issued'`

const bangkokToday = `(NOW() AT TIME ZONE 'Asia/Bangkok')::date`

func validate(user platform.AuthUser, input Input, creating bool) (Input, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Phone = NormalizePhone(input.Phone)
	input.TaxID = strings.TrimSpace(input.TaxID)
	input.Address = strings.TrimSpace(input.Address)
	input.Note = strings.TrimSpace(input.Note)
	input.CustomerType = strings.TrimSpace(input.CustomerType)
	input.PriceTier = strings.TrimSpace(input.PriceTier)
	if input.Name == "" {
		return input, platform.NewError(http.StatusBadRequest, "กรุณากรอกชื่อลูกค้า")
	}
	if input.Phone != "" && (len(input.Phone) < 9 || len(input.Phone) > 10) {
		return input, platform.NewError(http.StatusBadRequest, "เบอร์โทรต้องเป็นตัวเลข 9-10 หลัก")
	}
	if input.TaxID != "" && len(NormalizePhone(input.TaxID)) != 13 {
		return input, platform.NewError(http.StatusBadRequest, "เลขประจำตัวผู้เสียภาษีต้องมี 13 หลัก")
	}
	if input.CustomerType == "" {
		input.CustomerType = "person"
	}
	if input.CustomerType != "person" && input.CustomerType != "business" {
		return input, platform.NewError(http.StatusBadRequest, "ประเภทลูกค้าไม่ถูกต้อง")
	}
	if creating && input.Phone == "" && input.CustomerType == "person" {
		return input, platform.NewError(http.StatusBadRequest, "สมาชิกบุคคลต้องมีเบอร์โทรสำหรับค้นหาที่หน้าร้าน")
	}
	if input.PriceTier != "" && input.PriceTier != "retail" && input.PriceTier != "wholesale" {
		return input, platform.NewError(http.StatusBadRequest, "ระดับราคาต้องเป็นขายปลีกหรือขายส่ง")
	}
	if input.CreditLimit != nil && (*input.CreditLimit < 0 || *input.CreditLimit > 100_000_000) {
		return input, platform.NewError(http.StatusBadRequest, "วงเงินเครดิตไม่ถูกต้อง")
	}
	if input.CreditDays != nil && (*input.CreditDays < 0 || *input.CreditDays > 365) {
		return input, platform.NewError(http.StatusBadRequest, "จำนวนวันเครดิตต้องอยู่ระหว่าง 0-365 วัน")
	}
	// The till may register a member and fix their contact details; the terms
	// that cost money — a credit line, a wholesale price — are head office's.
	touchesTerms := input.PriceTier == "wholesale" ||
		(input.CreditLimit != nil && *input.CreditLimit > 0) ||
		(input.CreditDays != nil && *input.CreditDays > 0)
	if touchesTerms && !platform.HasPermission(user, "customer.credit.manage") {
		return input, platform.NewError(http.StatusForbidden, "การกำหนดวงเงินเครดิตหรือราคาส่งต้องทำโดยสำนักงานใหญ่")
	}
	return input, nil
}

func (s *Service) Create(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input Input) (map[string]any, error) {
	input, err := validate(user, input, true)
	if err != nil {
		return nil, err
	}
	homeBranch := strings.TrimSpace(input.HomeBranchID)
	if !platform.IsGlobalScope(user) {
		homeBranch = platform.OwnBranchID(user)
	}
	priceTier := input.PriceTier
	if priceTier == "" {
		priceTier = "retail"
	}
	creditLimit, creditDays := 0.0, 0
	if input.CreditLimit != nil {
		creditLimit = platform.Round2(*input.CreditLimit)
	}
	if input.CreditDays != nil {
		creditDays = *input.CreditDays
	}
	id := platform.MustUUID()
	var code string
	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var next int64
		if err := tx.QueryRowContext(ctx, `SELECT nextval('customer_code_seq')`).Scan(&next); err != nil {
			return err
		}
		code = fmt.Sprintf("C%06d", next)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO customers (id, customer_code, customer_type, name, phone, tax_id, address, note,
				price_tier, credit_limit, credit_days, home_branch_id, active, created_by, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,TRUE,$13,NOW(),NOW())
		`, id, code, input.CustomerType, input.Name, input.Phone, input.TaxID, input.Address, input.Note,
			priceTier, creditLimit, creditDays, platform.NullUUID(&homeBranch), user.ID); err != nil {
			return platform.MapUniqueViolation(err, "เบอร์โทรนี้เป็นสมาชิกอยู่แล้ว")
		}
		meta.EntityType = "customer"
		meta.EntityID = &id
		meta.Action = "customer.create"
		meta.After = map[string]any{"customer_code": code, "name": input.Name, "price_tier": priceTier,
			"credit_limit": creditLimit, "credit_days": creditDays}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "customer_code": code}, nil
}

func (s *Service) Update(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id string, input Input) error {
	input, err := validate(user, input, false)
	if err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var before struct {
			Name, Phone, Tier string
			Limit             float64
			Days              int
			Active            bool
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT name, phone, price_tier, credit_limit, credit_days, active FROM customers WHERE id=$1 FOR UPDATE
		`, id).Scan(&before.Name, &before.Phone, &before.Tier, &before.Limit, &before.Days, &before.Active); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบลูกค้า")
			}
			return err
		}
		tier, limit, days, active := before.Tier, before.Limit, before.Days, before.Active
		canTerms := platform.HasPermission(user, "customer.credit.manage")
		if canTerms {
			if input.PriceTier != "" {
				tier = input.PriceTier
			}
			if input.CreditLimit != nil {
				limit = platform.Round2(*input.CreditLimit)
			}
			if input.CreditDays != nil {
				days = *input.CreditDays
			}
			if input.Active != nil {
				active = *input.Active
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE customers
			SET customer_type=$2, name=$3, phone=$4, tax_id=$5, address=$6, note=$7,
			    price_tier=$8, credit_limit=$9, credit_days=$10, active=$11, updated_at=NOW()
			WHERE id=$1
		`, id, input.CustomerType, input.Name, input.Phone, input.TaxID, input.Address, input.Note,
			tier, limit, days, active); err != nil {
			return platform.MapUniqueViolation(err, "เบอร์โทรนี้เป็นสมาชิกอยู่แล้ว")
		}
		meta.EntityType = "customer"
		meta.EntityID = &id
		meta.Action = "customer.update"
		meta.Before = map[string]any{"name": before.Name, "phone": before.Phone, "price_tier": before.Tier,
			"credit_limit": before.Limit, "credit_days": before.Days, "active": before.Active}
		meta.After = map[string]any{"name": input.Name, "phone": input.Phone, "price_tier": tier,
			"credit_limit": limit, "credit_days": days, "active": active}
		return s.audit.Log(ctx, tx, meta)
	})
}

// AdjustPoints corrects a balance by hand, with a reason, as one more ledger
// entry: the ledger is never rewritten.
func (s *Service) AdjustPoints(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id string, input PointsAdjustment) (int, error) {
	if !platform.HasPermission(user, "customer.credit.manage") {
		return 0, platform.NewError(http.StatusForbidden, "การปรับแต้มต้องทำโดยสำนักงานใหญ่")
	}
	note := strings.TrimSpace(input.Note)
	if input.Points == 0 || note == "" {
		return 0, platform.NewError(http.StatusBadRequest, "กรุณาระบุจำนวนแต้มที่ปรับและเหตุผล")
	}
	var balance int
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var current int
		if err := tx.QueryRowContext(ctx, `SELECT points_balance FROM customers WHERE id=$1 FOR UPDATE`, id).Scan(&current); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบลูกค้า")
			}
			return err
		}
		balance = current + input.Points
		if balance < 0 {
			return platform.NewError(http.StatusBadRequest, fmt.Sprintf("หักแต้มได้ไม่เกินยอดคงเหลือ %d แต้ม", current))
		}
		branchID := platform.OwnBranchID(user)
		if err := InsertPointEntry(ctx, tx, id, branchID, "", "adjust", input.Points, balance, note, user.ID); err != nil {
			return err
		}
		meta.EntityType = "customer"
		meta.EntityID = &id
		meta.Action = "customer.points_adjust"
		meta.Before = map[string]any{"points_balance": current}
		meta.After = map[string]any{"points_balance": balance, "points": input.Points, "note": note}
		return s.audit.Log(ctx, tx, meta)
	})
	return balance, err
}

// InsertPointEntry appends one ledger row and moves the cached balance to
// balanceAfter. The caller holds the customer row lock and has worked out the
// new balance; the CHECK constraints refuse a negative one.
func InsertPointEntry(ctx context.Context, tx *sql.Tx, customerID, branchID, invoiceID, entryType string, points, balanceAfter int, note, userID string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO loyalty_point_entries (id, customer_id, branch_id, invoice_id, entry_type, points, balance_after, note, created_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
	`, platform.MustUUID(), customerID, platform.NullUUID(&branchID), platform.NullUUID(&invoiceID), entryType,
		points, balanceAfter, note, platform.NullUUID(&userID)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE customers SET points_balance=$2, updated_at=NOW() WHERE id=$1`, customerID, balanceAfter)
	return err
}

type ListFilter struct {
	Search     string
	Tier       string
	CreditOnly bool
	Active     string
	Page       int
	PageSize   int
}

func (s *Service) List(ctx context.Context, user platform.AuthUser, filter ListFilter) (map[string]any, error) {
	args := []any{}
	conditions := []string{"TRUE"}
	if search := strings.TrimSpace(filter.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		like := fmt.Sprintf("$%d", len(args))
		digits := NormalizePhone(search)
		phoneClause := ""
		if digits != "" {
			args = append(args, "%"+digits+"%")
			phoneClause = fmt.Sprintf(" OR c.phone LIKE $%d", len(args))
		}
		conditions = append(conditions, fmt.Sprintf("(LOWER(c.name) LIKE %[1]s OR LOWER(c.customer_code) LIKE %[1]s OR c.tax_id LIKE %[1]s%[2]s)", like, phoneClause))
	}
	if filter.Tier == "retail" || filter.Tier == "wholesale" {
		args = append(args, filter.Tier)
		conditions = append(conditions, fmt.Sprintf("c.price_tier = $%d", len(args)))
	}
	if filter.CreditOnly {
		conditions = append(conditions, "c.credit_limit > 0")
	}
	switch filter.Active {
	case "true":
		conditions = append(conditions, "c.active")
	case "false":
		conditions = append(conditions, "NOT c.active")
	}
	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	where := strings.Join(conditions, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM customers c WHERE `+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `
		WITH open AS (`+openBillsSQL+`)
		SELECT c.id, c.customer_code, c.customer_type, c.name, c.phone, c.tax_id, c.price_tier,
		       c.credit_limit, c.credit_days, c.points_balance, c.active, COALESCE(b.name, ''),
		       COALESCE(SUM(o.outstanding), 0),
		       COALESCE(SUM(o.outstanding) FILTER (WHERE o.due_date < `+bangkokToday+`), 0),
		       c.created_at
		FROM customers c
		LEFT JOIN branches b ON b.id = c.home_branch_id
		LEFT JOIN open o ON o.customer_id = c.id
		WHERE `+where+`
		GROUP BY c.id, b.name
		ORDER BY c.created_at DESC, c.id
		LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, code, kind, name, phone, taxID, tier, branch string
		var limit, outstanding, overdue float64
		var days, points int
		var active bool
		var createdAt time.Time
		if err := rows.Scan(&id, &code, &kind, &name, &phone, &taxID, &tier, &limit, &days, &points, &active,
			&branch, &outstanding, &overdue, &createdAt); err != nil {
			return nil, err
		}
		items = append(items, customerSummary(id, code, kind, name, phone, taxID, tier, limit, days, points, active,
			branch, outstanding, overdue, createdAt))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize}, nil
}

func customerSummary(id, code, kind, name, phone, taxID, tier string, limit float64, days, points int, active bool,
	branch string, outstanding, overdue float64, createdAt time.Time) map[string]any {
	available := 0.0
	if limit > 0 {
		available = platform.Round2(limit - outstanding)
		if available < 0 {
			available = 0
		}
	}
	return map[string]any{
		"id": id, "customer_code": code, "customer_type": kind, "name": name, "phone": phone, "tax_id": taxID,
		"price_tier": tier, "credit_limit": limit, "credit_days": days, "credit_enabled": limit > 0,
		"credit_available": available, "points_balance": points, "active": active,
		"home_branch_name": branch, "outstanding": platform.Round2(outstanding),
		"overdue_amount": platform.Round2(overdue), "created_at": createdAt,
	}
}

// Lookup is the till's search: a short list, members first by exact phone.
func (s *Service) Lookup(ctx context.Context, user platform.AuthUser, query string) ([]map[string]any, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []map[string]any{}, nil
	}
	result, err := s.List(ctx, user, ListFilter{Search: query, Active: "true", PageSize: 10})
	if err != nil {
		return nil, err
	}
	items, _ := result["items"].([]map[string]any)
	digits := NormalizePhone(query)
	if digits != "" {
		// An exact phone match is the member standing at the counter.
		for index, item := range items {
			if item["phone"] == digits && index > 0 {
				items[0], items[index] = items[index], items[0]
				break
			}
		}
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, user platform.AuthUser, id string) (map[string]any, error) {
	result, err := s.listOne(ctx, id)
	if err != nil {
		return nil, err
	}
	var visits int
	var spent float64
	var lastVisit sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(total_amount), 0), MAX(issued_at)
		FROM invoices WHERE customer_id=$1 AND deleted_at IS NULL AND invoice_status='issued'
	`, id).Scan(&visits, &spent, &lastVisit); err != nil {
		return nil, err
	}
	result["visit_count"] = visits
	result["total_spent"] = platform.Round2(spent)
	if lastVisit.Valid {
		result["last_visit_at"] = lastVisit.Time
	}

	invoiceRows, err := s.db.QueryContext(ctx, `
		SELECT i.id, i.invoice_number, b.name, i.issued_at, i.total_amount, i.payment_status, i.sale_type,
		       i.due_date, i.points_earned, i.points_redeemed,
		       i.total_amount - COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id=i.id), 0)
		FROM invoices i INNER JOIN branches b ON b.id = i.branch_id
		WHERE i.customer_id=$1 AND i.deleted_at IS NULL AND i.invoice_status='issued'
		ORDER BY i.issued_at DESC LIMIT 30
	`, id)
	if err != nil {
		return nil, err
	}
	defer invoiceRows.Close()
	invoices := []map[string]any{}
	for invoiceRows.Next() {
		var invoiceID, number, branch, status, saleType string
		var issuedAt time.Time
		var dueDate sql.NullTime
		var total, outstanding float64
		var earned, redeemed int
		if err := invoiceRows.Scan(&invoiceID, &number, &branch, &issuedAt, &total, &status, &saleType, &dueDate,
			&earned, &redeemed, &outstanding); err != nil {
			return nil, err
		}
		item := map[string]any{"id": invoiceID, "invoice_number": number, "branch_name": branch, "issued_at": issuedAt,
			"total_amount": total, "payment_status": status, "sale_type": saleType, "points_earned": earned,
			"points_redeemed": redeemed, "outstanding": platform.Round2(outstanding)}
		if dueDate.Valid {
			item["due_date"] = dueDate.Time.Format("2006-01-02")
		}
		invoices = append(invoices, item)
	}
	if err := invoiceRows.Err(); err != nil {
		return nil, err
	}
	result["invoices"] = invoices

	ledger, err := s.pointLedger(ctx, id, 50)
	if err != nil {
		return nil, err
	}
	result["points_ledger"] = ledger
	return result, nil
}

func (s *Service) listOne(ctx context.Context, id string) (map[string]any, error) {
	row := s.db.QueryRowContext(ctx, `
		WITH open AS (`+openBillsSQL+` AND i.customer_id = $1)
		SELECT c.id, c.customer_code, c.customer_type, c.name, c.phone, c.tax_id, c.price_tier,
		       c.credit_limit, c.credit_days, c.points_balance, c.active, COALESCE(b.name, ''),
		       COALESCE((SELECT SUM(outstanding) FROM open), 0),
		       COALESCE((SELECT SUM(outstanding) FROM open WHERE due_date < `+bangkokToday+`), 0),
		       c.created_at, c.address, c.note, COALESCE(c.home_branch_id::text, '')
		FROM customers c LEFT JOIN branches b ON b.id = c.home_branch_id
		WHERE c.id = $1
	`, id)
	var code, kind, name, phone, taxID, tier, branch, address, note, homeBranchID string
	var limit, outstanding, overdue float64
	var days, points int
	var active bool
	var createdAt time.Time
	if err := row.Scan(&id, &code, &kind, &name, &phone, &taxID, &tier, &limit, &days, &points, &active, &branch,
		&outstanding, &overdue, &createdAt, &address, &note, &homeBranchID); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบลูกค้า")
		}
		return nil, err
	}
	item := customerSummary(id, code, kind, name, phone, taxID, tier, limit, days, points, active, branch,
		outstanding, overdue, createdAt)
	item["address"] = address
	item["note"] = note
	item["home_branch_id"] = homeBranchID
	return item, nil
}

func (s *Service) pointLedger(ctx context.Context, customerID string, limit int) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.entry_type, e.points, e.balance_after, e.note, e.created_at,
		       COALESCE(b.name, ''), COALESCE(i.invoice_number, ''), COALESCE(i.id::text, ''), COALESCE(u.full_name, '')
		FROM loyalty_point_entries e
		LEFT JOIN branches b ON b.id = e.branch_id
		LEFT JOIN invoices i ON i.id = e.invoice_id
		LEFT JOIN users u ON u.id = e.created_by
		WHERE e.customer_id = $1
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $2
	`, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, entryType, note, branch, invoiceNumber, invoiceID, actor string
		var points, balance int
		var createdAt time.Time
		if err := rows.Scan(&id, &entryType, &points, &balance, &note, &createdAt, &branch, &invoiceNumber, &invoiceID, &actor); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": id, "entry_type": entryType, "points": points, "balance_after": balance,
			"note": note, "created_at": createdAt, "branch_name": branch, "invoice_number": invoiceNumber,
			"invoice_id": invoiceID, "created_by_name": actor})
	}
	return items, rows.Err()
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	result, err := h.service.List(c.Request().Context(), platform.CurrentUser(c), ListFilter{
		Search: c.QueryParam("search"), Tier: c.QueryParam("price_tier"),
		CreditOnly: c.QueryParam("credit_only") == "true", Active: c.QueryParam("active"),
		Page: page, PageSize: pageSize,
	})
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดรายชื่อลูกค้าไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) Lookup(c echo.Context) error {
	items, err := h.service.Lookup(c.Request().Context(), platform.CurrentUser(c), c.QueryParam("q"))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "ค้นหาลูกค้าไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Get(c echo.Context) error {
	item, err := h.service.Get(c.Request().Context(), platform.CurrentUser(c), c.Param("customerID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}

func (h *Handler) Create(c echo.Context) error {
	var input Input
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	result, err := h.service.Create(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	result["message"] = "สมัครสมาชิกแล้ว"
	return platform.JSON(c, http.StatusCreated, result)
}

func (h *Handler) Update(c echo.Context) error {
	var input Input
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	if err := h.service.Update(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("customerID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกข้อมูลลูกค้าแล้ว")
}

func (h *Handler) AdjustPoints(c echo.Context) error {
	var input PointsAdjustment
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	balance, err := h.service.AdjustPoints(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("customerID"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"points_balance": balance, "message": "ปรับแต้มแล้ว"})
}

type LoyaltySettings struct {
	BahtPerPoint    float64 `json:"baht_per_point"`
	PointValue      float64 `json:"point_value"`
	MinRedeemPoints int     `json:"min_redeem_points"`
}

func (s *Service) GetLoyaltySettings(ctx context.Context) (LoyaltySettings, error) {
	perPoint, err := platform.GetSettingFloat(ctx, s.db, "loyalty_baht_per_point", 25)
	if err != nil {
		return LoyaltySettings{}, err
	}
	value, err := platform.GetSettingFloat(ctx, s.db, "loyalty_point_value", 0.25)
	if err != nil {
		return LoyaltySettings{}, err
	}
	minimum, err := platform.GetSettingFloat(ctx, s.db, "loyalty_min_redeem_points", 40)
	if err != nil {
		return LoyaltySettings{}, err
	}
	return LoyaltySettings{BahtPerPoint: perPoint, PointValue: value, MinRedeemPoints: int(minimum)}, nil
}

// SaveLoyaltySettings changes the rules for sales from now on. Points already
// earned keep the value they were earned at only in the sense that the ledger
// records points, not baht; a new point value applies to every redemption.
func (s *Service) SaveLoyaltySettings(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input LoyaltySettings) error {
	if input.BahtPerPoint < 1 || input.BahtPerPoint > 10000 {
		return platform.NewError(http.StatusBadRequest, "ยอดซื้อต่อ 1 แต้มต้องอยู่ระหว่าง 1-10,000 บาท")
	}
	if input.PointValue <= 0 || input.PointValue > 100 {
		return platform.NewError(http.StatusBadRequest, "มูลค่าแต้มต้องมากกว่า 0 และไม่เกิน 100 บาท")
	}
	if input.MinRedeemPoints < 1 {
		return platform.NewError(http.StatusBadRequest, "แต้มขั้นต่ำที่ใช้ได้ต้องอย่างน้อย 1 แต้ม")
	}
	before, err := s.GetLoyaltySettings(ctx)
	if err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		values := map[string]string{
			"loyalty_baht_per_point":    strconv.FormatFloat(input.BahtPerPoint, 'f', -1, 64),
			"loyalty_point_value":       strconv.FormatFloat(input.PointValue, 'f', -1, 64),
			"loyalty_min_redeem_points": strconv.Itoa(input.MinRedeemPoints),
		}
		for key, value := range values {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO app_settings (setting_key, setting_value, metadata, created_at, updated_at)
				VALUES ($1, $2, '{}'::jsonb, NOW(), NOW())
				ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value, updated_at = NOW()
			`, key, value); err != nil {
				return err
			}
		}
		meta.EntityType = "settings"
		meta.Action = "loyalty.settings.update"
		meta.Before = before
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
}

func (h *Handler) GetLoyaltySettings(c echo.Context) error {
	settings, err := h.service.GetLoyaltySettings(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดเงื่อนไขแต้มไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, settings)
}

func (h *Handler) SaveLoyaltySettings(c echo.Context) error {
	var input LoyaltySettings
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	if err := h.service.SaveLoyaltySettings(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกเงื่อนไขแต้มสะสมแล้ว")
}
