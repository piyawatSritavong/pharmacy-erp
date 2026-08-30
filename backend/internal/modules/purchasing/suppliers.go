package purchasing

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type SupplierInput struct {
	SupplierCode      string `json:"supplier_code"`
	LegalName         string `json:"legal_name"`
	TaxID             string `json:"tax_id"`
	CompanyBranchType string `json:"company_branch_type"`
	CompanyBranchNo   string `json:"company_branch_number"`
	AddressLine       string `json:"address_line"`
	Subdistrict       string `json:"subdistrict"`
	District          string `json:"district"`
	Province          string `json:"province"`
	PostalCode        string `json:"postal_code"`
	ContactName       string `json:"contact_name"`
	Phone             string `json:"phone"`
	Email             string `json:"email"`
	PaymentTermsDays  int    `json:"payment_terms_days"`
	Notes             string `json:"notes"`
	Active            *bool  `json:"active"`
}

type CursorResult struct {
	Items      []map[string]any `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
	HasMore    bool             `json:"has_more"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func decodeOffset(value string) int {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	offset, err := strconv.Atoi(string(decoded))
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func encodeOffset(value int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(value)))
}

// normalizeLimit defaults to a page of 20 but allows a caller to ask for the
// whole list. The suppliers screen filters and pages client-side, so it needs
// every row up front; the ceiling only exists to stop an unbounded query.
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func validateSupplier(input SupplierInput) error {
	if strings.TrimSpace(input.LegalName) == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณากรอกชื่อบริษัทคู่ค้า")
	}
	if input.PaymentTermsDays < 0 {
		return platform.NewError(http.StatusBadRequest, "จำนวนวันเครดิตต้องไม่ติดลบ")
	}
	if input.CompanyBranchType == "" {
		input.CompanyBranchType = "head_office"
	}
	if input.CompanyBranchType != "head_office" && input.CompanyBranchType != "branch" {
		return platform.NewError(http.StatusBadRequest, "ประเภทสำนักงานบริษัทไม่ถูกต้อง")
	}
	return nil
}

func (s *Service) ListSuppliers(ctx context.Context, search, active, cursor string, limit int) (CursorResult, error) {
	limit = normalizeLimit(limit)
	offset := decodeOffset(cursor)
	args := []any{}
	conditions := []string{}
	if keyword := strings.TrimSpace(search); keyword != "" {
		args = append(args, "%"+strings.ToLower(keyword)+"%")
		p := "$" + strconv.Itoa(len(args))
		conditions = append(conditions, fmt.Sprintf("(LOWER(s.legal_name) LIKE %s OR LOWER(s.supplier_code) LIKE %s OR LOWER(COALESCE(s.tax_id, '')) LIKE %s OR LOWER(s.phone) LIKE %s)", p, p, p, p))
	}
	if active == "true" || active == "false" {
		args = append(args, active == "true")
		conditions = append(conditions, "s.active = $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	args = append(args, limit+1, offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id::text, s.supplier_code, s.legal_name, COALESCE(s.tax_id, ''),
		       s.company_branch_type, s.company_branch_number, s.address_line,
		       s.subdistrict, s.district, s.province, s.postal_code,
		       s.contact_name, s.phone, s.email, s.payment_terms_days,
		       s.notes, s.active, s.created_at, s.updated_at,
		       COUNT(po.id)::bigint
		FROM suppliers s
		LEFT JOIN purchase_orders po ON po.supplier_id = s.id
		`+where+`
		GROUP BY s.id
		ORDER BY s.active DESC, s.legal_name, s.id
		LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return CursorResult{}, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, code, name, taxID, branchType, branchNo, address, subdistrict, district, province, postalCode, contact, phone, email, notes string
		var terms int
		var activeValue bool
		var createdAt, updatedAt any
		var poCount int64
		if err := rows.Scan(&id, &code, &name, &taxID, &branchType, &branchNo, &address, &subdistrict, &district, &province, &postalCode, &contact, &phone, &email, &terms, &notes, &activeValue, &createdAt, &updatedAt, &poCount); err != nil {
			return CursorResult{}, err
		}
		items = append(items, map[string]any{
			"id": id, "supplier_code": code, "legal_name": name, "tax_id": taxID,
			"company_branch_type": branchType, "company_branch_number": branchNo,
			"address_line": address, "subdistrict": subdistrict, "district": district,
			"province": province, "postal_code": postalCode, "contact_name": contact,
			"phone": phone, "email": email, "payment_terms_days": terms, "notes": notes,
			"active": activeValue, "purchase_order_count": poCount,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return CursorResult{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	result := CursorResult{Items: items, HasMore: hasMore}
	if hasMore {
		result.NextCursor = encodeOffset(offset + limit)
	}
	return result, nil
}

func (s *Service) GetSupplier(ctx context.Context, id string) (map[string]any, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT jsonb_build_object(
			'id',s.id::text,'supplier_code',s.supplier_code,'legal_name',s.legal_name,
			'tax_id',COALESCE(s.tax_id,''),'company_branch_type',s.company_branch_type,
			'company_branch_number',s.company_branch_number,'address_line',s.address_line,
			'subdistrict',s.subdistrict,'district',s.district,'province',s.province,
			'postal_code',s.postal_code,'contact_name',s.contact_name,'phone',s.phone,
			'email',s.email,'payment_terms_days',s.payment_terms_days,'notes',s.notes,
			'active',s.active,'created_at',s.created_at,'updated_at',s.updated_at,
			'purchase_order_count',(SELECT COUNT(*) FROM purchase_orders po WHERE po.supplier_id=s.id)
		) FROM suppliers s WHERE s.id=$1
	`, id).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบบริษัทคู่ค้า")
		}
		return nil, err
	}
	item := map[string]any{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) CreateSupplier(ctx context.Context, meta audit.LogEntry, input SupplierInput) (string, error) {
	if err := validateSupplier(input); err != nil {
		return "", err
	}
	id := platform.MustUUID()
	code := strings.ToUpper(strings.TrimSpace(input.SupplierCode))
	if code == "" {
		code = platform.GenerateReadableCode("SUP")
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO suppliers (
				id,supplier_code,legal_name,tax_id,company_branch_type,company_branch_number,
				address_line,subdistrict,district,province,postal_code,contact_name,phone,email,
				payment_terms_days,notes,active,created_at,updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NOW(),NOW())
		`, id, code, strings.TrimSpace(input.LegalName), platform.NullString(input.TaxID), normalizeCompanyBranchType(input.CompanyBranchType), strings.TrimSpace(input.CompanyBranchNo),
			strings.TrimSpace(input.AddressLine), strings.TrimSpace(input.Subdistrict), strings.TrimSpace(input.District), strings.TrimSpace(input.Province), strings.TrimSpace(input.PostalCode),
			strings.TrimSpace(input.ContactName), strings.TrimSpace(input.Phone), strings.TrimSpace(input.Email), input.PaymentTermsDays, strings.TrimSpace(input.Notes), active)
		if err != nil {
			return platform.MapUniqueViolation(err, "ชื่อบริษัท รหัสคู่ค้า หรือเลขผู้เสียภาษีนี้มีอยู่แล้ว")
		}
		meta.EntityType = "supplier"
		meta.EntityID = &id
		meta.Action = "supplier.create"
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
	return id, err
}

func normalizeCompanyBranchType(value string) string {
	if strings.TrimSpace(value) == "branch" {
		return "branch"
	}
	return "head_office"
}

func (s *Service) UpdateSupplier(ctx context.Context, id string, meta audit.LogEntry, input SupplierInput) error {
	if err := validateSupplier(input); err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var before string
		var currentActive bool
		if err := tx.QueryRowContext(ctx, `SELECT row_to_json(s)::text, active FROM suppliers s WHERE id = $1 FOR UPDATE`, id).Scan(&before, &currentActive); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบบริษัทคู่ค้า")
			}
			return err
		}
		active := currentActive
		if input.Active != nil {
			active = *input.Active
		}
		code := strings.ToUpper(strings.TrimSpace(input.SupplierCode))
		if code == "" {
			return platform.NewError(http.StatusBadRequest, "กรุณากรอกรหัสคู่ค้า")
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE suppliers SET supplier_code=$2,legal_name=$3,tax_id=$4,company_branch_type=$5,
			company_branch_number=$6,address_line=$7,subdistrict=$8,district=$9,province=$10,
			postal_code=$11,contact_name=$12,phone=$13,email=$14,payment_terms_days=$15,
			notes=$16,active=$17,updated_at=NOW() WHERE id=$1
		`, id, code, strings.TrimSpace(input.LegalName), platform.NullString(input.TaxID), normalizeCompanyBranchType(input.CompanyBranchType), strings.TrimSpace(input.CompanyBranchNo),
			strings.TrimSpace(input.AddressLine), strings.TrimSpace(input.Subdistrict), strings.TrimSpace(input.District), strings.TrimSpace(input.Province), strings.TrimSpace(input.PostalCode),
			strings.TrimSpace(input.ContactName), strings.TrimSpace(input.Phone), strings.TrimSpace(input.Email), input.PaymentTermsDays, strings.TrimSpace(input.Notes), active)
		if err != nil {
			return platform.MapUniqueViolation(err, "ชื่อบริษัท รหัสคู่ค้า หรือเลขผู้เสียภาษีนี้มีอยู่แล้ว")
		}
		meta.EntityType = "supplier"
		meta.EntityID = &id
		meta.Action = "supplier.update"
		meta.Before = before
		meta.After = input
		return s.audit.Log(ctx, tx, meta)
	})
}

// SupplierDeletionImpact reports what still references this supplier so the
// UI can state the consequence before deleting (same pattern as branches,
// users and products).
func (s *Service) SupplierDeletionImpact(ctx context.Context, id string) (map[string]any, error) {
	var legalName string
	if err := s.db.QueryRowContext(ctx, `SELECT legal_name FROM suppliers WHERE id=$1`, id).Scan(&legalName); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบบริษัทคู่ค้า")
		}
		return nil, err
	}
	counts := map[string]int{}
	for label, query := range map[string]string{
		"ใบสั่งซื้อเข้า": `SELECT COUNT(*) FROM purchase_orders WHERE supplier_id=$1`,
		"งานเคลม":        `SELECT COUNT(*) FROM product_returns WHERE supplier_id=$1`,
	} {
		var count int
		if err := s.db.QueryRowContext(ctx, query, id).Scan(&count); err != nil {
			return nil, err
		}
		counts[label] = count
	}
	return map[string]any{
		"id": id, "name": legalName, "counts": counts,
		"confirmation": "ลบ " + legalName,
	}, nil
}

// DeleteSupplier is a real delete, not an archive (business-flow.md Global
// Rules allow CRUD only). Existing purchase orders keep their own snapshot of
// the supplier's code/name/tax id/address, so their documents are unchanged —
// only the live reference is cleared (migration 031).
func (s *Service) DeleteSupplier(ctx context.Context, id string, confirmation string, meta audit.LogEntry) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var legalName string
		if err := tx.QueryRowContext(ctx, `SELECT legal_name FROM suppliers WHERE id=$1 FOR UPDATE`, id).Scan(&legalName); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบบริษัทคู่ค้า")
			}
			return err
		}
		if strings.TrimSpace(confirmation) != "ลบ "+legalName {
			return platform.NewError(http.StatusBadRequest, "ข้อความยืนยันการลบไม่ถูกต้อง")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM suppliers WHERE id=$1`, id); err != nil {
			return err
		}
		meta.EntityType = "supplier"
		meta.EntityID = nil
		meta.Action = "supplier.delete"
		meta.After = map[string]any{"deleted_id": id, "legal_name": legalName}
		return s.audit.Log(ctx, tx, meta)
	})
}

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) ListSuppliers(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	result, err := h.service.ListSuppliers(c.Request().Context(), c.QueryParam("search"), c.QueryParam("active"), c.QueryParam("cursor"), limit)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) GetSupplier(c echo.Context) error {
	item, err := h.service.GetSupplier(c.Request().Context(), c.Param("supplierID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}

func (h *Handler) CreateSupplier(c echo.Context) error {
	var input SupplierInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	id, err := h.service.CreateSupplier(c.Request().Context(), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "บันทึกบริษัทคู่ค้าแล้ว"})
}

func (h *Handler) UpdateSupplier(c echo.Context) error {
	var input SupplierInput
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	if err := h.service.UpdateSupplier(c.Request().Context(), c.Param("supplierID"), audit.MetaFromContext(c), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "แก้ไขบริษัทคู่ค้าแล้ว")
}

func (h *Handler) SupplierDeletionImpact(c echo.Context) error {
	impact, err := h.service.SupplierDeletionImpact(c.Request().Context(), c.Param("supplierID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, impact)
}

func (h *Handler) DeleteSupplier(c echo.Context) error {
	var input struct {
		Confirmation string `json:"confirmation"`
	}
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลยืนยันการลบไม่ถูกต้อง"))
	}
	if err := h.service.DeleteSupplier(c.Request().Context(), c.Param("supplierID"), input.Confirmation, audit.MetaFromContext(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบบริษัทคู่ค้าแล้ว")
}
