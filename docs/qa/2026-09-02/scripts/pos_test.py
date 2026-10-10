from qa import *
import json
ctx=json.load(open("ctx.json")); B=ctx["branches"]; P=ctx["products"]
units = {u["unit_name"]:u["id"] for u in ctx["units"]["items"]}
ctx.setdefault("pos_invoices",{})
def lot(role, pid, idx=0, need=1):
    bid = me(role)["user"]["branch_id"]
    r = api("super","GET","/sales/lot-options",query={"branch_id":bid,"product_id":pid,"stock_bucket":"real"})
    items = r["body"].get("items",[]) if r["status"]==200 else []
    for it in items:
        if it.get("remaining_quantity",0)>=need: return it["id"]
    return items[idx]["id"] if len(items)>idx else None
def line(role, sku, qty, unit="", **kw):
    conv = {"": 1}
    for u in ctx["units"]["items"]: conv[u["id"]]=u["conversion_qty"]
    d = {"product_id":P[sku],"inventory_lot_id":lot(role,P[sku],need=qty*conv.get(unit,1)),"quantity":qty,"unit_id":unit,"stock_bucket":"real"}
    d.update(kw); return d
def checkout(role, items, ptype="cash", tendered=0, transfer=0, **kw):
    body = {"items":items,"payment_type":ptype,"tendered_amount":tendered,"transfer_amount":transfer,"customer_name":kw.get("customer_name",""),"customer_tax_id":kw.get("customer_tax_id",""),
            "full_tax_invoice":kw.get("full_tax_invoice",False),"is_government_mode":kw.get("gov",False),"bill_discount_amount":kw.get("bill_discount",0),"reference_code":kw.get("ref",""),"notes":kw.get("notes","")}
    pv = api(role,"POST","/pos/preview",body)
    co = api(role,"POST","/pos/checkout",body)
    return pv, co
def stock(role, sku):
    bid = me(role)["user"]["branch_id"]
    r = api("super","GET","/inventory",query={"branch_id":bid,"search":sku})
    for it in r["body"].get("items",[]):
        if it["sku"]==sku: return it["qty_real"]
    return None
def keep(key, co):
    if co["status"]==201: ctx["pos_invoices"][key]=co["body"]
# ---------- MES ----------
R="pos.mes"
s1,s2 = stock(R,"QA-001"), stock(R,"QA-002")
pv,co = checkout(R,[line(R,"QA-001",3),line(R,"QA-002",1)],"cash",2000)
keep("mes_cash1",co)
sm = pv["body"].get("summary",{}) if pv["status"]==200 else {}
print("MES cash preview summary:", json.dumps(sm, ensure_ascii=False)[:400]); print("checkout:", co["status"], str(co["body"])[:300])
ok = co["status"]==201 and abs(co["body"].get("total_amount",0)-1669.20)<0.01 and abs(co["body"].get("change_amount",0)-330.80)<0.01
record("D-01","POS/ขายหน้าร้าน",R,"ขายเงินสด QA-001 x3 @20 + QA-002 x1 @1500 รับเงิน 2000","PASS" if ok else "FAIL","total 1669.20 (1560+VAT 109.20), ทอน 330.80", f'{co["status"]} total={co["body"].get("total_amount")} change={co["body"].get("change_amount")} inv={co["body"].get("invoice_number")}')
record("D-01b","POS/ขายหน้าร้าน",R,"สต๊อกจริงลดตามขาย (QA-001 -3, QA-002 -1) และสต๊อกผีไม่เปลี่ยน","PASS" if stock(R,"QA-001")==s1-3 and stock(R,"QA-002")==s2-1 else "FAIL",f"{s1-3}/{s2-1}", f'QA-001 {s1}→{stock(R,"QA-001")} QA-002 {s2}→{stock(R,"QA-002")}')
pv,co = checkout(R,[line(R,"QA-003",2)],"bank_transfer",0,0,ref="TRX-MES-001")
keep("mes_transfer",co); sm = pv["body"].get("summary",{}) if pv["status"]==200 else {}
print("MES transfer preview:", json.dumps(sm, ensure_ascii=False)[:300], [ (l.get("display_name"), l.get("promotion_discount"), l.get("line_total")) for l in pv["body"].get("lines",[])] if pv["status"]==200 else pv["body"], pv["body"].get("applied_promotions") if pv["status"]==200 else "")
# 2*105=210 (price updated to 105) -10% = 189 → VAT 13.23 → 202.23
record("D-02","POS/ขายหน้าร้าน",R,"ขายเงินโอน QA-003 x2 @105 มีโปรฯ 10% (min 2)","PASS" if co["status"]==201 and abs(co["body"].get("total_amount",0)-202.23)<0.01 else "FAIL","total 202.23 (210-21=189 +VAT 13.23)", f'{co["status"]} total={co["body"].get("total_amount")} promo_discount={sm.get("promotion_discount_total")} transfer={co["body"].get("transfer_amount")}')
pv,co = checkout(R,[line(R,"QA-001",10),line(R,"QA-003",1)],"mixed",250,100)
keep("mes_mixed",co)
# 200+105=305 → VAT 21.35 → 326.35 ; transfer 100 → cash 226.35 ; tendered 250 → change 23.65
record("D-02A","POS/ขายหน้าร้าน",R,"ขายเงินสด+โอน QA-001 x10 + QA-003 x1: โอน 100 รับสด 250","PASS" if co["status"]==201 and abs(co["body"].get("cash_amount",0)-226.35)<0.01 and abs(co["body"].get("change_amount",0)-23.65)<0.01 else "FAIL","total 326.35, cash 226.35, transfer 100, ทอน 23.65", f'{co["status"]} {json.dumps({k:co["body"].get(k) for k in ["total_amount","cash_amount","transfer_amount","tendered_amount","change_amount"]})}')
pv,co = checkout(R,[line(R,"QA-002",1)],"cash",1605,customer_name="บริษัท ลูกค้าเต็มรูป จำกัด",customer_tax_id="0105557778889",full_tax_invoice=True)
keep("mes_fulltax",co)
record("D-05","POS/ขายหน้าร้าน",R,"ขายเงินสดขอใบกำกับภาษีเต็มรูป QA-002 x1 (1605 พอดี)","PASS" if co["status"]==201 and abs(co["body"].get("change_amount",0))<0.01 else "FAIL","201 ทอน 0", f'{co["status"]} total={co["body"].get("total_amount")} change={co["body"].get("change_amount")}')
pv,co = checkout(R,[line(R,"QA-002",1)],"cash",1605,customer_name="x",customer_tax_id="",full_tax_invoice=True)
record("D-05b","POS/ขายหน้าร้าน",R,"ขอใบกำกับภาษีเต็มรูปโดยไม่ใส่เลขผู้เสียภาษี","PASS" if co["status"]==400 else "FAIL","400", f'{co["status"]} {str(co["body"])[:120]}')
s1 = stock(R,"QA-001")
pv,co = checkout(R,[line(R,"QA-001",2,units["แผง"])],"cash",500)
keep("mes_unit",co)
# 2 แผง @180 = 360 → VAT 25.2 → 385.20 ; stock -20
record("D-06","POS/ขายหน้าร้าน",R,"ขาย QA-001 2 แผง (หน่วยใหญ่ x10 @180) → ตัดสต๊อก 20 เม็ด","PASS" if co["status"]==201 and abs(co["body"].get("total_amount",0)-385.20)<0.01 and stock(R,"QA-001")==s1-20 else "FAIL","total 385.20, stock -20", f'{co["status"]} total={co["body"].get("total_amount")} stock {s1}→{stock(R,"QA-001")}')
s3 = stock(R,"QA-003")
pv,co = checkout(R,[line(R,"QA-001",3,units["แผง"])],"cash",1000)
keep("mes_bxgy",co)
lines = pv["body"].get("lines",[]) if pv["status"]==200 else []
give = [l for l in lines if l.get("is_giveaway")]
print("BXGY lines:", [(l.get("display_name"),l.get("quantity"),l.get("unit_price"),l.get("is_giveaway")) for l in lines], pv["body"].get("applied_promotions") if pv["status"]==200 else pv["body"])
# 3 แผง = 540 → VAT 37.8 → 577.80 ; giveaway QA-003 1 → stock -1
record("D-07","POS/ขายหน้าร้าน",R,"โปรฯ ซื้อ QA-001 3 แผง แถม QA-003 1 (buy_x_get_y)","PASS" if co["status"]==201 and give and stock(R,"QA-003")==s3-1 and abs(co["body"].get("total_amount",0)-577.80)<0.01 else "FAIL","มีบรรทัดของแถม, total 577.80, QA-003 -1", f'{co["status"]} total={co["body"].get("total_amount")} giveaway={[(g.get("display_name"),g.get("quantity"),g.get("unit_price")) for g in give]} QA-003 {s3}→{stock(R,"QA-003")}')
pv,co = checkout(R,[line(R,"QA-001",2,discount_amount=4)],"cash",100)
record("D-08","POS/ขายหน้าร้าน",R,"POS ให้ส่วนลดรายบรรทัด 4 บาท (ไม่เกินเพดาน 5/ชิ้น)","FAIL" if co["status"]==403 else ("PASS" if co["status"]==201 else "FAIL"),"201 (ต้องให้ส่วนลดได้ตาม migration 047)", f'{co["status"]} {str(co["body"])[:120]}')
keep("mes_linedisc",co)
pv,co = checkout(R,[line(R,"QA-001",2)],"cash",100,bill_discount=5)
record("D-08b","POS/ขายหน้าร้าน",R,"POS ให้ส่วนลดท้ายบิล 5 บาท","FAIL" if co["status"]==403 else ("PASS" if co["status"]==201 else "FAIL"),"201", f'{co["status"]} {str(co["body"])[:120]}')
keep("mes_billdisc",co)
pv,co = checkout(R,[line(R,"QA-002",999)],"cash",99999)
record("D-03","POS/ขายหน้าร้าน",R,"ขายเกินสต๊อก QA-002 x999","PASS" if co["status"] in (400,409) else "FAIL","400/409 ไม่มี invoice ใหม่", f'{co["status"]} {str(co["body"])[:120]}')
pv,co = checkout(R,[line(R,"QA-001",1,override_unit_price=17,override_reason="ลูกค้าประจำ")],"cash",100)
keep("mes_override",co)
record("D-09","POS/ขายหน้าร้าน",R,"POS override ราคา 17 (floor = 20-5 = 15)","PASS" if co["status"]==201 and abs(co["body"].get("total_amount",0)-18.19)<0.01 else "FAIL","201 total 18.19", f'{co["status"]} total={co["body"].get("total_amount")} {str(co["body"])[:80]}')
pv,co = checkout(R,[line(R,"QA-001",1,override_unit_price=10,override_reason="ต่ำกว่า floor")],"cash",100)
record("D-09b","POS/ขายหน้าร้าน",R,"POS override ราคา 10 (ต่ำกว่า floor 15)","PASS" if co["status"] in (400,403) else "FAIL","400/403", f'{co["status"]} {str(co["body"])[:120]}')
pv,co = checkout(R,[line(R,"QA-002",1,alias_id=ctx["alias_qa2"])],"cash",2000,gov=True)
record("D-10","POS/ขายหน้าร้าน",R,"POS ใช้โหมดราชการ/alias","PASS" if co["status"]==403 else "FAIL","403", f'{co["status"]} {str(co["body"])[:120]}')
pv,co = checkout(R,[{"product_id":P["QA-001"],"inventory_lot_id":lot(R,P["QA-001"]),"quantity":1,"stock_bucket":"ghost"}],"cash",100)
record("D-11","POS/ขายหน้าร้าน",R,"POS ขายจาก Ghost bucket","PASS" if co["status"] in (400,403) else "FAIL","400/403", f'{co["status"]} {str(co["body"])[:120]}')
pv,co = checkout(R,[line(R,"QA-001",1)],"cash",10)
record("D-12","POS/ขายหน้าร้าน",R,"เงินสดรับน้อยกว่ายอด","PASS" if co["status"]==400 else "FAIL","400", f'{co["status"]} {str(co["body"])[:120]}')
pv,co = checkout(R,[line(R,"QA-001",1)],"mixed",100,100)
record("D-12b","POS/ขายหน้าร้าน",R,"mixed โดยยอดโอน ≥ ยอดรวม","PASS" if co["status"]==400 else "FAIL","400", f'{co["status"]} {str(co["body"])[:120]}')
pv,co = checkout(R,[line(R,"QA-001",1)],"credit_card",100)
record("D-12c","POS/ขายหน้าร้าน",R,"payment_type ไม่รู้จัก","PASS" if co["status"]==400 else "FAIL","400", f'{co["status"]} {str(co["body"])[:120]}')
pv,co = checkout(R,[{**line(R,"QA-001",1),"inventory_lot_id":"00000000-0000-0000-0000-000000000000"}],"cash",100)
record("D-12d","POS/ขายหน้าร้าน",R,"lot id ไม่มีอยู่จริง","PASS" if co["status"] in (400,404,409) else "FAIL","400/404", f'{co["status"]} {str(co["body"])[:120]}')
# park bill
r = api(R,"POST","/parked-bills",{"customer_name":"ลูกค้าพักบิล","note":"รอโทรถาม","items":[{"product_id":P["QA-001"],"inventory_lot_id":lot(R,P["QA-001"]),"quantity":2,"unit_price":20,"product_name":"QA ยาพาราเซตามอล 500mg","sku":"QA-001"}]})
pid = r["body"].get("id"); s1=stock(R,"QA-001")
record("D-13","POS/พักบิล",R,"พักบิล (ไม่ตัดสต๊อก)","PASS" if r["status"]==201 else "FAIL","201", f'{r["status"]} {str(r["body"])[:120]}')
l = api(R,"GET","/parked-bills"); g = api(R,"GET",f"/parked-bills/{pid}")
o = api("pos.phh","GET",f"/parked-bills/{pid}")
record("D-13b","POS/พักบิล",R,"ดูรายการพักบิล / เปิดบิล / สาขาอื่นเปิดไม่ได้","PASS" if l["status"]==200 and any(x["id"]==pid for x in l["body"]["items"]) and g["status"]==200 and o["status"]==404 else "FAIL","200/200/404", f'list={l["status"]} get={g["status"]} other={o["status"]}')
d = api(R,"DELETE",f"/parked-bills/{pid}")
record("D-13c","POS/พักบิล",R,"ลบบิลที่พัก","PASS" if d["status"]==200 else "FAIL","200", f'{d["status"]}')
# ---------- WH ----------
R="pos.wh"
pv,co = checkout(R,[line(R,"QA-001",1)],"cash",100)
record("D-14","POS/ขายหน้าร้าน",R,"POS โกดัง (pos.warehouse) ขายสินค้า","INFO","README/migration 037: โกดังต้องขายได้ | seed สด: sales_enabled=false", f'{co["status"]} {str(co["body"])[:120]} → บัญชี pos.warehouse ใช้ขายไม่ได้บน seed สด')
json.dump(ctx, open("ctx.json","w"), ensure_ascii=False, indent=1)
