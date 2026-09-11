สรุปรวมทุกอย่างที่คุยกันมา

## สถาปัตยกรรม

```
เบราว์เซอร์
   │
   ├─ mes.ihavepro.com ──────┐
   └─ *.mes.ihavepro.com ────┤
                             ▼
                    Vercel (Next.js)
                             │  HTTPS
                             ▼
            Cloud Run (Go API, asia-southeast3)
                             │  pooler :6543
                             ▼
              Supabase Pro — project แยกของ Pharmacy
                  (branches, users, RLS)
```

**หลักการเดียวที่ต้องยึด: สาขาคือข้อมูล ไม่ใช่ infrastructure** เพิ่มสาขา = INSERT หนึ่งแถว ไม่ต้อง deploy ไม่ต้องแตะ DNS

## ลำดับการ deploy

**1. Supabase — สร้าง project ใหม่แยกของ Pharmacy**

region Singapore (Supabase ยังไม่มี region ไทย), รัน migration, seed 6 สาขา, เปิด RLS ทุกตารางที่มี `branch_id` แล้วทดสอบด้วย service key ปิด — ต้องเข้าข้อมูลสาขาอื่นไม่ได้

**2. Cloud Run — deploy Go API**

region `asia-southeast3` (กรุงเทพฯ), `min-instances=1`, `max-instances` ตั้งเพดานไว้กัน bill พุ่ง, concurrency 80, health check `/healthz`

ต่อ Supabase ผ่าน **transaction pooler พอร์ต 6543** ห้ามต่อตรงพอร์ต 5432 ข้อนี้พลาดไม่ได้ — Cloud Run ขยาย instance เองแล้ว connection จะหมดตอนคนใช้เยอะ

**3. Vercel — deploy frontend**

ผูก `mes.ihavepro.com` (เจาะจง) + `*.mes.ihavepro.com` (wildcard)

**4. DNS**

ย้าย nameserver ihavepro.com ไป Vercel — export DNS record เดิมทั้งหมดเก็บไว้ก่อน โดยเฉพาะ MX

## ค่าที่ต้องตั้งให้ถูก

| จุด | ค่า | ถ้าพลาดจะเกิดอะไร |
|---|---|---|
| Cloud Run region | asia-southeast3 | latency สูง / ข้อมูลออกนอกประเทศ |
| Cloud Run min-instances | 1 | cold start ตอนคิดเงินหน้าเคาน์เตอร์ |
| Supabase connection | pooler :6543 | ระบบล่มตอนขายดี |
| Cookie Domain | **ห้ามตั้ง** ปล่อย host-only | token รั่วข้ามลูกค้า |
| RLS | เปิดทุกตาราง | ข้อมูลข้ามสาขา |
| บัญชี POS | 1 บัญชีต่อ 1 เครื่อง | ดีดกันเองในสาขาเดียวกัน |
| Spend cap | เปิดทั้ง Vercel + Supabase | บิลพุ่งโดยไม่รู้ตัว |

**เช็ค cookie ก่อน go-live** — DevTools → Application → Cookies ดูคอลัมน์ Domain ถ้าขึ้นต้นด้วยจุด คือมีปัญหา ต้องเป็น `mes.ihavepro.com` เต็มๆ

## สิ่งที่อยู่ในโค้ดแล้ว (อัปเดต 2026-09-11)

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
- **access log ใหม่**: JSON บรรทัดละ 1 request ออก stdout → Cloud Logging เก็บให้เอง
  เก็บ `request_id`, `method`, `path`, `query` (redact password/token), `status`, `latency_ms`, `user_id`, `user_email`, `role_key`, `token_branch`, `effective_branch`, `branch_refused`
  4xx = `WARNING`, 5xx = `ERROR` พร้อม field `error` ที่บอกสาเหตุจริง
- `effective_branch` คือสาขาที่ **ตัดสินแล้ว** ไม่ใช่ที่ขอมา — `*` แปลว่าทุกสาขา, ว่างแปลว่า request นี้ไม่เกี่ยวกับสาขา

ค้นหาใน Cloud Logging:

```
resource.type="cloud_run_revision"
jsonPayload.branch_refused=true
```

---

## Deploy topology (ของจริง)

```
เบราว์เซอร์
   │
   ├─ mes.ihavepro.com ──────┐
   └─ *.mes.ihavepro.com ────┤  (Phase 2)
                             ▼
                    Vercel (Next.js 15)
                             │  proxy /api/backend/* → ตั้ง cookie ที่นี่
                             ▼  HTTPS
         Cloud Run: pharmacy-erp-api (asia-southeast1)
            serve เท่านั้น · non-root uid 10001 · PORT จาก Cloud Run
                             │  sslmode=require
                             ▼  pooler :6543  (MaxOpenConns 5 / instance)
                      Supabase Pro (Singapore)
```

> **region — ต้องยืนยันก่อน deploy**: แผนเดิมเขียน `asia-southeast3` (กรุงเทพฯ) เท่าที่ทราบ Cloud Run ไม่มี region นี้ — ในโซนนี้มี `asia-southeast1` (สิงคโปร์) กับ `asia-southeast2` (จาการ์ตา) เท่านั้น **ยังไม่ได้ยืนยันจากเครื่องนี้** (gcloud ล็อกอินอยู่กับ project อื่นและยังไม่ได้เปิด Cloud Run API) สั่งเองก่อน deploy:
>
> ```bash
> gcloud run regions list
> ```
>
> ถ้าไม่มี `asia-southeast3` จริง ให้ใช้ `asia-southeast1` ซึ่งอยู่ region เดียวกับ Supabase พอดี latency ระหว่าง API กับ DB จึงต่ำที่สุด

### คำสั่ง 2 แบบ ไม่ใช่แบบเดียว

`serve` **ไม่ migrate แล้ว** เดิมทุก boot จะรัน `Migrate()` + `Seed()` ซึ่งพังเมื่อ Cloud Run เปิดหลาย instance พร้อมกัน (`schema_migrations` ไม่มี lock, ตัดสินใจจากการ SELECT ก่อน) และ cold start ต้องรอ migrate+seed จบใน 60 วินาทีก่อนรับ request แรก

| คำสั่ง | ใช้ตอนไหน |
|---|---|
| `serve` | Cloud Run CMD — เสิร์ฟอย่างเดียว |
| `migrate` | ขั้นตอน deploy รันครั้งเดียว (Cloud Run Job) |
| `migrate-and-seed` | ตอนตั้งระบบใหม่ / compose ตอน dev |

### Connection budget

`MaxOpenConns = 5` ต่อ instance ไม่ใช่ 20 — เพดานจริงคือ **instances × 5** ถ้า `max-instances=10` คือ 50 connection ไปที่ pooler ถ้าตั้ง 20 เหมือนเดิมจะเป็น 200 ซึ่ง pooler ไม่ให้

### Timeouts

| จุด | ค่า | เหตุผล |
|---|---|---|
| ReadTimeout | 15s | client ที่ค้างไม่ยึด instance |
| WriteTimeout | 30s | รายงานสิ้นเดือนหนักสุดยังทัน |
| IdleTimeout | 60s | keep-alive |
| SIGTERM drain | 10s | เท่ากับ grace period ของ Cloud Run — request ที่ค้างอยู่ได้ทำจนจบ ไม่ตัดกลางบิล |

### Health check

| path | auth | ทำอะไร |
|---|---|---|
| `/healthz` | ไม่ต้อง | ping DB ด้วย timeout 2s → 200 / 503 ใช้กับ Cloud Run probe + UptimeRobot |
| `/api/v1/health` | ไม่ต้อง | ของเดิม ไม่แตะ ตอบ `{"status":"ok"}` เฉยๆ |

`/healthz` แข่งกับ deadline เอง ไม่ฝากไว้กับ driver — `lib/pq` ยกเลิก query ด้วยการเปิด connection ที่สองไปบอก server ซึ่งค้างพอกันเมื่อ server เป็นตัวที่ไม่ตอบ (วัดจริง: DB ค้าง → ping กลับมา 10 วินาทีให้หลัง แล้วตอบ 200) ตอนนี้ตอบ 503 ที่ 2.01s

## ค่าที่ต้องตั้งให้ถูก

| จุด | ค่า | ถ้าพลาดจะเกิดอะไร |
|---|---|---|
| Cloud Run region | asia-southeast1 (ยืนยันก่อน) | latency สูง / deploy ไม่ผ่านถ้า region ไม่มีจริง |
| Cloud Run min-instances | 1 | cold start ตอนคิดเงินหน้าเคาน์เตอร์ |
| Supabase connection | pooler :6543 | ระบบล่มตอนขายดี |
| MaxOpenConns | 5 ต่อ instance | pooler เต็ม |
| sslmode | require (default ในโค้ดแล้ว) | lib/pq default คือ `prefer` = ยอมต่อแบบไม่เข้ารหัสเงียบๆ |
| Cookie Domain | **ห้ามตั้ง** ปล่อย host-only | token รั่วข้ามลูกค้า |
| RLS | ยังไม่เปิด — กันที่ชั้นแอป | ดูหัวข้อ RLS ด้านบน |
| บัญชี POS | 1 บัญชีต่อ 1 เครื่อง | ดีดกันเองในสาขาเดียวกัน |
| Spend cap | เปิดทั้ง Vercel + Supabase | บิลพุ่งโดยไม่รู้ตัว |

**เช็ค cookie ก่อน go-live** — DevTools → Application → Cookies ดูคอลัมน์ Domain ถ้าขึ้นต้นด้วยจุด คือมีปัญหา ต้องเป็น `mes.ihavepro.com` เต็มๆ

## ก่อนส่งมอบ

- **เปิด RLS + แยก DB role ที่ไม่ใช่ superuser** (ข้อเดียวที่ยังค้างจากแผนเดิม)
- ทดสอบ `pg_dump` แล้ว restore ขึ้น project เปล่า จับเวลา — สัญญาลูกค้าไว้ว่าส่งข้อมูลได้ใน 7 วัน ยังไม่เคยพิสูจน์
- ตั้ง UptimeRobot ยิง `/healthz` ทุก 5 นาที เข้า Telegram bot ที่มีอยู่แล้ว
- เขียนไฟล์ทะเบียน tenant — Pharmacy ใช้ Supabase project ref ไหน, Cloud Run service อะไร, subdomain อะไร
- ล็อกอินพร้อมกันจากสองเครื่องจริง ทดสอบว่าไม่ดีดกัน (อย่าทดสอบในเบราว์เซอร์เดียว มันจะชนแน่นอนและไม่ใช่ของจริง)

## ข้อจำกัดเวลา

ให้เวลา Cloud Run 2 วัน ถ้ายังไม่ผ่าน กลับไป Render ทันที แล้วค่อยย้ายทีหลัง

ลูกค้าโอนเงินเต็มจำนวนมาแล้วด้วยความเชื่อใจ การส่งมอบช้าเพราะกำลังเรียน platform ใหม่คือการเอาความเชื่อใจนั้นไปเสี่ยงโดยไม่จำเป็น Render ยังเป็นทางถอยที่ดีเสมอ
