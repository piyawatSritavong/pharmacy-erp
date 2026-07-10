package marketplace

import (
	"context"
	"database/sql"
	"encoding/json"
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
	args := []any{}
	if user.BranchID != nil && user.RoleKey != "super_admin" {
		args = append(args, *user.BranchID)
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

func (s *Service) UpsertConnection(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input ConnectionInput) error {
	if user.BranchID != nil && user.RoleKey != "super_admin" && *user.BranchID != input.BranchID {
		return platform.NewError(http.StatusForbidden, "branch scope mismatch")
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
	return platform.JSONMessage(c, http.StatusOK, "marketplace connection saved")
}
