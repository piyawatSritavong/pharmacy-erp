// Package returns implements Part B Rule 4 — POS return → back-office claim
// → supplier replacement, with the Case A (same model restocked) / Case B
// (discontinued, different model received in) accounting split.
//
// Claim-driven stock is lot-tracked: goods leave on FEFO allocations and come
// back as a fresh lot, so inventory.qty_real/qty_ghost and inventory_lots stay
// in step. A stock claim (InitiateStockClaim, stock_claim.go) is also the one
// operational path Ghost Stock may take out of inventory outside the month-end
// close — the case business-flow.md calls "เคลมหรือคืนสินค้า กับบริษัทที่
// ซื้อขายโดยตรง".
package returns

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/stocklot"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type InitiateInput struct {
	InvoiceItemID string `json:"invoice_item_id"`
	Quantity      int    `json:"quantity"`
	Reason        string `json:"reason"`
}

type SendToSupplierInput struct {
	SupplierID string `json:"supplier_id"`
}

type ResolveCaseBInput struct {
	ReplacementProductID string `json:"replacement_product_id"`
}

type RejectInput struct {
	Note string `json:"note"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

// Initiate is the POS action: "customer returns a defective item, POS looks
// up the original sale and issues a replacement on the spot." The
// replacement is deducted from current stock immediately, same as any sale;
// the original item goes into the back-office claim queue.
func (s *Service) Initiate(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input InitiateInput) (string, error) {
	if !platform.HasPermission(user, "invoice.create.pos") {
		return "", platform.NewError(http.StatusForbidden, "เฉพาะพนักงานขายหน้าร้านเท่านั้นที่ออกสินค้าทดแทนได้")
	}
	if strings.TrimSpace(input.InvoiceItemID) == "" || input.Quantity <= 0 {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเลือกรายการและจำนวนที่ถูกต้อง")
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาระบุเหตุผลการคืนสินค้า")
	}

	returnID := platform.MustUUID()
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var branchID, productID, stockBucket string
		var soldQuantity int
		var invoiceStatus string
		if err := tx.QueryRowContext(ctx, `
			SELECT i.branch_id::text, ii.product_id::text, ii.stock_bucket, ii.quantity, i.invoice_status
			FROM invoice_items ii
			INNER JOIN invoices i ON i.id = ii.invoice_id
			WHERE ii.id = $1 AND i.deleted_at IS NULL AND ii.reconciliation_removed_at IS NULL
			FOR UPDATE OF ii
		`, input.InvoiceItemID).Scan(&branchID, &productID, &stockBucket, &soldQuantity, &invoiceStatus); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบรายการใบขายนี้")
			}
			return err
		}
		if invoiceStatus != "issued" {
			return platform.NewError(http.StatusBadRequest, "ใบขายนี้ไม่ได้อยู่ในสถานะออกเอกสารแล้ว")
		}
		if _, err := platform.MustBranchID(user, branchID); err != nil {
			return err
		}
		// A sale is only ever drawn from real stock, so this is belt-and-braces
		// against a line that somehow is not. Ghost never reaches a customer
		// document; it is claimed against the shelf instead — InitiateStockClaim.
		if stockBucket != "real" {
			return platform.NewError(http.StatusConflict, "การคืนสินค้าใช้ได้เฉพาะรายการที่ตัดจากสต๊อกจริง")
		}

		var alreadyReturned int
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(quantity), 0) FROM product_returns
			WHERE original_invoice_item_id = $1 AND status != 'rejected'
		`, input.InvoiceItemID).Scan(&alreadyReturned); err != nil {
			return err
		}
		if alreadyReturned+input.Quantity > soldQuantity {
			return platform.NewError(http.StatusConflict, "จำนวนที่คืนเกินกว่าจำนวนที่ขายในรายการนี้")
		}

		column := "qty_real"
		var available int
		if err := tx.QueryRowContext(ctx, `
			SELECT `+column+` FROM inventory WHERE branch_id = $1 AND product_id = $2 FOR UPDATE
		`, branchID, productID).Scan(&available); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusConflict, "ไม่พบสต๊อกสินค้านี้ที่สาขา")
			}
			return err
		}
		if available < input.Quantity {
			return platform.NewError(http.StatusConflict, "สต๊อกไม่เพียงพอสำหรับออกสินค้าทดแทน")
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory SET `+column+` = `+column+` - $3, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, branchID, productID, input.Quantity); err != nil {
			return err
		}
		allocations, err := stocklot.AllocateFEFO(ctx, tx, branchID, productID, stockBucket, input.Quantity)
		if err != nil {
			return err
		}
		issueMovementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'return_replacement_issue', $4, $5, 'product_return', $6, $7, $8, NOW())
		`, issueMovementID, branchID, productID, stockBucket, -input.Quantity, returnID, "ออกสินค้าทดแทนให้ลูกค้า: "+reason, user.ID); err != nil {
			return err
		}
		if err := stocklot.AttachMovement(ctx, tx, issueMovementID, allocations, -1); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_returns (id, branch_id, original_invoice_item_id, product_id, stock_bucket, quantity, reason, status, created_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending_claim', $8, NOW(), NOW())
		`, returnID, branchID, input.InvoiceItemID, productID, stockBucket, input.Quantity, reason, user.ID); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, returnID, "pending_claim", "ออกสินค้าทดแทนที่หน้าร้าน", user.ID); err != nil {
			return err
		}

		meta.EntityType = "product_return"
		meta.EntityID = &returnID
		meta.Action = "product_return.initiate"
		meta.After = map[string]any{"invoice_item_id": input.InvoiceItemID, "quantity": input.Quantity, "reason": reason}
		return s.audit.Log(ctx, tx, meta)
	})
	return returnID, err
}

func insertEvent(ctx context.Context, tx *sql.Tx, returnID, status, note, actorID string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO return_events (id, return_id, status, note, actor_id, event_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, platform.MustUUID(), returnID, status, note, actorID)
	return err
}

func (s *Service) List(ctx context.Context, user platform.AuthUser, status string) ([]map[string]any, error) {
	query := `
		SELECT pr.id::text, pr.status, pr.quantity, pr.reason, pr.stock_bucket, pr.origin,
		       p.name, p.sku, b.name, COALESCE(s.legal_name, ''),
		       pr.resolution_case, COALESCE(rp.name, ''), pr.claim_sent_at, pr.resolved_at, pr.created_at
		FROM product_returns pr
		INNER JOIN products p ON p.id = pr.product_id
		INNER JOIN branches b ON b.id = pr.branch_id
		LEFT JOIN suppliers s ON s.id = pr.supplier_id
		LEFT JOIN products rp ON rp.id = pr.replacement_product_id
	`
	own, err := platform.BranchFilter(user, "")
	if err != nil {
		return nil, err
	}
	args := []any{}
	conditions := []string{}
	if own != "" {
		args = append(args, own)
		conditions = append(conditions, "pr.branch_id = $1")
	}
	if status != "" {
		args = append(args, status)
		conditions = append(conditions, "pr.status = $"+strconv.Itoa(len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY CASE WHEN pr.status = 'pending_claim' THEN 0 WHEN pr.status = 'sent_to_supplier' THEN 1 ELSE 2 END, pr.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, status, reason, stockBucket, origin, productName, sku, branchName, supplierName, replacementName string
		var quantity int
		var resolutionCase sql.NullString
		var claimSentAt, resolvedAt sql.NullTime
		var createdAt any
		if err := rows.Scan(&id, &status, &quantity, &reason, &stockBucket, &origin, &productName, &sku, &branchName, &supplierName,
			&resolutionCase, &replacementName, &claimSentAt, &resolvedAt, &createdAt); err != nil {
			return nil, err
		}
		if user.RoleKey != "super_admin" && stockBucket == "ghost" {
			continue
		}
		item := map[string]any{
			"id": id, "status": status, "quantity": quantity, "reason": reason, "origin": origin,
			"product_name": productName, "sku": sku, "branch_name": branchName, "supplier_name": supplierName,
			"resolution_case": resolutionCase.String, "replacement_product_name": replacementName, "created_at": createdAt,
		}
		if user.RoleKey == "super_admin" {
			item["stock_bucket"] = stockBucket
		}
		if claimSentAt.Valid {
			item["claim_sent_at"] = claimSentAt.Time
		}
		if resolvedAt.Valid {
			item["resolved_at"] = resolvedAt.Time
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) SendToSupplier(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, returnID string, input SendToSupplierInput) error {
	if !platform.HasPermission(user, "returns.manage") {
		return platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์จัดการเคลม")
	}
	if strings.TrimSpace(input.SupplierID) == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณาเลือกคู่ค้า")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status, branchID, stockBucket string
		if err := tx.QueryRowContext(ctx, `SELECT status,branch_id::text,stock_bucket FROM product_returns WHERE id = $1 FOR UPDATE`, returnID).Scan(&status, &branchID, &stockBucket); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบรายการคืนสินค้า")
			}
			return err
		}
		if err := validateReturnVisibility(user, branchID, stockBucket); err != nil {
			return err
		}
		if status != "pending_claim" {
			return platform.NewError(http.StatusConflict, "รายการนี้ถูกดำเนินการแล้ว")
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE product_returns SET status = 'sent_to_supplier', supplier_id = $2, claim_sent_at = NOW(), updated_at = NOW()
			WHERE id = $1
		`, returnID, input.SupplierID); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, returnID, "sent_to_supplier", "ส่งเคลมให้คู่ค้าแล้ว", user.ID); err != nil {
			return err
		}
		meta.EntityType = "product_return"
		meta.EntityID = &returnID
		meta.Action = "product_return.send_to_supplier"
		meta.After = map[string]any{"supplier_id": input.SupplierID}
		return s.audit.Log(ctx, tx, meta)
	})
}

// ResolveCaseA: supplier sent back the same model, new unit — direct 1-for-1
// restock into the branch's own stock.
func (s *Service) ResolveCaseA(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, returnID string) error {
	if !platform.HasPermission(user, "returns.manage") {
		return platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์จัดการเคลม")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status, branchID, productID, stockBucket string
		var quantity int
		if err := tx.QueryRowContext(ctx, `
			SELECT status, branch_id::text, product_id::text, stock_bucket, quantity
			FROM product_returns WHERE id = $1 FOR UPDATE
		`, returnID).Scan(&status, &branchID, &productID, &stockBucket, &quantity); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบรายการคืนสินค้า")
			}
			return err
		}
		if status != "sent_to_supplier" {
			return platform.NewError(http.StatusConflict, "รายการนี้ยังไม่ได้ส่งเคลม หรือถูกปิดแล้ว")
		}
		if stockBucket == "ghost" && user.RoleKey != "super_admin" {
			return platform.NewError(http.StatusForbidden, "รายการนี้เป็นสต๊อกผีและดำเนินการได้เฉพาะผู้ดูแลระบบสูงสุด")
		}
		if err := validateReturnVisibility(user, branchID, stockBucket); err != nil {
			return err
		}
		column := "qty_real"
		if stockBucket == "ghost" {
			column = "qty_ghost"
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory SET `+column+` = `+column+` + $3, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, branchID, productID, quantity); err != nil {
			return err
		}
		restockMovementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'claim_restock', $4, $5, 'product_return', $6, 'รับสินค้ารุ่นเดิมจากคู่ค้าตามเคลม', $7, NOW())
		`, restockMovementID, branchID, productID, stockBucket, quantity, returnID, user.ID); err != nil {
			return err
		}
		// A replacement unit is physically new, so it opens its own lot rather
		// than topping up the one the defective unit came off.
		if err := attachClaimRestockLot(ctx, tx, restockMovementID, returnID, branchID, productID, stockBucket, quantity); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE product_returns SET status = 'resolved_case_a', resolution_case = 'a', resolved_at = NOW(), updated_at = NOW()
			WHERE id = $1
		`, returnID); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, returnID, "resolved_case_a", "รับสินค้ารุ่นเดิมจากคู่ค้า คืนสต๊อกแล้ว", user.ID); err != nil {
			return err
		}
		meta.EntityType = "product_return"
		meta.EntityID = &returnID
		meta.Action = "product_return.resolve_case_a"
		return s.audit.Log(ctx, tx, meta)
	})
}

// ResolveCaseB: model discontinued, supplier sends a different (same-price)
// model instead. NOT a restock of the original — that unit stays written
// off (it already left inventory when the POS replacement went out). This
// receives the *different* model in as its own stock-in.
func (s *Service) ResolveCaseB(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, returnID string, input ResolveCaseBInput) error {
	if !platform.HasPermission(user, "returns.manage") {
		return platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์จัดการเคลม")
	}
	if strings.TrimSpace(input.ReplacementProductID) == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณาเลือกสินค้ารุ่นทดแทน")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status, branchID, stockBucket string
		var quantity int
		if err := tx.QueryRowContext(ctx, `
			SELECT status, branch_id::text, stock_bucket, quantity
			FROM product_returns WHERE id = $1 FOR UPDATE
		`, returnID).Scan(&status, &branchID, &stockBucket, &quantity); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบรายการคืนสินค้า")
			}
			return err
		}
		if status != "sent_to_supplier" {
			return platform.NewError(http.StatusConflict, "รายการนี้ยังไม่ได้ส่งเคลม หรือถูกปิดแล้ว")
		}
		if stockBucket == "ghost" && user.RoleKey != "super_admin" {
			return platform.NewError(http.StatusForbidden, "รายการนี้เป็นสต๊อกผีและดำเนินการได้เฉพาะผู้ดูแลระบบสูงสุด")
		}
		if err := validateReturnVisibility(user, branchID, stockBucket); err != nil {
			return err
		}
		var replacementCost float64
		if err := tx.QueryRowContext(ctx, `SELECT cost_price FROM products WHERE id = $1`, input.ReplacementProductID).Scan(&replacementCost); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusBadRequest, "ไม่พบสินค้ารุ่นทดแทน")
			}
			return err
		}
		column := "qty_real"
		if stockBucket == "ghost" {
			column = "qty_ghost"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory (id, branch_id, product_id, qty_real, qty_ghost, created_at, updated_at)
			VALUES ($1, $2, $3, 0, 0, NOW(), NOW())
			ON CONFLICT (branch_id, product_id) DO NOTHING
		`, platform.MustUUID(), branchID, input.ReplacementProductID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory SET `+column+` = `+column+` + $3, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, branchID, input.ReplacementProductID, quantity); err != nil {
			return err
		}
		receiveMovementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'claim_replacement_receive', $4, $5, 'product_return', $6, 'รับสินค้ารุ่นทดแทนจากคู่ค้าตามเคลม (รุ่นเดิมเลิกผลิต)', $7, NOW())
		`, receiveMovementID, branchID, input.ReplacementProductID, stockBucket, quantity, returnID, user.ID); err != nil {
			return err
		}
		if err := attachClaimRestockLot(ctx, tx, receiveMovementID, returnID, branchID, input.ReplacementProductID, stockBucket, quantity); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE product_returns
			SET status = 'resolved_case_b', resolution_case = 'b', replacement_product_id = $2,
			    replacement_cost_snapshot = $3, resolved_at = NOW(), updated_at = NOW()
			WHERE id = $1
		`, returnID, input.ReplacementProductID, platform.Round2(replacementCost*float64(quantity))); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, returnID, "resolved_case_b", "รุ่นเดิมเลิกผลิต รับรุ่นทดแทนจากคู่ค้าแทน — รุ่นเดิมตัดจำหน่าย", user.ID); err != nil {
			return err
		}
		meta.EntityType = "product_return"
		meta.EntityID = &returnID
		meta.Action = "product_return.resolve_case_b"
		meta.After = map[string]any{"replacement_product_id": input.ReplacementProductID}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) Reject(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, returnID string, input RejectInput) error {
	if !platform.HasPermission(user, "returns.manage") {
		return platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์จัดการเคลม")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status, branchID, stockBucket string
		if err := tx.QueryRowContext(ctx, `SELECT status,branch_id::text,stock_bucket FROM product_returns WHERE id = $1 FOR UPDATE`, returnID).Scan(&status, &branchID, &stockBucket); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบรายการคืนสินค้า")
			}
			return err
		}
		if err := validateReturnVisibility(user, branchID, stockBucket); err != nil {
			return err
		}
		if status == "resolved_case_a" || status == "resolved_case_b" || status == "rejected" {
			return platform.NewError(http.StatusConflict, "รายการนี้ถูกปิดแล้ว")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE product_returns SET status = 'rejected', resolved_at = NOW(), updated_at = NOW() WHERE id = $1`, returnID); err != nil {
			return err
		}
		note := strings.TrimSpace(input.Note)
		if note == "" {
			note = "คู่ค้าปฏิเสธเคลม"
		}
		if err := insertEvent(ctx, tx, returnID, "rejected", note, user.ID); err != nil {
			return err
		}
		meta.EntityType = "product_return"
		meta.EntityID = &returnID
		meta.Action = "product_return.reject"
		meta.After = map[string]any{"note": note}
		return s.audit.Log(ctx, tx, meta)
	})
}

// attachClaimRestockLot opens the lot a supplier's replacement arrives on and
// ties it to the movement, so the restock is countable on the shelf and not
// only on the inventory row.
func attachClaimRestockLot(ctx context.Context, tx *sql.Tx, movementID, returnID, branchID, productID, stockBucket string, quantity int) error {
	var unitCost float64
	if err := tx.QueryRowContext(ctx, `SELECT cost_price FROM products WHERE id = $1`, productID).Scan(&unitCost); err != nil {
		return err
	}
	lotID, err := stocklot.Create(ctx, tx, stocklot.Lot{
		BranchID: branchID, ProductID: productID, StockBucket: stockBucket,
		ReceivedQuantity: quantity, RemainingQuantity: quantity, UnitCost: unitCost,
	}, "product_return", &returnID, nil, nil)
	if err != nil {
		return err
	}
	return stocklot.AttachMovement(ctx, tx, movementID, []stocklot.Allocation{{Lot: stocklot.Lot{ID: lotID}, Quantity: quantity}}, 1)
}

func validateReturnVisibility(user platform.AuthUser, branchID, stockBucket string) error {
	if err := platform.EnforceGhostClaimPolicy(user, stockBucket == "ghost"); err != nil {
		return err
	}
	if _, err := platform.MustBranchID(user, branchID); err != nil {
		return err
	}
	return nil
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Initiate(c echo.Context) error {
	var input InitiateInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	id, err := h.service.Initiate(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "ออกสินค้าทดแทนและบันทึกคำขอคืนสินค้าแล้ว"})
}

func (h *Handler) List(c echo.Context) error {
	items, err := h.service.List(c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("status")))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดรายการคืนสินค้าไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) SendToSupplier(c echo.Context) error {
	var input SendToSupplierInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	if err := h.service.SendToSupplier(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("returnID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ส่งเคลมให้คู่ค้าแล้ว")
}

func (h *Handler) ResolveCaseA(c echo.Context) error {
	if err := h.service.ResolveCaseA(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("returnID")); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "รับสินค้ารุ่นเดิมและคืนสต๊อกแล้ว")
}

func (h *Handler) ResolveCaseB(c echo.Context) error {
	var input ResolveCaseBInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	if err := h.service.ResolveCaseB(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("returnID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "รับสินค้ารุ่นทดแทนแล้ว")
}

func (h *Handler) Reject(c echo.Context) error {
	var input RejectInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	if err := h.service.Reject(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("returnID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ปฏิเสธเคลมแล้ว")
}
