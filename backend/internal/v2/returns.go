package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/labstack/echo/v4"
	"pharmacy-erp/backend/internal/platform"
)

type ReturnInput struct {
	BranchID      string `json:"branch_id"`
	SourceEventID string `json:"source_event_id"`
	BaseQuantity  int64  `json:"base_quantity"`
	Reason        string `json:"reason"`
}

const returnsQuery = `SELECT jsonb_build_object('sources',COALESCE((SELECT jsonb_agg(row_to_json(x)) FROM (
 SELECT e.id,e.product_id,p.name AS product_name,e.lot_id,l.lot_number,l.expires_on,e.reference,d.document_number,e.unit_snapshot,
 -e.quantity_delta AS sold_base_quantity,-e.value_delta_cents AS issued_cost_cents,
 -e.quantity_delta-COALESCE((SELECT SUM(r.base_quantity) FROM v2_stock_returns r WHERE r.source_event_id=e.id),0)::bigint AS returnable_base_quantity
 FROM v2_stock_events e JOIN v2_lots l ON l.id=e.lot_id JOIN products p ON p.id=e.product_id JOIN v2_documents d ON d.id::text=e.reference AND d.kind='ar_invoice'
 WHERE e.branch_id=$1 AND e.event_type='invoice.issue' ORDER BY e.created_at DESC LIMIT 200)x),'[]'::jsonb),
 'items',COALESCE((SELECT jsonb_agg(row_to_json(r) ORDER BY r.created_at DESC) FROM v2_stock_returns r WHERE r.branch_id=$1),'[]'::jsonb))`

func (s *Service) returnStock(ctx context.Context, tx *sql.Tx, a Actor, in ReturnInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.BaseQuantity <= 0 || in.BaseQuantity > maxInteger || strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุจำนวนหน่วยฐานและเหตุผลคืนสินค้า")
	}
	var p, sourceLot, lotNumber, expiry, reference string
	var sold, cost int64
	var snapshot []byte
	if err = tx.QueryRowContext(ctx, `SELECT e.product_id::text,e.lot_id::text,l.lot_number,COALESCE(l.expires_on::text,''),e.reference,-e.quantity_delta,-e.value_delta_cents,e.unit_snapshot FROM v2_stock_events e JOIN v2_lots l ON l.id=e.lot_id JOIN v2_documents d ON d.id::text=e.reference AND d.kind='ar_invoice' WHERE e.id=$1 AND e.branch_id=$2 AND e.event_type='invoice.issue' AND e.quantity_delta<0`, in.SourceEventID, b).Scan(&p, &sourceLot, &lotNumber, &expiry, &reference, &sold, &cost, &snapshot); err != nil {
		return nil, conflict("คืนได้เฉพาะ movement ขาย V2 ที่ระบุล็อตเดิม")
	}
	if err = lockStock(ctx, tx, []string{b}, []StockLine{{ProductID: p}}); err != nil {
		return nil, err
	}
	var priorQty, priorCost int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(base_quantity),0)::bigint,COALESCE(SUM(value_cents),0)::bigint FROM v2_stock_returns WHERE source_event_id=$1`, in.SourceEventID).Scan(&priorQty, &priorCost); err != nil {
		return nil, err
	}
	if in.BaseQuantity > sold-priorQty {
		return nil, conflict("จำนวนคืนสะสมเกินจำนวนขายจากล็อตเดิม")
	}
	value := cost - priorCost
	if in.BaseQuantity < sold-priorQty {
		value, err = proportion(cost, in.BaseQuantity, sold)
		if err != nil {
			return nil, err
		}
	}
	var u UnitSnapshot
	if err = json.Unmarshal(snapshot, &u); err != nil {
		return nil, err
	}
	// Preserve original conversion/price even if the product's current master changed.
	returnedLot, err := receiveLot(ctx, tx, a, b, p, lotNumber, expiry, "quarantined", sourceLot, "return:"+in.SourceEventID, in.BaseQuantity, value, u)
	if err != nil {
		return nil, err
	}
	var state string
	if err = tx.QueryRowContext(ctx, `SELECT state FROM v2_lots WHERE id=$1`, returnedLot).Scan(&state); err != nil {
		return nil, err
	}
	id := newID()
	if _, err = tx.ExecContext(ctx, `INSERT INTO v2_stock_returns(id,operation_id,branch_id,source_event_id,returned_lot_id,base_quantity,value_cents,reason,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, a.OperationID, b, in.SourceEventID, returnedLot, in.BaseQuantity, value, in.Reason, a.User.ID); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "lot_id": returnedLot, "base_quantity": in.BaseQuantity, "value_cents": value, "state": state, "money_adjusted": false}, emit(ctx, tx, a, "stock.returned", map[string]string{"id": id, "branch_id": b, "source_invoice_id": reference})
}

func (s *Service) lotTrace(c echo.Context) error {
	ctx := c.Request().Context()
	b, err := branch(ctx, s.db, platform.CurrentUser(c), c.QueryParam("branch_id"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	var raw []byte
	err = s.db.QueryRowContext(ctx, `WITH RECURSIVE ancestors AS (
 SELECT id,origin_lot_id FROM v2_lots WHERE id=$1 AND branch_id=$2
 UNION SELECT l.id,l.origin_lot_id FROM v2_lots l JOIN ancestors a ON a.origin_lot_id=l.id
 ), lineage AS (
 SELECT l.* FROM v2_lots l WHERE l.id IN (SELECT id FROM ancestors WHERE origin_lot_id IS NULL)
 UNION SELECT child.* FROM v2_lots child JOIN lineage parent ON child.origin_lot_id=parent.id
 ) SELECT CASE WHEN EXISTS(SELECT 1 FROM ancestors) THEN jsonb_build_object(
 'lots',COALESCE((SELECT jsonb_agg(row_to_json(l) ORDER BY l.received_at) FROM lineage l),'[]'::jsonb),
 'movements',COALESCE((SELECT jsonb_agg(jsonb_build_object('event',row_to_json(e),'document_number',d.document_number,'customer_snapshot',d.counterparty_snapshot,'customer_traceable',d.customer_id IS NOT NULL) ORDER BY e.sequence_no) FROM v2_stock_events e JOIN lineage l ON l.id=e.lot_id LEFT JOIN v2_documents d ON d.id::text=e.reference),'[]'::jsonb),
 'states',COALESCE((SELECT jsonb_agg(row_to_json(ev)) FROM v2_lot_events ev JOIN lineage l ON l.id=ev.lot_id),'[]'::jsonb)) ELSE NULL END`, c.Param("id"), b).Scan(&raw)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(500, "โหลดประวัติล็อตไม่ได้", err))
	}
	if len(raw) == 0 {
		return platform.HandleHTTPError(c, platform.NewError(404, "ไม่พบล็อตในสาขานี้"))
	}
	return c.JSONBlob(200, raw)
}
