package products

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type ProductInput struct {
	SKU              string  `json:"sku"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	CostPrice        float64 `json:"cost_price"`
	BaseSellingPrice float64 `json:"base_selling_price"`
	UnitName         string  `json:"unit_name"`
	TaxExempt        bool    `json:"tax_exempt"`
	Active           *bool   `json:"active"`
}

type AliasInput struct {
	ProductID              string   `json:"product_id"`
	BranchID               *string  `json:"branch_id"`
	AliasCode              string   `json:"alias_code"`
	AliasName              string   `json:"alias_name"`
	DefaultGovernmentPrice *float64 `json:"default_government_price"`
	Active                 *bool    `json:"active"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) List(ctx context.Context, branchID string) ([]map[string]any, error) {
	if branchID == "" {
		rows, err := s.db.QueryContext(ctx, `
			SELECT p.id, p.sku, p.name, p.description, p.cost_price, p.base_selling_price, p.unit_name, p.tax_exempt, p.active, p.base_selling_price
			FROM products p
			ORDER BY p.name
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanProducts(rows)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.sku, p.name, p.description, p.cost_price, p.base_selling_price, p.unit_name, p.tax_exempt, p.active,
		       COALESCE(bpp.selling_price, p.base_selling_price)
		FROM products p
		LEFT JOIN branch_product_prices bpp ON bpp.product_id = p.id AND bpp.branch_id = $1
		ORDER BY p.name
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProducts(rows)
}

func scanProducts(rows *sql.Rows) ([]map[string]any, error) {
	items := []map[string]any{}
	for rows.Next() {
		var id, sku, name, description, unit string
		var costPrice, baseSellingPrice, effectivePrice float64
		var taxExempt, active bool
		if err := rows.Scan(&id, &sku, &name, &description, &costPrice, &baseSellingPrice, &unit, &taxExempt, &active, &effectivePrice); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id":                 id,
			"sku":                sku,
			"name":               name,
			"description":        description,
			"cost_price":         costPrice,
			"base_selling_price": baseSellingPrice,
			"effective_price":    effectivePrice,
			"unit_name":          unit,
			"tax_exempt":         taxExempt,
			"active":             active,
		})
	}
	return items, rows.Err()
}

func (s *Service) ListAliases(ctx context.Context, branchID string, productID string) ([]map[string]any, error) {
	query := `
		SELECT a.id, a.alias_code, a.alias_name, COALESCE(a.default_government_price, 0), a.active,
		       p.id, p.name, COALESCE(b.id::text, ''), COALESCE(b.name, '')
		FROM product_aliases a
		INNER JOIN products p ON p.id = a.product_id
		LEFT JOIN branches b ON b.id = a.branch_id
		WHERE 1 = 1
	`
	args := []any{}
	if productID != "" {
		args = append(args, productID)
		query += " AND a.product_id = $" + strconv.Itoa(len(args))
	}
	if branchID != "" {
		args = append(args, branchID)
		query += fmt.Sprintf(" AND (a.branch_id IS NULL OR a.branch_id = $%d)", len(args))
	}
	query += " ORDER BY a.alias_name ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var id, aliasCode, aliasName, productID, productName, branchID, branchName string
		var defaultGovernmentPrice float64
		var active bool
		if err := rows.Scan(&id, &aliasCode, &aliasName, &defaultGovernmentPrice, &active, &productID, &productName, &branchID, &branchName); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id":                       id,
			"alias_code":               aliasCode,
			"alias_name":               aliasName,
			"default_government_price": defaultGovernmentPrice,
			"active":                   active,
			"product_id":               productID,
			"product_name":             productName,
		}
		if branchID != "" {
			item["branch_id"] = branchID
			item["branch_name"] = branchName
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) CreateProduct(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input ProductInput) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		id := platform.MustUUID()
		active := true
		if input.Active != nil {
			active = *input.Active
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO products (id, sku, name, description, cost_price, base_selling_price, unit_name, tax_exempt, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		`, id, strings.TrimSpace(input.SKU), strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), platform.Round2(input.CostPrice), platform.Round2(input.BaseSellingPrice), strings.TrimSpace(input.UnitName), input.TaxExempt, active)
		if err != nil {
			return err
		}
		entityID := id
		meta.EntityType = "product"
		meta.EntityID = &entityID
		meta.Action = "product.create"
		meta.After = map[string]any{"sku": input.SKU, "name": input.Name}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) UpdateProduct(ctx context.Context, productID string, user platform.AuthUser, meta audit.LogEntry, input ProductInput) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var beforeJSON string
		if err := tx.QueryRowContext(ctx, `SELECT row_to_json(p)::text FROM (SELECT sku, name, base_selling_price, active FROM products WHERE id = $1) p`, productID).Scan(&beforeJSON); err != nil && err != sql.ErrNoRows {
			return err
		}
		active := true
		if input.Active != nil {
			active = *input.Active
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE products
			SET sku = $2, name = $3, description = $4, cost_price = $5, base_selling_price = $6, unit_name = $7, tax_exempt = $8, active = $9, updated_at = NOW()
			WHERE id = $1
		`, productID, strings.TrimSpace(input.SKU), strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), platform.Round2(input.CostPrice), platform.Round2(input.BaseSellingPrice), strings.TrimSpace(input.UnitName), input.TaxExempt, active)
		if err != nil {
			return err
		}
		meta.EntityType = "product"
		meta.EntityID = &productID
		meta.Action = "product.update"
		meta.Before = beforeJSON
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) CreateAlias(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input AliasInput) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		id := platform.MustUUID()
		active := true
		if input.Active != nil {
			active = *input.Active
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO product_aliases (id, product_id, branch_id, alias_code, alias_name, default_government_price, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		`, id, input.ProductID, platform.NullUUID(input.BranchID), strings.TrimSpace(input.AliasCode), strings.TrimSpace(input.AliasName), platform.NullFloat64(input.DefaultGovernmentPrice), active)
		if err != nil {
			return err
		}
		entityID := id
		meta.EntityType = "product_alias"
		meta.EntityID = &entityID
		meta.Action = "product_alias.create"
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

func (h *Handler) List(c echo.Context) error {
	branchID := strings.TrimSpace(c.QueryParam("branch_id"))
	user := platform.CurrentUser(c)
	if branchID == "" && user.BranchID != nil {
		branchID = *user.BranchID
	}
	if user.BranchID != nil && user.RoleKey != "super_admin" && branchID != "" && branchID != *user.BranchID {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusForbidden, "branch scope mismatch"))
	}
	items, err := h.service.List(c.Request().Context(), branchID)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load products", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ListAliases(c echo.Context) error {
	branchID := strings.TrimSpace(c.QueryParam("branch_id"))
	productID := strings.TrimSpace(c.QueryParam("product_id"))
	user := platform.CurrentUser(c)
	if branchID == "" && user.BranchID != nil && user.RoleKey != "super_admin" {
		branchID = *user.BranchID
	}
	if user.BranchID != nil && user.RoleKey != "super_admin" && branchID != "" && branchID != *user.BranchID {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusForbidden, "branch scope mismatch"))
	}
	items, err := h.service.ListAliases(c.Request().Context(), branchID, productID)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to load aliases", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateProduct(c echo.Context) error {
	var input ProductInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.CreateProduct(c.Request().Context(), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to create product", err))
	}
	return platform.JSONMessage(c, http.StatusCreated, "product created")
}

func (h *Handler) UpdateProduct(c echo.Context) error {
	var input ProductInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.UpdateProduct(c.Request().Context(), c.Param("productID"), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to update product", err))
	}
	return platform.JSONMessage(c, http.StatusOK, "product updated")
}

func (h *Handler) CreateAlias(c echo.Context) error {
	var input AliasInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	meta := audit.MetaFromContext(c)
	if err := h.service.CreateAlias(c.Request().Context(), platform.CurrentUser(c), meta, input); err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "failed to create alias", err))
	}
	return platform.JSONMessage(c, http.StatusCreated, "alias created")
}
