package marketplace

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type ConnectionInput struct {
	ProviderID     string         `json:"provider_id"`
	BranchID       string         `json:"branch_id"`
	ConnectionName string         `json:"connection_name"`
	Credentials    map[string]any `json:"credentials"`
	Settings       map[string]any `json:"settings"`
	Status         string         `json:"status"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) ListProviders(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, provider_key, name, description, active FROM marketplace_providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, providerKey, name, description string
		var active bool
		if err := rows.Scan(&id, &providerKey, &name, &description, &active); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":           id,
			"provider_key": providerKey,
			"name":         name,
			"description":  description,
			"active":       active,
		})
	}
	return items, rows.Err()
}

func (s *Service) ListOrders(ctx context.Context, user platform.AuthUser) ([]map[string]any, error) {
	query := `
		SELECT mo.id, mp.name, mo.external_order_id, mo.status, mo.customer_name, mo.order_total, b.name, mo.placed_at
		FROM marketplace_orders mo
		INNER JOIN marketplace_providers mp ON mp.id = mo.provider_id
		INNER JOIN branches b ON b.id = mo.branch_id
	`
	own, err := platform.BranchFilter(user, "")
	if err != nil {
		return nil, err
	}
	args := []any{}
	if own != "" {
		args = append(args, own)
		query += " WHERE mo.branch_id = $1"
	}
	query += " ORDER BY mo.placed_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, providerName, externalOrderID, status, customerName, branchName string
		var orderTotal float64
		var placedAt time.Time
		if err := rows.Scan(&id, &providerName, &externalOrderID, &status, &customerName, &orderTotal, &branchName, &placedAt); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":                id,
			"provider_name":     providerName,
			"external_order_id": externalOrderID,
			"status":            status,
			"customer_name":     customerName,
			"order_total":       orderTotal,
			"branch_name":       branchName,
			"placed_at":         placedAt,
		})
	}
	return items, rows.Err()
}

// TestConnection checks whatever is currently in the connection form (not
// necessarily saved yet) — D13's "test connection" action. There is no real
// external marketplace API to call from this system, so this is honestly a
// structural completeness check (required fields present) rather than a
// live network call, and says so in its own message.
func (s *Service) TestConnection(ctx context.Context, input ConnectionInput) (map[string]any, error) {
	var providerName string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM marketplace_providers WHERE id = $1`, input.ProviderID).Scan(&providerName); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusBadRequest, "ไม่พบผู้ให้บริการตลาดออนไลน์ที่เลือก")
		}
		return nil, err
	}
	var branchName string
	var onlineSalesEnabled bool
	if err := s.db.QueryRowContext(ctx, `SELECT name, online_sales_enabled FROM branches WHERE id = $1`, input.BranchID).Scan(&branchName, &onlineSalesEnabled); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusBadRequest, "ไม่พบสาขาที่เลือก")
		}
		return nil, err
	}
	// Part B, Rule 3: only branches with online_sales_enabled may sell
	// online — in-store POS is unaffected and open to every branch.
	if !onlineSalesEnabled {
		return map[string]any{
			"success": false,
			"message": fmt.Sprintf("สาขา%sไม่สามารถขายออนไลน์ได้", branchName),
		}, nil
	}
	missing := []string{}
	if strings.TrimSpace(input.ConnectionName) == "" {
		missing = append(missing, "ชื่อการเชื่อมต่อ")
	}
	apiKey, _ := input.Credentials["api_key"].(string)
	if strings.TrimSpace(apiKey) == "" {
		missing = append(missing, "คีย์ API")
	}
	if len(missing) > 0 {
		return map[string]any{
			"success": false,
			"message": fmt.Sprintf("ข้อมูลไม่ครบถ้วน: %s", strings.Join(missing, ", ")),
		}, nil
	}
	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("ข้อมูลการเชื่อมต่อ %s สาขา%s ครบถ้วนพร้อมบันทึก (ระบบตรวจสอบเฉพาะความครบถ้วนของข้อมูล ไม่ได้เชื่อมต่อ API จริงของผู้ให้บริการ)", providerName, branchName),
	}, nil
}

func (s *Service) UpsertConnection(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input ConnectionInput) error {
	branchID, err := platform.MustBranchID(user, input.BranchID)
	if err != nil {
		return err
	}
	input.BranchID = branchID
	// Part B, Rule 3: only branches with online_sales_enabled may sell
	// online — enforced server-side, not just filtered out of the picker,
	// since a direct request should be rejected the same way any other
	// direct-URL/API bypass attempt is throughout this app. In-store POS
	// checkout does not go through this check at all.
	var onlineSalesEnabled bool
	if err := s.db.QueryRowContext(ctx, `SELECT online_sales_enabled FROM branches WHERE id = $1`, input.BranchID).Scan(&onlineSalesEnabled); err != nil {
		if err == sql.ErrNoRows {
			return platform.NewError(http.StatusBadRequest, "ไม่พบสาขาที่เลือก")
		}
		return err
	}
	if !onlineSalesEnabled {
		return platform.NewError(http.StatusBadRequest, "สาขานี้ไม่สามารถขายออนไลน์ได้")
	}
	credentials, _ := json.Marshal(input.Credentials)
	settings, _ := json.Marshal(input.Settings)
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var existingID string
		err := tx.QueryRowContext(ctx, `
			SELECT id::text
			FROM marketplace_connections
			WHERE provider_id = $1 AND branch_id = $2
		`, input.ProviderID, input.BranchID).Scan(&existingID)
		switch {
		case err == sql.ErrNoRows:
			existingID = platform.MustUUID()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO marketplace_connections (id, provider_id, branch_id, connection_name, credentials_json, settings_json, status, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8, NOW(), NOW())
			`, existingID, input.ProviderID, input.BranchID, strings.TrimSpace(input.ConnectionName), string(credentials), string(settings), input.Status, user.ID); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if _, err := tx.ExecContext(ctx, `
				UPDATE marketplace_connections
				SET connection_name = $2, credentials_json = $3::jsonb, settings_json = $4::jsonb, status = $5, updated_at = NOW()
				WHERE id = $1
			`, existingID, strings.TrimSpace(input.ConnectionName), string(credentials), string(settings), input.Status); err != nil {
				return err
			}
		}
		meta.EntityType = "marketplace_connection"
		meta.EntityID = &existingID
		meta.Action = "marketplace.upsert_connection"
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListProviders(c echo.Context) error {
	items, err := h.service.ListProviders(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load marketplace providers", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ListOrders(c echo.Context) error {
	items, err := h.service.ListOrders(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load marketplace orders", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) UpsertConnection(c echo.Context) error {
	var input ConnectionInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.UpsertConnection(c.Request().Context(), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกการเชื่อมต่อตลาดออนไลน์แล้ว")
}

func (h *Handler) TestConnection(c echo.Context) error {
	var input ConnectionInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	result, err := h.service.TestConnection(c.Request().Context(), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}
