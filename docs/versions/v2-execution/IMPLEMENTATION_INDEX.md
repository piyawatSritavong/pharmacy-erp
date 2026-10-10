# V2 additive implementation · updated 2026-10-09

โค้ดรอบนี้เพิ่มเฉพาะไฟล์ V2 ใหม่ ไม่แก้ V1/month-end เพิ่ม หลังผู้ใช้อนุญาตตรวจแล้ว มี build/lint/fresh migration/seed และ V2 integration บน environment แยก ดู [ผลล่าสุด](VERIFICATION_2026-10-01.md) งานเชื่อม V1 พักไว้ตามคำสั่ง

## ไฟล์สำหรับทำต่อโดยไม่อ่านโปรเจกต์ซ้ำ

- `backend/cmd/api-v2/main.go`: entrypoint V2, port 8082 default, worker opt-in, shutdown
- `backend/internal/v2/service.go`: JSON command/idempotency, branch scope, safe integer cents/base units, unit snapshots
- `backend/internal/v2/router.go`: owner-only pilot API `/api/v2`; ใช้ JWT/session เดิม อ่าน user/branch/catalog V1
- `backend/internal/v2/inventory.go`: branch policy, receipt/landed-cost, FEFO/FIFO/Moving Average, reservations, lot states, write-off, instant transfer
- `backend/internal/v2/lot_workflows.go`: quarantine split และ recall ทั้ง lineage
- `backend/internal/v2/shipments.go`: dispatch/partial receive/return unreceived quantities, in-transit trace/value
- `backend/internal/v2/counts.go`: immutable snapshot/observations, ledger sequence checkpoint, single approval/adjustment
- `backend/internal/v2/returns.go`: partial sales return จาก source movement/cost/unit snapshot เข้ากักกัน/recall และ lot trace
- `backend/internal/v2/finance.go`: customers, AR/AP/quotations, price rules, partial payments/allocations/cheques, credit notes
- `backend/internal/v2/quotations.go`: immutable quotation revision chain
- `backend/internal/v2/promotions.go`: percent/amount/buy-x-get-y/bundle/bill-giveaway, branch/central policy, immutable versions/document rule snapshots; เลือกหนึ่งโปรต่อเอกสาร
- `backend/internal/v2/approvals.go`: owner discount grant 15 นาที, cart+catalog/unit/price-rule/promotion/policy fingerprint, consume ครั้งเดียว
- `backend/internal/v2/credits.go`: negative invoice balance → apply same counterparty or cash/bank refund, no automatic stock return
- `backend/internal/v2/reports.go`: effective-date opening/movements/closing/aging; inventory ledger reconciliation; allocated expiry risk vs realized write-off
- `backend/internal/v2/operations.go`: expiry task lifecycle และ cash drawer per branch+owner
- `backend/internal/v2/jobs.go`: transactional outbox claim/lease/retry/backoff/dead-letter, internal notifications, expiry catch-up
- `backend/migrations/066_v2_workspace.sql`–`071_v2_discount_approvals.sql`: `v2_*` schema เท่านั้น apply ผ่านเฉพาะ fresh test DB; ยังไม่ apply ฐานใช้งานจริง
- `frontend/src/app/api/v2/[...path]/route.ts`: BFF ต้องตั้ง `BACKEND_V2_INTERNAL_URL` แยก ไม่มี fallback เข้า API V1
- `frontend/src/app/(app)/v2/page.tsx`: pilot เฉพาะ Superadmin
- `frontend/src/components/v2/workspace.tsx`, `workflows.tsx`, `document-tools.tsx`, `promotions.tsx`: responsive forms/cards, customer create/edit, count/return/shipment/pricing/promotions/discount approval/credit/report/job UI และ print preview
- `frontend/src/services/v2.ts`: client API; mutation key คงเดิมเมื่อ retry หลังคำขอที่ไม่ทราบผล

## ข้อกำหนด implementation candidate

- Base quantity/conversion เป็นจำนวนเต็มบวก เก็บ money เป็น cents; FEFO expiry ascending/received time/ID; ไม่มี expiry ใช้ท้ายแถว; expires today ยังจ่ายได้ ส่วนก่อนวันนี้ห้ามจ่าย
- วิธีต้นทุนตั้งต่อสาขาก่อน stock writer; ห้ามเปลี่ยนหลังมี movement Physical FEFO แยก valuation FIFO/Moving Average
- Landed cost: ส่วนลดท้ายบิลแบ่งตาม net value ที่เหลือ ค่าส่ง/ภาษีที่เลือก capitalize แบ่งตาม base quantity; เศษไปบรรทัดท้ายและไม่ทำต้นทุนติดลบ
- VAT candidate ปัดลงต่อบรรทัดเป็น cents; ต้องยืนยัน rounding policy และตัวอย่างภาษีใน P3 ก่อนยอมรับ
- Price rule ก่อน cashier discount ก่อนโปรที่เลือกก่อน VAT; เฉพาะ manual discount ตรวจ branch ceiling หรือใช้ owner grant; promo ส่วนกลาง/สาขามี rule version history; pilot ไม่ stack หลายโปรและไม่มีคูปอง
- ของแถมเป็น document line ราคา 0 โดยยัง issue stock/cost ปกติ; quote conversion คง snapshot ไม่คำนวณโปรซ้ำ; revision คำนวณราคา/โปรใหม่และไม่นำ giveaway เดิมมาแถมซ้ำ
- Approval fingerprint ผูก payload/date/customer/policy/master products+units/price rules/promotion; เปลี่ยนข้อมูลหรือหมดเวลาใช้ไม่ได้ การอนุมัติส่วนลดไม่ยกเว้น credit limit และไม่ใช่ business sign-off ของรูปแบบเอกสาร
- ล็อตคืน/คืนส่วนค้างการโอนเข้ากักกัน; origin ที่ recalled ทำให้ receipt ลูก recalled ด้วย; money correction ไม่เกิดจาก stock return อัตโนมัติ
- Partial quarantine เปลี่ยน physical lots แต่ไม่เปลี่ยนยอดฐานหรือต้นทุนบัญชี Cost layers เป็น valuation แยกจาก physical lot
- Recall family ตาม origin chain ของ receipt root; ยังไม่ถือว่า recall ครบทุก receipt ที่ใช้ lot number เดียวกันต่าง roots
- Snapshot count เทียบ current expected ตอนบันทึกจำนวน; checkpoint กัน movement หลังบันทึก แม้ยอดกลับมาเท่าเดิม ต้องยกเลิก/นับใหม่
- เอกสารออกวันนี้; quotation conversion คงราคา/หน่วย/ภาษี snapshot; revision สร้างเอกสารใหม่เก็บฉบับเดิมและ chain; print เป็น preview ยังไม่มี company/legal form sign-off
- เครดิตใช้ยอดติดลบของ invoice/bill เดิมเท่านั้น แยกจากเงินที่ยังไม่ allocate; apply ต้องคู่ค้า/สาขา/AR หรือ AP เดียวกัน
- เช็คยังไม่ลดหนี้จน cleared; bounced หลัง clear append กลับหนี้; duplicate bank+number ต่อสาขาถูกกันตาม candidate policy
- Cash drawer ใช้ branch+actor; cash payments/refunds และ cash-in/out append events; ส่วนต่างปิดรอบต้องมีเหตุผล
- Expiry ใช้ PostgreSQL calendar months ใน Asia/Bangkok, unique lot+band; job catch-up hourly หลังเปิด worker ไม่มี LINE/SMS จริง
- Inventory risk ใช้ carrying value ของ product จาก cost policy กระจายตาม physical quantity พร้อมเศษ ต้นทุน write-off มาจาก movement จริง; historical physical lot cost estimate ในหน้า expiry ระบุว่าเป็น estimate

## ยังต้อง coding ก่อน P3

ดูช่องส่วนที่ยังขาดใน [TASK_BOARD.md](TASK_BOARD.md): stacking policy (full promotion editor added 2026-10-09), opened-pack tracking, supplier return/price-only credits/cost adjustment/receipt linkage, payment/credit void/reversal, actual company/รพ.สต. print aliases, อย. export fields และ role/entitlement onboarding ยังไม่ครบ ห้ามรายงานว่า P0/P1/P2 เสร็จแล้ว

P0 ด้าน secrets/legacy session/CI/isolation/restore/runtime และการรักษาข้อมูลเงิน V1 ยังมีงานค้างเดิม คำสั่งล่าสุดอนุญาต checks ที่จำเป็นแล้ว รายการผ่านในรายงาน verification ใช้เฉพาะขอบเขตที่ตรวจ ไม่ใช้ผลเดิมปิด acceptance ทั้งหมด

## เงื่อนไขเปิดใช้ภายหลัง

V2 เป็น pilot ledger ใหม่ ไม่ใช่ source of truth ของสินค้าจริงที่ V1 ใช้อยู่ ห้ามนำยอด V1 และ V2 มารวม หรือปล่อยสอง writer บน goods เดียวกันโดยไม่มี cutover/reconciliation การทำ P1-13 ครบกับ V1 ต้องออกแบบทางย้าย writer ภายใต้ข้อห้ามแก้ V1 และให้ผู้ใช้เห็นผลกระทบก่อนเปิดจริง ไม่มีการย้ายข้อมูล/deploy/commit/push ในรอบ coding นี้

การตั้งค่า runtime เมื่อถึงช่วงที่อนุญาต: `V2_HTTP_PORT`, `BACKEND_V2_INTERNAL_URL` และ `V2_JOBS_ENABLED=true` เฉพาะเมื่อใช้ worker ไม่มี auto-migration/auto-seed ใน entrypoint

## October 9 additions

- `cancellations.go` + migration 072: immutable cancellation/remaining credit facts, pending refund queue, explicit stock return, cash/bank/equivalent-goods settlements with stock and value snapshots. Cancelled invoice refunds must use this queue; generic credit use cannot bypass it. Cheque-linked active/cleared invoices are held for bank reconciliation.
- `shift_warnings.go`, `operations.go`: one open shift per account; derived current expected cash; carried pending refunds and unresolved variance; current-warning acknowledgement before close; append-only acknowledgement and superadmin variance resolution.
- `finance.go`: positive debt counted per invoice for credit exposure. A refund liability cannot mask another outstanding invoice.
- `service.go`: account/branch finance serialization prevents close/acknowledgement racing a new refund or payment.
- `refund-shifts.tsx`: cancellation review, full/partial cash/bank/goods refund forms, drawer chooser, carried warnings and shift close forms.
- `line-editor.tsx`, `document-tools.tsx`, `promotions.tsx`, `workflows.tsx`: shared line editor, multi-product shipment, complete quote revision and promotion rule editor. Explicit V2 form control labels support screen readers and browser checks.
- `pro-access.ts`, sales menu/Pro wrapper/sidebar: temporary entitlement gate and badge removal. Owner-only V2 permission remains; deferred pages display development status.
- `refund_shifts_integration_test.go`, `workflows_integration_test.go`, `tests/v2-refunds.spec.ts`, `playwright.v2.config.ts`: money/stock/concurrency/shift invariant and real-browser checks. Latest evidence: [VERIFICATION_2026-10-09.md](VERIFICATION_2026-10-09.md).
