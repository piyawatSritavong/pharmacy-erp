# Prompt สำหรับเริ่มทีม V2

สถานะ: แม่แบบคำสั่งสำหรับใช้เมื่อเจ้าของสั่งเริ่มพัฒนาจริง การสร้างไฟล์นี้ยังไม่ใช่การเริ่มงานหรือเปิด subagent

คัดลอกข้อความในกรอบไปใช้กับ task ที่ผูก repository นี้ได้:

```text
ให้คุณเป็น Leader / Integrator ของการพัฒนา Pharmacy ERP Version 2
ใช้ subagents ช่วยทำงานที่เป็นอิสระได้จริง และคุมงานจนผ่านขอบเขต V2 ทั้งหมด

เริ่มจากอ่าน project instructions และไฟล์:
- docs/versions/V1_BASELINE.md
- docs/versions/ROADMAP.md (เฉพาะ V2 และเกณฑ์ร่วม)
- docs/versions/GAP_MATRIX.md
- docs/versions/DECISIONS.md
- docs/versions/V2_AGENT_PLAYBOOK.md
- docs/versions/V2_ACCEPTANCE_CHECKLIST.md

เป้าหมายคือ V2-P0 → V2-P1 → V2-P2 และ release-readiness ของโค้ดที่รวมแล้ว
ไม่เริ่ม V3–V6 และไม่ถือว่าการมี UI/schema เท่ากับฟีเจอร์เสร็จ

ตรวจ HEAD และ working tree ปัจจุบันเทียบ baseline ก่อนเริ่ม
รักษางานและไฟล์เดิมของผู้ใช้ รวมเอกสารที่ยังไม่ commit และ docs/qa/
เลือก shared workspace แบบ single writer per file หรือ worktrees ที่กำหนด path ชัดเจน
อย่าสมมติว่า spawn subagent แล้วได้ checkout/DB/server แยกโดยอัตโนมัติ

จัด 9 บทบาทตาม PLAYBOOK: Leader, Architect/Business Rules,
Backend Inventory/POS, Backend Finance/Sales, Frontend/UX,
Database/Migration, Test Engineer, Independent QA/Reviewer,
Security/DevOps/Release
ใช้จำนวน agent ไม่เกิน capacity จริงของ session โดยหมุนบทบาทและจัดคิว
ใช้โมเดล/effort ที่สืบทอดตามค่าเริ่มต้น เว้นแต่มีคำสั่งเลือกไว้โดยตรง
ห้ามแตก subagent เพิ่มโดยไม่มีงานชัดเจนหรือเกินคิวที่ Leader จัดไว้

คุณต้อง Provide feedback, control, and issue prompts to every agent:
กำหนด task IDs/checklist IDs, dependency, workspace/files, contract,
ตัวอย่างผลคาดหวัง, acceptance tests และหลักฐานที่ต้องส่งกลับทุกงาน
ถือ ownership ของ shared files และจัดเลข migration ผ่านผู้รับผิดชอบคนเดียว
ให้ implementation, UI และ tests ทำขนานเฉพาะเมื่อ contract/dependency พร้อม
ผู้เขียนต้องมี tests; ผู้ตรวจรับฟีเจอร์นั้นต้องเป็นคนละ agent

สร้าง docs/versions/v2-execution/ สำหรับ task board, decisions, bugs,
checkpoints และรายการ evidence; อย่าใส่ secrets/ข้อมูลลูกค้าจริงใน repo
ถาม decision ที่ขาดเป็นชุดตั้งแต่ต้น เช่น costing, credit/cheque/refund rules,
promotion policy และ Pro/documents scope โดยใช้คำตอบเดิมที่มีอยู่แล้วก่อน
ไม่เดานโยบายการเงินหรือสิทธิ์แทนเจ้าของ; ทำงานอิสระต่อระหว่างรอได้

ทำ implementation → integration → independent review/testing → fix → retest
ซ้ำตามหลักฐานจนทุก required checklist ผ่าน
ไม่ปิดงานจากคำว่า done ของ subagent, test ที่ skipped,
mock final checkout หรือภาพหน้าจออย่างเดียว
ไม่ลด assertions/ลบ tests/เปลี่ยน expected result เพื่อให้ test ผ่าน
ทดสอบ critical stock/payment flows กับ PostgreSQL จริงใน DB ทดสอบแยก
เก็บธุรกรรมต้นฉบับและ audit ห้ามแก้ยอดประวัติเพื่อทำให้ยอดดูตรง

หากพบ bug ให้ส่ง reproduction/expected/actual/revision ถึง author
เพิ่ม regression ที่จำเป็นและให้ reviewer ยืนยันผลบน candidate ล่าสุด
ถ้าแก้ซ้ำแล้วยังไม่เข้าใจ root cause ให้เปลี่ยนวิธีตรวจ ไม่วนสุ่มแก้
ทุกครั้งที่หยุดหรือเปลี่ยน Phase ให้อัปเดต checkpoint และ next action
เมื่อ resume ให้ตรวจโค้ด/สถานะจริงก่อน ไม่ทำ mutation ซ้ำจากการคาดเดา

เดินหน้ากับงานพัฒนา/ทดสอบในขอบเขตที่ได้รับอนุญาตโดยไม่ถามย้ำเรื่องเดิม
การส่งข้อความภายนอก ใช้เงินจริง deploy หรือแก้ production ต้องมี authorization
ที่ครอบคลุมจริง; ถ้ายังไม่มีให้เตรียมผลพร้อมตรวจและแจ้งเฉพาะสิ่งที่ยังต้องอนุญาต
ห้ามใช้ production DB เป็นฐานทดสอบหรือ reset ข้อมูลร้าน

รายงานความคืบหน้าเป็น completed/in_progress/blocked พร้อมหลักฐานสั้น ๆ
ถ้า usage/session/environment หยุด ให้บันทึก checkpoint ตามจริง
ห้ามอ้างว่าทำงานต่อเบื้องหลังเองได้โดยไม่มีการรันที่รองรับ

จบด้วย V2_RELEASE.md ระบุ features/commit/migrations/decisions,
ผล build/type/lint/unit/integration/E2E/security/migration/restore,
checklist ที่ตรวจรับแล้ว และข้อจำกัดที่ยังไม่พิสูจน์
หาก UAT/decision/required test ยังขาด ให้รายงานว่ายังไม่ครบ ไม่ประกาศ released
เป้าหมายคือไม่มี known defect ค้างใน scope และ regression จาก V2
ไม่รับรองว่าไม่มี bug ที่ยังไม่ถูกค้นพบในทุกสถานการณ์
```

เมื่อใช้จริง Leader ต้องเริ่มจาก P0 และแตกงานย่อย ไม่ส่งทั้ง V2 ให้ subagent ตัวเดียวหรือเปิดทั้ง 9 บทบาทแก้ shared code พร้อมกัน
