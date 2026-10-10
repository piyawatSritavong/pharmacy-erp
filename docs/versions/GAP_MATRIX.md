# เป้าหมายทั้งหมด → ช่องว่างจาก V1 → รุ่นที่จะพัฒนา

อ้างอิง [V1 baseline](V1_BASELINE.md) และ [รายละเอียด Phase](ROADMAP.md) · 2026-09-16

สถานะในตารางเป็น **ความครบเมื่อเทียบกับเป้าหมาย** ไม่ใช่เปอร์เซ็นต์ความสำเร็จของโครงการ จำนวนหน้าจอ/ตารางไม่สามารถนำไปคิดเป็นเปอร์เซ็นต์ได้อย่างน่าเชื่อถือ

| ID | เป้าหมายเจ้าของโครงการ | สิ่งที่มีจริงใน V1 | งานที่ยังขาด | ส่งมอบใน |
| --- | --- | --- | --- | --- |
| T01 | แตกขายปลีก–ส่ง แปลงหน่วย | หน่วยขาย/ตัวคูณ/ราคาต่อหน่วยและสาขา (V1-11/13); 2026-10-10: หน้าจอหน่วยขาย, ราคาตามจำนวน, ราคาลูกค้าขายส่ง ([V1 ext](V1_EXTENSIONS_2026-10-10.md)) | หน่วยซื้อ–โอน–คืนครบสาย, แตกแพ็ก, ราคาส่งตามจำนวน/ลูกค้า/สัญญา | V2-P0, V2-P1, V2-P2 |
| T02 | เครดิตเทอม | Supplier credit days, PO due date; 2026-10-10: ขายเชื่อที่ POS ตามวงเงิน/วันเครดิต, ลูกหนี้ อายุหนี้ รับบางส่วน ([V1 ext](V1_EXTENSIONS_2026-10-10.md)) | ลูกหนี้/เจ้าหนี้, วงเงิน/วันครบกำหนด, รับ/จ่ายบางส่วน, aging, ตัดชำระตามเอกสาร | V2-P2 |
| T03 | เช็ค | เคยมีแต่ถูกถอดใน migration 016 | รับ/จ่าย/นำฝาก/ผ่าน/คืนเช็ค, ผูก settlement, ป้องกันยอดชำระซ้ำ | V2-P2 |
| T04 | ใบลดหนี้ | เคลมเปลี่ยนสินค้า (V1-29/30) | ใบลดหนี้อ้างใบเดิม, คืนบางส่วน, เครดิตคงเหลือ/คืนเงิน, แยกคืนสต๊อกออกจากผลการเงิน | V2-P2; ข้ามช่องทาง V4-P3/V5-P1 |
| T05 | โปรโมชั่นแต่ละร้านต่างกัน | โปรโมชั่น 5 แบบ แยกเจ้าของสาขา/ส่วนกลาง (V1-15/16); 2026-10-10: โปรเฉพาะสมาชิก | นโยบายว่าสาขาใดสร้าง/อนุมัติได้, โปรซ้อนกัน/ราคาขั้นต่ำ, ขอบเขตช่องทางและสมาชิก | V2-P2; สมาชิก V3-P1; ออนไลน์ V4-P2/V5-P2 |
| T06 | เซลล์เสนอขายตามพื้นที่ เบิกจากสาขาใกล้ลูกค้า | ขอเบิก/โอน, quotation API, remote POS (V1-27/28/37/38) | sales rep role, เขต/ลูกค้า/visit, quotation→sales order, เลือกแหล่งจ่ายและจองของ | V3-P2 |
| T07 | รับเอง/ส่งเอง/จ้างขนส่ง | โอนระหว่างสาขา | รูปแบบ fulfillment, ที่อยู่/นัดหมาย, picking/packing, ค่าส่ง, หลักฐานรับของและ tracking | V3-P2; carrier integration V4-P3/V5-P1 |
| T08 | ขายออนไลน์ | product channel, branch online flag, connection settings (V1-17/46) | คำสั่งซื้อ/สต๊อกจอง/ชำระ/ยกเลิก/คืนเงินจริงครบสาย | ช่องทางตนเอง V4; Marketplace V5-P1 |
| T09 | เว็บ E-Commerce | ไม่มี storefront ลูกค้า (V1-47) | Catalog สาธารณะที่อนุญาต, account/cart/checkout, สถานะคำสั่งซื้อ, mobile UX, SEO พื้นฐาน | V4-P2 |
| T10 | แคมเปญดึงคนเข้าร้านออนไลน์ | POS promotions แต่ไม่มี acquisition/attribution | campaign/landing/coupon, UTM/แหล่งที่มา, funnel, การวัดยอดและกำไร | เริ่มพร้อมเว็บ V4-P2; ขยาย V5-P2 |
| T11 | Lot/Expiry ตั้งแต่รับเข้า | PO และ lot ledger พร้อม expiry (V1-19/21) | บังคับข้อมูลทุกช่องทางรับเข้า รวมเคลม/คืน; trace/recall/quarantine | V2-P1 |
| T12 | แจ้งใกล้หมดอายุ 3/6/9 เดือน | `expiry_warning_days` และ badge ในรายละเอียด (V1-22) | หลายช่วงแบบเดือนปฏิทิน, scheduled alerts, ผู้รับ/การรับทราบ/ไม่แจ้งซ้ำ | V2-P1; LINE/SMS delivery V3-P3 |
| T13 | จ่ายสินค้า FEFO อัตโนมัติ | allocator ใช้โอน/ของแถม; ขายปกติเลือกล็อต (V1-23) | auto-allocation ข้ามล็อตและทุกช่องทางขาย, override พร้อมเหตุผลตามสิทธิ์ | V2-P1; ต่อ OMS V4-P1 |
| T14 | คะแนนสมาชิกทุกช่องทาง | 2026-10-10: ทะเบียนลูกค้ากลาง, ledger แต้ม ได้/ใช้/คืน/ปรับ ใช้ได้ทุกสาขาที่ POS ([V1 ext](V1_EXTENSIONS_2026-10-10.md)) | customer identity, points ledger, earn/redeem/expiry/reversal, รวมบัญชีซ้ำ | เริ่ม POS V3-P1; เว็บ V4-P2; Marketplace V5-P1 ตามข้อมูลระบุตัวตนที่เชื่อถือได้ |
| T15 | Segmentation เช่นกลุ่มโรค/แม่และเด็ก | ไม่มี | tag/segment rules, สิทธิ์ข้อมูลเฉพาะ, ความยินยอมและวัตถุประสงค์; ไม่อนุมานโรคจากการซื้อเอง | V3-P1; แคมเปญ V5-P2 |
| T16 | SMS/LINE เตือน refill เฉพาะสมาชิก | ไม่มี scheduler/ช่องทางส่ง (V1-49) | ลงทะเบียนยินยอม, รอบซื้อ/วันคาดหมด, opt-out/quiet hours, queue/retry/delivery log | V3-P3; ไม่ต้องรอ AI |
| T17 | Real-time stock POS/Web/App/Shopee/Lazada | DB กลางสำหรับ ERP; ไม่มี connector จริง | available/reserved/quarantine, event outbox, connector/retry/reconcile, กัน oversell | ฐาน V2-P1; เว็บ V4; ช่องทางภายนอก V5-P1 |
| T18 | BOPIS จองสินค้า รับสาขาที่สะดวก | ไม่มี reservation/customer order | จองมีอายุ, เตรียมสินค้า, แจ้งพร้อมรับ, ตรวจสิทธิ์รับของ, no-show/refund | V4-P3 |
| T19 | OMS รวม order จัดยา/แพ็ก/ส่งจุดเดียว | marketplace order list ใน DB เท่านั้น | order state machine, channel/source, picking lot, pack, shipment, cancel/refund, exception queue | field order V3-P2; OMS V4-P1; รวม Marketplace V5-P1 |
| T20 | PromptPay QR/บัตร/E-Wallet ทุกช่องทาง | สด/โอน/ผสมที่พนักงานบันทึก (V1-33) | payment intent, provider integration, verified callback, expiry/refund/settlement; QR อย่างเดียวไม่ยืนยันเงินเข้า | V4-P3; ผูกค่าธรรมเนียม V5-P3 |
| T21 | ต้นทุนจริง FIFO หรือ Moving Average รวมส่ง/GP | lot unit cost และ invoice cost snapshot (V1-25/43) | เลือกวิธีตีราคา, landed cost, ส่วนลดซื้อ/คืนซื้อ, shipment/fee allocation, reconciliation | ต้นทุนคลัง V2-P1; ค่าใช้จ่ายคำสั่งซื้อ V4-P1/P3; settlement/GP V5-P3 |
| T22 | Dashboard ยอดขาย/กำไรแยกช่องทาง | แยกสาขาและวิธีชำระ (V1-40/43) | channel dimension, net sales/COGS/fees/refunds/ad cost, data freshness | POS/field V3-P2; เว็บ V4-P3; ครบช่องทาง V5-P3 |
| T23 | รายงานสูญเสียจากหมดอายุ | ยอด expired และ lot cost (V1-19/22) | write-off มูลค่าจริงพร้อมเหตุผล/อนุมัติ, แยกของใกล้หมดอายุจาก loss ที่เกิดแล้ว | V2-P1; แนวโน้มและแคมเปญระบาย V5-P2/P3 |
| T24 | AI local ไม่ส่งข้อมูลไปฝึกโมเดล | ไม่มี (V1-49) | local inference/embeddings/storage, network/telemetry policy, retention, ประเมินความแม่นและสิทธิ์ | V6-P1 |
| T25 | แชทถามรายงานเร็ว | มี report APIs แต่ไม่มีแชท | semantic definitions, read-only report tools ตามสิทธิ์, แหล่งอ้างอิง/ช่วงเวลา/ความสด | V6-P1 |
| T26 | AI ส่งรายงานเข้า LINE เจ้าของ | ไม่มี | ใช้ notification service, schedule, recipient verification, template และขอบเขตข้อมูลส่งออก | ส่งตัวเลขตามกฎได้ตั้งแต่ V3-P3; AI สรุปภาษา V6-P2 |
| T27 | AI ตอบในกลุ่มพนักงาน | ไม่มี | knowledge base, ขอบเขตแต่ละกลุ่ม/สาขา, ป้องกันข้อมูลเกินสิทธิ์และคำสั่งแทรก | V6-P2 |
| T28 | สั่ง AI โอน wheelchair V-001 จำนวน 5 ผ่าน Superadmin | คนสร้างใบโอน/ส่ง/รับได้ (V1-28) | tool สร้างข้อเสนอ/ร่าง, ตรวจ SKU+หน่วย+ล็อต+สต๊อก+สิทธิ์, approve ใน UI, กันซ้ำ/audit | V6-P3 |
| T29 | สต๊อกและออเดอร์จากแอปลูกค้า | มีเว็บ POS บนมือถือสำหรับพนักงาน | customer API เดียวกับเว็บ; mobile web/PWA ก่อน; native app ต้องกำหนดขอบเขตเพิ่ม | เว็บมือถือ V4-P2; API/การเชื่อมแอป V5-P1 |

## งานสนับสนุนที่ต้องมีแม้ไม่ได้เป็นเมนูใหม่

| งาน | เหตุผล | Phase |
| --- | --- | --- |
| แก้ contract พักบิล/remote และกัน checkout ซ้ำ | หน่วย/ราคา/จำนวนต้องคงเดิม และไม่สร้างการขายซ้ำตอน retry | V2-P0 |
| ตรวจฐานเอกสารและยอดเงินจริงก่อน AR/AP | รายงานใหม่ต้องอธิบายการรับเงินและ adjustment ได้ | V2-P0/P2 |
| CI, isolated integration DB, backup/restore และสิทธิ์ที่เพิกถอนได้ | เพิ่มจำนวนผู้ใช้/ช่องทางอย่างตรวจสอบได้ | V2-P0 และตรวจซ้ำทุก release |
| งานเบื้องหลังที่ retry ได้ และ event outbox | รองรับ expiry/reminder/stock sync โดยไม่ทำเหตุการณ์หาย | เริ่ม V2-P1; ขยาย V3-P3/V4-P1 |
| ตัดสินใจเปิดโมดูลที่ติด Pro gate | หน้าข้อเสนอขายไม่ใช่ฟีเจอร์สำเร็จรูปที่ซื้อแล้วเปิดได้จริงในโค้ดนี้ | ก่อน V2-P2 |
| รอบเงินสด/ตรวจนับสต๊อก/กระทบยอด | สนับสนุนความถูกต้องของ V1 เมื่อปริมาณธุรกรรมเพิ่ม | V2-P0/P1 |

คำว่า “โปรแต่ละร้านต่างกัน” ถูกตีความเป็นความสามารถกำหนดนโยบายรายสาขา; ยังไม่ได้เปลี่ยนสิทธิ์จริง ดู [ประเด็นตัดสินใจ D01](DECISIONS.md)
