package sales

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/customers"
	"pharmacy-erp/backend/internal/platform"
)

// What a sale does for the customer it is sold to: their name on the bill,
// the points it spends and earns, and — for a credit sale — the credit check
// and the due date. Everything here runs inside the checkout transaction with
// the customer row already locked by priceCart.

// customerOnBill fills the bill's customer name and tax id from the register
// when the cashier did not type them.
func customerOnBill(customer *cartCustomer, name, taxID string) (string, string) {
	name, taxID = strings.TrimSpace(name), strings.TrimSpace(taxID)
	if customer == nil {
		return name, taxID
	}
	if name == "" {
		name = customer.Name
	}
	if taxID == "" {
		taxID = customer.TaxID
	}
	return name, taxID
}

func customerIDOf(customer *cartCustomer) sql.NullString {
	if customer == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: customer.ID, Valid: true}
}

// recordLoyalty writes the points this bill spent and earned to the ledger and
// returns what was earned and the balance after. A bill earns on what the
// customer actually pays, after every discount and the points spent.
func recordLoyalty(ctx context.Context, tx *sql.Tx, cart cartResult, invoiceID, branchID, userID string) (int, int, error) {
	customer := cart.Customer
	if customer == nil {
		return 0, 0, nil
	}
	balance := customer.PointsBalance
	if cart.PointsRedeemed > 0 {
		balance -= cart.PointsRedeemed
		if balance < 0 {
			return 0, 0, platform.NewError(http.StatusConflict, "แต้มคงเหลือไม่พอ")
		}
		if err := customers.InsertPointEntry(ctx, tx, customer.ID, branchID, invoiceID, "redeem", -cart.PointsRedeemed, balance,
			fmt.Sprintf("ใช้แต้มเป็นส่วนลด %.2f บาท", cart.PointsDiscount), userID); err != nil {
			return 0, 0, err
		}
	}
	earned := cart.Loyalty.PointsFor(cart.TotalAmount)
	if earned > 0 {
		balance += earned
		if err := customers.InsertPointEntry(ctx, tx, customer.ID, branchID, invoiceID, "earn", earned, balance,
			fmt.Sprintf("ยอดซื้อ %.2f บาท", cart.TotalAmount), userID); err != nil {
			return 0, 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invoices SET points_earned=$2 WHERE id=$1`, invoiceID, earned); err != nil {
		return 0, 0, err
	}
	return earned, balance, nil
}

// creditDecision is the outcome of checking a credit sale against the
// customer's line.
type creditDecision struct {
	Outstanding float64
	Available   float64
	DueDate     time.Time
}

// checkCredit refuses a credit sale the customer's terms do not cover: no
// credit line, a bill already past due, or a total beyond what is left of the
// line. The customer row is locked, so the outstanding figure cannot move
// under a concurrent sale or payment.
func checkCredit(ctx context.Context, tx platform.DBTX, customer *cartCustomer, total float64) (creditDecision, error) {
	if customer == nil {
		return creditDecision{}, platform.NewError(http.StatusBadRequest, "ขายเชื่อได้เฉพาะเมื่อเลือกลูกค้าที่มีวงเงินเครดิต")
	}
	if customer.CreditLimit <= 0 {
		return creditDecision{}, platform.NewError(http.StatusBadRequest, fmt.Sprintf("%s ยังไม่มีวงเงินเครดิต", customer.Name))
	}
	var outstanding float64
	var overdueBills int
	var today time.Time
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(i.total_amount - COALESCE(paid.amount, 0)), 0),
		       COUNT(*) FILTER (WHERE i.due_date < (NOW() AT TIME ZONE 'Asia/Bangkok')::date),
		       (NOW() AT TIME ZONE 'Asia/Bangkok')::date
		FROM invoices i
		LEFT JOIN LATERAL (SELECT SUM(amount) AS amount FROM invoice_payments WHERE invoice_id = i.id) paid ON TRUE
		WHERE i.customer_id = $1 AND i.payment_status <> 'paid'
		  AND i.deleted_at IS NULL AND i.invoice_status = 'issued'
	`, customer.ID).Scan(&outstanding, &overdueBills, &today); err != nil {
		return creditDecision{}, err
	}
	outstanding = platform.Round2(outstanding)
	available := platform.Round2(customer.CreditLimit - outstanding)
	if overdueBills > 0 {
		return creditDecision{}, platform.NewError(http.StatusConflict,
			fmt.Sprintf("%s มีบิลเกินกำหนดชำระ %d บิล กรุณารับชำระก่อนขายเชื่อเพิ่ม", customer.Name, overdueBills))
	}
	if total > available {
		if available < 0 {
			available = 0
		}
		return creditDecision{}, platform.NewError(http.StatusConflict,
			fmt.Sprintf("เกินวงเงินเครดิต: วงเงิน %.2f บาท ค้างชำระ %.2f บาท ใช้ได้อีก %.2f บาท", customer.CreditLimit, outstanding, available))
	}
	return creditDecision{
		Outstanding: outstanding,
		Available:   platform.Round2(available - total),
		DueDate:     today.AddDate(0, 0, customer.CreditDays),
	}, nil
}

// reverseInvoiceCustomerEffects undoes what a bill did for its customer when
// the bill is deleted: points it spent come back, points it earned go. A
// credit bill that has already been partly paid is refused instead — that
// money was taken and must be refunded first, not lost with the bill.
func reverseInvoiceCustomerEffects(ctx context.Context, tx *sql.Tx, invoiceID, number, branchID, userID string) error {
	var customerID sql.NullString
	var earned, redeemed, payments int
	var saleType string
	if err := tx.QueryRowContext(ctx, `
		SELECT customer_id::text, points_earned, points_redeemed, sale_type,
		       (SELECT COUNT(*) FROM invoice_payments WHERE invoice_id = $1)
		FROM invoices WHERE id = $1
	`, invoiceID).Scan(&customerID, &earned, &redeemed, &saleType, &payments); err != nil {
		return err
	}
	if saleType == "credit" && payments > 0 {
		return platform.NewError(http.StatusConflict, "บิลขายเชื่อนี้รับชำระไปแล้วบางส่วน ต้องคืนเงินลูกค้าก่อนจึงจะลบได้")
	}
	if !customerID.Valid || (earned == 0 && redeemed == 0) {
		return nil
	}
	var balance int
	if err := tx.QueryRowContext(ctx, `SELECT points_balance FROM customers WHERE id=$1 FOR UPDATE`, customerID.String).Scan(&balance); err != nil {
		return err
	}
	if redeemed > 0 {
		balance += redeemed
		if err := customers.InsertPointEntry(ctx, tx, customerID.String, branchID, invoiceID, "redeem_reversal", redeemed, balance,
			"คืนแต้มที่ใช้ใน "+number+" (ลบบิล)", userID); err != nil {
			return err
		}
	}
	if earned > 0 {
		// Points the member has already spent cannot be taken back; the
		// ledger says how many were short instead of going negative.
		take := earned
		note := "หักแต้มที่ได้จาก " + number + " (ลบบิล)"
		if take > balance {
			note = fmt.Sprintf("%s · แต้มคงเหลือไม่พอ หักได้ %d จาก %d แต้ม", note, balance, earned)
			take = balance
		}
		if take > 0 {
			balance -= take
			if err := customers.InsertPointEntry(ctx, tx, customerID.String, branchID, invoiceID, "earn_reversal", -take, balance,
				note, userID); err != nil {
				return err
			}
		}
	}
	return nil
}

// invoiceCustomerDetails is what a bill says about its customer on screen and
// on the receipt: the member, the points the visit moved, and for a credit
// bill when it is due and how much is still owed.
func invoiceCustomerDetails(ctx context.Context, db platform.DBTX, invoiceID string) (map[string]any, error) {
	var customerID, code, phone, saleType string
	var dueDate sql.NullTime
	var earned, redeemed int
	var pointsDiscount, paid, total float64
	var balance sql.NullInt64
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(i.customer_id::text, ''), COALESCE(c.customer_code, ''), COALESCE(c.phone, ''),
		       i.sale_type, i.due_date, i.points_earned, i.points_redeemed, i.points_discount, c.points_balance,
		       COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id = i.id), 0), i.total_amount
		FROM invoices i LEFT JOIN customers c ON c.id = i.customer_id
		WHERE i.id = $1
	`, invoiceID).Scan(&customerID, &code, &phone, &saleType, &dueDate, &earned, &redeemed, &pointsDiscount, &balance, &paid, &total); err != nil {
		return nil, err
	}
	details := map[string]any{
		"sale_type":       saleType,
		"points_earned":   earned,
		"points_redeemed": redeemed,
		"points_discount": pointsDiscount,
		"paid_amount":     platform.Round2(paid),
		"outstanding":     platform.Round2(total - paid),
	}
	if customerID != "" {
		details["customer_id"] = customerID
		details["customer_code"] = code
		details["customer_phone"] = phone
		if balance.Valid {
			details["points_balance"] = balance.Int64
		}
	}
	if dueDate.Valid {
		details["due_date"] = dueDate.Time.Format("2006-01-02")
	}
	return details, nil
}
