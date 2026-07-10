# รายงานวิเคราะห์การ Merge DockBill เข้า pharmacy-erp-main

> จัดทำ: 2026-07-11 — วิเคราะห์จากโค้ดจริงทั้งสองระบบ (ไม่ใช่จากเอกสาร)

## ✅ สถานะ: ดำเนินการเสร็จสิ้นแล้ว (2026-07-11)

ตามการตัดสินใจ: ไม่ merge โค้ด แต่ port ฟีเจอร์เข้าสถาปัตยกรรมโปรเจคหลัก — **ทำครบทั้ง 3 ฟีเจอร์และลบโฟลเดอร์ DockBill แล้ว**

| ฟีเจอร์ | ผลลัพธ์ |
|---|---|
| บิลผ่อน/งวด | migration `003_installments_price_tiers.sql` + `modules/installments` + หน้า `/installments` (ทั้ง 3 role) — เก็บเงินงวดลง `invoice_payments` เดิม รายงานการเงินเห็นครบ, ครบงวดแล้ว invoice เปลี่ยนเป็น paid อัตโนมัติ |
| ราคา 3 ระดับ | `products.retail_price`/`installment_price` + `price_tier` ต่อบรรทัดขาย (override > gov alias > tier > branch/base) + UI ใน product console / document composer |
| รับสินค้าเข้าแยก real/ghost | `POST /inventory/receive` (tx เดียว: upsert + movements + audit) + โหมด Receive Stock ใน inventory console |
| การทดสอบ | `go vet`/`go test` ผ่าน, `next build` ผ่าน, ทดสอบ E2E 27 ข้อกับ Postgres 16 จริงผ่านทั้งหมด (migration, seed, RBAC, edge cases) |
| DockBill | ลบโฟลเดอร์ออกจากโปรเจคแล้ว (ไม่เคยเข้า git) — โค้ดวิเคราะห์ไว้ในรายงานนี้ |
| การย้ายข้อมูลจาก Google Sheets | **ยังไม่ทำ** — ถ้าลูกค้ารายเดิมของ DockBill จะย้ายเข้าระบบนี้ ใช้ mapping ในหัวข้อ 5 ด้านล่าง |

รายละเอียดการวิเคราะห์เดิมอยู่ด้านล่าง (คงไว้เพื่ออ้างอิง โดยเฉพาะหัวข้อ 5 สำหรับ import ข้อมูลในอนาคต)

---

---

## 1. สถานะ QUICKSTART.md — ตรวจสอบแล้ว: ทุกอย่างมีอยู่แล้วในโค้ดจริง

QUICKSTART.md ใน `pharmacy-erp-dev-skill/` **ล้าสมัยกว่าโค้ดจริง** ทุกขั้นตอนที่สั่งให้ทำ ถูก implement ไปแล้วในรูปแบบที่ดีกว่าไฟล์ตัวอย่าง:

| ขั้นตอนใน QUICKSTART | สถานะจริงใน pharmacy-erp-main |
|---|---|
| Step 1: copy `database-schema.sql` เป็น migration | ✅ มีแล้ว — `backend/migrations/001_init.sql` ใช้ตาราง `inventory` เดียว มีคอลัมน์ `qty_real` / `qty_ghost` + `inventory_movements` (design คนละแบบกับไฟล์ตัวอย่างที่แยก 2 ตาราง) |
| Step 2: copy `inventory-handlers-example.go` | ✅ มีแล้ว — `backend/internal/modules/inventory/module.go` มี `List` / `Rebalance` (โอน real↔ghost) / `Adjust` พร้อม `FOR UPDATE` lock, transaction, movements และ audit log |
| Step 2: register routes | ✅ มีแล้ว — `backend/internal/http/server.go:93-95` (`GET /inventory`, `POST /inventory/rebalance`, `POST /inventory/adjust`) พร้อม permission middleware |
| Step 3: copy `transfer-stock.component.tsx` | ✅ มีแล้ว — `frontend/src/components/sections/inventory-console.tsx` มี UI ทั้ง adjust และ rebalance (Real ↔ Ghost) |
| Step 4-5: test local | ⛔ ยังทำไม่ได้ — ดิสก์เครื่องเหลือ ~288MB (เต็ม 100%) build/รัน Postgres ไม่ได้ ต้องเคลียร์พื้นที่ก่อน |

### ⚠️ ห้าม copy ไฟล์ reference ตาม QUICKSTART ตรง ๆ — จะทำ backend พังทันที

เหตุผล (ตรวจจากโค้ดแล้ว):

1. `backend/migrations/embed.go` ใช้ `//go:embed *.sql` — ไฟล์ `.sql` ทุกไฟล์ที่วางในโฟลเดอร์จะถูกรันตอน start (`serve` รัน migrate ก่อนเสมอ — `cmd/api/main.go:44`)
2. `references/database-schema.sql` เป็น **SQL ที่รันบน PostgreSQL ไม่ผ่าน**: ใช้ syntax `INDEX idx_... (...)` ภายใน `CREATE TABLE` (เป็น syntax ของ MySQL)
3. อ้างตาราง `app_user` ซึ่งไม่มีอยู่ (ของจริงชื่อ `users`)
4. สร้างตาราง `invoices`, `invoice_payments`, `audit_logs` ซ้ำชื่อกับตารางเดิมที่มี definition คนละแบบ (ไม่มี `IF NOT EXISTS`) → migration fail → backend boot ไม่ขึ้น
5. `references/inventory-handlers-example.go` มีบั๊กจริง: `string(rune(quantity))` แปลงตัวเลขผิด และ JSON string ใน audit log ประกอบผิดรูปแบบ — โค้ดจริงในโปรเจคเขียนถูกต้องอยู่แล้ว

**ข้อสรุป:** ไม่ต้องทำอะไรเพิ่มตาม QUICKSTART — โปรเจคหลักครบและดีกว่าแล้ว ควรอัปเดต skill ให้ตรงกับโค้ดจริงแทน

---

## 2. DockBill คืออะไรจริง ๆ (ต่างจากที่ skill อธิบายมาก)

SKILL.md บอกว่า DockBill ใช้ "Drizzle ORM + NextAuth.js" — **ไม่จริง** ของจริงคือ:

- **Next.js 14 App Router + Server Actions** — ไม่มี backend แยก ไม่มี database
- **เก็บข้อมูลใน Google Sheets** ผ่าน `googleapis` (service account) — sheets: `DB_Products`, `DB_Warehouses`, `DB_StockMovements`, `DB_Bills`, `DB_BillItems`, `DB_Installments`, `DB_Users`, `DB_Roles`, `DB_PriceRules`
- มี fallback เป็นไฟล์ JSON local (`.data/dockbill.local.json`) สำหรับ dev
- สร้างจาก template โปรเจค `manage-sale` — มี module ตกค้างที่ไม่เกี่ยวกับ DockBill: `capital`, `daily`, `services`, `purchases` (ไม่ต้อง merge)

### ฟีเจอร์จริงของ DockBill

| ฟีเจอร์ | รายละเอียด |
|---|---|
| คลังสินค้า | ประเภท `MAIN` / `BRANCH` / `GHOST` — **ghost เป็น "คลัง" แยก** ไม่ใช่ bucket ต่อสาขา |
| รับสินค้านำเข้า | ฟอร์มเดียวแยกจำนวนเข้า Real (→ WH_MAIN) และ Ghost (→ WH_GHOST) ตอนรับของ |
| กระจายสินค้า | MAIN → คลังย่อย (movement type `DISTRIBUTE`) |
| Stock | **ไม่เก็บยอดคงเหลือ** — คำนวณใหม่จาก movement ทั้งหมดทุกครั้งที่เปิด dashboard |
| บิล | บิลละ 1 สินค้า, เงินสด (`CASH`) หรือ **ผ่อน (`INSTALLMENT`)**, เลขบิล = `DB-` + timestamp 8 หลักท้าย |
| งวดผ่อน | แตกงวดอัตโนมัติรายเดือน (total/months), สถานะ `PENDING`/`PAID`/`OVERDUE`, dashboard ยอดค้างชำระ |
| ราคา | 3 ระดับต่อสินค้า: `cashPrice` / `retailPrice` / `installmentPrice` |
| RBAC | boolean ต่อหน้า (dashboard/receiving/inventory/distribution/billing/pricing/permissions) ต่อ role: `OWNER` / `WAREHOUSE` / `CASHIER` |
| Auth | username+password **เก็บ plaintext ใน Google Sheets**, cookie ค่าคงที่ตัวเดียวกันทุก user |

---

## 3. เปรียบเทียบสถาปัตยกรรม — ทำไม merge โค้ดตรง ๆ ไม่ได้

| ด้าน | pharmacy-erp-main | DockBill | ผลต่อการ merge |
|---|---|---|---|
| Stack | Go + Echo backend / Next.js frontend / PostgreSQL | Next.js อย่างเดียว / Google Sheets | คนละ paradigm — merge โค้ดตรง ๆ เป็นไปไม่ได้ |
| Stock model | ยอดคงเหลือใน `inventory` (qty_real/qty_ghost ต่อ branch+product) + `inventory_movements` เป็น log | event-sourced ล้วน — sum จาก movements ทุก row | ต้องแปลงข้อมูล ไม่ใช่แปลงโค้ด |
| แนวคิด Ghost | **bucket** ต่อสาขา | **warehouse แยก** (WH_GHOST) | map ได้: ของใน WH_GHOST → `qty_ghost` ของสาขาหลัก |
| Transaction safety | SQL transaction + `SELECT ... FOR UPDATE` | อ่านทั้ง sheet → clear → เขียนทับ (race condition, ข้อมูลหายได้ถ้าใช้พร้อมกัน) | จุดแข็งของโปรเจคหลัก — ต้องคงไว้ |
| ตรวจ stock ก่อนขาย/กระจาย | ✅ เช็ค insufficient stock | ❌ ไม่เช็คเลย — ติดลบได้เงียบ ๆ | ยอดใน Sheets อาจเพี้ยน ต้อง reconcile ก่อน import |
| ภาษี/VAT | ✅ tax_rate, tax_amount, tax_exempt, government mode | ❌ ไม่มี | บิลเก่าจาก DockBill ต้องตัดสินใจวิธีลง VAT ตอน import |
| เลขที่เอกสาร | `document_sequences` ต่อสาขา + ล็อกได้ | timestamp — ชนกันได้, ไม่เรียงต่อเนื่อง | ใช้ระบบของโปรเจคหลัก |
| Auth | bcrypt + JWT + permission keys ละเอียด (seed: `super_admin`, `branch_admin`, `branch_pos`) | plaintext password + static cookie (ปลอม session ได้ง่าย) | **ห้ามเอา auth ของ DockBill มา** |
| Audit | `audit_logs` (before/after JSONB, actor, IP) ทุกธุรกรรม | ไม่มี | จุดแข็งของโปรเจคหลัก |
| งวดผ่อน | ❌ **ไม่มี** | ✅ มี | **ฟีเจอร์หลักที่ต้อง port** |
| ราคา 3 ระดับ | มี cost + base_selling + ราคาต่อสาขา (`branch_product_prices`) | cash/retail/installment ต่อสินค้า | ต้องขยาย schema |
| รับสินค้าเข้า (GRN) | มีแค่ `adjust` ทีละ bucket | ฟอร์ม receive แยก real/ghost ในครั้งเดียว | ควร port เป็น flow ใหม่ |
| กระจายสินค้าระหว่างคลัง | ✅ `transfers` ครบกว่า (requested → in_transit → completed, QR, ผู้รับ/ผู้ส่ง) | append movement row เดียว | ของโปรเจคหลักดีกว่า — ไม่ต้อง port |
| ขายปลีก/POS | ✅ sales module + role `branch_pos` | หน้า retail อย่างง่าย | ครอบคลุมแล้ว |

**ข้อสรุปหลัก: อย่า merge โค้ด — ให้ "port ฟีเจอร์" เข้าสถาปัตยกรรมของ pharmacy-erp-main แล้ว "import ข้อมูล" จาก Google Sheets**

---

## 4. Gap ที่ต้อง port จาก DockBill (เรียงตามความสำคัญ)

### 4.1 ระบบบิลผ่อน/งวด (Installments) — ฟีเจอร์ใหญ่ที่สุดที่โปรเจคหลักไม่มี
- Migration ใหม่ `003_installments.sql`:
  - `installment_plans` (id, invoice_id FK, months, monthly_amount, start_date, status)
  - `installment_payments` (id, plan_id FK, due_date, amount, paid_amount, paid_at, status `pending|paid|overdue`, received_by)
- ขยาย `invoices.payment_status` ให้รองรับ `installment` (ปัจจุบัน CHECK เฉพาะ `unpaid|paid` — ต้อง ALTER constraint)
- Module ใหม่ `backend/internal/modules/installments/` ตาม pattern module เดิม + permission keys ใหม่ (`installment.view`, `installment.manage`, `installment.collect`)
- Frontend: หน้า `(app)/installments/` + การ์ดยอดค้างชำระใน dashboard (มี pattern `sections/*-console.tsx` ให้ทำตามอยู่แล้ว)

### 4.2 ราคาหลายระดับ (cash / retail / installment)
- ทางเลือก A (แนะนำ): เพิ่มคอลัมน์ `retail_price`, `installment_price` ใน `products` + override ต่อสาขาใน `branch_product_prices`
- ทางเลือก B: ตาราง `price_tiers` แบบ generic (ยืดหยุ่นกว่าแต่ซับซ้อน) — ค่อยทำเมื่อมี tier มากกว่า 3
- ผูกกับ `price_source` ใน `invoice_items` ที่มีอยู่แล้ว (ระบุว่าราคามาจาก tier ไหน)

### 4.3 Flow รับสินค้าเข้า (Receiving / GRN)
- Endpoint ใหม่ `POST /inventory/receive`: รับ `real_quantity` + `ghost_quantity` ในคำขอเดียว, เพิ่มเข้า `inventory` ทั้ง 2 bucket, ลง `inventory_movements` (movement_type `receive`) + audit — ใช้ transaction pattern เดียวกับ `Rebalance`
- Frontend: ฟอร์ม receiving ใน inventory console

### 4.4 สิ่งที่ **ไม่ควร** เอามา
- Google Sheets storage ทั้งหมด (`sheets.actions.ts`, `local-sheets.store.ts`)
- Auth แบบ static cookie + plaintext password
- การคำนวณ stock จาก movement ทั้งชีต
- บิล 1 สินค้า/ใบ และเลขบิลจาก timestamp
- Module ตกค้างจาก manage-sale: `capital`, `daily`, `services`, `purchases`

---

## 5. การย้ายข้อมูล (Google Sheets → PostgreSQL)

ลำดับ mapping:

1. `DB_Products` → `products` (sku, name) — ราคา: cashPrice → base_selling_price, retail/installment → คอลัมน์ใหม่ (ข้อ 4.2)
2. `DB_Warehouses` → `branches` — `MAIN` → สาขาหลัก, `BRANCH` → สาขาย่อย, **`WH_GHOST` ไม่สร้างเป็นสาขา** แต่ยอดของมันไปเป็น `qty_ghost` ของสาขาหลัก
3. `DB_StockMovements` → คำนวณยอดสุทธิต่อ (product, warehouse, stockKind) แล้ว **insert ยอดคงเหลือ** เข้า `inventory` + เก็บ movements ดิบเข้า `inventory_movements` (reference_type `dockbill_import`)
4. `DB_Bills` + `DB_BillItems` → `invoices` + `invoice_items` (ออกเลขใหม่ผ่าน `document_sequences` หรือเก็บเลขเดิมใน field อ้างอิง — ต้องตัดสินใจร่วมกับฝ่ายบัญชี เพราะเลข DockBill ไม่เรียงต่อเนื่อง)
5. `DB_Installments` → `installment_plans` / `installment_payments` (ต้องทำ 4.1 ก่อน)
6. `DB_Users` → `users` — **บังคับ reset password ทุกคน** (ของเดิม plaintext), map role: `OWNER` → `super_admin`, `WAREHOUSE` → `branch_admin`, `CASHIER` → `branch_pos`

**ก่อน import ต้องตรวจนับ stock จริง (physical count)** — เพราะ DockBill ไม่เช็ค stock ติดลบและมี race condition ยอดใน Sheets เชื่อไม่ได้ 100%

---

## 6. แผนงานแนะนำ (เป็น Phase)

| Phase | งาน | หมายเหตุ |
|---|---|---|
| 0 | เคลียร์ดิสก์ (เหลือ 288MB — build ไม่ได้), `git init` โปรเจค | **โปรเจคยังไม่เป็น git repo — เสี่ยงมาก ควรทำก่อนแก้โค้ดใด ๆ** |
| 1 | Migration `003_installments.sql` + price tier columns | ระวัง: ALTER CHECK constraint ของ `invoices.payment_status` |
| 2 | Backend modules: `installments`, endpoint `inventory/receive` + permissions ใหม่ + seed | ตาม pattern `modules/*/module.go` เดิม |
| 3 | Frontend: หน้า installments, ฟอร์ม receiving, dashboard ค้างชำระ | ตาม pattern `sections/*-console.tsx` |
| 4 | Script import จาก Google Sheets (อ่านผ่าน service account เดิมของ DockBill) + reconcile กับการตรวจนับจริง | ทำใน staging ก่อน |
| 5 | ทดสอบ end-to-end, ย้ายผู้ใช้, freeze DockBill (read-only) | เก็บ Sheets ไว้เป็น archive |

---

## 7. หมายเหตุความแตกต่างระหว่างเอกสาร skill กับโค้ดจริง (ควรอัปเดต skill)

- SKILL.md/QUICKSTART.md อ้างโครงสร้าง `internal/handlers/` — ของจริงคือ `internal/modules/<name>/module.go`
- อ้างตาราง `inventory_real`/`inventory_ghost` แยก — ของจริงคือตาราง `inventory` เดียว (qty_real/qty_ghost)
- อ้างว่า DockBill ใช้ Drizzle + NextAuth — ของจริงคือ Google Sheets + custom cookie auth
- อ้างตาราง `app_user` — ของจริงชื่อ `users`
- คำสั่ง `pnpm db:generate`/`db:migrate` (Drizzle) — ไม่มีอยู่จริงใน DockBill
