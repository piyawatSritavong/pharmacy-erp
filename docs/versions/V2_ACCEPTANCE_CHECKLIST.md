# V2 — Checklist ฟีเจอร์และเกณฑ์ตรวจรับ

2026-09-22 · สถานะเริ่มต้น: **ทุกข้อยังไม่ยืนยันผ่าน** รายการนี้เป็นการแตกงานจาก V2 ใน [ROADMAP.md](ROADMAP.md) ไม่ใช่ผลการทดสอบที่ทำเสร็จแล้ว

วิธีใช้ร่วมกับ [ทีม agent](V2_AGENT_PLAYBOOK.md): ตัวอักษรท้ายรายการคือบทบาทเจ้าของงานหลัก L=Leader, A=Architect/กฎธุรกิจ, I=Backend Inventory/POS, F=Backend Finance, U=Frontend, D=Database, T=Test Engineer, Q=QA, S=Security/DevOps

ก่อนติ๊กผ่าน **ทุกข้อ** ต้องมี task record: `ID → ข้อกำหนด/decision → implementation/ไฟล์ → test IDs → candidate revision → environment/command/ผลจริง → reviewer → evidence` ผู้เขียนส่ง implementation ได้ แต่ Q/ผู้ตรวจที่ไม่ได้เขียนงานนั้นต้องยืนยันก่อน L ปิดรายการ ทุกข้อที่เป็นฟีเจอร์ผู้ใช้ต้องรวม UI/API/permissions/validation/error states ไม่ผ่านจาก API อย่างเดียว

กรณีใช้ของเดิมได้ ให้บันทึกว่า reuse อะไรและผลทดสอบล่าสุด ไม่จำเป็นต้องเขียนใหม่ แต่ยังไม่ติ๊กผ่านโดยไม่มีหลักฐาน หาก dependency/บัญชี/ข้อมูล/นโยบายขาด ให้เป็น blocked; ห้ามเปลี่ยนเป็น N/A เพื่อทำให้ครบ

## V2-P0 — ยืนยันฐานและแก้ช่องว่างสำคัญ

- [ ] **V2-P0-01** ตรวจ HEAD/working tree/V1 baseline จัด task board, file ownership, test environments และรายการ decision ที่ต้องใช้ก่อนแต่ละงาน; ไม่ทำงานเดิมของผู้ใช้หาย — L/A
- [ ] **V2-P0-02** กำหนด cart/checkout contract ร่วม POS, Admin และ remote: product/lot/unit/conversion/quantity/discount/customer/tax/note; ระบุ source of truth และ backward compatibility — A/I/F/U
- [ ] **V2-P0-03** พัก/เรียกบิลรักษาหน่วย จำนวน ส่วนลดรายการ/ท้ายบิล ลูกค้า ภาษี และ note; ทดสอบ refresh/หมดอายุ/ข้ามบัญชีต่างสาขา/ราคาและstockเปลี่ยน; ไม่สร้าง reservation โดยไม่ตั้งใจ — I/U/T
- [ ] **V2-P0-04** Remote checkout atomic และ idempotent: request เดียวกันพร้อมกัน/ซ้ำหลัง timeout ได้ invoice/payment/stock movement ชุดเดียว; key เดิมแต่ payload ต่างต้องไม่เขียนซ้ำ; session ไม่ค้าง open หลังขายสำเร็จ — I/D/T
- [ ] **V2-P0-05** ยอดขาย/รับเงินจริงต้นฉบับคงหลักฐานและอธิบาย correction/reissue/reconciliation ได้; ตรวจ retained/suppressed/reissued ตามสิทธิ์; report ใหม่ไม่ใช้ยอดที่ถูกแก้แทนเงินจริงโดยไม่มี reconciliation — A/F/D/T
- [ ] **V2-P0-06** Regression V1: POS ทุกวิธีชำระ, ภาษี/เปลี่ยนใบกำกับ, ส่วนลด/โปร, PO/โอน/เคลม/ประวัติและสิทธิ์ไม่เสีย; รวม Admin/POS บน mobile/desktop — T/Q/U
- [ ] **V2-P0-07** เปลี่ยนสิทธิ์/reset password/ปิดบัญชีแล้ว session เก่าใช้ต่อไม่ได้ตามนโยบายที่กำหนด; ทดสอบ cross-branch, role escalation, API โดยตรง และบัญชี Superadmin ขั้นตอนที่เกี่ยวข้อง — S/I/T
- [ ] **V2-P0-08** Password/JWT/connection credentials ไม่ปรากฏใน response, audit, access log, screenshots หรือ test artifact; secret ถูกเก็บ/หมุนตามวิธีที่กำหนด — S/D/T
- [ ] **V2-P0-09** CI รัน build/typecheck/lint/unit/integration และ E2E ที่จำเป็นจาก checkout สะอาด; failure ทำให้ gate ไม่ผ่าน ไม่กลบ error หรือ skip required tests — S/T
- [ ] **V2-P0-10** DB/fixtures/ports/build outputs ของ test แยกจาก production และงานอื่น; data reset จำกัด test target; สร้าง environment ซ้ำได้พร้อม versions/config ที่ไม่มี secret — S/D/T
- [ ] **V2-P0-11** Backup และ restore DB+รูปลง environment แยกได้จริง; ตรวจจำนวน/ยอดสำคัญและความสัมพันธ์ invoice/lot/movement หลัง restore; บันทึกเวลาที่ใช้และแผนกู้คืน — S/D/Q
- [ ] **V2-P0-12** ยืนยัน runtime configuration, health check, migration lifecycle และ storage ที่ environment ส่งมอบ; ตรวจรูปจริงและ mobile device/UAT ที่จำเป็น; รายงานส่วนที่ยังทดสอบได้เฉพาะ emulation — S/U/Q
- [ ] **V2-P0-13** รอบเปิด/ปิดเงินสด: เงินตั้งต้น ยอดรับ/คืนที่ผูกเอกสาร เงินนับจริง ส่วนต่าง เหตุผลและสิทธิ์; กำหนด cash-in/out หากใช้งาน; กันปิดซ้ำและกระทบยอดกับธุรกรรมต้นฉบับได้ — A/F/U/T
- [ ] **V2-P0-14** ตรวจ Pro gate/API entitlement, endpoint/service legacy และเอกสารเก่าที่อ้าง schema ถูกถอด; ตัดสินใจเก็บ/ปรับ/ถอดโดยไม่ทำ consumer ที่ยังใช้เสีย; ระบุรายการเปิดใช้ใน P2 — A/I/F/S

**Gate P0:** T/Q ยืนยันรายการข้างต้นบน candidate เดียวกันก่อนเริ่มงานที่พึ่งพา contract/ยอดเงิน/session ใหม่

## V2-P1 — หน่วย ล็อต FEFO ต้นทุนและการจอง

- [ ] **V2-P1-01** ตกลงหน่วยฐาน/หน่วยซื้อ/ขาย/โอน/คืน, conversion และกฎ integer/fractional ที่ธุรกิจใช้; ปฏิเสธ conversion ศูนย์/ติดลบ/ซ้ำหรือ overflow — A/I/D/T
- [ ] **V2-P1-02** ซื้อ/รับเข้า/ขาย/โอน/คืนแปลงหน่วยเป็นฐานสอดคล้องกันทั้ง UI/API/ledger; ตัวอย่างรับ 20 ขาย 3 โอน 10 เหลือต้นทาง 7 และปลายทางรับ 10 — I/F/U/T
- [ ] **V2-P1-03** เอกสารและ parked/remote flow เก็บ conversion/หน่วย/ราคา snapshot; เปลี่ยน master unit ภายหลังไม่เปลี่ยนเอกสารย้อนหลังหรือคืนสินค้าผิดจำนวน — I/F/D/T
- [ ] **V2-P1-04** แตกแพ็ก/ขายบางหน่วยคงยอดฐานและ trace ล็อต/วันหมดอายุเดิม; กำหนดวิธีนับแพ็กเปิดตาม D02 และแสดงให้ผู้ปฏิบัติงานเข้าใจ — A/I/U/Q
- [ ] **V2-P1-05** ขายปกติ auto FEFO แบ่งข้ามล็อตได้ ตาม expiry/received date/tie-breaker ที่กำหนด; กัน expired/quarantine/reserved; ถ้าล็อตไม่มี expiry ใช้กฎที่กำหนด — I/D/T
- [ ] **V2-P1-06** Override ล็อตได้เฉพาะสิทธิ์ที่อนุญาตพร้อมเหตุผล/audit; preview กับ commit ตรวจความพร้อมใหม่; คนไม่มีสิทธิ์เรียก API ตรงไม่ได้ — I/U/S/T
- [ ] **V2-P1-07** Quarantine/release ทำงานระดับล็อต/จำนวนด้วยเหตุผลและสิทธิ์; สินค้า quarantine ไม่ถูกเสนอขายหรือจอง; ปล่อยซ้ำไม่เพิ่มยอด — I/D/U/T
- [ ] **V2-P1-08** Recall ค้นจากล็อตรับเข้าถึงโอน/ขาย/คืน/สาขาและลูกค้าที่มีข้อมูลระบุตัวตน; บล็อกของที่เรียกคืน; รายงานข้อมูลที่ตามตัวลูกค้าไม่ได้ตามจริง — I/F/U/T
- [ ] **V2-P1-09** Write-off หมดอายุ/เสียหายระบุ lot/qty/cost/reason/ผู้อนุมัติ; ตัด expired lot ได้โดยไม่ใช้ sellable FEFO ผิดประเภท; กันเกินยอด/ซ้ำ/ย้อนหลังข้ามงวดที่ล็อก — I/F/D/T
- [ ] **V2-P1-10** รับของทดแทนจากเคลมและคืนสินค้าทุกเส้นทางมี lot/expiry/cost ถูกต้อง; ของคืนรอตรวจไม่กลับเข้า sellable อัตโนมัติ; รองรับข้อจำกัด Real/Ghost เดิม — I/U/T
- [ ] **V2-P1-11** Reservation มี reference, branch, product/unit/base qty, expiry, state และ audit; กำหนดว่า reserve ระดับ product หรือ lot และใช้เหมือนกันทุก consumer — A/I/D
- [ ] **V2-P1-12** Reserve/release/consume/expire atomic และ idempotent; concurrent จอง/ขายชิ้นสุดท้ายไม่ oversell; release/expire ที่แข่งกับ consume ไม่คืนยอดซ้ำ — I/D/T
- [ ] **V2-P1-13** ทุก writer ที่เกี่ยวข้องเคารพยอดจอง: POS/Admin/remote, transfer, claim, adjustment และ PO correction/cancel; ห้ามลดยอดจนของที่จองไว้หายโดยไม่มี conflict resolution — I/F/T
- [ ] **V2-P1-14** นิยาม on-hand/sellable/reserved/expired/quarantine/in-transit ไม่ซ้อนกันผิด; รายงาน reconcile inventory/lot/movement/reservation หา mismatch ได้โดยไม่แอบแก้ยอด — A/I/F/T
- [ ] **V2-P1-15** รอบตรวจนับมี scope/snapshot/count/difference/reason/approval และ movement ขณะนับ; ยืนยันผลครั้งเดียว; แยกของจอง/ระหว่างทาง/หมดอายุและรักษา audit — I/U/T
- [ ] **V2-P1-16** เลือกและอนุมัตินโยบาย FIFO หรือ Moving Average ก่อน implement; แยก physical FEFO กับ valuation; ทดสอบหลายราคาซื้อ/รับย้อนหลัง/คืน/ต้นทุนศูนย์/การปิดงวดตามกฎ — A/F/I/D/T
- [ ] **V2-P1-17** Landed cost กระจายส่วนลดรายการ/ท้ายบิล/ค่าส่ง/ภาษีที่นโยบายกำหนดไปล็อต โดยมี allocation basis และ rounding remainder; ผลรวมตรงต้นทุนรับเข้าทั้งเอกสาร — A/F/D/T
- [ ] **V2-P1-18** แก้/cancel PO หรือคืนซื้อหลังขาย/โอนบางส่วนแล้วตรวจผลกระทบต้นทุนได้; เก็บ adjustment/snapshot และไม่เขียนทับต้นทุนประวัติโดยไร้หลักฐาน — F/I/D/T
- [ ] **V2-P1-19** Job runner + transactional outbox บันทึก event พร้อม transaction, retry/backoff/deduplicate/dead-letter/monitoring; ทดสอบ crash ก่อน/หลัง commit และ worker restart — I/S/D/T
- [ ] **V2-P1-20** Expiry bands 3/6/9 เดือนปฏิทินตาม Asia/Bangkok; ระบุช่วงไม่ให้นับซ้ำโดยไม่ตั้งใจ; ทดสอบสิ้นเดือน/leap year/วันนี้/ไม่มี expiry/หมดอายุแล้ว/หมดล็อต — I/T
- [ ] **V2-P1-21** หน้าติดตาม expiry มี filter สาขา/ช่วง/ล็อต, ผู้รับผิดชอบ, รับทราบ/ปิดงานและป้องกันแจ้งซ้ำ; สิทธิ์ถูกต้องและ job catch-up ได้; LINE/SMS จริงอยู่ V3 — I/U/T/Q
- [ ] **V2-P1-22** รายงานมูลค่าเสี่ยงหมดอายุแยกจาก realized write-off loss พร้อม drill-down lot/document; กรองเวลา/สาขาและรวมต้นทุนตาม policy เดียวกัน — F/I/U/T

**ขอบเขต PO:** การรองรับ approval/partial receipts หลายครั้งต้องตัดสินตาม DECISIONS.md ก่อนเปิดงาน; V2-P1-02/18 ต้องทำงานรับเข้าแบบปัจจุบันให้ครบอยู่แล้ว หากเพิ่มวงจรใหม่ให้เพิ่ม checklist และ estimate โดยไม่แทนที่รายการเดิม

**Gate P1:** ไม่มี quantity/cost mismatch ใน fixtures และ concurrency scenarios ที่กำหนด; migration/backfill/recovery ผ่าน; การจองและ FEFO ใช้ร่วมได้ก่อนต่อ sales order รุ่นถัดไป

## V2-P2 — ขายส่ง เครดิต เช็ค ใบลดหนี้ และเอกสาร

- [ ] **V2-P2-01** Customer master บุคคล/องค์กรมี ID, ข้อมูลออกเอกสาร/ที่อยู่ส่ง, สถานะและการตรวจซ้ำตาม policy; บิลเก่ายังคง snapshot ไม่ผูกลูกค้าจากชื่ออย่างเดา — F/D/U/T
- [ ] **V2-P2-02** เปิดเส้นทางเอกสารที่ธุรกิจอนุมัติพร้อม server permission/entitlement ที่สอดคล้อง UI; ผู้ใช้ที่ไม่ได้สิทธิ์ข้าม Pro/role ผ่าน API ไม่ได้; ไม่อ้างว่ามีระบบ subscription billing — A/F/U/S
- [ ] **V2-P2-03** Quotation create/edit/preview/convert/cancel และ invoice issued/unpaid/paid ตาม state machine; convert/retry ไม่ตัด stock ซ้ำ; รับชำระและพิมพ์ตรวจข้อมูลจริง — F/I/U/T
- [ ] **V2-P2-04** Price list/ราคาสัญญาลูกค้ามีสาขา/หน่วย/ช่วงเวลาและสิทธิ์; fallback ชัดเจน; เก็บแหล่งราคาในเอกสาร; ราคาไม่เปลี่ยนเพราะ master ถูกแก้ย้อนหลัง — A/F/U/T
- [ ] **V2-P2-05** ราคาส่ง quantity break ใช้จำนวน/หน่วยที่นิยามและทดสอบก่อนถึง/พอดี/เกิน threshold; กรณีหลายบรรทัดสินค้าเดียวกันไม่ให้แบ่งบรรทัดเพื่อเลี่ยงกฎ — F/I/T
- [ ] **V2-P2-06** Branch promotion policy ระบุร้านที่สร้าง/แก้/ลบได้และโปรส่วนกลาง; ไม่กระทบสาขาอื่น; ตรวจผ่าน API และ UI รวมวันที่และสถานะ inactive — F/U/S/T
- [ ] **V2-P2-07** เพดานส่วนลด/override/ผู้อนุมัติแยกตามนโยบาย; เปลี่ยนตะกร้าหลังอนุมัติต้องตรวจใหม่; actor/reason/approved values มีหลักฐาน — A/F/I/U/T
- [ ] **V2-P2-08** Price priority/โปรซ้อน/คูปองถ้ามี/ของแถม/discount/VAT คำนวณลำดับเดียว; ทดสอบโปร V1 ทั้ง 5 แบบร่วมหน่วยขายและราคาส่ง; บิลไม่ติดลบและต้นทุนของแถมไม่หาย — A/F/I/T
- [ ] **V2-P2-09** AR ลูกหนี้: credit terms/limit/due date/exposure/approval และ hold ตาม policy; concurrent ออกบิลไม่ทำวงเงินเกินโดยเลี่ยงการตรวจ; ผูกลูกค้า/สาขา/เอกสารชัดเจน — A/F/D/T
- [ ] **V2-P2-10** AP เจ้าหนี้: supplier terms, bill/PO receipt linkage, due date/ยอดค้าง/การจ่าย/credit adjustment; การรับสินค้ากับการจ่ายเงินเป็นคนละเหตุการณ์ — F/I/D/U/T
- [ ] **V2-P2-11** รับ/จ่ายเต็มหรือบางส่วนได้ ยอดศูนย์/ติดลบ/เกินยอดมีนโยบายชัดเจน; paid/unpaid/partial สอดคล้อง ledger; วันและสิทธิ์รับจ่ายถูกต้อง — F/U/T
- [ ] **V2-P2-12** เงินก้อนเดียว allocate หลายเอกสารและหลายครั้งต่อเอกสารได้; เงินที่ยังไม่ allocate แยกยอด; กัน allocation ซ้ำ/ข้ามลูกค้า/สาขา/นิติบุคคลที่ไม่อนุญาต — F/D/T
- [ ] **V2-P2-13** Aging/statement/as-of date แสดง opening/new payments/credits/closing ถูกต้อง; ทดสอบวันครบกำหนด/ขอบช่วงเดือน/partial payment และ drill-down ถึงเอกสาร — F/U/T/Q
- [ ] **V2-P2-14** เช็ครับ/จ่ายเก็บเลขธนาคาร/วัน/จำนวนเงิน/คู่ค้า/เอกสารอ้างอิงตาม policy; กัน duplicate ที่กำหนดและ state transition ผิด; ยังไม่ถือเป็นเงินสำเร็จโดยไม่มีเงื่อนไขผ่าน — A/F/D/U/T
- [ ] **V2-P2-15** นำฝาก/ผ่านเช็ค/ยกเลิกเช็คมีสิทธิ์และหลักฐาน; เวลาเคลียร์เทียบวันที่ออกบิลถูกต้อง; callback/manual retry ไม่ลง settlement ซ้ำ — F/U/T
- [ ] **V2-P2-16** เช็คตีกลับ/กลับรายการคืนยอดหนี้/สถานะ/วงเงินตาม policy และยังตาม settlement เดิมได้; การแจ้งเหตุซ้ำไม่คืนหนี้สองครั้ง — F/D/T
- [ ] **V2-P2-17** ใบลดหนี้ฝั่งขายผูกใบเดิม/บรรทัด/หน่วย/ราคา/ภาษี/reason/approval; ยอดและจำนวนที่อ้างลดสะสมไม่เกินสิทธิ์รวมทุกใบ — A/F/I/U/T
- [ ] **V2-P2-18** ใบลดหนี้/เครดิตจาก supplier ผูกเอกสารซื้อและ AP/cost adjustment; ตรวจกรณีคืนของ/ลดราคาโดยไม่คืนของแยกกัน — A/F/I/D/T
- [ ] **V2-P2-19** คืนสินค้าบางส่วนหลายครั้งได้ตามยอดที่ขาย/รับจริง; หน่วยและล็อตถูกต้อง; returned/quarantine/sellable แยกสถานะ; เคลมเปลี่ยนสินค้าไม่ถูกนับเป็น refund อัตโนมัติ — I/F/U/T
- [ ] **V2-P2-20** เครดิตคงเหลือ/นำเครดิตไปใช้/คืนเงินและ void/reverse มีเอกสาร/สิทธิ์/audit; คืนเงินไม่บังคับรับของ และรับของไม่คืนเงินเอง; retry ไม่จ่ายซ้ำ — A/F/D/T
- [ ] **V2-P2-21** กระทบยอด AR/AP/cash/payment/invoice/credit note และต้นทุนครบ; รักษา original transaction/adjustment; กำหนดข้อจำกัดเอกสารข้ามงวด; precision/rounding ใช้กฎเดียว — A/F/D/T
- [ ] **V2-P2-22** เอกสาร รพ.สต. ใช้ชื่อ alias/ราคา/หน่วย/ภาษี/ข้อมูลบริษัทและพิมพ์ถูกต้องตามตัวอย่างที่ผู้รับผิดชอบยืนยัน; ไม่ใช้ชื่อใน UI เป็นหลักฐานความถูกต้องเอกสาร — A/F/U/Q
- [ ] **V2-P2-23** Summary/export อย. ตรวจสินค้า/เลขทะเบียน/บริษัท/ใบอนุญาต/หน่วยฐาน/ชนิด movement/ช่วงเวลา/สาขา; มีตัวอย่างที่ตรวจรับได้; ไม่อ้างว่าส่งเข้าหน่วยงานจริงถ้ายังไม่มี integration — A/F/U/T/Q

**Gate P2:** ตัวอย่างซื้อ/ขายเครดิต รับ/จ่ายบางส่วน เช็คตีกลับ คืนบางส่วน/ลดหนี้ และรายงานยอดตรงกันตั้งแต่ UI ถึง ledger; decision ด้านธุรกิจที่มีผลต่อผลลัพธ์ต้องถูกปิดก่อนถือว่าครบ

## เกณฑ์ข้ามฟีเจอร์และจบ V2

- [ ] **V2-G01** ตรวจ traceability จากทุกข้อของ V2 ใน ROADMAP/GAP_MATRIX/DECISIONS มายัง checklist และ task/test; scope เพิ่มต้องเพิ่มรายการ ไม่มี feature หายระหว่างแบ่ง agent — L/A/Q
- [ ] **V2-G02** Required tests ผ่านบน integrated revision ที่ส่งมอบ: build/type/lint/unit/DB integration/API/E2E; บันทึก pass/fail/skip ตามจริง ไม่ใช้ผลคนละ branch มารวมเป็นผ่าน — L/T/S
- [ ] **V2-G03** Fresh install และ upgrade จาก V1 ที่มีธุรกรรม/หน่วย/ล็อต/เคลม/ปิดเดือน ผ่านทั้ง schema/backfill/reconciliation; migration failure มีทางกู้คืนที่ทดลองแล้ว — D/S/T
- [ ] **V2-G04** ทดสอบฐานสต๊อก/เงินด้วย PostgreSQL จริงใน environment แยก รวม locking/unique constraints/transactions; mock ภายนอกได้ แต่ไม่ mock checkout/payment/stock write ที่กำลังตรวจ — T/Q
- [ ] **V2-G05** Role/branch matrix และ direct API access ครบทั้งอ่าน/เขียน/รายงาน/export/print; ผู้ไม่มีสิทธิ์ไม่ได้ข้อมูลผ่าน error/notification/cache/log — S/T/Q
- [ ] **V2-G06** Duplicate/concurrency/stale form/offline retry/crash/partial failure/rollback ครอบคลุม critical writes; มี deterministic expected results และ invariant checks — T/I/F
- [ ] **V2-G07** Mobile/desktop ทั้ง Admin/POS และหน้าการเงินใหม่ผ่าน keyboard/focus/dialog/scroll/long Thai labels/loading/error/conflict; console/server error ที่ไม่คาดหมายถูกแก้ — U/Q
- [ ] **V2-G08** Performance smoke กับข้อมูล/จำนวนผู้ใช้เป้าหมายที่ตกลง; กำหนด response/error/stock sync/job backlog budgets ก่อนวัด; ไม่มีการตั้ง SLA โดยเดาจากจำนวน agents — A/S/T
- [ ] **V2-G09** Backup/restore/recovery และ environment configuration ใช้ได้กับ release candidate; secrets ไม่อยู่ Git/artifact และไม่มี test ชี้ฐานร้านจริง — S/D/Q
- [ ] **V2-G10** Known defects ใน scope และ regression ที่เกิดจาก V2 ถูกแก้และตรวจซ้ำหมด; ไม่มี required skipped/blocked test; ถ้าต้องเลื่อนรายการให้รายงานว่า V2 ยังไม่ครบฉบับนี้ — L/Q
- [ ] **V2-G11** ผู้ใช้จริงตรวจ UAT เส้นทาง POS/คลัง/ซื้อ/ขายส่ง/การเงิน/เอกสารตามหน้าที่; policy/ตัวอย่างเอกสารที่ agent ตัดสินแทนไม่ได้มีผลยืนยัน; ไม่ให้ agent สวมบทผู้ใช้แล้วถือเป็น business sign-off — L/A/Q
- [ ] **V2-G12** Release report ระบุ commit/migrations/decisions/checklist/evidence/known limitations/วิธี deploy และ recovery; deploy/push/การเปลี่ยน production ตาม authorization ที่มีจริง; แยก technical verification จาก released — L/S

การติ๊กครบยืนยันได้ว่า **ผ่านข้อกำหนดและการทดสอบที่ระบุบน revision นั้น** ไม่ใช่การรับรองว่าจะไม่พบข้อผิดพลาดใหม่ในทุกข้อมูล/เครื่อง/สถานการณ์ในอนาคต
