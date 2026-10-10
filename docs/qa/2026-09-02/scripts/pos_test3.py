from qa import *
import json, time
exec(open("pos_test.py").read().split("# ---------- MES ----------")[0])
# ---------- PHH ----------
R="pos.phh"
s1=stock(R,"QA-001")
pv,co = checkout(R,[line(R,"QA-001",5),line(R,"QA-003",2)],"cash",1000); keep("phh_cash1",co)
sm = pv["body"].get("summary",{}) if pv["status"]==200 else {}
# 100 + 210 = 310 ; promo 10% on QA-003 = 21 → 289 → VAT 20.23 → 309.23
record("D-20","POS/ขายหน้าร้าน",R,"PHH ขายเงินสด QA-001 x5 + QA-003 x2 (โปรฯ 10% ทุกสาขา)","PASS" if co["status"]==201 and abs(co["body"].get("total_amount",0)-309.23)<0.01 else "FAIL","total 309.23", f'{co["status"]} total={co["body"].get("total_amount")} promo={sm.get("promotion_discount_total")}')
pv,co = checkout(R,[line(R,"QA-003",1)],"bank_transfer"); keep("phh_transfer",co)
record("D-21","POS/ขายหน้าร้าน",R,"PHH ขายเงินโอน QA-003 x1 (ไม่ถึง min 2 → ไม่ลด)","PASS" if co["status"]==201 and abs(co["body"].get("total_amount",0)-112.35)<0.01 else "FAIL","total 112.35", f'{co["status"]} total={co["body"].get("total_amount")}')
pv,co = checkout(R,[line(R,"QA-001",2)],"cash",100,customer_name="ลูกค้า PHH เต็มรูป",customer_tax_id="1234567890123",full_tax_invoice=True); keep("phh_fulltax",co)
record("D-22","POS/ขายหน้าร้าน",R,"PHH ขายเงินสดขอใบกำกับเต็มรูป QA-001 x2","PASS" if co["status"]==201 else "FAIL","201", f'{co["status"]} total={co["body"].get("total_amount")}')
for i in range(2):
    pv,co = checkout(R,[line(R,"QA-001",3)],"cash",100); keep(f"phh_cash_extra{i}",co)
# returns: get invoice item id of phh_cash1
inv = api(R,"GET",f"/invoices/{ctx['pos_invoices']['phh_cash1']['invoice_id']}")["body"]
item = [i for i in inv.get("items",[]) if i.get("product_id")==P["QA-001"]][0]
print("phh invoice items keys:", list(item.keys())[:20])
s_before = stock(R,"QA-001")
r = api(R,"POST","/product-returns",{"invoice_item_id":item["id"],"quantity":1,"reason":"ซองฉีก ลูกค้าขอเปลี่ยน"})
ctx["return_phh"]=r["body"].get("id") if r["status"]==201 else None
record("RET-01","POS/เคลม-คืนสินค้า",R,"POS รับคืน/เปลี่ยนสินค้า QA-001 x1 จากบิล (เปลี่ยนตัวใหม่ทันที → สต๊อก -1)","PASS" if r["status"]==201 and stock(R,"QA-001")==s_before-1 else "FAIL","201 + สต๊อก -1", f'{r["status"]} {str(r["body"])[:100]} stock {s_before}→{stock(R,"QA-001")}')
r = api(R,"POST","/product-returns",{"invoice_item_id":item["id"],"quantity":10,"reason":"เกินจำนวนที่ซื้อ"})
record("RET-02","POS/เคลม-คืนสินค้า",R,"คืนเกินจำนวนที่ซื้อ (10 > 5)","PASS" if r["status"]==409 else "FAIL","409", f'{r["status"]} {str(r["body"])[:100]}')
r = api(R,"POST","/product-returns",{"invoice_item_id":item["id"],"quantity":1,"reason":""})
record("RET-03","POS/เคลม-คืนสินค้า",R,"คืนโดยไม่ระบุเหตุผล","PASS" if r["status"]==400 else "FAIL","400", f'{r["status"]} {str(r["body"])[:100]}')
r = api("pos.mes","POST","/product-returns",{"invoice_item_id":item["id"],"quantity":1,"reason":"สาขาอื่น"})
record("RET-04","POS/เคลม-คืนสินค้า","pos.mes","POS สาขาอื่นทำคืนบิลของ PHH","PASS" if r["status"] in (403,404) else "FAIL","403", f'{r["status"]} {str(r["body"])[:100]}')
# ---------- PHS ----------
R="pos.phs"
pv,co = checkout(R,[line(R,"QA-001",4)],"cash",100); keep("phs_cash1",co)
sm = pv["body"].get("summary",{}) if pv["status"]==200 else {}
print("PHS amount promo:", json.dumps(sm, ensure_ascii=False)[:300], pv["body"].get("applied_promotions") if pv["status"]==200 else pv["body"])
# 80 - 5 (amount promo, once per bill?) = 75 → VAT 5.25 → 80.25  (or per item 4x5=20 → 60 → 64.20)
record("D-23","POS/ขายหน้าร้าน",R,"PHS ขายเงินสด QA-001 x4 มีโปรฯ ลด 5 บาท (amount เฉพาะ PHS)","PASS" if co["status"]==201 and sm.get("promotion_discount_total",0)>0 else "FAIL","มีส่วนลดโปรฯ", f'{co["status"]} total={co["body"].get("total_amount")} promo={sm.get("promotion_discount_total")}')
pv,co = checkout(R,[line(R,"QA-002",1)],"mixed",1000,800); keep("phs_mixed",co)
record("D-24","POS/ขายหน้าร้าน",R,"PHS ขาย QA-002 x1 เงินสด+โอน (โอน 800 สด 1000)","PASS" if co["status"]==201 and abs(co["body"].get("cash_amount",0)-805)<0.01 else "FAIL","total 1605, cash 805, ทอน 195", f'{co["status"]} {json.dumps({k:co["body"].get(k) for k in ["total_amount","cash_amount","transfer_amount","change_amount"]})}')
pv,co = checkout(R,[line(R,"QA-003",3)],"cash",400); keep("phs_cash2",co)
# ---------- NPT ----------
R="pos.npt"
pv,co = checkout(R,[line(R,"QA-001",2)],"cash",100); keep("npt_cash1",co)
# branch price 25 → 50 → VAT 3.5 → 53.50
record("D-25","POS/ขายหน้าร้าน",R,"NPT ขาย QA-001 x2 ใช้ราคาเฉพาะสาขา 25","PASS" if co["status"]==201 and abs(co["body"].get("total_amount",0)-53.50)<0.01 else "FAIL","total 53.50", f'{co["status"]} total={co["body"].get("total_amount")}')
pv,co = checkout(R,[line(R,"QA-001",1,units["แผง"]),line(R,"QA-003",1)],"cash",500); keep("npt_bundle",co)
sm = pv["body"].get("summary",{}) if pv["status"]==200 else {}
print("NPT bundle:", json.dumps(sm, ensure_ascii=False)[:300], pv["body"].get("applied_promotions") if pv["status"]==200 else pv["body"], [(l.get("display_name"),l.get("unit_price"),l.get("line_total")) for l in pv["body"].get("lines",[])] if pv["status"]==200 else "")
# bundle 250 → VAT 17.5 → 267.50
record("D-26","POS/ขายหน้าร้าน",R,"NPT โปรฯ bundle QA-001 1 แผง + QA-003 1 = 250","PASS" if co["status"]==201 and abs(co["body"].get("total_amount",0)-267.50)<0.01 else "FAIL","total 267.50", f'{co["status"]} total={co["body"].get("total_amount")} promo={sm.get("promotion_discount_total")}')
pv,co = checkout(R,[line(R,"QA-002",1)],"bank_transfer"); keep("npt_transfer",co)
pv,co = checkout(R,[line(R,"QA-001",6)],"cash",200); keep("npt_cash2",co)
# ---------- KNP ----------
R="pos.knp"
s3=stock(R,"QA-003")
pv,co = checkout(R,[line(R,"QA-002",1)],"cash",2000); keep("knp_gift",co)
lines = pv["body"].get("lines",[]) if pv["status"]==200 else []
give = [l for l in lines if l.get("is_giveaway")]
print("KNP gift:", [(l.get("display_name"),l.get("quantity"),l.get("unit_price"),l.get("is_giveaway")) for l in lines], pv["body"].get("applied_promotions") if pv["status"]==200 else pv["body"])
record("D-27","POS/ขายหน้าร้าน",R,"KNP ซื้อครบ 1500 แถม QA-003 1 (bill_giveaway)","PASS" if co["status"]==201 and give and stock(R,"QA-003")==s3-1 else "FAIL","มีของแถม + QA-003 -1", f'{co["status"]} total={co["body"].get("total_amount")} give={[(g.get("display_name"),g.get("quantity")) for g in give]} QA-003 {s3}→{stock(R,"QA-003")}')
pv,co = checkout(R,[line(R,"QA-003",3)],"bank_transfer"); keep("knp_transfer",co)
pv,co = checkout(R,[line(R,"QA-001",4)],"cash",100); keep("knp_cash1",co)
pv,co = checkout(R,[line(R,"QA-001",2)],"cash",100,customer_name="ลูกค้า KNP",customer_tax_id="3100100100101",full_tax_invoice=True); keep("knp_fulltax",co)
print("all invoices:", {k:v.get("invoice_number") for k,v in ctx["pos_invoices"].items()})
# daily sales per POS
for R in ["pos.mes","pos.phh","pos.phs","pos.npt","pos.knp"]:
    d = api(R,"GET","/dashboard/daily-sales",query={"date":"2026-09-02"})
    print(R, d["status"], json.dumps(d["body"], ensure_ascii=False)[:350])
json.dump(ctx, open("ctx.json","w"), ensure_ascii=False, indent=1)
