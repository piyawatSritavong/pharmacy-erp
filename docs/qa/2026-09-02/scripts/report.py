import json, collections, datetime
rows = [json.loads(l) for l in open("results.jsonl", encoding="utf-8") if l.strip()]
# de-dup by tc keeping last
seen = {}
for r in rows: seen[r["tc"]] = r
rows = list(seen.values())
order = {"FAIL":0,"INFO":1,"BLOCKED":2,"UNTESTABLE":3,"PASS":4}
cnt = collections.Counter(r["status"] for r in rows)
by_area = collections.OrderedDict()
for r in rows:
    by_area.setdefault(r["area"], []).append(r)
def esc(s): return str(s).replace("|","\\|").replace("\n"," ")
out = []
out.append(f"# รายงานผลทดสอบระบบ Pharmacy ERP (QA เต็มระบบ)\n")
out.append(f"วันที่ทดสอบ: 2026-09-02 · ผู้ทดสอบ: Claude (Senior Tester) · สภาพแวดล้อม: backend Go (local, seed สด) + frontend next dev + PostgreSQL 16 แยก (port 55440)\n")
out.append(f"บัญชีที่ใช้: superadmin@erp.local, admin.central@erp.local, admin.mes@erp.local, pos.* ทั้ง 6 สาขา (MES/PHH/PHS/NPT/KNP/WH)\n")
out.append("## สรุปภาพรวม\n")
out.append("| สถานะ | จำนวน |\n|---|---|")
for k in ["PASS","FAIL","INFO","BLOCKED","UNTESTABLE"]:
    if cnt.get(k): out.append(f"| {k} | {cnt[k]} |")
out.append(f"| รวม | {len(rows)} |\n")
out.append("## ผลรายละเอียดตามหมวด\n")
for area, items in by_area.items():
    c = collections.Counter(i["status"] for i in items)
    out.append(f"### {area} — PASS {c.get('PASS',0)} / FAIL {c.get('FAIL',0)} / INFO {c.get('INFO',0)}\n")
    out.append("| TC | บทบาท | รายการทดสอบ | สถานะ | ผลที่คาด | ผลจริง / หมายเหตุ |\n|---|---|---|---|---|---|")
    for i in sorted(items, key=lambda x: (order.get(x['status'],9), x['tc'])):
        note = i['actual'] + ((" — " + i['notes']) if i.get('notes') else "")
        out.append(f"| {i['tc']} | {esc(i['role'])} | {esc(i['title'])} | **{i['status']}** | {esc(i['expected'])} | {esc(note)} |")
    out.append("")
open("QA_REPORT.md","w",encoding="utf-8").write("\n".join(out))
print(json.dumps(cnt), len(rows), "areas:", len(by_area))
