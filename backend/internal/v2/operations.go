package v2

import (
	"context"
	"database/sql"
	"strings"
)

type BranchInput struct {
	BranchID string `json:"branch_id"`
}
type ExpiryTaskInput struct {
	BranchID   string `json:"branch_id"`
	ID         string `json:"id"`
	Status     string `json:"status"`
	AssignedTo string `json:"assigned_to"`
	Note       string `json:"note"`
}
type DrawerInput struct {
	WarningsToken string `json:"warnings_token"`
	BranchID      string `json:"branch_id"`
	ID            string `json:"id"`
	OpeningCents  int64  `json:"opening_cents"`
	CountedCents  int64  `json:"counted_cents"`
	AmountCents   int64  `json:"amount_cents"`
	Reason        string `json:"reason"`
}

const expiryQuery = `SELECT jsonb_build_object('items',COALESCE(jsonb_agg(row_to_json(x) ORDER BY x.expires_on,x.lot_number),'[]'::jsonb)) FROM (
 SELECT l.id AS lot_id,l.lot_number,l.expires_on,l.remaining_quantity,l.state,p.name AS product_name,p.sku,
 t.id AS task_id,t.band_months,t.status,t.assigned_to,t.note,
 (l.landed_cost_cents::numeric*l.remaining_quantity/l.received_quantity)::bigint AS physical_lot_risk_estimate_cents
 FROM v2_lots l JOIN products p ON p.id=l.product_id LEFT JOIN v2_expiry_tasks t ON t.lot_id=l.id
 WHERE l.branch_id=$1 AND l.expires_on IS NOT NULL AND l.remaining_quantity>0
)x`

func (s *Service) scanExpiry(ctx context.Context, tx *sql.Tx, a Actor, in BranchInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	// PostgreSQL interval month arithmetic clamps month-end dates. Each lot is
	// in exactly one current band, measured using the Bangkok calendar day.
	result, err := tx.ExecContext(ctx, `INSERT INTO v2_expiry_tasks(id,lot_id,band_months)
 SELECT gen_random_uuid(),l.id,CASE
 WHEN l.expires_on<d.today THEN 0
 WHEN l.expires_on<=(d.today+INTERVAL '3 months')::date THEN 3
 WHEN l.expires_on<=(d.today+INTERVAL '6 months')::date THEN 6 ELSE 9 END
 FROM v2_lots l CROSS JOIN (SELECT (NOW() AT TIME ZONE 'Asia/Bangkok')::date AS today)d
 WHERE l.branch_id=$1 AND l.remaining_quantity>0 AND l.expires_on IS NOT NULL AND l.expires_on<=(d.today+INTERVAL '9 months')::date
 ON CONFLICT(lot_id,band_months) DO NOTHING`, b)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	return map[string]any{"created_tasks": count}, err
}

func (s *Service) expiryTask(ctx context.Context, tx *sql.Tx, a Actor, in ExpiryTaskInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.Status != "acknowledged" && in.Status != "closed" {
		return nil, bad("เลือก acknowledged หรือ closed")
	}
	if in.Status == "closed" && strings.TrimSpace(in.Note) == "" {
		return nil, bad("ปิดงานต้องมีบันทึกผล")
	}
	if in.AssignedTo != "" {
		var eligible bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN roles r ON r.id=u.role_id WHERE u.id=$1 AND u.active=TRUE AND r.active=TRUE AND (u.branch_id=$2 OR r.scope='global'))`, in.AssignedTo, b).Scan(&eligible); err != nil {
			return nil, err
		}
		if !eligible {
			return nil, bad("ผู้รับผิดชอบไม่พร้อมใช้งานในสาขานี้")
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE v2_expiry_tasks t SET status=$3,assigned_to=$4,note=$5,updated_at=NOW() FROM v2_lots l WHERE t.id=$1 AND l.id=t.lot_id AND l.branch_id=$2 AND t.status<>'closed'`, in.ID, b, in.Status, optionalID(in.AssignedTo), in.Note)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, conflict("งานหมดอายุถูกปิดหรือไม่อยู่ในสาขานี้")
	}
	return map[string]string{"id": in.ID, "status": in.Status}, emit(ctx, tx, a, "expiry.task_changed", map[string]string{"id": in.ID, "branch_id": b})
}

func (s *Service) openDrawer(ctx context.Context, tx *sql.Tx, a Actor, in DrawerInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.OpeningCents < 0 || in.OpeningCents > maxInteger {
		return nil, bad("เงินตั้งต้นไม่ถูกต้อง")
	}
	var opened bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM v2_drawers WHERE opened_by=$1 AND closed_at IS NULL)`, a.User.ID).Scan(&opened); err != nil {
		return nil, err
	}
	if opened {
		return nil, conflict("บัญชีนี้มีรอบเงินสดเปิดอยู่แล้ว กรุณาปิดรอบเดิมก่อน")
	}
	id := newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_drawers(id,branch_id,opened_by,opening_cents) VALUES($1,$2,$3,$4)`, id, b, a.User.ID, in.OpeningCents)
	if err != nil {
		return nil, err
	}
	warnings, token, err := shiftWarnings(ctx, tx, b, a.User.ID)
	return map[string]any{"id": id, "warnings": warnings, "warnings_token": token}, err
}

func drawerEntry(ctx context.Context, tx *sql.Tx, a Actor, b, drawer, payment, direction string, amount int64) error {
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM v2_drawers WHERE id=$1 AND branch_id=$2 AND opened_by=$3 AND closed_at IS NULL FOR UPDATE`, drawer, b, a.User.ID).Scan(&id); err != nil {
		return conflict("เปิดรอบเงินสดของบัญชีนี้ก่อนรับ/จ่ายเงินสด")
	}
	if direction == "pay" {
		amount = -amount
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO v2_drawer_events(id,operation_id,drawer_id,payment_id,amount_cents,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, newID(), a.OperationID, drawer, optionalID(payment), amount, "payment."+direction, a.User.ID)
	return err
}

func (s *Service) drawerCash(ctx context.Context, tx *sql.Tx, a Actor, in DrawerInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.AmountCents == 0 || in.AmountCents > maxInteger || in.AmountCents < -maxInteger || strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุยอด cash-in/out และเหตุผล")
	}
	var id string
	if err = tx.QueryRowContext(ctx, `SELECT id::text FROM v2_drawers WHERE id=$1 AND branch_id=$2 AND opened_by=$3 AND closed_at IS NULL FOR UPDATE`, in.ID, b, a.User.ID).Scan(&id); err != nil {
		return nil, conflict("ไม่พบรอบเงินสดที่เปิดอยู่ของบัญชีนี้")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_drawer_events(id,operation_id,drawer_id,amount_cents,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6)`, newID(), a.OperationID, in.ID, in.AmountCents, in.Reason, a.User.ID)
	return map[string]string{"id": in.ID}, err
}

func (s *Service) closeDrawer(ctx context.Context, tx *sql.Tx, a Actor, in DrawerInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.CountedCents < 0 || in.CountedCents > maxInteger {
		return nil, bad("ยอดเงินนับจริงไม่ถูกต้อง")
	}
	var opening, events int64
	if err = tx.QueryRowContext(ctx, `SELECT opening_cents FROM v2_drawers WHERE id=$1 AND branch_id=$2 AND opened_by=$3 AND closed_at IS NULL FOR UPDATE`, in.ID, b, a.User.ID).Scan(&opening); err != nil {
		return nil, conflict("รอบเงินสดถูกปิดแล้วหรือเป็นของบัญชีอื่น")
	}
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0)::bigint FROM v2_drawer_events WHERE drawer_id=$1`, in.ID).Scan(&events); err != nil {
		return nil, err
	}
	if events > maxInteger || events < -maxInteger {
		return nil, bad("ยอดเงินสดเกินขอบเขต")
	}
	expected := opening + events
	variance := in.CountedCents - expected
	if expected > maxInteger || expected < -maxInteger || variance > maxInteger || variance < -maxInteger {
		return nil, bad("ยอดเงินสดเกินขอบเขต")
	}
	warnings, token, err := shiftWarnings(ctx, tx, b, a.User.ID)
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		if in.WarningsToken != token {
			return nil, conflict("มีรายการคืนเงินหรือยอดค้าง ต้องโหลดล่าสุดและรับทราบก่อนปิดกะ รายการที่ยังไม่จบจะส่งต่อกะใหม่")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO v2_shift_acknowledgements(id,operation_id,drawer_id,warnings,actor_id) VALUES($1,$2,$3,$4::jsonb,$5)`, newID(), a.OperationID, in.ID, rawJSON(warnings), a.User.ID); err != nil {
			return nil, err
		}
	}
	if in.CountedCents != expected && strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ยอดนับต่างจากยอดระบบ กรุณาระบุเหตุผล")
	}
	_, err = tx.ExecContext(ctx, `UPDATE v2_drawers SET closed_at=NOW(),counted_cents=$2,expected_cents=$3,close_reason=$4 WHERE id=$1`, in.ID, in.CountedCents, expected, in.Reason)
	if err != nil {
		return nil, err
	}
	if in.CountedCents != expected {
		if _, err = tx.ExecContext(ctx, `INSERT INTO v2_shift_issues(id,drawer_id,branch_id,account_id,amount_cents,reason) VALUES($1,$2,$3,$4,$5,$6)`, newID(), in.ID, b, a.User.ID, in.CountedCents-expected, in.Reason); err != nil {
			return nil, err
		}
	}
	return map[string]any{"id": in.ID, "expected_cents": expected, "counted_cents": in.CountedCents, "variance_cents": in.CountedCents - expected}, emit(ctx, tx, a, "drawer.closed", map[string]any{"id": in.ID, "branch_id": b, "variance_cents": in.CountedCents - expected})
}
