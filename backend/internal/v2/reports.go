package v2

import (
	"github.com/labstack/echo/v4"
	"pharmacy-erp/backend/internal/platform"
)

// Historical balances come exclusively from immutable effective-date events.
func (s *Service) statements(c echo.Context) error {
	ctx := c.Request().Context()
	b, err := branch(ctx, s.db, platform.CurrentUser(c), c.QueryParam("branch_id"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	from, to := c.QueryParam("from"), c.QueryParam("to")
	if to == "" {
		to = today()
	}
	if from == "" {
		from = to[:7] + "-01"
	}
	if err = date(from); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	if err = date(to); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	if from > to {
		return platform.HandleHTTPError(c, bad("ช่วงวันไม่ถูกต้อง"))
	}
	customer, supplier := c.QueryParam("customer_id"), c.QueryParam("supplier_id")
	var raw []byte
	err = s.db.QueryRowContext(ctx, `WITH balances AS (
 SELECT d.id,d.document_number,d.kind,d.customer_id,d.supplier_id,d.counterparty_snapshot,d.issued_on,d.due_on,d.total_cents,
 COALESCE(SUM(e.amount_cents) FILTER(WHERE e.effective_on<$2::date),0)::bigint AS opening_cents,
 COALESCE(SUM(e.amount_cents) FILTER(WHERE e.effective_on BETWEEN $2::date AND $3::date AND e.amount_cents>0),0)::bigint AS increases_cents,
 -COALESCE(SUM(e.amount_cents) FILTER(WHERE e.effective_on BETWEEN $2::date AND $3::date AND e.amount_cents<0),0)::bigint AS decreases_cents,
 COALESCE(SUM(e.amount_cents),0)::bigint AS closing_cents,
 GREATEST(0,$3::date-d.due_on) AS overdue_days
 FROM v2_documents d LEFT JOIN v2_money_events e ON e.document_id=d.id AND e.effective_on<=$3::date
 WHERE d.branch_id=$1 AND d.kind IN ('ar_invoice','ap_bill') AND d.issued_on<=$3::date
 AND ($4='' OR d.customer_id=NULLIF($4,'')::uuid) AND ($5='' OR d.supplier_id=NULLIF($5,'')::uuid)
 GROUP BY d.id
 ) SELECT jsonb_build_object('from',$2::text,'to',$3::text,'items',COALESCE((SELECT jsonb_agg(row_to_json(x) ORDER BY x.due_on,x.id) FROM balances x),'[]'::jsonb),
 'aging',COALESCE((SELECT jsonb_agg(row_to_json(x)) FROM (
 SELECT kind,SUM(GREATEST(closing_cents,0)) FILTER(WHERE due_on>=$3::date) AS current_cents,
 SUM(GREATEST(closing_cents,0)) FILTER(WHERE overdue_days BETWEEN 1 AND 30) AS days_1_30_cents,
 SUM(GREATEST(closing_cents,0)) FILTER(WHERE overdue_days BETWEEN 31 AND 60) AS days_31_60_cents,
 SUM(GREATEST(closing_cents,0)) FILTER(WHERE overdue_days BETWEEN 61 AND 90) AS days_61_90_cents,
 SUM(GREATEST(closing_cents,0)) FILTER(WHERE overdue_days>90) AS days_91_plus_cents,
 SUM(GREATEST(-closing_cents,0)) AS credit_cents FROM balances GROUP BY kind)x),'[]'::jsonb),
 'events',COALESCE((SELECT jsonb_agg(row_to_json(x) ORDER BY x.effective_on,x.created_at) FROM (
 SELECT e.*,d.document_number FROM v2_money_events e JOIN v2_documents d ON d.id=e.document_id WHERE d.branch_id=$1 AND d.kind IN ('ar_invoice','ap_bill')
 AND e.effective_on BETWEEN $2::date AND $3::date AND ($4='' OR d.customer_id=NULLIF($4,'')::uuid) AND ($5='' OR d.supplier_id=NULLIF($5,'')::uuid))x),'[]'::jsonb))`, b, from, to, customer, supplier).Scan(&raw)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(500, "โหลด statement V2 ไม่สำเร็จ", err))
	}
	return c.JSONBlob(200, raw)
}

const stockReportQuery = `WITH allocated AS (
 SELECT l.id,l.product_id,l.lot_number,l.expires_on,l.state,l.remaining_quantity,p.name AS product_name,a.value_cents,a.base_quantity,bp.cost_method,
 CASE WHEN a.base_quantity=0 THEN 0 ELSE FLOOR(a.value_cents::numeric*l.remaining_quantity/a.base_quantity)::bigint END AS provisional_value,
 ROW_NUMBER() OVER(PARTITION BY l.product_id ORDER BY l.id DESC) AS remainder_row
 FROM v2_lots l JOIN products p ON p.id=l.product_id JOIN v2_stock_accounts a ON a.branch_id=l.branch_id AND a.product_id=l.product_id JOIN v2_branch_policies bp ON bp.branch_id=l.branch_id
 WHERE l.branch_id=$1 AND l.remaining_quantity>0
 ), valued AS (
 SELECT *,provisional_value+CASE WHEN remainder_row=1 THEN value_cents-SUM(provisional_value) OVER(PARTITION BY product_id) ELSE 0 END AS carrying_value_cents
 FROM allocated
 ) SELECT jsonb_build_object('valuation_basis','product carrying value allocated by physical quantity; remainder assigned by lot ID',
 'lots',COALESCE((SELECT jsonb_agg(row_to_json(v) ORDER BY v.expires_on,v.id) FROM valued v),'[]'::jsonb),
 'reconciliation',COALESCE((SELECT jsonb_agg(row_to_json(x)) FROM (
 SELECT a.product_id,a.base_quantity,a.value_cents,COALESCE((SELECT SUM(l.remaining_quantity) FROM v2_lots l WHERE l.branch_id=a.branch_id AND l.product_id=a.product_id),0)::bigint AS lot_quantity,
 COALESCE((SELECT SUM(e.quantity_delta) FROM v2_stock_events e WHERE e.branch_id=a.branch_id AND e.product_id=a.product_id),0)::bigint AS ledger_quantity,
 COALESCE((SELECT SUM(e.value_delta_cents) FROM v2_stock_events e WHERE e.branch_id=a.branch_id AND e.product_id=a.product_id),0)::bigint AS ledger_value_cents
 FROM v2_stock_accounts a WHERE a.branch_id=$1)x),'[]'::jsonb),
 'write_offs',COALESCE((SELECT jsonb_agg(row_to_json(e) ORDER BY e.created_at DESC) FROM v2_stock_events e WHERE e.branch_id=$1 AND e.event_type='write_off'),'[]'::jsonb))`
