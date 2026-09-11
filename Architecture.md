สรุปรวมทุกอย่างที่คุยกันมา

> **เปลี่ยน target: Cloud Run → Render** (2026-09-11) งานที่ทำไว้ส่วนใหญ่ยังใช้ได้ทั้งหมด — ขอบเขตสาขา, access log, health check, timeouts, graceful shutdown, `serve` ที่ไม่ migrate เอง ไม่มีอะไรผูกกับ Cloud Run สิ่งที่เปลี่ยนมีแค่ ขนาด connection pool, `EXPOSE`, และไฟล์ blueprint

## สถาปัตยกรรม

```
เบราว์เซอร์
   │
   ├─ mes.ihavepro.com ──────┐
   └─ *.mes.ihavepro.com ────┤  (Phase 2)
                             ▼
                    Vercel (Next.js 15)
                             │  proxy /api/backend/* → ตั้ง cookie ที่นี่
                             ▼  HTTPS
        Render web service: pharmacy-erp-api (singapore)
          docker · plan starter · instance เดียวคงที่
          serve เท่านั้น · non-root uid 10001 · PORT จาก Render
                             │  sslmode=require
                             ▼  pooler :6543  (MaxOpenConns 20)
                   Supabase Pro (Singapore)
```

**หลักการเดียวที่ต้องยึด: สาขาคือข้อมูล ไม่ใช่ infrastructure** เพิ่มสาขา = INSERT หนึ่งแถว ไม่ต้อง deploy ไม่ต้องแตะ DNS

Render กับ Supabase อยู่ที่สิงคโปร์ทั้งคู่ latency ระหว่าง API กับ DB จึงต่ำที่สุดเท่าที่ทำได้ (ไม่มี region ไทยทั้งสองเจ้า)

## ลำดับการ deploy

**1. Supabase — สร้าง project ใหม่แยกของ Pharmacy**

region Singapore, seed 6 สาขา — migration ไม่ต้องรันมือ Render รันให้เองใน `preDeployCommand`

**2. Render — deploy Go API**

ใช้ `render.yaml` ที่ root: New → Blueprint → ชี้ที่ repo

ตั้ง 2 ค่าในหน้า dashboard เอง (ในไฟล์เป็น `sync: false` ไม่เก็บลง repo)

| key | ค่า |
|---|---|
| `DATABASE_URL` | Supabase **transaction pooler พอร์ต 6543** ห้ามพอร์ต 5432 |
| `JWT_SECRET` | สุ่มยาวๆ ไม่ใช่ค่า dev |

**3. Vercel — deploy frontend**

ผูก `mes.ihavepro.com` (เจาะจง) + `*.mes.ihavepro.com` (wildcard) ชี้ `BACKEND_INTERNAL_URL` ไปที่ URL ของ Render

**4. DNS**

ย้าย nameserver ihavepro.com ไป Vercel — export DNS record เดิมทั้งหมดเก็บไว้ก่อน โดยเฉพาะ MX

## สิ่งที่อยู่ในโค้ดแล้ว

### ขอบเขตสาขา — `platform.MustBranchID`

ฟังก์ชัน `resolveBranchContext()` ที่วางแผนไว้ ตอนนี้คือ `backend/internal/platform/branch_scope.go` มี 3 รูปแบบ

| ใช้เมื่อ | ฟังก์ชัน | ถ้าไม่ระบุสาขา |
|---|---|---|
| เขียนข้อมูล / ต้องมีสาขาเสมอ | `MustBranchID(user, requested)` | 403 |
| แสดงรายการ (สนญ. เห็นทุกสาขาได้) | `BranchFilter(user, requested)` | สาขาตัวเอง หรือ `""` = ทุกสาขา (เฉพาะ scope=global) |
| จัดการตัวสาขาเอง (แก้/ลบ/เลขเอกสาร) | `RequireGlobalScope(user)` | 403 สำหรับ scope=branch |

**กฎเดียว: สาขาที่มีผลมาจาก JWT เสมอ** `branch_id` ที่ส่งมาทาง query/path/body ทำได้แค่ *แคบลง* ภายในสิทธิ์ของผู้เรียก ห้ามขยาย

> Phase 2 (อ่านสาขาจาก subdomain) เพิ่มได้ที่ `decideBranch()` จุดเดียว โครงสร้างเดิมรองรับไว้แล้ว

### สถานะ RLS — ยังไม่ได้ทำ

แผนเดิมเขียนว่า "เปิด RLS ทุกตารางที่มี `branch_id`" **ตอนนี้ยังไม่ได้เปิด** ของจริงคือ 59 ตาราง มี `branch_id` 28 ตาราง เปิด RLS 0 ตาราง policy 0 อัน และ DB user ของแอปเป็น superuser (`bypassrls=true`) — ถึงเปิด RLS ก็จะถูกข้ามอยู่ดี

การกันข้ามสาขาตอนนี้อยู่ที่ชั้นแอปทั้งหมด (ตารางด้านบน) ไม่ใช่ที่ชั้นฐานข้อมูล **ก่อน go-live ต้องทำ**: สร้าง DB role แยกที่ไม่ใช่ superuser ให้แอปใช้ แล้วค่อยเปิด RLS

### Audit trail

- `audit_logs` = การ**เขียน**เท่านั้น (`promotion.create`, `pos.checkout`, …) ไม่เก็บการอ่าน
- **access log**: JSON บรรทัดละ 1 request ออก stdout → Render เก็บให้เองใน Logs
  เก็บ `request_id`, `method`, `path`, `query` (redact password/token), `status`, `latency_ms`, `user_id`, `user_email`, `role_key`, `token_branch`, `effective_branch`, `branch_refused`
  4xx = `WARNING`, 5xx = `ERROR` พร้อม field `error` ที่บอกสาเหตุจริง
- `effective_branch` คือสาขาที่ **ตัดสินแล้ว** ไม่ใช่ที่ขอมา — `*` แปลว่าทุกสาขา, ว่างแปลว่า request นี้ไม่เกี่ยวกับสาขา

ค้นหาใน Render Logs: กรองคำว่า `"branch_refused":true` — เจอทุกครั้งที่มีคนพยายามข้ามสาขา

> Render เก็บ log ย้อนหลังจำกัด (plan starter ไม่กี่วัน) ถ้าต้องการเก็บยาวสำหรับ audit ต้องตั้ง **Log Stream** ส่งออกไปที่อื่น — ไม่งั้นจะกลับไปเจอปัญหาเดิมคือ "ย้อนหลังไม่ได้เพราะไม่มี log"

### คำสั่ง 2 แบบ ไม่ใช่แบบเดียว

`serve` **ไม่ migrate แล้ว** เดิมทุก boot จะรัน `Migrate()` + `Seed()` — บน Render จะ migrate ซ้ำทุกครั้งที่ restart และคั่นเวลา boot อยู่ดี (`schema_migrations` ไม่มี lock ตัดสินใจจากการ SELECT ก่อน) ตอนนี้ migration เป็น `preDeployCommand` ซึ่ง Render รันครั้งเดียวต่อ deploy ก่อน instance ใหม่รับ traffic

| คำสั่ง | ใช้ตอนไหน |
|---|---|
| `serve` | Dockerfile CMD — เสิร์ฟอย่างเดียว |
| `migrate` | `preDeployCommand` ใน render.yaml |
| `migrate-and-seed` | ตอนตั้งระบบใหม่ / compose ตอน dev |

### Connection pool — อย่าลดกลับเป็น 5

```
MaxOpenConns = 20
MaxIdleConns = 10
```

**เคยเป็น 5 ตอนที่ target เป็น Cloud Run และนั่นถูกสำหรับตอนนั้น** บน Cloud Run เลขที่มีความหมายคือ *instances × pool* — เปิดได้ถึง 10 instance ถ้าตัวละ 20 คือ 200 connection ที่ pooler ไม่มีให้ จึงต้องกดเหลือ 5

**Render รัน instance เดียวคงที่ ไม่ใช่ autoscale** เลขคูณจึงหายไป เหลือ pool เดียว 20 connection ถ้าเก็บ 5 ไว้ = request ที่ 6 ขึ้นไปต้องรอคิว *connection* ไม่ใช่รอ *ฐานข้อมูล* ซึ่งคือการทำให้ช้าลงเปล่าๆ

> **ถ้าจะเปลี่ยนเลขนี้ ต้องเปลี่ยนพร้อมกับจำนวน instance เสมอ** สองค่านี้คือการตัดสินใจเดียวกัน ขึ้น Render เป็น plan ที่ scale หลาย instance เมื่อไหร่ ค่อยหารกลับ

ต่อผ่าน **transaction pooler พอร์ต 6543** เสมอ ห้ามต่อตรง 5432

### Timeouts

| จุด | ค่า | เหตุผล |
|---|---|---|
| ReadTimeout | 15s | client ที่ค้างไม่ยึด instance |
| WriteTimeout | 30s | รายงานสิ้นเดือนหนักสุดยังทัน |
| IdleTimeout | 60s | keep-alive |
| SIGTERM drain | 10s | Render ส่ง SIGTERM ตอน deploy ใหม่ — request ที่ค้างอยู่ได้ทำจนจบ ไม่ตัดกลางบิล |

### Health check

| path | auth | ทำอะไร |
|---|---|---|
| `/healthz` | ไม่ต้อง | ping DB ด้วย timeout 2s → 200 / 503 ใช้เป็น `healthCheckPath` ของ Render + UptimeRobot |
| `/api/v1/health` | ไม่ต้อง | ของเดิม ไม่แตะ ตอบ `{"status":"ok"}` เฉยๆ |

`/healthz` แข่งกับ deadline เอง ไม่ฝากไว้กับ driver — `lib/pq` ยกเลิก query ด้วยการเปิด connection ที่สองไปบอก server ซึ่งค้างพอกันเมื่อ server เป็นตัวที่ไม่ตอบ (วัดจริง: DB ค้าง → ping กลับมา 10 วินาทีให้หลัง แล้วตอบ 200) ตอนนี้ตอบ 503 ที่ 2.01s

### PORT

`config.Load()` อ่าน `PORT` ก่อน แล้วค่อย `HTTP_PORT` แล้วค่อย 8080 — Render ตั้ง `PORT` ให้เอง `EXPOSE 8080` ใน Dockerfile เป็นเอกสารบอกค่า default เฉยๆ ไม่ได้บังคับ (ทดสอบแล้ว: ตั้ง `PORT=10000` container ฟังที่ 10000 ไม่ใช่ 8080)

## ค่าที่ต้องตั้งให้ถูก

| จุด | ค่า | ถ้าพลาดจะเกิดอะไร |
|---|---|---|
| Render region | singapore | latency สูง |
| Supabase connection | pooler :6543 | ระบบล่มตอนขายดี |
| MaxOpenConns | 20 (instance เดียว) | ตั้ง 5 = คอขวดเปล่าๆ ดูหัวข้อ pool |
| sslmode | require (default ในโค้ดแล้ว) | lib/pq default คือ `prefer` = ยอมต่อแบบไม่เข้ารหัสเงียบๆ |
| Cookie Domain | **ห้ามตั้ง** ปล่อย host-only | token รั่วข้ามลูกค้า |
| RLS | ยังไม่เปิด — กันที่ชั้นแอป | ดูหัวข้อ RLS ด้านบน |
| บัญชี POS | 1 บัญชีต่อ 1 เครื่อง | ดีดกันเองในสาขาเดียวกัน |
| Log retention | ตั้ง Log Stream ถ้าต้องเก็บยาว | ย้อนหลัง audit ไม่ได้ |
| Spend cap | เปิดทั้ง Vercel + Supabase | บิลพุ่งโดยไม่รู้ตัว |

**เช็ค cookie ก่อน go-live** — DevTools → Application → Cookies ดูคอลัมน์ Domain ถ้าขึ้นต้นด้วยจุด คือมีปัญหา ต้องเป็น `mes.ihavepro.com` เต็มๆ

## ก่อนส่งมอบ

- **เปิด RLS + แยก DB role ที่ไม่ใช่ superuser** (ข้อเดียวที่ยังค้างจากแผนเดิม)
- ทดสอบ `pg_dump` แล้ว restore ขึ้น project เปล่า จับเวลา — สัญญาลูกค้าไว้ว่าส่งข้อมูลได้ใน 7 วัน ยังไม่เคยพิสูจน์
- ตั้ง UptimeRobot ยิง `/healthz` ทุก 5 นาที เข้า Telegram bot ที่มีอยู่แล้ว
- ตั้ง Log Stream ถ้าต้องเก็บ access log ยาวกว่าที่ plan ให้
- เขียนไฟล์ทะเบียน tenant — Pharmacy ใช้ Supabase project ref ไหน, Render service อะไร, subdomain อะไร
- ล็อกอินพร้อมกันจากสองเครื่องจริง ทดสอบว่าไม่ดีดกัน (อย่าทดสอบในเบราว์เซอร์เดียว มันจะชนแน่นอนและไม่ใช่ของจริง)

---

Phase 2 (wildcard รายสาขา) รอไว้ก่อน โครงสร้าง DNS ที่วางวันนี้รองรับไว้แล้ว เปิดใช้ทีหลังโดยแก้แค่ `decideBranch()` ฟังก์ชันเดียว
