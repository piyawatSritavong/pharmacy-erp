# V2 limited verification · 2026-10-01

ผู้ใช้อนุญาต test/build/migration/seed เท่าที่จำเป็น และสั่งพักงานเชื่อม V1 รายงานนี้เป็น technical evidence บน local working tree ไม่ใช่ release/UAT/การยืนยัน V2 ครบทุกฟีเจอร์

## ผลที่รันจริง

| Check | Command / environment | ผล |
| --- | --- | --- |
| Backend compile + existing non-DB tests | `env -u TEST_DATABASE_URL go test ./...` ใน backend | PASS; DB integration ของแพ็กเกจเดิม skip เมื่อไม่มี URL ไม่ถือว่าผ่าน DB regression |
| V2 executable | `go build -o <temporary-root>/api-v2 ./cmd/api-v2` | PASS |
| Frontend production build/type/lint | `npm run build:check` ใน frontend; output `.next-build` | PASS; route `/v2` และ BFF `/api/v2/[...path]` build ได้ |
| V2 lint เฉพาะไฟล์ใหม่ | eslint `src/components/v2`, `src/app/(app)/v2`, `src/app/api/v2`, `src/services/v2.ts`, max warnings 0 | PASS |
| Fresh migration + base seed | `APP_ENV=test TEST_DATABASE_URL=<isolated-db> go test -count=1 -run '^TestIntegrationHarness$' ./tests` | PASS; migrations 001–071, seeded catalog/users/branches/inventory |
| V2 real PostgreSQL API workflows + race detector | `APP_ENV=test TEST_DATABASE_URL=<isolated-db> go test -race -count=1 -v ./internal/v2` | PASS; 9 top-level scenario groups, including FIFO/Moving Average nested cases |
| Whitespace | `git diff --check` | PASS |

เริ่ม seed ครั้งแรกถูกปฏิเสธเพราะยังไม่ได้ตั้ง password variables; ตั้ง `SEED_ADMIN_PASSWORD` และ `SEED_POS_PASSWORD` เป็นค่าสุ่มแยกสำหรับ test แล้วรันเฉพาะ harness ซ้ำจนผ่าน ไม่มีการแก้ seed V1 หรือเก็บรหัสผ่านในรายงาน

Build มี warning เดิมเกี่ยวกับ native SWC optional binary/fallback และ Browserslist data เก่า ไม่ทำให้ build fail ไม่ติดตั้งหรืออัปเดต dependency เพิ่มในรอบนี้

## DB scenarios ที่ยืนยัน

- GET ทุก route listing, JWT required, non-owner role ถูกปฏิเสธ และ trailing JSON ถูกปฏิเสธ
- รับ stock/unit conversion/landed-cost rounding; retry key เดิมไม่เพิ่ม stock; payload ต่าง key เดิม conflict
- Physical FEFO แยกจาก FIFO/Moving Average: 10@100 + 10@200 จ่าย 12 ต้นทุน 1,400/1,800 ตาม policy และยอดคงเหลือตรงทั้ง account/lot/ledger
- จองสินค้าชิ้นสุดท้ายพร้อมกันได้ผู้ชนะคนเดียว; ของจองจ่ายปกติไม่ได้; consume แล้ว release ซ้ำไม่ได้
- Dispatch/partial receive/replay/เกินจำนวน, partial quarantine และ recall ครอบคลุม descendants
- ตรวจนับปรับ stock ครั้งเดียวและปฏิเสธ stale count หลัง movement
- AR/รับเงิน/allocate หลายครั้งต่อเอกสาร, cheque deposited/cleared/bounced พร้อม debt reversal ภายใน V2, cash drawer ปิดครั้งเดียว
- ใบลดหนี้/ใช้เครดิต/คืนเงินและรับสินค้าคืนอ้างต้นทุนเดิมภายใน V2; cumulative return เกินยอดถูกปฏิเสธ
- โปรโมชั่น 5 types พร้อม expected cents/giveaway และ snapshot; discount ceiling/grant/cart-change/single-use
- Quote conversion รักษาหน่วย snapshot แม้ master conversion เปลี่ยน, expired band/dedup/write-off, outbox drain/expired lease/replay notification dedup
- V2 stock accounts ทุกแถว reconcile กับ physical lots และ signed stock/value events ไม่มี mismatch
- V1 inventory/movements/invoices/items/payments ใช้ hash ทั้งแถวก่อนและหลัง V2 scenarios เท่ากัน; immutable V2 money/document facts ถูกกันการ UPDATE

นี่ไม่ใช่การเปิดงาน V1 integration ที่พักไว้: source business/month-end V1 ไม่ถูกแก้ V1 transaction tables บน test DB ใช้ตรวจไม่ให้ V2 writes กระทบ ส่วน catalog/unit fixture ใช้ข้อมูลจำลองแยกเท่านั้น

## Environment / evidence

- PostgreSQL 17 ใหม่เฉพาะรอบนี้ bind `127.0.0.1:55489`; DB `pharmacy_v2_verify_test`; `APP_ENV=test` และชื่อ DB `_test` ผ่าน guard
- Temporary cluster `/tmp/pharmacy-v2-check.mdf05i` ไม่ใช้ DATABASE_URL จากฐานร้านจริง; ปิด server หลังตรวจเสร็จ ไม่มี deploy/production migration/production seed
- หลัง workflow fixtures: migrations 71, V2 stock events 42, money events 24, documents 12, operations 93, notifications 66
- Source test: `backend/internal/v2/workflows_integration_test.go`; fixtures IDs แยกใหม่ต่อ scenario ไม่ truncate/reset ฐานผู้ใช้
- Candidate SHA-256 manifest: `VERIFICATION_2026-10-01.sha256`; source ที่ตรวจเป็น working tree มีงานค้างก่อนรอบนี้ ไม่ใช่ clean committed release

## ไม่ได้ตรวจ / พักไว้

ไม่ได้รัน E2E/mobile browser/device UAT, restore drill, performance, CI จริง, upgrade พร้อมข้อมูลธุรกรรม V1 เดิม หรือ full existing PostgreSQL regression suite ไม่ติ๊ก acceptance เป็นครบจากผลชุดนี้

งานเชื่อม V1 ที่พัก: คืนซื้อ/ปรับต้นทุน/กลับรายการเงินที่เชื่อม V1, original-money reconciliation, writer migration/cutover, รพ.สต./อย. code/workflow/API ภายใน V2 ที่มีอยู่ยังคงไว้ ไม่มีการเปิดใช้ร่วมกับยอด stock V1 จริง
