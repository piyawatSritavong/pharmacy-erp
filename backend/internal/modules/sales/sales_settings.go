package sales

import (
	"context"
	"database/sql"
	"net/http"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

// SalesSettings are the shop-wide pricing switches the till reads.
type SalesSettings struct {
	PricesIncludeVAT bool    `json:"prices_include_vat"`
	VATRate          float64 `json:"vat_rate"`
}

func (s *Service) GetSalesSettings(ctx context.Context) (SalesSettings, error) {
	inclusive, err := pricesIncludeVAT(ctx, s.db)
	if err != nil {
		return SalesSettings{}, err
	}
	rate, err := platform.GetSettingFloat(ctx, s.db, "vat_rate", 7)
	if err != nil {
		return SalesSettings{}, err
	}
	return SalesSettings{PricesIncludeVAT: inclusive, VATRate: rate}, nil
}

// SaveSalesSettings switches VAT-inclusive pricing. The VAT rate itself is not
// edited here.
func (s *Service) SaveSalesSettings(ctx context.Context, meta audit.LogEntry, input SalesSettings) error {
	before, err := s.GetSalesSettings(ctx)
	if err != nil {
		return err
	}
	value := "false"
	if input.PricesIncludeVAT {
		value = "true"
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_settings (setting_key, setting_value, metadata, created_at, updated_at)
			VALUES ('prices_include_vat', $1, '{"label": "ราคาขายรวม VAT แล้ว", "type": "boolean"}'::jsonb, NOW(), NOW())
			ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value, updated_at = NOW()
		`, value); err != nil {
			return err
		}
		meta.EntityType = "settings"
		meta.Action = "sales.settings.update"
		meta.Before = map[string]any{"prices_include_vat": before.PricesIncludeVAT}
		meta.After = map[string]any{"prices_include_vat": input.PricesIncludeVAT}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (h *Handler) GetSalesSettings(c echo.Context) error {
	settings, err := h.service.GetSalesSettings(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดการตั้งค่าการขายไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, settings)
}

func (h *Handler) SaveSalesSettings(c echo.Context) error {
	var input SalesSettings
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	if err := h.service.SaveSalesSettings(c.Request().Context(), audit.MetaFromContext(c), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกการตั้งค่าการขายแล้ว")
}
