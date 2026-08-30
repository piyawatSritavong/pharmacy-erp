# Pharmacy ERP

ระบบบริหารร้านขายยาและจุดขายหลายสาขา แยกสต๊อกจริง/สต๊อกผี โดยให้ backend เป็นผู้คำนวณราคา ภาษี สต๊อก เลขที่เอกสาร และการชำระเงินทั้งหมด

## เทคโนโลยี

- Backend: Go 1.25, Echo, PostgreSQL 16
- Frontend: Next.js 15 App Router, React 18, Tailwind CSS, Radix UI
- Authentication: JWT ใน HttpOnly cookie
- Deployment: Docker Compose
- Test: Go test และ Playwright

## บัญชีทดสอบ

| บทบาท | อีเมล | รหัสผ่าน | หน้าแรก |
|---|---|---|---|
| ผู้ดูแลระบบสูงสุด | `superadmin@erp.local` | `DevPassword123!` | `/dashboard` |
| ผู้ดูแลระบบส่วนกลาง | `admin.central@erp.local` | `DevPassword123!` | `/dashboard` |
| ผู้ดูแลสาขา MES | `admin.mes@erp.local` | `DevPassword123!` | `/dashboard` |
| POS คลังหลัก MES | `pos.mes@erp.local` | `DevPassword123!` | `/sales` |
| POS หน้ารพ.พหลฯ | `pos.phahol@erp.local` | `DevPassword123!` | `/sales` |
| POS หน้าตลาดผาสุก | `pos.phasuk@erp.local` | `DevPassword123!` | `/sales` |
| POS จังหวัดนครปฐม | `pos.nakhonpathom@erp.local` | `DevPassword123!` | `/sales` |

บัญชีผู้ดูแลสาขาอื่นใช้รูปแบบ `admin.<branch>@erp.local` ตามข้อมูล seed ส่วนรหัสผ่านบัญชี seed กำหนดผ่าน `SEED_POS_PASSWORD`; ค่า fallback สำหรับ development คือ `DevPassword123!`

เมนู **สต๊อกจริง** และ **สต๊อกผี** แยกหน้าจอและการทำงานออกจากกัน สต๊อกผีเป็น bucket ภายใน `WH` ที่ Superadmin ใช้ค้นหา ดูยอด ประวัติ และ Lot เท่านั้น โดยไม่มีตัวเลือกสาขาหรือการปรับยอด ปริมาณสต๊อกผีเปลี่ยนได้เฉพาะวงจรใบสั่งซื้อเข้าและสรุปสิ้นเดือน ส่วนเมนู **สรุปสิ้นเดือน** จำกัดเฉพาะ `super_admin`: เลือกช่วงวันที่และสาขาขาย, ซ่อนบิล `issued + paid + cash only + ไม่ขอใบกำกับภาษีเต็มรูป` ทั้งหมด, ส่ง Real คืน `WH`, รับ Real เข้า `WH`, ตัด Ghost และเก็บ deficit/audit ใน transaction เดียว ช่องยอดเป้าหมายและเปอร์เซ็นต์เป็น Legacy แบบ disabled และไม่มีผลกับรอบใหม่

เมนู **โอนสินค้า** รวมใบโอนและคำขอรับสินค้าไว้ในหน้าเดียว พนักงาน POS ระบุเพียงสินค้าและจำนวน ส่วนผู้ดูแลเลือกสาขาต้นทาง โดยเอกสารโอนใช้สต๊อกจริงเท่านั้น ระบบแยกเอกสารทั่วไปกับงาน **รพ.สต.** และรองรับ POS แบบเงินสด เงินโอน และเงินสดผสมเงินโอน

## รันด้วย Docker

บน macOS ที่ใช้ Colima:

```bash
colima start
docker context use colima
cd deploy
DOCKER_BUILDKIT=0 docker compose build
docker compose up -d
docker compose ps
```

เปิดใช้งาน:

- เว็บ: <http://localhost:3000>
- API health check: <http://localhost:8080/api/v1/health>
- PostgreSQL: `localhost:5432`

หากพอร์ตชนกับโปรเจกต์อื่น สามารถกำหนด host port โดยไม่แก้พอร์ตภายใน container:

```bash
cd deploy
FRONTEND_HOST_PORT=3100 DATABASE_HOST_PORT=5433 \
FRONTEND_URL=http://localhost:3100 \
DOCKER_BUILDKIT=0 COMPOSE_DOCKER_CLI_BUILD=0 \
docker compose up -d --build
```

ตรวจ log:

```bash
cd deploy
docker compose logs -f backend frontend
```

หยุดระบบโดยเก็บข้อมูลไว้:

```bash
docker compose down
```

คำเตือน `Docker Compose requires buildx plugin` ไม่กระทบ runtime แต่ builder แบบใหม่ใช้งานไม่ได้ในเครื่องนั้น คำสั่ง `DOCKER_BUILDKIT=0 docker compose build` จะใช้ classic builder และหลีกเลี่ยงปัญหา image SHA ที่เคยเกิดขึ้น

## ข้อมูลถาวร

Docker Compose ใช้สอง volume:

- `pharmacy_erp_db`: ฐานข้อมูล PostgreSQL
- `pharmacy_erp_uploads`: รูปสินค้าที่อัปโหลดจากหน้าจัดการสินค้า

อย่าใช้ `docker compose down -v` หากยังต้องการข้อมูลและรูปสินค้า

## ชุดข้อมูล Ocha POS

ไฟล์ `backend/internal/app/seeddata/ocha_catalog.json` สร้างจาก MHTML 8 ไฟล์แบบ deterministic และฝังเข้า backend สำหรับ fresh seed โดยตรง ตรวจว่าไฟล์ต้นฉบับและ JSON ยังตรงกันได้ด้วย:

```bash
python3 scripts/extract_ocha_seed.py --check
```

เติมสต๊อกสำหรับทดสอบให้สินค้าทุกตัวในทุกสาขาที่เปิดใช้งานมีสต๊อกจริงอย่างน้อย
10 ชิ้น และให้สต๊อกผีอย่างน้อย 10 ชิ้นเฉพาะโกดัง `WH` โดยสร้างบริษัทคู่ค้า
ใบสั่งซื้อเข้า รายการรับเข้า ล็อต และ inventory ledger ครบทั้งสายได้ด้วยคำสั่ง:

```bash
cd backend
APP_ENV=development DATABASE_URL='postgres://pharmacy:pharmacy@localhost:5432/pharmacy_erp?sslmode=disable' \
go run ./cmd/api seed-inventory-floor
```

คำสั่งนี้เพิ่มเฉพาะยอดที่ขาดและรันซ้ำได้โดยไม่ลดสต๊อกเดิม

## สรุปสิ้นเดือนแบบช่วงวันที่

1. Login ด้วย `superadmin@erp.local` แล้วเปิด `/month-end`
2. เลือก `date_from`, `date_to` และสาขาขาย (ตัวเลือกไม่รวม `WH`)
3. กด **ตรวจสอบใบขายและสต๊อก** เพื่อดูบิลทั้งหมดที่จะซ่อน พร้อม projection ของ Branch Real returned, WH Real received, WH Ghost deducted และ deficit
4. คำเตือน Ghost ไม่พอเป็นข้อมูลเพื่อทราบและไม่ปิดปุ่มยืนยัน พิมพ์ข้อความยืนยันตามหน้าจอเพื่อจบ transaction
5. เปิด `/month-end-report` แล้วเลือก reconciliation เพื่อดู Before/After และขยาย movement รายสินค้า

API หลักคือ `POST /api/v1/accounting/month-end/reconciliation-overview`, `POST /api/v1/accounting/month-end/reconciliation-preview`, `POST /api/v1/accounting/month-end/reconciliations` และ `GET /api/admin/month-end-report?reconciliation_id=...` ทุก endpoint ตรวจ literal role `super_admin`

คำสั่งแทนที่ catalog ปัจจุบันเป็น dry-run โดยปริยาย และยังไม่แก้ฐานข้อมูล:

```bash
cd backend
DATABASE_URL='postgres://pharmacy:pharmacy@localhost:5432/pharmacy_erp?sslmode=disable' \
  go run ./cmd/api replace-ocha-catalog
```

ก่อนยืนยันต้องสำรอง PostgreSQL แบบ custom-format และ upload volume พร้อมตรวจว่าไฟล์สำรองอ่านได้ จากนั้นจึงใช้ guard ทั้งสองชั้น:

```bash
cd backend
ALLOW_MASTER_DATA_RESET=true \
SEED_POS_PASSWORD='เปลี่ยนเป็นรหัสผ่านที่ต้องการ' \
DATABASE_URL='postgres://pharmacy:pharmacy@localhost:5432/pharmacy_erp?sslmode=disable' \
  go run ./cmd/api replace-ocha-catalog --confirm=REPLACE-OCHA-CATALOG
```

คำสั่งนี้ล้างธุรกรรมและ master data เดิม แต่คง Super Admin, suppliers, saved reports, roles, permissions และ settings ไว้ แล้วสร้าง opening movement/lot ให้ยอด ledger ตรงกับ inventory การทำงานอยู่ใน transaction เดียวและใช้ advisory lock; หากขั้นตอนใดล้มเหลวฐานข้อมูลจะ rollback ทั้งหมด รหัสผ่าน POS อ่านจาก `SEED_POS_PASSWORD` และไม่ถูกเก็บใน JSON

## ล้างข้อมูลธุรกรรม

คำสั่งนี้เก็บ roles, users, branches, settings, หมวดสินค้า, สินค้า, ราคา, ชื่อราชการ, รูปสินค้า และยอดคงเหลือจริง/ผีไว้ แต่ล้างใบขาย ใบเสนอราคา การโอน คำขอรับสินค้า รอบสิ้นเดือน marketplace และ audit log

ดูจำนวนแถวที่จะลบแบบ dry-run:

```bash
cd backend
DATABASE_URL='postgres://pharmacy:pharmacy@localhost:5432/pharmacy_erp?sslmode=disable' \
  go run ./cmd/api reset-operational-data
```

ยืนยันการล้างจริง:

```bash
cd backend
ALLOW_OPERATIONAL_DATA_RESET=true \
DATABASE_URL='postgres://pharmacy:pharmacy@localhost:5432/pharmacy_erp?sslmode=disable' \
  go run ./cmd/api reset-operational-data --confirm=RESET-OPERATIONAL-DATA
```

## รันแบบ Local

Backend:

```bash
cd backend
cp configs/app.example.env .env
go run ./cmd/api serve
```

Frontend:

```bash
cd frontend
npm ci
npm run dev
```

## คำสั่งตรวจสอบ

```bash
cd backend
go test ./...
```

```bash
cd frontend
npm run lint
npm run build
npm audit --audit-level=moderate
```

เมื่อ Docker ทั้งสาม service ทำงานแล้ว:

```bash
cd frontend
E2E_RUN=1 E2E_SKIP_WEBSERVER=1 npm run e2e
```

Playwright regression ครอบคลุม login, navigation ใหม่, responsive layout, สต๊อกจริง/ผีรายสาขา, คำขอและใบโอนสินค้า, เอกสารทั่วไป/รพ.สต., POS เงินสด/เงินโอน/ชำระผสม, รายงาน และสรุปสิ้นเดือน

## เอกสาร

- [สเปคระบบ](docs/SPEC.md)
- [Manual Test Case](docs/MANUAL_TEST_CASES.md)
- [สถาปัตยกรรมและคู่มือสรุปสิ้นเดือน](docs/MONTH_END_WORKFLOW.md)
