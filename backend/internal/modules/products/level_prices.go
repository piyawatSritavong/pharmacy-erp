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

// LevelPriceInput is a product's own price for one price level (2-5) of one
// selling unit. Level 1 is the shelf price and is set on the product itself.
type LevelPriceInput struct {
	UnitID     string  `json:"unit_id"`
	PriceLevel int     `json:"price_level"`
	UnitPrice  float64 `json:"unit_price"`
}

func (s *Service) ListLevelPrices(ctx context.Context, productID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(lp.unit_id::text, ''), COALESCE(u.unit_name, ''), lp.price_level, lp.unit_price
		FROM product_level_prices lp
		LEFT JOIN product_units u ON u.id = lp.unit_id
		WHERE lp.product_id = $1
		ORDER BY COALESCE(u.conversion_qty, 1), lp.price_level
	`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var unitID, unitName string
		var level int
		var price float64
		if err := rows.Scan(&unitID, &unitName, &level, &price); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"unit_id": unitID, "unit_name": unitName, "price_level": level, "unit_price": price})
	}
	return items, rows.Err()
}

// SaveLevelPrices replaces the product's level prices. Like tier prices they
// are rules, not documents: a bill keeps the price it was sold at.
func (s *Service) SaveLevelPrices(ctx context.Context, user platform.AuthUser, productID string, meta audit.LogEntry, prices []LevelPriceInput) error {
	if len(prices) > 100 {
		return platform.NewError(http.StatusBadRequest, "กำหนดราคาตามระดับได้ไม่เกิน 100 รายการต่อสินค้า")
	}
	seen := map[string]bool{}
	for index, price := range prices {
		price.UnitID = strings.TrimSpace(price.UnitID)
		if price.PriceLevel < 2 || price.PriceLevel > 5 {
			return platform.NewError(http.StatusBadRequest, "ระดับราคาที่กำหนดต่อสินค้าต้องเป็นระดับ 2-5 (ระดับ 1 คือราคาขายปกติ)")
		}
		if price.UnitPrice <= 0 {
			return platform.NewError(http.StatusBadRequest, "ราคาต่อหน่วยต้องมากกว่าศูนย์")
		}
		key := fmt.Sprintf("%s|%d", price.UnitID, price.PriceLevel)
		if seen[key] {
			return platform.NewError(http.StatusBadRequest, "มีราคาระดับเดียวกันของหน่วยเดียวกันซ้ำ")
		}
		seen[key] = true
		price.UnitPrice = platform.Round2(price.UnitPrice)
		prices[index] = price
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1)`, productID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
		}
		before, err := s.ListLevelPrices(ctx, productID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM product_level_prices WHERE product_id=$1`, productID); err != nil {
			return err
		}
		for _, price := range prices {
			if price.UnitID != "" {
				var belongs bool
				if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM product_units WHERE id=$1 AND product_id=$2)`, price.UnitID, productID).Scan(&belongs); err != nil {
					return err
				}
				if !belongs {
					return platform.NewError(http.StatusBadRequest, "หน่วยนับที่เลือกไม่ใช่ของสินค้านี้")
				}
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO product_level_prices (id, product_id, unit_id, price_level, unit_price, updated_by, created_at, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,NOW(),NOW())
			`, platform.MustUUID(), productID, platform.NullUUID(&price.UnitID), price.PriceLevel, price.UnitPrice, user.ID); err != nil {
				return err
			}
		}
		meta.EntityType = "product_level_prices"
		meta.EntityID = &productID
		meta.Action = "product.level_prices.save"
		meta.Before = map[string]any{"prices": before}
		meta.After = map[string]any{"prices": prices}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (h *Handler) ListLevelPrices(c echo.Context) error {
	items, err := h.service.ListLevelPrices(c.Request().Context(), c.Param("productID"))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดราคาตามระดับไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) SaveLevelPrices(c echo.Context) error {
	var input struct {
		Prices []LevelPriceInput `json:"prices"`
	}
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	if err := h.service.SaveLevelPrices(c.Request().Context(), platform.CurrentUser(c), c.Param("productID"), audit.MetaFromContext(c), input.Prices); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกราคาตามระดับแล้ว")
}
