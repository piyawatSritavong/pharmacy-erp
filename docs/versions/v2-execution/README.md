# V2 execution record

เริ่ม 2026-09-22 จาก revision `43e5ad4` บน branch `master` ซึ่ง track `origin/main` โดยรักษางานเดิม `README.md`, `docs/qa/` และ `docs/versions/` ไว้ทั้งหมด งานรอบแรกจำกัดที่ V2-P0 และยังไม่มีการ deploy หรือเปลี่ยนฐาน production

เอกสารชุดนี้เป็นสถานะระหว่างพัฒนา ไม่ใช่ release report และยังไม่ถือว่า P0 หรือ V2 ผ่าน ดูเกณฑ์จริงที่ [V2_ACCEPTANCE_CHECKLIST.md](../V2_ACCEPTANCE_CHECKLIST.md)

**2026-10-01 · คำสั่งล่าสุด:** พักงานเชื่อม V1 และอนุญาตตรวจเท่าที่จำเป็น ผ่าน build/type/lint, fresh migrations+seed บน DB แยก และ V2 workflow integration พร้อม race detector ดู [VERIFICATION_2026-10-01.md](VERIFICATION_2026-10-01.md) ยังไม่ถือว่าครบ acceptance/UAT ผล verification เก่าในส่วนท้ายเป็นประวัติคนละ candidate

- [TASK_BOARD.md](TASK_BOARD.md) — เจ้าของงาน สถานะ dependency และหลักฐาน
- [CART_CHECKOUT_CONTRACT.md](CART_CHECKOUT_CONTRACT.md) — contract ของ POS/Admin/parked/remote
- [DECISION_LOG.md](DECISION_LOG.md) — decision ที่ใช้ได้และ decision ธุรกิจที่ยังขาด
- [BUGS.md](BUGS.md) — finding ที่ต้องปิดก่อน gate
- [CODING_POLICY.md](CODING_POLICY.md) — ข้อห้ามแก้ V1 และการเลื่อน verification
- [IMPLEMENTATION_INDEX.md](IMPLEMENTATION_INDEX.md) — source map ของ V2-only และ coding backlog

## Environment รอบเริ่มต้น

| ส่วน | สถานะ |
| --- | --- |
| source | local working tree; มี user changes เดิม ห้าม reset/clean |
| backend unit/non-DB | `go test ./...` ผ่านหลัง candidate |
| frontend | `npm run lint` ผ่านหลัง patch แรก |
| test DB | รอบทดสอบก่อนหน้าบน PostgreSQL 17 ชั่วคราวที่ `/tmp`, port 55482, DB `pharmacy_v2_candidate_test`: migration 001–065 + fresh seed และ full DB suite ยกเว้น fresh harness ผ่าน; server ชั่วคราวไม่ได้รันอยู่ ณ 2026-10-01 |
| browser/mobile | ยังไม่รัน candidate รอบใหม่; หลักฐานเดิมเป็น emulation และไม่ใช้ปิด P0 |
| production | ไม่แตะ |

## Gate ปัจจุบัน

P0-02/03/04/07 อยู่ใน review; P0-09 มี workflow candidate แต่ยังไม่เคยผ่าน GitHub CI จริง; P0-05/08/10/11/12/13/14 ยังมี finding หรือ decision ค้างตาม task board จึงยังไม่ติ๊ก checklist ข้อใดผ่านจนมี independent review และ UAT บน candidate เดียวกัน

## Verification ล่าสุด

- `cd backend && go test ./...` — pass (DB tests skip เมื่อไม่มี URL)
- `APP_ENV=test TEST_DATABASE_URL=<isolated-v2-db> go test -count=1 -p 1 $(go list ./... | grep -v '/tests$')` — pass บน PostgreSQL 17, รวม auth, parked, remote concurrent checkout, multi-unit และ migrations
- `backend/tests` ต้องรันบนฐานว่างแยกจาก seeded suite; CI workflow เตรียมฐานนั้นไว้ แต่ยังไม่ได้ยืนยันผล CI จริง
- `cd frontend && npm run lint` — pass
- `cd frontend && npm run build:check` — pass; มี SWC optional-binary warning แต่ compile/type/static generation สำเร็จ
- `git diff --check` — pass
