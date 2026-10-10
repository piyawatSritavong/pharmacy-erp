# Cart and checkout contract · V2-P0-02

สถานะ: implementation candidate; ต้องยืนยันด้วย DB integration และ E2E ก่อนปิด P0-02

## Canonical request

Header ใช้ `branch_id`, `customer_name`, `customer_tax_id`, `full_tax_invoice`, `is_government_mode`, `notes` และ `bill_discount_amount` รายการใช้ `product_id`, `inventory_lot_id`, `alias_id`, `unit_id`, `quantity`, `stock_bucket`, `discount_amount`, `override_unit_price` และ `override_reason`

`quantity` คือจำนวนในหน่วยขายที่ `unit_id` ระบุ Backend เป็น source of truth ของ unit conversion, active unit, ราคาปัจจุบัน, promotion, tax, lot availability และจำนวนฐานที่จะตัด stock ค่า `unit_id=""` หมายถึงหน่วยฐานเพื่อรองรับ client เก่า

`unit_name`, `conversion_qty`, `sold_quantity`, `unit_price`, product/SKU และ lot label ใน parked/remote cart เป็น display snapshot เท่านั้น Backend ต้องคำนวณใหม่ก่อน commit และห้ามใช้ snapshot เหล่านี้ตัด stock

Payment แยกจาก cart: `payment_type`, `tendered_amount`, `transfer_amount`, `reference_code` ส่วน remote ต้องส่ง `session_id` เป็น idempotency key และ Admin ส่ง `branch_id` ด้วย Backend lock session และทำ invoice, payment, stock movement, audit และ session completion ใน transaction เดียว

## Compatibility and conflicts

- Parked rows ก่อน migration 064 อ่าน `bill_discount_amount=0`; JSON item ที่ไม่มี unit ใช้หน่วยฐาน
- Remote client เก่าที่ไม่ส่ง `session_id` ยัง checkout open session ได้ครั้งแรก แต่ไม่รับ guarantee replay หลัง timeout; client V2 ต้องส่ง session ID
- session เดิม + payment payload เดิมหลัง timeout คืน checkout result เดิม
- session เดิม + payload ต่างตอบ conflict โดยไม่เขียนเพิ่ม
- delayed autosave ที่อ้าง session ซึ่ง completed/cancelled แล้วตอบ conflict และไม่สร้าง open session ใหม่
- ราคา หน่วย lot หรือ stock ที่เปลี่ยนหลัง park/save ต้องถูก revalidate จาก master/lot ปัจจุบันตอน preview/checkout

## Parked lifecycle candidate

Create เก็บ unit/quantity/line discount/bill discount/customer/tax/note; resume ใช้ `claim` พร้อม owner/token ที่ server ตรวจและเก็บใน browser เฉพาะ id/token. Checkout เปลี่ยนสถานะเป็น `consumed` และผูก invoice ใน transaction เดียว; การล้างตะกร้าเปลี่ยนเป็น `abandoned`. Claim แข่งกันได้ผู้ชนะคนเดียว และ claim ค้างเกิน 15 นาทีคืนสิทธิ์ได้ DB integration ยืนยัน round-trip, owner/token และ recovery แล้ว แต่ refresh/crash, ราคาและสต๊อกเปลี่ยน และสอง terminal ใน frontend E2E ยังต้องตรวจ จึงยังไม่ปิด P0-03

`remote_sale_sessions.checkout_result` ใช้ตอบ retry เท่านั้น ไม่ใช่หลักฐานเงินจริงต้นฉบับหรือเอกสาร correction สำหรับ P0-05
