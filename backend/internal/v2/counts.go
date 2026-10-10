package v2

import (
	"context"
	"database/sql"
	"strings"
)

type CountInput struct {
	BranchID   string      `json:"branch_id"`
	ID         string      `json:"id"`
	Reference  string      `json:"reference"`
	Reason     string      `json:"reason"`
	Action     string      `json:"action"`
	Revision   int         `json:"revision"`
	ProductIDs []string    `json:"product_ids"`
	Lines      []CountLine `json:"lines"`
}
type CountLine struct {
	SnapshotID        string `json:"snapshot_id"`
	CountedQuantity   int64  `json:"counted_quantity"`
	IncreaseCostCents int64  `json:"increase_cost_cents"`
	Reason            string `json:"reason"`
}

const countsQuery = `SELECT jsonb_build_object('items',COALESCE(jsonb_agg(row_to_json(x) ORDER BY x.created_at DESC),'[]'::jsonb)) FROM (
 SELECT c.*,(SELECT COALESCE(jsonb_agg(jsonb_build_object('id',sn.id,'lot_id',sn.lot_id,'product_id',sn.product_id,'product_name',p.name,'lot_number',l.lot_number,'snapshot_quantity',sn.quantity,'current_quantity',l.remaining_quantity,'state',sn.lot_state,'expires_on',sn.expires_on,'reserved_quantity',sn.reserved_quantity,'observation',row_to_json(o)) ORDER BY p.name,l.id),'[]'::jsonb)
 FROM v2_count_snapshots sn JOIN v2_lots l ON l.id=sn.lot_id JOIN products p ON p.id=sn.product_id LEFT JOIN v2_count_observations o ON o.snapshot_id=sn.id WHERE sn.count_id=c.id) AS lines
 FROM v2_stock_counts c WHERE c.branch_id=$1 ORDER BY c.created_at DESC LIMIT 100)x`

func (s *Service) createCount(ctx context.Context, tx *sql.Tx, a Actor, in CountInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Reference) == "" || len(in.ProductIDs) == 0 || len(in.ProductIDs) > 200 {
		return nil, bad("ระบุอ้างอิงและสินค้าในขอบเขตตรวจนับ 1–200 รายการ")
	}
	items := []StockLine{}
	for _, p := range in.ProductIDs {
		items = append(items, StockLine{ProductID: p})
	}
	if err = lockStock(ctx, tx, []string{b}, items); err != nil {
		return nil, err
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_stock_counts(id,branch_id,reference,created_by) VALUES($1,$2,$3,$4)`, id, b, in.Reference, a.User.ID); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var count int64
	for _, p := range in.ProductIDs {
		if seen[p] {
			return nil, bad("ขอบเขตสินค้าซ้ำ")
		}
		seen[p] = true
		result, e := tx.ExecContext(ctx, `INSERT INTO v2_count_snapshots(id,count_id,lot_id,product_id,quantity,reserved_quantity,lot_state,expires_on)
 SELECT gen_random_uuid(),$1,l.id,l.product_id,l.remaining_quantity,COALESCE((SELECT SUM(rl.base_quantity) FROM v2_reservation_lines rl JOIN v2_reservations r ON r.id=rl.reservation_id WHERE rl.lot_id=l.id AND r.status='active' AND r.expires_at>NOW()),0),l.state,l.expires_on FROM v2_lots l WHERE l.branch_id=$2 AND l.product_id=$3`, id, b, p)
		if e != nil {
			return nil, e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return nil, e
		}
		count += n
	}
	if count == 0 {
		return nil, conflict("ขอบเขตนี้ไม่มีล็อต V2 ให้ตรวจนับ")
	}
	return map[string]any{"id": id, "lots": count, "revision": 1}, emit(ctx, tx, a, "count.created", map[string]string{"id": id, "branch_id": b})
}

func (s *Service) observeCount(ctx context.Context, tx *sql.Tx, a Actor, in CountInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	var state string
	var revision int
	if err = tx.QueryRowContext(ctx, `SELECT status,revision FROM v2_stock_counts WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.ID, b).Scan(&state, &revision); err != nil {
		return nil, err
	}
	if state != "open" || revision != in.Revision {
		return nil, conflict("รอบนี้บันทึกจำนวนแล้ว หรือข้อมูลเปลี่ยน")
	}
	var required int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM v2_count_snapshots WHERE count_id=$1`, in.ID).Scan(&required); err != nil {
		return nil, err
	}
	if len(in.Lines) != required {
		return nil, bad("ส่งจำนวนครบทุกล็อตในขอบเขต")
	}
	items := []StockLine{}
	seen := map[string]bool{}
	for _, l := range in.Lines {
		if seen[l.SnapshotID] || l.CountedQuantity < 0 || l.CountedQuantity > maxInteger || l.IncreaseCostCents < 0 || l.IncreaseCostCents > maxInteger {
			return nil, bad("จำนวนตรวจนับหรือต้นทุนไม่ถูกต้อง")
		}
		seen[l.SnapshotID] = true
		var p string
		if err = tx.QueryRowContext(ctx, `SELECT product_id::text FROM v2_count_snapshots WHERE id=$1 AND count_id=$2`, l.SnapshotID, in.ID).Scan(&p); err != nil {
			return nil, err
		}
		items = append(items, StockLine{ProductID: p})
	}
	if err = lockStock(ctx, tx, []string{b}, items); err != nil {
		return nil, err
	}
	for _, l := range in.Lines {
		var expected, reserved int64
		if err = tx.QueryRowContext(ctx, `SELECT lot.remaining_quantity,COALESCE((SELECT SUM(rl.base_quantity) FROM v2_reservation_lines rl JOIN v2_reservations r ON r.id=rl.reservation_id WHERE rl.lot_id=lot.id AND r.status='active' AND r.expires_at>NOW()),0)::bigint FROM v2_count_snapshots sn JOIN v2_lots lot ON lot.id=sn.lot_id WHERE sn.id=$1 AND sn.count_id=$2 FOR UPDATE OF lot`, l.SnapshotID, in.ID).Scan(&expected, &reserved); err != nil {
			return nil, err
		}
		if l.CountedQuantity < reserved {
			return nil, conflict("จำนวนที่นับต่ำกว่าของที่จอง ต้องแก้การจองก่อน")
		}
		if l.CountedQuantity != expected && strings.TrimSpace(l.Reason) == "" {
			return nil, bad("ล็อตที่ต่างจากยอดปัจจุบันต้องมีเหตุผล")
		}
		if l.CountedQuantity <= expected && l.IncreaseCostCents != 0 {
			return nil, bad("ต้นทุนเพิ่มใช้เฉพาะล็อตที่ตรวจนับเกิน")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO v2_count_observations(id,operation_id,count_id,snapshot_id,expected_quantity,counted_quantity,increase_cost_cents,reason,observed_by,checkpoint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,(SELECT COALESCE(MAX(e.sequence_no),0) FROM v2_stock_events e JOIN v2_count_snapshots sn ON sn.lot_id=e.lot_id WHERE sn.id=$4))`, newID(), a.OperationID, in.ID, l.SnapshotID, expected, l.CountedQuantity, l.IncreaseCostCents, l.Reason, a.User.ID); err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE v2_stock_counts SET status='counted',revision=revision+1 WHERE id=$1`, in.ID)
	return map[string]any{"id": in.ID, "revision": revision + 1}, err
}

func (s *Service) confirmCount(ctx context.Context, tx *sql.Tx, a Actor, in CountInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	var state, ref string
	var revision int
	if err = tx.QueryRowContext(ctx, `SELECT status,reference,revision FROM v2_stock_counts WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.ID, b).Scan(&state, &ref, &revision); err != nil {
		return nil, err
	}
	if revision != in.Revision || state == "confirmed" || state == "cancelled" {
		return nil, conflict("รอบตรวจนับเปลี่ยนแล้ว")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุเหตุผลอนุมัติหรือยกเลิก")
	}
	if in.Action == "cancel" {
		_, err = tx.ExecContext(ctx, `UPDATE v2_stock_counts SET status='cancelled',revision=revision+1,reason=$2 WHERE id=$1`, in.ID, in.Reason)
		return map[string]string{"id": in.ID, "status": "cancelled"}, err
	}
	if in.Action != "confirm" || state != "counted" {
		return nil, conflict("ต้องบันทึกจำนวนก่อนอนุมัติ")
	}
	rows, err := tx.QueryContext(ctx, `SELECT sn.lot_id::text,sn.product_id::text,o.expected_quantity,o.counted_quantity,o.increase_cost_cents,o.reason FROM v2_count_snapshots sn JOIN v2_count_observations o ON o.snapshot_id=sn.id WHERE sn.count_id=$1 ORDER BY sn.product_id,sn.lot_id`, in.ID)
	if err != nil {
		return nil, err
	}
	type observed struct {
		lot, p, reason          string
		expected, counted, cost int64
	}
	lines := []observed{}
	items := []StockLine{}
	for rows.Next() {
		var l observed
		if err = rows.Scan(&l.lot, &l.p, &l.expected, &l.counted, &l.cost, &l.reason); err != nil {
			rows.Close()
			return nil, err
		}
		lines = append(lines, l)
		items = append(items, StockLine{ProductID: l.p})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = lockStock(ctx, tx, []string{b}, items); err != nil {
		return nil, err
	}
	for _, l := range lines {
		var current, reserved int64
		var changed bool
		if err = tx.QueryRowContext(ctx, `SELECT remaining_quantity FROM v2_lots WHERE id=$1 FOR UPDATE`, l.lot).Scan(&current); err != nil {
			return nil, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM v2_stock_events e JOIN v2_count_observations o ON o.count_id=$2 JOIN v2_count_snapshots sn ON sn.id=o.snapshot_id WHERE sn.lot_id=$1 AND e.lot_id=$1 AND e.sequence_no>o.checkpoint)`, l.lot, in.ID).Scan(&changed); err != nil {
			return nil, err
		}
		if changed || current != l.expected {
			return nil, conflict("มี movement หลังบันทึกจำนวน กรุณายกเลิกและเปิดรอบนับใหม่")
		}
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(rl.base_quantity),0)::bigint FROM v2_reservation_lines rl JOIN v2_reservations r ON r.id=rl.reservation_id WHERE rl.lot_id=$1 AND r.status='active' AND r.expires_at>NOW()`, l.lot).Scan(&reserved); err != nil {
			return nil, err
		}
		if l.counted < reserved {
			return nil, conflict("จำนวนตรวจนับกระทบสินค้าจอง")
		}
		delta := l.counted - current
		u := UnitSnapshot{Name: "base", Conversion: 1, Quantity: delta, BaseQuantity: delta}
		if delta < 0 {
			u.Quantity = -delta
			u.BaseQuantity = -delta
			if _, err = issueAllocations(ctx, tx, a, b, l.p, "count.decrease", in.ID, l.reason, u, []allocation{{LotID: l.lot, Quantity: -delta}}); err != nil {
				return nil, err
			}
		}
		if delta > 0 {
			var qty, value int64
			if err = tx.QueryRowContext(ctx, `SELECT base_quantity,value_cents FROM v2_stock_accounts WHERE branch_id=$1 AND product_id=$2`, b, l.p).Scan(&qty, &value); err != nil {
				return nil, err
			}
			if _, err = add(qty, delta); err != nil {
				return nil, err
			}
			if _, err = add(value, l.cost); err != nil {
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE v2_stock_accounts SET base_quantity=base_quantity+$3,value_cents=value_cents+$4 WHERE branch_id=$1 AND product_id=$2`, b, l.p, delta, l.cost); err != nil {
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE v2_lots SET remaining_quantity=remaining_quantity+$2 WHERE id=$1`, l.lot, delta); err != nil {
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO v2_cost_layers(id,branch_id,product_id,lot_id,remaining_quantity,remaining_value_cents) VALUES($1,$2,$3,$4,$5,$6)`, newID(), b, l.p, l.lot, delta, l.cost); err != nil {
				return nil, err
			}
			if err = stockEvent(ctx, tx, a, b, l.p, l.lot, "count.increase", in.ID, l.reason, delta, l.cost, u); err != nil {
				return nil, err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE v2_stock_counts SET status='confirmed',revision=revision+1,reason=$2,confirmed_by=$3,confirmed_at=NOW() WHERE id=$1`, in.ID, in.Reason, a.User.ID); err != nil {
		return nil, err
	}
	return map[string]string{"id": in.ID, "status": "confirmed"}, emit(ctx, tx, a, "count.confirmed", map[string]string{"id": in.ID, "branch_id": b, "reference": ref})
}
