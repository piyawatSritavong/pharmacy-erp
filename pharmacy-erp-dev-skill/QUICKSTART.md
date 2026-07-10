# Pharmacy ERP Development Quick Start

> **สถานะ (2026-07-11):** ทุกฟีเจอร์ในเอกสารนี้ถูก implement แล้วในโค้ดจริง รวมถึงฟีเจอร์ที่ port มาจาก DockBill (บิลผ่อน, ราคา 3 ระดับ, รับสินค้าเข้า) — เอกสารนี้อธิบาย "ของจริง" ไม่ใช่ขั้นตอน setup อีกต่อไป
>
> ⚠️ **อย่า copy ไฟล์ใน `references/` เข้าโปรเจค** — เป็นตัวอย่างเก่าที่ schema/โครงสร้างไม่ตรงกับโค้ดจริง (SQL เป็น MySQL syntax, อ้างตาราง `app_user`/`inventory_real` ที่ไม่มีจริง) ใช้ดูเป็นแนวคิดเท่านั้น

## สถาปัตยกรรมจริง

- Backend: Go + Echo, module ละไฟล์ที่ `backend/internal/modules/<name>/module.go` (Service + Handler ในไฟล์เดียว)
- Migrations: `backend/migrations/NNN_name.sql` — embed ผ่าน `migrations/embed.go` รันอัตโนมัติเรียงตามชื่อไฟล์ก่อน serve
- Routes + permissions: `backend/internal/http/server.go` (ทุก route ผูก `RequireAnyPermission`)
- Seed (roles/permissions/demo data): `backend/internal/app/seed.go` — ข้ามทั้งหมดถ้ามี users แล้ว ดังนั้น **permission ใหม่ต้องใส่ทั้งใน seed และ migration** (แบบ `ON CONFLICT DO NOTHING` — ดูตัวอย่างใน `003_installments_price_tiers.sql`)
- Frontend: Next.js App Router — หน้าใน `frontend/src/app/(app)/`, client component ใน `frontend/src/components/sections/*-console.tsx`, service ฝั่ง server ใน `frontend/src/services/erp.ts`
- เมนู sidebar มาจาก backend: `navigationFor()` ใน `backend/internal/modules/auth/module.go`

## ฟีเจอร์หลักและตำแหน่งในโค้ด

| ฟีเจอร์ | Backend | Frontend |
|---|---|---|
| Dual inventory (real/ghost) | ตาราง `inventory` (qty_real/qty_ghost) + `inventory_movements`; `modules/inventory` (List / Rebalance / Adjust / Receive) | `sections/inventory-console.tsx` |
| รับสินค้าเข้า แยก real/ghost | `POST /inventory/receive` (permission `inventory.receive`) | โหมด "Receive Stock" ใน inventory console |
| ราคา 3 ระดับ (cash/retail/installment) | `products.retail_price` / `installment_price`; `price_tier` ต่อบรรทัดขายใน `modules/sales` (ลำดับ: override > gov alias > tier > branch/base) | ช่องราคาใน product console + Price Tier ใน document composer |
| บิลผ่อน/งวด | `installment_plans` + `installment_payments`; `modules/installments` (สร้างแผนจาก invoice unpaid, เก็บเงินรายงวดลง `invoice_payments`, ครบงวด → invoice paid) | หน้า `/installments` + `sections/installment-console.tsx` |
| Government mode (รพสต.) | `product_aliases` + `invoices.is_government_mode`; permissions `government.*` | Government mode ใน document composer |
| เลขที่เอกสารล็อก | `document_sequences` (`PREFIXYYYYMMDDNNNNN`, `is_locked`) | จัดการใน Settings |
| RBAC | roles: `super_admin` / `branch_admin` / `branch_pos` + permission keys ละเอียด | `requireRole` ใน page + navigation จาก backend |
| Audit | `audit_logs` — ทุก mutation ผ่าน `audit.Service.Log` ใน transaction เดียวกัน | หน้า audit ใน Settings |

## วิธีเพิ่มฟีเจอร์ใหม่ (pattern จริงของโปรเจค)

1. **Migration**: สร้าง `backend/migrations/NNN_feature.sql` (เลขถัดไป, PostgreSQL syntax, idempotent ด้วย `IF NOT EXISTS` / `ON CONFLICT`)
2. **Module**: สร้าง `backend/internal/modules/<feature>/module.go` — struct Service{db, audit} + Handler; mutation ทุกตัวใช้ `platform.WithTx` + `FOR UPDATE` + `audit.Log` ใน tx เดียวกัน
3. **Routes**: ลงทะเบียนใน `server.go` พร้อม `RequireAnyPermission("<feature>.xxx")`
4. **Permissions**: เพิ่มใน `seed.go` (list + role mapping) **และ** ใน migration แบบ `ON CONFLICT DO NOTHING`
5. **Frontend**: เพิ่ม getter ใน `services/erp.ts` → สร้าง `sections/<feature>-console.tsx` (เรียก `proxyClient` + `router.refresh()`) → สร้างหน้าใน `(app)/<feature>/page.tsx` (guard ด้วย `requireRole`) → เพิ่มเมนูใน `navigationFor()`
6. **Test**: unit test ข้าง module (`module_test.go`) + `go vet ./... && go test ./...` + `npx tsc --noEmit && npm run build`

## รันและทดสอบ

```bash
# Backend (ต้องมี PostgreSQL)
cd backend
go run ./cmd/api serve        # migrate + seed + serve อัตโนมัติ

# Frontend
cd frontend
npm run dev

# ทั้ง stack
cd deploy && docker compose up --build

# ทดสอบ
cd backend && go vet ./... && go test ./...
cd frontend && npx tsc --noEmit && npm run build
```

Postgres ชั่วคราวสำหรับทดสอบ migration:

```bash
docker run -d --name erp-test-pg -e POSTGRES_PASSWORD=test -e POSTGRES_DB=pharmacy_test -p 55432:5432 postgres:16-alpine
DATABASE_URL="postgres://postgres:test@localhost:55432/pharmacy_test?sslmode=disable" go run ./cmd/api migrate
```

## บัญชีทดสอบ (จาก seed)

- `superadmin@erp.local` / `DevPassword123!` — จัดการสินค้า, ราคา, ระบบทั้งหมด
- `branchadmin@erp.local` / `DevPassword123!` — รับของ, สร้างแผนผ่อน, ออกบิลสาขา
- `pos@erp.local` / `DevPassword123!` — ขาย, เก็บเงินงวด (สร้างแผน/รับของไม่ได้)
