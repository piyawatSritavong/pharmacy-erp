package v2

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

type CreditUseInput struct {
	allowCancelled   bool
	BranchID         string `json:"branch_id"`
	SourceDocumentID string `json:"source_document_id"`
	TargetDocumentID string `json:"target_document_id"`
	Kind             string `json:"kind"`
	AmountCents      int64  `json:"amount_cents"`
	Method           string `json:"method"`
	DrawerID         string `json:"drawer_id"`
	Reference        string `json:"reference"`
	Reason           string `json:"reason"`
}

func (s *Service) useCredit(ctx context.Context, tx *sql.Tx, a Actor, in CreditUseInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.AmountCents <= 0 || in.AmountCents > maxInteger || strings.TrimSpace(in.Reference) == "" || strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุยอดเครดิต อ้างอิง และเหตุผล")
	}
	if in.Kind != "apply" && in.Kind != "refund" {
		return nil, bad("เลือกนำเครดิตไปใช้หรือคืนเงิน")
	}
	if in.Kind == "apply" && (in.TargetDocumentID == "" || in.TargetDocumentID == in.SourceDocumentID || in.Method != "") {
		return nil, bad("เลือกเอกสารหนี้ปลายทางคนละใบกับเครดิต")
	}
	if in.Kind == "refund" && (in.TargetDocumentID != "" || (in.Method != "cash" && in.Method != "bank_transfer")) {
		return nil, bad("การคืนเงินต้องระบุวิธีเงินสดหรือโอน")
	}
	ids := []string{in.SourceDocumentID}
	if in.Kind == "apply" {
		ids = append(ids, in.TargetDocumentID)
	}
	sort.Strings(ids)
	type doc struct {
		kind, customer, supplier, status string
		balance                          int64
	}
	docs := map[string]doc{}
	for _, id := range ids {
		var d doc
		if err = tx.QueryRowContext(ctx, `SELECT kind,COALESCE(customer_id::text,''),COALESCE(supplier_id::text,''),status FROM v2_documents WHERE id=$1 AND branch_id=$2 FOR UPDATE`, id, b).Scan(&d.kind, &d.customer, &d.supplier, &d.status); err != nil {
			return nil, err
		}
		allowedCancelled := in.allowCancelled && in.Kind == "refund" && id == in.SourceDocumentID && d.status == "cancelled" && d.kind == "ar_invoice"
		if (d.status != "issued" && !allowedCancelled) || (d.kind != "ar_invoice" && d.kind != "ap_bill") {
			return nil, conflict("เครดิตต้องอ้างใบขายหรือใบซื้อที่ออกแล้ว")
		}
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0)::bigint FROM v2_money_events WHERE document_id=$1`, id).Scan(&d.balance); err != nil {
			return nil, err
		}
		docs[id] = d
	}
	source := docs[in.SourceDocumentID]
	if source.balance >= 0 || in.AmountCents > -source.balance {
		return nil, conflict("ยอดเครดิตคงเหลือไม่พอ")
	}
	if in.Kind == "apply" {
		target := docs[in.TargetDocumentID]
		if source.customer != target.customer || source.supplier != target.supplier || source.kind != target.kind {
			return nil, conflict("ใช้เครดิตได้เฉพาะคู่ค้าและประเภทหนี้เดียวกันในสาขานี้")
		}
		var pending int64
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(al.amount_cents),0)::bigint FROM v2_allocations al JOIN v2_cheques ch ON ch.payment_id=al.payment_id WHERE al.document_id=$1 AND ch.status IN ('received','deposited')`, in.TargetDocumentID).Scan(&pending); err != nil {
			return nil, err
		}
		if in.AmountCents > target.balance-pending {
			return nil, conflict("เครดิตเกินยอดหนี้ที่ไม่ติดเช็ค")
		}
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_credit_uses(id,operation_id,source_document_id,target_document_id,kind,amount_cents,method,reference,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10)`, id, a.OperationID, in.SourceDocumentID, optionalID(in.TargetDocumentID), in.Kind, in.AmountCents, in.Method, in.Reference, in.Reason, a.User.ID); err != nil {
		return nil, err
	}
	if err = moneyEvent(ctx, tx, a, b, in.SourceDocumentID, "", "", "credit."+in.Kind+".source", today(), in.Reference+": "+in.Reason, in.AmountCents); err != nil {
		return nil, err
	}
	if in.Kind == "apply" {
		if err = moneyEvent(ctx, tx, a, b, in.TargetDocumentID, "", "", "credit.applied", today(), in.Reference+": "+in.Reason, -in.AmountCents); err != nil {
			return nil, err
		}
	}
	if in.Kind == "refund" {
		signed := in.AmountCents
		if source.kind == "ar_invoice" {
			signed = -signed
		}
		if err = moneyEvent(ctx, tx, a, b, "", "", "", "credit.refund."+source.kind, today(), in.Reference+": "+in.Reason, signed); err != nil {
			return nil, err
		}
		if in.Method == "cash" {
			var drawer string
			if err = tx.QueryRowContext(ctx, `SELECT id::text FROM v2_drawers WHERE id=$1 AND branch_id=$2 AND opened_by=$3 AND closed_at IS NULL FOR UPDATE`, in.DrawerID, b, a.User.ID).Scan(&drawer); err != nil {
				return nil, conflict("คืนเงินสดต้องมีรอบเงินสดที่เปิดของบัญชีนี้")
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO v2_drawer_events(id,operation_id,drawer_id,credit_use_id,amount_cents,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, newID(), a.OperationID, drawer, id, signed, "credit.refund:"+in.SourceDocumentID, a.User.ID); err != nil {
				return nil, err
			}
		}
	}
	return map[string]any{"id": id, "kind": in.Kind, "amount_cents": in.AmountCents}, emit(ctx, tx, a, "credit.used", map[string]string{"id": id, "branch_id": b})
}
