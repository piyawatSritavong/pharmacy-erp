# V2 task board

## Current scope · 2026-10-09

Read Basic Memory project `pharmacy-erp` and rechecked the working tree at HEAD `5e4ad86`. The four business questions are now three confirmed decisions and one explicit deferral; see [DECISION_LOG.md](DECISION_LOG.md). Month-end and V1-connected work stay deferred. Migration 072 adds only V2 evidence.

Added: paid/unpaid V2 sales cancellation, cash/bank/equal-value goods refund queue, explicit stock return to quarantine, retry/over-refund protection, account shifts with carried refund/variance warnings, stale acknowledgement protection and superadmin variance resolution. Pro gate/badge is temporarily disabled; sales menu links to the owner-only V2 pilot. Quote and promotion editors now support full line/rule changes; shipment UI now supports multiple products. Release acceptance is still incomplete.

## Implementation baseline · 2026-10-01 (updated with October 9 work)

คำสั่งล่าสุดอนุญาต checks ที่จำเป็นและพักงานเชื่อม V1 ผล build/lint และ V2 DB integration อยู่ใน [VERIFICATION_2026-10-01.md](VERIFICATION_2026-10-01.md) สถานะในตารางยังเป็น candidate ไม่ใช่ผ่าน acceptance/UAT เต็มชุด

**พักตามคำสั่งผู้ใช้:** supplier return/cost adjustment ที่เชื่อม V1, payment/credit reversal ที่เชื่อม V1, เอกสาร รพ.สต./อย., writer migration/cutover และ V1 financial-original reconciliation ห้ามเริ่ม coding/integration/deploy ส่วนนี้ต่อเอง คง source เดิมและไม่เปิด writer บนสต๊อกจริงร่วมกัน

| งานเพิ่มแบบ V2-only | Checklist ที่เกี่ยวข้อง | สถานะ | ส่วนที่ยังขาด |
| --- | --- | --- | --- |
| API/ledger/UI แยก namespace; immutable events; cash drawer | P0-05/13/14 | coding candidate | V1 original-money reconciliation, entitlement matrix, correction/void และ UAT |
| หน่วย snapshot, FEFO, FIFO/Moving Average, landed cost | P1-01/02/03/05/06/16/17 | coding candidate | หน่วยซื้อ/แพ็กเปิดจริง, business policy, เอกสาร V1/cart integration และ PO cost adjustment |
| กักกันเต็ม/บางจำนวน, recall lineage, write-off, คืนเข้ากักกัน | P1-07/08/09/10; P2-19 | coding candidate | ผู้อนุมัติแยกคน, period rules, supplier return/claim และ recall ตามเลขล็อตข้าม receipt roots |
| จอง/release/consume/expire | P1-11/12/13/14 | partial | writer V1 ยังไม่ใช้ V2 ledger; ห้ามเปิดขาย stock ชุดเดียวพร้อมกันก่อน cutover |
| โอนส่ง/ระหว่างทาง/รับบางส่วน/คืนส่วนค้าง | P1-02/14 | coding candidate | discrepancy/สูญหายระหว่างทาง; UI หลายสินค้าต่อใบเพิ่มแล้ว 2026-10-09 |
| ตรวจนับ snapshot/count/approval/checkpoint | P1-15 | coding candidate | workflow ผู้ตรวจ/ผู้อนุมัติแยกคนและ physical-count timing UAT |
| Transactional outbox, lease/retry/dead-letter, internal notification | P1-19 | coding candidate | crash/restart evidence, operational alerting และ backlog budget |
| Expiry bands/tasks/carrying-value risk/write-off/report reconcile | P1-20/21/22 | partial | filters/date range/export, ตรวจนโยบายกระจาย carrying value และ scheduler/runtime proof |
| ลูกค้า create/edit/credit terms/price rules/quantity break/quotation revision | P2-01/03/04/05/09 | coding candidate | credit approval/hold; full quote editor เพิ่มแล้ว 2026-10-09 และรูปแบบพิมพ์บริษัทจริง |
| AR/AP/payment/หลาย allocation/cheque state/credit notes | P2-10/11/12/14/15/16/17 | partial | receipt linkage, supplier price-only credit/cost correction, approval และ reversal |
| ใช้เครดิต/คืนเงิน/as-of statement/aging | P2-13/20/21 | coding candidate | reverse/void/refund evidence, export, settlement reconciliation และ business period rules |
| Branch/central promotion 5 types + rule snapshots, cart-bound discount approval | P2-06/07/08 | partial coding candidate | ใช้หนึ่งโปรต่อเอกสาร; stacking policy/role matrix (full promo editor เพิ่มแล้ว 2026-10-09) และเทียบ V1 ทั้ง 5 แบบยังขาด |
| รพ.สต. alias + เอกสาร และ อย. summary/export | P2-22/23 | pending coding + business samples | ต้องยืนยันตัวอย่าง/fields ก่อนให้เป็นเอกสารใช้งานจริง |

## P0 รอบก่อนคำสั่ง coding-first

| Task | Checklist | Owner | Status | Evidence / next gate |
| --- | --- | --- | --- | --- |
| P0-L1 baseline + ownership | 01 | Leader | in review | execution docs, dirty-tree record; Q ต้องตรวจว่าไม่ทับ user changes |
| P0-C1 canonical cart contract | 02 | Leader/checkout audit | in review | unit conversion, header/discount และ remote payment contract มี DB integration; frontend E2E/UAT ยังขาด |
| P0-C2 parked round-trip | 03 | Leader | in review | migration 064, claim/consume ใน checkout transaction, owner/token/expiry recovery และ DB integration ผ่าน; refresh/price-stock change E2E ยังขาด |
| P0-C3 remote atomic/idempotent | 04 | Leader | in review | shared transaction, row lock, stored hash/result, owner/version CAS, serialized autosave และ PostgreSQL concurrency integration ผ่าน; frontend race E2E ยังขาด |
| P0-F1 immutable receipt semantics | 05 | finance audit | blocked by business decisions | month-end แก้ payment เดิม; ดู `DECISION_LOG.md` |
| P0-Q1 V1 regression | 06 | quality audit | pending candidate | unit baseline มี แต่ required DB/E2E/mobile/UAT รอบเดียวกันยังขาด |
| P0-S1 session revocation | 07 | quality audit | in review | auth_version + current DB permission check และ integration สำหรับ version/inactive ผ่าน; role/branch/reset API cases ยังขาด |
| P0-S2 secret handling | 08 | Leader/quality audit | in progress | JWT ออกจาก JSON response ไป HttpOnly cookie; audit redaction ดีขึ้น; plaintext marketplace credential/rotation/artifact policy ยังขาด |
| P0-S3 CI | 09 | quality audit | implementation candidate | เพิ่ม GitHub Actions สำหรับ Go, frontend, PostgreSQL integration; ยังไม่ผ่าน CI จริงและ E2E gate ยังขาด |
| P0-S4 isolated tests | 10 | quality audit | in progress | OpenTest บังคับ APP_ENV=test และชื่อ DB ลงท้าย _test; full DB suite ผ่านบน DB แยก แต่ยังไม่มี per-run DB isolation ทุกชุด |
| P0-S5 restore drill | 11 | Leader | isolated restore passed | local snapshot restored into a separate test DB; row counts/full-row hashes match for 18 V1/V2 evidence tables; production recovery procedure/RPO/RTO remains unverified |
| P0-S6 runtime/mobile | 12 | quality audit | partial | health/migration มี; config/storage/device/UAT ยังขาด |
| P0-F2 cash drawer | 13 | Leader | tested candidate | owner confirmed account scope; V2 cash/refund/shift carry implemented and isolated integration checked; business UAT pending, V1 reconciliation deferred |
| P0-A1 Pro/legacy inventory | 14 | Leader | candidate | owner authorized temporary Pro gate/badge removal; role permissions remain; deferred modules show honest pending states |

File ownership รอบนี้: Leader เป็น writer ของ shared sales/parked/http/frontend/migration และ execution docs; audit agents เป็น read-only ผู้ตรวจ จึงไม่มี concurrent writer ชนกัน
