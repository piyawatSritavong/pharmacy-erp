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

// UnitInput is one selling unit of a product. The base unit always converts 1:1
// and is what stock is counted in; larger units (แพ็ค, ลัง) multiply it.
type UnitInput struct {
	ID            string   `json:"id"`
	UnitName      string   `json:"unit_name"`
	ConversionQty int      `json:"conversion_qty"`
	IsBase        bool     `json:"is_base"`
	SellingPrice  *float64 `json:"selling_price"`
	Barcode       string   `json:"barcode"`
	SortOrder     int      `json:"sort_order"`
	Active        *bool    `json:"active"`
}

// attachProductUnits fills each listed product with its selling units. The unit
// price shown is the explicit price when set, otherwise the product's effective
// price multiplied by the conversion, so a ลัง of 12 defaults to 12x the piece.
func attachProductUnits(ctx context.Context, db platform.DBTX, items []map[string]any) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]any, 0, len(items))
	placeholders := make([]string, 0, len(items))
	index := map[string]int{}
	for position, item := range items {
		id, _ := item["id"].(string)
		if id == "" {
			continue
		}
		index[id] = position
		ids = append(ids, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(ids)))
		items[position]["units"] = []map[string]any{}
	}
	if len(ids) == 0 {
		return nil
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id::text, product_id::text, unit_name, conversion_qty, is_base,
		       selling_price, COALESCE(barcode,''), sort_order, active
		FROM product_units
		WHERE product_id IN (%s) AND active = TRUE
		ORDER BY is_base DESC, sort_order, conversion_qty
	`, strings.Join(placeholders, ",")), ids...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, productID, unitName, barcode string
		var conversionQty, sortOrder int
		var isBase, active bool
		var sellingPrice sql.NullFloat64
		if err := rows.Scan(&id, &productID, &unitName, &conversionQty, &isBase, &sellingPrice, &barcode, &sortOrder, &active); err != nil {
			return err
		}
		position, ok := index[productID]
		if !ok {
			continue
		}
		basePrice, _ := items[position]["effective_price"].(float64)
		price := platform.Round2(basePrice * float64(conversionQty))
		priceSource := "derived"
		if sellingPrice.Valid {
			price = platform.Round2(sellingPrice.Float64)
			priceSource = "unit_price"
		}
		unit := map[string]any{
			"id":             id,
			"unit_name":      unitName,
			"conversion_qty": conversionQty,
			"is_base":        isBase,
			"price":          price,
			"price_source":   priceSource,
			"barcode":        barcode,
			"sort_order":     sortOrder,
		}
		current, _ := items[position]["units"].([]map[string]any)
		items[position]["units"] = append(current, unit)
	}
	return rows.Err()
}

func (s *Service) ListUnits(ctx context.Context, productID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, unit_name, conversion_qty, is_base, selling_price,
		       COALESCE(barcode,''), sort_order, active
		FROM product_units
		WHERE product_id = $1
		ORDER BY is_base DESC, sort_order, conversion_qty
	`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, unitName, barcode string
		var conversionQty, sortOrder int
		var isBase, active bool
		var sellingPrice sql.NullFloat64
		if err := rows.Scan(&id, &unitName, &conversionQty, &isBase, &sellingPrice, &barcode, &sortOrder, &active); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id": id, "unit_name": unitName, "conversion_qty": conversionQty,
			"is_base": isBase, "selling_price": nullableFloat(sellingPrice),
			"barcode": barcode, "sort_order": sortOrder, "active": active,
		})
	}
	return items, rows.Err()
}

// validateUnits enforces the invariants stock accounting depends on: exactly one
// base unit, base converts 1:1, names unique, conversions positive.
func validateUnits(units []UnitInput) ([]UnitInput, error) {
	if len(units) == 0 {
		return nil, platform.NewError(http.StatusBadRequest, "ต้องมีหน่วยนับอย่างน้อย 1 หน่วย")
	}
	baseCount := 0
	seen := map[string]bool{}
	cleaned := make([]UnitInput, 0, len(units))
	for _, unit := range units {
		name := strings.TrimSpace(unit.UnitName)
		if name == "" {
			return nil, platform.NewError(http.StatusBadRequest, "ชื่อหน่วยนับห้ามว่าง")
		}
		if seen[name] {
			return nil, platform.NewError(http.StatusBadRequest, fmt.Sprintf("หน่วยนับ %s ซ้ำกัน", name))
		}
		seen[name] = true
		if unit.IsBase {
			baseCount++
			unit.ConversionQty = 1
		}
		if unit.ConversionQty <= 0 {
			return nil, platform.NewError(http.StatusBadRequest, fmt.Sprintf("ตัวคูณของหน่วย %s ต้องมากกว่า 0", name))
		}
		if unit.SellingPrice != nil && *unit.SellingPrice < 0 {
			return nil, platform.NewError(http.StatusBadRequest, fmt.Sprintf("ราคาของหน่วย %s ต้องไม่ติดลบ", name))
		}
		unit.UnitName = name
		unit.Barcode = strings.TrimSpace(unit.Barcode)
		cleaned = append(cleaned, unit)
	}
	if baseCount != 1 {
		return nil, platform.NewError(http.StatusBadRequest, "ต้องกำหนดหน่วยฐานเพียงหน่วยเดียว")
	}
	return cleaned, nil
}

// SaveUnits replaces a product's unit list. Units already referenced by a sold
// document are kept (deactivated instead of deleted) so historical invoices keep
// their snapshot intact.
func (s *Service) SaveUnits(ctx context.Context, productID string, meta audit.LogEntry, units []UnitInput) error {
	cleaned, err := validateUnits(units)
	if err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1)`, productID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
		}

		keep := map[string]bool{}
		for _, unit := range cleaned {
			unitID := strings.TrimSpace(unit.ID)
			active := true
			if unit.Active != nil {
				active = *unit.Active
			}
			if unitID == "" {
				unitID = platform.MustUUID()
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO product_units (id, product_id, unit_name, conversion_qty, is_base,
						selling_price, barcode, sort_order, active, created_at, updated_at)
					VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,NOW(),NOW())
				`, unitID, productID, unit.UnitName, unit.ConversionQty, unit.IsBase,
					platform.NullFloat64(unit.SellingPrice), unit.Barcode, unit.SortOrder, active); err != nil {
					return platform.MapUniqueViolation(err, "ชื่อหน่วยนับหรือบาร์โค้ดซ้ำกับที่มีอยู่")
				}
			} else if _, err := tx.ExecContext(ctx, `
				UPDATE product_units
				SET unit_name=$3, conversion_qty=$4, is_base=$5, selling_price=$6,
				    barcode=NULLIF($7,''), sort_order=$8, active=$9, updated_at=NOW()
				WHERE id=$1 AND product_id=$2
			`, unitID, productID, unit.UnitName, unit.ConversionQty, unit.IsBase,
				platform.NullFloat64(unit.SellingPrice), unit.Barcode, unit.SortOrder, active); err != nil {
				return platform.MapUniqueViolation(err, "ชื่อหน่วยนับหรือบาร์โค้ดซ้ำกับที่มีอยู่")
			}
			keep[unitID] = true
		}

		rows, err := tx.QueryContext(ctx, `SELECT id::text FROM product_units WHERE product_id=$1`, productID)
		if err != nil {
			return err
		}
		existing := []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			existing = append(existing, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, id := range existing {
			if keep[id] {
				continue
			}
			var used bool
			if err := tx.QueryRowContext(ctx, `
				SELECT EXISTS(SELECT 1 FROM invoice_items WHERE unit_id=$1)
				    OR EXISTS(SELECT 1 FROM quotation_items WHERE unit_id=$1)
			`, id).Scan(&used); err != nil {
				return err
			}
			if used {
				if _, err := tx.ExecContext(ctx, `UPDATE product_units SET active=FALSE, updated_at=NOW() WHERE id=$1`, id); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM product_units WHERE id=$1`, id); err != nil {
				return err
			}
		}

		meta.EntityType = "product_units"
		meta.EntityID = &productID
		meta.Action = "product.units.save"
		meta.After = map[string]any{"unit_count": len(cleaned)}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (h *Handler) ListUnits(c echo.Context) error {
	items, err := h.service.ListUnits(c.Request().Context(), c.Param("productID"))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดหน่วยนับไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) SaveUnits(c echo.Context) error {
	var input struct {
		Units []UnitInput `json:"units"`
	}
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	if err := h.service.SaveUnits(c.Request().Context(), c.Param("productID"), audit.MetaFromContext(c), input.Units); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกหน่วยนับแล้ว")
}
