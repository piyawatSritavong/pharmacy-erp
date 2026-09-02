package inventory

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

type CreateTransferRequestInput struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	// Set only when head office raises a requisition on a branch's behalf; a
	// branch POS leaves it empty and the request lands at its own branch.
	DestinationBranchID string `json:"destination_branch_id"`
}

type ReviewTransferRequestInput struct {
	Decision       string `json:"decision"`
	SourceBranchID string `json:"source_branch_id"`
	StockBucket    string `json:"stock_bucket"`
}

func (s *Service) CreateStockTransferRequest(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input CreateTransferRequestInput) (string, error) {
	// Two callers: a branch POS requisitions for its own branch; head office
	// (transfer.approve) requisitions for a branch it names.
	destinationBranchID := ""
	switch {
	case strings.TrimSpace(input.DestinationBranchID) != "":
		if !platform.HasPermission(user, "transfer.approve") {
			return "", platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์สร้างใบเบิกแทนสาขา")
		}
		destinationBranchID = strings.TrimSpace(input.DestinationBranchID)
	case platform.HasPermission(user, "transfer.request.branch") && user.BranchID != nil:
		destinationBranchID = *user.BranchID
	default:
		return "", platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์ส่งคำขอสินค้า")
	}
	if strings.TrimSpace(input.ProductID) == "" || input.Quantity <= 0 {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเลือกสินค้าและระบุจำนวนมากกว่า 0")
	}
	requestID := platform.MustUUID()
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var active bool
		if err := tx.QueryRowContext(ctx, `SELECT active FROM products WHERE id = $1`, input.ProductID).Scan(&active); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
			}
			return err
		}
		if !active {
			return platform.NewError(http.StatusConflict, "สินค้านี้ถูกปิดใช้งาน")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO stock_transfer_requests (
				id, destination_branch_id, product_id, requested_quantity, status,
				requested_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, 'pending', $5, NOW(), NOW())
		`, requestID, destinationBranchID, input.ProductID, input.Quantity, user.ID); err != nil {
			if strings.Contains(err.Error(), "idx_stock_transfer_requests_pending_product") {
				return platform.NewError(http.StatusConflict, "สินค้านี้มีคำขอที่รอตรวจสอบอยู่แล้ว")
			}
			return err
		}
		meta.EntityType = "stock_transfer_request"
		meta.EntityID = &requestID
		meta.Action = "stock_transfer_request.create"
		meta.After = map[string]any{"product_id": input.ProductID, "quantity": input.Quantity, "destination_branch_id": destinationBranchID}
		return s.audit.Log(ctx, tx, meta)
	})
	return requestID, err
}

func (s *Service) ListStockTransferRequests(ctx context.Context, user platform.AuthUser, status string) ([]map[string]any, error) {
	query := `
		SELECT r.id::text, r.destination_branch_id::text, destination.name,
		       r.product_id::text, p.sku, p.name, r.requested_quantity, r.status,
		       COALESCE(r.source_branch_id::text, ''), COALESCE(source.name, ''),
		       COALESCE(r.approved_stock_bucket, ''), COALESCE(r.transfer_id::text, ''),
		       COALESCE(t.transfer_code, ''), COALESCE(t.status, ''),
		       requester.full_name, COALESCE(reviewer.full_name, ''), r.reviewed_at, r.created_at
		FROM stock_transfer_requests r
		INNER JOIN branches destination ON destination.id = r.destination_branch_id
		INNER JOIN products p ON p.id = r.product_id
		INNER JOIN users requester ON requester.id = r.requested_by
		LEFT JOIN branches source ON source.id = r.source_branch_id
		LEFT JOIN transfers t ON t.id = r.transfer_id
		LEFT JOIN users reviewer ON reviewer.id = r.reviewed_by
	`
	args := []any{}
	conditions := []string{}
	if !platform.HasPermission(user, "transfer.approve") {
		if user.BranchID == nil {
			return nil, platform.NewError(http.StatusForbidden, "บัญชีนี้ไม่ได้ผูกกับสาขา")
		}
		args = append(args, *user.BranchID)
		conditions = append(conditions, fmt.Sprintf("r.destination_branch_id = $%d", len(args)))
	}
	if status != "" {
		if status != "pending" && status != "approved" && status != "rejected" {
			return nil, platform.NewError(http.StatusBadRequest, "สถานะคำขอไม่ถูกต้อง")
		}
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("r.status = $%d", len(args)))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY CASE WHEN r.status = 'pending' THEN 0 ELSE 1 END, r.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, destinationID, destinationName, productID, sku, productName, requestStatus string
		var sourceID, sourceName, bucket, transferID, transferCode, transferStatus, requesterName, reviewerName string
		var quantity int
		var reviewedAt sql.NullTime
		var createdAt time.Time
		if err := rows.Scan(
			&id, &destinationID, &destinationName, &productID, &sku, &productName, &quantity, &requestStatus,
			&sourceID, &sourceName, &bucket, &transferID, &transferCode, &transferStatus,
			&requesterName, &reviewerName, &reviewedAt, &createdAt,
		); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id": id, "destination_branch_id": destinationID, "destination_branch_name": destinationName,
			"product_id": productID, "sku": sku, "product_name": productName,
			"requested_quantity": quantity, "status": requestStatus,
			"requested_by_name": requesterName, "reviewed_by_name": reviewerName, "created_at": createdAt,
		}
		if user.RoleKey == "super_admin" {
			item["source_branch_id"] = sourceID
			item["source_branch_name"] = sourceName
			item["approved_stock_bucket"] = bucket
			item["transfer_id"] = transferID
			item["transfer_code"] = transferCode
			item["transfer_status"] = transferStatus
		}
		if reviewedAt.Valid {
			item["reviewed_at"] = reviewedAt.Time
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ReviewStockTransferRequest(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, requestID string, input ReviewTransferRequestInput) (string, error) {
	if !platform.HasPermission(user, "transfer.approve") {
		return "", platform.NewError(http.StatusForbidden, "เฉพาะผู้ดูแลระบบเท่านั้นที่ตรวจสอบคำขอได้")
	}
	if input.Decision != "approve" && input.Decision != "reject" {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเลือกอนุมัติหรือปฏิเสธ")
	}
	if input.Decision == "approve" {
		if strings.TrimSpace(input.SourceBranchID) == "" {
			return "", platform.NewError(http.StatusBadRequest, "กรุณาเลือกสาขาต้นทาง")
		}
		if strings.TrimSpace(input.StockBucket) == "" {
			input.StockBucket = "real"
		}
		if input.StockBucket != "real" {
			return "", platform.NewError(http.StatusBadRequest, "รายการโอนระหว่างสาขาใช้ได้เฉพาะสต๊อกจริง")
		}
	}

	transferID := ""
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var destinationID, productID, status string
		var quantity int
		if err := tx.QueryRowContext(ctx, `
			SELECT destination_branch_id::text, product_id::text, requested_quantity, status
			FROM stock_transfer_requests WHERE id = $1 FOR UPDATE
		`, requestID).Scan(&destinationID, &productID, &quantity, &status); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบคำขอสินค้า")
			}
			return err
		}
		if status != "pending" {
			return platform.NewError(http.StatusConflict, "คำขอนี้ได้รับการตรวจสอบแล้ว")
		}
		if input.Decision == "reject" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE stock_transfer_requests
				SET status = 'rejected', reviewed_by = $2, reviewed_at = NOW(), updated_at = NOW()
				WHERE id = $1
			`, requestID, user.ID); err != nil {
				return err
			}
			meta.EntityType = "stock_transfer_request"
			meta.EntityID = &requestID
			meta.Action = "stock_transfer_request.reject"
			meta.After = map[string]any{"status": "rejected"}
			return s.audit.Log(ctx, tx, meta)
		}
		if input.SourceBranchID == destinationID {
			return platform.NewError(http.StatusBadRequest, "สาขาต้นทางและปลายทางต้องไม่ใช่สาขาเดียวกัน")
		}
		var available int
		if err := tx.QueryRowContext(ctx, `
				SELECT qty_real
			FROM inventory
			WHERE branch_id = $1 AND product_id = $2
			FOR UPDATE
		`, input.SourceBranchID, productID).Scan(&available); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusConflict, "สาขาต้นทางไม่มีสินค้านี้")
			}
			return err
		}
		if available < quantity {
			return platform.NewError(http.StatusConflict, "สต๊อกต้นทางไม่เพียงพอ")
		}

		transferID = platform.MustUUID()
		transferCode := fmt.Sprintf("TRF-%s", strings.ToUpper(transferID[:8]))
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfers (
				id, transfer_code, source_branch_id, destination_branch_id, status,
				request_note, requested_by, requested_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, 'requested', $5, $6, NOW(), NOW(), NOW())
		`, transferID, transferCode, input.SourceBranchID, destinationID, "สร้างจากคำขอ "+requestID, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfer_items (id, transfer_id, product_id, quantity, stock_bucket, created_at)
			VALUES ($1, $2, $3, $4, $5, NOW())
		`, platform.MustUUID(), transferID, productID, quantity, input.StockBucket); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
			VALUES ($1, $2, 'requested', 'สร้างจากคำขอของสาขาปลายทาง', $3, NOW())
		`, platform.MustUUID(), transferID, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE stock_transfer_requests
			SET status = 'approved', source_branch_id = $2, approved_stock_bucket = $3,
			    transfer_id = $4, reviewed_by = $5, reviewed_at = NOW(), updated_at = NOW()
			WHERE id = $1
		`, requestID, input.SourceBranchID, input.StockBucket, transferID, user.ID); err != nil {
			return err
		}
		meta.EntityType = "stock_transfer_request"
		meta.EntityID = &requestID
		meta.Action = "stock_transfer_request.approve"
		meta.After = map[string]any{
			"status": "approved", "transfer_id": transferID, "source_branch_id": input.SourceBranchID,
			"destination_branch_id": destinationID, "stock_bucket": input.StockBucket, "quantity": quantity,
		}
		return s.audit.Log(ctx, tx, meta)
	})
	return transferID, err
}

func (h *Handler) ListTransferRequests(c echo.Context) error {
	items, err := h.service.ListStockTransferRequests(c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("status")))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดคำขอโอนสินค้าไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateTransferRequest(c echo.Context) error {
	var input CreateTransferRequestInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลคำขอไม่ถูกต้อง"))
	}
	id, err := h.service.CreateStockTransferRequest(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "ส่งคำขอโอนสินค้าให้ผู้ดูแลแล้ว"})
}

func (h *Handler) ReviewTransferRequest(c echo.Context) error {
	var input ReviewTransferRequestInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลการตรวจสอบไม่ถูกต้อง"))
	}
	transferID, err := h.service.ReviewStockTransferRequest(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("requestID"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	message := "ปฏิเสธคำขอโอนสินค้าแล้ว"
	if input.Decision == "approve" {
		message = "อนุมัติและสร้างใบโอนสินค้าแล้ว"
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"message": message, "transfer_id": transferID})
}
