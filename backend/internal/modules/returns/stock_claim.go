package returns

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/stocklot"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

// A claim raised against stock on the shelf rather than against a sale — the
// second case the spec asks of เคลม/คืนสินค้า: "สาขาแต่ละสาขา เคลมหรือคืนสินค้า
// กับบริษัทที่ซื้อขายโดยตรง". It is also the only way a Ghost unit can leave
// inventory outside the month-end close: Ghost arrives on a purchase order, and
// a purchase order is exactly what you claim against when it arrives defective.
type InitiateStockClaimInput struct {
	BranchID    string `json:"branch_id"`
	ProductID   string `json:"product_id"`
	StockBucket string `json:"stock_bucket"`
	Quantity    int    `json:"quantity"`
	Reason      string `json:"reason"`
}

func (s *Service) InitiateStockClaim(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input InitiateStockClaimInput) (string, error) {
	if !platform.HasPermission(user, "returns.manage") {
		return "", platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์จัดการเคลม")
	}
	branchID := strings.TrimSpace(input.BranchID)
	productID := strings.TrimSpace(input.ProductID)
	if branchID == "" || productID == "" || input.Quantity <= 0 {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเลือกสาขา สินค้า และจำนวนที่ถูกต้อง")
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาระบุเหตุผลการเคลม")
	}
	bucket := strings.TrimSpace(input.StockBucket)
	if bucket == "" {
		bucket = "real"
	}
	if bucket != "real" && bucket != "ghost" {
		return "", platform.NewError(http.StatusBadRequest, "ประเภทสต๊อกไม่ถูกต้อง")
	}
	// central_admin and the tills hold real stock and nothing else, so this is
	// where a claim on Ghost is turned away — before it can confirm to the
	// caller that any Ghost quantity exists.
	if err := platform.EnforceGhostClaimPolicy(user, bucket == "ghost"); err != nil {
		return "", err
	}
	if user.BranchID != nil && user.Scope != "global" && *user.BranchID != branchID {
		return "", platform.NewError(http.StatusForbidden, "เคลมได้เฉพาะสต๊อกของสาขาตนเอง")
	}

	returnID := platform.MustUUID()
	if bucket == "ghost" {
		// Ghost is held at the active main warehouse and nowhere else — the
		// database enforces it too, but a 409 explains what a trigger cannot.
		var isWarehouse bool
		if err := s.db.QueryRowContext(ctx, `
			SELECT EXISTS(SELECT 1 FROM branches WHERE id=$1 AND branch_type='main_warehouse' AND active=TRUE)
		`, branchID).Scan(&isWarehouse); err != nil {
			return "", err
		}
		if !isWarehouse {
			return "", platform.NewError(http.StatusConflict, "สต๊อกผีอยู่ที่โกดังใหญ่เท่านั้น")
		}
	}
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		column := "qty_real"
		if bucket == "ghost" {
			column = "qty_ghost"
		}
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
			return platform.NewError(http.StatusConflict, "สต๊อกไม่พอสำหรับจำนวนที่เคลม")
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory SET `+column+` = `+column+` - $3, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, branchID, productID, input.Quantity); err != nil {
			return err
		}
		// Take the goods off actual lots, oldest expiry first, the same way the
		// month-end close does. Skipping this is what lets inventory.qty_* and
		// inventory_lots drift apart, and Ghost coverage is read off inventory
		// while the shelf is counted off lots.
		allocations, err := stocklot.AllocateFEFO(ctx, tx, branchID, productID, bucket, input.Quantity)
		if err != nil {
			return err
		}
		// The movement is what makes the deduction traceable from the stock
		// side: reference_id points back at the claim, so a Ghost quantity that
		// moved can always be explained by the claim that moved it.
		movementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'claim_stock_withdraw', $4, $5, 'product_return', $6, $7, $8, NOW())
		`, movementID, branchID, productID, bucket, -input.Quantity, returnID,
			"ตัดสต๊อกส่งเคลมคู่ค้า: "+reason, user.ID); err != nil {
			return err
		}
		if err := stocklot.AttachMovement(ctx, tx, movementID, allocations, -1); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_returns (id, branch_id, original_invoice_item_id, product_id, stock_bucket, quantity, reason, status, origin, created_by, created_at, updated_at)
			VALUES ($1, $2, NULL, $3, $4, $5, $6, 'pending_claim', 'stock_claim', $7, NOW(), NOW())
		`, returnID, branchID, productID, bucket, input.Quantity, reason, user.ID); err != nil {
			return err
		}
		if err := insertEvent(ctx, tx, returnID, "pending_claim", "ตัดสต๊อกเพื่อส่งเคลมกับคู่ค้า", user.ID); err != nil {
			return err
		}
		meta.EntityType = "product_return"
		meta.EntityID = &returnID
		meta.Action = "product_return.initiate_stock_claim"
		meta.After = map[string]any{
			"branch_id": branchID, "product_id": productID, "stock_bucket": bucket,
			"quantity": input.Quantity, "reason": reason,
			"qty_before": available, "qty_after": available - input.Quantity,
		}
		return s.audit.Log(ctx, tx, meta)
	})
	return returnID, err
}

// Trace answers "where did this claim come from, and what did it do to stock" —
// the claim header, the sale behind it when there is one, every stock movement
// booked against it, and the status trail. The stock movements carry the
// reference the other way round, so the two directions meet.
func (s *Service) Trace(ctx context.Context, user platform.AuthUser, returnID string) (map[string]any, error) {
	if !platform.HasPermission(user, "returns.manage") && !platform.HasPermission(user, "invoice.view") {
		return nil, platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์ดูรายการเคลม")
	}
	var (
		branchID, productID, bucket, status, reason, origin string
		branchName, productName, sku, supplierName          string
		quantity                                            int
		resolutionCase, replacementName, invoiceNumber      sql.NullString
		invoiceItemID                                       sql.NullString
		soldQuantity                                        sql.NullInt64
		claimSentAt, resolvedAt                             sql.NullTime
		createdAt                                           any
	)
	if err := s.db.QueryRowContext(ctx, `
		SELECT pr.branch_id::text, pr.product_id::text, pr.stock_bucket, pr.status, pr.reason, pr.origin,
		       b.name, p.name, p.sku, COALESCE(sup.legal_name, ''), pr.quantity,
		       pr.resolution_case, rp.name, inv.invoice_number, pr.original_invoice_item_id::text, ii.quantity,
		       pr.claim_sent_at, pr.resolved_at, pr.created_at
		FROM product_returns pr
		INNER JOIN branches b ON b.id = pr.branch_id
		INNER JOIN products p ON p.id = pr.product_id
		LEFT JOIN suppliers sup ON sup.id = pr.supplier_id
		LEFT JOIN products rp ON rp.id = pr.replacement_product_id
		LEFT JOIN invoice_items ii ON ii.id = pr.original_invoice_item_id
		LEFT JOIN invoices inv ON inv.id = ii.invoice_id
		WHERE pr.id = $1
	`, returnID).Scan(&branchID, &productID, &bucket, &status, &reason, &origin,
		&branchName, &productName, &sku, &supplierName, &quantity,
		&resolutionCase, &replacementName, &invoiceNumber, &invoiceItemID, &soldQuantity,
		&claimSentAt, &resolvedAt, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบรายการเคลม")
		}
		return nil, err
	}
	// Same rule as the list: a Ghost claim does not exist for anyone but a
	// Superadmin, and a branch sees only its own.
	if bucket == "ghost" && user.RoleKey != "super_admin" {
		return nil, platform.NewError(http.StatusNotFound, "ไม่พบรายการเคลม")
	}
	if user.BranchID != nil && user.Scope != "global" && *user.BranchID != branchID {
		return nil, platform.NewError(http.StatusForbidden, "ดูได้เฉพาะรายการของสาขาตนเอง")
	}

	claim := map[string]any{
		"id": returnID, "origin": origin, "status": status, "reason": reason,
		"quantity": quantity, "branch_name": branchName, "product_name": productName,
		"sku": sku, "supplier_name": supplierName, "created_at": createdAt,
		"resolution_case": resolutionCase.String, "replacement_product_name": replacementName.String,
	}
	if user.RoleKey == "super_admin" {
		claim["stock_bucket"] = bucket
	}
	if invoiceNumber.Valid {
		claim["invoice_number"] = invoiceNumber.String
		claim["invoice_item_id"] = invoiceItemID.String
		claim["sold_quantity"] = soldQuantity.Int64
	}
	if claimSentAt.Valid {
		claim["claim_sent_at"] = claimSentAt.Time
	}
	if resolvedAt.Valid {
		claim["resolved_at"] = resolvedAt.Time
	}

	movements, err := s.traceMovements(ctx, user, returnID)
	if err != nil {
		return nil, err
	}
	events, err := s.traceEvents(ctx, returnID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"claim": claim, "movements": movements, "events": events}, nil
}

func (s *Service) traceMovements(ctx context.Context, user platform.AuthUser, returnID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.movement_type, m.stock_bucket, m.quantity_delta, COALESCE(m.note, ''),
		       p.name, COALESCE(u.full_name, ''), m.created_at
		FROM inventory_movements m
		INNER JOIN products p ON p.id = m.product_id
		LEFT JOIN users u ON u.id = m.performed_by
		WHERE m.reference_type = 'product_return' AND m.reference_id = $1
		ORDER BY m.created_at, p.name
	`, returnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var movementType, movementBucket, note, productName, actor string
		var delta int
		var at any
		if err := rows.Scan(&movementType, &movementBucket, &delta, &note, &productName, &actor, &at); err != nil {
			return nil, err
		}
		entry := map[string]any{
			"movement_type": movementType, "quantity_delta": delta, "note": note,
			"product_name": productName, "performed_by": actor, "created_at": at,
		}
		if user.RoleKey == "super_admin" {
			entry["stock_bucket"] = movementBucket
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *Service) traceEvents(ctx context.Context, returnID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.status, COALESCE(e.note, ''), COALESCE(u.full_name, ''), e.event_at
		FROM return_events e
		LEFT JOIN users u ON u.id = e.actor_id
		WHERE e.return_id = $1
		ORDER BY e.event_at
	`, returnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var status, note, actor string
		var at any
		if err := rows.Scan(&status, &note, &actor, &at); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"status": status, "note": note, "actor": actor, "event_at": at})
	}
	return out, rows.Err()
}

func (h *Handler) InitiateStockClaim(c echo.Context) error {
	var input InitiateStockClaimInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	id, err := h.service.InitiateStockClaim(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "ตัดสต๊อกและเปิดรายการเคลมกับคู่ค้าแล้ว"})
}

func (h *Handler) Trace(c echo.Context) error {
	payload, err := h.service.Trace(c.Request().Context(), platform.CurrentUser(c), c.Param("returnID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, payload)
}
