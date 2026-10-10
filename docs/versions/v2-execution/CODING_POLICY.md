# Coding policy · 2026-10-09

**คำสั่ง 2026-10-01:** พักงานที่เชื่อม V1 เช่นคืนซื้อ/ปรับต้นทุน กลับรายการเงิน เอกสาร รพ.สต./อย. และการย้าย writer/cutover อนุญาต test/build/migration/seed และ checks ที่จำเป็นได้ทันที โดยใช้ environment แยกและไม่รันซ้ำหากไม่มีการเปลี่ยนหรือปัญหาใหม่ ดูผลที่ [VERIFICATION_2026-10-01.md](VERIFICATION_2026-10-01.md)

คำสั่ง coding-first ก่อนหน้านี้เลื่อน test/build/lint/typecheck/migration/seed ไป P3; ข้อห้ามรันนั้นถูกยกเลิกด้วยคำสั่งล่าสุด ส่วนข้อห้ามแก้ V1 และการไม่ติ๊ก acceptance โดยไม่มีหลักฐานที่ครบยังมีผล

รักษา source และพฤติกรรม V1 โดยเฉพาะ month-end ที่ลูกค้าอนุมัติแล้ว ไม่แก้ module/migration/หน้าจอธุรกรรม V1 เพิ่มเติม ยกเว้น Pro gate/เมนูและทางเข้าหน้าเอกสารที่ผู้ใช้อนุญาตชัดเจนเมื่อ 2026-10-09 งาน V2 ที่มีอยู่ก่อนคำสั่งนี้คงไว้โดยไม่ reset งานผู้ใช้

เพิ่ม entrypoint `backend/cmd/api-v2`, API `/api/v2`, UI `/v2` และตาราง `v2_*` แยกจากธุรกรรม V1 ใช้ catalog/branch/user เดิมแบบอ่านเท่านั้น V2 pilot ledger ไม่ใช่ยอดรวม V1+V2 และไม่ sync สต๊อกอัตโนมัติ การย้ายยอดและเปลี่ยน writer จริงต้องมี cutover ใน P3 ก่อนเปิดขายบนยอดเดียวกัน P1-13 จึงยังไม่ถือว่าครบจากการมี V2 reservation อย่างเดียว

P0-05 ใช้ immutable V2 money events และรายงาน V2; ไม่เปลี่ยน month-end V1 หรือสร้างข้อสรุปว่า payment เดิมไม่เคยถูกปรับ ข้อมูล V1 ที่นำมาใช้ภายหลังต้องระบุเวลานำเข้าและแหล่งข้อมูล

P3 เป็นช่วงตรวจรวมที่เพิ่มตามคำสั่งล่าสุด ไม่มีฟีเจอร์ธุรกิจใหม่ใน P3 แผน V2 มี P0/P1/P2 และ P3 verification รายการธุรกิจ 59 ข้อ + เกณฑ์ข้ามฟีเจอร์ 12 ข้อ = 71 ข้อ (P0 14, P1 22, P2 23)

Cost method ต้องตั้ง FIFO หรือ Moving Average ต่อสาขาก่อนรับ stock V2 และห้ามเปลี่ยนเมื่อมี movement แล้ว Cash drawer V2 ใช้ต่อบัญชี หนึ่งรอบเปิดต่อบัญชี และส่งต่อรายการค้างกะถัดไป ตาม decision 2026-10-09 โดยยังไม่ผูกกับ payment V1; implementation ยังรอ business UAT ใน P3

## Update 2026-10-09

Continue V2 with the confirmed refund/account-shift/temporary-Pro decisions in [DECISION_LOG.md](DECISION_LOG.md). Month-end remains deferred. Necessary isolated tests/build/migration/seed/restore and focused browser checks are authorized; rerun only when a changed implementation or failure justifies it.
