package v2

import (
	"context"
	"database/sql"
	"strings"
)

type LotSplitInput struct {
	BranchID     string `json:"branch_id"`
	LotID        string `json:"lot_id"`
	BaseQuantity int64  `json:"base_quantity"`
	Reason       string `json:"reason"`
}

func (s *Service) quarantinePart(ctx context.Context, tx *sql.Tx, a Actor, in LotSplitInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.BaseQuantity <= 0 || in.BaseQuantity > maxInteger || strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุจำนวนฐานและเหตุผลกักกัน")
	}
	var p string
	if err = tx.QueryRowContext(ctx, `SELECT product_id::text FROM v2_lots WHERE id=$1 AND branch_id=$2`, in.LotID, b).Scan(&p); err != nil {
		return nil, err
	}
	if err = lockStock(ctx, tx, []string{b}, []StockLine{{ProductID: p}}); err != nil {
		return nil, err
	}
	var number, expiry, state string
	var available, received, value int64
	if err = tx.QueryRowContext(ctx, `SELECT lot_number,COALESCE(expires_on::text,''),state,remaining_quantity-COALESCE((SELECT SUM(rl.base_quantity) FROM v2_reservation_lines rl JOIN v2_reservations r ON r.id=rl.reservation_id WHERE rl.lot_id=l.id AND r.status='active' AND r.expires_at>NOW()),0)::bigint,received_quantity,landed_cost_cents FROM v2_lots l WHERE id=$1 FOR UPDATE`, in.LotID).Scan(&number, &expiry, &state, &available, &received, &value); err != nil {
		return nil, err
	}
	if state != "available" || in.BaseQuantity > available {
		return nil, conflict("กักกันบางส่วนได้จากล็อตพร้อมขายที่ยังไม่ถูกจอง")
	}
	historicalCost, err := proportion(value, in.BaseQuantity, received)
	if err != nil {
		return nil, err
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `UPDATE v2_lots SET remaining_quantity=remaining_quantity-$2 WHERE id=$1`, in.LotID, in.BaseQuantity); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_lots(id,branch_id,product_id,lot_number,expires_on,state,received_quantity,remaining_quantity,landed_cost_cents,origin_lot_id,created_by) VALUES($1,$2,$3,$4,NULLIF($5,'')::date,'quarantined',$6,$6,$7,$8,$9)`, id, b, p, number, expiry, in.BaseQuantity, historicalCost, in.LotID, a.User.ID); err != nil {
		return nil, err
	}
	u := UnitSnapshot{Name: "base", Conversion: 1, Quantity: in.BaseQuantity, BaseQuantity: in.BaseQuantity}
	if err = stockEvent(ctx, tx, a, b, p, in.LotID, "quarantine.split.out", id, in.Reason, -in.BaseQuantity, 0, u); err != nil {
		return nil, err
	}
	if err = stockEvent(ctx, tx, a, b, p, id, "quarantine.split.in", in.LotID, in.Reason, in.BaseQuantity, 0, u); err != nil {
		return nil, err
	}
	return map[string]string{"lot_id": id, "state": "quarantined"}, emit(ctx, tx, a, "lot.quarantined", map[string]string{"id": id, "branch_id": b})
}

func (s *Service) recallFamily(ctx context.Context, tx *sql.Tx, a Actor, in LotStateInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุเหตุผล recall")
	}
	var product string
	if err = tx.QueryRowContext(ctx, `SELECT product_id::text FROM v2_lots WHERE id=$1 AND branch_id=$2`, in.LotID, b).Scan(&product); err != nil {
		return nil, err
	}
	// Owner-only pilot: lock policy scopes in the same sorted order as transfers.
	// Re-read lineage after locks, so receipts cannot create an unblocked child.
	rows, err := tx.QueryContext(ctx, `SELECT branch_id::text FROM v2_branch_policies ORDER BY branch_id`)
	if err != nil {
		return nil, err
	}
	branches := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		branches = append(branches, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = lockStock(ctx, tx, branches, []StockLine{{ProductID: product}}); err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `WITH RECURSIVE ancestors AS (
 SELECT id,origin_lot_id FROM v2_lots WHERE id=$1 UNION SELECT l.id,l.origin_lot_id FROM v2_lots l JOIN ancestors a ON l.id=a.origin_lot_id
 ), family AS (
 SELECT id FROM ancestors WHERE origin_lot_id IS NULL UNION SELECT l.id FROM v2_lots l JOIN family f ON l.origin_lot_id=f.id
 ) SELECT l.id::text,l.state FROM v2_lots l JOIN family f ON f.id=l.id ORDER BY l.branch_id,l.id FOR UPDATE OF l`, in.LotID)
	if err != nil {
		return nil, err
	}
	type lot struct{ id, state string }
	lots := []lot{}
	for rows.Next() {
		var l lot
		if err = rows.Scan(&l.id, &l.state); err != nil {
			rows.Close()
			return nil, err
		}
		lots = append(lots, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	changed := 0
	for _, l := range lots {
		if l.state == "recalled" {
			continue
		}
		if _, err = tx.ExecContext(ctx, `UPDATE v2_lots SET state='recalled' WHERE id=$1`, l.id); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO v2_lot_events(id,operation_id,lot_id,from_state,to_state,reason,actor_id) VALUES($1,$2,$3,$4,'recalled',$5,$6)`, newID(), a.OperationID, l.id, l.state, in.Reason, a.User.ID); err != nil {
			return nil, err
		}
		changed++
	}
	return map[string]any{"lots_recalled": changed}, emit(ctx, tx, a, "lot.family_recalled", map[string]string{"id": in.LotID, "branch_id": b})
}
