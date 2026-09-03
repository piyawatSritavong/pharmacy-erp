package sales

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

// รีโมตหน้าร้าน — head office builds a cart in a branch's name and the branch's
// own till collects the money.
//
// What travels between the two screens is the *cart*, not the operator's input.
// Mirroring clicks and pointer positions would break the moment the two screens
// differ in size or scroll offset, or a product moved in the grid, and would
// leave nothing to audit; a stored cart survives a refresh on either side and
// records who opened it. The branch till never edits it — it reads the cart the
// server holds and takes payment — so what the customer pays for is exactly
// what head office rang up.

type RemoteCartLine struct {
	ProductID      string  `json:"product_id"`
	ProductName    string  `json:"product_name"`
	SKU            string  `json:"sku"`
	InventoryLotID string  `json:"inventory_lot_id"`
	LotNumber      string  `json:"lot_number"`
	StockBucket    string  `json:"stock_bucket"`
	Quantity       int     `json:"quantity"`
	UnitPrice      float64 `json:"unit_price"`
	DiscountAmount float64 `json:"discount_amount"`
}

type RemoteCart struct {
	Lines              []RemoteCartLine `json:"lines"`
	BillDiscountAmount float64          `json:"bill_discount_amount"`
	FullTaxInvoice     bool             `json:"full_tax_invoice"`
	CustomerName       string           `json:"customer_name"`
	CustomerTaxID      string           `json:"customer_tax_id"`
}

type RemoteSessionRequest struct {
	BranchID string     `json:"branch_id"`
	Cart     RemoteCart `json:"cart"`
}

// RemoteSessionPayment is all the branch till supplies: how the customer paid.
type RemoteSessionPayment struct {
	PaymentType    string  `json:"payment_type"`
	TenderedAmount float64 `json:"tendered_amount"`
	TransferAmount float64 `json:"transfer_amount"`
	ReferenceCode  string  `json:"reference_code"`
}

func remoteSessionRow(scanner interface{ Scan(...any) error }) (map[string]any, error) {
	var id, branchID, branchName, operatorName, status string
	var cartRaw []byte
	var invoiceID, invoiceNumber sql.NullString
	var updatedAt time.Time
	if err := scanner.Scan(&id, &branchID, &branchName, &operatorName, &status, &cartRaw, &invoiceID, &invoiceNumber, &updatedAt); err != nil {
		return nil, err
	}
	cart := RemoteCart{}
	if len(cartRaw) > 0 {
		_ = json.Unmarshal(cartRaw, &cart)
	}
	if cart.Lines == nil {
		cart.Lines = []RemoteCartLine{}
	}
	item := map[string]any{
		"id":            id,
		"branch_id":     branchID,
		"branch_name":   branchName,
		"operator_name": operatorName,
		"status":        status,
		"cart":          cart,
		"updated_at":    updatedAt,
	}
	if invoiceID.Valid {
		item["invoice_id"] = invoiceID.String
	}
	if invoiceNumber.Valid {
		item["invoice_number"] = invoiceNumber.String
	}
	return item, nil
}

const remoteSessionSelect = `
	SELECT s.id::text, s.branch_id::text, b.name, u.full_name, s.status, s.cart,
	       i.id::text, i.invoice_number, s.updated_at
	FROM remote_sale_sessions s
	INNER JOIN branches b ON b.id = s.branch_id
	INNER JOIN users u ON u.id = s.operator_id
	LEFT JOIN invoices i ON i.id = s.invoice_id
`

// SaveRemoteSession opens or replaces the cart waiting at a branch's till.
func (s *Service) SaveRemoteSession(ctx context.Context, user platform.AuthUser, input RemoteSessionRequest) (map[string]any, error) {
	branchID := strings.TrimSpace(input.BranchID)
	if branchID == "" {
		return nil, platform.NewError(http.StatusBadRequest, "กรุณาเลือกสาขาที่จะเปิดขาย")
	}
	if input.Cart.Lines == nil {
		input.Cart.Lines = []RemoteCartLine{}
	}
	cartRaw, err := json.Marshal(input.Cart)
	if err != nil {
		return nil, platform.NewError(http.StatusBadRequest, "ตะกร้าไม่ถูกต้อง")
	}
	var sessionID string
	// One open cart per branch: keep editing it rather than stacking sessions
	// the cashier would have to choose between.
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO remote_sale_sessions (id, branch_id, operator_id, status, cart)
		VALUES ($1, $2, $3, 'open', $4)
		ON CONFLICT (branch_id) WHERE status = 'open'
		DO UPDATE SET cart = EXCLUDED.cart, operator_id = EXCLUDED.operator_id, updated_at = NOW()
		RETURNING id::text
	`, platform.MustUUID(), branchID, user.ID, cartRaw).Scan(&sessionID)
	if err != nil {
		return nil, platform.WrapError(http.StatusInternalServerError, "บันทึกตะกร้ารีโมตไม่สำเร็จ", err)
	}
	return s.RemoteSessionForBranch(ctx, branchID)
}

// CancelRemoteSession withdraws the cart from the branch till.
func (s *Service) CancelRemoteSession(ctx context.Context, branchID string) error {
	if strings.TrimSpace(branchID) == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณาเลือกสาขา")
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE remote_sale_sessions SET status = 'cancelled', updated_at = NOW()
		WHERE branch_id = $1 AND status = 'open'
	`, branchID); err != nil {
		return platform.WrapError(http.StatusInternalServerError, "ยกเลิกการรีโมตไม่สำเร็จ", err)
	}
	return nil
}

// RemoteSessionForBranch returns the branch's latest session — the open cart
// when there is one, otherwise the last one closed, so head office can see that
// the till has taken the money.
func (s *Service) RemoteSessionForBranch(ctx context.Context, branchID string) (map[string]any, error) {
	if strings.TrimSpace(branchID) == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, remoteSessionSelect+`
		WHERE s.branch_id = $1
		ORDER BY (s.status = 'open') DESC, s.updated_at DESC
		LIMIT 1
	`, branchID)
	item, err := remoteSessionRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, platform.WrapError(http.StatusInternalServerError, "โหลดสถานะการรีโมตไม่สำเร็จ", err)
	}
	return item, nil
}

// CheckoutRemoteSession is the branch till's only move: take the cart the
// server holds and collect payment for it. The cart itself is never read from
// the request, so the till cannot alter what head office rang up.
func (s *Service) CheckoutRemoteSession(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, payment RemoteSessionPayment) (map[string]any, error) {
	if user.BranchID == nil || *user.BranchID == "" {
		return nil, platform.NewError(http.StatusForbidden, "บัญชีนี้ไม่ได้ผูกกับสาขา")
	}
	branchID := *user.BranchID

	var sessionID string
	var cartRaw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, cart FROM remote_sale_sessions
		WHERE branch_id = $1 AND status = 'open'
	`, branchID).Scan(&sessionID, &cartRaw)
	if err == sql.ErrNoRows {
		return nil, platform.NewError(http.StatusNotFound, "ไม่มีรายการรีโมตที่รอรับชำระ")
	}
	if err != nil {
		return nil, platform.WrapError(http.StatusInternalServerError, "โหลดตะกร้ารีโมตไม่สำเร็จ", err)
	}

	cart := RemoteCart{}
	if err := json.Unmarshal(cartRaw, &cart); err != nil {
		return nil, platform.NewError(http.StatusInternalServerError, "ตะกร้ารีโมตเสียหาย")
	}
	if len(cart.Lines) == 0 {
		return nil, platform.NewError(http.StatusBadRequest, "ตะกร้ารีโมตยังไม่มีสินค้า")
	}

	items := make([]LineInput, 0, len(cart.Lines))
	for _, line := range cart.Lines {
		items = append(items, LineInput{
			ProductID:      line.ProductID,
			InventoryLotID: line.InventoryLotID,
			Quantity:       line.Quantity,
			StockBucket:    line.StockBucket,
			DiscountAmount: line.DiscountAmount,
		})
	}
	result, err := s.Checkout(ctx, user, meta, CheckoutRequest{
		BranchID:           branchID,
		CustomerName:       cart.CustomerName,
		CustomerTaxID:      cart.CustomerTaxID,
		FullTaxInvoice:     cart.FullTaxInvoice,
		Items:              items,
		BillDiscountAmount: cart.BillDiscountAmount,
		PaymentType:        payment.PaymentType,
		TenderedAmount:     payment.TenderedAmount,
		TransferAmount:     payment.TransferAmount,
		ReferenceCode:      payment.ReferenceCode,
	})
	if err != nil {
		return nil, err
	}

	invoiceID, _ := result["invoice_id"].(string)
	if _, err := s.db.ExecContext(ctx, `
		UPDATE remote_sale_sessions
		SET status = 'completed', invoice_id = NULLIF($2,'')::uuid, updated_at = NOW()
		WHERE id = $1
	`, sessionID, invoiceID); err != nil {
		// The sale is already recorded; leaving the session open would only
		// invite a second charge, so surface it.
		return nil, platform.WrapError(http.StatusInternalServerError, "ปิดรายการรีโมตไม่สำเร็จ", err)
	}
	return result, nil
}

// posPresenceWindow is how stale a heartbeat may be before the till counts as
// closed. The POS polls every few seconds, so this tolerates a handful of
// missed beats without declaring a busy shop offline.
const posPresenceWindow = 60 * time.Second

// TouchPosPresence records that a branch's till is on the sales screen.
func (s *Service) TouchPosPresence(ctx context.Context, user platform.AuthUser) {
	if user.BranchID == nil || *user.BranchID == "" {
		return
	}
	// Best effort: a missed heartbeat only costs a moment of looking offline.
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO pos_terminal_presence (branch_id, user_id, last_seen_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (branch_id)
		DO UPDATE SET user_id = EXCLUDED.user_id, last_seen_at = NOW()
	`, *user.BranchID, user.ID)
}

// OnlineBranches lists the tills head office may sell through right now.
func (s *Service) OnlineBranches(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.branch_id::text, b.name, u.full_name, p.last_seen_at
		FROM pos_terminal_presence p
		INNER JOIN branches b ON b.id = p.branch_id
		INNER JOIN users u ON u.id = p.user_id
		WHERE p.last_seen_at > NOW() - $1::interval
		ORDER BY b.name
	`, posPresenceWindow.String())
	if err != nil {
		return nil, platform.WrapError(http.StatusInternalServerError, "โหลดสถานะเครื่อง POS ไม่สำเร็จ", err)
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var branchID, branchName, cashierName string
		var lastSeen time.Time
		if err := rows.Scan(&branchID, &branchName, &cashierName, &lastSeen); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"branch_id":    branchID,
			"branch_name":  branchName,
			"cashier_name": cashierName,
			"last_seen_at": lastSeen,
		})
	}
	return items, rows.Err()
}

// --- handlers -------------------------------------------------------------

func (h *Handler) SaveRemoteSession(c echo.Context) error {
	var input RemoteSessionRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลตะกร้าไม่ถูกต้อง"))
	}
	result, err := h.service.SaveRemoteSession(c.Request().Context(), platform.CurrentUser(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) CancelRemoteSession(c echo.Context) error {
	branchID := strings.TrimSpace(c.QueryParam("branch_id"))
	if err := h.service.CancelRemoteSession(c.Request().Context(), branchID); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"message": "ยกเลิกการรีโมตแล้ว"})
}

// AdminRemoteSession lets head office watch the branch it is selling through.
func (h *Handler) AdminRemoteSession(c echo.Context) error {
	item, err := h.service.RemoteSessionForBranch(c.Request().Context(), strings.TrimSpace(c.QueryParam("branch_id")))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"item": item})
}

// PosRemoteSession is the till asking whether head office has left it a cart.
func (h *Handler) PosRemoteSession(c echo.Context) error {
	user := platform.CurrentUser(c)
	branchID := ""
	if user.BranchID != nil {
		branchID = *user.BranchID
	}
	// This poll is the till's heartbeat: it only runs while the sales screen is
	// open, which is exactly when head office may sell through it.
	h.service.TouchPosPresence(c.Request().Context(), user)
	item, err := h.service.RemoteSessionForBranch(c.Request().Context(), branchID)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"item": item})
}

// OnlineBranches tells head office which tills are open to sell through.
func (h *Handler) OnlineBranches(c echo.Context) error {
	items, err := h.service.OnlineBranches(c.Request().Context())
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

// AdminCheckout is head office settling the bill at its own counter. Whatever
// cart it had left waiting at the branch till is withdrawn in the same request,
// so the branch can never collect a second time for a bill already paid.
func (h *Handler) AdminCheckout(c echo.Context) error {
	var input CheckoutRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลการขายไม่ถูกต้อง"))
	}
	user := platform.CurrentUser(c)
	if input.BranchID == "" && user.BranchID != nil {
		input.BranchID = *user.BranchID
	}
	result, err := h.service.Checkout(c.Request().Context(), user, audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	if input.BranchID != "" {
		// Best effort: the sale is already recorded, and an orphaned open cart
		// is the one thing that could cause a double charge.
		_ = h.service.CancelRemoteSession(c.Request().Context(), input.BranchID)
	}
	result["message"] = "ชำระเงินและออกใบเสร็จแล้ว"
	return platform.JSON(c, http.StatusCreated, result)
}

func (h *Handler) CheckoutRemoteSession(c echo.Context) error {
	var payment RemoteSessionPayment
	if err := c.Bind(&payment); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลการชำระเงินไม่ถูกต้อง"))
	}
	result, err := h.service.CheckoutRemoteSession(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), payment)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, result)
}
