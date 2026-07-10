package transfers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type TransferLine struct {
	ProductID   string `json:"product_id"`
	Quantity    int    `json:"quantity"`
	StockBucket string `json:"stock_bucket"`
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
	TransferCode string `json:"transfer_code"`
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
		SELECT t.id, t.transfer_code, t.qr_code, t.source_branch_id::text, t.destination_branch_id::text,
		       sb.name, db2.name, t.status, t.request_note, t.pickup_name, t.courier_name, t.requested_at
		FROM transfers t
		INNER JOIN branches sb ON sb.id = t.source_branch_id
		INNER JOIN branches db2 ON db2.id = t.destination_branch_id
	`
	args := []any{}
	if user.BranchID != nil && user.RoleKey != "super_admin" {
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
		var id, code, qrCode, sourceBranchID, destinationBranchID string
		var sourceName, destinationName, status, note, pickupName, courierName string
		var requestedAt time.Time
		if err := rows.Scan(&id, &code, &qrCode, &sourceBranchID, &destinationBranchID, &sourceName, &destinationName, &status, &note, &pickupName, &courierName, &requestedAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":                      id,
			"transfer_code":           code,
			"qr_code":                 qrCode,
			"source_branch_id":        sourceBranchID,
			"destination_branch_id":   destinationBranchID,
			"source_branch_name":      sourceName,
			"destination_branch_name": destinationName,
			"status":                  status,
			"request_note":            note,
			"pickup_name":             pickupName,
			"courier_name":            courierName,
			"requested_at":            requestedAt,
		})
	}
	return items, rows.Err()
}

func (s *Service) Create(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input CreateTransferRequest) (string, error) {
	if user.BranchID != nil && user.RoleKey != "super_admin" && *user.BranchID != input.DestinationBranchID && *user.BranchID != input.SourceBranchID {
		return "", platform.NewError(http.StatusForbidden, "one side of the transfer must match your branch")
	}
	if input.SourceBranchID == input.DestinationBranchID {
		return "", platform.NewError(http.StatusBadRequest, "source and destination branch must differ")
	}
	if len(input.Items) == 0 {
		return "", platform.NewError(http.StatusBadRequest, "transfer items are required")
	}

	var transferID string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		transferID = platform.MustUUID()
		transferCode := fmt.Sprintf("TRF-%s", strings.ToUpper(strings.ReplaceAll(transferID[:8], "-", "")))
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfers (id, transfer_code, source_branch_id, destination_branch_id, status, request_note, requested_by, pickup_name, courier_name, qr_code, requested_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'requested', $5, $6, $7, $8, $2, NOW(), NOW(), NOW())
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
		if user.BranchID != nil && user.RoleKey != "super_admin" && *user.BranchID != sourceBranchID {
			return platform.NewError(http.StatusForbidden, "only source branch can dispatch")
		}
		if status != "requested" {
			return platform.NewError(http.StatusConflict, "transfer is not in requested state")
		}

		lines, err := loadTransferLines(ctx, tx, transferID)
		if err != nil {
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
			VALUES ($1, $2, 'in_transit', 'Transfer dispatched', $3, NOW())
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

func (s *Service) Receive(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, transferID string, transferCode string) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		query := `
			SELECT id::text, destination_branch_id::text, status
			FROM transfers
			WHERE 1 = 1
		`
		args := []any{}
		if transferID != "" {
			args = append(args, transferID)
			query += " AND id = $" + strconv.Itoa(len(args))
		}
		if transferCode != "" {
			args = append(args, transferCode)
			query += " AND transfer_code = $" + strconv.Itoa(len(args))
		}
		query += " FOR UPDATE"

		var id, destinationBranchID, status string
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&id, &destinationBranchID, &status); err != nil {
			return err
		}
		if user.BranchID != nil && user.RoleKey != "super_admin" && *user.BranchID != destinationBranchID {
			return platform.NewError(http.StatusForbidden, "only destination branch can receive")
		}
		if status != "in_transit" {
			return platform.NewError(http.StatusConflict, "transfer is not in transit")
		}

		lines, err := loadTransferLines(ctx, tx, id)
		if err != nil {
			return err
		}
		for _, line := range lines {
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
				`, destinationBranchID, line.ProductID, line.Quantity); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `
					UPDATE inventory SET qty_ghost = qty_ghost + $3, updated_at = NOW()
					WHERE branch_id = $1 AND product_id = $2
				`, destinationBranchID, line.ProductID, line.Quantity); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
				VALUES ($1, $2, $3, 'transfer_in', $4, $5, 'transfer', $6, '', $7, NOW())
			`, platform.MustUUID(), destinationBranchID, line.ProductID, line.StockBucket, line.Quantity, id, user.ID); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE transfers
			SET status = 'completed', received_by = $2, received_at = NOW(), updated_at = NOW()
			WHERE id = $1
		`, id, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
			VALUES ($1, $2, 'completed', 'Transfer received', $3, NOW())
		`, platform.MustUUID(), id, user.ID); err != nil {
			return err
		}
		meta.EntityType = "transfer"
		meta.EntityID = &id
		meta.Action = "transfer.receive"
		meta.After = map[string]any{"status": "completed"}
		return s.audit.Log(ctx, tx, meta)
	})
}

func loadTransferLines(ctx context.Context, db platform.DBTX, transferID string) ([]TransferLine, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT product_id::text, quantity, stock_bucket
		FROM transfer_items
		WHERE transfer_id = $1
		ORDER BY product_id
	`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TransferLine{}
	for rows.Next() {
		var item TransferLine
		if err := rows.Scan(&item.ProductID, &item.Quantity, &item.StockBucket); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
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
		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory SET qty_real = $3, qty_ghost = $4, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, branchID, line.ProductID, qtyReal, qtyGhost); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'transfer_out', $4, $5, 'transfer', '', $6, NOW())
		`, platform.MustUUID(), branchID, line.ProductID, line.StockBucket, -line.Quantity, userID); err != nil {
			return err
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
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "transfer requested"})
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
	return platform.JSONMessage(c, http.StatusOK, "transfer dispatched")
}

func (h *Handler) Receive(c echo.Context) error {
	var input ReceiveRequest
	_ = c.Bind(&input)
	meta := audit.MetaFromContext(c)
	if err := h.service.Receive(c.Request().Context(), platform.CurrentUser(c), meta, c.Param("transferID"), strings.TrimSpace(input.TransferCode)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "transfer received")
}

func (h *Handler) ReceiveByCode(c echo.Context) error {
	var input ReceiveRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Receive(c.Request().Context(), platform.CurrentUser(c), meta, "", strings.TrimSpace(input.TransferCode)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "transfer received")
}
