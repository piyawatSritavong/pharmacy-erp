package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type StockLine struct {
	ProductID     string `json:"product_id"`
	UnitID        string `json:"unit_id"`
	Quantity      int64  `json:"quantity"`
	LotID         string `json:"lot_id"`
	LotNumber     string `json:"lot_number"`
	ExpiresOn     string `json:"expires_on"`
	UnitCostCents int64  `json:"unit_cost_cents"`
	DiscountCents int64  `json:"discount_cents"`
}
type StockInput struct {
	BranchID            string      `json:"branch_id"`
	DestinationBranchID string      `json:"destination_branch_id"`
	Reference           string      `json:"reference"`
	Reason              string      `json:"reason"`
	Items               []StockLine `json:"items"`
	HeaderDiscountCents int64       `json:"header_discount_cents"`
	ShippingCents       int64       `json:"shipping_cents"`
	CapitalizedTaxCents int64       `json:"capitalized_tax_cents"`
	Quarantine          bool        `json:"quarantine"`
	ExpiresAt           string      `json:"expires_at"`
}
type LotStateInput struct {
	BranchID string `json:"branch_id"`
	LotID    string `json:"lot_id"`
	State    string `json:"state"`
	Reason   string `json:"reason"`
}
type ReservationInput struct {
	BranchID      string `json:"branch_id"`
	ReservationID string `json:"reservation_id"`
	Action        string `json:"action"`
	Reason        string `json:"reason"`
}
type allocation struct {
	LotID, LotNumber, ExpiresOn string
	Quantity                    int64
}

const inventoryQuery = `SELECT jsonb_build_object('items',COALESCE(jsonb_agg(row_to_json(x) ORDER BY x.product_name,x.expires_on,x.id),'[]'::jsonb)) FROM (
 SELECT l.*,p.name AS product_name,p.sku,
 COALESCE((SELECT SUM(rl.base_quantity) FROM v2_reservation_lines rl JOIN v2_reservations r ON r.id=rl.reservation_id WHERE rl.lot_id=l.id AND r.status='active' AND r.expires_at>NOW()),0)::bigint AS reserved_quantity,
 CASE WHEN l.state='available' AND (l.expires_on IS NULL OR l.expires_on>=(NOW() AT TIME ZONE 'Asia/Bangkok')::date)
 THEN l.remaining_quantity-COALESCE((SELECT SUM(rl.base_quantity) FROM v2_reservation_lines rl JOIN v2_reservations r ON r.id=rl.reservation_id WHERE rl.lot_id=l.id AND r.status='active' AND r.expires_at>NOW()),0) ELSE 0 END AS sellable_quantity,
 a.base_quantity AS account_quantity,a.value_cents AS account_value_cents
 FROM v2_lots l JOIN products p ON p.id=l.product_id JOIN v2_stock_accounts a ON a.branch_id=l.branch_id AND a.product_id=l.product_id WHERE l.branch_id=$1
)x`

func (s *Service) savePolicy(ctx context.Context, tx *sql.Tx, a Actor, in PolicyInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.CostMethod != "fifo" && in.CostMethod != "moving_average" {
		return nil, bad("เลือก FIFO หรือ Moving Average")
	}
	if in.MaxDiscountBPS < 0 || in.MaxDiscountBPS > 10000 {
		return nil, bad("เพดานส่วนลดไม่ถูกต้อง")
	}
	// Serialize setup against stock operations even before a policy row exists.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "v2-policy:"+b); err != nil {
		return nil, err
	}
	var old string
	err = tx.QueryRowContext(ctx, `SELECT cost_method FROM v2_branch_policies WHERE branch_id=$1 FOR UPDATE`, b).Scan(&old)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if old != "" && old != in.CostMethod {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM v2_stock_events WHERE branch_id=$1)`, b).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			return nil, conflict("มีประวัติ stock แล้ว เปลี่ยนวิธีต้นทุนไม่ได้")
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_branch_policies(branch_id,cost_method,allow_branch_promotions,max_discount_bps,updated_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(branch_id) DO UPDATE SET cost_method=$2,allow_branch_promotions=$3,max_discount_bps=$4,updated_by=$5,updated_at=NOW()`, b, in.CostMethod, in.AllowBranchPromotions, in.MaxDiscountBPS, a.User.ID)
	return map[string]any{"branch_id": b, "cost_method": in.CostMethod}, err
}

func lockStock(ctx context.Context, tx *sql.Tx, branches []string, items []StockLine) error {
	keys := map[string][2]string{}
	for _, b := range branches {
		for _, line := range items {
			keys[b+":"+line.ProductID] = [2]string{b, line.ProductID}
		}
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		pair := keys[key]
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "v2-policy:"+pair[0]); err != nil {
			return err
		}
		var method string
		if err := tx.QueryRowContext(ctx, `SELECT cost_method FROM v2_branch_policies WHERE branch_id=$1`, pair[0]).Scan(&method); err != nil {
			return conflict("ตั้งวิธีต้นทุนของสาขาก่อนทำรายการ stock V2")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO v2_stock_accounts(branch_id,product_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, pair[0], pair[1]); err != nil {
			return err
		}
		var qty int64
		if err := tx.QueryRowContext(ctx, `SELECT base_quantity FROM v2_stock_accounts WHERE branch_id=$1 AND product_id=$2 FOR UPDATE`, pair[0], pair[1]).Scan(&qty); err != nil {
			return err
		}
	}
	return nil
}

func stockEvent(ctx context.Context, tx *sql.Tx, a Actor, b, p, lot, kind, reference, reason string, qty, cost int64, u UnitSnapshot) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO v2_stock_events(id,operation_id,branch_id,product_id,lot_id,event_type,quantity_delta,value_delta_cents,reference,reason,unit_snapshot,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12)`, newID(), a.OperationID, b, p, optionalID(lot), kind, qty, cost, reference, reason, rawJSON(u), a.User.ID)
	return err
}

func receiveLot(ctx context.Context, tx *sql.Tx, a Actor, b, p, lotNumber, expiry, state, origin, reference string, qty, cost int64, u UnitSnapshot) (string, error) {
	if origin != "" {
		var inherited string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM v2_lots WHERE id=$1`, origin).Scan(&inherited); err != nil {
			return "", err
		}
		if inherited == "recalled" {
			state = "recalled"
		} else if inherited == "quarantined" && state == "available" {
			state = "quarantined"
		}
	}
	var currentQty, currentValue int64
	if err := tx.QueryRowContext(ctx, `SELECT base_quantity,value_cents FROM v2_stock_accounts WHERE branch_id=$1 AND product_id=$2`, b, p).Scan(&currentQty, &currentValue); err != nil {
		return "", err
	}
	if _, err := add(currentQty, qty); err != nil {
		return "", err
	}
	if _, err := add(currentValue, cost); err != nil {
		return "", err
	}
	id := newID()
	_, err := tx.ExecContext(ctx, `INSERT INTO v2_lots(id,branch_id,product_id,lot_number,expires_on,state,received_quantity,remaining_quantity,landed_cost_cents,origin_lot_id,created_by) VALUES($1,$2,$3,$4,NULLIF($5,'')::date,$6,$7,$7,$8,$9,$10)`, id, b, p, lotNumber, expiry, state, qty, cost, optionalID(origin), a.User.ID)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `UPDATE v2_stock_accounts SET base_quantity=base_quantity+$3,value_cents=value_cents+$4 WHERE branch_id=$1 AND product_id=$2`, b, p, qty, cost)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_cost_layers(id,branch_id,product_id,lot_id,remaining_quantity,remaining_value_cents) VALUES($1,$2,$3,$4,$5,$6)`, newID(), b, p, id, qty, cost)
	if err != nil {
		return "", err
	}
	return id, stockEvent(ctx, tx, a, b, p, id, "receive", reference, "", qty, cost, u)
}

func (s *Service) receive(ctx context.Context, tx *sql.Tx, a Actor, in StockInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Reference) == "" || len(in.Items) == 0 || len(in.Items) > 200 {
		return nil, bad("ระบุเอกสารอ้างอิงและสินค้า 1–200 รายการ")
	}
	if in.HeaderDiscountCents < 0 || in.ShippingCents < 0 || in.CapitalizedTaxCents < 0 {
		return nil, bad("ส่วนลด ค่าส่ง และภาษีต้องไม่ติดลบ")
	}
	if err = lockStock(ctx, tx, []string{b}, in.Items); err != nil {
		return nil, err
	}
	units := make([]UnitSnapshot, len(in.Items))
	nets := make([]int64, len(in.Items))
	var total, totalQty int64
	for i, line := range in.Items {
		if line.LotNumber == "" || line.UnitCostCents < 0 || line.DiscountCents < 0 {
			return nil, bad("ล็อตและต้นทุนรับเข้าไม่ถูกต้อง")
		}
		if line.ExpiresOn != "" {
			if err = date(line.ExpiresOn); err != nil {
				return nil, err
			}
		}
		var tracks bool
		if err = tx.QueryRowContext(ctx, `SELECT tracks_expiry FROM products WHERE id=$1`, line.ProductID).Scan(&tracks); err != nil {
			return nil, err
		}
		if tracks && line.ExpiresOn == "" {
			return nil, bad("สินค้านี้ต้องระบุวันหมดอายุ")
		}
		units[i], err = resolveUnit(ctx, tx, line.ProductID, line.UnitID, line.Quantity)
		if err != nil {
			return nil, err
		}
		gross, e := multiply(line.Quantity, line.UnitCostCents)
		if e != nil {
			return nil, e
		}
		if line.DiscountCents > gross {
			return nil, bad("ส่วนลดเกินมูลค่ารายการ")
		}
		nets[i] = gross - line.DiscountCents
		total, err = add(total, nets[i])
		if err != nil {
			return nil, err
		}
		totalQty, err = add(totalQty, units[i].BaseQuantity)
		if err != nil {
			return nil, err
		}
	}
	if in.HeaderDiscountCents > total {
		return nil, bad("ส่วนลดท้ายเอกสารเกินยอด")
	}
	charges, err := add(in.ShippingCents, in.CapitalizedTaxCents)
	if err != nil {
		return nil, err
	}
	remainingDiscount, remainingCharges := in.HeaderDiscountCents, charges
	remainingNet, remainingQty := total, totalQty
	lots := []string{}
	for i, line := range in.Items {
		discount, shipping := remainingDiscount, remainingCharges
		if i < len(in.Items)-1 {
			if remainingNet > 0 {
				discount, err = proportion(remainingDiscount, nets[i], remainingNet)
				if err != nil {
					return nil, err
				}
			} else {
				discount = 0
			}
			shipping, err = proportion(remainingCharges, units[i].BaseQuantity, remainingQty)
			if err != nil {
				return nil, err
			}
		}
		remainingDiscount -= discount
		remainingCharges -= shipping
		remainingNet -= nets[i]
		remainingQty -= units[i].BaseQuantity
		cost, e := add(nets[i]-discount, shipping)
		if e != nil {
			return nil, e
		}
		state := "available"
		if in.Quarantine {
			state = "quarantined"
		}
		lot, e := receiveLot(ctx, tx, a, b, line.ProductID, line.LotNumber, line.ExpiresOn, state, "", in.Reference, units[i].BaseQuantity, cost, units[i])
		if e != nil {
			return nil, e
		}
		lots = append(lots, lot)
	}
	if err = emit(ctx, tx, a, "stock.received", map[string]any{"branch_id": b, "lot_ids": lots}); err != nil {
		return nil, err
	}
	landed, err := add(total-in.HeaderDiscountCents, charges)
	if err != nil {
		return nil, err
	}
	return map[string]any{"lot_ids": lots, "base_quantity": totalQty, "landed_cost_cents": landed}, nil
}

func availableLots(ctx context.Context, tx *sql.Tx, b, p, selected string, needed int64) ([]allocation, error) {
	rows, err := tx.QueryContext(ctx, `SELECT l.id::text,l.lot_number,COALESCE(l.expires_on::text,''),l.remaining_quantity-COALESCE((SELECT SUM(rl.base_quantity) FROM v2_reservation_lines rl JOIN v2_reservations r ON r.id=rl.reservation_id WHERE rl.lot_id=l.id AND r.status='active' AND r.expires_at>NOW()),0) FROM v2_lots l WHERE l.branch_id=$1 AND l.product_id=$2 AND l.state='available' AND (l.expires_on IS NULL OR l.expires_on>=(NOW() AT TIME ZONE 'Asia/Bangkok')::date) AND ($3='' OR l.id=NULLIF($3,'')::uuid) ORDER BY l.expires_on ASC NULLS LAST,l.received_at,l.id FOR UPDATE OF l`, b, p, selected)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []allocation{}
	for rows.Next() {
		var item allocation
		var available int64
		if err = rows.Scan(&item.LotID, &item.LotNumber, &item.ExpiresOn, &available); err != nil {
			return nil, err
		}
		if available <= 0 {
			continue
		}
		item.Quantity = available
		if item.Quantity > needed {
			item.Quantity = needed
		}
		needed -= item.Quantity
		result = append(result, item)
		if needed == 0 {
			break
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if needed > 0 {
		return nil, conflict("ยอดขายได้ไม่พอ หรือสินค้าถูกจอง/กักกัน/หมดอายุ")
	}
	return result, nil
}

func takeCost(ctx context.Context, tx *sql.Tx, b, p string, qty int64) (int64, error) {
	var method string
	var onHand, value int64
	if err := tx.QueryRowContext(ctx, `SELECT p.cost_method,a.base_quantity,a.value_cents FROM v2_stock_accounts a JOIN v2_branch_policies p ON p.branch_id=a.branch_id WHERE a.branch_id=$1 AND a.product_id=$2`, b, p).Scan(&method, &onHand, &value); err != nil {
		return 0, err
	}
	if qty <= 0 || qty > onHand {
		return 0, conflict("stock ไม่พอ")
	}
	var cost int64
	if method == "moving_average" {
		var err error
		cost, err = proportion(value, qty, onHand)
		if err != nil {
			return 0, err
		}
	} else {
		rows, err := tx.QueryContext(ctx, `SELECT id::text,remaining_quantity,remaining_value_cents FROM v2_cost_layers WHERE branch_id=$1 AND product_id=$2 AND remaining_quantity>0 ORDER BY received_at,id FOR UPDATE`, b, p)
		if err != nil {
			return 0, err
		}
		type layer struct {
			id         string
			qty, value int64
		}
		layers := []layer{}
		for rows.Next() {
			var l layer
			if err = rows.Scan(&l.id, &l.qty, &l.value); err != nil {
				rows.Close()
				return 0, err
			}
			layers = append(layers, l)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return 0, err
		}
		remaining := qty
		for _, l := range layers {
			take := l.qty
			if take > remaining {
				take = remaining
			}
			part, e := proportion(l.value, take, l.qty)
			if e != nil {
				return 0, e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE v2_cost_layers SET remaining_quantity=remaining_quantity-$2,remaining_value_cents=remaining_value_cents-$3 WHERE id=$1`, l.id, take, part); e != nil {
				return 0, e
			}
			cost += part
			remaining -= take
			if remaining == 0 {
				break
			}
		}
		if remaining != 0 {
			return 0, conflict("cost layers ไม่ตรงยอด stock")
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE v2_stock_accounts SET base_quantity=base_quantity-$3,value_cents=value_cents-$4 WHERE branch_id=$1 AND product_id=$2`, b, p, qty, cost)
	return cost, err
}

func issueAllocations(ctx context.Context, tx *sql.Tx, a Actor, b, p, kind, ref, reason string, u UnitSnapshot, items []allocation) (int64, error) {
	var qty int64
	for _, item := range items {
		qty += item.Quantity
	}
	cost, err := takeCost(ctx, tx, b, p, qty)
	if err != nil {
		return 0, err
	}
	left := cost
	for i, item := range items {
		part := left
		if i < len(items)-1 {
			part, err = proportion(cost, item.Quantity, qty)
			if err != nil {
				return 0, err
			}
		}
		left -= part
		result, e := tx.ExecContext(ctx, `UPDATE v2_lots SET remaining_quantity=remaining_quantity-$2 WHERE id=$1 AND remaining_quantity>=$2`, item.LotID, item.Quantity)
		if e != nil {
			return 0, e
		}
		n, e := result.RowsAffected()
		if e != nil || n != 1 {
			return 0, conflict("ล็อตเปลี่ยนยอดแล้ว")
		}
		if e = stockEvent(ctx, tx, a, b, p, item.LotID, kind, ref, reason, -item.Quantity, -part, u); e != nil {
			return 0, e
		}
	}
	return cost, nil
}

func (s *Service) issue(ctx context.Context, tx *sql.Tx, a Actor, in StockInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if len(in.Items) == 0 || len(in.Items) > 200 || in.Reference == "" {
		return nil, bad("ระบุสินค้าและเอกสารอ้างอิง")
	}
	if err = lockStock(ctx, tx, []string{b}, in.Items); err != nil {
		return nil, err
	}
	var cost int64
	for _, line := range in.Items {
		if line.LotID != "" && strings.TrimSpace(in.Reason) == "" {
			return nil, bad("เลือกล็อตเองต้องมีเหตุผล")
		}
		u, e := resolveUnit(ctx, tx, line.ProductID, line.UnitID, line.Quantity)
		if e != nil {
			return nil, e
		}
		items, e := availableLots(ctx, tx, b, line.ProductID, line.LotID, u.BaseQuantity)
		if e != nil {
			return nil, e
		}
		part, e := issueAllocations(ctx, tx, a, b, line.ProductID, "issue", in.Reference, in.Reason, u, items)
		if e != nil {
			return nil, e
		}
		cost, e = add(cost, part)
		if e != nil {
			return nil, e
		}
	}
	return map[string]any{"cost_cents": cost}, emit(ctx, tx, a, "stock.issued", map[string]any{"branch_id": b, "reference": in.Reference})
}

func (s *Service) reserve(ctx context.Context, tx *sql.Tx, a Actor, in StockInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	expires, e := time.Parse(time.RFC3339, in.ExpiresAt)
	if e != nil || !expires.After(time.Now()) || expires.After(time.Now().Add(7*24*time.Hour)) {
		return nil, bad("อายุการจองต้องอยู่ในอนาคตไม่เกิน 7 วัน")
	}
	if in.Reference == "" || len(in.Items) == 0 || len(in.Items) > 200 {
		return nil, bad("ระบุเอกสารอ้างอิงและรายการจอง")
	}
	if err = lockStock(ctx, tx, []string{b}, in.Items); err != nil {
		return nil, err
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_reservations(id,branch_id,reference,expires_at,created_by) VALUES($1,$2,$3,$4,$5)`, id, b, in.Reference, expires, a.User.ID); err != nil {
		return nil, err
	}
	for _, line := range in.Items {
		u, e := resolveUnit(ctx, tx, line.ProductID, line.UnitID, line.Quantity)
		if e != nil {
			return nil, e
		}
		if line.LotID != "" && strings.TrimSpace(in.Reason) == "" {
			return nil, bad("เลือกล็อตจองเองต้องมีเหตุผล")
		}
		items, e := availableLots(ctx, tx, b, line.ProductID, line.LotID, u.BaseQuantity)
		if e != nil {
			return nil, e
		}
		for _, item := range items {
			if _, e = tx.ExecContext(ctx, `INSERT INTO v2_reservation_lines(id,reservation_id,lot_id,product_id,base_quantity,unit_snapshot) VALUES($1,$2,$3,$4,$5,$6::jsonb)`, newID(), id, item.LotID, line.ProductID, item.Quantity, rawJSON(u)); e != nil {
				return nil, e
			}
		}
	}
	return map[string]any{"id": id, "expires_at": expires}, emit(ctx, tx, a, "stock.reserved", map[string]any{"reservation_id": id, "branch_id": b})
}

func (s *Service) reservationTransition(ctx context.Context, tx *sql.Tx, a Actor, in ReservationInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	var status, reference string
	var expiry time.Time
	if err = tx.QueryRowContext(ctx, `SELECT status,reference,expires_at FROM v2_reservations WHERE id=$1 AND branch_id=$2 FOR UPDATE`, in.ReservationID, b).Scan(&status, &reference, &expiry); err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, conflict("การจองถูกดำเนินการแล้ว")
	}
	if in.Action != "release" && in.Action != "consume" && in.Action != "expire" {
		return nil, bad("action การจองไม่ถูกต้อง")
	}
	if in.Action == "consume" && !expiry.After(time.Now()) {
		return nil, conflict("การจองหมดอายุ")
	}
	if in.Action == "expire" && expiry.After(time.Now()) {
		return nil, conflict("การจองยังไม่หมดอายุ")
	}
	if in.Action == "consume" {
		rows, e := tx.QueryContext(ctx, `SELECT rl.product_id::text,rl.lot_id::text,rl.base_quantity,l.state,COALESCE(l.expires_on::text,''),rl.unit_snapshot FROM v2_reservation_lines rl JOIN v2_lots l ON l.id=rl.lot_id WHERE rl.reservation_id=$1 ORDER BY rl.product_id,rl.lot_id`, in.ReservationID)
		if e != nil {
			return nil, e
		}
		type line struct {
			p, lot, state, expiry string
			qty                   int64
			unit                  []byte
		}
		lines := []line{}
		stockLines := []StockLine{}
		for rows.Next() {
			var l line
			if e = rows.Scan(&l.p, &l.lot, &l.qty, &l.state, &l.expiry, &l.unit); e != nil {
				rows.Close()
				return nil, e
			}
			lines = append(lines, l)
			stockLines = append(stockLines, StockLine{ProductID: l.p})
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		if e = lockStock(ctx, tx, []string{b}, stockLines); e != nil {
			return nil, e
		}
		for _, l := range lines {
			var currentState, currentExpiry string
			if e = tx.QueryRowContext(ctx, `SELECT state,COALESCE(expires_on::text,'') FROM v2_lots WHERE id=$1`, l.lot).Scan(&currentState, &currentExpiry); e != nil {
				return nil, e
			}
			if currentState != "available" || (currentExpiry != "" && currentExpiry < today()) {
				return nil, conflict("ล็อตจองถูกกักกัน เรียกคืน หรือหมดอายุ")
			}
			var u UnitSnapshot
			if e = json.Unmarshal(l.unit, &u); e != nil {
				return nil, e
			}
			if _, e = issueAllocations(ctx, tx, a, b, l.p, "reservation.consume", reference, in.Reason, u, []allocation{{LotID: l.lot, Quantity: l.qty}}); e != nil {
				return nil, e
			}
		}
	}
	newStatus := map[string]string{"release": "released", "consume": "consumed", "expire": "expired"}[in.Action]
	_, err = tx.ExecContext(ctx, `UPDATE v2_reservations SET status=$2,updated_at=NOW() WHERE id=$1`, in.ReservationID, newStatus)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": in.ReservationID, "status": newStatus}, emit(ctx, tx, a, "reservation."+newStatus, map[string]any{"id": in.ReservationID, "branch_id": b})
}

func (s *Service) lotState(ctx context.Context, tx *sql.Tx, a Actor, in LotStateInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.State != "available" && in.State != "quarantined" && in.State != "recalled" {
		return nil, bad("สถานะล็อตไม่ถูกต้อง")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, bad("กรุณาระบุเหตุผล")
	}
	if in.State == "recalled" {
		return s.recallFamily(ctx, tx, a, in)
	}
	var productID string
	if err = tx.QueryRowContext(ctx, `SELECT product_id::text FROM v2_lots WHERE id=$1 AND branch_id=$2`, in.LotID, b).Scan(&productID); err != nil {
		return nil, err
	}
	if err = lockStock(ctx, tx, []string{b}, []StockLine{{ProductID: productID}}); err != nil {
		return nil, err
	}
	var old string
	if err = tx.QueryRowContext(ctx, `SELECT state FROM v2_lots WHERE id=$1 FOR UPDATE`, in.LotID).Scan(&old); err != nil {
		return nil, err
	}
	if old == in.State {
		return nil, conflict("ล็อตอยู่ในสถานะนี้แล้ว")
	}
	if old == "recalled" && in.State == "available" {
		return nil, conflict("ล็อต recall ต้องผ่าน quarantine ก่อนตรวจปล่อย")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE v2_lots SET state=$2 WHERE id=$1`, in.LotID, in.State); err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_lot_events(id,operation_id,lot_id,from_state,to_state,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, newID(), a.OperationID, in.LotID, old, in.State, in.Reason, a.User.ID)
	return map[string]any{"id": in.LotID, "state": in.State}, err
}

func (s *Service) writeOff(ctx context.Context, tx *sql.Tx, a Actor, in StockInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Reason) == "" || in.Reference == "" || len(in.Items) == 0 || len(in.Items) > 200 {
		return nil, bad("ระบุสินค้า เอกสารอ้างอิง และเหตุผลตัดจำหน่าย")
	}
	if err = lockStock(ctx, tx, []string{b}, in.Items); err != nil {
		return nil, err
	}
	var total int64
	for _, line := range in.Items {
		u, e := resolveUnit(ctx, tx, line.ProductID, line.UnitID, line.Quantity)
		if e != nil {
			return nil, e
		}
		var available int64
		if e = tx.QueryRowContext(ctx, `SELECT remaining_quantity-COALESCE((SELECT SUM(rl.base_quantity) FROM v2_reservation_lines rl JOIN v2_reservations r ON r.id=rl.reservation_id WHERE rl.lot_id=l.id AND r.status='active' AND r.expires_at>NOW()),0) FROM v2_lots l WHERE id=$1 AND branch_id=$2 AND product_id=$3 FOR UPDATE`, line.LotID, b, line.ProductID).Scan(&available); e != nil {
			return nil, e
		}
		if available < u.BaseQuantity {
			return nil, conflict("จำนวนตัดจำหน่ายเกินยอดที่ไม่ได้จอง")
		}
		cost, e := issueAllocations(ctx, tx, a, b, line.ProductID, "write_off", in.Reference, in.Reason, u, []allocation{{LotID: line.LotID, Quantity: u.BaseQuantity}})
		if e != nil {
			return nil, e
		}
		total, e = add(total, cost)
		if e != nil {
			return nil, e
		}
	}
	return map[string]any{"loss_cents": total}, nil
}

func (s *Service) transfer(ctx context.Context, tx *sql.Tx, a Actor, in StockInput) (any, error) {
	source, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	dest, err := branch(ctx, tx, a.User, in.DestinationBranchID)
	if err != nil {
		return nil, err
	}
	if source == dest || in.Reference == "" || len(in.Items) == 0 || len(in.Items) > 200 {
		return nil, bad("ระบุต้นทาง ปลายทาง เอกสาร และรายการโอน")
	}
	if err = lockStock(ctx, tx, []string{source, dest}, in.Items); err != nil {
		return nil, err
	}
	destinationLots := []string{}
	for _, line := range in.Items {
		u, e := resolveUnit(ctx, tx, line.ProductID, line.UnitID, line.Quantity)
		if e != nil {
			return nil, e
		}
		if line.LotID != "" && in.Reason == "" {
			return nil, bad("เลือกล็อตเองต้องมีเหตุผล")
		}
		items, e := availableLots(ctx, tx, source, line.ProductID, line.LotID, u.BaseQuantity)
		if e != nil {
			return nil, e
		}
		cost, e := issueAllocations(ctx, tx, a, source, line.ProductID, "transfer.out", in.Reference, in.Reason, u, items)
		if e != nil {
			return nil, e
		}
		left := cost
		for i, item := range items {
			part := left
			if i < len(items)-1 {
				part, e = proportion(cost, item.Quantity, u.BaseQuantity)
				if e != nil {
					return nil, e
				}
			}
			left -= part
			id, e := receiveLot(ctx, tx, a, dest, line.ProductID, item.LotNumber, item.ExpiresOn, "available", item.LotID, in.Reference, item.Quantity, part, u)
			if e != nil {
				return nil, e
			}
			destinationLots = append(destinationLots, id)
		}
	}
	return map[string]any{"destination_lot_ids": destinationLots, "status": "received"}, emit(ctx, tx, a, "stock.transferred", map[string]any{"source_branch_id": source, "destination_branch_id": dest, "reference": in.Reference})
}
