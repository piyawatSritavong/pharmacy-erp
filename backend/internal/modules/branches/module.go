package branches

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

type Branch struct {
	ID                 string `json:"id"`
	Code               string `json:"code"`
	Name               string `json:"name"`
	Address            string `json:"address"`
	BranchType         string `json:"branch_type"`
	ParentBranchID     string `json:"parent_branch_id,omitempty"`
	ParentBranchName   string `json:"parent_branch_name,omitempty"`
	Active             bool   `json:"active"`
	SalesEnabled       bool   `json:"sales_enabled"`
	OnlineSalesEnabled bool   `json:"online_sales_enabled"`
}

type BranchInput struct {
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	Address            string  `json:"address"`
	BranchType         string  `json:"branch_type"`
	ParentBranchID     *string `json:"parent_branch_id"`
	Active             *bool   `json:"active"`
	SalesEnabled       *bool   `json:"sales_enabled"`
	OnlineSalesEnabled *bool   `json:"online_sales_enabled"`
}

type Sequence struct {
	ID         string `json:"id"`
	BranchID   string `json:"branch_id"`
	BranchCode string `json:"branch_code"`
	BranchName string `json:"branch_name"`
	DocType    string `json:"doc_type"`
	Prefix     string `json:"prefix"`
	NextNumber int64  `json:"next_number"`
	IsLocked   bool   `json:"is_locked"`
	Example    string `json:"example_number"`
}

type UpdateSequenceRequest struct {
	Prefix     string `json:"prefix"`
	NextNumber int64  `json:"next_number"`
	IsLocked   bool   `json:"is_locked"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) List(ctx context.Context) ([]Branch, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id, b.code, b.name, b.address, b.branch_type,
		       COALESCE(b.parent_branch_id::text, ''), COALESCE(parent.name, ''), b.active, b.sales_enabled, b.online_sales_enabled
		FROM branches b
		LEFT JOIN branches parent ON parent.id = b.parent_branch_id
		ORDER BY CASE WHEN b.branch_type = 'main_warehouse' THEN 0 ELSE 1 END, b.name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Branch{}
	for rows.Next() {
		var item Branch
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Address, &item.BranchType, &item.ParentBranchID, &item.ParentBranchName, &item.Active, &item.SalesEnabled, &item.OnlineSalesEnabled); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Create(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input BranchInput) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	if code == "" {
		code = platform.GenerateReadableCode("BR")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", platform.NewError(http.StatusBadRequest, "กรุณากรอกชื่อสาขา")
	}

	branchID := platform.MustUUID()
	branchType, parentBranchID, err := s.validateHierarchy(ctx, branchID, input.BranchType, input.ParentBranchID)
	if err != nil {
		return "", err
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	salesEnabled := branchType != "main_warehouse"
	if input.SalesEnabled != nil {
		salesEnabled = *input.SalesEnabled
	}
	onlineSalesEnabled := false
	if input.OnlineSalesEnabled != nil {
		onlineSalesEnabled = *input.OnlineSalesEnabled
	}
	if branchType == "main_warehouse" {
		salesEnabled = false
		onlineSalesEnabled = false
	}

	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO branches (id, code, name, address, branch_type, parent_branch_id, active, sales_enabled, online_sales_enabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		`, branchID, code, name, strings.TrimSpace(input.Address), branchType, platform.NullUUID(parentBranchID), active, salesEnabled, onlineSalesEnabled); err != nil {
			return platform.MapUniqueViolation(err, "รหัสสาขานี้มีอยู่แล้ว")
		}

		for _, docType := range []string{"invoice", "quotation", "purchase_order"} {
			prefix := "BL"
			if docType == "quotation" {
				prefix = "QT"
			} else if docType == "purchase_order" {
				prefix = "PO"
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO document_sequences (id, branch_id, doc_type, prefix, next_number, is_locked, created_at, updated_at)
				VALUES ($1, $2, $3, $4, 1, FALSE, NOW(), NOW())
			`, platform.MustUUID(), branchID, docType, prefix); err != nil {
				return err
			}
		}

		meta.EntityType = "branch"
		meta.EntityID = &branchID
		meta.Action = "branch.create"
		meta.After = map[string]any{"code": code, "name": name, "branch_type": branchType, "parent_branch_id": parentBranchID, "active": active, "sales_enabled": salesEnabled, "online_sales_enabled": onlineSalesEnabled}
		return s.audit.Log(ctx, tx, meta)
	})
	return branchID, err
}

func (s *Service) Update(ctx context.Context, branchID string, user platform.AuthUser, meta audit.LogEntry, input BranchInput) error {
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	name := strings.TrimSpace(input.Name)
	if code == "" || name == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณากรอกรหัสและชื่อสาขา")
	}
	branchType, parentBranchID, err := s.validateHierarchy(ctx, branchID, input.BranchType, input.ParentBranchID)
	if err != nil {
		return err
	}

	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var beforeJSON string
		if err := tx.QueryRowContext(ctx, `
			SELECT row_to_json(b)::text
			FROM (
				SELECT code, name, address, branch_type, parent_branch_id::text, active, sales_enabled, online_sales_enabled
				FROM branches
				WHERE id = $1
			) b
		`, branchID).Scan(&beforeJSON); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบสาขา")
			}
			return err
		}

		var currentType string
		var currentActive, currentSalesEnabled, currentOnlineSalesEnabled bool
		if err := tx.QueryRowContext(ctx, `SELECT branch_type, active, sales_enabled, online_sales_enabled FROM branches WHERE id = $1`, branchID).Scan(&currentType, &currentActive, &currentSalesEnabled, &currentOnlineSalesEnabled); err != nil {
			return err
		}
		active := currentActive
		if input.Active != nil {
			active = *input.Active
		}
		salesEnabled := currentSalesEnabled
		if input.SalesEnabled != nil {
			salesEnabled = *input.SalesEnabled
		} else if currentType == "main_warehouse" && branchType == "branch" {
			salesEnabled = true
		}
		onlineSalesEnabled := currentOnlineSalesEnabled
		if input.OnlineSalesEnabled != nil {
			onlineSalesEnabled = *input.OnlineSalesEnabled
		}
		if branchType == "main_warehouse" {
			salesEnabled = false
			onlineSalesEnabled = false
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE branches
			SET code = $2, name = $3, address = $4, branch_type = $5, parent_branch_id = $6, active = $7, sales_enabled = $8, online_sales_enabled = $9, updated_at = NOW()
			WHERE id = $1
		`, branchID, code, name, strings.TrimSpace(input.Address), branchType, platform.NullUUID(parentBranchID), active, salesEnabled, onlineSalesEnabled); err != nil {
			return platform.MapUniqueViolation(err, "รหัสสาขานี้มีอยู่แล้ว")
		}

		meta.EntityType = "branch"
		meta.EntityID = &branchID
		meta.Action = "branch.update"
		meta.Before = beforeJSON
		meta.After = map[string]any{"code": code, "name": name, "address": strings.TrimSpace(input.Address), "branch_type": branchType, "parent_branch_id": parentBranchID, "active": active, "sales_enabled": salesEnabled, "online_sales_enabled": onlineSalesEnabled}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) validateHierarchy(ctx context.Context, branchID, branchType string, parentBranchID *string) (string, *string, error) {
	branchType = strings.TrimSpace(branchType)
	if branchType == "" {
		branchType = "branch"
	}
	if branchType != "main_warehouse" && branchType != "branch" {
		return "", nil, platform.NewError(http.StatusBadRequest, "ประเภทสาขาไม่ถูกต้อง")
	}
	if parentBranchID != nil {
		trimmed := strings.TrimSpace(*parentBranchID)
		if trimmed == "" {
			parentBranchID = nil
		} else {
			parentBranchID = &trimmed
		}
	}
	if branchType == "main_warehouse" {
		return branchType, nil, nil
	}
	if parentBranchID == nil {
		return branchType, nil, nil
	}
	if *parentBranchID == branchID {
		return "", nil, platform.NewError(http.StatusBadRequest, "สาขาไม่สามารถใช้ตัวเองเป็นคลังหลักได้")
	}
	var parentType string
	if err := s.db.QueryRowContext(ctx, `SELECT branch_type FROM branches WHERE id = $1 AND active = TRUE`, *parentBranchID).Scan(&parentType); err != nil {
		if err == sql.ErrNoRows {
			return "", nil, platform.NewError(http.StatusBadRequest, "ไม่พบคลังหลักที่เลือก")
		}
		return "", nil, err
	}
	if parentType != "main_warehouse" {
		return "", nil, platform.NewError(http.StatusBadRequest, "สาขาต้นสังกัดต้องเป็นคลังหลัก")
	}
	return branchType, parentBranchID, nil
}

func (s *Service) ListSequences(ctx context.Context) ([]Sequence, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ds.id, ds.branch_id::text, b.code, b.name, ds.doc_type, ds.prefix, ds.next_number, ds.is_locked
		FROM document_sequences ds
		INNER JOIN branches b ON b.id = ds.branch_id
		ORDER BY b.name, ds.doc_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Sequence{}
	for rows.Next() {
		var item Sequence
		if err := rows.Scan(&item.ID, &item.BranchID, &item.BranchCode, &item.BranchName, &item.DocType, &item.Prefix, &item.NextNumber, &item.IsLocked); err != nil {
			return nil, err
		}
		item.Example = platform.FormatBranchDocumentNumber(item.BranchCode, item.Prefix, time.Now().UTC(), item.NextNumber)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) UpdateSequence(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, branchID string, docType string, input UpdateSequenceRequest) error {
	if input.NextNumber <= 0 {
		return platform.NewError(http.StatusBadRequest, "next_number must be greater than zero")
	}

	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var beforeJSON string
		var sequenceID string
		var currentPrefix, branchCode string
		var currentNextNumber int64
		var currentLocked bool
		if err := tx.QueryRowContext(ctx, `
			SELECT row_to_json(ds)::text, ds.id::text, ds.prefix, ds.next_number, ds.is_locked,
			       (SELECT code FROM branches WHERE id=$1)
			FROM (
				SELECT id, prefix, next_number, is_locked
				FROM document_sequences
				WHERE branch_id = $1 AND doc_type = $2
			) ds
		`, branchID, docType).Scan(&beforeJSON, &sequenceID, &currentPrefix, &currentNextNumber, &currentLocked, &branchCode); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "sequence not found")
			}
			return err
		}

		prefix, err := validateSequenceUpdate(currentPrefix, currentNextNumber, currentLocked, input)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE document_sequences
			SET prefix = $3, next_number = $4, is_locked = $5, updated_at = NOW()
			WHERE branch_id = $1 AND doc_type = $2
		`, branchID, docType, prefix, input.NextNumber, input.IsLocked); err != nil {
			return err
		}

		meta.EntityType = "document_sequence"
		meta.EntityID = &sequenceID
		meta.Action = "sequence.update"
		meta.Before = beforeJSON
		meta.After = map[string]any{
			"branch_id":      branchID,
			"doc_type":       docType,
			"prefix":         prefix,
			"next_number":    input.NextNumber,
			"is_locked":      input.IsLocked,
			"example_number": platform.FormatBranchDocumentNumber(branchCode, prefix, time.Now().UTC(), input.NextNumber),
		}
		return s.audit.Log(ctx, tx, meta)
	})
}

func validateSequenceUpdate(currentPrefix string, currentNextNumber int64, currentLocked bool, input UpdateSequenceRequest) (string, error) {
	prefix := strings.ToUpper(strings.TrimSpace(input.Prefix))
	if prefix == "" {
		return "", platform.NewError(http.StatusBadRequest, "prefix is required")
	}
	if !currentLocked {
		return prefix, nil
	}

	changedFields := prefix != currentPrefix || input.NextNumber != currentNextNumber
	if changedFields {
		if !input.IsLocked {
			return "", platform.NewError(http.StatusConflict, "sequence is locked; unlock it before editing prefix or next number")
		}
		return "", platform.NewError(http.StatusConflict, "sequence is locked; prefix and next number cannot be edited")
	}
	return prefix, nil
}

func (s *Service) DeletionImpact(ctx context.Context, branchID string) (map[string]any, error) {
	var code, name string
	if err := s.db.QueryRowContext(ctx, `SELECT code, name FROM branches WHERE id = $1`, branchID).Scan(&code, &name); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบสาขา")
		}
		return nil, err
	}
	queries := map[string]string{
		"ผู้ใช้":       "SELECT COUNT(*) FROM users WHERE branch_id = $1",
		"รายการสต๊อก":  "SELECT COUNT(*) FROM inventory WHERE branch_id = $1",
		"ใบเสนอราคา":   "SELECT COUNT(*) FROM quotations WHERE branch_id = $1",
		"ใบขาย":        "SELECT COUNT(*) FROM invoices WHERE branch_id = $1",
		"การโอนสินค้า": "SELECT COUNT(*) FROM transfers WHERE source_branch_id = $1 OR destination_branch_id = $1",
		"คำสั่งซื้อจากตลาดออนไลน์": "SELECT COUNT(*) FROM marketplace_orders WHERE branch_id = $1",
	}
	counts := map[string]int{}
	for key, query := range queries {
		var count int
		if err := s.db.QueryRowContext(ctx, query, branchID).Scan(&count); err != nil {
			return nil, err
		}
		counts[key] = count
	}
	return map[string]any{
		"id": branchID, "code": code, "name": name, "counts": counts,
		"confirmation": "ลบ " + code,
	}, nil
}

func (s *Service) Delete(ctx context.Context, branchID string, confirmation string, meta audit.LogEntry) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var code, name string
		if err := tx.QueryRowContext(ctx, `
			SELECT code, name FROM branches WHERE id = $1 FOR UPDATE
		`, branchID).Scan(&code, &name); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบสาขา")
			}
			return err
		}
		if confirmation != "ลบ "+code {
			return platform.NewError(http.StatusBadRequest, "ข้อความยืนยันการลบไม่ถูกต้อง")
		}
		var invoiceCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoices WHERE branch_id=$1`, branchID).Scan(&invoiceCount); err != nil {
			return err
		}
		if invoiceCount > 0 {
			return platform.NewError(http.StatusConflict, "ไม่สามารถลบสาขาที่มีประวัติใบขายได้")
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM inventory_movements
			WHERE branch_id = $1
			   OR reference_id IN (
				SELECT id FROM transfers WHERE source_branch_id = $1 OR destination_branch_id = $1
			   )
		`, branchID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM audit_logs WHERE branch_id = $1`, branchID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM branches WHERE id = $1`, branchID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory i
			SET qty_real = COALESCE((
				SELECT SUM(m.quantity_delta) FROM inventory_movements m
				WHERE m.branch_id = i.branch_id AND m.product_id = i.product_id AND m.stock_bucket = 'real'
			), 0),
			qty_ghost = COALESCE((
				SELECT SUM(m.quantity_delta) FROM inventory_movements m
				WHERE m.branch_id = i.branch_id AND m.product_id = i.product_id AND m.stock_bucket = 'ghost'
			), 0),
			updated_at = NOW()
		`); err != nil {
			return err
		}
		meta.BranchID = nil
		meta.EntityType = "branch"
		meta.EntityID = nil
		meta.Action = "branch.delete"
		meta.After = map[string]any{"deleted_id": branchID, "code": code, "name": name}
		return s.audit.Log(ctx, tx, meta)
	})
}

type deleteRequest struct {
	Confirmation string `json:"confirmation"`
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c echo.Context) error {
	items, err := h.service.List(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load branches", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Create(c echo.Context) error {
	var input BranchInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	id, err := h.service.Create(c.Request().Context(), platform.CurrentUser(c), meta, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "เพิ่มสาขาแล้ว"})
}

func (h *Handler) Update(c echo.Context) error {
	if err := platform.RequireGlobalScope(platform.CurrentUser(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	var input BranchInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Update(c.Request().Context(), c.Param("branchID"), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกสาขาแล้ว")
}

func (h *Handler) DeletionImpact(c echo.Context) error {
	if err := platform.RequireGlobalScope(platform.CurrentUser(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	impact, err := h.service.DeletionImpact(c.Request().Context(), c.Param("branchID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, impact)
}

func (h *Handler) Delete(c echo.Context) error {
	if err := platform.RequireGlobalScope(platform.CurrentUser(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	var input deleteRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลยืนยันการลบไม่ถูกต้อง"))
	}
	if err := h.service.Delete(c.Request().Context(), c.Param("branchID"), input.Confirmation, audit.MetaFromContext(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบสาขาและข้อมูลที่เกี่ยวข้องแล้ว")
}

func (h *Handler) ListSequences(c echo.Context) error {
	items, err := h.service.ListSequences(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load sequences", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) UpdateSequence(c echo.Context) error {
	if err := platform.RequireGlobalScope(platform.CurrentUser(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	var input UpdateSequenceRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	err := h.service.UpdateSequence(c.Request().Context(), platform.CurrentUser(c), meta, c.Param("branchID"), c.Param("docType"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกเลขที่เอกสารแล้ว")
}
