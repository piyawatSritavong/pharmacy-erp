#!/usr/bin/env python3
"""Replay a branch's real Ocha POS trading on this system, and compare.

Inputs are the branch's own exports, passed by path and never copied into the
repository (they are real business data):

  --orders  the order list copied from Ocha back office's network panel
            (one or more JSON pages; pages may be separated by "=====" lines)
  --stock   Ocha's stock report (.xlsx, "สต็อคสินค้า") for the same period
  --sales   Ocha's sales history (.xlsx, "ประวัติการขาย") for the same period

Steps (run in order, or `all`):

  analyze    map every Ocha item to a product here; write mapping.json and a
             readable report. Changes nothing.
  seed       on the target system: switch on VAT-inclusive prices, create the
             products Ocha sells that are missing here, set the branch's
             selling prices to what Ocha charged, and set the branch's stock to
             Ocha's opening count (topped up where Ocha sold more than it had).
  replay     ring up every Ocha bill, oldest first, as the branch's till user:
             same items, quantities, prices, bill discount and payment method.
  report     compare bill by bill, the period's money by payment method, and
             closing stock against Ocha.
  checklist  write the same bills as a sheet a person keys in by hand.

Environment: ERP_BASE, DATABASE_URL and the seed passwords, as for erp_api.py.
Point it only at a demo or UAT database, never production.
"""
import argparse
import collections
import datetime as dt
import hashlib
import json
import os
import re
import sys
import zipfile
import xml.etree.ElementTree as ET

sys.path.insert(0, os.path.dirname(__file__))
from erp_api import api, me  # noqa: E402

BANGKOK = dt.timezone(dt.timedelta(hours=7))
OUT = os.environ.get("OCHA_OUT", os.path.join(os.path.dirname(__file__), "ocha_out"))
BRANCH_ACCOUNT = os.environ.get("OCHA_BRANCH_ACCOUNT", "PHH")
# Ocha payment method codes as seen in the order JSON.
CASH, TRANSFER = 1, 11
# Ocha price names that only label the one price a product has.
PLAIN_LABEL = re.compile(r"^(ราคาปกติ|ระดับ\s*\d+|ราคา\s*\d+|ถัง|ชิ้นละ|เครื่อง)$")


def money(value):
    return round(float(value or 0) + 1e-9, 2)


def norm(name):
    return re.sub(r"\s+", "", str(name or "")).lower()


# ---------------------------------------------------------------- inputs

def load_orders(path):
    text = open(path, encoding="utf-8").read()
    text = re.sub(r"^=+\s*$", "", text, flags=re.M)
    decoder, index, orders = json.JSONDecoder(), 0, []
    while index < len(text):
        while index < len(text) and text[index].isspace():
            index += 1
        if index >= len(text):
            break
        page, index = decoder.raw_decode(text, index)
        orders += page.get("orders", [])
    seen, unique = set(), []
    for order in orders:
        key = order["order"]["server_id"]
        if key not in seen and order["order"].get("enabled", True) and order["order"].get("status", 0) == 0:
            seen.add(key)
            unique.append(order)
    unique.sort(key=lambda o: o["order"]["client_time"])
    return unique


def read_xlsx(path):
    ns = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
    archive = zipfile.ZipFile(path)
    shared = []
    if "xl/sharedStrings.xml" in archive.namelist():
        for item in ET.fromstring(archive.read("xl/sharedStrings.xml")).findall("m:si", ns):
            shared.append("".join(t.text or "" for t in item.iter("{%s}t" % ns["m"])))
    rows = []
    sheet = sorted(n for n in archive.namelist() if re.match(r"xl/worksheets/sheet\d+\.xml", n))[0]
    for row in ET.fromstring(archive.read(sheet)).iter("{%s}row" % ns["m"]):
        values = []
        for cell in row.findall("m:c", ns):
            value = cell.find("m:v", ns)
            kind = cell.get("t")
            if kind == "s" and value is not None:
                values.append(shared[int(value.text)])
            elif kind == "inlineStr":
                values.append("".join(t.text or "" for t in cell.iter("{%s}t" % ns["m"])))
            else:
                values.append(value.text if value is not None else "")
        rows.append(values)
    return rows


def load_stock(path):
    """Ocha stock rows: name with its price label, opening count, units sold."""
    rows = []
    for values in read_xlsx(path):
        if len(values) >= 5 and str(values[0]).strip().isdigit():
            rows.append({"name": values[1].strip(), "opening": float(values[2] or 0), "sold": float(values[4] or 0)})
    return rows


def load_sales(path):
    rows = []
    for values in read_xlsx(path):
        if len(values) >= 14 and str(values[0]).strip().isdigit():
            rows.append({"receipt": values[2], "time": values[1], "method": values[6], "total": money(values[8]),
                         "items": money(values[10]), "discount": money(values[11])})
    return rows


def split_label(name):
    """'Walker (ราคา 2)' -> ('Walker', 'ราคา 2'); the label may hold brackets."""
    name = name.strip()
    if not name.endswith(")"):
        return name, ""
    depth = 0
    for index in range(len(name) - 1, -1, -1):
        depth += {")": 1, "(": -1}.get(name[index], 0)
        if depth == 0:
            base = name[:index].strip()
            return (base, name[index + 1:-1].strip()) if base else (name, "")
    return name, ""


# ---------------------------------------------------------------- mapping

def catalog():
    products, page = [], 1
    while True:
        resp = api("central", "GET", "/products", query={"page": page, "page_size": 200})
        items = resp.body.get("items", []) if resp.ok() else []
        products += items
        if len(items) < 200:
            return products
        page += 1


def search_product(name):
    resp = api("central", "GET", "/products", query={"search": name, "page_size": 10})
    items = resp.body.get("items", []) if resp.ok() else []
    exact = [p for p in items if norm(p["name"]) == norm(name)]
    if exact:
        return exact[0]
    return items[0] if len(items) == 1 else None


def build_mapping(orders, stock):
    products = catalog()
    by_name = {norm(p["name"]): p for p in products}

    def resolve(name):
        if norm(name) in by_name:
            return by_name[norm(name)], "exact"
        base, _ = split_label(name)
        if norm(base) in by_name:
            return by_name[norm(base)], "base-name"
        found = search_product(base)
        return (found, "search") if found else (None, "missing")

    # One Ocha product per base name; its labelled rows (sizes, colours, price
    # labels) are one product here, as the catalog merges them.
    items = collections.OrderedDict()

    def entry(base):
        if base not in items:
            items[base] = {"ocha_name": base, "labels": [], "opening": 0.0, "ocha_sold": 0.0, "prices": [],
                           "categories": set(), "lines": 0}
        return items[base]

    for row in stock:
        base, label = split_label(row["name"])
        item = entry(base)
        item["labels"].append(label)
        item["opening"] += row["opening"]
        item["ocha_sold"] += row["sold"]
    for order in orders:
        for line in order["items"]:
            item = entry(line["item_name"].strip())
            label = (line.get("price_name") or "").strip()
            if label and label not in item["labels"]:
                item["labels"].append(label)
            unit = money((line.get("item_price") or {}).get("unit_price") or line["money_nominal"]) or money(line["money_nominal"]) / max(1, line["quantity"])
            item["prices"].append(unit)
            item["categories"].add(line.get("category_name") or "")
            item["lines"] += 1

    mapping = []
    for base, item in items.items():
        product, how = resolve(base)
        prices = collections.Counter(item["prices"])
        mapping.append({
            "ocha_name": base,
            "labels": [label for label in item["labels"] if label],
            "match": how,
            "product_id": product["id"] if product else None,
            "product_name": product["name"] if product else None,
            "system_price": money(product.get("base_selling_price")) if product else None,
            "ocha_price": prices.most_common(1)[0][0] if prices else None,
            "ocha_prices": sorted(prices),
            "opening": item["opening"],
            "ocha_sold": item["ocha_sold"],
            "sold_lines": item["lines"],
            "category": sorted(item["categories"] - {""})[0] if item["categories"] - {""} else "",
        })
    return mapping


def mapping_index(mapping):
    return {norm(row["ocha_name"]): row for row in mapping}


# ---------------------------------------------------------------- steps

def analyze(args, orders, stock, sales):
    mapping = build_mapping(orders, stock)
    save("mapping.json", mapping)
    counts = collections.Counter(row["match"] for row in mapping)
    sold = [row for row in mapping if row["sold_lines"]]
    lines = [
        f"# วิเคราะห์ข้อมูล Ocha · {len(orders)} บิล · รายการสต๊อก {len(stock)} แถว",
        "",
        f"- สินค้า Ocha (รวมตัวเลือกเป็นสินค้าเดียว): {len(mapping)} · ขายในช่วงนี้ {len(sold)}",
        f"- จับคู่ได้: ชื่อตรง {counts['exact']} · ตัดตัวเลือกแล้วตรง {counts['base-name']} · ค้นหาเจอ {counts['search']} · **ไม่มีในระบบ {counts['missing']}**",
        f"- ราคาขายจริงต่างจากราคาในระบบ: {sum(1 for r in sold if r['product_id'] and r['ocha_price'] != r['system_price'])} สินค้า",
        f"- สินค้ามีหลายราคาในช่วงนี้ (ตามตัวเลือก): {sum(1 for r in sold if len(set(r['ocha_prices'])) > 1)}",
        f"- ขายมากกว่าสต๊อกเริ่มต้น (ต้องเติมสต๊อก): {sum(1 for r in mapping if r['ocha_sold'] > r['opening'] or (r['sold_lines'] and r['opening'] == 0))}",
        "",
        "## ไม่มีในระบบ (จะสร้างตอน seed)",
    ]
    lines += [f"- {r['ocha_name']} · {r['category'] or '-'} · ราคา {r['ocha_price'] or '-'} · สต๊อกเริ่ม {r['opening']:g}" for r in mapping if r["match"] == "missing"]
    lines += ["", "## ราคาต่างจากระบบ (จะตั้งราคาสาขาตาม Ocha)"]
    lines += [f"- {r['ocha_name']}: Ocha {r['ocha_prices']} · ระบบ {r['system_price']}" for r in sold if r["product_id"] and r["ocha_price"] != r["system_price"]]
    if sales:
        lines += ["", f"## ประวัติการขาย.xlsx: {len(sales)} บิล ยอดรวม {sum(s['total'] for s in sales):,.2f}"]
    write("analysis.md", "\n".join(lines) + "\n")
    print("\n".join(lines[:9]))
    return mapping


def seed(args, orders, stock, sales):
    mapping = load_json("mapping.json") or analyze(args, orders, stock, sales)
    branch_id = me(BRANCH_ACCOUNT)["user"]["branch_id"]
    issues = []
    resp = api("central", "PUT", "/sales-settings", {"prices_include_vat": True})
    if not resp.ok():
        resp = api("super", "PUT", "/sales-settings", {"prices_include_vat": True})
    must("ราคารวม VAT", resp)

    categories = {norm(c["name"]): c["id"] for c in api("central", "GET", "/product-categories").body.get("items", [])}
    for row in mapping:
        if row["product_id"]:
            continue
        price = row["ocha_price"] or 0
        if not price:
            issues.append(f"ไม่ได้สร้าง {row['ocha_name']}: ไม่รู้ราคา (ไม่มีการขายในช่วงนี้)")
            continue
        sku = "OCHA-" + hashlib.sha256(row["ocha_name"].encode("utf-8")).hexdigest()[:8].upper()
        body = {"sku": sku, "name": row["ocha_name"], "description": "นำเข้าจากข้อมูลขายจริง Ocha",
                "category_id": categories.get(norm(row["category"])), "base_selling_price": price,
                "cost_price": 0, "max_discount_amount": price, "unit_name": "ชิ้น", "sales_channel": "in_store"}
        resp = api("central", "POST", "/products", body)
        if not resp.ok():
            issues.append(f"สร้าง {row['ocha_name']} ไม่สำเร็จ: {resp}")
            continue
        row["product_id"] = resp.body.get("id") or search_product(row["ocha_name"])["id"]
        row["product_name"], row["system_price"], row["created"] = row["ocha_name"], price, True

    discounts = collections.defaultdict(float)
    for order in orders:
        for discount in order.get("discounts", []):
            for share in discount.get("item_discounts", []):
                for line in order["items"]:
                    if line["client_id"] == share["order_item_cid"]:
                        discounts[norm(line["item_name"])] = max(discounts[norm(line["item_name"])], money(share["discounted_value"]))

    for row in mapping:
        if not row["product_id"] or not row["sold_lines"]:
            continue
        settings = {"selling_price": row["ocha_price"]}
        if discounts.get(norm(row["ocha_name"])):
            settings["max_discount_amount"] = max(discounts[norm(row["ocha_name"])], row["ocha_price"] or 0)
        resp = api("central", "PUT", f"/products/{row['product_id']}/branch-settings/{branch_id}", settings)
        if not resp.ok():
            issues.append(f"ตั้งราคา {row['ocha_name']} ไม่สำเร็จ: {resp}")

    # The branch's shelf becomes Ocha's: its opening count, topped up where Ocha
    # sold more than it had, and nothing else on hand.
    targets = {}
    for row in mapping:
        if not row["product_id"]:
            continue
        need = int(round(row["opening"]))
        sold_here = sum(line["quantity"] for order in orders for line in order["items"] if norm(line["item_name"]) == norm(row["ocha_name"]))
        if sold_here > need:
            issues.append(f"เติมสต๊อก {row['ocha_name']}: Ocha เริ่ม {need} แต่ขาย {sold_here}")
            need = sold_here
        targets[row["product_id"]] = targets.get(row["product_id"], 0) + need
        row["target_opening"] = need
    stocked = {}
    page = 1
    while True:
        resp = api("central", "GET", "/inventory", query={"branch_id": branch_id, "page": page, "page_size": 200, "stock_bucket": "real"})
        items = resp.body.get("items", []) if resp.ok() else []
        for item in items:
            stocked[str(item["product_id"])] = int(item.get("qty_real") or 0)
        if len(items) < 200:
            break
        page += 1
    opening_expiry = (dt.datetime.now(BANGKOK) + dt.timedelta(days=365)).strftime("%Y-%m-%d")
    for product_id in set(stocked) | set(targets):
        delta = targets.get(product_id, 0) - stocked.get(product_id, 0)
        if delta == 0:
            continue
        # Ocha keeps no lots or expiry dates: the opening count is one lot,
        # dated a year out where the product must carry an expiry date.
        if delta > 0:
            resp = api("central", "POST", "/inventory/receive", {
                "product_id": product_id, "branch_id": branch_id, "real_quantity": delta, "note": "ยอดยกมาตามสต๊อก Ocha",
                "lot_number": "OCHA-OPENING", "expires_on": opening_expiry, "unit_cost": 0})
        else:
            resp = api("central", "POST", "/inventory/adjust", {
                "product_id": product_id, "branch_id": branch_id, "stock_bucket": "real", "quantity_delta": delta,
                "reason": "ยอดยกมาตามสต๊อก Ocha"})
        if not resp.ok():
            issues.append(f"ปรับสต๊อก {product_id} ({delta:+d}) ไม่สำเร็จ: {resp}")
    save("mapping.json", mapping)
    save("seed_issues.json", issues)
    print(f"seed: {sum(1 for r in mapping if r.get('created'))} products created · {len(targets)} stocked · {len(issues)} notes")


def lines_for(order, index):
    lines = []
    for line in order["items"]:
        row = index.get(norm(line["item_name"]))
        unit = money((line.get("item_price") or {}).get("unit_price")) or money(line["money_nominal"]) / max(1, line["quantity"])
        lines.append({"row": row, "name": line["item_name"], "label": (line.get("price_name") or "").strip(),
                      "quantity": int(line["quantity"]), "unit": unit, "nominal": money(line["money_nominal"])})
    return lines


def order_payment(order):
    methods = collections.defaultdict(float)
    for payment in order.get("payments", []):
        for method in payment.get("order_payment_methods", []):
            methods[int(method["payment_type"])] += money(method["money_collected"] or method["money_to_pay"])
    return methods


def bill_discount(order):
    return money(sum(money(d.get("discounted_value") or d.get("value")) * (d.get("quantity") or 1) if d.get("discount_type") == 1
                     else money(d.get("discounted_value")) for d in order.get("discounts", [])))


def receipt_of(order):
    payments = order.get("payments") or [{}]
    return payments[0].get("receipt_number_v2") or str(order["order"]["server_id"])


def replay(args, orders, stock, sales):
    mapping = load_json("mapping.json")
    if not mapping:
        sys.exit("run analyze and seed first")
    index = mapping_index(mapping)
    branch_id = me(BRANCH_ACCOUNT)["user"]["branch_id"]
    results = []
    for order in orders:
        receipt = receipt_of(order)
        when = dt.datetime.fromtimestamp(order["order"]["client_time"], BANGKOK).strftime("%d/%m/%Y %H:%M")
        expected = money(order["order"]["money_payable"])
        record = {"receipt": receipt, "time": when, "expected": expected, "issues": []}
        items = []
        for line in lines_for(order, index):
            if not line["row"] or not line["row"].get("product_id"):
                record["issues"].append(f"ไม่มีสินค้า {line['name']}")
                continue
            product_id = line["row"]["product_id"]
            lots = api(BRANCH_ACCOUNT, "GET", "/sales/lot-options", query={"branch_id": branch_id, "product_id": product_id, "stock_bucket": "real"})
            # The opening lot holds the branch's whole count; lot options come
            # oldest-expiry first.
            lot = next(iter(lots.body.get("items", []) if lots.ok() else []), None)
            if not lot:
                record["issues"].append(f"ไม่มี Lot พอขาย {line['name']}")
                continue
            item = {"product_id": product_id, "inventory_lot_id": lot["id"], "quantity": line["quantity"], "stock_bucket": "real"}
            if line["unit"] != money(line["row"].get("ocha_price")):
                # A labelled option (a size, a colour) priced apart from the
                # product's usual price.
                item["override_unit_price"] = line["unit"]
                item["override_reason"] = f"ราคาตามตัวเลือก Ocha: {line['label'] or '-'}"
                record["issues"].append(f"ราคา {line['name']} {line['label']} = {line['unit']:g} (ใช้แก้ราคา)")
            items.append(item)
        if not items:
            record["status"] = "skipped"
            results.append(record)
            continue
        body = {"branch_id": branch_id, "items": items, "notes": f"Ocha {receipt} · {when}"}
        discount = bill_discount(order)
        if discount:
            body["bill_discount_amount"] = discount
        preview = api(BRANCH_ACCOUNT, "POST", "/invoices/preview", body)
        if not preview.ok():
            record.update(status="preview-failed", error=str(preview))
            results.append(record)
            continue
        total = money(preview.body["summary"]["total_amount"])
        methods = order_payment(order)
        cash, transfer = money(methods.get(CASH, 0)), money(methods.get(TRANSFER, 0))
        if cash and transfer:
            body.update(payment_type="mixed", transfer_amount=money(min(transfer, total - 0.01)), tendered_amount=money(total - min(transfer, total - 0.01)))
        elif transfer:
            body.update(payment_type="bank_transfer")
        else:
            body.update(payment_type="cash", tendered_amount=total)
        resp = api(BRANCH_ACCOUNT, "POST", "/pos/checkout", body)
        if not resp.ok():
            record.update(status="checkout-failed", error=str(resp), actual=total)
        else:
            record.update(status="ok", actual=money(resp.body.get("total_amount", total)), invoice_id=resp.body.get("invoice_id"),
                          invoice_number=resp.body.get("invoice_number"), payment=body["payment_type"],
                          cash=cash, transfer=transfer, discount=discount)
        record["match"] = record.get("actual") == expected
        results.append(record)
        print(f"{receipt} {when} expected {expected:>10,.2f} actual {record.get('actual', 0):>10,.2f} {record['status']}{'' if record['match'] else '  ≠'}")
    save("replay.json", results)


def report(args, orders, stock, sales):
    results = load_json("replay.json") or []
    mapping = load_json("mapping.json") or []
    branch_id = me(BRANCH_ACCOUNT)["user"]["branch_id"]
    ok = [r for r in results if r.get("status") == "ok"]
    matched = [r for r in ok if r.get("match")]
    expected_total = money(sum(money(o["order"]["money_payable"]) for o in orders))
    actual_total = money(sum(r["actual"] for r in ok))
    by_method = collections.defaultdict(lambda: [0.0, 0.0])
    for r, order in zip(results, orders):
        if r.get("status") != "ok":
            continue
        methods = order_payment(order)
        by_method["เงินสด"][0] += methods.get(CASH, 0)
        by_method["โอน"][0] += methods.get(TRANSFER, 0)
    invoice_ids = [r["invoice_id"] for r in ok if r.get("invoice_id")]
    if invoice_ids:
        detail = {}
        for invoice_id in invoice_ids:
            resp = api("central", "GET", f"/invoices/{invoice_id}/print")
            if resp.ok():
                for payment in resp.body.get("payments", []) or []:
                    kind = "เงินสด" if payment.get("payment_type") == "cash" else "โอน"
                    detail[kind] = detail.get(kind, 0) + money(payment.get("amount"))
        for kind, amount in detail.items():
            by_method[kind][1] += amount
    sales_total = money(sum(s["total"] for s in sales)) if sales else None

    stock_rows = []
    for row in mapping:
        if not row.get("product_id") or not row.get("target_opening") and not row.get("sold_lines"):
            continue
        sold = sum(l["quantity"] for o in orders for l in o["items"] if norm(l["item_name"]) == norm(row["ocha_name"]))
        resp = api("central", "GET", "/inventory", query={"branch_id": branch_id, "search": row["product_name"], "page_size": 5, "stock_bucket": "real"})
        qty = next((int(i.get("qty_real") or 0) for i in (resp.body.get("items", []) if resp.ok() else []) if str(i["product_id"]) == row["product_id"]), None)
        expected_left = int(row.get("target_opening", row["opening"])) - sold
        if sold:
            stock_rows.append((row["ocha_name"], int(row["opening"]), sold, expected_left, qty))

    lines = [
        f"# ผลเล่นซ้ำบิลจริง Ocha · สาขา {BRANCH_ACCOUNT}",
        "",
        f"- บิลทั้งหมด {len(orders)} · ขายสำเร็จ {len(ok)} · **ยอดตรงทุกสตางค์ {len(matched)}/{len(orders)}**",
        f"- ยอดรวม Ocha {expected_total:,.2f} · ระบบใหม่ {actual_total:,.2f} · ต่าง {actual_total - expected_total:,.2f}"
        + (f" · ประวัติการขาย.xlsx {sales_total:,.2f}" if sales_total is not None else ""),
        "",
        "| วิธีจ่าย | Ocha | ระบบใหม่ |",
        "| --- | ---: | ---: |",
    ]
    lines += [f"| {k} | {v[0]:,.2f} | {v[1]:,.2f} |" for k, v in by_method.items()]
    lines += ["", "## บิลที่ไม่ตรงหรือมีหมายเหตุ", "", "| เลขที่ Ocha | เวลา | Ocha | ระบบใหม่ | สถานะ | หมายเหตุ |", "| --- | --- | ---: | ---: | --- | --- |"]
    for r in results:
        if r.get("match") and not r.get("issues"):
            continue
        lines.append(f"| {r['receipt']} | {r['time']} | {r['expected']:,.2f} | {r.get('actual', 0):,.2f} | {r.get('status')} | {'; '.join(r.get('issues', []))} {r.get('error', '')[:120]} |")
    lines += ["", "## สต๊อกสินค้าที่ขาย (ยกมา − ขาย = คงเหลือ)", "", "| สินค้า | ยกมา (Ocha) | ขาย | ควรเหลือ | ระบบใหม่เหลือ |", "| --- | ---: | ---: | ---: | ---: |"]
    lines += [f"| {n} | {o} | {s} | {e} | {q if q is not None else '-'}{'' if q == e else ' ≠'} |" for n, o, s, e, q in stock_rows]
    write("report.md", "\n".join(lines) + "\n")
    print("\n".join(lines[:5]))


def checklist(args, orders, stock, sales):
    mapping = load_json("mapping.json") or []
    index = mapping_index(mapping)
    lines = ["# ใบงานทดสอบมือ: คีย์บิลจริงของ Ocha ลงระบบใหม่", "",
             "เข้าระบบด้วยบัญชีหน้าร้านของสาขา แล้วคีย์ทีละบิลตามลำดับ เทียบ **ยอดที่ต้องได้** กับหน้าจอ · จดเวลาและปัญหาในช่องหมายเหตุ", ""]
    for number, order in enumerate(orders, 1):
        when = dt.datetime.fromtimestamp(order["order"]["client_time"], BANGKOK).strftime("%d/%m/%Y %H:%M")
        methods = order_payment(order)
        pay = " + ".join(f"{'เงินสด' if k == CASH else 'โอน'} {v:,.2f}" for k, v in methods.items())
        lines.append(f"## {number}. บิล Ocha {receipt_of(order)} · {when}")
        for line in lines_for(order, index):
            name = (line["row"] or {}).get("product_name") or line["name"]
            label = f" ({line['label']})" if line["label"] else ""
            lines.append(f"- [ ] {name}{label} × {line['quantity']} @ {line['unit']:,.2f}")
        discount = bill_discount(order)
        if discount:
            lines.append(f"- [ ] ส่วนลดท้ายบิล {discount:,.2f}")
        lines.append(f"- [ ] รับชำระ: {pay}")
        lines.append(f"- **ยอดที่ต้องได้ {money(order['order']['money_payable']):,.2f}** · เวลาที่ใช้ ____ วินาที · จำนวนแตะ ____ · หมายเหตุ ______")
        lines.append("")
    write("checklist.md", "\n".join(lines))
    print(f"checklist: {len(orders)} bills → {os.path.join(OUT, 'checklist.md')}")


# ---------------------------------------------------------------- helpers

def must(label, resp):
    if not resp.ok():
        sys.exit(f"{label}: {resp}")


def save(name, data):
    os.makedirs(OUT, exist_ok=True)
    with open(os.path.join(OUT, name), "w", encoding="utf-8") as handle:
        json.dump(data, handle, ensure_ascii=False, indent=1, default=lambda value: sorted(value) if isinstance(value, set) else str(value))


def write(name, text):
    os.makedirs(OUT, exist_ok=True)
    with open(os.path.join(OUT, name), "w", encoding="utf-8") as handle:
        handle.write(text)


def load_json(name):
    path = os.path.join(OUT, name)
    return json.load(open(path, encoding="utf-8")) if os.path.exists(path) else None


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("step", choices=["analyze", "seed", "replay", "report", "checklist", "all"])
    parser.add_argument("--orders", required=True)
    parser.add_argument("--stock", required=True)
    parser.add_argument("--sales")
    args = parser.parse_args()
    orders = load_orders(args.orders)
    stock = load_stock(args.stock)
    sales = load_sales(args.sales) if args.sales else []
    steps = ["analyze", "seed", "replay", "report", "checklist"] if args.step == "all" else [args.step]
    for step in steps:
        globals()[step](args, orders, stock, sales)


if __name__ == "__main__":
    main()
