// Package parkedbills implements พักบิล — a POS-only "suspend this cart"
// buffer. It never touches inventory, never issues a document number, and
// never reaches accounting; parking and resuming are pure UI state that
// happens to survive a page reload and a shift handover.
package parkedbills

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

// Parked bills are a scratchpad, not a record — anything older than this is
// assumed abandoned and disappears on the next read.
const lifetime = 24 * time.Hour

type Item struct {
	ProductID      string  `json:"product_id"`
	InventoryLotID string  `json:"inventory_lot_id"`
	Quantity       int     `json:"quantity"`
	UnitPrice      float64 `json:"unit_price"`
	DiscountAmount float64 `json:"discount_amount"`
	ProductName    string  `json:"product_name"`
	SKU            string  `json:"sku"`
	LotNumber      string  `json:"lot_number"`
}

type Input struct {
	CustomerName   string `json:"customer_name"`
	CustomerTaxID  string `json:"customer_tax_id"`
	FullTaxInvoice bool   `json:"full_tax_invoice"`
	Note           string `json:"note"`
	Items          []Item `json:"items"`
}

type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

// branchOf resolves the caller's own branch. Parking is a counter activity,
// so an account with no branch (global back-office roles) has nothing to park
// against — this is what keeps the endpoints POS-shaped without adding a
// permission (the routes are intentionally permission-free).
func branchOf(user platform.AuthUser) (string, error) {
	if user.BranchID == nil || strings.TrimSpace(*user.BranchID) == "" {
		return "", platform.NewError(http.StatusBadRequest, "บัญชีนี้ไม่ได้ผูกกับสาขา จึงพักบิลไม่ได้")
	}
	return *user.BranchID, nil
}

func (s *Service) Create(ctx context.Context, user platform.AuthUser, input Input) (map[string]any, error) {
	branchID, err := branchOf(user)
	if err != nil {
		return nil, err
	}
	if len(input.Items) == 0 {
		return nil, platform.NewError(http.StatusBadRequest, "ไม่มีรายการสินค้าให้พัก")
	}
	total := 0.0
	count := 0
	for _, item := range input.Items {
		if strings.TrimSpace(item.ProductID) == "" || item.Quantity <= 0 {
			return nil, platform.NewError(http.StatusBadRequest, "รายการสินค้าที่พักไม่ถูกต้อง")
		}
		total += item.UnitPrice*float64(item.Quantity) - item.DiscountAmount
		count += item.Quantity
	}
	encoded, err := json.Marshal(input.Items)
	if err != nil {
		return nil, err
	}
	id := platform.MustUUID()
	expires := time.Now().Add(lifetime)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO parked_bills (id, branch_id, created_by, customer_name, customer_tax_id,
		                          full_tax_invoice, note, items, item_count, estimated_total,
		                          expires_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,NOW(),NOW())
	`, id, branchID, user.ID, strings.TrimSpace(input.CustomerName), strings.TrimSpace(input.CustomerTaxID),
		input.FullTaxInvoice, strings.TrimSpace(input.Note), string(encoded), count,
		platform.Round2(total), expires); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "expires_at": expires.Format(time.RFC3339)}, nil
}

// purgeExpired runs on every read: no scheduler, and the table stays small.
func (s *Service) purgeExpired(ctx context.Context) {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM parked_bills WHERE expires_at <= NOW()`)
}

// List returns the caller's branch's live parked bills. Scoped to the branch
// rather than the individual cashier so a bill survives a shift handover —
// each row carries who parked it.
func (s *Service) List(ctx context.Context, user platform.AuthUser) ([]map[string]any, error) {
	branchID, err := branchOf(user)
	if err != nil {
		return nil, err
	}
	s.purgeExpired(ctx)
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id::text, p.customer_name, p.note, p.item_count, p.estimated_total,
		       p.expires_at, p.created_at, COALESCE(u.full_name, ''), p.created_by::text
		FROM parked_bills p
		LEFT JOIN users u ON u.id = p.created_by
		WHERE p.branch_id = $1 AND p.expires_at > NOW()
		ORDER BY p.created_at DESC
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, customer, note, actor, actorID string
		var count int
		var total float64
		var expires, created time.Time
		if err := rows.Scan(&id, &customer, &note, &count, &total, &expires, &created, &actor, &actorID); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"id": id, "customer_name": customer, "note": note,
			"item_count": count, "estimated_total": total,
			"expires_at": expires.Format(time.RFC3339), "created_at": created.Format(time.RFC3339),
			"created_by_name": actor, "created_by": actorID,
			"is_mine": actorID == user.ID,
		})
	}
	return items, rows.Err()
}

// Get returns one parked bill with its lines, for resuming into the cart.
func (s *Service) Get(ctx context.Context, user platform.AuthUser, id string) (map[string]any, error) {
	branchID, err := branchOf(user)
	if err != nil {
		return nil, err
	}
	var customer, taxID, note, raw string
	var fullTax bool
	var created time.Time
	if err := s.db.QueryRowContext(ctx, `
		SELECT customer_name, customer_tax_id, full_tax_invoice, note, items::text, created_at
		FROM parked_bills WHERE id = $1 AND branch_id = $2 AND expires_at > NOW()
	`, id, branchID).Scan(&customer, &taxID, &fullTax, &note, &raw, &created); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบบิลที่พักไว้ หรือหมดอายุแล้ว")
		}
		return nil, err
	}
	var items []Item
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "customer_name": customer, "customer_tax_id": taxID,
		"full_tax_invoice": fullTax, "note": note, "items": items,
		"created_at": created.Format(time.RFC3339),
	}, nil
}

// Delete backs both "abandon" and the cleanup after a successful resume.
func (s *Service) Delete(ctx context.Context, user platform.AuthUser, id string) error {
	branchID, err := branchOf(user)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM parked_bills WHERE id = $1 AND branch_id = $2`, id, branchID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return platform.NewError(http.StatusNotFound, "ไม่พบบิลที่พักไว้")
	}
	return nil
}

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Create(c echo.Context) error {
	var input Input
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลบิลที่พักไม่ถูกต้อง"))
	}
	result, err := h.service.Create(c.Request().Context(), platform.CurrentUser(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	result["message"] = "พักบิลไว้แล้ว"
	return platform.JSON(c, http.StatusCreated, result)
}

func (h *Handler) List(c echo.Context) error {
	items, err := h.service.List(c.Request().Context(), platform.CurrentUser(c))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Get(c echo.Context) error {
	result, err := h.service.Get(c.Request().Context(), platform.CurrentUser(c), c.Param("parkedBillID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) Delete(c echo.Context) error {
	if err := h.service.Delete(c.Request().Context(), platform.CurrentUser(c), c.Param("parkedBillID")); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ยกเลิกบิลที่พักไว้แล้ว")
}
