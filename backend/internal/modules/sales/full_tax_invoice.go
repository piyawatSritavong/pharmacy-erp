package sales

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

// ใบกำกับภาษีอย่างย่อ -> ใบกำกับภาษีเต็มรูป.
//
// A customer given an abbreviated tax invoice at the counter may come back and
// ask for a full one. The sale itself does not change — the goods left, the
// money came in — but the document does, so the abbreviated bill is CANCELLED
// and a full tax invoice is issued in its place. That is what a tax audit
// expects to find: a voided slip and its replacement, joined both ways.
//
// Two limits keep this honest:
//
//   - same calendar day only. A full tax invoice dated to a day the shop has
//     already reported is a different problem entirely, so the door shuts at
//     midnight, Bangkok time.
//   - not once a month-end close has touched the bill. The close rewrites and
//     renumbers bills and freezes a snapshot of them; reissuing afterwards would
//     leave the snapshot describing a bill that no longer exists.
//
// Stock is deliberately untouched: the goods were handed over at the original
// sale, and the replacement bill records the same lines from the same lots.

type FullTaxInvoiceRequest struct {
	CustomerName  string `json:"customer_name"`
	CustomerTaxID string `json:"customer_tax_id"`
}

func (s *Service) UpgradeToFullTaxInvoice(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, invoiceID string, input FullTaxInvoiceRequest) (map[string]any, error) {
	taxID := strings.TrimSpace(input.CustomerTaxID)
	if taxID == "" {
		return nil, platform.NewError(http.StatusBadRequest, "ใบกำกับภาษีเต็มรูปต้องระบุเลขประจำตัวผู้เสียภาษี")
	}
	customerName := strings.TrimSpace(input.CustomerName)
	if customerName == "" {
		return nil, platform.NewError(http.StatusBadRequest, "ใบกำกับภาษีเต็มรูปต้องระบุชื่อผู้ซื้อ")
	}

	result := map[string]any{}
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var branchID, invoiceNumber, taxInvoiceType, invoiceStatus, paymentStatus string
		var requestFullTax, isGovernment bool
		var subtotal, taxRate, taxAmount, totalAmount, billDiscount, lineDiscount, promotionDiscount, giveawayCost float64
		var issuedAt, createdAt time.Time
		var deletedAt sql.NullTime
		var paymentMethod sql.NullString
		if err := tx.QueryRowContext(ctx, `
			SELECT branch_id::text,invoice_number,tax_invoice_type,invoice_status,payment_status,
			       request_full_tax_invoice,is_government_mode,
			       subtotal,tax_rate,tax_amount,total_amount,
			       bill_discount_amount,line_discount_total,promotion_discount_total,giveaway_cost_total,
			       issued_at,created_at,deleted_at,payment_method
			FROM invoices WHERE id=$1 FOR UPDATE
		`, invoiceID).Scan(&branchID, &invoiceNumber, &taxInvoiceType, &invoiceStatus, &paymentStatus,
			&requestFullTax, &isGovernment,
			&subtotal, &taxRate, &taxAmount, &totalAmount,
			&billDiscount, &lineDiscount, &promotionDiscount, &giveawayCost,
			&issuedAt, &createdAt, &deletedAt, &paymentMethod); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบใบขายนี้")
			}
			return err
		}

		if user.BranchID != nil && user.Scope != "global" && branchID != *user.BranchID {
			return platform.NewError(http.StatusForbidden, "ออกใบกำกับภาษีข้ามสาขาไม่ได้")
		}
		if deletedAt.Valid || invoiceStatus != "issued" {
			return platform.NewError(http.StatusConflict, "ใบขายนี้ถูกยกเลิกหรือถูกซ่อนไปแล้ว")
		}
		if taxInvoiceType == "full" || requestFullTax {
			return platform.NewError(http.StatusConflict, "ใบขายนี้เป็นใบกำกับภาษีเต็มรูปอยู่แล้ว")
		}
		if platform.InBangkok(issuedAt).Format("2006-01-02") != platform.InBangkok(time.Now()).Format("2006-01-02") {
			return platform.NewError(http.StatusConflict, "ขอใบกำกับภาษีเต็มรูปย้อนหลังได้เฉพาะภายในวันที่ขายเท่านั้น")
		}
		var closed bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reconciliation_invoice_snapshots WHERE invoice_id=$1)`, invoiceID).Scan(&closed); err != nil {
			return err
		}
		if closed {
			return platform.NewError(http.StatusConflict, "ใบขายนี้ผ่านการสรุปสิ้นเดือนแล้ว ออกใบกำกับภาษีเต็มรูปย้อนหลังไม่ได้")
		}

		replacementID := platform.MustUUID()
		replacementNumber, err := nextDocumentNumber(ctx, tx, branchID, "invoice", time.Now().UTC())
		if err != nil {
			return err
		}

		// The replacement keeps the original sale's timestamps: it is the same
		// sale, so it must land in the same day's books and the same month-end
		// period as the bill it replaces.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invoices (
				id,branch_id,invoice_number,customer_name,customer_tax_id,payment_status,invoice_status,
				is_government_mode,tax_invoice_type,request_full_tax_invoice,
				subtotal,tax_rate,tax_amount,total_amount,
				bill_discount_amount,line_discount_total,promotion_discount_total,giveaway_cost_total,
				payment_method,created_by,issued_at,created_at,updated_at,replaces_invoice_id
			) VALUES ($1,$2,$3,$4,$5,$6,'issued',$7,'full',TRUE,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,NOW(),$20)
		`, replacementID, branchID, replacementNumber, customerName, taxID, paymentStatus,
			isGovernment, subtotal, taxRate, taxAmount, totalAmount,
			billDiscount, lineDiscount, promotionDiscount, giveawayCost,
			paymentMethod, user.ID, issuedAt, createdAt, invoiceID); err != nil {
			return err
		}

		// Same lines, same lots, same money — copied, not re-sold, so no stock
		// moves a second time.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO invoice_items (
				id,invoice_id,product_id,alias_id,actual_product_name,display_name,
				quantity,stock_bucket,unit_price,line_subtotal,tax_rate,tax_amount,
				line_total,price_source,override_reason,cost_snapshot,inventory_lot_id,
				lot_number_snapshot,lot_received_at_snapshot,lot_expires_on_snapshot,
				unit_id,unit_name_snapshot,unit_conversion_qty,sold_quantity,sold_unit_price,
				discount_amount,bill_discount_share,is_giveaway,promotion_id,promotion_name_snapshot,created_at
			)
			SELECT gen_random_uuid(),$2,product_id,alias_id,actual_product_name,display_name,
			       quantity,stock_bucket,unit_price,line_subtotal,tax_rate,tax_amount,
			       line_total,price_source,override_reason,cost_snapshot,inventory_lot_id,
			       lot_number_snapshot,lot_received_at_snapshot,lot_expires_on_snapshot,
			       unit_id,unit_name_snapshot,unit_conversion_qty,sold_quantity,sold_unit_price,
			       discount_amount,bill_discount_share,is_giveaway,promotion_id,promotion_name_snapshot,created_at
			FROM invoice_items WHERE invoice_id=$1 AND reconciliation_removed_at IS NULL
			ORDER BY created_at
		`, invoiceID, replacementID); err != nil {
			return err
		}

		// The money follows the document it belongs to, so each bill's payments
		// tie to exactly one bill and the takings are not counted twice.
		if _, err := tx.ExecContext(ctx, `UPDATE invoice_payments SET invoice_id=$2 WHERE invoice_id=$1`, invoiceID, replacementID); err != nil {
			return err
		}

		// The voided number is retired, not held: the unique index on
		// invoice_number covers every row with deleted_at IS NULL, and a
		// month-end close compacts surviving numbers into the gaps it frees.
		// A cancelled bill that kept its number would sit in that index and
		// collide with the compaction. Marking it -VOID frees the number while
		// original_invoice_number preserves what the customer was handed.
		if _, err := tx.ExecContext(ctx, `
			UPDATE invoices
			SET invoice_status='cancelled',
			    original_invoice_number=COALESCE(original_invoice_number,invoice_number),
			    invoice_number=invoice_number||'-VOID',
			    replaced_by_invoice_id=$2,
			    updated_at=NOW()
			WHERE id=$1
		`, invoiceID, replacementID); err != nil {
			return err
		}

		result = map[string]any{
			"invoice_id":              replacementID,
			"invoice_number":          replacementNumber,
			"replaces_invoice_id":     invoiceID,
			"replaces_invoice_number": invoiceNumber,
			"tax_invoice_type":        "full",
			"customer_name":           customerName,
			"customer_tax_id":         taxID,
			"total_amount":            totalAmount,
			"message":                 "ยกเลิกใบกำกับภาษีอย่างย่อ และออกใบกำกับภาษีเต็มรูปแล้ว",
		}
		meta.EntityType = "invoice"
		meta.EntityID = &replacementID
		meta.BranchID = &branchID
		meta.Action = "full_tax_invoice_reissued"
		meta.Before = map[string]any{"invoice_id": invoiceID, "invoice_number": invoiceNumber, "tax_invoice_type": taxInvoiceType, "invoice_status": "issued"}
		meta.After = map[string]any{"invoice_id": replacementID, "invoice_number": replacementNumber, "tax_invoice_type": "full", "cancelled_invoice_status": "cancelled", "customer_tax_id": taxID}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *Handler) UpgradeToFullTaxInvoice(c echo.Context) error {
	var input FullTaxInvoiceRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลผู้ซื้อไม่ถูกต้อง"))
	}
	result, err := h.service.UpgradeToFullTaxInvoice(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("invoiceID"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, result)
}
