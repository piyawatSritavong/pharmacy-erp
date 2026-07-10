package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"your-module/internal/middleware"
	"your-module/internal/models"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// ============================================================================
// STOCK TRANSFER: Real <-> Ghost
// ============================================================================

// TransferStockRequest handles moving stock between real and ghost inventories
type TransferStockRequest struct {
	BranchID  uuid.UUID `json:"branch_id" validate:"required"`
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	FromType  string    `json:"from_type" validate:"required,oneof=real ghost"`
	ToType    string    `json:"to_type" validate:"required,oneof=real ghost"`
	Quantity  int       `json:"quantity" validate:"required,min=1"`
	Reason    string    `json:"reason" validate:"required"`
}

type TransferStockResponse struct {
	ID            uuid.UUID              `json:"id"`
	FromType      string                 `json:"from_type"`
	ToType        string                 `json:"to_type"`
	Quantity      int                    `json:"quantity"`
	Reason        string                 `json:"reason"`
	Status        string                 `json:"status"`
	InitiatedAt   time.Time              `json:"initiated_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
}

// HandleTransferStock transfers stock between real and ghost inventories
func (h *Handler) HandleTransferStock(c echo.Context) error {
	user := c.Get("user").(*middleware.UserClaims)

	req := new(TransferStockRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}
	if err := c.Validate(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Begin transaction for atomic operation
	tx, err := h.db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database error"})
	}
	defer tx.Rollback()

	// 1. Validate current stock in source inventory
	var currentStock int
	sourceTable := "inventory_real"
	if req.FromType == "ghost" {
		sourceTable = "inventory_ghost"
	}

	err = tx.QueryRowContext(
		c.Request().Context(),
		"SELECT quantity_in_stock FROM "+sourceTable+" WHERE branch_id = $1 AND product_id = $2",
		req.BranchID, req.ProductID,
	).Scan(&currentStock)

	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Inventory not found"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database error"})
	}

	if currentStock < req.Quantity {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Insufficient stock",
			"available": string(rune(currentStock)),
			"requested": string(rune(req.Quantity)),
		})
	}

	// 2. Debit from source
	_, err = tx.ExecContext(
		c.Request().Context(),
		"UPDATE "+sourceTable+" SET quantity_in_stock = quantity_in_stock - $1, updated_at = NOW() WHERE branch_id = $2 AND product_id = $3",
		req.Quantity, req.BranchID, req.ProductID,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to debit stock"})
	}

	// 3. Credit to destination
	destTable := "inventory_real"
	if req.ToType == "ghost" {
		destTable = "inventory_ghost"
	}

	_, err = tx.ExecContext(
		c.Request().Context(),
		"UPDATE "+destTable+" SET quantity_in_stock = quantity_in_stock + $1, updated_at = NOW() WHERE branch_id = $2 AND product_id = $3",
		req.Quantity, req.BranchID, req.ProductID,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to credit stock"})
	}

	// 4. Record the transfer
	transferID := uuid.New()
	now := time.Now()
	_, err = tx.ExecContext(
		c.Request().Context(),
		`INSERT INTO stock_transfers
		 (id, branch_id, product_id, from_type, to_type, quantity, reason, transfer_status, initiated_by, initiated_at, completed_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		transferID, req.BranchID, req.ProductID, req.FromType, req.ToType,
		req.Quantity, req.Reason, "completed", user.ID, now, now, now,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to record transfer"})
	}

	// 5. Log the audit
	auditID := uuid.New()
	_, err = tx.ExecContext(
		c.Request().Context(),
		`INSERT INTO audit_logs
		 (id, entity_type, entity_id, action, user_id, user_role, changes, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		auditID, "stock_transfer", transferID, "transfer", user.ID, user.Role,
		`{"from_type":""`+req.FromType+`"","to_type":""`+req.ToType+`"","quantity":"`+string(rune(req.Quantity))+`"}`,
		req.Reason, now,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to log audit"})
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to commit"})
	}

	return c.JSON(http.StatusCreated, TransferStockResponse{
		ID:          transferID,
		FromType:    req.FromType,
		ToType:      req.ToType,
		Quantity:    req.Quantity,
		Reason:      req.Reason,
		Status:      "completed",
		InitiatedAt: now,
		CompletedAt: &now,
	})
}

// ============================================================================
// GET INVENTORY: Role-based filtering
// ============================================================================

type InventoryItem struct {
	ProductID    uuid.UUID `json:"product_id"`
	ProductName  string    `json:"product_name"`
	SKU          string    `json:"sku"`
	RealStock    int       `json:"real_stock,omitempty"`
	GhostStock   int       `json:"ghost_stock,omitempty"`
	TotalStock   int       `json:"total_stock,omitempty"`
	ReorderLevel int       `json:"reorder_level,omitempty"`
	CostPerUnit  float64   `json:"cost_per_unit,omitempty"`
}

// HandleGetInventory returns inventory filtered by user role
func (h *Handler) HandleGetInventory(c echo.Context) error {
	user := c.Get("user").(*middleware.UserClaims)
	branchID := c.Param("branch_id")

	// Manager role: only real stock
	// Admin role: both real and ghost stock
	var query string
	var args []interface{}

	if user.Role == "manager" || user.Role == "branch_admin" {
		query = `
			SELECT ir.product_id, p.name, p.sku, ir.quantity_in_stock as real_stock, 0 as ghost_stock,
			       ir.quantity_in_stock as total_stock, ir.reorder_level, ir.cost_per_unit
			FROM inventory_real ir
			JOIN products p ON ir.product_id = p.id
			WHERE ir.branch_id = $1
			ORDER BY p.name
		`
		args = []interface{}{branchID}
	} else if user.Role == "superadmin" {
		query = `
			SELECT
				COALESCE(ir.product_id, ig.product_id) as product_id,
				p.name,
				p.sku,
				COALESCE(ir.quantity_in_stock, 0) as real_stock,
				COALESCE(ig.quantity_in_stock, 0) as ghost_stock,
				COALESCE(ir.quantity_in_stock, 0) + COALESCE(ig.quantity_in_stock, 0) as total_stock,
				ir.reorder_level,
				ir.cost_per_unit
			FROM products p
			FULL OUTER JOIN inventory_real ir ON p.id = ir.product_id AND ir.branch_id = $1
			FULL OUTER JOIN inventory_ghost ig ON p.id = ig.product_id AND ig.branch_id = $1
			WHERE ir.branch_id = $1 OR ig.branch_id = $1
			ORDER BY p.name
		`
		args = []interface{}{branchID}
	} else {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Insufficient permissions"})
	}

	rows, err := h.db.QueryContext(c.Request().Context(), query, args...)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database error"})
	}
	defer rows.Close()

	var items []InventoryItem
	for rows.Next() {
		var item InventoryItem
		if err := rows.Scan(&item.ProductID, &item.ProductName, &item.SKU, &item.RealStock,
			&item.GhostStock, &item.TotalStock, &item.ReorderLevel, &item.CostPerUnit); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Scan error"})
		}
		items = append(items, item)
	}

	return c.JSON(http.StatusOK, items)
}

// ============================================================================
// PRICE ADJUSTMENT
// ============================================================================

type PriceAdjustmentRequest struct {
	ProductID            uuid.UUID `json:"product_id" validate:"required"`
	BasePrice            float64   `json:"base_price" validate:"required,min=0"`
	AdjustedPrice        float64   `json:"adjusted_price" validate:"required,min=0"`
	AdjustmentReason     string    `json:"adjustment_reason" validate:"required"`
}

type PriceAdjustmentResponse struct {
	ID                   uuid.UUID `json:"id"`
	ProductID            uuid.UUID `json:"product_id"`
	BasePrice            float64   `json:"base_price"`
	AdjustedPrice        float64   `json:"adjusted_price"`
	DiscountPercentage   float64   `json:"discount_percentage"`
	AdjustmentReason     string    `json:"adjustment_reason"`
	AdjustedBy           string    `json:"adjusted_by"`
	CreatedAt            time.Time `json:"created_at"`
}

// HandleRecordPriceAdjustment records a price override in the system
func (h *Handler) HandleRecordPriceAdjustment(c echo.Context) error {
	user := c.Get("user").(*middleware.UserClaims)
	invoiceID := c.Param("invoice_id")

	req := new(PriceAdjustmentRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}
	if err := c.Validate(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Only allow if base_price > adjusted_price (can't mark price UP without special permission)
	if req.AdjustedPrice > req.BasePrice && user.Role != "superadmin" {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Only superadmin can increase prices",
		})
	}

	id := uuid.New()
	now := time.Now()
	discountPct := ((req.BasePrice - req.AdjustedPrice) / req.BasePrice) * 100

	_, err := h.db.ExecContext(
		c.Request().Context(),
		`INSERT INTO price_adjustments
		 (id, invoice_id, product_id, base_price, adjusted_price, adjustment_reason, adjusted_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, invoiceID, req.ProductID, req.BasePrice, req.AdjustedPrice, req.AdjustmentReason, user.ID, now,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to record adjustment"})
	}

	// Audit log
	auditID := uuid.New()
	_, _ = h.db.ExecContext(
		c.Request().Context(),
		`INSERT INTO audit_logs
		 (id, entity_type, entity_id, action, user_id, user_role, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		auditID, "price_adjustment", id, "price_override", user.ID, user.Role, req.AdjustmentReason, now,
	)

	return c.JSON(http.StatusCreated, PriceAdjustmentResponse{
		ID:                   id,
		ProductID:            req.ProductID,
		BasePrice:            req.BasePrice,
		AdjustedPrice:        req.AdjustedPrice,
		DiscountPercentage:   discountPct,
		AdjustmentReason:     req.AdjustmentReason,
		AdjustedBy:           user.ID.String(),
		CreatedAt:            now,
	})
}
