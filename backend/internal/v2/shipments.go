package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

type ShipmentReceiptInput struct {
	BranchID   string                `json:"branch_id"`
	ShipmentID string                `json:"shipment_id"`
	Action     string                `json:"action"`
	Reason     string                `json:"reason"`
	Quarantine bool                  `json:"quarantine"`
	Lines      []ShipmentReceiptLine `json:"lines"`
}
type ShipmentReceiptLine struct {
	ID           string `json:"id"`
	BaseQuantity int64  `json:"base_quantity"`
}

const shipmentsQuery = `SELECT jsonb_build_object('items',COALESCE(jsonb_agg(row_to_json(x) ORDER BY x.created_at DESC),'[]'::jsonb)) FROM (
 SELECT sh.*,(SELECT COALESCE(jsonb_agg(jsonb_build_object('id',sl.id,'product_id',sl.product_id,'product_name',p.name,'lot_number',l.lot_number,'expires_on',l.expires_on,'base_quantity',sl.base_quantity,'value_cents',sl.value_cents,'in_transit_quantity',sl.base_quantity-COALESCE((SELECT SUM(r.base_quantity) FROM v2_shipment_receipts r WHERE r.shipment_line_id=sl.id),0)) ORDER BY sl.id),'[]'::jsonb)
 FROM v2_shipment_lines sl JOIN v2_lots l ON l.id=sl.source_lot_id JOIN products p ON p.id=sl.product_id WHERE sl.shipment_id=sh.id) AS lines
 FROM v2_shipments sh WHERE sh.source_branch_id=$1 OR sh.destination_branch_id=$1 ORDER BY sh.created_at DESC LIMIT 100)x`

func (s *Service) dispatchShipment(ctx context.Context, tx *sql.Tx, a Actor, in StockInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	destination, err := branch(ctx, tx, a.User, in.DestinationBranchID)
	if err != nil {
		return nil, err
	}
	if destination == b || len(in.Items) == 0 || len(in.Items) > 200 || strings.TrimSpace(in.Reference) == "" {
		return nil, bad("ระบุสินค้า อ้างอิง และสาขาปลายทางคนละสาขา")
	}
	if err = lockStock(ctx, tx, []string{b, destination}, in.Items); err != nil {
		return nil, err
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_shipments(id,operation_id,source_branch_id,destination_branch_id,reference,status,created_by) VALUES($1,$2,$3,$4,$5,'dispatched',$6)`, id, a.OperationID, b, destination, in.Reference, a.User.ID); err != nil {
		return nil, err
	}
	for _, line := range in.Items {
		if line.LotID != "" && strings.TrimSpace(in.Reason) == "" {
			return nil, bad("เลือกล็อตโอนเองต้องมีเหตุผล")
		}
		u, e := resolveUnit(ctx, tx, line.ProductID, line.UnitID, line.Quantity)
		if e != nil {
			return nil, e
		}
		allocations, e := availableLots(ctx, tx, b, line.ProductID, line.LotID, u.BaseQuantity)
		if e != nil {
			return nil, e
		}
		cost, e := issueAllocations(ctx, tx, a, b, line.ProductID, "shipment.dispatch", id, in.Reason, u, allocations)
		if e != nil {
			return nil, e
		}
		left := cost
		for i, l := range allocations {
			value := left
			if i < len(allocations)-1 {
				value, e = proportion(cost, l.Quantity, u.BaseQuantity)
				if e != nil {
					return nil, e
				}
			}
			left -= value
			if _, e = tx.ExecContext(ctx, `INSERT INTO v2_shipment_lines(id,shipment_id,source_lot_id,product_id,base_quantity,value_cents,unit_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)`, newID(), id, l.LotID, line.ProductID, l.Quantity, value, rawJSON(u)); e != nil {
				return nil, e
			}
		}
	}
	return map[string]string{"id": id, "status": "dispatched"}, emit(ctx, tx, a, "shipment.dispatched", map[string]string{"id": id, "branch_id": b, "destination_branch_id": destination})
}

func (s *Service) receiveShipment(ctx context.Context, tx *sql.Tx, a Actor, in ShipmentReceiptInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.Action != "receive" && in.Action != "return_to_source" {
		return nil, bad("เลือกรับปลายทางหรือคืนส่วนค้างให้ต้นทาง")
	}
	var source, destination, status string
	if err = tx.QueryRowContext(ctx, `SELECT source_branch_id::text,destination_branch_id::text,status FROM v2_shipments WHERE id=$1 AND (source_branch_id=$2 OR destination_branch_id=$2) FOR UPDATE`, in.ShipmentID, b).Scan(&source, &destination, &status); err != nil {
		return nil, err
	}
	if status != "dispatched" && status != "partial" {
		return nil, conflict("ใบโอนนี้รับครบหรือปิดไปแล้ว")
	}
	target := destination
	if in.Action == "return_to_source" {
		target = source
		if strings.TrimSpace(in.Reason) == "" {
			return nil, bad("คืนของระหว่างทางต้องระบุเหตุผล")
		}
	}
	if b != target {
		return nil, conflict("เลือกสาขาที่กำลังรับของตามการดำเนินการ")
	}
	if len(in.Lines) == 0 || len(in.Lines) > 400 {
		return nil, bad("ระบุรายการที่รับ 1–400 ล็อต")
	}
	type line struct {
		id, p, lot, number, expiry              string
		qty, cost, received, receivedCost, take int64
		unit                                    []byte
	}
	lines := []line{}
	items := []StockLine{}
	seen := map[string]bool{}
	for _, input := range in.Lines {
		if seen[input.ID] || input.BaseQuantity <= 0 || input.BaseQuantity > maxInteger {
			return nil, bad("รายการซ้ำหรือจำนวนไม่ถูกต้อง")
		}
		seen[input.ID] = true
		l := line{id: input.ID, take: input.BaseQuantity}
		if err = tx.QueryRowContext(ctx, `SELECT sl.product_id::text,sl.source_lot_id::text,l.lot_number,COALESCE(l.expires_on::text,''),sl.base_quantity,sl.value_cents,sl.unit_snapshot FROM v2_shipment_lines sl JOIN v2_lots l ON l.id=sl.source_lot_id WHERE sl.id=$1 AND sl.shipment_id=$2`, input.ID, in.ShipmentID).Scan(&l.p, &l.lot, &l.number, &l.expiry, &l.qty, &l.cost, &l.unit); err != nil {
			return nil, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(base_quantity),0)::bigint,COALESCE(SUM(value_cents),0)::bigint FROM v2_shipment_receipts WHERE shipment_line_id=$1`, input.ID).Scan(&l.received, &l.receivedCost); err != nil {
			return nil, err
		}
		if l.take > l.qty-l.received {
			return nil, conflict("รับเกินจำนวนค้างระหว่างทาง")
		}
		lines = append(lines, l)
		items = append(items, StockLine{ProductID: l.p})
	}
	if err = lockStock(ctx, tx, []string{source, destination}, items); err != nil {
		return nil, err
	}
	for _, l := range lines {
		cost := l.cost - l.receivedCost
		if l.take < l.qty-l.received {
			cost, err = proportion(l.cost, l.take, l.qty)
			if err != nil {
				return nil, err
			}
		}
		state := "available"
		if in.Quarantine || in.Action == "return_to_source" {
			state = "quarantined"
		}
		var u UnitSnapshot
		if err = json.Unmarshal(l.unit, &u); err != nil {
			return nil, err
		}
		lot, err := receiveLot(ctx, tx, a, target, l.p, l.number, l.expiry, state, l.lot, in.ShipmentID, l.take, cost, u)
		if err != nil {
			return nil, err
		}
		kind := "destination"
		if in.Action == "return_to_source" {
			kind = in.Action
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO v2_shipment_receipts(id,operation_id,shipment_line_id,received_lot_id,base_quantity,value_cents,receipt_kind,reason,received_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, newID(), a.OperationID, l.id, lot, l.take, cost, kind, in.Reason, a.User.ID); err != nil {
			return nil, err
		}
	}
	var outstanding int64
	var returned bool
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(sl.base_quantity-COALESCE((SELECT SUM(r.base_quantity) FROM v2_shipment_receipts r WHERE r.shipment_line_id=sl.id),0)),0)::bigint,EXISTS(SELECT 1 FROM v2_shipment_receipts r JOIN v2_shipment_lines sl ON sl.id=r.shipment_line_id WHERE sl.shipment_id=$1 AND r.receipt_kind='return_to_source') FROM v2_shipment_lines sl WHERE sl.shipment_id=$1`, in.ShipmentID).Scan(&outstanding, &returned); err != nil {
		return nil, err
	}
	next := "partial"
	if outstanding == 0 {
		next = "received"
		if returned {
			next = "cancelled"
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE v2_shipments SET status=$2 WHERE id=$1`, in.ShipmentID, next); err != nil {
		return nil, err
	}
	return map[string]any{"id": in.ShipmentID, "status": next, "in_transit_quantity": outstanding}, emit(ctx, tx, a, "shipment.received", map[string]string{"id": in.ShipmentID, "branch_id": target})
}
