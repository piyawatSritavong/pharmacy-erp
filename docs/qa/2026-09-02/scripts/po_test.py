from qa import *
import json
ctx = json.load(open("ctx.json"))
B = {"KNP":"b8aaf520-9204-54d8-bb4e-b745a12dcd94","MES":"6a996b92-3280-500d-a621-d308494f0e9d","NPT":"214c26cc-a7dd-50d8-be3f-e568f84593f7","PHH":"74fb7490-9874-5f85-a1d9-bce0748f2f2b","PHS":"7106c44b-bc31-5af0-bd2a-801af863101d","WH":"f0d67341-54ff-5475-8ca0-ed261f96c941"}
ctx["branches"]=B
P = ctx["products"]; sup = ctx["supplier_id"]
def po(branch, items, **kw):
    body = {"branch_id":B[branch],"supplier_id":sup,"purchased_at":"2026-09-02T10:00:00+07:00","vat_mode":kw.get("vat_mode","exclusive"),"vat_rate":7,
            "header_discount":kw.get("header_discount",0),"shipping_amount":kw.get("shipping",0),"notes":kw.get("notes","QA PO "+branch),
            "supplier_document_number":kw.get("doc","QA-INV-"+branch),"job_name":"QA","delivery_terms":"ส่งถึงสาขา","items":items}
    return api(kw.get("role","super"),"POST","/purchase-orders",body)
def line(sku,bucket,qty,cost,lot,exp="2027-06-30",disc=0):
    return {"product_id":P[sku],"stock_bucket":bucket,"quantity":qty,"unit_cost":cost,"line_discount":disc,"lot_number":lot,"expires_on":exp}
r = po("WH",[line("QA-001","real",500,10,"QA1-WH-R1"),line("QA-001","ghost",300,10,"QA1-WH-G1"),
             line("QA-002","real",20,900,"QA2-WH-R1",""),line("QA-002","ghost",10,900,"QA2-WH-G1",""),
             line("QA-003","real",200,60,"QA3-WH-R1","2027-03-31",disc=200),line("QA-003","ghost",100,60,"QA3-WH-G1","2027-03-31")],header_discount=100,shipping=50)
print("PO-A:", r["status"], str(r["body"])[:600])
ctx["po_wh"]= r["body"].get("id") if isinstance(r["body"],dict) else None
record("S-05","ใบเอกสาร/ใบสั่งซื้อเข้า","super","สร้าง PO ที่ WH: Real+Ghost 6 บรรทัด, ส่วนลดบรรทัด 200, ส่วนลดท้ายบิล 100, ค่าส่ง 50, VAT 7% exclusive","PASS" if r["status"] in (200,201) else "FAIL","201 + PO number", f'{r["status"]} {str(r["body"])[:200]}')
for br, items in {
  "MES":[line("QA-001","real",100,10,"QA1-MES-R1"),line("QA-002","real",5,900,"QA2-MES-R1",""),line("QA-003","real",40,60,"QA3-MES-R1","2027-03-31")],
  "PHH":[line("QA-001","real",50,10,"QA1-PHH-R1"),line("QA-003","real",20,60,"QA3-PHH-R1","2026-10-15")],
  "NPT":[line("QA-001","real",50,10,"QA1-NPT-R1"),line("QA-002","real",3,900,"QA2-NPT-R1","")],
  "PHS":[line("QA-001","real",30,10,"QA1-PHS-R1"),line("QA-003","real",15,60,"QA3-PHS-R1","2027-03-31")],
  "KNP":[line("QA-001","real",30,10,"QA1-KNP-R1"),line("QA-002","real",2,900,"QA2-KNP-R1",""),line("QA-003","real",10,60,"QA3-KNP-R1","2027-03-31")],
}.items():
    r = po(br, items)
    print("PO", br, r["status"], str(r["body"])[:200])
    ctx["po_"+br.lower()] = r["body"].get("id") if isinstance(r["body"],dict) else None
record("S-06","ใบเอกสาร/ใบสั่งซื้อเข้า","super","สร้าง PO Real ให้ MES/PHH/NPT/PHS/KNP","PASS" if all(ctx.get("po_"+b.lower()) for b in ["MES","PHH","NPT","PHS","KNP"]) else "FAIL","201 ทุกสาขา", json.dumps({k:v for k,v in ctx.items() if k.startswith("po_")}))
r = po("MES",[line("QA-001","ghost",10,10,"QA1-MES-G1")])
record("S-07","ใบเอกสาร/ใบสั่งซื้อเข้า","super","PO บรรทัด Ghost ที่สาขาที่ไม่ใช่ WH","PASS" if r["status"]==400 else "FAIL","400 (Ghost เฉพาะ WH)", f'{r["status"]} {str(r["body"])[:160]}')
r = po("WH",[line("QA-001","real",0,10,"X")])
record("S-07b","ใบเอกสาร/ใบสั่งซื้อเข้า","super","PO จำนวน 0","PASS" if r["status"]==400 else "FAIL","400", f'{r["status"]} {str(r["body"])[:160]}')
r = po("WH",[line("QA-001","real",5,10,"QA1-WH-EXPIRED","2020-01-01")],doc="QA-INV-EXP")
record("S-07c","ใบเอกสาร/ใบสั่งซื้อเข้า","super","PO วันหมดอายุในอดีต (2020-01-01)","INFO" if r["status"] in (200,201) else "PASS","ควรเตือน/ปฏิเสธ (spec ไม่ระบุ)", f'{r["status"]} {str(r["body"])[:160]}')
if r["status"] in (200,201): ctx["po_expired"]=r["body"].get("id")
r = po("WH",[line("QA-001","ghost",5,10,"QA1-WH-G-central")],role="central")
record("S-07d","ใบเอกสาร/ใบสั่งซื้อเข้า","central","central admin สร้าง PO บรรทัด Ghost ที่ WH","PASS" if r["status"]==403 else "FAIL","403 (Ghost เฉพาะ superadmin)", f'{r["status"]} {str(r["body"])[:160]}')
r = po("WH",[line("QA-001","real",7,10,"QA1-WH-R-central")],role="central",doc="QA-INV-CENTRAL")
record("S-07e","ใบเอกสาร/ใบสั่งซื้อเข้า","central","central admin สร้าง PO Real ที่ WH","PASS" if r["status"] in (200,201) else "FAIL","201", f'{r["status"]} {str(r["body"])[:160]}')
ctx["po_central"]=r["body"].get("id") if r["status"] in (200,201) else None
r = po("WH",[line("QA-001","real",7,10,"X")],role="pos.mes")
record("S-07f","ใบเอกสาร/ใบสั่งซื้อเข้า","pos.mes","POS สร้าง PO","PASS" if r["status"]==403 else "FAIL","403", f'{r["status"]} {str(r["body"])[:160]}')
json.dump(ctx, open("ctx.json","w"), ensure_ascii=False, indent=1)
d = api("super","GET",f"/purchase-orders/{ctx['po_wh']}")
b = d["body"]
print(json.dumps({k:b.get(k) for k in ["po_number","status","subtotal","line_discount_total","header_discount","shipping_amount","tax_amount","total_amount","vat_mode","vat_rate"]}, ensure_ascii=False))
for it in (b.get("items") or [])[:6]:
    print({k:it.get(k) for k in ["stock_bucket","quantity","unit_cost","line_discount","line_subtotal","line_tax","line_total","lot_number","expires_on","received_quantity"]})
print("keys:", list(b.keys())[:40])
