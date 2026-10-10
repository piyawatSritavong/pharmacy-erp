# Open findings · V2-P0

| ID | Severity | Checklist | Finding | State |
| --- | --- | --- | --- | --- |
| P0-B001 | critical | 04 | remote checkout เดิมสร้าง sale และปิด session คนละ transaction | fixed in candidate; PostgreSQL concurrency test passed; independent review pending |
| P0-B002 | critical | 02/04 | remote cart ทำ unit ID หาย ทำ conversion ผิด | fixed in candidate; multi-unit DB test passed, E2E pending |
| P0-B003 | high | 03 | parked bill ทิ้ง line/bill discount และ unit | fixed in candidate; DB round-trip passed, E2E pending |
| P0-B004 | high | 03 | resume ลบ parked bill ก่อน checkout สำเร็จ | fixed in candidate by claim/consume; DB test passed, refresh/crash E2E pending |
| P0-B005 | high | 04 | delayed autosave เปิด session เก่าซ้ำหลัง completed | guarded by version CAS and serialized autosave; E2E pending |
| P0-B006 | high | 05/13 | month-end แก้ invoice payment ต้นฉบับ ทำให้เงินจริงไม่ immutable | blocked by business decision + finance redesign |
| P0-B007 | high | 07 | JWT เก่าไม่ถูก revoke หลัง password/role/active เปลี่ยน | auth_version/current permissions fixed in candidate; broader API integration pending |
| P0-B008 | critical | 08 | marketplace credentials ถูกเขียน audit และ JWT ถึง browser body | new audit/JWT response fixed in candidate; plaintext storage/old-log rotation pending |
| P0-B009 | high | 09/10 | ไม่มี CI และ DB integration สามารถ skip/ชี้ DB ที่ไม่ปลอดภัย | CI/OpenTest candidate added; CI run/E2E/per-run isolation pending |
