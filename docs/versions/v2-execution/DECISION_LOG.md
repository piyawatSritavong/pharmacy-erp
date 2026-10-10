# V2-P0 decision log

## ใช้ได้ใน implementation รอบแรก

- `session_id` ของ remote cart เป็น idempotency key; payment payload เดิมคืน result เดิม payload ต่าง conflict
- Backend คำนวณ conversion/price/tax/promotion/stock ใหม่ก่อน checkout; snapshot ใน parked/remote ใช้แสดงผล
- invoice/payment/stock/audit/remote completion ต้อง commit หรือ rollback พร้อมกัน
- Admin ที่รับชำระ remote cart ใช้ remote checkout flow เดียวกับสาขา; HQ-pickup ปกติห้าม cancel remote cart อื่น
- migration 064 เป็น additive และข้อมูลเก่า default อย่างปลอดภัย
- audit payload ต้อง redact credential ซ้ำอีกชั้น แม้ caller ควรเลือก field ที่บันทึกอยู่แล้ว

## Confirmed business decisions · 2026-10-09

1. **Month-end:** deferred, preserve V1 code/behavior. Only superadmin may approve when the work resumes. Real-money versus book-adjustment design remains unresolved and is outside the active scope.
2. **Paid invoice cancellation:** refund required; cash, bank transfer or equivalent-value replacement goods. Warn on pending cross-day refunds before closing a shift. V2 cancellation preserves originals and creates a remaining credit note plus a tracked refund liability; physical return requires explicit confirmation and goes into quarantine.
3. **Cash drawer:** per account (one account per branch in the business model), one open shift per account. Pending refunds and unresolved cash variance survive shift close/open. Closing requires acknowledgement of the current warning snapshot; acknowledgement does not resolve an issue. Actual opening cash is entered separately so pending amounts are not counted twice.
4. **Pro:** temporarily disable the entitlement gate and badge. Keep role permissions. The owner will define/reinstate entitlement later. Removing the gate does not complete deferred government/FDA/marketplace workflows.

Only superadmin approves the V2 pilot, stock-count confirmation and cash-variance resolution. No month-end/V1 transaction rewrite is authorized by these decisions.

## Remaining constraints

- V1 supplier return/cost correction/reversal, writer migration/cutover and government/FDA work remain deferred.
- Cheque-linked V2 invoices cannot be cancelled while a received/deposited/cleared cheque remains linked. Clearing can still be followed by a bounce; cancellation requires a separate approved bank reconciliation workflow rather than silently losing that liability.
- VAT rounding, actual company print forms, opened-pack counting and promotion stacking still require business acceptance. Current pilot uses integer base units, floor cents per VAT line and one selected promotion.
