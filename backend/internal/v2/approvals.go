package v2

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
)

type DiscountApprovalInput struct {
	BranchID string        `json:"branch_id"`
	Reason   string        `json:"reason"`
	Document DocumentInput `json:"document"`
}

func documentFingerprint(ctx context.Context, tx *sql.Tx, b string, in DocumentInput) (string, error) {
	in.ApprovalID = ""
	in.BranchID = b
	products := []string{}
	for _, l := range in.Lines {
		products = append(products, l.ProductID)
	}
	if in.PromotionID != "" {
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT id::text FROM v2_promotions WHERE id=$1 FOR SHARE`, in.PromotionID).Scan(&id); err != nil {
			return "", err
		}
	}
	for _, query := range []string{`SELECT id FROM products WHERE id::text IN (SELECT jsonb_array_elements_text($1::jsonb)) ORDER BY id FOR SHARE`, `SELECT id FROM product_units WHERE product_id::text IN (SELECT jsonb_array_elements_text($1::jsonb)) ORDER BY id FOR SHARE`} {
		rows, err := tx.QueryContext(ctx, query, rawJSON(products))
		if err != nil {
			return "", err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return "", err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
	}
	var master []byte
	err := tx.QueryRowContext(ctx, `SELECT jsonb_build_object('date',(NOW() AT TIME ZONE 'Asia/Bangkok')::date,
 'policy',(SELECT row_to_json(p) FROM v2_branch_policies p WHERE branch_id=$1),
 'customer',(SELECT row_to_json(c) FROM v2_customers c WHERE id=NULLIF($3,'')::uuid AND branch_id=$1),
 'products',(SELECT jsonb_agg(row_to_json(p) ORDER BY p.id) FROM products p WHERE p.id::text IN (SELECT jsonb_array_elements_text($2::jsonb))),
 'units',(SELECT jsonb_agg(row_to_json(u) ORDER BY u.id) FROM product_units u WHERE u.product_id::text IN (SELECT jsonb_array_elements_text($2::jsonb))),
 'prices',(SELECT jsonb_agg(row_to_json(r) ORDER BY r.id) FROM v2_price_rules r WHERE r.branch_id=$1 AND r.product_id::text IN (SELECT jsonb_array_elements_text($2::jsonb))),
 'promotion',(SELECT row_to_json(p) FROM v2_promotions p WHERE id=NULLIF($4,'')::uuid))`, b, rawJSON(products), in.CustomerID, in.PromotionID).Scan(&master)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(rawJSON(in)+string(master)))), nil
}

func (s *Service) approveDiscount(ctx context.Context, tx *sql.Tx, a Actor, in DiscountApprovalInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Reason) == "" || len(in.Document.Lines) == 0 || len(in.Document.Lines) > 200 || (in.Document.Kind != "quotation" && in.Document.Kind != "ar_invoice") {
		return nil, bad("ระบุใบขาย/ใบเสนอราคา รายการ และเหตุผลอนุมัติ")
	}
	if _, _, err = counterparty(ctx, tx, b, in.Document.CustomerID, ""); err != nil {
		return nil, err
	}
	items := []StockLine{}
	for _, l := range in.Document.Lines {
		items = append(items, StockLine{ProductID: l.ProductID})
	}
	if err = lockStock(ctx, tx, []string{b}, items); err != nil {
		return nil, err
	}
	for _, l := range in.Document.Lines {
		if _, err = resolveUnit(ctx, tx, l.ProductID, l.UnitID, l.Quantity); err != nil {
			return nil, err
		}
		if l.DiscountCents < 0 || l.DiscountCents > maxInteger {
			return nil, bad("ยอดส่วนลดไม่ถูกต้อง")
		}
	}
	hash, err := documentFingerprint(ctx, tx, b, in.Document)
	if err != nil {
		return nil, err
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_discount_approvals(id,operation_id,branch_id,cart_hash,reason,approved_by,expires_at) VALUES($1,$2,$3,$4,$5,$6,NOW()+INTERVAL '15 minutes')`, id, a.OperationID, b, hash, in.Reason, a.User.ID); err != nil {
		return nil, err
	}
	return map[string]string{"id": id}, emit(ctx, tx, a, "discount.approved", map[string]string{"id": id, "branch_id": b, "reason": in.Reason})
}

func consumeDiscountApproval(ctx context.Context, tx *sql.Tx, a Actor, b string, in DocumentInput) error {
	hash, err := documentFingerprint(ctx, tx, b, in)
	if err != nil {
		return err
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, `SELECT cart_hash=$3 AND expires_at>NOW() FROM v2_discount_approvals WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.ApprovalID, b, hash).Scan(&valid); err != nil {
		return conflict("ไม่พบหลักฐานอนุมัติส่วนลด")
	}
	if !valid {
		return conflict("รายการ/ราคา/นโยบายเปลี่ยน หรือการอนุมัติหมดอายุ ต้องอนุมัติใหม่")
	}
	var used bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM v2_discount_approval_uses WHERE approval_id=$1)`, in.ApprovalID).Scan(&used); err != nil {
		return err
	}
	if used {
		return conflict("การอนุมัตินี้ถูกใช้ไปแล้ว")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_discount_approval_uses(approval_id,operation_id,used_by) VALUES($1,$2,$3)`, in.ApprovalID, a.OperationID, a.User.ID)
	return err
}
