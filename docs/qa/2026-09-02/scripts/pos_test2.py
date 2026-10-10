from qa import *
import json
exec(open("pos_test.py").read().split("# ---------- MES ----------")[0])
# restock MES QA-001 via PO
r = api("super","POST","/purchase-orders",{"branch_id":B["MES"],"supplier_id":ctx["supplier_id"],"purchased_at":"2026-09-02T12:00:00+07:00","vat_mode":"exclusive","vat_rate":7,"supplier_document_number":"QA-INV-MES-2","items":[{"product_id":P["QA-001"],"stock_bucket":"real","quantity":300,"unit_cost":10,"lot_number":"QA1-MES-R2","expires_on":"2027-06-30"},{"product_id":P["QA-002"],"stock_bucket":"real","quantity":4,"unit_cost":900,"lot_number":"QA2-MES-R2"}]})
print("restock PO:", r["status"], str(r["body"])[:100])
R="pos.mes"
s1,s2 = stock(R,"QA-001"), stock(R,"QA-002")
pv,co = checkout(R,[line(R,"QA-001",3),line(R,"QA-002",1)],"cash",2000)
keep("mes_cash1",co)
sm = pv["body"].get("summary",{}) if pv["status"]==200 else {}
print("MES cash preview summary:", json.dumps(sm, ensure_ascii=False)[:400]); print("checkout:", co["status"], str(co["body"])[:300])
ok = co["status"]==201 and abs(co["body"].get("total_amount",0)-1669.20)<0.01 and abs(co["body"].get("change_amount",0)-330.80)<0.01
record("D-01","POS/ขายหน้าร้าน",R,"ขายเงินสด QA-001 x3 @20 + QA-002 x1 @1500 รับเงิน 2000","PASS" if ok else "FAIL","total 1669.20 (1560+VAT 109.20), ทอน 330.80", f'{co["status"]} total={co["body"].get("total_amount")} change={co["body"].get("change_amount")} inv={co["body"].get("invoice_number")}')
record("D-01b","POS/ขายหน้าร้าน",R,"สต๊อกจริงลดตามขาย (QA-001 -3, QA-002 -1)","PASS" if stock(R,"QA-001")==s1-3 and stock(R,"QA-002")==s2-1 else "FAIL",f"{s1-3}/{s2-1}", f'QA-001 {s1}→{stock(R,"QA-001")} QA-002 {s2}→{stock(R,"QA-002")}')
pv,co = checkout(R,[line(R,"QA-001",10),line(R,"QA-003",1)],"mixed",250,100)
keep("mes_mixed",co)
record("D-02A","POS/ขายหน้าร้าน",R,"ขายเงินสด+โอน QA-001 x10 + QA-003 x1: โอน 100 รับสด 250","PASS" if co["status"]==201 and abs(co["body"].get("cash_amount",0)-226.35)<0.01 and abs(co["body"].get("change_amount",0)-23.65)<0.01 else "FAIL","total 326.35, cash 226.35, transfer 100, ทอน 23.65", f'{co["status"]} {json.dumps({k:co["body"].get(k) for k in ["total_amount","cash_amount","transfer_amount","tendered_amount","change_amount"]})}')
s1 = stock(R,"QA-001")
pv,co = checkout(R,[line(R,"QA-001",2,units["แผง"])],"cash",500)
keep("mes_unit",co)
record("D-06","POS/ขายหน้าร้าน",R,"ขาย QA-001 2 แผง (หน่วยใหญ่ x10 @180) → ตัดสต๊อก 20 เม็ด","PASS" if co["status"]==201 and abs(co["body"].get("total_amount",0)-385.20)<0.01 and stock(R,"QA-001")==s1-20 else "FAIL","total 385.20, stock -20", f'{co["status"]} total={co["body"].get("total_amount")} stock {s1}→{stock(R,"QA-001")}')
s3 = stock(R,"QA-003")
pv,co = checkout(R,[line(R,"QA-001",3,units["แผง"])],"cash",1000)
keep("mes_bxgy",co)
lines = pv["body"].get("lines",[]) if pv["status"]==200 else []
give = [l for l in lines if l.get("is_giveaway")]
print("BXGY lines:", [(l.get("display_name"),l.get("quantity"),l.get("unit_price"),l.get("is_giveaway"),l.get("line_total")) for l in lines], pv["body"].get("applied_promotions") if pv["status"]==200 else pv["body"], json.dumps(pv["body"].get("summary"),ensure_ascii=False) if pv["status"]==200 else "")
record("D-07","POS/ขายหน้าร้าน",R,"โปรฯ ซื้อ QA-001 3 แผง แถม QA-003 1 (buy_x_get_y)","PASS" if co["status"]==201 and give and stock(R,"QA-003")==s3-1 and abs(co["body"].get("total_amount",0)-577.80)<0.01 else "FAIL","มีบรรทัดของแถม, total 577.80, QA-003 -1", f'{co["status"]} total={co["body"].get("total_amount")} giveaway={[(g.get("display_name"),g.get("quantity"),g.get("unit_price")) for g in give]} QA-003 {s3}→{stock(R,"QA-003")}')
# unit barcode lookup: products list search by unit barcode
pl = api(R,"GET","/products",query={"search":"8850000000111"})
record("D-06b","POS/ขายหน้าร้าน",R,"ค้นหาสินค้าด้วยบาร์โค้ดของหน่วยแผง (8850000000111)","PASS" if pl["status"]==200 and any(p["sku"]=="QA-001" for p in pl["body"].get("items",[])) else "FAIL","พบ QA-001", f'{pl["status"]} found={[p["sku"] for p in pl["body"].get("items",[])][:3]}')
# extra cash sales for month-end volume
for i in range(3):
    pv,co = checkout(R,[line(R,"QA-001",5+i)],"cash",500); keep(f"mes_cash_extra{i}",co)
print("extra:", [ctx["pos_invoices"][k]["invoice_number"] for k in ctx["pos_invoices"] if k.startswith("mes_cash_extra")])
# sales history as POS: own branch only
h = api(R,"GET","/invoices")
nums = [x["invoice_number"] for x in h["body"].get("items",[])]
record("D-04","POS/ประวัติ",R,"POS ดูประวัติใบขาย (เฉพาะสาขา MES)","PASS" if h["status"]==200 and nums and all(n.startswith("MES-") for n in nums) else "FAIL","เฉพาะ MES-*", f'{h["status"]} count={len(nums)} sample={nums[:4]}')
inv_id = ctx["pos_invoices"]["mes_cash1"]["invoice_id"]
pr = api(R,"GET",f"/invoices/{inv_id}/print")
record("D-04b","POS/ประวัติ",R,"POS เปิดข้อมูลพิมพ์ใบเสร็จย้อนหลัง","PASS" if pr["status"]==200 else "FAIL","200", f'{pr["status"]} keys={list(pr["body"].keys()) if isinstance(pr["body"],dict) else ""}')
o = api("pos.phh","GET",f"/invoices/{inv_id}")
record("D-04c","POS/ประวัติ","pos.phh","POS สาขาอื่นเปิดใบเสร็จของ MES","PASS" if o["status"] in (403,404) else "FAIL","403/404", f'{o["status"]}')
json.dump(ctx, open("ctx.json","w"), ensure_ascii=False, indent=1)
