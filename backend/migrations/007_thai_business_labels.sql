UPDATE permissions AS permission
SET name = translated.name,
    description = translated.description,
    updated_at = NOW()
FROM (VALUES
    ('dashboard.view.global', 'ดูแดชบอร์ดส่วนกลาง', 'ดูภาพรวมทุกสาขา'),
    ('dashboard.view.self', 'ดูสรุปยอดขายของตนเอง', 'ดูยอดขายรายวันของผู้ใช้ปัจจุบัน'),
    ('products.manage', 'จัดการสินค้า', 'เพิ่ม แก้ไข และลบสินค้า'),
    ('products.view', 'ดูสินค้า', 'ดูรายการสินค้า'),
    ('inventory.manage.global', 'จัดการสต๊อกทุกสาขา', 'ปรับสต๊อกของทุกสาขา'),
    ('inventory.view.branch', 'ดูสต๊อกสาขา', 'ดูสต๊อกของสาขาที่กำหนด'),
    ('inventory.rebalance', 'ย้ายประเภทสต๊อก', 'ย้ายระหว่างสต๊อกจริงและสต๊อกผี'),
    ('inventory.receive', 'รับสินค้าเข้า', 'รับสินค้าเข้าสต๊อกจริงและสต๊อกผี'),
    ('installment.view', 'ดูแผนผ่อน', 'ดูแผนและรายการชำระค่างวด'),
    ('installment.manage', 'จัดการแผนผ่อน', 'สร้างแผนผ่อนจากใบขาย'),
    ('installment.collect', 'รับชำระค่างวด', 'บันทึกการชำระค่างวด'),
    ('price.override.global', 'กำหนดราคาพิเศษส่วนกลาง', 'กำหนดราคาพิเศษสำหรับทุกสาขา'),
    ('price.override.pos', 'กำหนดราคาพิเศษหน้าร้าน', 'กำหนดราคาพิเศษจากจุดขาย'),
    ('government.manage_alias', 'จัดการชื่อสินค้าสำหรับราชการ', 'กำหนดชื่อสินค้าที่ใช้ในเอกสารราชการ'),
    ('government.use', 'ใช้โหมดราชการ', 'ขายสินค้าโดยใช้ชื่อสำหรับเอกสารราชการ'),
    ('invoice.sequence.manage', 'จัดการเลขที่เอกสาร', 'กำหนดคำนำหน้าและเลขถัดไป'),
    ('invoice.create.pos', 'สร้างใบขายหน้าร้าน', 'สร้างใบขายจากจุดขาย'),
    ('invoice.view', 'ดูใบขาย', 'ดูใบขายที่มีสิทธิ์เข้าถึง'),
    ('invoice.reprint', 'พิมพ์ใบขายซ้ำ', 'เปิดและพิมพ์ใบขายย้อนหลัง'),
    ('quotation.manage', 'จัดการใบเสนอราคา', 'สร้าง แปลง และลบใบเสนอราคา'),
    ('transfer.approve', 'ดูแลการโอนสินค้า', 'ดูรายการโอนสินค้าทุกสาขา'),
    ('transfer.request', 'ขอโอนสินค้า', 'สร้างรายการโอนสินค้า'),
    ('transfer.dispatch', 'ส่งสินค้าโอน', 'ยืนยันส่งสินค้าจากต้นทาง'),
    ('transfer.receive', 'รับสินค้าโอน', 'ยืนยันรับสินค้าที่ปลายทาง'),
    ('finance.manage.global', 'จัดการการเงินส่วนกลาง', 'จัดการเช็คของทุกสาขา'),
    ('payment.collect', 'รับชำระเงิน', 'รับเงินสดและเงินโอน'),
    ('reports.view.global', 'ดูรายงานส่วนกลาง', 'ดูรายงานภาษีและกำไรขาดทุน'),
    ('settings.manage', 'จัดการตั้งค่า', 'จัดการการตั้งค่าของระบบ'),
    ('users.manage', 'จัดการผู้ใช้', 'เพิ่ม แก้ไข และลบผู้ใช้'),
    ('audit.view.global', 'ดูประวัติการทำงาน', 'ดูประวัติการเปลี่ยนแปลงของระบบ'),
    ('marketplace.manage.global', 'จัดการตลาดออนไลน์', 'จัดการผู้ให้บริการและการเชื่อมต่อตลาดออนไลน์'),
    ('marketplace.view.branch', 'ดูคำสั่งซื้อตลาดออนไลน์', 'ดูคำสั่งซื้อจากตลาดออนไลน์')
) AS translated(permission_key, name, description)
WHERE permission.permission_key = translated.permission_key;

UPDATE app_settings
SET metadata = jsonb_set(metadata, '{label}', to_jsonb('อัตราภาษีมูลค่าเพิ่ม'::text), TRUE),
    updated_at = NOW()
WHERE setting_key = 'vat_rate';

UPDATE app_settings
SET setting_value = CASE WHEN setting_value = 'Pharmacy ERP Demo' THEN 'ระบบบริหารร้านขายยา PharmaPOS' ELSE setting_value END,
    metadata = jsonb_set(metadata, '{label}', to_jsonb('ชื่อกิจการ'::text), TRUE),
    updated_at = NOW()
WHERE setting_key = 'company_name';

UPDATE app_settings
SET metadata = jsonb_set(metadata, '{label}', to_jsonb('เลขประจำตัวผู้เสียภาษี'::text), TRUE),
    updated_at = NOW()
WHERE setting_key = 'company_tax_id';

UPDATE app_settings
SET metadata = jsonb_set(metadata, '{label}', to_jsonb('ที่อยู่กิจการ'::text), TRUE),
    updated_at = NOW()
WHERE setting_key = 'company_address';

UPDATE checks SET bank_name = 'ธนาคารกรุงไทย' WHERE bank_name = 'Krungthai';
UPDATE checks SET bank_name = 'ธนาคารกสิกรไทย' WHERE bank_name = 'Kasikorn';

UPDATE transfers
SET pickup_name = CASE pickup_name
        WHEN 'Somchai Receiver' THEN 'สมชาย ผู้รับสินค้า'
        WHEN 'Niran Receiver' THEN 'นิรันดร์ ผู้รับสินค้า'
        ELSE pickup_name
    END,
    courier_name = CASE WHEN courier_name = 'ERP Courier' THEN 'ขนส่ง ERP' ELSE courier_name END,
    updated_at = NOW()
WHERE pickup_name IN ('Somchai Receiver', 'Niran Receiver') OR courier_name = 'ERP Courier';

UPDATE transfer_events
SET note = CASE note
    WHEN 'Branch requested masks' THEN 'สาขาขอโอนหน้ากากอนามัย'
    WHEN 'Shipment left branch' THEN 'สินค้าออกจากสาขาต้นทางแล้ว'
    WHEN 'Branch requested diapers' THEN 'สาขาขอโอนผ้าอ้อมผู้ใหญ่'
    WHEN 'Shipment left source branch' THEN 'สินค้าออกจากสาขาต้นทางแล้ว'
    WHEN 'Transfer queued for dispatch' THEN 'รายการโอนรอยืนยันส่งสินค้า'
    WHEN 'Transfer dispatched' THEN 'ยืนยันส่งสินค้าแล้ว'
    WHEN 'Transfer received' THEN 'รับโอนสินค้าแล้ว'
    ELSE note
END
WHERE note IN (
    'Branch requested masks',
    'Shipment left branch',
    'Branch requested diapers',
    'Shipment left source branch',
    'Transfer queued for dispatch',
    'Transfer dispatched',
    'Transfer received'
);

UPDATE marketplace_providers
SET description = 'ช่องทางรับคำสั่งซื้อจากตลาดออนไลน์', updated_at = NOW()
WHERE provider_key = 'health-mart' AND description = 'Provider-ready marketplace inbox';

UPDATE marketplace_connections
SET connection_name = 'การเชื่อมต่อสาขา MNS', updated_at = NOW()
WHERE connection_name = 'Branch MNS Connector';

UPDATE marketplace_orders
SET customer_name = CASE WHEN customer_name = 'HealthMart Customer' THEN 'ลูกค้า Health Mart' ELSE customer_name END,
    raw_payload = CASE
        WHEN raw_payload ->> 'notes' = 'awaiting branch confirmation'
            THEN jsonb_set(raw_payload, '{notes}', to_jsonb('รอยืนยันจากสาขา'::text), TRUE)
        ELSE raw_payload
    END
WHERE customer_name = 'HealthMart Customer'
   OR raw_payload ->> 'notes' = 'awaiting branch confirmation';

UPDATE invoice_payments
SET notes = 'ตัดชำระด้วยเช็ค'
WHERE payment_type = 'check' AND notes = 'Matched by check reconciliation';
