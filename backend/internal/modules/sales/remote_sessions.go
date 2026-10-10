package sales

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	ProductID      string           `json:"product_id"`
	ProductName    string           `json:"product_name"`
	SKU            string           `json:"sku"`
	InventoryLotID string           `json:"inventory_lot_id"`
	LotNumber      string           `json:"lot_number"`
	StockBucket    string           `json:"stock_bucket"`
	Quantity       int              `json:"quantity"`
	UnitID         string           `json:"unit_id,omitempty"`
	UnitName       string           `json:"unit_name,omitempty"`
	ConversionQty  int              `json:"conversion_qty,omitempty"`
	SoldQuantity   int              `json:"sold_quantity,omitempty"`
	UnitPrice      float64          `json:"unit_price"`
	DiscountAmount float64          `json:"discount_amount"`
	Units          []map[string]any `json:"units,omitempty"`
}

type RemoteCart struct {
	Lines              []RemoteCartLine `json:"lines"`
	BillDiscountAmount float64          `json:"bill_discount_amount"`
	FullTaxInvoice     bool             `json:"full_tax_invoice"`
	CustomerName       string           `json:"customer_name"`
	CustomerTaxID      string           `json:"customer_tax_id"`
	IsGovernment       bool             `json:"is_government_mode"`
	Notes              string           `json:"notes"`
	// The member head office picked travels with the cart, so the till
	// settles the same priced bill (tier prices, points) it was shown.
	CustomerID   string `json:"customer_id,omitempty"`
	RedeemPoints int    `json:"redeem_points,omitempty"`
}

type RemoteSessionRequest struct {
	BranchID    string     `json:"branch_id"`
	SessionID   string     `json:"session_id"`
	CartVersion int64      `json:"cart_version"`
	Cart        RemoteCart `json:"cart"`
}

// RemoteSessionPayment is all the branch till supplies: how the customer paid.
type RemoteSessionPayment struct {
	SessionID      string  `json:"session_id"`
	BranchID       string  `json:"branch_id"`
	PaymentType    string  `json:"payment_type"`
	TenderedAmount float64 `json:"tendered_amount"`
	TransferAmount float64 `json:"transfer_amount"`
	ReferenceCode  string  `json:"reference_code"`
	PaymentNote    string  `json:"payment_note"`
}

func remoteSessionRow(scanner interface{ Scan(...any) error }) (map[string]any, error) {
	var id, branchID, branchName, operatorID, operatorName, status string
	var cartRaw []byte
	var invoiceID, invoiceNumber sql.NullString
	var updatedAt time.Time
	var cartVersion int64
	if err := scanner.Scan(&id, &branchID, &branchName, &operatorID, &operatorName, &status, &cartRaw, &invoiceID, &invoiceNumber, &cartVersion, &updatedAt); err != nil {
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
		"operator_id":   operatorID,
		"operator_name": operatorName,
		"status":        status,
		"cart":          cart,
		"updated_at":    updatedAt,
		"cart_version":  cartVersion,
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
	SELECT s.id::text, s.branch_id::text, b.name, s.operator_id::text, u.full_name, s.status, s.cart,
	       i.id::text, i.invoice_number, s.cart_version, s.updated_at
	FROM remote_sale_sessions s
	INNER JOIN branches b ON b.id = s.branch_id
	INNER JOIN users u ON u.id = s.operator_id
	LEFT JOIN invoices i ON i.id = s.invoice_id
`

// SaveRemoteSession opens or replaces the cart waiting at a branch's till.
func (s *Service) SaveRemoteSession(ctx context.Context, user platform.AuthUser, input RemoteSessionRequest) (map[string]any, error) {
	branchID, err := platform.MustBranchID(user, strings.TrimSpace(input.BranchID))
	if err != nil {
		return nil, err
	}
	if err := validateSalesBranch(ctx, s.db, branchID); err != nil {
		return nil, err
	}
	if input.Cart.Lines == nil {
		input.Cart.Lines = []RemoteCartLine{}
	}
	cartRaw, err := json.Marshal(input.Cart)
	if err != nil {
		return nil, platform.NewError(http.StatusBadRequest, "ตะกร้าไม่ถูกต้อง")
	}
	var sessionID string
	if strings.TrimSpace(input.SessionID) != "" {
		if input.CartVersion <= 0 {
			return nil, platform.NewError(http.StatusConflict, "ตะกร้ารีโมตไม่มี version กรุณาโหลดรายการใหม่")
		}
		err = s.db.QueryRowContext(ctx, `
			UPDATE remote_sale_sessions
			SET cart = $4, cart_version=cart_version+1, updated_at = NOW()
			WHERE id = $1 AND branch_id = $2 AND operator_id=$3
			  AND status = 'open' AND cart_version=$5
			RETURNING id::text
		`, input.SessionID, branchID, user.ID, cartRaw, input.CartVersion).Scan(&sessionID)
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusConflict, "รายการรีโมตเปลี่ยนผู้ดำเนินการ เปลี่ยน version หรือปิดแล้ว กรุณาโหลดใหม่")
		}
		if err != nil {
			return nil, platform.WrapError(http.StatusInternalServerError, "บันทึกตะกร้ารีโมตไม่สำเร็จ", err)
		}
		return s.RemoteSessionForBranch(ctx, branchID)
	}
	// One open cart per branch: keep editing it rather than stacking sessions
	// the cashier would have to choose between.
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO remote_sale_sessions (id, branch_id, operator_id, status, cart)
		VALUES ($1, $2, $3, 'open', $4)
		RETURNING id::text
	`, platform.MustUUID(), branchID, user.ID, cartRaw).Scan(&sessionID)
	if err != nil {
		return nil, platform.MapUniqueViolation(err, "สาขานี้มีรายการรีโมตที่กำลังใช้งาน กรุณาโหลดรายการเดิม")
	}
	return s.RemoteSessionForBranch(ctx, branchID)
}

// CancelRemoteSession withdraws the cart from the branch till.
func (s *Service) CancelRemoteSession(ctx context.Context, user platform.AuthUser, branchID, sessionID string, cartVersion int64) error {
	if strings.TrimSpace(branchID) == "" {
		return platform.NewError(http.StatusBadRequest, "กรุณาเลือกสาขา")
	}
	if strings.TrimSpace(sessionID) == "" || cartVersion <= 0 {
		return platform.NewError(http.StatusBadRequest, "กรุณาระบุรายการรีโมตและ version")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE remote_sale_sessions SET status = 'cancelled', updated_at = NOW()
		WHERE id=$1 AND branch_id=$2 AND operator_id=$3
		  AND cart_version=$4 AND status='open'
	`, sessionID, branchID, user.ID, cartVersion)
	if err != nil {
		return platform.WrapError(http.StatusInternalServerError, "ยกเลิกการรีโมตไม่สำเร็จ", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return platform.NewError(http.StatusConflict, "รายการรีโมตเปลี่ยนผู้ดำเนินการ เปลี่ยน version หรือปิดแล้ว")
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
	branchID := ""
	if user.Portal == "pos" {
		if user.BranchID == nil || *user.BranchID == "" {
			return nil, platform.NewError(http.StatusForbidden, "บัญชีนี้ไม่ได้ผูกกับสาขา")
		}
		branchID = *user.BranchID
	} else if platform.HasPermission(user, "invoice.create.remote") {
		var err error
		branchID, err = platform.MustBranchID(user, payment.BranchID)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, platform.NewError(http.StatusForbidden, "เฉพาะพนักงานขายหน้าร้านหรือสำนักงานใหญ่เท่านั้น")
	}
	if err := validateSalesBranch(ctx, s.db, branchID); err != nil {
		return nil, err
	}
	payment.SessionID = strings.TrimSpace(payment.SessionID)
	payment.BranchID = branchID
	payment.PaymentType = strings.TrimSpace(payment.PaymentType)
	payment.ReferenceCode = strings.TrimSpace(payment.ReferenceCode)
	payment.PaymentNote = strings.TrimSpace(payment.PaymentNote)
	if payment.PaymentType == "cash" {
		payment.ReferenceCode = ""
	}
	requestRaw, err := json.Marshal(payment)
	if err != nil {
		return nil, platform.NewError(http.StatusBadRequest, "ข้อมูลการชำระเงินไม่ถูกต้อง")
	}
	requestHash := fmt.Sprintf("%x", sha256.Sum256(requestRaw))

	var result map[string]any
	err = platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var sessionID, operatorID, status string
		var cartRaw []byte
		var storedHash sql.NullString
		var storedResult []byte
		var row *sql.Row
		if payment.SessionID != "" {
			row = tx.QueryRowContext(ctx, `
				SELECT id::text, operator_id::text, status, cart, checkout_request_hash, checkout_result
				FROM remote_sale_sessions
				WHERE id = $1 AND branch_id = $2
				FOR UPDATE
			`, payment.SessionID, branchID)
		} else {
			// Backward compatibility for older clients. New clients always send
			// the session id, which is the idempotency key for retries.
			row = tx.QueryRowContext(ctx, `
				SELECT id::text, operator_id::text, status, cart, checkout_request_hash, checkout_result
				FROM remote_sale_sessions
				WHERE branch_id = $1 AND status = 'open'
				ORDER BY updated_at DESC LIMIT 1
				FOR UPDATE
			`, branchID)
		}
		if err := row.Scan(&sessionID, &operatorID, &status, &cartRaw, &storedHash, &storedResult); err == sql.ErrNoRows {
			return platform.NewError(http.StatusNotFound, "ไม่มีรายการรีโมตที่รอรับชำระ")
		} else if err != nil {
			return platform.WrapError(http.StatusInternalServerError, "โหลดตะกร้ารีโมตไม่สำเร็จ", err)
		}
		if user.Portal != "pos" && operatorID != user.ID {
			return platform.NewError(http.StatusConflict, "รายการรีโมตนี้กำลังดำเนินการโดยผู้ใช้อื่น")
		}

		if status == "completed" {
			if !storedHash.Valid || len(storedResult) == 0 {
				return platform.NewError(http.StatusConflict, "รายการรีโมตนี้ชำระแล้ว")
			}
			if storedHash.String != requestHash {
				return platform.NewError(http.StatusConflict, "รายการรีโมตนี้ชำระแล้วด้วยข้อมูลการชำระเงินชุดอื่น")
			}
			if err := json.Unmarshal(storedResult, &result); err != nil {
				return platform.NewError(http.StatusInternalServerError, "ผลการชำระเงินเดิมเสียหาย")
			}
			return nil
		}
		if status != "open" {
			return platform.NewError(http.StatusConflict, "รายการรีโมตนี้ถูกยกเลิกแล้ว")
		}

		cart := RemoteCart{}
		if err := json.Unmarshal(cartRaw, &cart); err != nil {
			return platform.NewError(http.StatusInternalServerError, "ตะกร้ารีโมตเสียหาย")
		}
		if len(cart.Lines) == 0 {
			return platform.NewError(http.StatusBadRequest, "ตะกร้ารีโมตยังไม่มีสินค้า")
		}
		if cart.FullTaxInvoice && strings.TrimSpace(cart.CustomerTaxID) == "" {
			return platform.NewError(http.StatusBadRequest, "ใบกำกับภาษีเต็มรูปต้องระบุเลขประจำตัวผู้เสียภาษี")
		}

		items := make([]LineInput, 0, len(cart.Lines))
		for _, line := range cart.Lines {
			items = append(items, LineInput{
				ProductID:      line.ProductID,
				InventoryLotID: line.InventoryLotID,
				Quantity:       line.Quantity,
				UnitID:         line.UnitID,
				StockBucket:    line.StockBucket,
				DiscountAmount: line.DiscountAmount,
			})
		}
		result, err = s.checkoutInTx(ctx, tx, user, meta, CheckoutRequest{
			BranchID:           branchID,
			CustomerID:         cart.CustomerID,
			RedeemPoints:       cart.RedeemPoints,
			CustomerName:       cart.CustomerName,
			CustomerTaxID:      cart.CustomerTaxID,
			IsGovernment:       cart.IsGovernment,
			FullTaxInvoice:     cart.FullTaxInvoice,
			Items:              items,
			BillDiscountAmount: cart.BillDiscountAmount,
			PaymentType:        payment.PaymentType,
			TenderedAmount:     payment.TenderedAmount,
			TransferAmount:     payment.TransferAmount,
			ReferenceCode:      payment.ReferenceCode,
			DocumentNote:       cart.Notes,
			PaymentNote:        payment.PaymentNote,
		})
		if err != nil {
			return err
		}
		resultRaw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		invoiceID, _ := result["invoice_id"].(string)
		update, err := tx.ExecContext(ctx, `
			UPDATE remote_sale_sessions
			SET status = 'completed', invoice_id = NULLIF($2,'')::uuid,
			    checkout_request_hash = $3, checkout_result = $4::jsonb,
			    updated_at = NOW()
			WHERE id = $1 AND status = 'open'
		`, sessionID, invoiceID, requestHash, string(resultRaw))
		if err != nil {
			return platform.WrapError(http.StatusInternalServerError, "ปิดรายการรีโมตไม่สำเร็จ", err)
		}
		if affected, _ := update.RowsAffected(); affected != 1 {
			return platform.NewError(http.StatusConflict, "รายการรีโมตถูกชำระหรือยกเลิกแล้ว")
		}
		return nil
	})
	return result, err
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
	user := platform.CurrentUser(c)
	branchID, err := platform.MustBranchID(user, c.QueryParam("branch_id"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	cartVersion, err := strconv.ParseInt(c.QueryParam("cart_version"), 10, 64)
	if err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "cart_version ไม่ถูกต้อง"))
	}
	if err := h.service.CancelRemoteSession(c.Request().Context(), user, branchID, c.QueryParam("session_id"), cartVersion); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"message": "ยกเลิกการรีโมตแล้ว"})
}

// AdminRemoteSession lets head office watch the branch it is selling through.
func (h *Handler) AdminRemoteSession(c echo.Context) error {
	branchID, err := platform.MustBranchID(platform.CurrentUser(c), c.QueryParam("branch_id"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	item, err := h.service.RemoteSessionForBranch(c.Request().Context(), branchID)
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

// AdminCheckout is head office settling an ordinary HQ-pickup bill. Remote
// carts use CheckoutRemoteSession so closing the session and creating the sale
// remain atomic; this handler must not cancel an unrelated branch cart.
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
