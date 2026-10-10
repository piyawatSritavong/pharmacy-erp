# Version 1 — ทะเบียนความสามารถปัจจุบัน

ฐานอ้างอิง: `43e5ad4b8f47c2d2572d6b2c5ccf67b8cf7770cb` · ตรวจเมื่อ 2026-09-16 · [ดัชนี](README.md)

## วิธีอ่านสถานะ

- **มี**: พบเส้นทางใช้งานและ implementation ในรุ่นนี้ ไม่ได้แปลว่าผ่าน UAT ทุกกรณีแล้ว
- **บางส่วน**: มีแกนงาน แต่ยังขาดส่วนสำคัญของเป้าหมาย
- **ปิดหน้าจอ**: มีโค้ด/API แต่หน้าจอปัจจุบันเป็น Pro preview
- **โครงสร้าง**: มีการตั้งค่าหรือ schema แต่ยังไม่มีวงจรทำงานจริงครบ
- **ไม่มี / ถอดออก**: ไม่พบ implementation ปัจจุบัน หรือมี migration ถอดออกอย่างชัดเจน

การตรวจครอบคลุมโครงสร้างไฟล์ที่ติดตามด้วย Git, frontend routes/services/components, backend routing และโมดูลธุรกิจทั้ง 18 โมดูล, migration 001–063, tests, seed/import, deployment และเอกสารธุรกิจ ใช้การอ่านโค้ดส่วนทำงานและค้นข้าม source เพื่อยืนยันขอบเขต ไม่ได้นับรูปสินค้า dependency build output หรือ backup เป็นฟีเจอร์ และไม่ได้ตรวจข้อมูล production ทั้งหมด ดูรายละเอียดใน [SOURCE_MAP.md](SOURCE_MAP.md)

## 1. ระบบกลางและสิทธิ์

| ID | ความสามารถ V1 | สถานะและขอบเขต |
| --- | --- | --- |
| V1-01 | Login/logout, session, รหัสผ่าน hash, JWT ผ่าน HttpOnly cookie และ API proxy | มี; frontend ส่งต่อ token ไป Go API |
| V1-02 | แยกหลังบ้านและ POS; เมนูตาม permission; จำกัดข้อมูลตามสาขา | มี; backend ตรวจ role/permission และ branch scope |
| V1-03 | บทบาท `super_admin`, `central_admin`, `branch_pos` | มี; seed ปัจจุบัน 7 บัญชี ไม่ใช่จำนวนผู้ใช้สูงสุด |
| V1-04 | สาขา รหัส ประเภท ที่อยู่ คลังที่สังกัด เปิดใช้งาน เปิด POS/ออนไลน์ | มี; warehouse และ storefront มีหน้าที่ต่างกัน; seed เปิดออนไลน์เฉพาะ KNP |
| V1-05 | เพิ่ม/แก้ไข/ลบผู้ใช้ reset password และแก้ permission ของบทบาทที่มี | มี; ไม่พบหน้าสร้าง role อิสระครบวงจร; ไม่มี sales rep role |
| V1-06 | เลขที่เอกสารและ prefix รายสาขา พร้อมการล็อกก่อนแก้ | มี; backend สร้างเลขเอกสาร |
| V1-07 | Audit การเปลี่ยนแปลง และ access log ของ request | มี; เก็บผู้กระทำ เวลา และข้อมูลก่อน/หลังตามแต่ละ action; ไม่ใช่ระบบเก็บหลักฐานแบบแก้ไม่ได้ทุกชั้น |
| V1-08 | ตรวจผลกระทบก่อน hard delete และข้อความยืนยัน | มีใน master data/เอกสารที่รองรับ; บางรายการติด snapshot/FK แล้วลบไม่ได้ |

หลักฐาน: [auth](../../backend/internal/modules/auth/module.go), [users](../../backend/internal/modules/users/module.go), [branches](../../backend/internal/modules/branches/module.go), [branch scope](../../backend/internal/platform/branch_scope.go), [audit](../../backend/internal/modules/audit/module.go), [API routes](../../backend/internal/http/server.go)

ข้อจำกัด: สิทธิ์อยู่ใน JWT; ต้องทบทวนการเพิกถอน session เมื่อเปลี่ยนสิทธิ์/ปิดบัญชีก่อนขยายระบบ ไม่พบ RLS policy ใน migration ของ repository; ไม่ได้ตรวจการตั้งค่าบนฐาน production จึงไม่สรุปสถานะ production จากไฟล์เพียงอย่างเดียว

## 2. สินค้า ราคา และโปรโมชั่น

| ID | ความสามารถ V1 | สถานะและขอบเขต |
| --- | --- | --- |
| V1-09 | Catalog กลาง: SKU, barcode, ชื่อ, รายละเอียด, หมวด, หน่วยฐาน, ทุน, ราคาขาย, VAT, active | มี; ค้นหา กรอง แบ่งหน้า สร้าง/แก้/ลบ และตรวจข้อมูลซ้ำ |
| V1-10 | หมวดสินค้า และชื่อสินค้าราชการพร้อมราคา alias | มี API; การนำ alias ไปใช้เอกสารราชการอยู่หลังหน้าจอ Pro |
| V1-11 | ราคาขายเฉพาะสาขา เพดานส่วนลด และจุดเตือนสต๊อก | มี; fallback ไปค่า catalog เมื่อไม่ได้ตั้งเฉพาะสาขา |
| V1-12 | รูปหลักและ gallery รวมรูปเพิ่มเติมจากสาขา | มี; JPEG/PNG/WebP, ตรวจขนาด/ชนิดไฟล์, ใช้ private Supabase Storage และ signed URL; ต้องตั้ง storage ก่อน |
| V1-13 | หลายหน่วยขายต่อสินค้า ตัวคูณหน่วยฐาน ราคาและ barcode รายหน่วย | มีสำหรับการขาย; conversion เป็นจำนวนเต็มบวก หน่วยฐานหนึ่งหน่วย; snapshot หน่วย/จำนวนในเอกสาร |
| V1-14 | ส่วนลดต่อรายการ/ท้ายบิล และ override ราคาตามสิทธิ์/เหตุผล | มี; backend คำนวณและตรวจเพดาน |
| V1-15 | โปรโมชั่นซื้อแถม ลดเปอร์เซ็นต์ ลดจำนวนเงิน ราคาชุด และของแถมท้ายบิล | มี; กำหนดช่วงวัน ขั้นต่ำ priority และจำนวนใช้ต่อบิล; ต้นทุนของแถมถูกบันทึก |
| V1-16 | โปรโมชั่นของสำนักงานใหญ่และของแต่ละสาขา | มี; สาขาจัดการได้เฉพาะของตนเอง เห็นโปรส่วนกลางแต่แก้ไม่ได้; code ซ้ำข้ามสาขาได้ |
| V1-17 | ระบุช่องทางสินค้า `in_store / online / both` | มีข้อมูลและตัวกรอง; ไม่ใช่การเปิดร้านออนไลน์จริง |

หลักฐาน: [products](../../backend/internal/modules/products/module.go), [units](../../backend/internal/modules/products/units.go), [selling tools](../../backend/internal/modules/sales/selling_tools.go), [promotions](../../backend/internal/modules/promotions/module.go), [migration 063](../../backend/migrations/063_branch_owned_promotions.sql)

**ยังไม่ครบ:** ราคาขายส่งตามจำนวน/กลุ่มลูกค้า/สัญญา, หน่วยซื้อและหน่วยโอนที่ใช้ conversion ร่วมกันตลอดสาย, การแตกแพ็กเชิงปฏิบัติการ, นโยบายอนุมัติโปรแยกแต่ละสาขา/ช่องทาง และระบบคูปองสมาชิก การขายเป็นกล่องได้ด้วยหน่วยขายที่มีอยู่ ไม่ได้แปลว่าระบบขายส่งครบแล้ว

## 3. คลัง ล็อต รับเข้า โอน และเคลม

| ID | ความสามารถ V1 | สถานะและขอบเขต |
| --- | --- | --- |
| V1-18 | สต๊อกจริงรายสาขา และ Ghost bucket ที่ WH | มี; POS ขายเฉพาะ Real; ข้อมูล Ghost จำกัด literal `super_admin` |
| V1-19 | ดูยอดคงเหลือ/ขายได้/หมดอายุ ล็อต ต้นทุน วันรับ วันหมดอายุ และ movement history | มี; POS บางหน้ารับเพียงสถานะพร้อมขายแทนจำนวนละเอียด |
| V1-20 | รับเข้า/ปรับยอด Real พร้อมเหตุผลและ lot ledger | มี; การลดทั่วไปใช้ล็อตที่ยังใช้ได้; ไม่ใช่วงจรตรวจนับ/ตัดจำหน่ายหมดอายุครบระบบ |
| V1-21 | บังคับ expiry สำหรับสินค้าที่ตั้ง `tracks_expiry` ตอนรับ PO | มี; lot number สร้างได้เมื่อไม่ระบุ; มี `expiry_warning_days` รายสินค้า |
| V1-22 | แสดงสถานะล็อต normal/expiring/expired และกันขายล็อตหมดอายุ | มี; การเตือนในรายละเอียดเป็นจำนวนวันหนึ่งเกณฑ์ ไม่ใช่การแจ้งล่วงหน้า 3/6/9 เดือนหลายระดับ |
| V1-23 | FEFO | บางส่วน; allocator ใช้กับการโอน ของแถม และการตัดบางประเภท แต่ขายปกติบังคับเลือก lot ID และไม่แบ่งล็อตอัตโนมัติ |
| V1-24 | Supplier กลาง: บริษัท/ภาษี/ที่อยู่/ผู้ติดต่อ/เครดิตกี่วัน | มี; credit days เป็นข้อมูล ไม่ใช่ระบบเจ้าหนี้ |
| V1-25 | PO รับเข้าสาขา สร้างสินค้าใหม่ระหว่างรับเข้า และบันทึกหลายรายการ | มี; การบันทึกเป็น posted และเพิ่มสต๊อก/ล็อตทันที ไม่ใช่ purchase request → PO approval → รับบางส่วนหลายครั้ง |
| V1-26 | PO VAT รวม/แยก/ไม่คิด, ส่วนลดรายการ/ท้ายบิล, ค่าส่ง, วันครบกำหนด, เลขเอกสารคู่ค้า, เงื่อนไขส่ง และพิมพ์ | มี; เก็บ snapshot; แก้ไข/cancel พร้อมเหตุผลและข้อจำกัดของยอดที่ใช้แล้ว |
| V1-27 | POS ขอเบิก → ผู้ดูแลอนุมัติ/ปฏิเสธและเลือกต้นทาง | มี; ผูกคำขอกับเอกสารโอน |
| V1-28 | โอนหลายสินค้า: สร้าง → ส่ง → รับตามจำนวนจริง พร้อมเหตุผลส่วนต่าง | มี; ตัดต้นทางตอนส่ง เพิ่มปลายทางตอนรับ เก็บที่มาล็อตและ event; Real เท่านั้น |
| V1-29 | เคลมลูกค้าจากรายการขายและจ่ายสินค้าทดแทน | มี; ติดตามเคส ส่งต่อ supplier และปิดเคสรับรุ่นเดิม/รุ่นทดแทน/ปฏิเสธ |
| V1-30 | เคลมสินค้าบนชั้นโดยไม่ต้องมีใบขาย และ trace movement/events | มี; Real ตามสิทธิ์ ส่วน Ghost เฉพาะ Superadmin ที่ WH ตาม migration 061 |

หลักฐาน: [inventory](../../backend/internal/modules/inventory/module.go), [stocklot](../../backend/internal/modules/stocklot/stocklot.go), [suppliers](../../backend/internal/modules/purchasing/suppliers.go), [purchase orders](../../backend/internal/modules/purchasing/purchase_orders.go), [requests](../../backend/internal/modules/inventory/transfer_requests.go), [transfers](../../backend/internal/modules/transfers/module.go), [returns](../../backend/internal/modules/returns/module.go), [stock claims](../../backend/internal/modules/returns/stock_claim.go)

**ยังไม่ครบ:** การจองสต๊อก, quarantine/recall, ตรวจนับเป็นรอบ, ตัดจำหน่ายหมดอายุแบบระบุล็อตและมูลค่า, แจ้ง expiry ผ่าน job/LINE/SMS, ขนส่งถึงลูกค้า และ landed cost รายล็อต การเคลมรุ่นทดแทนปัจจุบันสร้างล็อตโดยไม่ได้รับ expiry ใหม่จากผู้ใช้ ต้องเติมก่อนถือว่าทุกช่องทางรับเข้าติดตาม expiry ได้ครบ

## 4. POS และเอกสารขาย

| ID | ความสามารถ V1 | สถานะและขอบเขต |
| --- | --- | --- |
| V1-31 | ค้นหาสินค้า ชื่อ/SKU/barcode ดูรูป เลือกล็อต เพิ่มตะกร้า เปลี่ยนหน่วย/จำนวน/ส่วนลด | มี; พื้นฐานเดียวกันรองรับ desktop/tablet/mobile |
| V1-32 | Preview และ checkout จาก backend พร้อม VAT/เลขบิล/payment/stock/audit | มี; งานขายปกติอยู่ใน transaction และล็อกสต๊อกก่อนตัด |
| V1-33 | เงินสดพร้อมเงินทอน เงินโอนพร้อมอ้างอิง และสด+โอน | มี; payment ที่เก็บจริงเป็น cash/bank_transfer; ยังไม่มี payment gateway หรือยืนยันโอนจากธนาคารอัตโนมัติ |
| V1-34 | ขอใบกำกับเต็มรูปตอนขาย ดู/พิมพ์ใบเสร็จและประวัติ | มี; บันทึกชื่อและเลขภาษีลูกค้าเป็น snapshot ไม่ใช่ทะเบียนสมาชิก |
| V1-35 | เปลี่ยนใบกำกับอย่างย่อเป็นเต็มรูปย้อนหลังในวันเดียวกัน | มี; โค้ดกำหนดก่อน 17:00 น. และก่อน month-end แตะบิล ยกเลิกใบเดิมแล้วผูกใบใหม่ โดยไม่ตัดสต๊อกซ้ำ; เป็นกติกาปัจจุบันของแอป |
| V1-36 | พักบิล/เรียกกลับ ลบ และหมดอายุ 24 ชั่วโมง | มีพื้นฐาน; ไม่จองสต๊อก ไม่ออกเอกสาร; มีช่องว่างการรักษาหน่วยขายและส่วนลด ดูรายการ V2-P0 |
| V1-37 | Admin ขายในนามสาขา และส่งตะกร้า remote ให้ POS รับเงิน | มี; แสดงสถานะเครื่อง POS จาก heartbeat/polling; ไม่ใช่ระบบเซลล์ภาคสนาม |
| V1-38 | ใบเสนอราคา → ใบขาย, ใบขายค้างชำระ → รับเต็มยอด, เอกสาร รพ.สต. | ปิดหน้าจอ; service/API และ components ยังอยู่ แต่ page เป็น Pro preview |
| V1-39 | รับคืนเป็นเงิน/คืนบางส่วน/ใบลดหนี้/เครดิตเทอม/เช็ค | ไม่มีวงจรปัจจุบันครบ; เคลมเปลี่ยนสินค้าไม่ใช่ใบลดหนี้ |

หลักฐาน: [sales](../../backend/internal/modules/sales/module.go), [POS](../../frontend/src/components/sections/pos-workspace.tsx), [remote sessions](../../backend/internal/modules/sales/remote_sessions.go), [parked bills](../../backend/internal/modules/parkedbills/module.go), [full tax reissue](../../backend/internal/modules/sales/full_tax_invoice.go), [Pro pages](../../frontend/src/app/(app)/sales-management/page.tsx)

## 5. รายงานและสรุปสิ้นเดือน

| ID | ความสามารถ V1 | สถานะและขอบเขต |
| --- | --- | --- |
| V1-40 | Dashboard หลายสาขา ยอดขายรายวัน เปรียบเทียบยอด/จำนวนบิล กลุ่มชำระเงิน/ภาษี | มี; กรองช่วงวัน สถานะ และรอบ reconciliation; ดู before/after ตามสิทธิ์ |
| V1-41 | สรุปขาย POS ช่วงวันที่ และ export PDF/XLSX | มี; ขอบเขตตามผู้ใช้/สิทธิ์ของรายงาน ไม่ใช่ dashboard ทุกช่องทาง |
| V1-42 | การแจ้งเตือนหลังบ้าน: low stock, คำขอเบิก, เคลม | มี; นับจากข้อมูลและ polling; ยังไม่มี notification delivery service |
| V1-43 | รายงานภาษีและกำไรขั้นต้นรายสาขา | มีที่ `/global-reports` และ API แต่ไม่อยู่เมนูหลัก; คำนวณขายลบ `cost_snapshot × quantity` ไม่รวมค่าใช้จ่ายทุกประเภท |
| V1-44 | รายงาน อย. เลือกสินค้า/หมวด/สาขา/ช่วงวัน | ปิดหน้าจอ; API summary ยังอยู่; ไม่พบการส่งเข้าระบบหน่วยงานจริง |
| V1-45 | Month-end preview/finalize, snapshot, before/after, trace stock, scope/overlap checks | มีเฉพาะ Superadmin; เป็นกระบวนการเฉพาะของระบบ ไม่ใช่ GL/AR/AP ครบชุด |

หลักฐาน: [dashboard](../../backend/internal/modules/dashboard), [reports](../../backend/internal/modules/reports/module.go), [FDA](../../backend/internal/modules/fda/module.go), [month-end reconciliation](../../backend/internal/modules/monthend/reconciliation.go), [report](../../backend/internal/modules/monthend/report.go), [daily breakdown](../../backend/internal/modules/monthend/daily_breakdown.go)

ข้อสังเกตของ month-end ที่ต้องเก็บเป็นประวัติ V1: โค้ดล่าสุดทำงานระดับ **บรรทัดสินค้า** ของบิลเงินสดที่เข้าเงื่อนไข มีทั้งการซ่อนบรรทัด/บิลและเปลี่ยนราคาบรรทัดที่คงอยู่ พร้อม stock movement และ snapshot ส่วนหน้ารายงานทั่วไปกรองรายการที่ถูกซ่อนออก นี่เป็นคำอธิบายพฤติกรรมที่พบ ไม่ใช่การรับรองรายงานสำหรับใช้นำส่งบัญชี เป้าหมายรุ่นถัดไปต้องเก็บยอดขาย/การรับเงินจริงต้นฉบับและเอกสารปรับปรุงที่ตรวจสอบย้อนกลับได้ก่อนนำไปใช้กับ AR/AP หรือ analytics ใหม่

## 6. ออนไลน์ UX และงานดูแลระบบ

| ID | ความสามารถ V1 | สถานะและขอบเขต |
| --- | --- | --- |
| V1-46 | Provider/connection settings และรายการ marketplace orders ที่บันทึกใน DB | โครงสร้าง; test connection ตรวจว่าข้อมูลครบ ไม่เรียกผู้ให้บริการจริง; หน้า `/marketplace` เป็น Pro preview แต่แท็บตั้งค่ายังมี |
| V1-47 | Web shop, customer checkout/account, OMS, stock sync, BOPIS, แอปลูกค้า | ไม่มี; responsive POS เป็นหน้าพนักงาน ไม่ใช่ E-Commerce |
| V1-48 | CRM/คะแนน/segmentation/refill/campaign attribution | ไม่มี; ชื่อลูกค้าในบิลกับข้อมูล supplier ใช้แทน CRM ไม่ได้ |
| V1-49 | Chat AI, local model, LINE/SMS bot, AI automation | ไม่มี |
| V1-50 | Mobile Admin drawer, POS bottom bar/menu, compact cards, cart/payment dialogs, safe area/keyboard handling, light/dark | มี; รวมใน baseline commit นี้ |
| V1-51 | Docker Compose, Render blueprint, migration แยกจาก serve, health check, timeouts, shutdown drain | มีไฟล์และ implementation; ไม่ใช่การตรวจยืนยันการ deploy production ปัจจุบัน |
| V1-52 | Seed/import Ocha, upload รูป, เติม stock ทดสอบ, reset/replace แบบมี guard | มี CLI; งานผู้ดูแลระบบ ไม่ใช่หน้า import ทั่วไป; ไม่ได้รัน reset/seed ในการสำรวจนี้ |
| V1-53 | Go tests, Playwright และเอกสาร manual/mobile validation | มี; ขอบเขตผลทดสอบดูด้านล่าง; ไม่พบ CI workflow ใน Git |
| V1-54 | การสมัครแพ็กเกจ Pro | โครงหน้าจอ; ปุ่มแสดง toast เท่านั้น ไม่ได้เรียก billing, เก็บ lead หรือเปิด entitlement จริง |

หลักฐาน: [marketplace](../../backend/internal/modules/marketplace/module.go), [Pro button](../../frontend/src/components/sections/pro-subscribe-button.tsx), [mobile validation](../MOBILE_UX_VALIDATION.md), [app/CLI](../../backend/internal/app), [Render](../../render.yaml), [Docker](../../deploy)

## 7. สิ่งที่เอกสารเก่าระบุ แต่ไม่ใช่ความสามารถปัจจุบัน

| เรื่อง | ข้อเท็จจริงที่ใช้เป็น baseline |
| --- | --- |
| เช็ค ผ่อนชำระ รายได้อื่น สมุดงานบัญชีภายนอก | migration 016 ลบตารางและสิทธิ์ที่เกี่ยวข้อง ไม่ควรนับจาก migration 001/003 เพียงอย่างเดียว |
| ราคาปลีก/ผ่อนหลาย tier แบบเก่า | ผ่อนถูกถอดใน 016; retail_price ถูก snapshot แล้วถอดใน 033; ยังมีราคาต่อสาขาและหน่วยขายแบบใหม่ |
| Generate Report / saved reports / pins | migration 054 ลบ report_definitions และ permission แล้ว |
| หลายบทบาท เช่น branch_admin/admin/office | ให้ยึดสามบทบาทปัจจุบันและ migration ที่ตามมา ไม่ยึดชื่อใน test/ข้อความเก่า |
| Ghost เปลี่ยนผ่าน PO และ month-end เท่านั้น | ไม่ครบแล้ว: migration 061 เพิ่ม stock claim/สินค้าทดแทนสำหรับ Superadmin ที่ WH |
| Month-end แบบทั้งบิลเท่านั้น | migration 058/059 และ reconciliation.go มีการทำระดับบรรทัดแล้ว |
| serve รัน migration เอง; รูปอยู่ local volume | serve แยก migration; เส้นทางรูปปัจจุบันใช้ Supabase Storage |
| ข้อเสนอ Ocha ว่ายังไม่มีพักบิล/ส่วนลด/โปร | ล้าสมัย; ปัจจุบันมีแล้วแต่ต้องตรวจความครบตาม baseline |

## 8. ช่องว่างที่ควรจัดการก่อนขยายระบบ

รายการต่อไปนี้ได้จากการอ่านโค้ด เป็นงานยืนยัน/แก้ใน **V2-P0/P1** ไม่ใช่ผลทดสอบ production ที่เกิดเหตุแล้ว:

1. **พักบิลรักษารายละเอียดไม่ครบ:** `parkBill()` ไม่ส่ง `unit_id`, ส่งส่วนลดรายการเป็น 0 และไม่เก็บส่วนลดท้ายบิลใน contract; การเรียกกลับต้องทดสอบหน่วยกล่อง/แผงและส่วนลดจริง
2. **Remote checkout ต้องกันซ้ำ:** อ่าน open session → เรียก Checkout → update session เป็นคนละ transaction; ต้องทดสอบกดพร้อมกัน/เครือข่าย retry และทำ idempotency/atomic transition; remote contract ยังไม่มีหน่วยขายครบ
3. **FEFO ไม่ครอบคลุมทุกการขาย:** lot selector เรียง expiry แต่ผู้ใช้เลือกได้เอง; ยังไม่ใช่ auto-allocation ข้ามล็อต
4. **Expiry lifecycle ยังมีช่องว่าง:** negative adjustment/stock claim ใช้ allocator ที่ไม่รับล็อตหมดอายุ จึงต้องมี write-off/quarantine แยก; สินค้าทดแทนจากเคลมต้องรับ lot/expiry/cost ที่ถูกต้อง
5. **ต้นทุนยังเป็น lot purchase unit cost:** PO เก็บส่วนลด/ค่าส่ง แต่สร้างล็อตด้วย `line.Input.UnitCost`; cost_price กลางใช้ต้นทุนซื้อล่าสุด ยังไม่ใช่ FIFO/Moving Average พร้อม landed cost
6. **API/หน้า Pro/legacy ต้องจัดระเบียบ:** Pro เป็น UI gate ไม่ใช่ entitlement ฝั่ง server; service เก่าบางตัว เช่น receipt requests ยังอ้างตารางที่ถูกถอด แต่ไม่ได้ผูก route ปัจจุบัน
7. **ข้อมูลบัญชีต้องมีฐานชัดเจน:** แยกยอดขาย/เงินรับเดิมกับ adjustment/reconciliation และใช้ฐานเดียวใน report/credit/refund; ไม่ให้ยอดรายงานใหม่คลาดจากเหตุการณ์เงินจริง
8. **การขยายสิทธิ์และ operations:** ทดสอบสิทธิ์ใหม่กับทุก endpoint, session revocation, secret ใน connection/audit, backup/restore, CI และการกู้คืนเมื่อเขียนค้าง

## 9. หลักฐานการตรวจสอบ

- 2026-09-16: `env -u TEST_DATABASE_URL go test ./...` **ผ่าน**; Go รายงานผลจาก cache; integration tests ที่ต้องใช้ `TEST_DATABASE_URL` ไม่ได้รันกับ DB ในรอบนี้
- หลักฐานงานมือถือเดิมวันที่ 2026-09-15: build/lint/typecheck ผ่าน, 10 mobile scenarios และ 5 selected regressions ผ่าน, 638 route/width/browser checks ไม่พบ document overflow ตาม [รายงานที่บันทึกไว้](../MOBILE_UX_VALIDATION.md)
- การทดสอบมือถือ intercept เฉพาะ final checkout จึงไม่ยืนยันการรับเงินจริง/ตัดสต๊อกจริงจากชุดนั้น; รูปสินค้าบน local ยังไม่ได้ตั้ง storage; เป็น browser emulation ไม่ใช่การทดสอบเครื่องจริง
- การมี test file ไม่เท่ากับผ่าน UAT ทั้งโมดูล และยังไม่ถือว่า V1 production-ready โดยอัตโนมัติ
