package inventory

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type RebalanceRequest struct {
	ProductID  string `json:"product_id"`
	BranchID   string `json:"branch_id"`
	Quantity   int    `json:"quantity"`
	FromBucket string `json:"from_bucket"`
	ToBucket   string `json:"to_bucket"`
	Reason     string `json:"reason"`
}

type AdjustRequest struct {
	ProductID     string `json:"product_id"`
	BranchID      string `json:"branch_id"`
	StockBucket   string `json:"stock_bucket"`
	QuantityDelta int    `json:"quantity_delta"`
	Reason        string `json:"reason"`
}

type ReceiveRequest struct {
	ProductID     string `json:"product_id"`
	BranchID      string `json:"branch_id"`
	RealQuantity  int    `json:"real_quantity"`
	GhostQuantity int    `json:"ghost_quantity"`
	Note          string `json:"note"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) List(ctx context.Context, user platform.AuthUser, branchID string) ([]map[string]any, error) {
	if branchID == "" && user.BranchID != nil {
		branchID = *user.BranchID
	}
	if err := validateBranchScope(user, branchID); err != nil {
		return nil, err
	}
	if branchID == "" {
		rows, err := s.db.QueryContext(ctx, `
			SELECT i.id, i.branch_id::text, b.name, p.id::text, p.sku, p.name, i.qty_real, i.qty_ghost, COALESCE(bpp.selling_price, p.base_selling_price)
			FROM inventory i
			INNER JOIN branches b ON b.id = i.branch_id
			INNER JOIN products p ON p.id = i.product_id
			LEFT JOIN branch_product_prices bpp ON bpp.product_id = p.id AND bpp.branch_id = i.branch_id
			ORDER BY b.name, p.name
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanInventory(rows)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id, i.branch_id::text, b.name, p.id::text, p.sku, p.name, i.qty_real, i.qty_ghost, COALESCE(bpp.selling_price, p.base_selling_price)
		FROM inventory i
		INNER JOIN branches b ON b.id = i.branch_id
		INNER JOIN products p ON p.id = i.product_id
		LEFT JOIN branch_product_prices bpp ON bpp.product_id = p.id AND bpp.branch_id = i.branch_id
		WHERE i.branch_id = $1
		ORDER BY p.name
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInventory(rows)
}

func scanInventory(rows *sql.Rows) ([]map[string]any, error) {
	items := []map[string]any{}
	for rows.Next() {
		var id, branchID, branchName, productID, sku, productName string
		var qtyReal, qtyGhost int
		var price float64
		if err := rows.Scan(&id, &branchID, &branchName, &productID, &sku, &productName, &qtyReal, &qtyGhost, &price); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":           id,
			"branch_id":    branchID,
			"branch_name":  branchName,
			"product_id":   productID,
			"sku":          sku,
			"product_name": productName,
			"qty_real":     qtyReal,
			"qty_ghost":    qtyGhost,
			"price":        price,
		})
	}
	return items, rows.Err()
}

func (s *Service) Rebalance(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input RebalanceRequest) error {
	if input.Quantity <= 0 || input.FromBucket == input.ToBucket {
		return platform.NewError(http.StatusBadRequest, "invalid rebalance request")
	}
	if strings.TrimSpace(input.BranchID) == "" {
		return platform.NewError(http.StatusBadRequest, "branch_id is required")
	}
	if err := validateBranchScope(user, input.BranchID); err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var qtyReal, qtyGhost int
		if err := tx.QueryRowContext(ctx, `
			SELECT qty_real, qty_ghost FROM inventory
			WHERE branch_id = $1 AND product_id = $2
			FOR UPDATE
		`, input.BranchID, input.ProductID).Scan(&qtyReal, &qtyGhost); err != nil {
			return err
		}

		before := map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost}
		var err error
		qtyReal, qtyGhost, err = applyRebalanceResult(qtyReal, qtyGhost, input.Quantity, input.FromBucket)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory
			SET qty_real = $3, qty_ghost = $4, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, input.BranchID, input.ProductID, qtyReal, qtyGhost); err != nil {
			return err
		}

		for _, movement := range []struct {
			Bucket string
			Delta  int
		}{
			{input.FromBucket, -input.Quantity},
			{input.ToBucket, input.Quantity},
		} {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, note, performed_by, created_at)
				VALUES ($1, $2, $3, 'rebalance', $4, $5, 'inventory.rebalance', $6, $7, NOW())
			`, platform.MustUUID(), input.BranchID, input.ProductID, movement.Bucket, movement.Delta, input.Reason, user.ID); err != nil {
				return err
			}
		}

		entityID := input.ProductID
		meta.EntityType = "inventory"
		meta.EntityID = &entityID
		meta.Action = "inventory.rebalance"
		meta.Before = before
		meta.After = map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost, "from_bucket": input.FromBucket, "to_bucket": input.ToBucket, "quantity": input.Quantity}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) Adjust(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input AdjustRequest) error {
	if input.QuantityDelta == 0 {
		return platform.NewError(http.StatusBadRequest, "quantity_delta must not be zero")
	}
	if input.StockBucket != "real" && input.StockBucket != "ghost" {
		return platform.NewError(http.StatusBadRequest, "invalid stock bucket")
	}
	if strings.TrimSpace(input.BranchID) == "" {
		return platform.NewError(http.StatusBadRequest, "branch_id is required")
	}
	if err := validateBranchScope(user, input.BranchID); err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var qtyReal, qtyGhost int
		if err := tx.QueryRowContext(ctx, `
			SELECT qty_real, qty_ghost FROM inventory
			WHERE branch_id = $1 AND product_id = $2
			FOR UPDATE
		`, input.BranchID, input.ProductID).Scan(&qtyReal, &qtyGhost); err != nil {
			return err
		}

		before := map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost}
		var err error
		qtyReal, qtyGhost, err = applyAdjustmentResult(qtyReal, qtyGhost, input.StockBucket, input.QuantityDelta)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory
			SET qty_real = $3, qty_ghost = $4, updated_at = NOW()
			WHERE branch_id = $1 AND product_id = $2
		`, input.BranchID, input.ProductID, qtyReal, qtyGhost); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, note, performed_by, created_at)
			VALUES ($1, $2, $3, 'manual_adjust', $4, $5, 'inventory.adjust', $6, $7, NOW())
		`, platform.MustUUID(), input.BranchID, input.ProductID, input.StockBucket, input.QuantityDelta, input.Reason, user.ID); err != nil {
			return err
		}

		entityID := input.ProductID
		meta.EntityType = "inventory"
		meta.EntityID = &entityID
		meta.Action = "inventory.adjust"
		meta.Before = before
		meta.After = map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost, "stock_bucket": input.StockBucket, "quantity_delta": input.QuantityDelta}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) Receive(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input ReceiveRequest) error {
	if input.RealQuantity < 0 || input.GhostQuantity < 0 {
		return platform.NewError(http.StatusBadRequest, "quantities must not be negative")
	}
	if input.RealQuantity == 0 && input.GhostQuantity == 0 {
		return platform.NewError(http.StatusBadRequest, "at least one of real_quantity or ghost_quantity is required")
	}
	if strings.TrimSpace(input.BranchID) == "" {
		return platform.NewError(http.StatusBadRequest, "branch_id is required")
	}
	if err := validateBranchScope(user, input.BranchID); err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var productExists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM products WHERE id = $1 AND active = TRUE)`, input.ProductID).Scan(&productExists); err != nil {
			return err
		}
		if !productExists {
			return platform.NewError(http.StatusNotFound, "product not found")
		}

		var qtyReal, qtyGhost int
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO inventory (id, branch_id, product_id, qty_real, qty_ghost, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			ON CONFLICT (branch_id, product_id) DO UPDATE
			SET qty_real = inventory.qty_real + EXCLUDED.qty_real,
			    qty_ghost = inventory.qty_ghost + EXCLUDED.qty_ghost,
			    updated_at = NOW()
			RETURNING qty_real, qty_ghost
		`, platform.MustUUID(), input.BranchID, input.ProductID, input.RealQuantity, input.GhostQuantity).Scan(&qtyReal, &qtyGhost); err != nil {
			return err
		}

		note := strings.TrimSpace(input.Note)
		for _, movement := range []struct {
			Bucket string
			Delta  int
		}{
			{"real", input.RealQuantity},
			{"ghost", input.GhostQuantity},
		} {
			if movement.Delta == 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, note, performed_by, created_at)
				VALUES ($1, $2, $3, 'receive', $4, $5, 'inventory.receive', $6, $7, NOW())
			`, platform.MustUUID(), input.BranchID, input.ProductID, movement.Bucket, movement.Delta, note, user.ID); err != nil {
				return err
			}
		}

		entityID := input.ProductID
		meta.EntityType = "inventory"
		meta.EntityID = &entityID
		meta.Action = "inventory.receive"
		meta.After = map[string]any{"qty_real": qtyReal, "qty_ghost": qtyGhost, "received_real": input.RealQuantity, "received_ghost": input.GhostQuantity}
		return s.audit.Log(ctx, tx, meta)
	})
}

func applyRebalanceResult(qtyReal, qtyGhost, quantity int, fromBucket string) (int, int, error) {
	if fromBucket == "real" {
		if qtyReal < quantity {
			return 0, 0, platform.NewError(http.StatusConflict, "insufficient real stock")
		}
		return qtyReal - quantity, qtyGhost + quantity, nil
	}
	if qtyGhost < quantity {
		return 0, 0, platform.NewError(http.StatusConflict, "insufficient ghost stock")
	}
	return qtyReal + quantity, qtyGhost - quantity, nil
}

func applyAdjustmentResult(qtyReal, qtyGhost int, stockBucket string, quantityDelta int) (int, int, error) {
	if stockBucket == "real" {
		if qtyReal+quantityDelta < 0 {
			return 0, 0, platform.NewError(http.StatusConflict, "real stock would become negative")
		}
		return qtyReal + quantityDelta, qtyGhost, nil
	}
	if qtyGhost+quantityDelta < 0 {
		return 0, 0, platform.NewError(http.StatusConflict, "ghost stock would become negative")
	}
	return qtyReal, qtyGhost + quantityDelta, nil
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
	items, err := h.service.List(c.Request().Context(), platform.CurrentUser(c), strings.TrimSpace(c.QueryParam("branch_id")))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load inventory", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Rebalance(c echo.Context) error {
	var input RebalanceRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Rebalance(c.Request().Context(), user, meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "inventory rebalanced")
}

func (h *Handler) Receive(c echo.Context) error {
	var input ReceiveRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Receive(c.Request().Context(), user, meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "stock received")
}

func (h *Handler) Adjust(c echo.Context) error {
	var input AdjustRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.Adjust(c.Request().Context(), user, meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "inventory adjusted")
}
