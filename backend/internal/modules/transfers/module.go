package transfers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/stocklot"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type TransferLine struct {
	ID               string `json:"id,omitempty"`
	ProductID        string `json:"product_id"`
	SKU              string `json:"sku,omitempty"`
	ProductName      string `json:"product_name,omitempty"`
	Quantity         int    `json:"quantity"`
	StockBucket      string `json:"stock_bucket,omitempty"`
	ReceivedQuantity *int   `json:"received_quantity"`
	DiscrepancyNote  string `json:"discrepancy_note"`
}

type CreateTransferRequest struct {
	SourceBranchID      string         `json:"source_branch_id"`
	DestinationBranchID string         `json:"destination_branch_id"`
	RequestNote         string         `json:"request_note"`
	PickupName          string         `json:"pickup_name"`
	CourierName         string         `json:"courier_name"`
	Items               []TransferLine `json:"items"`
}

type DispatchRequest struct {
	PickupName  string `json:"pickup_name"`
	CourierName string `json:"courier_name"`
}

type ReceiveRequest struct {
	Items []ReceiveLineRequest `json:"items"`
}

type ReceiveLineRequest struct {
	ItemID           string `json:"item_id"`
	ReceivedQuantity int    `json:"received_quantity"`
	DiscrepancyNote  string `json:"discrepancy_note"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) List(ctx context.Context, user platform.AuthUser) ([]map[string]any, error) {
	query := `
		SELECT t.id, t.transfer_code, t.source_branch_id::text, t.destination_branch_id::text,
		       sb.name, db2.name, t.status, t.request_note, t.pickup_name, t.courier_name,
		       t.requested_at, t.dispatched_at, t.received_at,
		       COALESCE((
		           SELECT jsonb_agg(jsonb_build_object(
		               'id', ti.id,
		               'product_id', ti.product_id,
		               'sku', p.sku,
		               'product_name', p.name,
		               'quantity', ti.quantity,
		               'stock_bucket', ti.stock_bucket,
		               'received_quantity', ti.received_quantity,
		               'discrepancy_note', ti.discrepancy_note
		           ) ORDER BY p.name, p.sku)
		           FROM transfer_items ti
		           INNER JOIN products p ON p.id = ti.product_id
		           WHERE ti.transfer_id = t.id
		       ), '[]'::jsonb)
		FROM transfers t
		INNER JOIN branches sb ON sb.id = t.source_branch_id
		INNER JOIN branches db2 ON db2.id = t.destination_branch_id
	`
	args := []any{}
	if user.BranchID != nil && !platform.HasPermission(user, "transfer.approve") {
		args = append(args, *user.BranchID)
		query += " WHERE t.source_branch_id = $1 OR t.destination_branch_id = $1"
	}
	query += " ORDER BY t.requested_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, code, sourceBranchID, destinationBranchID string
		var sourceName, destinationName, status, note, pickupName, courierName string
		var requestedAt time.Time
		var dispatchedAt, receivedAt sql.NullTime
		var rawLines []byte
		if err := rows.Scan(&id, &code, &sourceBranchID, &destinationBranchID, &sourceName, &destinationName, &status, &note, &pickupName, &courierName, &requestedAt, &dispatchedAt, &receivedAt, &rawLines); err != nil {
			return nil, err
		}
		var lines []map[string]any
		if err := json.Unmarshal(rawLines, &lines); err != nil {
			return nil, err
		}
		hasDiscrepancy := false
		visibleLines := make([]map[string]any, 0, len(lines))
		for _, line := range lines {
			if user.RoleKey != "super_admin" && line["stock_bucket"] == "ghost" {
				continue
			}
			if user.RoleKey != "super_admin" {
				delete(line, "stock_bucket")
			}
			visibleLines = append(visibleLines, line)
			received, hasReceived := line["received_quantity"].(float64)
			sent, _ := line["quantity"].(float64)
			if hasReceived && received != sent {
				hasDiscrepancy = true
			}
		}
		if user.RoleKey != "super_admin" && len(lines) > 0 && len(visibleLines) == 0 {
			continue
		}
		item := map[string]any{
			"id":                      id,
			"transfer_code":           code,
			"source_branch_id":        sourceBranchID,
			"destination_branch_id":   destinationBranchID,
			"source_branch_name":      sourceName,
			"destination_branch_name": destinationName,
			"status":                  status,
			"request_note":            note,
			"pickup_name":             pickupName,
			"courier_name":            courierName,
			"requested_at":            requestedAt,
			"items":                   visibleLines,
			"item_count":              len(visibleLines),
			"has_discrepancy":         hasDiscrepancy,
		}
		if dispatchedAt.Valid {
			item["dispatched_at"] = dispatchedAt.Time
		}
		if receivedAt.Valid {
			item["received_at"] = receivedAt.Time
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Create(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input CreateTransferRequest) (string, error) {
	if !platform.HasPermission(user, "transfer.request") {
		return "", platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์สร้างรายการโอนสินค้า")
	}
	if input.SourceBranchID == input.DestinationBranchID {
		return "", platform.NewError(http.StatusBadRequest, "source and destination branch must differ")
	}
	if len(input.Items) == 0 {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเพิ่มสินค้าที่ต้องการโอน")
	}
	seenProducts := map[string]bool{}
	for index := range input.Items {
		item := &input.Items[index]
		if strings.TrimSpace(item.ProductID) == "" || item.Quantity <= 0 {
			return "", platform.NewError(http.StatusBadRequest, "สินค้าและจำนวนที่โอนต้องถูกต้อง")
		}
		if strings.TrimSpace(item.StockBucket) == "" {
			item.StockBucket = "real"
		}
		if err := platform.EnforceGhostWritePolicy(user, item.StockBucket == "ghost"); err != nil {
			return "", err
		}
		if item.StockBucket != "real" {
			return "", platform.NewError(http.StatusBadRequest, "รายการโอนระหว่างสาขาใช้ได้เฉพาะสต๊อกจริง")
		}
		key := item.ProductID + ":" + item.StockBucket
		if seenProducts[key] {
			return "", platform.NewError(http.StatusBadRequest, "มีสินค้าและประเภทสต๊อกซ้ำในรายการโอน")
		}
		seenProducts[key] = true
	}

	var transferID string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		transferID = platform.MustUUID()
		transferCode := fmt.Sprintf("TRF-%s", strings.ToUpper(strings.ReplaceAll(transferID[:8], "-", "")))
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfers (id, transfer_code, source_branch_id, destination_branch_id, status, request_note, requested_by, pickup_name, courier_name, requested_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'requested', $5, $6, $7, $8, NOW(), NOW(), NOW())
		`, transferID, transferCode, input.SourceBranchID, input.DestinationBranchID, strings.TrimSpace(input.RequestNote), user.ID, strings.TrimSpace(input.PickupName), strings.TrimSpace(input.CourierName)); err != nil {
			return err
		}
		for _, item := range input.Items {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO transfer_items (id, transfer_id, product_id, quantity, stock_bucket, created_at)
				VALUES ($1, $2, $3, $4, $5, NOW())
			`, platform.MustUUID(), transferID, item.ProductID, item.Quantity, item.StockBucket); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
			VALUES ($1, $2, 'requested', $3, $4, NOW())
		`, platform.MustUUID(), transferID, strings.TrimSpace(input.RequestNote), user.ID); err != nil {
			return err
		}
		meta.EntityType = "transfer"
		meta.EntityID = &transferID
		meta.Action = "transfer.request"
		meta.After = map[string]any{"source_branch_id": input.SourceBranchID, "destination_branch_id": input.DestinationBranchID}
		return s.audit.Log(ctx, tx, meta)
	})
	return transferID, err
}

func (s *Service) Dispatch(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, transferID string, input DispatchRequest) error {
	if !platform.HasPermission(user, "transfer.dispatch") && !platform.HasPermission(user, "transfer.approve") {
		return platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์ยืนยันส่งสินค้า")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var sourceBranchID string
		var status string
		if err := tx.QueryRowContext(ctx, `
			SELECT source_branch_id::text, status
			FROM transfers
			WHERE id = $1
			FOR UPDATE
		`, transferID).Scan(&sourceBranchID, &status); err != nil {
			return err
		}
		if status != "requested" {
			return platform.NewError(http.StatusConflict, "รายการโอนไม่ได้อยู่ในสถานะรอส่ง")
		}

		lines, err := loadTransferLines(ctx, tx, transferID)
		if err != nil {
			return err
		}
		if err := requireVisibleTransferLines(user, lines); err != nil {
			return err
		}
		if err := applyTransferStockOut(ctx, tx, sourceBranchID, user.ID, lines); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE transfers
			SET status = 'in_transit', dispatched_by = $2, dispatched_at = NOW(), pickup_name = $3, courier_name = $4, updated_at = NOW()
			WHERE id = $1
		`, transferID, user.ID, strings.TrimSpace(input.PickupName), strings.TrimSpace(input.CourierName)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
			VALUES ($1, $2, 'in_transit', 'ยืนยันส่งสินค้าแล้ว', $3, NOW())
		`, platform.MustUUID(), transferID, user.ID); err != nil {
			return err
		}
		meta.EntityType = "transfer"
		meta.EntityID = &transferID
		meta.Action = "transfer.dispatch"
		meta.After = map[string]any{"status": "in_transit"}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) Receive(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, transferID string, input ReceiveRequest) error {
	if strings.TrimSpace(transferID) == "" {
		return platform.NewError(http.StatusBadRequest, "ไม่พบรายการโอน")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var destinationBranchID, status string
		if err := tx.QueryRowContext(ctx, `
			SELECT destination_branch_id::text, status
			FROM transfers
			WHERE id = $1
			FOR UPDATE
		`, transferID).Scan(&destinationBranchID, &status); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบรายการโอน")
			}
			return err
		}
		if _, err := platform.MustBranchID(user, destinationBranchID); err != nil {
			return err
		}
		if status != "in_transit" {
			return platform.NewError(http.StatusConflict, "รายการโอนไม่ได้อยู่ในสถานะรอรับ")
		}

		lines, err := loadTransferLines(ctx, tx, transferID)
		if err != nil {
			return err
		}
		if err := requireVisibleTransferLines(user, lines); err != nil {
			return err
		}
		receivedByID := map[string]ReceiveLineRequest{}
		for _, received := range input.Items {
			if strings.TrimSpace(received.ItemID) == "" || received.ReceivedQuantity < 0 {
				return platform.NewError(http.StatusBadRequest, "จำนวนสินค้าที่ได้รับต้องไม่น้อยกว่า 0")
			}
			if _, duplicate := receivedByID[received.ItemID]; duplicate {
				return platform.NewError(http.StatusBadRequest, "มีรายการสินค้าที่รับซ้ำ")
			}
			receivedByID[received.ItemID] = received
		}
		if len(receivedByID) != len(lines) {
			return platform.NewError(http.StatusBadRequest, "กรุณาตรวจสอบและระบุจำนวนรับจริงให้ครบทุกรายการ")
		}
		discrepancyCount := 0
		for _, line := range lines {
			received, ok := receivedByID[line.ID]
			if !ok {
				return platform.NewError(http.StatusBadRequest, "พบรายการสินค้าที่ไม่อยู่ในใบโอน")
			}
			received.DiscrepancyNote = strings.TrimSpace(received.DiscrepancyNote)
			if received.ReceivedQuantity != line.Quantity {
				discrepancyCount++
				if received.DiscrepancyNote == "" {
					return platform.NewError(http.StatusBadRequest, "กรุณาระบุหมายเหตุเมื่อจำนวนรับจริงไม่ตรงกับจำนวนที่ส่ง")
				}
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory (id, branch_id, product_id, qty_real, qty_ghost, created_at, updated_at)
				VALUES ($1, $2, $3, 0, 0, NOW(), NOW())
				ON CONFLICT (branch_id, product_id) DO NOTHING
			`, platform.MustUUID(), destinationBranchID, line.ProductID); err != nil {
				return err
			}
			if line.StockBucket == "real" {
				if _, err := tx.ExecContext(ctx, `
					UPDATE inventory SET qty_real = qty_real + $3, updated_at = NOW()
					WHERE branch_id = $1 AND product_id = $2
				`, destinationBranchID, line.ProductID, received.ReceivedQuantity); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `
					UPDATE inventory SET qty_ghost = qty_ghost + $3, updated_at = NOW()
					WHERE branch_id = $1 AND product_id = $2
				`, destinationBranchID, line.ProductID, received.ReceivedQuantity); err != nil {
					return err
				}
			}
			destinationAllocations := []stocklot.Allocation{}
			remainingToReceive := received.ReceivedQuantity
			type sourceAllocation struct {
				id, sourceLotID, lotNumber string
				dispatched                 int
				expiresOn                  sql.NullTime
				unitCost                   float64
				receivedAt                 time.Time
			}
			sourceAllocations := []sourceAllocation{}
			allocationRows, err := tx.QueryContext(ctx, `
				SELECT tila.id::text,tila.source_lot_id::text,tila.dispatched_quantity,
				       il.lot_number,il.expires_on,il.unit_cost,il.received_at
				FROM transfer_item_lot_allocations tila
				INNER JOIN inventory_lots il ON il.id=tila.source_lot_id
				WHERE tila.transfer_item_id=$1 ORDER BY il.expires_on ASC NULLS LAST,il.received_at,il.id
				FOR UPDATE OF tila
			`, line.ID)
			if err != nil {
				return err
			}
			for allocationRows.Next() {
				var allocation sourceAllocation
				if err := allocationRows.Scan(&allocation.id, &allocation.sourceLotID, &allocation.dispatched, &allocation.lotNumber, &allocation.expiresOn, &allocation.unitCost, &allocation.receivedAt); err != nil {
					allocationRows.Close()
					return err
				}
				sourceAllocations = append(sourceAllocations, allocation)
			}
			if err := allocationRows.Close(); err != nil {
				return err
			}
			// Close the allocation cursor before creating destination lots. A SQL
			// transaction uses one connection and cannot execute nested statements
			// while PostgreSQL is still streaming rows on that connection.
			for _, allocation := range sourceAllocations {
				if remainingToReceive <= 0 {
					break
				}
				quantity := allocation.dispatched
				if quantity > remainingToReceive {
					quantity = remainingToReceive
				}
				destinationLotID, err := stocklot.Create(ctx, tx, stocklot.Lot{BranchID: destinationBranchID, ProductID: line.ProductID, StockBucket: line.StockBucket, LotNumber: allocation.lotNumber, ExpiresOn: allocation.expiresOn, ReceivedQuantity: quantity, RemainingQuantity: quantity, UnitCost: allocation.unitCost, ReceivedAt: time.Now().UTC()}, "transfer", &transferID, &line.ID, &allocation.sourceLotID)
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE transfer_item_lot_allocations SET destination_lot_id=$2,received_quantity=$3,updated_at=NOW() WHERE id=$1`, allocation.id, destinationLotID, quantity); err != nil {
					return err
				}
				destinationAllocations = append(destinationAllocations, stocklot.Allocation{Lot: stocklot.Lot{ID: destinationLotID}, Quantity: quantity})
				remainingToReceive -= quantity
			}
			if remainingToReceive > 0 {
				var cost float64
				if err := tx.QueryRowContext(ctx, `SELECT cost_price FROM products WHERE id=$1`, line.ProductID).Scan(&cost); err != nil {
					return err
				}
				overageLotID, err := stocklot.Create(ctx, tx, stocklot.Lot{BranchID: destinationBranchID, ProductID: line.ProductID, StockBucket: line.StockBucket, ReceivedQuantity: remainingToReceive, RemainingQuantity: remainingToReceive, UnitCost: cost}, "transfer_overage", &transferID, &line.ID, nil)
				if err != nil {
					return err
				}
				destinationAllocations = append(destinationAllocations, stocklot.Allocation{Lot: stocklot.Lot{ID: overageLotID}, Quantity: remainingToReceive})
			}
			if received.ReceivedQuantity > 0 {
				movementID := platform.MustUUID()
				if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
				VALUES ($1, $2, $3, 'transfer_in', $4, $5, 'transfer', $6, $7, $8, NOW())
				`, movementID, destinationBranchID, line.ProductID, line.StockBucket, received.ReceivedQuantity, transferID, received.DiscrepancyNote, user.ID); err != nil {
					return err
				}
				if err := stocklot.AttachMovement(ctx, tx, movementID, destinationAllocations, 1); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE transfer_items
				SET received_quantity = $2, discrepancy_note = $3
				WHERE id = $1 AND transfer_id = $4
			`, line.ID, received.ReceivedQuantity, received.DiscrepancyNote, transferID); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE transfers
			SET status = 'completed', received_by = $2, received_at = NOW(), updated_at = NOW()
			WHERE id = $1
		`, transferID, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
			VALUES ($1, $2, 'completed', $3, $4, NOW())
		`, platform.MustUUID(), transferID, fmt.Sprintf("รับโอนสินค้าแล้ว พบส่วนต่าง %d รายการ", discrepancyCount), user.ID); err != nil {
			return err
		}
		meta.EntityType = "transfer"
		meta.EntityID = &transferID
		meta.Action = "transfer.receive"
		meta.After = map[string]any{"status": "completed", "discrepancy_count": discrepancyCount, "received_items": input.Items}
		return s.audit.Log(ctx, tx, meta)
	})
}

func loadTransferLines(ctx context.Context, db platform.DBTX, transferID string) ([]TransferLine, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT ti.id::text, ti.product_id::text, p.sku, p.name, ti.quantity, ti.stock_bucket,
		       ti.received_quantity, ti.discrepancy_note
		FROM transfer_items ti
		INNER JOIN products p ON p.id = ti.product_id
		WHERE transfer_id = $1
		ORDER BY p.name, p.sku
	`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TransferLine{}
	for rows.Next() {
		var item TransferLine
		if err := rows.Scan(&item.ID, &item.ProductID, &item.SKU, &item.ProductName, &item.Quantity, &item.StockBucket, &item.ReceivedQuantity, &item.DiscrepancyNote); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func requireVisibleTransferLines(user platform.AuthUser, lines []TransferLine) error {
	for _, line := range lines {
		if err := platform.EnforceGhostWritePolicy(user, line.StockBucket == "ghost"); err != nil {
			return err
		}
	}
	return nil
}

func applyTransferStockOut(ctx context.Context, tx *sql.Tx, branchID string, userID string, lines []TransferLine) error {
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].ProductID < lines[j].ProductID
	})
	for _, line := range lines {
		var qtyReal, qtyGhost int
		if err := tx.QueryRowContext(ctx, `
			SELECT qty_real, qty_ghost FROM inventory
			WHERE branch_id = $1 AND product_id = $2
			FOR UPDATE
		`, branchID, line.ProductID).Scan(&qtyReal, &qtyGhost); err != nil {
			return err
		}
		if line.StockBucket == "real" {
			if qtyReal < line.Quantity {
				return platform.NewError(http.StatusConflict, "insufficient real stock for transfer dispatch")
			}
			qtyReal -= line.Quantity
		} else {
			if qtyGhost < line.Quantity {
				return platform.NewError(http.StatusConflict, "insufficient ghost stock for transfer dispatch")
			}
			qtyGhost -= line.Quantity
		}
		allocations, err := stocklot.AllocateFEFO(ctx, tx, branchID, line.ProductID, line.StockBucket, line.Quantity)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory SET qty_real = $3, qty_ghost = $4, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, branchID, line.ProductID, qtyReal, qtyGhost); err != nil {
			return err
		}
		movementID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'transfer_out', $4, $5, 'transfer', '', $6, NOW())
		`, movementID, branchID, line.ProductID, line.StockBucket, -line.Quantity, userID); err != nil {
			return err
		}
		if err := stocklot.AttachMovement(ctx, tx, movementID, allocations, -1); err != nil {
			return err
		}
		for _, allocation := range allocations {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO transfer_item_lot_allocations(
					id,transfer_item_id,source_lot_id,dispatched_quantity,received_quantity,created_at,updated_at
				) VALUES($1,$2,$3,$4,0,NOW(),NOW())
			`, platform.MustUUID(), line.ID, allocation.Lot.ID, allocation.Quantity); err != nil {
				return err
			}
		}
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
	items, err := h.service.List(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load transfers", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Create(c echo.Context) error {
	var input CreateTransferRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	id, err := h.service.Create(c.Request().Context(), platform.CurrentUser(c), meta, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "สร้างรายการโอนสินค้าแล้ว"})
}

func (h *Handler) Dispatch(c echo.Context) error {
	var input DispatchRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Dispatch(c.Request().Context(), platform.CurrentUser(c), meta, c.Param("transferID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ยืนยันส่งสินค้าแล้ว")
}

func (h *Handler) Receive(c echo.Context) error {
	var input ReceiveRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลรับสินค้าไม่ถูกต้อง"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Receive(c.Request().Context(), platform.CurrentUser(c), meta, c.Param("transferID"), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "รับโอนสินค้าแล้ว")
}
