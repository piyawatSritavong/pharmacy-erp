# ระบบสรุปสิ้นเดือน

อัปเดตล่าสุด: 2026-08-18

## สิทธิ์และขอบเขต

- หน้า `/month-end`, `/month-end-report` และ API ทุกตัวในกลุ่ม Month-End ใช้ได้เฉพาะ literal role `super_admin`
- middleware และ service layer ตรวจ role ซ้ำกัน ผู้ใช้ Central Admin, Branch Admin และ POS ได้ `403` และไม่เห็นเมนู
- เลือก `date_from`, `date_to` ตามเขตเวลา Asia/Bangkok และเลือกได้เฉพาะสาขาขาย; โกดัง `WH` ไม่เป็นสาขาต้นทาง
- ตาราง `month_end_reconciliation_scopes` ป้องกันช่วงวันที่ทับซ้อนกันต่อสาขา

## เกณฑ์บิลและผลของรอบใหม่

บิลที่จะซ่อนต้องตรงทุกข้อ:

1. `invoice_status = issued`
2. `payment_status = paid`
3. `payment_method = cash` จากรายละเอียด `invoice_payments`
4. ไม่ขอใบกำกับภาษีเต็มรูป
5. ยังไม่ถูก soft-delete
6. `created_at` อยู่ในช่วงวันที่ที่เลือก

ระบบซ่อนบิลที่เข้าเงื่อนไขทั้งหมด ยอดเป้าหมายและเปอร์เซ็นต์ยังอยู่ใน payload/UI เพื่อ compatibility กับ client เดิม แต่ถูก disabled และไม่มีผลต่อรอบใหม่ ไม่มีการปรับราคาใน flow ใหม่นี้ ส่วนรายงานยังอ่าน `price_adjusted` ของรอบเก่าได้

สำหรับสินค้าแต่ละบรรทัดของบิลที่ซ่อน transaction จะทำตามลำดับ:

1. คืน Real allocation เดิมที่สาขาแบบ internal
2. สร้าง completed transfer จากสาขาไป `WH` ด้วย reason `PRODUCT_RETURN_TO_WAREHOUSE`
3. ลด Real ที่สาขาและรับ Real lot ใหม่เข้า `WH` ในจำนวนเท่ากัน
4. ลด Ghost lot ที่ `WH` แบบ FEFO
5. หาก Ghost lot ไม่พอ ให้ยอดรวม Ghost ติดลบและบันทึกส่วนขาดใน immutable `inventory_ghost_deficits`
6. soft-delete invoice และเก็บ `original_invoice_number` ครั้งเดียว
7. เรียงเลข Active invoice ในช่วง แยกสาขา ตาม `created_at, id` เริ่ม suffix `00001` โดยไม่แก้ `created_at` หรือ `updated_at` ระหว่างการ renumber

แหล่งตัด effective ของบิลที่ซ่อนคือ `Ghost Stock (สต๊อกผี)` เพียงค่าเดียว การย้อน Real, ส่งคืนจากสาขา และรับเข้า `WH` เป็น movement/adjustment แยก

## โครงสร้างข้อมูลสำคัญ

- `invoices.payment_method`: tender class ที่ trigger sync จาก `invoice_payments`
- `month_end_reconciliations`: header, ช่วงวันที่, branch scope, source hash และผลรวม
- `month_end_reconciliation_scopes`: ขอบเขตต่อสาขาพร้อม exclusion constraint กันช่วงทับซ้อน
- `reconciliation_invoice_snapshots`: invoice Before state และ `invoice_created_at`
- `reconciliation_item_snapshots`: item Before state และ `effective_stock_bucket`
- `reconciliation_logs`: invoice hide/renumber และ movement roles ของ Real/Ghost/deficit
- `stock_adjustment_notes`: signed quantity, stock type, invoice/reconciliation/movement reference และ reason
- `inventory_ghost_deficits`: immutable ledger ของ Ghost ที่ตัดเกิน lot
- `transfers` และ `transfer_item_lot_allocations`: หลักฐาน Real return จากสาขาไป `WH`

Migration `044_global_warehouse_ghost_reconciliation.sql` ล้างประวัติ Ghost ที่สาขาอื่นแบบ atomic ตาม product decision เอกสารผสมถูกลบทั้งฉบับ ส่วน Real lot/movement ที่ยังต้องอยู่จะถูก rebase เป็น opening ledger ก่อนลบต้นทาง ถ้า topology, relation หรือ lot aggregate ไม่สมดุล migration จะ rollback ทั้งไฟล์

Migration `045_ghost_po_month_end_only.sql` เก็บ Ghost history เดิมทั้งหมด แต่ป้องกันรายการใหม่จาก manual adjust/receive/rebalance/approval และเอกสารขาย เสนอราคา โอน หรือคืน ปริมาณ Ghost หลัง cut-over เปลี่ยนได้เฉพาะใบสั่งซื้อเข้าและ Month-End reconciliation

## API

- `POST /api/v1/accounting/month-end/reconciliation-overview`
- `POST /api/v1/accounting/month-end/reconciliation-preview`
- `POST /api/v1/accounting/month-end/reconciliations`
- `GET /api/v1/accounting/month-end/reconciliations`
- `GET /api/v1/accounting/month-end/reconciliations/:reconciliationID`
- `GET /api/admin/month-end-report`
- versioned alias: `GET /api/v1/admin/month-end-report`
- `GET /api/v1/inventory/stock-adjustment-notes`
- `GET /api/v1/inventory/movements`

ตัวอย่าง finalize:

```json
{
  "date_from": "2026-08-01",
  "date_to": "2026-08-31",
  "branch_ids": ["branch-uuid"],
  "target_revenue": 0,
  "adjustment_percent": 0
}
```

`period=YYYY-MM` ยังรองรับเป็น fallback สำหรับ client เดิม รายงานควรใช้ `reconciliation_id` เป็นตัวเลือกหลัก และรองรับ `date_from`, `date_to`, `branch_id`, `page`, `page_size`

## การมองเห็นประวัติ

- Superadmin เห็น timestamp เต็ม, บิลที่ซ่อน, internal reversal, Ghost movement และ deficit
- Central/Branch Admin เห็นเฉพาะ Real adjustment/movement ตาม scope; movement ของบิลที่ซ่อนและ internal Month-End ถูกกรองออก
- POS ถูกปฏิเสธ history API
- UI ของผู้ใช้ที่ไม่ใช่ Superadmin แสดงเฉพาะวันที่ แม้ API จะคง timestamp เต็ม

## Seed สำหรับทดสอบ

```bash
cd backend
APP_ENV=development \
DATABASE_URL='postgres://pharmacy:pharmacy@localhost:5432/pharmacy_erp?sslmode=disable' \
go run ./cmd/api seed-inventory-floor
```

คำสั่ง idempotent นี้ทำให้ทุก active product มี Real อย่างน้อย 10 ชิ้นในทุก active branch รวม `WH` และมี Ghost อย่างน้อย 10 ชิ้นเฉพาะ `WH` พร้อม supplier, posted PO, PO items, lots, movements และ movement-lot links ที่สัมพันธ์กัน

SQL ตรวจ topology และ balance:

```sql
SELECT COUNT(*)
FROM inventory i JOIN branches b ON b.id=i.branch_id
WHERE b.branch_type <> 'main_warehouse' AND i.qty_ghost <> 0;

SELECT COUNT(*)
FROM inventory_lots l JOIN branches b ON b.id=l.branch_id
WHERE b.branch_type <> 'main_warehouse' AND l.stock_bucket='ghost';

SELECT COUNT(*)
FROM inventory i
WHERE i.qty_real <> COALESCE((
  SELECT SUM(l.remaining_quantity)::integer FROM inventory_lots l
  WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='real'
),0)
OR i.qty_ghost <> COALESCE((
  SELECT SUM(l.remaining_quantity)::integer FROM inventory_lots l
  WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='ghost'
),0) - COALESCE((
  SELECT SUM(d.quantity)::integer FROM inventory_ghost_deficits d
  WHERE d.branch_id=i.branch_id AND d.product_id=i.product_id
),0);
```

ผลลัพธ์ที่ถูกต้องของทั้งสาม query คือ `0`
