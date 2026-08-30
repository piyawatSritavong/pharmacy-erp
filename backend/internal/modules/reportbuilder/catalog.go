package reportbuilder

import "strings"

func Catalog() CatalogResponse {
	return CatalogResponse{
		DefinitionVersion: DefinitionVersion,
		Datasets:          datasets(),
		Operators: []Operator{
			{Key: "eq", Label: "เท่ากับ", ValueCount: 1, Types: []string{"text", "number", "integer", "boolean", "date", "datetime", "uuid"}},
			{Key: "neq", Label: "ไม่เท่ากับ", ValueCount: 1, Types: []string{"text", "number", "integer", "boolean", "date", "datetime", "uuid"}},
			{Key: "contains", Label: "มีข้อความ", ValueCount: 1, Types: []string{"text"}},
			{Key: "starts_with", Label: "ขึ้นต้นด้วย", ValueCount: 1, Types: []string{"text"}},
			{Key: "in", Label: "อยู่ในรายการ", ValueCount: -1, Types: []string{"text", "number", "integer", "date", "datetime", "uuid"}},
			{Key: "gt", Label: "มากกว่า", ValueCount: 1, Types: []string{"number", "integer", "date", "datetime"}},
			{Key: "gte", Label: "มากกว่าหรือเท่ากับ", ValueCount: 1, Types: []string{"number", "integer", "date", "datetime"}},
			{Key: "lt", Label: "น้อยกว่า", ValueCount: 1, Types: []string{"number", "integer", "date", "datetime"}},
			{Key: "lte", Label: "น้อยกว่าหรือเท่ากับ", ValueCount: 1, Types: []string{"number", "integer", "date", "datetime"}},
			{Key: "between", Label: "อยู่ระหว่าง", ValueCount: 2, Types: []string{"number", "integer", "date", "datetime"}},
			{Key: "is_null", Label: "ไม่มีข้อมูล", ValueCount: 0, Types: []string{"text", "number", "integer", "boolean", "date", "datetime", "uuid"}},
			{Key: "not_null", Label: "มีข้อมูล", ValueCount: 0, Types: []string{"text", "number", "integer", "boolean", "date", "datetime", "uuid"}},
		},
		Aggregates:      []CatalogChoice{{Key: "sum", Label: "ผลรวม"}, {Key: "count", Label: "จำนวน"}, {Key: "avg", Label: "ค่าเฉลี่ย"}, {Key: "min", Label: "ต่ำสุด"}, {Key: "max", Label: "สูงสุด"}},
		Formats:         []CatalogChoice{{Key: "text", Label: "ข้อความ"}, {Key: "number", Label: "ตัวเลข"}, {Key: "currency", Label: "สกุลเงิน"}, {Key: "date", Label: "วันที่"}, {Key: "datetime", Label: "วันที่และเวลา"}},
		Limits:          map[string]int{"columns": 50, "filters": 25, "sorts": 5, "relation_depth": 4, "timeout_seconds": 8},
		PageSizes:       []int{50, 100, 250, 500},
		TimelineBuckets: []CatalogChoice{{Key: "auto", Label: "อัตโนมัติ"}, {Key: "hour", Label: "รายชั่วโมง"}, {Key: "day", Label: "รายวัน"}, {Key: "week", Label: "รายสัปดาห์"}, {Key: "month", Label: "รายเดือน"}},
		FilterLogic:     []CatalogChoice{{Key: "and", Label: "ตรงทุกเงื่อนไข (AND)"}, {Key: "or", Label: "ตรงอย่างน้อยหนึ่งเงื่อนไข (OR)"}},
	}
}

func datasetByKey(key string) (Dataset, bool) {
	for _, item := range datasets() {
		if item.Key == key {
			return item, true
		}
	}
	return Dataset{}, false
}

func fieldByKey(dataset Dataset, key string) (Field, bool) {
	for _, item := range dataset.Fields {
		if item.Key == key {
			return item, true
		}
	}
	return Field{}, false
}

func field(key, label, group, dataType, expression, relation string, depth int) Field {
	aggregates := []string{"count"}
	formats := []string{"text"}
	switch dataType {
	case "number":
		aggregates = []string{"sum", "count", "avg", "min", "max"}
		formats = []string{"number", "currency"}
	case "integer":
		aggregates = []string{"sum", "count", "avg", "min", "max"}
		formats = []string{"number"}
	case "date":
		aggregates = []string{"count", "min", "max"}
		formats = []string{"date"}
	case "datetime":
		aggregates = []string{"count", "min", "max"}
		formats = []string{"datetime", "date"}
	case "boolean":
		formats = []string{"text"}
	case "uuid":
		formats = []string{"text"}
	}
	return Field{Key: key, Label: label, Group: group, Type: dataType, Expression: expression, RelationKey: relation, RelationDepth: depth, Filterable: true, Sortable: true, Groupable: true, Aggregates: aggregates, Formats: formats}
}

func relation(key, label, description, cardinality string, depth int) Relation {
	return Relation{Key: key, Label: label, Description: description, Cardinality: cardinality, Depth: depth}
}

func datasets() []Dataset {
	return []Dataset{
		branchProductOverviewDataset(),
		suppliersDataset(),
		purchaseOrderItemsDataset(),
		inventoryLotsDataset(),
		inventoryMovementsDataset(),
		salesDataset(),
		paymentsDataset(),
		quotationsDataset(),
		transfersDataset(),
		transferEventsDataset(),
		transferRequestsDataset(),
		monthEndDataset(),
		monthEndRenumberedDataset(),
		marketplaceDataset(),
		usersDataset(),
		auditDataset(),
	}
}

func branchProductOverviewDataset() Dataset {
	fields := []Field{
		field("branch_id", "รหัสอ้างอิงสาขา", "สาขา", "uuid", "r.branch_id", "branch", 1),
		field("branch_code", "รหัสสาขา", "สาขา", "text", "r.branch_code", "branch", 1),
		field("branch_name", "ชื่อสาขา", "สาขา", "text", "r.branch_name", "branch", 1),
		field("branch_type", "ประเภทสาขา", "สาขา", "text", "r.branch_type", "branch", 1),
		field("sales_enabled", "เปิดขาย", "สาขา", "boolean", "r.sales_enabled", "branch", 1),
		field("product_id", "รหัสอ้างอิงสินค้า", "สินค้า", "uuid", "r.product_id", "product", 1),
		field("sku", "SKU", "สินค้า", "text", "r.sku", "product", 1),
		field("barcode", "บาร์โค้ด", "สินค้า", "text", "r.barcode", "product", 1),
		field("product_name", "ชื่อสินค้า", "สินค้า", "text", "r.product_name", "product", 1),
		field("category_name", "ประเภทสินค้า", "ประเภทสินค้า", "text", "r.category_name", "category", 2),
		field("unit_name", "หน่วย", "สินค้า", "text", "r.unit_name", "product", 1),
		field("cost_price", "ราคาทุน", "ราคา", "number", "r.cost_price", "pricing", 2),
		field("base_selling_price", "ราคาตั้งต้นจากโกดัง", "ราคา", "number", "r.base_selling_price", "pricing", 2),
		field("branch_price_override", "ราคา Override ของสาขา", "ราคา", "number", "r.branch_price_override", "pricing", 2),
		field("branch_selling_price", "ราคาขายที่มีผล", "ราคา", "number", "r.branch_selling_price", "pricing", 2),
		field("selling_price_source", "แหล่งราคาขาย", "ราคา", "text", "r.selling_price_source", "pricing", 2),
		field("max_discount_amount", "ลดได้สูงสุดต่อหน่วย", "ราคา", "number", "r.max_discount_amount", "pricing", 2),
		field("low_stock_real_threshold", "จุดเตือนสต๊อกจริง", "คลังปัจจุบัน", "integer", "r.low_stock_real_threshold", "inventory", 2),
		field("low_stock_ghost_threshold", "จุดเตือนสต๊อกผี", "คลังปัจจุบัน", "integer", "r.low_stock_ghost_threshold", "inventory", 2),
		field("qty_real", "สต๊อกจริง", "คลังปัจจุบัน", "integer", "r.qty_real", "inventory", 2),
		field("qty_ghost", "สต๊อกผี", "คลังปัจจุบัน", "integer", "r.qty_ghost", "inventory", 2),
		field("qty_total", "สต๊อกรวม", "คลังปัจจุบัน", "integer", "r.qty_total", "inventory", 2),
		field("sales_quantity", "จำนวนขาย", "การขาย", "integer", "r.sales_quantity", "sales", 2),
		field("sales_revenue", "ยอดขาย", "การขาย", "number", "r.sales_revenue", "sales", 2),
		field("invoice_count", "จำนวนบิล", "การขาย", "integer", "r.invoice_count", "sales", 2),
		field("latest_sale_at", "ขายล่าสุด", "การขาย", "datetime", "r.latest_sale_at", "sales", 2),
		field("movement_in", "รับเข้า", "ความเคลื่อนไหว", "integer", "r.movement_in", "movements", 2),
		field("movement_out", "จ่ายออก", "ความเคลื่อนไหว", "integer", "r.movement_out", "movements", 2),
		field("movement_net", "สุทธิการเคลื่อนไหว", "ความเคลื่อนไหว", "integer", "r.movement_net", "movements", 2),
		field("movement_count", "จำนวน movement", "ความเคลื่อนไหว", "integer", "r.movement_count", "movements", 2),
		field("latest_movement_at", "เคลื่อนไหวล่าสุด", "ความเคลื่อนไหว", "datetime", "r.latest_movement_at", "movements", 2),
		field("transfer_in", "โอนเข้า", "การโอน", "integer", "r.transfer_in", "transfers", 2),
		field("transfer_out", "โอนออก", "การโอน", "integer", "r.transfer_out", "transfers", 2),
		field("transfer_count", "จำนวนรายการโอน", "การโอน", "integer", "r.transfer_count", "transfers", 2),
		field("latest_transfer_at", "โอนล่าสุด", "การโอน", "datetime", "r.latest_transfer_at", "transfers", 2),
		field("purchase_quantity", "จำนวนซื้อเข้า", "การจัดซื้อ", "integer", "r.purchase_quantity", "purchases", 2),
		field("purchase_amount", "มูลค่าซื้อเข้า", "การจัดซื้อ", "number", "r.purchase_amount", "purchases", 2),
		field("latest_purchase_at", "ซื้อล่าสุด", "การจัดซื้อ", "datetime", "r.latest_purchase_at", "purchases", 2),
		field("latest_supplier_name", "คู่ค้าล่าสุด", "การจัดซื้อ", "text", "r.latest_supplier_name", "purchases", 2),
		field("nearest_expiry_on", "หมดอายุใกล้ที่สุด", "Lot/วันหมดอายุ", "date", "r.nearest_expiry_on", "lots", 2),
		field("expiring_quantity", "จำนวนใกล้หมดอายุ", "Lot/วันหมดอายุ", "integer", "r.expiring_quantity", "lots", 2),
		field("expired_quantity", "จำนวนหมดอายุ", "Lot/วันหมดอายุ", "integer", "r.expired_quantity", "lots", 2),
		field("latest_activity_at", "กิจกรรมล่าสุด", "เวลา", "datetime", "r.latest_activity_at", "activity", 3),
	}
	return Dataset{
		Key: "branch_product_overview", Label: "ภาพรวมสินค้าแยกสาขา", Description: "สต๊อก ยอดขาย การเข้า-ออก และการโอนของสินค้าแต่ละรายการในทุกสาขา", Grain: "หนึ่งแถวต่อสินค้าและสาขา",
		DefaultColumns: []string{"branch_name", "sku", "product_name", "category_name", "qty_real", "qty_ghost", "sales_quantity", "sales_revenue", "movement_in", "movement_out", "transfer_in", "transfer_out", "latest_activity_at"}, DefaultTimeField: "latest_activity_at", Fields: fields,
		Relations: []Relation{
			relation("branch", "สาขา", "ข้อมูลสาขาของรายการ", "many-to-one", 1), relation("product", "สินค้า", "ข้อมูลหลักของสินค้า", "many-to-one", 1), relation("category", "ประเภทสินค้า", "ประเภทของสินค้า", "many-to-one", 2), relation("pricing", "ราคา", "ราคาหลักและราคาประจำสาขา", "one-to-one", 2), relation("inventory", "คลังปัจจุบัน", "ยอดคงเหลือปัจจุบัน", "one-to-one", 2), relation("sales", "การขาย", "ยอดขายที่รวมก่อนเชื่อมข้อมูล", "one-to-many-aggregated", 2), relation("movements", "ความเคลื่อนไหว", "การรับเข้าและจ่ายออกที่รวมก่อนเชื่อมข้อมูล", "one-to-many-aggregated", 2), relation("transfers", "การโอน", "การโอนเข้าและออกที่รวมก่อนเชื่อมข้อมูล", "one-to-many-aggregated", 2), relation("purchases", "การจัดซื้อ", "ยอดจัดซื้อที่รวมก่อนเชื่อมข้อมูล", "one-to-many-aggregated", 2), relation("lots", "Lot/วันหมดอายุ", "สถานะ lot ที่รวมก่อนเชื่อมข้อมูล", "one-to-many-aggregated", 2), relation("activity", "กิจกรรมล่าสุด", "เวลาล่าสุดจากการขาย movement โอน หรือซื้อ", "derived", 3),
		},
		Source: branchProductOverviewSource,
	}
}

const branchProductOverviewSource = `(
WITH sales AS (
    SELECT i.branch_id, ii.product_id,
           SUM(ii.quantity)::bigint AS sales_quantity,
           SUM(ii.line_total)::numeric AS sales_revenue,
           COUNT(DISTINCT i.id)::bigint AS invoice_count,
           MAX(i.issued_at) AS latest_sale_at
    FROM invoices i
    INNER JOIN invoice_items ii ON ii.invoice_id = i.id
    WHERE i.invoice_status = 'issued' AND i.deleted_at IS NULL
    GROUP BY i.branch_id, ii.product_id
), movements AS (
    SELECT branch_id, product_id,
           SUM(CASE WHEN quantity_delta > 0 THEN quantity_delta ELSE 0 END)::bigint AS movement_in,
           SUM(CASE WHEN quantity_delta < 0 THEN -quantity_delta ELSE 0 END)::bigint AS movement_out,
           SUM(quantity_delta)::bigint AS movement_net,
           COUNT(*)::bigint AS movement_count,
           MAX(created_at) AS latest_movement_at
    FROM inventory_movements
    GROUP BY branch_id, product_id
), transfer_sides AS (
    SELECT t.source_branch_id AS branch_id, ti.product_id, 0::bigint AS transfer_in,
           SUM(ti.quantity)::bigint AS transfer_out, COUNT(DISTINCT t.id)::bigint AS transfer_count,
           MAX(COALESCE(t.received_at, t.dispatched_at, t.requested_at)) AS latest_transfer_at
    FROM transfers t
    INNER JOIN transfer_items ti ON ti.transfer_id = t.id
    WHERE t.status IN ('in_transit', 'completed')
    GROUP BY t.source_branch_id, ti.product_id
    UNION ALL
    SELECT t.destination_branch_id, ti.product_id, SUM(COALESCE(ti.received_quantity, ti.quantity))::bigint,
           0::bigint, COUNT(DISTINCT t.id)::bigint,
           MAX(COALESCE(t.received_at, t.dispatched_at, t.requested_at))
    FROM transfers t
    INNER JOIN transfer_items ti ON ti.transfer_id = t.id
    WHERE t.status IN ('in_transit', 'completed')
    GROUP BY t.destination_branch_id, ti.product_id
), transfer_totals AS (
    SELECT branch_id, product_id, SUM(transfer_in)::bigint AS transfer_in,
           SUM(transfer_out)::bigint AS transfer_out, SUM(transfer_count)::bigint AS transfer_count,
           MAX(latest_transfer_at) AS latest_transfer_at
    FROM transfer_sides
    GROUP BY branch_id, product_id
), purchases AS (
    SELECT po.branch_id,poi.product_id,SUM(poi.received_quantity)::bigint AS purchase_quantity,
           SUM(poi.line_total)::numeric AS purchase_amount,MAX(po.purchased_at) AS latest_purchase_at
    FROM purchase_orders po INNER JOIN purchase_order_items poi ON poi.purchase_order_id=po.id
    WHERE po.status='posted' GROUP BY po.branch_id,poi.product_id
), latest_supplier AS (
    SELECT DISTINCT ON(po.branch_id,poi.product_id) po.branch_id,poi.product_id,po.supplier_name_snapshot
    FROM purchase_orders po INNER JOIN purchase_order_items poi ON poi.purchase_order_id=po.id
    WHERE po.status='posted' ORDER BY po.branch_id,poi.product_id,po.purchased_at DESC,po.created_at DESC
), lot_totals AS (
    SELECT il.branch_id,il.product_id,
           MIN(il.expires_on) FILTER(WHERE il.remaining_quantity>0 AND il.expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date) AS nearest_expiry_on,
           COALESCE(SUM(il.remaining_quantity) FILTER(WHERE il.expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date AND il.expires_on <= (NOW() AT TIME ZONE 'Asia/Bangkok')::date + p.expiry_warning_days),0)::bigint AS expiring_quantity,
           COALESCE(SUM(il.remaining_quantity) FILTER(WHERE il.expires_on < (NOW() AT TIME ZONE 'Asia/Bangkok')::date),0)::bigint AS expired_quantity
    FROM inventory_lots il INNER JOIN products p ON p.id=il.product_id
    GROUP BY il.branch_id,il.product_id
)
SELECT b.id::text AS branch_id, b.code AS branch_code, b.name AS branch_name, b.branch_type,b.sales_enabled,
       p.id::text AS product_id, p.sku, COALESCE(p.barcode, '') AS barcode, p.name AS product_name,
       COALESCE(pc.name, 'ไม่ระบุประเภท') AS category_name, p.unit_name, p.cost_price,
	   p.base_selling_price,bpp.selling_price AS branch_price_override,
	   COALESCE(bpp.selling_price, p.base_selling_price) AS branch_selling_price,
	   CASE WHEN bpp.selling_price IS NULL THEN 'warehouse' ELSE 'branch_override' END AS selling_price_source,
       COALESCE(bps.max_discount_amount,p.max_discount_amount) AS max_discount_amount,
       COALESCE(bps.low_stock_real_threshold,p.low_stock_real_threshold)::bigint AS low_stock_real_threshold,
       COALESCE(bps.low_stock_ghost_threshold,p.low_stock_ghost_threshold)::bigint AS low_stock_ghost_threshold,
       COALESCE(inv.qty_real, 0)::bigint AS qty_real, COALESCE(inv.qty_ghost, 0)::bigint AS qty_ghost,
       (COALESCE(inv.qty_real, 0) + COALESCE(inv.qty_ghost, 0))::bigint AS qty_total,
       COALESCE(s.sales_quantity, 0)::bigint AS sales_quantity, COALESCE(s.sales_revenue, 0)::numeric AS sales_revenue,
       COALESCE(s.invoice_count, 0)::bigint AS invoice_count, s.latest_sale_at,
       COALESCE(m.movement_in, 0)::bigint AS movement_in, COALESCE(m.movement_out, 0)::bigint AS movement_out,
       COALESCE(m.movement_net, 0)::bigint AS movement_net, COALESCE(m.movement_count, 0)::bigint AS movement_count,
       m.latest_movement_at, COALESCE(tt.transfer_in, 0)::bigint AS transfer_in,
       COALESCE(tt.transfer_out, 0)::bigint AS transfer_out, COALESCE(tt.transfer_count, 0)::bigint AS transfer_count,
       tt.latest_transfer_at,COALESCE(pu.purchase_quantity,0)::bigint AS purchase_quantity,
       COALESCE(pu.purchase_amount,0)::numeric AS purchase_amount,pu.latest_purchase_at,
       COALESCE(ls.supplier_name_snapshot,'') AS latest_supplier_name,lt.nearest_expiry_on,
       COALESCE(lt.expiring_quantity,0)::bigint AS expiring_quantity,COALESCE(lt.expired_quantity,0)::bigint AS expired_quantity,
	   GREATEST(s.latest_sale_at, m.latest_movement_at, tt.latest_transfer_at,pu.latest_purchase_at) AS latest_activity_at
FROM branches b
CROSS JOIN products p
LEFT JOIN product_categories pc ON pc.id = p.category_id
LEFT JOIN branch_product_prices bpp ON bpp.branch_id = b.id AND bpp.product_id = p.id
LEFT JOIN branch_product_settings bps ON bps.branch_id=b.id AND bps.product_id=p.id
LEFT JOIN inventory inv ON inv.branch_id = b.id AND inv.product_id = p.id
LEFT JOIN sales s ON s.branch_id = b.id AND s.product_id = p.id
LEFT JOIN movements m ON m.branch_id = b.id AND m.product_id = p.id
LEFT JOIN transfer_totals tt ON tt.branch_id = b.id AND tt.product_id = p.id
LEFT JOIN purchases pu ON pu.branch_id=b.id AND pu.product_id=p.id
LEFT JOIN latest_supplier ls ON ls.branch_id=b.id AND ls.product_id=p.id
LEFT JOIN lot_totals lt ON lt.branch_id=b.id AND lt.product_id=p.id
WHERE b.active = TRUE AND p.active = TRUE
) r`

func suppliersDataset() Dataset {
	return Dataset{Key: "suppliers", Label: "บริษัทคู่ค้า", Description: "ข้อมูลบริษัทผู้จำหน่ายส่วนกลาง โดยไม่เปิดข้อมูล credential", Grain: "หนึ่งแถวต่อบริษัทคู่ค้า", DefaultColumns: []string{"supplier_code", "legal_name", "tax_id", "contact_name", "phone", "province", "payment_terms_days", "active", "updated_at"}, DefaultTimeField: "updated_at", Relations: []Relation{relation("purchase_summary", "สรุปการจัดซื้อ", "ยอด PO ที่รวมก่อนเชื่อม", "one-to-many-aggregated", 1)}, Fields: []Field{
		field("supplier_id", "รหัสอ้างอิงคู่ค้า", "คู่ค้า", "uuid", "r.supplier_id", "", 0), field("supplier_code", "รหัสคู่ค้า", "คู่ค้า", "text", "r.supplier_code", "", 0), field("legal_name", "ชื่อบริษัท", "คู่ค้า", "text", "r.legal_name", "", 0), field("tax_id", "เลขผู้เสียภาษี", "คู่ค้า", "text", "r.tax_id", "", 0), field("company_branch_type", "ประเภทสำนักงาน", "คู่ค้า", "text", "r.company_branch_type", "", 0), field("company_branch_number", "เลขที่สาขาบริษัท", "คู่ค้า", "text", "r.company_branch_number", "", 0), field("province", "จังหวัด", "ที่อยู่", "text", "r.province", "", 0), field("contact_name", "ผู้ติดต่อ", "ติดต่อ", "text", "r.contact_name", "", 0), field("phone", "โทรศัพท์", "ติดต่อ", "text", "r.phone", "", 0), field("email", "Email", "ติดต่อ", "text", "r.email", "", 0), field("payment_terms_days", "เครดิต (วัน)", "เงื่อนไข", "integer", "r.payment_terms_days", "", 0), field("active", "เปิดใช้งาน", "สถานะ", "boolean", "r.active", "", 0), field("purchase_order_count", "จำนวน PO", "สรุปการจัดซื้อ", "integer", "r.purchase_order_count", "purchase_summary", 1), field("purchase_total", "ยอดซื้อรวม", "สรุปการจัดซื้อ", "number", "r.purchase_total", "purchase_summary", 1), field("latest_purchase_at", "ซื้อล่าสุด", "สรุปการจัดซื้อ", "datetime", "r.latest_purchase_at", "purchase_summary", 1), field("created_at", "เวลาสร้าง", "เวลา", "datetime", "r.created_at", "", 0), field("updated_at", "เวลาแก้ไข", "เวลา", "datetime", "r.updated_at", "", 0),
	}, Source: `(WITH summary AS (SELECT supplier_id,COUNT(*)::bigint AS purchase_order_count,SUM(total_amount) FILTER(WHERE status='posted')::numeric AS purchase_total,MAX(purchased_at) FILTER(WHERE status='posted') AS latest_purchase_at FROM purchase_orders GROUP BY supplier_id) SELECT s.id::text AS supplier_id,s.supplier_code,s.legal_name,COALESCE(s.tax_id,'') AS tax_id,s.company_branch_type,s.company_branch_number,s.province,s.contact_name,s.phone,s.email,s.payment_terms_days,s.active,COALESCE(ps.purchase_order_count,0)::bigint AS purchase_order_count,COALESCE(ps.purchase_total,0)::numeric AS purchase_total,ps.latest_purchase_at,s.created_at,s.updated_at FROM suppliers s LEFT JOIN summary ps ON ps.supplier_id=s.id) r`}
}

func purchaseOrderItemsDataset() Dataset {
	return Dataset{Key: "purchase_order_items", Label: "รายการใบสั่งซื้อเข้า", Description: "ประวัติการซื้อสินค้า คู่ค้า ราคา Lot และวันหมดอายุ", Grain: "หนึ่งแถวต่อรายการสินค้าใน PO", DefaultColumns: []string{"purchased_at", "po_number", "branch_name", "supplier_name", "product_name", "stock_bucket", "received_quantity", "remaining_quantity", "unit_cost", "line_total", "lot_number", "expires_on", "status"}, DefaultTimeField: "purchased_at", Relations: []Relation{relation("supplier", "บริษัทคู่ค้า", "คู่ค้าของ PO", "many-to-one", 1), relation("branch", "สาขา", "สาขารับสินค้า", "many-to-one", 1), relation("product", "สินค้า", "สินค้าใน PO", "many-to-one", 1), relation("lot", "Lot", "Lot ที่สร้างจากรายการ", "one-to-one", 1), relation("actor", "ผู้บันทึก", "ผู้สร้าง PO", "many-to-one", 1)}, Fields: []Field{
		field("purchase_order_id", "รหัส PO", "เอกสาร", "uuid", "r.purchase_order_id", "", 0), field("po_number", "เลข PO", "เอกสาร", "text", "r.po_number", "", 0), field("status", "สถานะ PO", "เอกสาร", "text", "r.status", "", 0), field("purchased_at", "วันเวลาซื้อ", "เอกสาร", "datetime", "r.purchased_at", "", 0), field("supplier_document_number", "เลขเอกสารคู่ค้า", "เอกสาร", "text", "r.supplier_document_number", "", 0), field("supplier_code", "รหัสคู่ค้า", "คู่ค้า", "text", "r.supplier_code", "supplier", 1), field("supplier_name", "ชื่อบริษัทคู่ค้า", "คู่ค้า", "text", "r.supplier_name", "supplier", 1), field("supplier_tax_id", "เลขผู้เสียภาษีคู่ค้า", "คู่ค้า", "text", "r.supplier_tax_id", "supplier", 1), field("branch_code", "รหัสสาขา", "สาขา", "text", "r.branch_code", "branch", 1), field("branch_name", "สาขารับสินค้า", "สาขา", "text", "r.branch_name", "branch", 1), field("sku", "SKU", "สินค้า", "text", "r.sku", "product", 1), field("product_name", "ชื่อสินค้า", "สินค้า", "text", "r.product_name", "product", 1), field("unit_name", "หน่วย", "สินค้า", "text", "r.unit_name", "product", 1), field("stock_bucket", "ประเภทสต๊อก", "รายการ", "text", "r.stock_bucket", "", 0), field("received_quantity", "จำนวนรับเข้า", "รายการ", "integer", "r.received_quantity", "", 0), field("remaining_quantity", "คงเหลือใน Lot", "Lot", "integer", "r.remaining_quantity", "lot", 1), field("unit_cost", "ราคาซื้อต่อหน่วย", "ราคา", "number", "r.unit_cost", "", 0), field("line_discount", "ส่วนลดรายการ", "ราคา", "number", "r.line_discount", "", 0), field("tax_amount", "VAT รายการ", "ราคา", "number", "r.tax_amount", "", 0), field("line_total", "ยอดรวมรายการ", "ราคา", "number", "r.line_total", "", 0), field("lot_number", "Lot/Batch", "Lot", "text", "r.lot_number", "lot", 1), field("expires_on", "วันหมดอายุ", "Lot", "date", "r.expires_on", "lot", 1), field("expiry_status", "สถานะหมดอายุ", "Lot", "text", "r.expiry_status", "lot", 1), field("created_by_name", "ผู้บันทึก", "ผู้ใช้", "text", "r.created_by_name", "actor", 1),
	}, Source: `(SELECT po.id::text AS purchase_order_id,po.po_number,po.status,po.purchased_at,po.supplier_document_number,po.supplier_code_snapshot AS supplier_code,po.supplier_name_snapshot AS supplier_name,po.supplier_tax_id_snapshot AS supplier_tax_id,b.code AS branch_code,b.name AS branch_name,poi.product_sku_snapshot AS sku,poi.product_name_snapshot AS product_name,poi.unit_name_snapshot AS unit_name,poi.stock_bucket,poi.received_quantity::bigint AS received_quantity,COALESCE(il.remaining_quantity,0)::bigint AS remaining_quantity,poi.unit_cost,poi.line_discount,poi.tax_amount,poi.line_total,poi.lot_number,poi.expires_on,CASE WHEN poi.expires_on IS NULL THEN 'ไม่กำหนด' WHEN poi.expires_on < (NOW() AT TIME ZONE 'Asia/Bangkok')::date THEN 'หมดอายุ' WHEN poi.expires_on <= (NOW() AT TIME ZONE 'Asia/Bangkok')::date+p.expiry_warning_days THEN 'ใกล้หมดอายุ' ELSE 'ปกติ' END AS expiry_status,u.full_name AS created_by_name FROM purchase_orders po INNER JOIN purchase_order_items poi ON poi.purchase_order_id=po.id INNER JOIN branches b ON b.id=po.branch_id INNER JOIN products p ON p.id=poi.product_id INNER JOIN users u ON u.id=po.created_by LEFT JOIN inventory_lots il ON il.source_item_id=poi.id) r`}
}

func inventoryLotsDataset() Dataset {
	return Dataset{Key: "inventory_lots", Label: "Lot และวันหมดอายุ", Description: "ยอดคงเหลือราย Lot แยกสาขาและประเภทสต๊อก", Grain: "หนึ่งแถวต่อ inventory lot", DefaultColumns: []string{"received_at", "branch_name", "product_name", "stock_bucket", "lot_number", "received_quantity", "remaining_quantity", "unit_cost", "expires_on", "expiry_status", "supplier_name", "po_number"}, DefaultTimeField: "received_at", Relations: []Relation{relation("branch", "สาขา", "สาขาของ Lot", "many-to-one", 1), relation("product", "สินค้า", "สินค้าของ Lot", "many-to-one", 1), relation("purchase", "การจัดซื้อ", "PO และคู่ค้าต้นทาง", "many-to-one", 2)}, Fields: []Field{
		field("lot_id", "รหัส Lot", "Lot", "uuid", "r.lot_id", "", 0), field("received_at", "เวลารับ", "Lot", "datetime", "r.received_at", "", 0), field("lot_number", "Lot/Batch", "Lot", "text", "r.lot_number", "", 0), field("stock_bucket", "ประเภทสต๊อก", "Lot", "text", "r.stock_bucket", "", 0), field("received_quantity", "จำนวนรับ", "Lot", "integer", "r.received_quantity", "", 0), field("remaining_quantity", "จำนวนคงเหลือ", "Lot", "integer", "r.remaining_quantity", "", 0), field("unit_cost", "ต้นทุน Lot", "Lot", "number", "r.unit_cost", "", 0), field("expires_on", "วันหมดอายุ", "Lot", "date", "r.expires_on", "", 0), field("expiry_status", "สถานะหมดอายุ", "Lot", "text", "r.expiry_status", "", 0), field("source_type", "แหล่งที่มา", "อ้างอิง", "text", "r.source_type", "", 0), field("branch_code", "รหัสสาขา", "สาขา", "text", "r.branch_code", "branch", 1), field("branch_name", "ชื่อสาขา", "สาขา", "text", "r.branch_name", "branch", 1), field("sku", "SKU", "สินค้า", "text", "r.sku", "product", 1), field("product_name", "ชื่อสินค้า", "สินค้า", "text", "r.product_name", "product", 1), field("po_number", "เลข PO", "การจัดซื้อ", "text", "r.po_number", "purchase", 2), field("supplier_name", "บริษัทคู่ค้า", "การจัดซื้อ", "text", "r.supplier_name", "purchase", 2), field("purchased_at", "วันเวลาซื้อ", "การจัดซื้อ", "datetime", "r.purchased_at", "purchase", 2),
	}, Source: `(SELECT il.id::text AS lot_id,il.received_at,il.lot_number,il.stock_bucket,il.received_quantity::bigint AS received_quantity,il.remaining_quantity::bigint AS remaining_quantity,il.unit_cost,il.expires_on,CASE WHEN il.expires_on IS NULL THEN 'ไม่กำหนด' WHEN il.expires_on < (NOW() AT TIME ZONE 'Asia/Bangkok')::date THEN 'หมดอายุ' WHEN il.expires_on <= (NOW() AT TIME ZONE 'Asia/Bangkok')::date+p.expiry_warning_days THEN 'ใกล้หมดอายุ' ELSE 'ปกติ' END AS expiry_status,il.source_type,b.code AS branch_code,b.name AS branch_name,p.sku,p.name AS product_name,COALESCE(po.po_number,'') AS po_number,COALESCE(po.supplier_name_snapshot,'') AS supplier_name,po.purchased_at FROM inventory_lots il INNER JOIN branches b ON b.id=il.branch_id INNER JOIN products p ON p.id=il.product_id LEFT JOIN purchase_order_items poi ON poi.id=il.source_item_id LEFT JOIN purchase_orders po ON po.id=poi.purchase_order_id) r`}
}

func inventoryMovementsDataset() Dataset {
	return Dataset{Key: "inventory_movements", Label: "ประวัติสต๊อกเข้า-ออก", Description: "ledger การเปลี่ยนแปลงสต๊อกจริงและสต๊อกผี", Grain: "หนึ่งแถวต่อ inventory movement", DefaultColumns: []string{"created_at", "branch_name", "sku", "product_name", "movement_type", "stock_bucket", "quantity_delta", "reference_type", "performed_by_name", "note"}, DefaultTimeField: "created_at", Relations: []Relation{relation("branch", "สาขา", "สาขาที่เกิดรายการ", "many-to-one", 1), relation("product", "สินค้า", "สินค้าที่เคลื่อนไหว", "many-to-one", 1), relation("actor", "ผู้ดำเนินการ", "ผู้บันทึกรายการ", "many-to-one", 1)}, Fields: []Field{
		field("movement_id", "รหัส movement", "รายการ", "uuid", "r.movement_id", "", 0), field("created_at", "เวลา", "รายการ", "datetime", "r.created_at", "", 0), field("branch_code", "รหัสสาขา", "สาขา", "text", "r.branch_code", "branch", 1), field("branch_name", "ชื่อสาขา", "สาขา", "text", "r.branch_name", "branch", 1), field("sku", "SKU", "สินค้า", "text", "r.sku", "product", 1), field("product_name", "ชื่อสินค้า", "สินค้า", "text", "r.product_name", "product", 1), field("movement_type", "ประเภท movement", "รายการ", "text", "r.movement_type", "", 0), field("stock_bucket", "ประเภทสต๊อก", "รายการ", "text", "r.stock_bucket", "", 0), field("quantity_delta", "จำนวนเปลี่ยนแปลง", "รายการ", "integer", "r.quantity_delta", "", 0), field("reference_type", "ประเภทอ้างอิง", "อ้างอิง", "text", "r.reference_type", "", 0), field("reference_id", "รหัสอ้างอิง", "อ้างอิง", "uuid", "r.reference_id", "", 0), field("note", "หมายเหตุ", "รายการ", "text", "r.note", "", 0), field("performed_by_name", "ผู้ดำเนินการ", "ผู้ใช้", "text", "r.performed_by_name", "actor", 1),
	}, Source: `(SELECT im.id::text AS movement_id, im.created_at, b.code AS branch_code, b.name AS branch_name, p.sku, p.name AS product_name, im.movement_type, im.stock_bucket, im.quantity_delta::bigint AS quantity_delta, im.reference_type, im.reference_id::text AS reference_id, im.note, COALESCE(u.full_name, '') AS performed_by_name FROM inventory_movements im INNER JOIN branches b ON b.id = im.branch_id INNER JOIN products p ON p.id = im.product_id LEFT JOIN users u ON u.id = im.performed_by) r`}
}

func salesDataset() Dataset {
	return Dataset{Key: "sales", Label: "รายการขาย", Description: "ใบขายและรายการสินค้า พร้อมยอดชำระที่รวมต่อใบแล้ว", Grain: "หนึ่งแถวต่อรายการสินค้าในใบขาย", DefaultColumns: []string{"issued_at", "branch_name", "invoice_number", "product_name", "quantity", "unit_price", "line_total", "payment_status", "cash_paid", "bank_paid", "created_by_name"}, DefaultTimeField: "issued_at", Relations: []Relation{relation("branch", "สาขา", "สาขาที่ออกใบขาย", "many-to-one", 1), relation("product", "สินค้า", "สินค้าที่ขาย", "many-to-one", 1), relation("lot", "Lot ที่ขาย", "Lot และต้นทุนจริงของบรรทัดขาย", "many-to-one", 1), relation("payments", "การชำระเงิน", "ยอดชำระรวมก่อนเชื่อม", "one-to-many-aggregated", 2), relation("actor", "ผู้ขาย", "ผู้สร้างใบขาย", "many-to-one", 1)}, Fields: []Field{
		field("invoice_id", "รหัสใบขาย", "ใบขาย", "uuid", "r.invoice_id", "", 0), field("invoice_number", "เลขที่ใบขาย", "ใบขาย", "text", "r.invoice_number", "", 0), field("issued_at", "เวลาออกใบขาย", "ใบขาย", "datetime", "r.issued_at", "", 0), field("branch_code", "รหัสสาขา", "สาขา", "text", "r.branch_code", "branch", 1), field("branch_name", "ชื่อสาขา", "สาขา", "text", "r.branch_name", "branch", 1), field("customer_name", "ชื่อลูกค้า", "ลูกค้า", "text", "r.customer_name", "", 0), field("payment_status", "สถานะชำระ", "ใบขาย", "text", "r.payment_status", "", 0), field("invoice_status", "สถานะใบขาย", "ใบขาย", "text", "r.invoice_status", "", 0), field("tax_invoice_type", "ประเภทใบกำกับภาษี", "ภาษี", "text", "r.tax_invoice_type", "", 0), field("is_government_mode", "โหมดราชการ", "ใบขาย", "boolean", "r.is_government_mode", "", 0), field("invoice_total", "ยอดรวมใบขาย", "ใบขาย", "number", "r.invoice_total", "", 0), field("sku", "SKU", "สินค้า", "text", "r.sku", "product", 1), field("product_name", "ชื่อสินค้าจริง", "สินค้า", "text", "r.product_name", "product", 1), field("display_name", "ชื่อที่แสดง", "สินค้า", "text", "r.display_name", "product", 1), field("quantity", "จำนวน", "รายการขาย", "integer", "r.quantity", "", 0), field("stock_bucket", "ประเภทสต๊อก", "รายการขาย", "text", "r.stock_bucket", "", 0), field("unit_price", "ราคาต่อหน่วย", "รายการขาย", "number", "r.unit_price", "", 0), field("line_subtotal", "ยอดก่อนภาษี", "รายการขาย", "number", "r.line_subtotal", "", 0), field("tax_amount", "ภาษี", "รายการขาย", "number", "r.tax_amount", "", 0), field("line_total", "ยอดรวมรายการ", "รายการขาย", "number", "r.line_total", "", 0), field("cost_snapshot", "ต้นทุน Lot ณ เวลาขาย", "Lot ที่ขาย", "number", "r.cost_snapshot", "lot", 1), field("lot_number", "เลข Lot ที่ขาย", "Lot ที่ขาย", "text", "r.lot_number", "lot", 1), field("lot_received_at", "วันที่รับ Lot", "Lot ที่ขาย", "datetime", "r.lot_received_at", "lot", 1), field("lot_expires_on", "วันหมดอายุ Lot", "Lot ที่ขาย", "date", "r.lot_expires_on", "lot", 1), field("cash_paid", "ชำระเงินสด", "การชำระ", "number", "r.cash_paid", "payments", 2), field("bank_paid", "ชำระเงินโอน", "การชำระ", "number", "r.bank_paid", "payments", 2), field("created_by_name", "ผู้ขาย", "ผู้ใช้", "text", "r.created_by_name", "actor", 1),
	}, Source: `(WITH pay AS (SELECT invoice_id, SUM(CASE WHEN payment_type = 'cash' THEN amount ELSE 0 END)::numeric AS cash_paid, SUM(CASE WHEN payment_type = 'bank_transfer' THEN amount ELSE 0 END)::numeric AS bank_paid FROM invoice_payments GROUP BY invoice_id) SELECT i.id::text AS invoice_id, i.invoice_number, i.issued_at, b.code AS branch_code, b.name AS branch_name, i.customer_name, i.payment_status, i.invoice_status, i.tax_invoice_type, i.is_government_mode, i.total_amount AS invoice_total, p.sku, ii.actual_product_name AS product_name, ii.display_name, ii.quantity::bigint AS quantity, ii.stock_bucket, ii.unit_price, ii.line_subtotal, ii.tax_amount, ii.line_total, ii.cost_snapshot,COALESCE(ii.lot_number_snapshot,'') AS lot_number,ii.lot_received_at_snapshot AS lot_received_at,ii.lot_expires_on_snapshot AS lot_expires_on, COALESCE(pay.cash_paid, 0)::numeric AS cash_paid, COALESCE(pay.bank_paid, 0)::numeric AS bank_paid, u.full_name AS created_by_name FROM invoices i INNER JOIN invoice_items ii ON ii.invoice_id = i.id INNER JOIN branches b ON b.id = i.branch_id INNER JOIN products p ON p.id = ii.product_id INNER JOIN users u ON u.id = i.created_by LEFT JOIN pay ON pay.invoice_id = i.id WHERE i.deleted_at IS NULL) r`}
}

func paymentsDataset() Dataset {
	return Dataset{Key: "invoice_payments", Label: "ประวัติการชำระเงิน", Description: "รายการชำระเงินของใบขาย โดยไม่เปิดข้อมูลภายในหรือ credential", Grain: "หนึ่งแถวต่อการชำระเงิน", DefaultColumns: []string{"created_at", "branch_name", "invoice_number", "customer_name", "payment_type", "amount", "reference_code", "created_by_name"}, DefaultTimeField: "created_at", Relations: []Relation{relation("invoice", "ใบขาย", "ใบขายที่รับชำระ", "many-to-one", 1), relation("branch", "สาขา", "สาขาของใบขาย", "many-to-one", 2), relation("actor", "ผู้รับชำระ", "ผู้บันทึกการชำระ", "many-to-one", 1)}, Fields: []Field{
		field("payment_id", "รหัสการชำระ", "การชำระ", "uuid", "r.payment_id", "", 0), field("created_at", "เวลาชำระ", "การชำระ", "datetime", "r.created_at", "", 0), field("payment_type", "ช่องทางชำระ", "การชำระ", "text", "r.payment_type", "", 0), field("amount", "จำนวนเงิน", "การชำระ", "number", "r.amount", "", 0), field("reference_code", "เลขอ้างอิง", "การชำระ", "text", "r.reference_code", "", 0), field("notes", "หมายเหตุ", "การชำระ", "text", "r.notes", "", 0), field("invoice_number", "เลขที่ใบขาย", "ใบขาย", "text", "r.invoice_number", "invoice", 1), field("issued_at", "เวลาออกใบขาย", "ใบขาย", "datetime", "r.issued_at", "invoice", 1), field("customer_name", "ลูกค้า", "ใบขาย", "text", "r.customer_name", "invoice", 1), field("invoice_status", "สถานะใบขาย", "ใบขาย", "text", "r.invoice_status", "invoice", 1), field("payment_status", "สถานะชำระ", "ใบขาย", "text", "r.payment_status", "invoice", 1), field("invoice_total", "ยอดใบขาย", "ใบขาย", "number", "r.invoice_total", "invoice", 1), field("branch_code", "รหัสสาขา", "สาขา", "text", "r.branch_code", "branch", 2), field("branch_name", "สาขา", "สาขา", "text", "r.branch_name", "branch", 2), field("created_by_name", "ผู้รับชำระ", "ผู้ใช้", "text", "r.created_by_name", "actor", 1),
	}, Source: `(SELECT ip.id::text AS payment_id, ip.created_at, ip.payment_type, ip.amount, ip.reference_code, ip.notes, i.invoice_number, i.issued_at, i.customer_name, i.invoice_status, i.payment_status, i.total_amount AS invoice_total, b.code AS branch_code, b.name AS branch_name, u.full_name AS created_by_name FROM invoice_payments ip INNER JOIN invoices i ON i.id = ip.invoice_id INNER JOIN branches b ON b.id = i.branch_id INNER JOIN users u ON u.id = ip.created_by WHERE i.deleted_at IS NULL) r`}
}

func quotationsDataset() Dataset {
	return Dataset{Key: "quotations", Label: "ใบเสนอราคา", Description: "ใบเสนอราคาและรายการสินค้าภายในเอกสาร", Grain: "หนึ่งแถวต่อรายการสินค้าในใบเสนอราคา", DefaultColumns: []string{"created_at", "branch_name", "quote_number", "customer_name", "status", "product_name", "quantity", "unit_price", "line_subtotal", "created_by_name"}, DefaultTimeField: "created_at", Relations: []Relation{relation("branch", "สาขา", "สาขาของใบเสนอราคา", "many-to-one", 1), relation("product", "สินค้า", "สินค้าในเอกสาร", "many-to-one", 1), relation("actor", "ผู้สร้าง", "ผู้สร้างเอกสาร", "many-to-one", 1)}, Fields: []Field{
		field("quotation_id", "รหัสใบเสนอราคา", "เอกสาร", "uuid", "r.quotation_id", "", 0), field("quote_number", "เลขที่ใบเสนอราคา", "เอกสาร", "text", "r.quote_number", "", 0), field("created_at", "วันที่สร้าง", "เอกสาร", "datetime", "r.created_at", "", 0), field("expires_at", "วันหมดอายุ", "เอกสาร", "datetime", "r.expires_at", "", 0), field("branch_name", "ชื่อสาขา", "สาขา", "text", "r.branch_name", "branch", 1), field("customer_name", "ชื่อลูกค้า", "ลูกค้า", "text", "r.customer_name", "", 0), field("status", "สถานะ", "เอกสาร", "text", "r.status", "", 0), field("quotation_total", "ยอดรวมเอกสาร", "เอกสาร", "number", "r.quotation_total", "", 0), field("sku", "SKU", "สินค้า", "text", "r.sku", "product", 1), field("product_name", "ชื่อสินค้า", "สินค้า", "text", "r.product_name", "product", 1), field("display_name", "ชื่อที่แสดง", "สินค้า", "text", "r.display_name", "product", 1), field("quantity", "จำนวน", "รายการ", "integer", "r.quantity", "", 0), field("stock_bucket", "ประเภทสต๊อก", "รายการ", "text", "r.stock_bucket", "", 0), field("unit_price", "ราคาต่อหน่วย", "รายการ", "number", "r.unit_price", "", 0), field("line_subtotal", "ยอดรายการ", "รายการ", "number", "r.line_subtotal", "", 0), field("created_by_name", "ผู้สร้าง", "ผู้ใช้", "text", "r.created_by_name", "actor", 1),
	}, Source: `(SELECT q.id::text AS quotation_id, q.quote_number, q.created_at, q.expires_at, b.name AS branch_name, q.customer_name, q.status, q.total_amount AS quotation_total, p.sku, p.name AS product_name, qi.display_name, qi.quantity::bigint AS quantity, qi.stock_bucket, qi.unit_price, qi.line_subtotal, u.full_name AS created_by_name FROM quotations q INNER JOIN quotation_items qi ON qi.quotation_id = q.id INNER JOIN branches b ON b.id = q.branch_id INNER JOIN products p ON p.id = qi.product_id INNER JOIN users u ON u.id = q.created_by) r`}
}

func transfersDataset() Dataset {
	return Dataset{Key: "transfers", Label: "การโอนสินค้า", Description: "รายการโอนสินค้า ต้นทาง ปลายทาง และเหตุการณ์ล่าสุด", Grain: "หนึ่งแถวต่อสินค้าในใบโอน", DefaultColumns: []string{"requested_at", "transfer_code", "source_branch_name", "destination_branch_name", "status", "product_name", "quantity", "received_quantity", "latest_event_status", "latest_event_at"}, DefaultTimeField: "requested_at", Relations: []Relation{relation("source_branch", "สาขาต้นทาง", "สาขาที่ส่งสินค้า", "many-to-one", 1), relation("destination_branch", "สาขาปลายทาง", "สาขาที่รับสินค้า", "many-to-one", 1), relation("product", "สินค้า", "สินค้าที่โอน", "many-to-one", 1), relation("events", "เหตุการณ์โอน", "เหตุการณ์ล่าสุดที่รวมก่อนเชื่อม", "one-to-many-aggregated", 2)}, Fields: []Field{
		field("transfer_id", "รหัสการโอน", "การโอน", "uuid", "r.transfer_id", "", 0), field("transfer_code", "เลขที่โอน", "การโอน", "text", "r.transfer_code", "", 0), field("status", "สถานะ", "การโอน", "text", "r.status", "", 0), field("source_branch_name", "สาขาต้นทาง", "สาขา", "text", "r.source_branch_name", "source_branch", 1), field("destination_branch_name", "สาขาปลายทาง", "สาขา", "text", "r.destination_branch_name", "destination_branch", 1), field("requested_at", "เวลาขอโอน", "เวลา", "datetime", "r.requested_at", "", 0), field("dispatched_at", "เวลาส่ง", "เวลา", "datetime", "r.dispatched_at", "", 0), field("received_at", "เวลารับ", "เวลา", "datetime", "r.received_at", "", 0), field("sku", "SKU", "สินค้า", "text", "r.sku", "product", 1), field("product_name", "ชื่อสินค้า", "สินค้า", "text", "r.product_name", "product", 1), field("quantity", "จำนวนส่ง", "รายการ", "integer", "r.quantity", "", 0), field("received_quantity", "จำนวนรับจริง", "รายการ", "integer", "r.received_quantity", "", 0), field("stock_bucket", "ประเภทสต๊อก", "รายการ", "text", "r.stock_bucket", "", 0), field("discrepancy_note", "หมายเหตุส่วนต่าง", "รายการ", "text", "r.discrepancy_note", "", 0), field("request_note", "หมายเหตุคำขอ", "การโอน", "text", "r.request_note", "", 0), field("pickup_name", "ผู้รับสินค้า", "ขนส่ง", "text", "r.pickup_name", "", 0), field("courier_name", "ผู้ขนส่ง", "ขนส่ง", "text", "r.courier_name", "", 0), field("latest_event_status", "เหตุการณ์ล่าสุด", "เหตุการณ์", "text", "r.latest_event_status", "events", 2), field("latest_event_at", "เวลาเหตุการณ์ล่าสุด", "เหตุการณ์", "datetime", "r.latest_event_at", "events", 2),
	}, Source: `(WITH latest_event AS (SELECT DISTINCT ON (transfer_id) transfer_id, status AS latest_event_status, event_at AS latest_event_at FROM transfer_events ORDER BY transfer_id, event_at DESC) SELECT t.id::text AS transfer_id, t.transfer_code, t.status, sb.name AS source_branch_name, db.name AS destination_branch_name, t.requested_at, t.dispatched_at, t.received_at, p.sku, p.name AS product_name, ti.quantity::bigint AS quantity, ti.received_quantity::bigint AS received_quantity, ti.stock_bucket, ti.discrepancy_note, t.request_note, t.pickup_name, t.courier_name, le.latest_event_status, le.latest_event_at FROM transfers t INNER JOIN transfer_items ti ON ti.transfer_id = t.id INNER JOIN branches sb ON sb.id = t.source_branch_id INNER JOIN branches db ON db.id = t.destination_branch_id INNER JOIN products p ON p.id = ti.product_id LEFT JOIN latest_event le ON le.transfer_id = t.id) r`}
}

func transferEventsDataset() Dataset {
	return Dataset{Key: "transfer_events", Label: "เหตุการณ์การโอน", Description: "log สถานะการโอน พร้อมสรุปสินค้าในใบโอนโดยไม่ขยายจำนวนแถว", Grain: "หนึ่งแถวต่อเหตุการณ์การโอน", DefaultColumns: []string{"event_at", "transfer_code", "source_branch_name", "destination_branch_name", "event_status", "transfer_status", "product_names", "total_quantity", "actor_name", "note"}, DefaultTimeField: "event_at", Relations: []Relation{relation("transfer", "ใบโอน", "ใบโอนของเหตุการณ์", "many-to-one", 1), relation("branches", "สาขา", "ต้นทางและปลายทาง", "many-to-one", 2), relation("items", "สินค้าในใบโอน", "สรุปรายการสินค้าก่อนเชื่อม", "one-to-many-aggregated", 2), relation("actor", "ผู้ดำเนินการ", "ผู้สร้างเหตุการณ์", "many-to-one", 1)}, Fields: []Field{
		field("event_id", "รหัสเหตุการณ์", "เหตุการณ์", "uuid", "r.event_id", "", 0), field("event_at", "เวลาเหตุการณ์", "เหตุการณ์", "datetime", "r.event_at", "", 0), field("event_status", "สถานะเหตุการณ์", "เหตุการณ์", "text", "r.event_status", "", 0), field("note", "หมายเหตุ", "เหตุการณ์", "text", "r.note", "", 0), field("transfer_code", "เลขที่โอน", "ใบโอน", "text", "r.transfer_code", "transfer", 1), field("transfer_status", "สถานะใบโอน", "ใบโอน", "text", "r.transfer_status", "transfer", 1), field("requested_at", "เวลาขอโอน", "ใบโอน", "datetime", "r.requested_at", "transfer", 1), field("source_branch_name", "สาขาต้นทาง", "สาขา", "text", "r.source_branch_name", "branches", 2), field("destination_branch_name", "สาขาปลายทาง", "สาขา", "text", "r.destination_branch_name", "branches", 2), field("product_names", "สินค้าในใบโอน", "สินค้า", "text", "r.product_names", "items", 2), field("item_count", "จำนวนรายการสินค้า", "สินค้า", "integer", "r.item_count", "items", 2), field("total_quantity", "จำนวนรวม", "สินค้า", "integer", "r.total_quantity", "items", 2), field("actor_name", "ผู้ดำเนินการ", "ผู้ใช้", "text", "r.actor_name", "actor", 1),
	}, Source: `(WITH item_summary AS (SELECT ti.transfer_id, STRING_AGG(DISTINCT p.name, ', ' ORDER BY p.name) AS product_names, COUNT(*)::bigint AS item_count, SUM(ti.quantity)::bigint AS total_quantity FROM transfer_items ti INNER JOIN products p ON p.id = ti.product_id GROUP BY ti.transfer_id) SELECT te.id::text AS event_id, te.event_at, te.status AS event_status, te.note, t.transfer_code, t.status AS transfer_status, t.requested_at, sb.name AS source_branch_name, db.name AS destination_branch_name, COALESCE(items.product_names, '') AS product_names, COALESCE(items.item_count, 0)::bigint AS item_count, COALESCE(items.total_quantity, 0)::bigint AS total_quantity, COALESCE(u.full_name, 'ระบบ') AS actor_name FROM transfer_events te INNER JOIN transfers t ON t.id = te.transfer_id INNER JOIN branches sb ON sb.id = t.source_branch_id INNER JOIN branches db ON db.id = t.destination_branch_id LEFT JOIN item_summary items ON items.transfer_id = t.id LEFT JOIN users u ON u.id = te.actor_id) r`}
}

func transferRequestsDataset() Dataset {
	return Dataset{Key: "transfer_requests", Label: "คำขอโอนสินค้า", Description: "คำขอสินค้าจากสาขาปลายทางและผลการอนุมัติ", Grain: "หนึ่งแถวต่อคำขอ", DefaultColumns: []string{"created_at", "destination_branch_name", "source_branch_name", "product_name", "requested_quantity", "status", "approved_stock_bucket", "reviewed_at"}, DefaultTimeField: "created_at", Relations: []Relation{relation("destination_branch", "สาขาปลายทาง", "สาขาที่ขอสินค้า", "many-to-one", 1), relation("source_branch", "สาขาต้นทาง", "สาขาที่อนุมัติให้ส่ง", "many-to-one", 1), relation("product", "สินค้า", "สินค้าที่ร้องขอ", "many-to-one", 1), relation("actors", "ผู้ดำเนินการ", "ผู้ขอและผู้ตรวจ", "many-to-one", 1)}, Fields: []Field{
		field("request_id", "รหัสคำขอ", "คำขอ", "uuid", "r.request_id", "", 0), field("created_at", "เวลาสร้าง", "คำขอ", "datetime", "r.created_at", "", 0), field("updated_at", "เวลาแก้ไข", "คำขอ", "datetime", "r.updated_at", "", 0), field("destination_branch_name", "สาขาปลายทาง", "สาขา", "text", "r.destination_branch_name", "destination_branch", 1), field("source_branch_name", "สาขาต้นทาง", "สาขา", "text", "r.source_branch_name", "source_branch", 1), field("sku", "SKU", "สินค้า", "text", "r.sku", "product", 1), field("product_name", "ชื่อสินค้า", "สินค้า", "text", "r.product_name", "product", 1), field("requested_quantity", "จำนวนที่ขอ", "คำขอ", "integer", "r.requested_quantity", "", 0), field("status", "สถานะ", "คำขอ", "text", "r.status", "", 0), field("approved_stock_bucket", "สต๊อกที่อนุมัติ", "คำขอ", "text", "r.approved_stock_bucket", "", 0), field("requested_by_name", "ผู้ขอ", "ผู้ใช้", "text", "r.requested_by_name", "actors", 1), field("reviewed_by_name", "ผู้ตรวจ", "ผู้ใช้", "text", "r.reviewed_by_name", "actors", 1), field("reviewed_at", "เวลาตรวจ", "คำขอ", "datetime", "r.reviewed_at", "", 0), field("transfer_id", "รหัสใบโอน", "อ้างอิง", "uuid", "r.transfer_id", "", 0),
	}, Source: `(SELECT sr.id::text AS request_id, sr.created_at, sr.updated_at, db.name AS destination_branch_name, COALESCE(sb.name, '') AS source_branch_name, p.sku, p.name AS product_name, sr.requested_quantity::bigint AS requested_quantity, sr.status, COALESCE(sr.approved_stock_bucket, '') AS approved_stock_bucket, ru.full_name AS requested_by_name, COALESCE(vu.full_name, '') AS reviewed_by_name, sr.reviewed_at, sr.transfer_id::text AS transfer_id FROM stock_transfer_requests sr INNER JOIN branches db ON db.id = sr.destination_branch_id LEFT JOIN branches sb ON sb.id = sr.source_branch_id INNER JOIN products p ON p.id = sr.product_id INNER JOIN users ru ON ru.id = sr.requested_by LEFT JOIN users vu ON vu.id = sr.reviewed_by) r`}
}

// monthEndRenumberedDataset — the closed period's invoice register with a
// gapless running number.
//
// Month-end closing drops invoices that meet its exclusion rules (included =
// false). That leaves holes in the numbering: BL001, BL002, BL005. Accounting
// needs a contiguous sequence, so this recomputes one over the surviving
// invoices — BL005 becomes #3 — while keeping invoice_number_snapshot so the
// row can still be traced back to the original bill.
//
// Grain is one row per INVOICE, not per line: the workpaper stores a row per
// invoice item, and numbering per item would inflate the sequence past the
// number of actual bills. Hence DISTINCT + the per-invoice aggregates below.
//
// The sequence restarts per workpaper (one accounting period), which is the
// unit that gets submitted.
func monthEndRenumberedDataset() Dataset {
	return Dataset{
		Key:              "month_end_renumbered",
		Label:            "รายงานสรุปสิ้นเดือน (เรียงเลขใหม่)",
		Description:      "บิลที่เหลือหลังตัดรายการตามเงื่อนไขสรุปสิ้นเดือน พร้อมเลขลำดับใหม่แบบต่อเนื่อง",
		Grain:            "หนึ่งแถวต่อบิลที่อยู่ในรอบบัญชี",
		DefaultColumns:   []string{"accounting_sequence", "invoice_number_snapshot", "branch_name", "issued_at", "line_count", "quantity", "original_total", "scenario_total"},
		DefaultTimeField: "issued_at",
		Relations:        []Relation{relation("branch", "สาขา", "สาขาที่ออกบิล", "many-to-one", 1)},
		Fields: []Field{
			field("accounting_sequence", "ลำดับใหม่", "ลำดับ", "integer", "r.accounting_sequence", "", 0),
			field("management_sequence", "ลำดับเดิม", "ลำดับ", "integer", "r.management_sequence", "", 0),
			field("invoice_number_snapshot", "เลขที่บิลเดิม", "บิล", "text", "r.invoice_number_snapshot", "", 0),
			field("invoice_id", "รหัสบิล", "บิล", "uuid", "r.invoice_id", "", 0),
			field("workpaper_number", "เลขรอบ", "รอบบัญชี", "text", "r.workpaper_number", "", 0),
			field("period_start", "เริ่มรอบ", "รอบบัญชี", "date", "r.period_start", "", 0),
			field("period_end", "สิ้นสุดรอบ", "รอบบัญชี", "date", "r.period_end", "", 0),
			field("workpaper_status", "สถานะรอบ", "รอบบัญชี", "text", "r.workpaper_status", "", 0),
			field("branch_name", "สาขา", "สาขา", "text", "r.branch_name", "branch", 1),
			field("issued_at", "วันที่ออกบิล", "บิล", "datetime", "r.issued_at", "", 0),
			field("payment_type", "วิธีชำระ", "บิล", "text", "r.payment_type", "", 0),
			field("tax_invoice_type", "ชนิดใบกำกับภาษี", "บิล", "text", "r.tax_invoice_type", "", 0),
			field("line_count", "จำนวนรายการ", "ยอด", "integer", "r.line_count", "", 0),
			field("quantity", "จำนวนชิ้น", "ยอด", "integer", "r.quantity", "", 0),
			field("original_total", "ยอดเดิม", "ยอด", "number", "r.original_total", "", 0),
			field("scenario_total", "ยอดหลังปรับ", "ยอด", "number", "r.scenario_total", "", 0),
			field("cash_amount", "เงินสด", "ยอด", "number", "r.cash_amount", "", 0),
			field("transfer_amount", "เงินโอน", "ยอด", "number", "r.transfer_amount", "", 0),
		},
		// ROW_NUMBER runs over the per-invoice roll-up, ordered the way the
		// workpaper itself orders bills, so the new sequence follows the same
		// order the accountant reviewed.
		Source: `(SELECT ROW_NUMBER() OVER (PARTITION BY inv.workpaper_id ORDER BY inv.management_sequence, inv.issued_at, inv.invoice_number_snapshot)::bigint AS accounting_sequence,
		                 inv.*
		          FROM (SELECT ml.workpaper_id::text AS workpaper_id_text,
		                       mw.workpaper_number, mw.period_start, mw.period_end, mw.status AS workpaper_status,
		                       ml.workpaper_id,
		                       ml.invoice_id::text AS invoice_id,
		                       MIN(ml.management_sequence)::bigint AS management_sequence,
		                       ml.invoice_number_snapshot,
		                       ml.branch_name_snapshot AS branch_name,
		                       MIN(ml.issued_at_snapshot) AS issued_at,
		                       MIN(ml.payment_type_snapshot) AS payment_type,
		                       MIN(ml.tax_invoice_type_snapshot) AS tax_invoice_type,
		                       COUNT(*)::bigint AS line_count,
		                       SUM(ml.quantity)::bigint AS quantity,
		                       SUM(ml.original_line_total) AS original_total,
		                       SUM(ml.scenario_line_total) AS scenario_total,
		                       SUM(ml.cash_payment_amount_snapshot) AS cash_amount,
		                       SUM(ml.bank_transfer_payment_amount_snapshot) AS transfer_amount
		                FROM month_end_workpaper_lines ml
		                INNER JOIN month_end_workpapers mw ON mw.id = ml.workpaper_id
		                WHERE ml.included = TRUE
		                GROUP BY ml.workpaper_id, mw.workpaper_number, mw.period_start, mw.period_end, mw.status,
		                         ml.invoice_id, ml.invoice_number_snapshot, ml.branch_name_snapshot) inv) r`,
	}
}

func monthEndDataset() Dataset {
	return Dataset{Key: "month_end", Label: "รอบสรุปสิ้นเดือน", Description: "สถานะและตัวเลขสรุปของกระดาษทำการสิ้นเดือน", Grain: "หนึ่งแถวต่อรอบบัญชี", DefaultColumns: []string{"period_start", "period_end", "workpaper_number", "branch_name", "status", "actual_revenue", "scenario_revenue", "approved_accounting_revenue", "gp_before", "gp_after", "updated_at"}, DefaultTimeField: "updated_at", Relations: []Relation{relation("branch", "สาขา", "สาขาของรอบบัญชี", "many-to-one", 1), relation("actor", "ผู้สร้าง", "ผู้สร้างรอบบัญชี", "many-to-one", 1)}, Fields: []Field{
		field("workpaper_id", "รหัสรอบ", "รอบบัญชี", "uuid", "r.workpaper_id", "", 0), field("workpaper_number", "เลขรอบ", "รอบบัญชี", "text", "r.workpaper_number", "", 0), field("branch_name", "สาขา", "สาขา", "text", "r.branch_name", "branch", 1), field("period_start", "เริ่มรอบ", "ช่วงเวลา", "date", "r.period_start", "", 0), field("period_end", "สิ้นสุดรอบ", "ช่วงเวลา", "date", "r.period_end", "", 0), field("status", "สถานะ", "รอบบัญชี", "text", "r.status", "", 0), field("current_step", "ขั้นตอน", "รอบบัญชี", "integer", "r.current_step", "", 0), field("revision", "revision", "รอบบัญชี", "integer", "r.revision", "", 0), field("actual_invoice_count", "จำนวนใบขาย", "ผลคำนวณ", "integer", "r.actual_invoice_count", "", 0), field("actual_revenue", "รายได้จริง", "ผลคำนวณ", "number", "r.actual_revenue", "", 0), field("target_revenue", "รายได้เป้าหมาย", "ผลคำนวณ", "number", "r.target_revenue", "", 0), field("scenario_revenue", "รายได้จำลอง", "ผลคำนวณ", "number", "r.scenario_revenue", "", 0), field("approved_accounting_revenue", "รายได้ที่อนุมัติ", "ผลคำนวณ", "number", "r.approved_accounting_revenue", "", 0), field("gp_before", "กำไรก่อนปรับ", "ผลคำนวณ", "number", "r.gp_before", "", 0), field("gp_after", "กำไรหลังปรับ", "ผลคำนวณ", "number", "r.gp_after", "", 0), field("unresolved_difference", "ส่วนต่างคงเหลือ", "ผลคำนวณ", "number", "r.unresolved_difference", "", 0), field("created_by_name", "ผู้สร้าง", "ผู้ใช้", "text", "r.created_by_name", "actor", 1), field("created_at", "เวลาสร้าง", "เวลา", "datetime", "r.created_at", "", 0), field("updated_at", "เวลาแก้ไข", "เวลา", "datetime", "r.updated_at", "", 0), field("closed_at", "เวลาปิดรอบ", "เวลา", "datetime", "r.closed_at", "", 0),
	}, Source: `(SELECT mw.id::text AS workpaper_id, mw.workpaper_number, COALESCE(b.name, 'ทุกสาขา') AS branch_name, mw.period_start, mw.period_end, mw.status, mw.current_step::bigint AS current_step, mw.revision::bigint AS revision, mw.actual_invoice_count::bigint AS actual_invoice_count, mw.actual_revenue, mw.target_revenue, mw.scenario_revenue, mw.approved_accounting_revenue, mw.gp_before, mw.gp_after, mw.unresolved_difference, COALESCE(u.full_name, '') AS created_by_name, mw.created_at, mw.updated_at, mw.closed_at FROM month_end_workpapers mw LEFT JOIN branches b ON b.id = mw.branch_id LEFT JOIN users u ON u.id = mw.created_by) r`}
}

func marketplaceDataset() Dataset {
	return Dataset{Key: "marketplace_orders", Label: "คำสั่งซื้อ Marketplace", Description: "คำสั่งซื้อจาก marketplace โดยไม่เปิด raw payload หรือ credentials", Grain: "หนึ่งแถวต่อสินค้าในคำสั่งซื้อ", DefaultColumns: []string{"placed_at", "provider_name", "branch_name", "external_order_id", "status", "customer_name", "product_name", "quantity", "unit_price", "order_total"}, DefaultTimeField: "placed_at", Relations: []Relation{relation("provider", "ผู้ให้บริการ", "Marketplace provider", "many-to-one", 1), relation("branch", "สาขา", "สาขาที่รับคำสั่งซื้อ", "many-to-one", 1), relation("product", "สินค้า", "สินค้าในระบบที่เชื่อมไว้", "many-to-one", 1)}, Fields: []Field{
		field("order_id", "รหัสคำสั่งซื้อ", "คำสั่งซื้อ", "uuid", "r.order_id", "", 0), field("external_order_id", "เลขคำสั่งซื้อภายนอก", "คำสั่งซื้อ", "text", "r.external_order_id", "", 0), field("provider_name", "Marketplace", "ผู้ให้บริการ", "text", "r.provider_name", "provider", 1), field("branch_name", "สาขา", "สาขา", "text", "r.branch_name", "branch", 1), field("status", "สถานะ", "คำสั่งซื้อ", "text", "r.status", "", 0), field("customer_name", "ลูกค้า", "คำสั่งซื้อ", "text", "r.customer_name", "", 0), field("order_total", "ยอดรวมคำสั่งซื้อ", "คำสั่งซื้อ", "number", "r.order_total", "", 0), field("placed_at", "เวลาสั่งซื้อ", "เวลา", "datetime", "r.placed_at", "", 0), field("sku", "SKU", "สินค้า", "text", "r.sku", "product", 1), field("product_name", "ชื่อสินค้า", "สินค้า", "text", "r.product_name", "product", 1), field("quantity", "จำนวน", "รายการ", "integer", "r.quantity", "", 0), field("unit_price", "ราคาต่อหน่วย", "รายการ", "number", "r.unit_price", "", 0),
	}, Source: `(SELECT mo.id::text AS order_id, mo.external_order_id, mp.name AS provider_name, b.name AS branch_name, mo.status, mo.customer_name, mo.order_total, mo.placed_at, moi.sku, moi.product_name, moi.quantity::bigint AS quantity, moi.unit_price FROM marketplace_orders mo INNER JOIN marketplace_order_items moi ON moi.marketplace_order_id = mo.id INNER JOIN marketplace_providers mp ON mp.id = mo.provider_id INNER JOIN branches b ON b.id = mo.branch_id) r`}
}

func usersDataset() Dataset {
	return Dataset{Key: "users", Label: "ผู้ใช้งาน", Description: "ข้อมูลผู้ใช้ที่ปลอดภัย ไม่รวม password หรือ token", Grain: "หนึ่งแถวต่อผู้ใช้", DefaultColumns: []string{"full_name", "email", "role_name", "branch_name", "active", "last_login_at", "updated_at"}, DefaultTimeField: "last_login_at", Relations: []Relation{relation("role", "บทบาท", "บทบาทของผู้ใช้", "many-to-one", 1), relation("branch", "สาขา", "สาขาที่ผู้ใช้สังกัด", "many-to-one", 1)}, Fields: []Field{
		field("user_id", "รหัสผู้ใช้", "ผู้ใช้", "uuid", "r.user_id", "", 0), field("full_name", "ชื่อ", "ผู้ใช้", "text", "r.full_name", "", 0), field("email", "อีเมล", "ผู้ใช้", "text", "r.email", "", 0), field("active", "เปิดใช้งาน", "ผู้ใช้", "boolean", "r.active", "", 0), field("role_key", "รหัสบทบาท", "บทบาท", "text", "r.role_key", "role", 1), field("role_name", "บทบาท", "บทบาท", "text", "r.role_name", "role", 1), field("branch_code", "รหัสสาขา", "สาขา", "text", "r.branch_code", "branch", 1), field("branch_name", "ชื่อสาขา", "สาขา", "text", "r.branch_name", "branch", 1), field("last_login_at", "เข้าสู่ระบบล่าสุด", "เวลา", "datetime", "r.last_login_at", "", 0), field("created_at", "เวลาสร้าง", "เวลา", "datetime", "r.created_at", "", 0), field("updated_at", "เวลาแก้ไข", "เวลา", "datetime", "r.updated_at", "", 0),
	}, Source: `(SELECT u.id::text AS user_id, u.full_name, u.email, u.active, r.role_key, r.name AS role_name, COALESCE(b.code, '') AS branch_code, COALESCE(b.name, '') AS branch_name, u.last_login_at, u.created_at, u.updated_at FROM users u INNER JOIN roles r ON r.id = u.role_id LEFT JOIN branches b ON b.id = u.branch_id) r`}
}

func auditDataset() Dataset {
	return Dataset{Key: "audit_logs", Label: "ประวัติการทำงาน", Description: "เหตุการณ์ audit ที่ปลอดภัย ไม่รวม payload, IP, user agent หรือ request token", Grain: "หนึ่งแถวต่อเหตุการณ์", DefaultColumns: []string{"created_at", "actor_name", "branch_name", "entity_type", "entity_id", "action"}, DefaultTimeField: "created_at", Relations: []Relation{relation("actor", "ผู้ดำเนินการ", "ผู้ใช้ที่ทำรายการ", "many-to-one", 1), relation("branch", "สาขา", "สาขาที่เกี่ยวข้อง", "many-to-one", 1)}, Fields: []Field{
		field("audit_id", "รหัสเหตุการณ์", "เหตุการณ์", "uuid", "r.audit_id", "", 0), field("created_at", "เวลา", "เหตุการณ์", "datetime", "r.created_at", "", 0), field("entity_type", "ประเภทข้อมูล", "เหตุการณ์", "text", "r.entity_type", "", 0), field("entity_id", "รหัสข้อมูล", "เหตุการณ์", "uuid", "r.entity_id", "", 0), field("action", "การกระทำ", "เหตุการณ์", "text", "r.action", "", 0), field("actor_name", "ผู้ดำเนินการ", "ผู้ใช้", "text", "r.actor_name", "actor", 1), field("branch_name", "สาขา", "สาขา", "text", "r.branch_name", "branch", 1),
	}, Source: `(SELECT a.id::text AS audit_id, a.created_at, a.entity_type, a.entity_id::text AS entity_id, a.action, COALESCE(u.full_name, 'ระบบ') AS actor_name, COALESCE(b.name, '') AS branch_name FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id LEFT JOIN branches b ON b.id = a.branch_id) r`}
}

func defaultDefinition(datasetKey string) Definition {
	dataset, ok := datasetByKey(datasetKey)
	if !ok {
		dataset = datasets()[0]
	}
	columns := make([]ColumnDefinition, 0, len(dataset.DefaultColumns))
	for _, key := range dataset.DefaultColumns {
		columns = append(columns, ColumnDefinition{Field: key})
	}
	return Definition{Version: DefinitionVersion, DatasetKey: dataset.Key, Columns: columns, Filters: FilterGroup{Logic: "and", Rules: []FilterRule{}, Groups: []FilterGroup{}}, Sorts: []SortDefinition{}, TimeConfig: &TimeConfig{Field: dataset.DefaultTimeField, RangeType: "relative", Relative: "last_30_days", Bucket: "auto"}, Layout: &LayoutConfig{FieldSidebarWidth: 288}, PageSize: 100}
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
