package products

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

// PriceTierInput is one tier price of a product: a quantity break open to
// everyone ("10 boxes or more: 95 each") or a price only wholesale customers
// get. The sales checkout picks the lowest tier a line qualifies for.
type PriceTierInput struct {
	UnitID       string  `json:"unit_id"`
	BranchID     string  `json:"branch_id"`
	CustomerTier string  `json:"customer_tier"`
	MinQuantity  int     `json:"min_quantity"`
	UnitPrice    float64 `json:"unit_price"`
}

func (s *Service) ListPriceTiers(ctx context.Context, productID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id::text, COALESCE(t.unit_id::text, ''), COALESCE(u.unit_name, ''),
		       COALESCE(t.branch_id::text, ''), COALESCE(b.name, ''), t.customer_tier, t.min_quantity, t.unit_price
		FROM product_price_tiers t
		LEFT JOIN product_units u ON u.id = t.unit_id
		LEFT JOIN branches b ON b.id = t.branch_id
		WHERE t.product_id = $1 AND t.active
		ORDER BY t.customer_tier, COALESCE(u.conversion_qty, 1), t.min_quantity
	`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, unitID, unitName, branchID, branchName, tier string
		var minQuantity int
		var price float64
		if err := rows.Scan(&id, &unitID, &unitName, &branchID, &branchName, &tier, &minQuantity, &price); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id": id, "unit_id": unitID, "unit_name": unitName, "branch_id": branchID, "branch_name": branchName,
			"customer_tier": tier, "min_quantity": minQuantity, "unit_price": price,
		})
	}
	return items, rows.Err()
}

// SavePriceTiers replaces the product's tier list. Tiers are pricing rules,
// not documents: a bill keeps the price it was sold at (invoice_items), so the
// old rows can simply go.
func (s *Service) SavePriceTiers(ctx context.Context, user platform.AuthUser, productID string, meta audit.LogEntry, tiers []PriceTierInput) error {
	if len(tiers) > 50 {
		return platform.NewError(http.StatusBadRequest, "กำหนดราคาส่งได้ไม่เกิน 50 ระดับต่อสินค้า")
	}
	seen := map[string]bool{}
	for index, tier := range tiers {
		tier.UnitID = strings.TrimSpace(tier.UnitID)
		tier.BranchID = strings.TrimSpace(tier.BranchID)
		tier.CustomerTier = strings.TrimSpace(tier.CustomerTier)
		if tier.CustomerTier == "" {
			tier.CustomerTier = "all"
		}
		if tier.CustomerTier != "all" && tier.CustomerTier != "wholesale" {
			return platform.NewError(http.StatusBadRequest, "กลุ่มลูกค้าของราคาส่งไม่ถูกต้อง")
		}
		if tier.MinQuantity < 1 {
			return platform.NewError(http.StatusBadRequest, "จำนวนขั้นต่ำต้องอย่างน้อย 1")
		}
		if tier.UnitPrice <= 0 {
			return platform.NewError(http.StatusBadRequest, "ราคาต่อหน่วยต้องมากกว่าศูนย์")
		}
		key := fmt.Sprintf("%s|%s|%s|%d", tier.UnitID, tier.BranchID, tier.CustomerTier, tier.MinQuantity)
		if seen[key] {
			return platform.NewError(http.StatusBadRequest, "มีราคาส่งซ้ำกันสำหรับหน่วย สาขา กลุ่มลูกค้า และจำนวนขั้นต่ำเดียวกัน")
		}
		seen[key] = true
		tier.UnitPrice = platform.Round2(tier.UnitPrice)
		tiers[index] = tier
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1)`, productID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM product_price_tiers WHERE product_id=$1`, productID); err != nil {
			return err
		}
		for _, tier := range tiers {
			if tier.UnitID != "" {
				var belongs bool
				if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM product_units WHERE id=$1 AND product_id=$2)`, tier.UnitID, productID).Scan(&belongs); err != nil {
					return err
				}
				if !belongs {
					return platform.NewError(http.StatusBadRequest, "หน่วยนับที่เลือกไม่ใช่ของสินค้านี้")
				}
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO product_price_tiers (id, product_id, unit_id, branch_id, customer_tier, min_quantity, unit_price, active, created_by, created_at, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,TRUE,$8,NOW(),NOW())
			`, platform.MustUUID(), productID, platform.NullUUID(&tier.UnitID), platform.NullUUID(&tier.BranchID),
				tier.CustomerTier, tier.MinQuantity, tier.UnitPrice, user.ID); err != nil {
				return err
			}
		}
		meta.EntityType = "product_price_tiers"
		meta.EntityID = &productID
		meta.Action = "product.price_tiers.save"
		meta.After = map[string]any{"tiers": tiers}
		return s.audit.Log(ctx, tx, meta)
	})
}

// attachPriceTiers lists each product's tier prices for the till, so a
// cashier can see "ราคาส่ง" before the customer asks. branchID narrows
// branch-only tiers to the till's own branch.
func attachPriceTiers(ctx context.Context, db platform.DBTX, items []map[string]any, branchID string) error {
	if len(items) == 0 {
		return nil
	}
	ids := []any{}
	placeholders := []string{}
	index := map[string]int{}
	for position, item := range items {
		id, _ := item["id"].(string)
		if id == "" {
			continue
		}
		index[id] = position
		ids = append(ids, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(ids)))
		items[position]["price_tiers"] = []map[string]any{}
	}
	if len(ids) == 0 {
		return nil
	}
	ids = append(ids, branchID)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT t.product_id::text, COALESCE(t.unit_id::text, ''), COALESCE(u.unit_name, ''), t.customer_tier,
		       t.min_quantity, t.unit_price
		FROM product_price_tiers t
		LEFT JOIN product_units u ON u.id = t.unit_id
		WHERE t.product_id IN (%s) AND t.active
		  AND (t.branch_id IS NULL OR t.branch_id::text = $%d)
		ORDER BY t.customer_tier, COALESCE(u.conversion_qty, 1), t.min_quantity
	`, strings.Join(placeholders, ","), len(ids)), ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var productID, unitID, unitName, tier string
		var minQuantity int
		var price float64
		if err := rows.Scan(&productID, &unitID, &unitName, &tier, &minQuantity, &price); err != nil {
			return err
		}
		position, ok := index[productID]
		if !ok {
			continue
		}
		current, _ := items[position]["price_tiers"].([]map[string]any)
		items[position]["price_tiers"] = append(current, map[string]any{
			"unit_id": unitID, "unit_name": unitName, "customer_tier": tier,
			"min_quantity": minQuantity, "unit_price": price,
		})
	}
	return rows.Err()
}

func (h *Handler) ListPriceTiers(c echo.Context) error {
	items, err := h.service.ListPriceTiers(c.Request().Context(), c.Param("productID"))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดราคาส่งไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) SavePriceTiers(c echo.Context) error {
	var input struct {
		Tiers []PriceTierInput `json:"tiers"`
	}
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	if err := h.service.SavePriceTiers(c.Request().Context(), platform.CurrentUser(c), c.Param("productID"), audit.MetaFromContext(c), input.Tiers); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกราคาส่งแล้ว")
}
