// Package promotions stores the selling campaigns POS applies at checkout:
// buy X get Y, percent/amount off, bundle price, and bill-level giveaways.
// Evaluation lives in the sales module so pricing stays in one transaction.
package promotions

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const (
	TypeBuyXGetY     = "buy_x_get_y"
	TypePercent      = "percent"
	TypeAmount       = "amount"
	TypeBundle       = "bundle"
	TypeBillGiveaway = "bill_giveaway"
)

type ItemInput struct {
	ProductID string `json:"product_id"`
	UnitID    string `json:"unit_id"`
	Quantity  int    `json:"quantity"`
	Role      string `json:"role"`
}

type Input struct {
	Code            string      `json:"code"`
	Name            string      `json:"name"`
	PromoType       string      `json:"promo_type"`
	BranchID        string      `json:"branch_id"`
	StartsAt        string      `json:"starts_at"`
	EndsAt          string      `json:"ends_at"`
	Active          *bool       `json:"active"`
	MinQuantity     int         `json:"min_quantity"`
	MinAmount       float64     `json:"min_amount"`
	DiscountPercent float64     `json:"discount_percent"`
	DiscountAmount  float64     `json:"discount_amount"`
	BundlePrice     *float64    `json:"bundle_price"`
	MaxUsesPerBill  int         `json:"max_uses_per_bill"`
	Priority        int         `json:"priority"`
	Notes           string      `json:"notes"`
	Items           []ItemInput `json:"items"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func validate(input Input) (Input, error) {
	input.Code = strings.ToUpper(strings.TrimSpace(input.Code))
	input.Name = strings.TrimSpace(input.Name)
	input.PromoType = strings.TrimSpace(input.PromoType)
	if input.Code == "" || input.Name == "" {
		return input, platform.NewError(http.StatusBadRequest, "กรุณากรอกรหัสและชื่อโปรโมชั่น")
	}
	switch input.PromoType {
	case TypeBuyXGetY, TypePercent, TypeAmount, TypeBundle, TypeBillGiveaway:
	default:
		return input, platform.NewError(http.StatusBadRequest, "ชนิดโปรโมชั่นไม่ถูกต้อง")
	}
	if input.StartsAt != "" && input.EndsAt != "" && input.EndsAt < input.StartsAt {
		return input, platform.NewError(http.StatusBadRequest, "วันสิ้นสุดต้องไม่ก่อนวันเริ่ม")
	}
	if input.DiscountPercent < 0 || input.DiscountPercent > 100 {
		return input, platform.NewError(http.StatusBadRequest, "ส่วนลดเป็นเปอร์เซ็นต์ต้องอยู่ระหว่าง 0-100")
	}
	if input.DiscountAmount < 0 || input.MinAmount < 0 || input.MinQuantity < 0 {
		return input, platform.NewError(http.StatusBadRequest, "จำนวนเงินและจำนวนขั้นต่ำต้องไม่ติดลบ")
	}

	conditions, rewards, bundleItems := 0, 0, 0
	for index, item := range input.Items {
		if strings.TrimSpace(item.ProductID) == "" {
			return input, platform.NewError(http.StatusBadRequest, "กรุณาเลือกสินค้าในเงื่อนไขโปรโมชั่น")
		}
		if item.Quantity <= 0 {
			input.Items[index].Quantity = 1
		}
		switch item.Role {
		case "condition":
			conditions++
		case "reward":
			rewards++
		case "bundle_item":
			bundleItems++
		default:
			return input, platform.NewError(http.StatusBadRequest, "บทบาทของสินค้าในโปรโมชั่นไม่ถูกต้อง")
		}
	}

	switch input.PromoType {
	case TypeBuyXGetY:
		if conditions == 0 || rewards == 0 {
			return input, platform.NewError(http.StatusBadRequest, "โปรซื้อแถมต้องมีทั้งสินค้าที่ต้องซื้อและของแถม")
		}
	case TypeBundle:
		if bundleItems < 2 {
			return input, platform.NewError(http.StatusBadRequest, "ราคาชุดต้องมีสินค้าในชุดอย่างน้อย 2 รายการ")
		}
		if input.BundlePrice == nil || *input.BundlePrice < 0 {
			return input, platform.NewError(http.StatusBadRequest, "กรุณากำหนดราคาชุด")
		}
	case TypePercent:
		if input.DiscountPercent <= 0 {
			return input, platform.NewError(http.StatusBadRequest, "กรุณากำหนดเปอร์เซ็นต์ส่วนลด")
		}
	case TypeAmount:
		if input.DiscountAmount <= 0 {
			return input, platform.NewError(http.StatusBadRequest, "กรุณากำหนดจำนวนเงินส่วนลด")
		}
	case TypeBillGiveaway:
		if rewards == 0 {
			return input, platform.NewError(http.StatusBadRequest, "ของแถมท้ายบิลต้องเลือกสินค้าที่จะแถม")
		}
		if input.MinAmount <= 0 {
			return input, platform.NewError(http.StatusBadRequest, "กรุณากำหนดยอดซื้อขั้นต่ำของของแถมท้ายบิล")
		}
	}
	return input, nil
}

func nullableDate(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func (s *Service) Create(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input Input) (string, error) {
	clean, err := validate(input)
	if err != nil {
		return "", err
	}
	promotionID := platform.MustUUID()
	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		active := true
		if clean.Active != nil {
			active = *clean.Active
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO promotions (id, code, name, promo_type, branch_id, starts_at, ends_at, active,
				min_quantity, min_amount, discount_percent, discount_amount, bundle_price,
				max_uses_per_bill, priority, notes, created_by, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NOW(),NOW())
		`, promotionID, clean.Code, clean.Name, clean.PromoType, platform.NullUUID(&clean.BranchID),
			nullableDate(clean.StartsAt), nullableDate(clean.EndsAt), active,
			clean.MinQuantity, platform.Round2(clean.MinAmount), clean.DiscountPercent,
			platform.Round2(clean.DiscountAmount), platform.NullFloat64(clean.BundlePrice),
			clean.MaxUsesPerBill, clean.Priority, strings.TrimSpace(clean.Notes), user.ID); err != nil {
			return platform.MapUniqueViolation(err, "รหัสโปรโมชั่นนี้ถูกใช้แล้ว")
		}
		if err := replaceItems(ctx, tx, promotionID, clean.Items); err != nil {
			return err
		}
		meta.EntityType = "promotion"
		meta.EntityID = &promotionID
		meta.Action = "promotion.create"
		meta.After = map[string]any{"code": clean.Code, "name": clean.Name, "promo_type": clean.PromoType}
		return s.audit.Log(ctx, tx, meta)
	})
	return promotionID, err
}

func (s *Service) Update(ctx context.Context, promotionID string, meta audit.LogEntry, input Input) error {
	clean, err := validate(input)
	if err != nil {
		return err
	}
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var beforeJSON string
		if err := tx.QueryRowContext(ctx, `
			SELECT row_to_json(p)::text FROM (
				SELECT code, name, promo_type, active FROM promotions WHERE id=$1
			) p
		`, promotionID).Scan(&beforeJSON); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบโปรโมชั่น")
			}
			return err
		}
		active := true
		if clean.Active != nil {
			active = *clean.Active
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE promotions
			SET code=$2, name=$3, promo_type=$4, branch_id=$5, starts_at=$6, ends_at=$7, active=$8,
			    min_quantity=$9, min_amount=$10, discount_percent=$11, discount_amount=$12,
			    bundle_price=$13, max_uses_per_bill=$14, priority=$15, notes=$16, updated_at=NOW()
			WHERE id=$1
		`, promotionID, clean.Code, clean.Name, clean.PromoType, platform.NullUUID(&clean.BranchID),
			nullableDate(clean.StartsAt), nullableDate(clean.EndsAt), active,
			clean.MinQuantity, platform.Round2(clean.MinAmount), clean.DiscountPercent,
			platform.Round2(clean.DiscountAmount), platform.NullFloat64(clean.BundlePrice),
			clean.MaxUsesPerBill, clean.Priority, strings.TrimSpace(clean.Notes)); err != nil {
			return platform.MapUniqueViolation(err, "รหัสโปรโมชั่นนี้ถูกใช้แล้ว")
		}
		if err := replaceItems(ctx, tx, promotionID, clean.Items); err != nil {
			return err
		}
		meta.EntityType = "promotion"
		meta.EntityID = &promotionID
		meta.Action = "promotion.update"
		meta.Before = beforeJSON
		meta.After = map[string]any{"code": clean.Code, "name": clean.Name, "promo_type": clean.PromoType}
		return s.audit.Log(ctx, tx, meta)
	})
}

func replaceItems(ctx context.Context, tx *sql.Tx, promotionID string, items []ItemInput) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM promotion_items WHERE promotion_id=$1`, promotionID); err != nil {
		return err
	}
	for _, item := range items {
		unitID := strings.TrimSpace(item.UnitID)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO promotion_items (id, promotion_id, product_id, unit_id, quantity, role, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,NOW())
		`, platform.MustUUID(), promotionID, strings.TrimSpace(item.ProductID),
			platform.NullUUID(&unitID), item.Quantity, item.Role); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, promotionID string, meta audit.LogEntry) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM promotions WHERE id=$1`, promotionID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return platform.NewError(http.StatusNotFound, "ไม่พบโปรโมชั่น")
		}
		meta.EntityType = "promotion"
		meta.EntityID = &promotionID
		meta.Action = "promotion.delete"
		return s.audit.Log(ctx, tx, meta)
	})
}

// List returns campaigns for the management screen. activeOnly restricts to
// campaigns currently in their date window, which is what POS asks for.
func (s *Service) List(ctx context.Context, user platform.AuthUser, branchID string, activeOnly bool) ([]map[string]any, error) {
	if branchID == "" && user.BranchID != nil {
		branchID = *user.BranchID
	}
	conditions := []string{"TRUE"}
	args := []any{}
	if branchID != "" {
		args = append(args, branchID)
		conditions = append(conditions, "(pr.branch_id IS NULL OR pr.branch_id = $1)")
	}
	if activeOnly {
		conditions = append(conditions,
			"pr.active",
			"(pr.starts_at IS NULL OR pr.starts_at <= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)",
			"(pr.ends_at IS NULL OR pr.ends_at >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT pr.id::text, pr.code, pr.name, pr.promo_type, COALESCE(pr.branch_id::text,''),
		       COALESCE(b.name,''), pr.starts_at, pr.ends_at, pr.active, pr.min_quantity, pr.min_amount,
		       pr.discount_percent, pr.discount_amount, pr.bundle_price, pr.max_uses_per_bill,
		       pr.priority, pr.notes
		FROM promotions pr
		LEFT JOIN branches b ON b.id = pr.branch_id
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY pr.active DESC, pr.priority DESC, pr.name
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []map[string]any{}
	index := map[string]int{}
	ids := []string{}
	for rows.Next() {
		var id, code, name, promoType, promoBranchID, branchName, notes string
		var startsAt, endsAt sql.NullTime
		var active bool
		var minQuantity, maxUses, priority int
		var minAmount, discountPercent, discountAmount float64
		var bundlePrice sql.NullFloat64
		if err := rows.Scan(&id, &code, &name, &promoType, &promoBranchID, &branchName,
			&startsAt, &endsAt, &active, &minQuantity, &minAmount, &discountPercent,
			&discountAmount, &bundlePrice, &maxUses, &priority, &notes); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id": id, "code": code, "name": name, "promo_type": promoType,
			"branch_id": promoBranchID, "branch_name": branchName,
			"starts_at": nullableDateValue(startsAt), "ends_at": nullableDateValue(endsAt),
			"active": active, "min_quantity": minQuantity, "min_amount": minAmount,
			"discount_percent": discountPercent, "discount_amount": discountAmount,
			"bundle_price": nullableFloatValue(bundlePrice), "max_uses_per_bill": maxUses,
			"priority": priority, "notes": notes, "items": []map[string]any{},
		}
		index[id] = len(items)
		ids = append(ids, id)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return items, nil
	}

	itemRows, err := s.db.QueryContext(ctx, `
		SELECT pi.promotion_id::text, pi.product_id::text, COALESCE(pi.unit_id::text,''),
		       pi.quantity, pi.role, p.name, p.sku,
		       COALESCE(u.unit_name, COALESCE(NULLIF(TRIM(p.unit_name),''),'ชิ้น')),
		       COALESCE(u.conversion_qty, 1)
		FROM promotion_items pi
		INNER JOIN products p ON p.id = pi.product_id
		LEFT JOIN product_units u ON u.id = pi.unit_id
		WHERE pi.promotion_id = ANY($1::uuid[])
		ORDER BY pi.role, p.name
	`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var promotionID, productID, unitID, role, productName, sku, unitName string
		var quantity, conversionQty int
		if err := itemRows.Scan(&promotionID, &productID, &unitID, &quantity, &role,
			&productName, &sku, &unitName, &conversionQty); err != nil {
			return nil, err
		}
		position, ok := index[promotionID]
		if !ok {
			continue
		}
		current, _ := items[position]["items"].([]map[string]any)
		items[position]["items"] = append(current, map[string]any{
			"product_id": productID, "unit_id": unitID, "quantity": quantity, "role": role,
			"product_name": productName, "sku": sku, "unit_name": unitName,
			"conversion_qty": conversionQty,
		})
	}
	return items, itemRows.Err()
}

func nullableDateValue(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time.Format("2006-01-02")
}

func nullableFloatValue(value sql.NullFloat64) any {
	if !value.Valid {
		return nil
	}
	return value.Float64
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c echo.Context) error {
	activeOnly := strings.EqualFold(strings.TrimSpace(c.QueryParam("active_only")), "true")
	items, err := h.service.List(c.Request().Context(), platform.CurrentUser(c),
		strings.TrimSpace(c.QueryParam("branch_id")), activeOnly)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดโปรโมชั่นไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Create(c echo.Context) error {
	var input Input
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	id, err := h.service.Create(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"id": id, "message": "สร้างโปรโมชั่นแล้ว"})
}

func (h *Handler) Update(c echo.Context) error {
	var input Input
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "รูปแบบข้อมูลไม่ถูกต้อง"))
	}
	if err := h.service.Update(c.Request().Context(), c.Param("promotionID"), audit.MetaFromContext(c), input); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกโปรโมชั่นแล้ว")
}

func (h *Handler) Delete(c echo.Context) error {
	if err := h.service.Delete(c.Request().Context(), c.Param("promotionID"), audit.MetaFromContext(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบโปรโมชั่นแล้ว")
}
