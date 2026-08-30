package products

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

const maxProductImageSize = 5 << 20

type ProductInput struct {
	SKU                    string  `json:"sku"`
	Barcode                string  `json:"barcode"`
	CategoryID             *string `json:"category_id"`
	Name                   string  `json:"name"`
	Description            string  `json:"description"`
	CostPrice              float64 `json:"cost_price"`
	BaseSellingPrice       float64 `json:"base_selling_price"`
	MaxDiscountAmount      float64 `json:"max_discount_amount"`
	LowStockRealThreshold  int     `json:"low_stock_real_threshold"`
	LowStockGhostThreshold int     `json:"low_stock_ghost_threshold"`
	TracksExpiry           bool    `json:"tracks_expiry"`
	ExpiryWarningDays      int     `json:"expiry_warning_days"`
	UnitName               string  `json:"unit_name"`
	TaxExempt              bool    `json:"tax_exempt"`
	Active                 *bool   `json:"active"`
	// SalesChannel is Part B Rule 2's catalog facet — "in_store", "online",
	// or "both". FDA fields are Part B Rule 1 — RequiresFDAReport flags a
	// product into the อย. report picker; FDARegistrationNo is optional even
	// then (may not be assigned yet).
	SalesChannel      string `json:"sales_channel"`
	RequiresFDAReport bool   `json:"requires_fda_report"`
	FDARegistrationNo string `json:"fda_registration_no"`
}

type AliasInput struct {
	ProductID              string   `json:"product_id"`
	BranchID               *string  `json:"branch_id"`
	AliasCode              string   `json:"alias_code"`
	AliasName              string   `json:"alias_name"`
	DefaultGovernmentPrice *float64 `json:"default_government_price"`
	Active                 *bool    `json:"active"`
}

type CategoryInput struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	Active *bool  `json:"active"`
}

type BranchSettingsInput struct {
	SellingPrice           OptionalFloat `json:"selling_price"`
	MaxDiscountAmount      *float64      `json:"max_discount_amount"`
	LowStockRealThreshold  *int          `json:"low_stock_real_threshold"`
	LowStockGhostThreshold *int          `json:"low_stock_ghost_threshold"`
}

// OptionalFloat distinguishes an omitted property from an explicit JSON null.
// For selling_price, null means remove the branch override and inherit WH price.
type OptionalFloat struct {
	Present bool
	Value   *float64
}

func (value *OptionalFloat) UnmarshalJSON(data []byte) error {
	value.Present = true
	if string(data) == "null" {
		value.Value = nil
		return nil
	}
	var parsed float64
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	value.Value = &parsed
	return nil
}

type ListFilter struct {
	Search            string
	CategoryID        string
	Active            string
	SalesChannel      string
	RequiresFDAReport string
	Page              int
	PageSize          int
}

type ListResult struct {
	Items      []map[string]any
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

type Service struct {
	db        *sql.DB
	audit     *audit.Service
	uploadDir string
}

func NewService(db *sql.DB, auditService *audit.Service, uploadDir string) *Service {
	return &Service{db: db, audit: auditService, uploadDir: uploadDir}
}

func (s *Service) List(ctx context.Context, user platform.AuthUser, branchID string, filter ListFilter) (ListResult, error) {
	priceExpression := "p.base_selling_price"
	joinPrice := ""
	args := []any{}
	if branchID != "" {
		priceExpression = "COALESCE(bpp.selling_price, p.base_selling_price)"
		joinPrice = `
			INNER JOIN inventory branch_inventory
			  ON branch_inventory.product_id = p.id AND branch_inventory.branch_id = $1
			LEFT JOIN branch_product_prices bpp
			  ON bpp.product_id = p.id AND bpp.branch_id = $1
		`
		args = append(args, branchID)
	}
	conditions := []string{}
	if keyword := strings.TrimSpace(filter.Search); keyword != "" {
		args = append(args, "%"+strings.ToLower(keyword)+"%")
		placeholder := "$" + strconv.Itoa(len(args))
		conditions = append(conditions, fmt.Sprintf("(LOWER(p.name) LIKE %[1]s OR LOWER(p.sku) LIKE %[1]s OR LOWER(COALESCE(p.barcode, '')) LIKE %[1]s OR EXISTS (SELECT 1 FROM product_source_aliases psa WHERE psa.product_id=p.id AND LOWER(psa.alias_name) LIKE %[1]s))", placeholder))
	}
	if strings.TrimSpace(filter.CategoryID) != "" {
		args = append(args, strings.TrimSpace(filter.CategoryID))
		conditions = append(conditions, "p.category_id = $"+strconv.Itoa(len(args)))
	}
	if filter.Active == "true" || filter.Active == "false" {
		args = append(args, filter.Active == "true")
		conditions = append(conditions, "p.active = $"+strconv.Itoa(len(args)))
	}
	if filter.SalesChannel == "in_store" || filter.SalesChannel == "online" || filter.SalesChannel == "both" {
		args = append(args, filter.SalesChannel)
		conditions = append(conditions, "p.sales_channel = $"+strconv.Itoa(len(args)))
	}
	if filter.RequiresFDAReport == "true" || filter.RequiresFDAReport == "false" {
		args = append(args, filter.RequiresFDAReport == "true")
		conditions = append(conditions, "p.requires_fda_report = $"+strconv.Itoa(len(args)))
	}
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countArgs := args
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM products p
		%s
		LEFT JOIN product_categories c ON c.id = p.category_id
		%s
	`, joinPrice, whereClause)
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return ListResult{}, err
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	limitClause := ""
	if pageSize > 0 {
		if pageSize > 200 {
			pageSize = 200
		}
		args = append(args, pageSize, (page-1)*pageSize)
		limitClause = fmt.Sprintf("LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}

	ghostThresholdExpression := "p.low_stock_ghost_threshold"
	if user.RoleKey != "super_admin" {
		ghostThresholdExpression = "0"
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
			SELECT p.id, p.sku, COALESCE(p.barcode, ''), p.name, p.description,
		       p.cost_price, p.base_selling_price,
		       p.max_discount_amount, p.low_stock_real_threshold,
		       %s, p.tracks_expiry, p.expiry_warning_days,
		       p.unit_name, p.tax_exempt, p.active, %s,
		       COALESCE(c.id::text, ''), COALESCE(c.name, 'ไม่มีหมวดหมู่'),
		       COALESCE(c.color, '#D71920'), COALESCE(p.image_storage_key, ''),
		       (SELECT COUNT(*) FROM product_images pi WHERE pi.product_id=p.id),
		       p.sales_channel, p.requires_fda_report, COALESCE(p.fda_registration_no, '')
		FROM products p
			%s
			LEFT JOIN product_categories c ON c.id = p.category_id
			%s
			ORDER BY p.active DESC, p.name
			%s
			`, ghostThresholdExpression, priceExpression, joinPrice, whereClause, limitClause), args...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()
	items, err := scanProducts(rows, user)
	if err != nil {
		return ListResult{}, err
	}
	totalPages := 1
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}
	} else {
		pageSize = total
	}
	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func scanProducts(rows *sql.Rows, user platform.AuthUser) ([]map[string]any, error) {
	items := []map[string]any{}
	for rows.Next() {
		var id, sku, barcode, name, description, unit, categoryID, categoryName, categoryColor, imageKey string
		var costPrice, baseSellingPrice, effectivePrice, maxDiscountAmount float64
		var lowStockRealThreshold, lowStockGhostThreshold, expiryWarningDays, imageCount int
		var tracksExpiry bool
		var taxExempt, active bool
		var salesChannel, fdaRegistrationNo string
		var requiresFDAReport bool
		if err := rows.Scan(
			&id, &sku, &barcode, &name, &description, &costPrice, &baseSellingPrice,
			&maxDiscountAmount, &lowStockRealThreshold, &lowStockGhostThreshold,
			&tracksExpiry, &expiryWarningDays, &unit, &taxExempt, &active, &effectivePrice,
			&categoryID, &categoryName, &categoryColor, &imageKey, &imageCount,
			&salesChannel, &requiresFDAReport, &fdaRegistrationNo,
		); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id": id, "sku": sku, "barcode": barcode, "name": name, "description": description,
			"low_stock_real_threshold": lowStockRealThreshold,
			"tracks_expiry":            tracksExpiry,
			"expiry_warning_days":      expiryWarningDays,
			"effective_price":          effectivePrice, "unit_name": unit, "tax_exempt": taxExempt,
			"active": active, "category_name": categoryName, "category_color": categoryColor,
			"image_available":     imageKey != "",
			"image_count":         imageCount,
			"sales_channel":       salesChannel,
			"requires_fda_report": requiresFDAReport,
			"fda_registration_no": fdaRegistrationNo,
		}
		if user.Portal != "pos" && user.RoleKey != "branch_pos" {
			item["cost_price"] = costPrice
			item["base_selling_price"] = baseSellingPrice
			item["max_discount_amount"] = maxDiscountAmount
		}
		if user.RoleKey == "super_admin" {
			item["low_stock_ghost_threshold"] = lowStockGhostThreshold
		}
		if categoryID != "" {
			item["category_id"] = categoryID
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ListCategories(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id::text, c.name, c.color, c.active, COUNT(p.id)
		FROM product_categories c
		LEFT JOIN products p ON p.category_id = c.id
		GROUP BY c.id
		ORDER BY c.active DESC, c.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, color string
		var active bool
		var productCount int
		if err := rows.Scan(&id, &name, &color, &active, &productCount); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id": id, "name": name, "color": color, "active": active, "product_count": productCount,
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
	query += " ORDER BY a.active DESC, a.alias_name"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, aliasCode, aliasName, resolvedProductID, productName, resolvedBranchID, branchName string
		var defaultGovernmentPrice float64
		var active bool
		if err := rows.Scan(&id, &aliasCode, &aliasName, &defaultGovernmentPrice, &active, &resolvedProductID, &productName, &resolvedBranchID, &branchName); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id": id, "alias_code": aliasCode, "alias_name": aliasName,
			"default_government_price": defaultGovernmentPrice, "active": active,
			"product_id": resolvedProductID, "product_name": productName,
		}
		if resolvedBranchID != "" {
			item["branch_id"] = resolvedBranchID
			item["branch_name"] = branchName
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func validateProduct(input ProductInput) error {
	if strings.TrimSpace(input.SKU) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.UnitName) == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณากรอก SKU ชื่อสินค้า และหน่วยนับ")
	}
	if input.CostPrice < 0 || input.BaseSellingPrice < 0 || input.MaxDiscountAmount < 0 {
		return platform.NewError(http.StatusBadRequest, "ราคาสินค้าต้องไม่ติดลบ")
	}
	if input.LowStockRealThreshold < 0 || input.LowStockGhostThreshold < 0 {
		return platform.NewError(http.StatusBadRequest, "จำนวนแจ้งเตือนใกล้หมดต้องไม่ติดลบ")
	}
	if input.ExpiryWarningDays == 0 {
		input.ExpiryWarningDays = 30
	}
	if input.ExpiryWarningDays < 0 || input.ExpiryWarningDays > 3650 {
		return platform.NewError(http.StatusBadRequest, "จำนวนวันแจ้งเตือนหมดอายุไม่ถูกต้อง")
	}
	if input.SalesChannel != "" && input.SalesChannel != "in_store" && input.SalesChannel != "online" && input.SalesChannel != "both" {
		return platform.NewError(http.StatusBadRequest, "ช่องทางขายไม่ถูกต้อง")
	}
	return nil
}

func normalizedSalesChannel(value string) string {
	if value == "" {
		return "in_store"
	}
	return value
}

func normalizedExpiryWarningDays(value int) int {
	if value == 0 {
		return 30
	}
	return value
}

func (s *Service) CreateProduct(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input ProductInput) (string, error) {
	if strings.TrimSpace(input.SKU) == "" {
		input.SKU = platform.GenerateReadableCode("PRD")
	}
	if err := validateProduct(input); err != nil {
		return "", err
	}
	if user.RoleKey != "super_admin" && input.LowStockGhostThreshold != 0 {
		return "", platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์กำหนดค่าสต๊อกผี")
	}
	id := platform.MustUUID()
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO products (
				id, sku, barcode, category_id, name, description, cost_price,
				base_selling_price, unit_name,
				max_discount_amount, low_stock_real_threshold, low_stock_ghost_threshold,
				tracks_expiry, expiry_warning_days, tax_exempt, active,
				sales_channel, requires_fda_report, fda_registration_no, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW(), NOW())
		`, id, strings.TrimSpace(input.SKU), platform.NullString(input.Barcode),
			platform.NullUUID(input.CategoryID), strings.TrimSpace(input.Name),
			strings.TrimSpace(input.Description), platform.Round2(input.CostPrice),
			platform.Round2(input.BaseSellingPrice),
			strings.TrimSpace(input.UnitName),
			platform.Round2(input.MaxDiscountAmount), input.LowStockRealThreshold,
			input.LowStockGhostThreshold, input.TracksExpiry, normalizedExpiryWarningDays(input.ExpiryWarningDays),
			input.TaxExempt, active,
			normalizedSalesChannel(input.SalesChannel), input.RequiresFDAReport, platform.NullString(input.FDARegistrationNo))
		if err != nil {
			return platform.MapUniqueViolation(err, "SKU หรือบาร์โค้ดนี้มีอยู่แล้ว")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory(id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at)
			SELECT $1,b.id,$2,0,0,NOW(),NOW()
			FROM branches b
			WHERE b.branch_type='main_warehouse' AND b.active=TRUE
			ON CONFLICT(branch_id,product_id) DO NOTHING
		`, platform.MustUUID(), id); err != nil {
			return err
		}
		meta.EntityType = "product"
		meta.EntityID = &id
		meta.Action = "product.create"
		meta.After = map[string]any{"sku": input.SKU, "name": input.Name}
		return s.audit.Log(ctx, tx, meta)
	})
	return id, err
}

func (s *Service) UpdateProduct(ctx context.Context, productID string, user platform.AuthUser, meta audit.LogEntry, input ProductInput) error {
	if err := validateProduct(input); err != nil {
		return err
	}
	if user.RoleKey != "super_admin" && input.LowStockGhostThreshold != 0 {
		return platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์กำหนดค่าสต๊อกผี")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var beforeJSON string
		if err := tx.QueryRowContext(ctx, `
			SELECT row_to_json(p)::text FROM (
				SELECT sku, barcode, name, base_selling_price, active FROM products WHERE id = $1
			) p
		`, productID).Scan(&beforeJSON); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
			}
			return err
		}
		active := true
		if input.Active != nil {
			active = *input.Active
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE products
			SET sku = $2, barcode = $3, category_id = $4, name = $5, description = $6,
			    cost_price = $7, base_selling_price = $8,
			    unit_name = $9, max_discount_amount = $10,
			    low_stock_real_threshold = $11,
			    low_stock_ghost_threshold = CASE WHEN $20 THEN $12 ELSE low_stock_ghost_threshold END,
			    tracks_expiry = $13, expiry_warning_days = $14, tax_exempt = $15,
			    active = $16, sales_channel = $17, requires_fda_report = $18,
			    fda_registration_no = $19, updated_at = NOW()
			WHERE id = $1
		`, productID, strings.TrimSpace(input.SKU), platform.NullString(input.Barcode),
			platform.NullUUID(input.CategoryID), strings.TrimSpace(input.Name),
			strings.TrimSpace(input.Description), platform.Round2(input.CostPrice),
			platform.Round2(input.BaseSellingPrice),
			strings.TrimSpace(input.UnitName),
			platform.Round2(input.MaxDiscountAmount), input.LowStockRealThreshold,
			input.LowStockGhostThreshold, input.TracksExpiry, normalizedExpiryWarningDays(input.ExpiryWarningDays),
			input.TaxExempt, active,
			normalizedSalesChannel(input.SalesChannel), input.RequiresFDAReport, platform.NullString(input.FDARegistrationNo), user.RoleKey == "super_admin")
		if err != nil {
			return platform.MapUniqueViolation(err, "SKU หรือบาร์โค้ดนี้มีอยู่แล้ว")
		}
		meta.EntityType = "product"
		meta.EntityID = &productID
		meta.Action = "product.update"
		meta.Before = beforeJSON
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) GetBranchSettings(ctx context.Context, user platform.AuthUser, productID, branchID string) (map[string]any, error) {
	var sellingPrice, maxDiscount sql.NullFloat64
	var warehousePrice float64
	var realThreshold, ghostThreshold sql.NullInt64
	ghostExpression := "bps.low_stock_ghost_threshold"
	if user.RoleKey != "super_admin" {
		ghostExpression = "NULL::integer"
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT bpp.selling_price,p.base_selling_price,bps.max_discount_amount,bps.low_stock_real_threshold,`+ghostExpression+`
		FROM products p CROSS JOIN branches b
		LEFT JOIN branch_product_prices bpp ON bpp.product_id=p.id AND bpp.branch_id=b.id
		LEFT JOIN branch_product_settings bps ON bps.product_id=p.id AND bps.branch_id=b.id
		WHERE p.id=$1 AND b.id=$2
	`, productID, branchID).Scan(&sellingPrice, &warehousePrice, &maxDiscount, &realThreshold, &ghostThreshold)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบสินค้าหรือสาขา")
		}
		return nil, err
	}
	result := map[string]any{
		"product_id": productID, "branch_id": branchID,
		"selling_price":            nullableFloat(sellingPrice),
		"warehouse_price":          warehousePrice,
		"effective_selling_price":  effectiveFloat(sellingPrice, warehousePrice),
		"selling_price_source":     priceSource(sellingPrice),
		"max_discount_amount":      nullableFloat(maxDiscount),
		"low_stock_real_threshold": nullableInt(realThreshold),
	}
	if user.RoleKey == "super_admin" {
		result["low_stock_ghost_threshold"] = nullableInt(ghostThreshold)
	}
	return result, nil
}

func effectiveFloat(override sql.NullFloat64, fallback float64) float64 {
	if override.Valid {
		return override.Float64
	}
	return fallback
}

func priceSource(override sql.NullFloat64) string {
	if override.Valid {
		return "branch_override"
	}
	return "warehouse"
}

func nullableFloat(value sql.NullFloat64) any {
	if !value.Valid {
		return nil
	}
	return value.Float64
}

func nullableInt(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func (s *Service) UpdateBranchSettings(ctx context.Context, user platform.AuthUser, productID, branchID string, meta audit.LogEntry, input BranchSettingsInput) error {
	if input.SellingPrice.Present && input.SellingPrice.Value != nil && *input.SellingPrice.Value < 0 {
		return platform.NewError(http.StatusBadRequest, "ราคาขายต้องไม่ติดลบ")
	}
	if input.MaxDiscountAmount != nil && *input.MaxDiscountAmount < 0 {
		return platform.NewError(http.StatusBadRequest, "ส่วนลดสูงสุดต้องไม่ติดลบ")
	}
	if (input.LowStockRealThreshold != nil && *input.LowStockRealThreshold < 0) || (input.LowStockGhostThreshold != nil && *input.LowStockGhostThreshold < 0) {
		return platform.NewError(http.StatusBadRequest, "จุดแจ้งเตือนต้องไม่ติดลบ")
	}
	if input.LowStockGhostThreshold != nil && user.RoleKey != "super_admin" {
		return platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์กำหนดค่าสต๊อกผี")
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var exists bool
		var branchType string
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS(SELECT 1 FROM products WHERE id=$1),COALESCE((SELECT branch_type FROM branches WHERE id=$2),'')
		`, productID, branchID).Scan(&exists, &branchType); err != nil {
			return err
		}
		if !exists || branchType == "" {
			return platform.NewError(http.StatusNotFound, "ไม่พบสินค้าหรือสาขา")
		}
		if branchType == "main_warehouse" && input.SellingPrice.Present {
			return platform.NewError(http.StatusBadRequest, "ราคาโกดังต้องแก้จากข้อมูลกลางของสินค้า")
		}
		if input.SellingPrice.Present {
			if input.SellingPrice.Value == nil {
				if _, err := tx.ExecContext(ctx, `DELETE FROM branch_product_prices WHERE branch_id=$1 AND product_id=$2`, branchID, productID); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO branch_product_prices(id,branch_id,product_id,selling_price,created_at,updated_at)
					VALUES($1,$2,$3,$4,NOW(),NOW())
					ON CONFLICT(branch_id,product_id) DO UPDATE
					SET selling_price=EXCLUDED.selling_price,updated_at=NOW()
				`, platform.MustUUID(), branchID, productID, platform.Round2(*input.SellingPrice.Value)); err != nil {
					return err
				}
			}
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO branch_product_settings(id,branch_id,product_id,max_discount_amount,low_stock_real_threshold,low_stock_ghost_threshold,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$6,NOW(),NOW())
			ON CONFLICT(branch_id,product_id) DO UPDATE SET max_discount_amount=EXCLUDED.max_discount_amount,
			low_stock_real_threshold=EXCLUDED.low_stock_real_threshold,
			low_stock_ghost_threshold=CASE WHEN $7 THEN EXCLUDED.low_stock_ghost_threshold ELSE branch_product_settings.low_stock_ghost_threshold END,
			updated_at=NOW()
		`, platform.MustUUID(), branchID, productID, input.MaxDiscountAmount, input.LowStockRealThreshold, input.LowStockGhostThreshold, user.RoleKey == "super_admin")
		if err != nil {
			return err
		}
		meta.EntityType = "branch_product_settings"
		meta.EntityID = &productID
		meta.Action = "branch_product_settings.update"
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) CreateCategory(ctx context.Context, meta audit.LogEntry, input CategoryInput) (string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", platform.NewError(http.StatusBadRequest, "กรุณากรอกชื่อหมวดสินค้า")
	}
	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = "#D71920"
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	id := platform.MustUUID()
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_categories (id, name, color, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
		`, id, name, color, active); err != nil {
			return platform.MapUniqueViolation(err, "ชื่อหมวดสินค้านี้มีอยู่แล้ว")
		}
		meta.EntityType = "product_category"
		meta.EntityID = &id
		meta.Action = "product_category.create"
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
	return id, err
}

func (s *Service) UpdateCategory(ctx context.Context, categoryID string, meta audit.LogEntry, input CategoryInput) error {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณากรอกชื่อหมวดสินค้า")
	}
	color := strings.TrimSpace(input.Color)
	if color == "" {
		color = "#D71920"
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		active := true
		if input.Active != nil {
			active = *input.Active
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE product_categories SET name = $2, color = $3, active = $4, updated_at = NOW()
			WHERE id = $1
		`, categoryID, name, color, active)
		if err != nil {
			return platform.MapUniqueViolation(err, "ชื่อหมวดสินค้านี้มีอยู่แล้ว")
		}
		if count, _ := result.RowsAffected(); count == 0 {
			return platform.NewError(http.StatusNotFound, "ไม่พบหมวดสินค้า")
		}
		meta.EntityType = "product_category"
		meta.EntityID = &categoryID
		meta.Action = "product_category.update"
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
}

// uncategorizedCategoryName is the D3 safety-net bucket: deleting a category
// never orphans its products, it reassigns them here instead of relying on
// category_id's plain ON DELETE SET NULL.
const uncategorizedCategoryName = "ยังไม่จัดหมวด"

func (s *Service) DeleteCategory(ctx context.Context, categoryID string, meta audit.LogEntry) (int64, error) {
	var movedCount int64
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var name string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM product_categories WHERE id = $1 FOR UPDATE`, categoryID).Scan(&name); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบหมวดสินค้า")
			}
			return err
		}
		if name == uncategorizedCategoryName {
			return platform.NewError(http.StatusBadRequest, "ไม่สามารถลบหมวด “ยังไม่จัดหมวด” ได้")
		}

		var uncategorizedID string
		err := tx.QueryRowContext(ctx, `
			INSERT INTO product_categories (id, name, color, active, created_at, updated_at)
			VALUES ($1, $2, '#9E9E9E', TRUE, NOW(), NOW())
			ON CONFLICT (name) DO NOTHING
			RETURNING id
		`, platform.MustUUID(), uncategorizedCategoryName).Scan(&uncategorizedID)
		if err == sql.ErrNoRows {
			err = tx.QueryRowContext(ctx, `SELECT id FROM product_categories WHERE name = $1`, uncategorizedCategoryName).Scan(&uncategorizedID)
		}
		if err != nil {
			return err
		}

		updateResult, err := tx.ExecContext(ctx, `
			UPDATE products SET category_id = $1, updated_at = NOW() WHERE category_id = $2
		`, uncategorizedID, categoryID)
		if err != nil {
			return err
		}
		movedCount, _ = updateResult.RowsAffected()

		result, err := tx.ExecContext(ctx, `DELETE FROM product_categories WHERE id = $1`, categoryID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count == 0 {
			return platform.NewError(http.StatusNotFound, "ไม่พบหมวดสินค้า")
		}
		meta.EntityType = "product_category"
		meta.EntityID = &categoryID
		meta.Action = "product_category.delete"
		meta.After = map[string]any{"deleted_name": name, "products_reassigned_to": uncategorizedID, "products_reassigned_count": movedCount}
		return s.audit.Log(ctx, tx, meta)
	})
	return movedCount, err
}

func (s *Service) CreateAlias(ctx context.Context, meta audit.LogEntry, input AliasInput) (string, error) {
	id := platform.MustUUID()
	if strings.TrimSpace(input.AliasCode) == "" {
		input.AliasCode = platform.GenerateReadableCode("GOV")
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	if strings.TrimSpace(input.ProductID) == "" || strings.TrimSpace(input.AliasCode) == "" || strings.TrimSpace(input.AliasName) == "" {
		return "", platform.NewError(http.StatusBadRequest, "กรุณาเลือกสินค้าและกรอกรหัสกับชื่อสำหรับราชการ")
	}
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO product_aliases (
				id, product_id, branch_id, alias_code, alias_name,
				default_government_price, active, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		`, id, input.ProductID, platform.NullUUID(input.BranchID), strings.TrimSpace(input.AliasCode),
			strings.TrimSpace(input.AliasName), platform.NullFloat64(input.DefaultGovernmentPrice), active)
		if err != nil {
			return platform.MapUniqueViolation(err, "รหัสชื่อสินค้าสำหรับราชการนี้มีอยู่แล้ว")
		}
		meta.EntityType = "product_alias"
		meta.EntityID = &id
		meta.Action = "product_alias.create"
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
	return id, err
}

func (s *Service) UpdateAlias(ctx context.Context, aliasID string, meta audit.LogEntry, input AliasInput) error {
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE product_aliases
			SET product_id = $2, branch_id = $3, alias_code = $4, alias_name = $5,
			    default_government_price = $6, active = $7, updated_at = NOW()
			WHERE id = $1
		`, aliasID, input.ProductID, platform.NullUUID(input.BranchID), strings.TrimSpace(input.AliasCode),
			strings.TrimSpace(input.AliasName), platform.NullFloat64(input.DefaultGovernmentPrice), active)
		if err != nil {
			return platform.MapUniqueViolation(err, "รหัสชื่อสินค้าสำหรับราชการนี้มีอยู่แล้ว")
		}
		if count, _ := result.RowsAffected(); count == 0 {
			return platform.NewError(http.StatusNotFound, "ไม่พบชื่อสินค้าสำหรับราชการ")
		}
		meta.EntityType = "product_alias"
		meta.EntityID = &aliasID
		meta.Action = "product_alias.update"
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) DeleteAlias(ctx context.Context, aliasID string, meta audit.LogEntry) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE invoice_items SET alias_id = NULL WHERE alias_id = $1`, aliasID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE quotation_items SET alias_id = NULL WHERE alias_id = $1`, aliasID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM product_aliases WHERE id = $1`, aliasID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count == 0 {
			return platform.NewError(http.StatusNotFound, "ไม่พบชื่อสินค้าสำหรับราชการ")
		}
		meta.EntityType = "product_alias"
		meta.EntityID = &aliasID
		meta.Action = "product_alias.delete"
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) ProductDeletionImpact(ctx context.Context, productID string) (map[string]any, error) {
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM products WHERE id = $1`, productID).Scan(&name); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
		}
		return nil, err
	}
	counts := map[string]int{}
	queries := map[string]string{
		"รายการในใบขาย":             "SELECT COUNT(*) FROM invoice_items WHERE product_id = $1",
		"รายการในใบเสนอราคา":        "SELECT COUNT(*) FROM quotation_items WHERE product_id = $1",
		"รายการโอนสินค้า":           "SELECT COUNT(*) FROM transfer_items WHERE product_id = $1",
		"ประวัติการเคลื่อนไหวสต๊อก": "SELECT COUNT(*) FROM inventory_movements WHERE product_id = $1",
		"ยอดสต๊อกแต่ละสาขา":         "SELECT COUNT(*) FROM inventory WHERE product_id = $1",
		"ชื่อสินค้าสำหรับราชการ":    "SELECT COUNT(*) FROM product_aliases WHERE product_id = $1",
	}
	for key, query := range queries {
		var count int
		if err := s.db.QueryRowContext(ctx, query, productID).Scan(&count); err != nil {
			return nil, err
		}
		counts[key] = count
	}
	return map[string]any{
		"id": productID, "name": name, "counts": counts,
		"confirmation": "ลบ " + name,
	}, nil
}

func (s *Service) DeleteProduct(ctx context.Context, productID string, confirmation string, meta audit.LogEntry) (string, error) {
	var imageKey, name string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			SELECT name, COALESCE(image_storage_key, '') FROM products WHERE id = $1 FOR UPDATE
		`, productID).Scan(&name, &imageKey); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
			}
			return err
		}
		if confirmation != "ลบ "+name {
			return platform.NewError(http.StatusBadRequest, "ข้อความยืนยันการลบไม่ถูกต้อง")
		}
		var invoiceItemCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM invoice_items WHERE product_id=$1`, productID).Scan(&invoiceItemCount); err != nil {
			return err
		}
		if invoiceItemCount > 0 {
			return platform.NewError(http.StatusConflict, "ไม่สามารถลบสินค้าที่มีประวัติใบขายได้")
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM quotation_items WHERE product_id = $1`, productID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE quotations q SET
				subtotal = x.subtotal,
				tax_amount = ROUND((x.subtotal * q.tax_rate / 100)::numeric, 2),
				total_amount = x.subtotal + ROUND((x.subtotal * q.tax_rate / 100)::numeric, 2),
				updated_at = NOW()
			FROM (SELECT quotation_id, SUM(line_subtotal) subtotal FROM quotation_items GROUP BY quotation_id) x
			WHERE q.id = x.quotation_id
		`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM quotations q WHERE NOT EXISTS (SELECT 1 FROM quotation_items x WHERE x.quotation_id = q.id)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM transfer_items WHERE product_id = $1`, productID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM transfers t WHERE NOT EXISTS (SELECT 1 FROM transfer_items x WHERE x.transfer_id = t.id)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM inventory_movements WHERE product_id = $1`, productID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM products WHERE id = $1`, productID); err != nil {
			return err
		}
		meta.EntityType = "product"
		meta.EntityID = nil
		meta.Action = "product.delete"
		meta.After = map[string]any{"deleted_id": productID, "name": name}
		return s.audit.Log(ctx, tx, meta)
	})
	if err == nil && imageKey != "" {
		_ = os.Remove(filepath.Join(s.uploadDir, filepath.Base(imageKey)))
	}
	return name, err
}

func validateImage(file *multipart.FileHeader) (string, string, error) {
	if file.Size <= 0 || file.Size > maxProductImageSize {
		return "", "", platform.NewError(http.StatusBadRequest, "รูปสินค้าต้องมีขนาดไม่เกิน 5 MB")
	}
	source, err := file.Open()
	if err != nil {
		return "", "", err
	}
	defer source.Close()
	buffer := make([]byte, 512)
	n, err := source.Read(buffer)
	if err != nil && err != io.EOF {
		return "", "", err
	}
	mimeType := http.DetectContentType(buffer[:n])
	extensions := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	}
	extension, ok := extensions[mimeType]
	if !ok {
		return "", "", platform.NewError(http.StatusBadRequest, "รองรับเฉพาะไฟล์ JPEG, PNG หรือ WebP")
	}
	return mimeType, extension, nil
}

// SaveProductImage stores an image for a product. branchID empty = a catalog
// image (becomes the product's primary, visible everywhere); branchID set =
// an extra image added from that branch's stock page, which sits alongside
// the catalog images without replacing the primary one.
func (s *Service) SaveProductImage(ctx context.Context, productID string, branchID string, file *multipart.FileHeader, meta audit.LogEntry) error {
	mimeType, extension, err := validateImage(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return err
	}
	storageKey := platform.MustUUID() + extension
	temporaryPath := filepath.Join(s.uploadDir, storageKey+".tmp")
	finalPath := filepath.Join(s.uploadDir, storageKey)
	source, err := file.Open()
	if err != nil {
		return err
	}
	destination, err := os.Create(temporaryPath)
	if err != nil {
		source.Close()
		return err
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	source.Close()
	if copyErr != nil {
		_ = os.Remove(temporaryPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(temporaryPath)
		return closeErr
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}

	imageID := platform.MustUUID()
	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var currentKey string
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(image_storage_key, '') FROM products WHERE id = $1 FOR UPDATE
		`, productID).Scan(&currentKey); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
			}
			return err
		}
		var nextOrder int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order)+1,0) FROM product_images WHERE product_id=$1`, productID).Scan(&nextOrder); err != nil {
			return err
		}
		branchScoped := strings.TrimSpace(branchID) != ""
		if branchScoped {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM branches WHERE id=$1 AND active=TRUE)`, branchID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return platform.NewError(http.StatusBadRequest, "ไม่พบสาขาที่เปิดใช้งาน")
			}
			// Extra branch image: never primary, never touches the catalog's
			// own image_storage_key.
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO product_images (id,product_id,branch_id,storage_key,mime_type,alt_text,is_primary,sort_order,created_at,updated_at)
				VALUES ($1,$2,$3,$4,$5,(SELECT name FROM products WHERE id=$2),FALSE,$6,NOW(),NOW())
			`, imageID, productID, branchID, storageKey, mimeType, nextOrder); err != nil {
				return err
			}
			meta.EntityType = "product"
			meta.EntityID = &productID
			meta.Action = "product.image.branch_add"
			meta.After = map[string]any{"mime_type": mimeType, "branch_id": branchID}
			return s.audit.Log(ctx, tx, meta)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE product_images SET is_primary=FALSE,updated_at=NOW() WHERE product_id=$1 AND is_primary=TRUE`, productID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_images (id,product_id,storage_key,mime_type,alt_text,is_primary,sort_order,created_at,updated_at)
			VALUES ($1,$2,$3,$4,(SELECT name FROM products WHERE id=$2),TRUE,$5,NOW(),NOW())
		`, imageID, productID, storageKey, mimeType, nextOrder); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE products SET image_storage_key = $2, image_mime_type = $3, updated_at = NOW() WHERE id = $1
		`, productID, storageKey, mimeType); err != nil {
			return err
		}
		meta.EntityType = "product"
		meta.EntityID = &productID
		meta.Action = "product.image.update"
		meta.After = map[string]any{"mime_type": mimeType}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		_ = os.Remove(finalPath)
		return err
	}
	return nil
}

// ListProductImages returns the catalog images (branch_id IS NULL) plus, when
// branchID is given, that branch's own added images — the stock pages show
// both, catalog first (business-flow.md รูปภาพ rule).
func (s *Service) ListProductImages(ctx context.Context, productID string, branchID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text,storage_key,mime_type,alt_text,is_primary,sort_order,
		       COALESCE(source_branch_code,''),COALESCE(source_name,''),branch_id IS NOT NULL
		FROM product_images
		WHERE product_id=$1 AND (branch_id IS NULL OR branch_id = $2)
		ORDER BY branch_id IS NOT NULL, is_primary DESC, sort_order, id
	`, productID, platform.NullUUID(&branchID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, storageKey, mimeType, altText, branchCode, sourceName string
		var primary, branchScoped bool
		var order int
		if err := rows.Scan(&id, &storageKey, &mimeType, &altText, &primary, &order, &branchCode, &sourceName, &branchScoped); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id": id, "mime_type": mimeType, "alt_text": altText, "is_primary": primary,
			"sort_order": order, "source_branch_code": branchCode, "source_name": sourceName,
			"branch_scoped": branchScoped,
			"url":           "/products/" + productID + "/images/" + id,
		})
	}
	return items, rows.Err()
}

func (s *Service) ProductGalleryImage(ctx context.Context, productID, imageID string) (string, string, error) {
	var storageKey, mimeType string
	if err := s.db.QueryRowContext(ctx, `SELECT storage_key,mime_type FROM product_images WHERE id=$1 AND product_id=$2`, imageID, productID).Scan(&storageKey, &mimeType); err != nil {
		if err == sql.ErrNoRows {
			return "", "", platform.NewError(http.StatusNotFound, "ไม่พบรูปสินค้า")
		}
		return "", "", err
	}
	return filepath.Join(s.uploadDir, filepath.Base(storageKey)), mimeType, nil
}

func (s *Service) ProductImage(ctx context.Context, productID string) (string, string, error) {
	var storageKey, mimeType string
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(image_storage_key, ''), COALESCE(image_mime_type, '')
		FROM products WHERE id = $1
	`, productID).Scan(&storageKey, &mimeType); err != nil {
		if err == sql.ErrNoRows {
			return "", "", platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
		}
		return "", "", err
	}
	if storageKey == "" {
		return "", "", platform.NewError(http.StatusNotFound, "สินค้านี้ยังไม่มีรูป")
	}
	return filepath.Join(s.uploadDir, filepath.Base(storageKey)), mimeType, nil
}

func (s *Service) DeleteProductImage(ctx context.Context, productID string, meta audit.LogEntry) error {
	var storageKey string
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(image_storage_key, '') FROM products WHERE id = $1 FOR UPDATE
		`, productID).Scan(&storageKey); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบสินค้า")
			}
			return err
		}
		var imageID string
		if err := tx.QueryRowContext(ctx, `SELECT id::text,storage_key FROM product_images WHERE product_id=$1 AND is_primary=TRUE FOR UPDATE`, productID).Scan(&imageID, &storageKey); err != nil && err != sql.ErrNoRows {
			return err
		}
		if imageID != "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM product_images WHERE id=$1`, imageID); err != nil {
				return err
			}
		}
		var nextID, nextKey, nextMime string
		err := tx.QueryRowContext(ctx, `SELECT id::text,storage_key,mime_type FROM product_images WHERE product_id=$1 ORDER BY sort_order,id LIMIT 1`, productID).Scan(&nextID, &nextKey, &nextMime)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if nextID != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE product_images SET is_primary=TRUE,updated_at=NOW() WHERE id=$1`, nextID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE products SET image_storage_key=NULLIF($2,''),image_mime_type=NULLIF($3,''),updated_at=NOW() WHERE id=$1
		`, productID, nextKey, nextMime); err != nil {
			return err
		}
		meta.EntityType = "product"
		meta.EntityID = &productID
		meta.Action = "product.image.delete"
		return s.audit.Log(ctx, tx, meta)
	})
	if err == nil && storageKey != "" {
		var referenced bool
		if scanErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM product_images WHERE storage_key=$1)`, storageKey).Scan(&referenced); scanErr == nil && !referenced {
			_ = os.Remove(filepath.Join(s.uploadDir, filepath.Base(storageKey)))
		}
	}
	return err
}

type deleteRequest struct {
	Confirmation string `json:"confirmation"`
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
	if user.BranchID != nil && user.Scope != "global" && branchID != "" && branchID != *user.BranchID {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์เข้าถึงสาขานี้"))
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	result, err := h.service.List(c.Request().Context(), user, branchID, ListFilter{
		Search: strings.TrimSpace(c.QueryParam("search")), CategoryID: strings.TrimSpace(c.QueryParam("category_id")),
		Active: strings.TrimSpace(c.QueryParam("active")), Page: page, PageSize: pageSize,
		SalesChannel: strings.TrimSpace(c.QueryParam("sales_channel")), RequiresFDAReport: strings.TrimSpace(c.QueryParam("requires_fda_report")),
	})
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดสินค้าไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{
		"items":      result.Items,
		"pagination": map[string]any{"page": result.Page, "page_size": result.PageSize, "total": result.Total, "total_pages": result.TotalPages},
	})
}

func (h *Handler) ListCategories(c echo.Context) error {
	items, err := h.service.ListCategories(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดหมวดสินค้าไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ListAliases(c echo.Context) error {
	branchID := strings.TrimSpace(c.QueryParam("branch_id"))
	productID := strings.TrimSpace(c.QueryParam("product_id"))
	user := platform.CurrentUser(c)
	if branchID == "" && user.BranchID != nil && user.Scope != "global" {
		branchID = *user.BranchID
	}
	if user.BranchID != nil && user.Scope != "global" && branchID != "" && branchID != *user.BranchID {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์เข้าถึงสาขานี้"))
	}
	items, err := h.service.ListAliases(c.Request().Context(), branchID, productID)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดชื่อสินค้าสำหรับราชการไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) CreateProduct(c echo.Context) error {
	var input ProductInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลสินค้าไม่ถูกต้อง"))
	}
	id, err := h.service.CreateProduct(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "เพิ่มสินค้าแล้ว"})
}

func (h *Handler) UpdateProduct(c echo.Context) error {
	var input ProductInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลสินค้าไม่ถูกต้อง"))
	}
	if err := h.service.UpdateProduct(c.Request().Context(), c.Param("productID"), platform.CurrentUser(c), audit.MetaFromContext(c), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกสินค้าแล้ว")
}

func (h *Handler) GetBranchSettings(c echo.Context) error {
	item, err := h.service.GetBranchSettings(c.Request().Context(), platform.CurrentUser(c), c.Param("productID"), c.Param("branchID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}

func (h *Handler) UpdateBranchSettings(c echo.Context) error {
	var input BranchSettingsInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	if err := h.service.UpdateBranchSettings(c.Request().Context(), platform.CurrentUser(c), c.Param("productID"), c.Param("branchID"), audit.MetaFromContext(c), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกการตั้งค่าสินค้าประจำสาขาแล้ว")
}

func (h *Handler) CreateCategory(c echo.Context) error {
	var input CategoryInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลหมวดสินค้าไม่ถูกต้อง"))
	}
	id, err := h.service.CreateCategory(c.Request().Context(), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "เพิ่มหมวดสินค้าแล้ว"})
}

func (h *Handler) UpdateCategory(c echo.Context) error {
	var input CategoryInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลหมวดสินค้าไม่ถูกต้อง"))
	}
	if err := h.service.UpdateCategory(c.Request().Context(), c.Param("categoryID"), audit.MetaFromContext(c), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกหมวดสินค้าแล้ว")
}

func (h *Handler) DeleteCategory(c echo.Context) error {
	moved, err := h.service.DeleteCategory(c.Request().Context(), c.Param("categoryID"), audit.MetaFromContext(c))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	message := "ลบหมวดสินค้าแล้ว"
	if moved > 0 {
		message = fmt.Sprintf("ลบหมวดสินค้าแล้ว และย้ายสินค้า %d รายการไปยัง “%s”", moved, uncategorizedCategoryName)
	}
	return platform.JSONMessage(c, http.StatusOK, message)
}

func (h *Handler) CreateAlias(c echo.Context) error {
	var input AliasInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลชื่อสินค้าสำหรับราชการไม่ถูกต้อง"))
	}
	id, err := h.service.CreateAlias(c.Request().Context(), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "เพิ่มชื่อสินค้าสำหรับราชการแล้ว"})
}

func (h *Handler) UpdateAlias(c echo.Context) error {
	var input AliasInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลชื่อสินค้าสำหรับราชการไม่ถูกต้อง"))
	}
	if err := h.service.UpdateAlias(c.Request().Context(), c.Param("aliasID"), audit.MetaFromContext(c), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกชื่อสินค้าสำหรับราชการแล้ว")
}

func (h *Handler) DeleteAlias(c echo.Context) error {
	if err := h.service.DeleteAlias(c.Request().Context(), c.Param("aliasID"), audit.MetaFromContext(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบชื่อสินค้าสำหรับราชการแล้ว")
}

func (h *Handler) ProductDeletionImpact(c echo.Context) error {
	impact, err := h.service.ProductDeletionImpact(c.Request().Context(), c.Param("productID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, impact)
}

func (h *Handler) DeleteProduct(c echo.Context) error {
	var input deleteRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลยืนยันการลบไม่ถูกต้อง"))
	}
	name, err := h.service.DeleteProduct(c.Request().Context(), c.Param("productID"), input.Confirmation, audit.MetaFromContext(c))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบสินค้า "+name+" และข้อมูลที่เกี่ยวข้องแล้ว")
}

func (h *Handler) UploadProductImage(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "กรุณาเลือกไฟล์รูปสินค้า"))
	}
	// branch_id present = an extra image added from that branch's stock page.
	branchID := c.FormValue("branch_id")
	if err := h.service.SaveProductImage(c.Request().Context(), c.Param("productID"), branchID, file, audit.MetaFromContext(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	if strings.TrimSpace(branchID) != "" {
		return platform.JSONMessage(c, http.StatusOK, "เพิ่มรูปของสาขาแล้ว")
	}
	return platform.JSONMessage(c, http.StatusOK, "อัปโหลดรูปสินค้าแล้ว")
}

func (h *Handler) ProductImage(c echo.Context) error {
	path, mimeType, err := h.service.ProductImage(c.Request().Context(), c.Param("productID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	c.Response().Header().Set(echo.HeaderContentType, mimeType)
	c.Response().Header().Set(echo.HeaderCacheControl, "private, max-age=300")
	return c.File(path)
}

func (h *Handler) ListProductImages(c echo.Context) error {
	items, err := h.service.ListProductImages(c.Request().Context(), c.Param("productID"), c.QueryParam("branch_id"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) ProductGalleryImage(c echo.Context) error {
	path, mimeType, err := h.service.ProductGalleryImage(c.Request().Context(), c.Param("productID"), c.Param("imageID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	c.Response().Header().Set(echo.HeaderContentType, mimeType)
	c.Response().Header().Set(echo.HeaderCacheControl, "private, max-age=86400")
	return c.File(path)
}

func (h *Handler) DeleteProductImage(c echo.Context) error {
	if err := h.service.DeleteProductImage(c.Request().Context(), c.Param("productID"), audit.MetaFromContext(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบรูปสินค้าแล้ว")
}
