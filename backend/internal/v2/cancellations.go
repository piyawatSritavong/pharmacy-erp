package v2

import (
	"context"
	"database/sql"
	"strings"
)

type CancellationInput struct {
	BranchID    string `json:"branch_id"`
	DocumentID  string `json:"document_id"`
	Reference   string `json:"reference"`
	Reason      string `json:"reason"`
	ReturnStock bool   `json:"return_stock"`
}
type RefundGoodsLine struct {
	ProductID string `json:"product_id"`
	UnitID    string `json:"unit_id"`
	Quantity  int64  `json:"quantity"`
	VATBPS    int64  `json:"vat_bps"`
}
type RefundInput struct {
	BranchID       string            `json:"branch_id"`
	CancellationID string            `json:"cancellation_id"`
	Method         string            `json:"method"`
	AmountCents    int64             `json:"amount_cents"`
	DrawerID       string            `json:"drawer_id"`
	Reference      string            `json:"reference"`
	Reason         string            `json:"reason"`
	Goods          []RefundGoodsLine `json:"goods"`
}

const cancellationsQuery = `SELECT jsonb_build_object('items',COALESCE(jsonb_agg(row_to_json(x) ORDER BY x.created_at),'[]'::jsonb)) FROM (
 SELECT c.*,d.document_number,d.counterparty_snapshot,
 c.refund_due_cents-COALESCE((SELECT SUM(r.amount_cents) FROM v2_refund_settlements r WHERE r.cancellation_id=c.id),0)::bigint AS pending_cents,
 LEAST(d.issued_on,(c.created_at AT TIME ZONE 'Asia/Bangkok')::date) < (NOW() AT TIME ZONE 'Asia/Bangkok')::date AS cross_day,
 COALESCE((SELECT jsonb_agg(to_jsonb(r)||jsonb_build_object('goods',(SELECT jsonb_agg(row_to_json(g)) FROM v2_refund_goods g WHERE g.settlement_id=r.id)) ORDER BY r.created_at) FROM v2_refund_settlements r WHERE r.cancellation_id=c.id),'[]'::jsonb) AS settlements
 FROM v2_cancellations c JOIN v2_documents d ON d.id=c.document_id WHERE c.branch_id=$1
 )x`

func (s *Service) cancelInvoice(ctx context.Context, tx *sql.Tx, a Actor, in CancellationInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Reference) == "" || strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุอ้างอิงและเหตุผลยกเลิกบิล")
	}
	var kind, status string
	if err = tx.QueryRowContext(ctx, `SELECT kind,status FROM v2_documents WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.DocumentID, b).Scan(&kind, &status); err != nil {
		return nil, err
	}
	if kind != "ar_invoice" || status != "issued" {
		return nil, conflict("ยกเลิกได้เฉพาะใบขาย V2 ที่ออกแล้วและยังไม่ยกเลิก")
	}
	var pending bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM v2_allocations al JOIN v2_cheques ch ON ch.payment_id=al.payment_id WHERE al.document_id=$1 AND ch.status IN ('received','deposited','cleared'))`, in.DocumentID).Scan(&pending); err != nil {
		return nil, err
	}
	// A cleared cheque can still bounce. Do not lose that liability by refunding
	// it without a separately approved bank reconciliation/reversal workflow.
	if pending {
		return nil, conflict("บิลมีเช็คที่ยังผูกอยู่ ต้องจัดการเช็คและยอดชำระก่อนยกเลิก")
	}
	rows, err := tx.QueryContext(ctx, `SELECT l.id::text,l.quantity-COALESCE((SELECT SUM(cl.quantity) FROM v2_document_lines cl JOIN v2_documents cd ON cd.id=cl.document_id WHERE cl.source_line_id=l.id AND cd.kind='ar_credit' AND cd.status='issued'),0)::bigint,l.total_cents-COALESCE((SELECT SUM(cl.total_cents) FROM v2_document_lines cl JOIN v2_documents cd ON cd.id=cl.document_id WHERE cl.source_line_id=l.id AND cd.kind='ar_credit' AND cd.status='issued'),0)::bigint FROM v2_document_lines l WHERE l.document_id=$1 ORDER BY l.id`, in.DocumentID)
	if err != nil {
		return nil, err
	}
	lines := []CreditLine{}
	var remainingValue int64
	for rows.Next() {
		var l CreditLine
		var value int64
		if err = rows.Scan(&l.SourceLineID, &l.Quantity, &value); err != nil {
			break
		}
		if l.Quantity > 0 {
			lines = append(lines, l)
		}
		remainingValue, err = add(remainingValue, value)
		if err != nil {
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	creditID := ""
	if remainingValue > 0 {
		result, e := s.creditNote(ctx, tx, a, CreditInput{BranchID: b, DocumentID: in.DocumentID, Reason: in.Reason, Lines: lines})
		if e != nil {
			return nil, e
		}
		creditID = result.(map[string]any)["id"].(string)
	}
	if in.ReturnStock {
		sources, e := tx.QueryContext(ctx, `SELECT e.id::text,-e.quantity_delta-COALESCE((SELECT SUM(r.base_quantity) FROM v2_stock_returns r WHERE r.source_event_id=e.id),0)::bigint,e.product_id::text FROM v2_stock_events e WHERE e.reference=$1 AND e.branch_id=$2 AND e.event_type='invoice.issue' ORDER BY e.product_id,e.id`, in.DocumentID, b)
		if e != nil {
			return nil, e
		}
		returns := []ReturnInput{}
		stock := []StockLine{}
		for sources.Next() {
			var r ReturnInput
			var p string
			if e = sources.Scan(&r.SourceEventID, &r.BaseQuantity, &p); e != nil {
				break
			}
			if r.BaseQuantity > 0 {
				r.BranchID = b
				r.Reason = in.Reason
				returns = append(returns, r)
				stock = append(stock, StockLine{ProductID: p})
			}
		}
		if e == nil {
			e = sources.Err()
		}
		sources.Close()
		if e != nil {
			return nil, e
		}
		if e = lockStock(ctx, tx, []string{b}, stock); e != nil {
			return nil, e
		}
		for _, r := range returns {
			if _, e = s.returnStock(ctx, tx, a, r); e != nil {
				return nil, e
			}
		}
	}
	var balance int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0)::bigint FROM v2_money_events WHERE document_id=$1`, in.DocumentID).Scan(&balance); err != nil {
		return nil, err
	}
	if balance > 0 {
		return nil, conflict("ยอดลดหนี้ยังไม่ครอบคลุมบิล กรุณาตรวจบัญชีก่อนยกเลิก")
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_cancellations(id,operation_id,branch_id,document_id,credit_document_id,refund_due_cents,stock_returned,reference,reason,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, a.OperationID, b, in.DocumentID, optionalID(creditID), -balance, in.ReturnStock, in.Reference, in.Reason, a.User.ID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE v2_documents SET status='cancelled' WHERE id=$1`, in.DocumentID); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "document_id": in.DocumentID, "refund_due_cents": -balance, "stock_returned": in.ReturnStock}, emit(ctx, tx, a, "invoice.cancelled", map[string]any{"id": id, "branch_id": b, "refund_due_cents": -balance})
}

func (s *Service) settleRefund(ctx context.Context, tx *sql.Tx, a Actor, in RefundInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.AmountCents <= 0 || in.AmountCents > maxInteger || strings.TrimSpace(in.Reference) == "" || strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุยอดคืน อ้างอิง และเหตุผล")
	}
	if in.Method != "cash" && in.Method != "bank_transfer" && in.Method != "goods" {
		return nil, bad("เลือกคืนเงินสด เงินโอน หรือสินค้า")
	}
	if in.Method != "goods" && len(in.Goods) > 0 {
		return nil, bad("รายการสินค้าใช้ได้เฉพาะคืนเป็นสินค้า")
	}
	var doc string
	var due, settled, balance int64
	if err = tx.QueryRowContext(ctx, `SELECT document_id::text,refund_due_cents FROM v2_cancellations WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.CancellationID, b).Scan(&doc, &due); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0)::bigint FROM v2_refund_settlements WHERE cancellation_id=$1`, in.CancellationID).Scan(&settled); err != nil {
		return nil, err
	}
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM v2_documents WHERE id=$1 FOR UPDATE`, doc).Scan(&status); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0)::bigint FROM v2_money_events WHERE document_id=$1`, doc).Scan(&balance); err != nil {
		return nil, err
	}
	if status != "cancelled" || in.AmountCents > due-settled || in.AmountCents > -balance {
		return nil, conflict("ยอดคืนเกินยอดค้างหรือบิลไม่ได้ยกเลิก")
	}
	id, useID := newID(), ""
	units := []UnitSnapshot{}
	values := []int64{}
	if in.Method == "goods" {
		if len(in.Goods) == 0 || len(in.Goods) > 200 {
			return nil, bad("ระบุสินค้าแทนเงินคืน")
		}
		stock := []StockLine{}
		for _, g := range in.Goods {
			stock = append(stock, StockLine{ProductID: g.ProductID})
		}
		if err = lockStock(ctx, tx, []string{b}, stock); err != nil {
			return nil, err
		}
		var total int64
		for _, g := range in.Goods {
			if g.VATBPS < 0 || g.VATBPS > 10000 {
				return nil, bad("VAT ไม่ถูกต้อง")
			}
			u, e := resolveUnit(ctx, tx, g.ProductID, g.UnitID, g.Quantity)
			if e != nil {
				return nil, e
			}
			gross, e := multiply(g.Quantity, u.DefaultPriceCents)
			if e != nil {
				return nil, e
			}
			vat, e := proportion(gross, g.VATBPS, 10000)
			if e != nil {
				return nil, e
			}
			value, e := add(gross, vat)
			if e != nil {
				return nil, e
			}
			total, e = add(total, value)
			if e != nil {
				return nil, e
			}
			units = append(units, u)
			values = append(values, value)
		}
		if total != in.AmountCents {
			return nil, conflict("มูลค่าสินค้ารวม VAT ต้องเท่ากับยอดคืนที่เลือก")
		}
		for i, g := range in.Goods {
			lots, e := availableLots(ctx, tx, b, g.ProductID, "", units[i].BaseQuantity)
			if e != nil {
				return nil, e
			}
			if _, e = issueAllocations(ctx, tx, a, b, g.ProductID, "refund.goods", id, in.Reason, units[i], lots); e != nil {
				return nil, e
			}
		}
		if err = moneyEvent(ctx, tx, a, b, doc, "", "", "cancellation.refund.goods", today(), in.Reference+": "+in.Reason, in.AmountCents); err != nil {
			return nil, err
		}
	} else {
		result, e := s.useCredit(ctx, tx, a, CreditUseInput{allowCancelled: true, BranchID: b, SourceDocumentID: doc, Kind: "refund", Method: in.Method, AmountCents: in.AmountCents, DrawerID: in.DrawerID, Reference: in.Reference, Reason: in.Reason})
		if e != nil {
			return nil, e
		}
		useID = result.(map[string]any)["id"].(string)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_refund_settlements(id,operation_id,cancellation_id,credit_use_id,method,amount_cents,reference,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, a.OperationID, in.CancellationID, optionalID(useID), in.Method, in.AmountCents, in.Reference, in.Reason, a.User.ID); err != nil {
		return nil, err
	}
	for i, g := range in.Goods {
		if _, err = tx.ExecContext(ctx, `INSERT INTO v2_refund_goods(id,settlement_id,product_id,unit_snapshot,unit_price_cents,vat_bps,total_cents) VALUES($1,$2,$3,$4::jsonb,$5,$6,$7)`, newID(), id, g.ProductID, rawJSON(units[i]), units[i].DefaultPriceCents, g.VATBPS, values[i]); err != nil {
			return nil, err
		}
	}
	return map[string]any{"id": id, "pending_cents": due - settled - in.AmountCents, "method": in.Method}, emit(ctx, tx, a, "refund.settled", map[string]string{"id": id, "branch_id": b, "cancellation_id": in.CancellationID})
}
