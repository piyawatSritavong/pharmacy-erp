"""Many branches selling at once, then every number checked back to the bills.

Runs against a disposable database (never production):

  PYTHONPATH=scripts/qa python3 scripts/qa/multi_branch_reconcile.py [--sales 30] [--threads 2]

Phase 1 — load: every selling branch's till rings up sales on its own threads
at the same time, while head office sells remotely, takes money owed on credit
bills, moves stock between branches, and members spend points at several
branches at once. Each successful sale is recorded on the client side.

Phase 2 — races aimed at the places money or stock could be double-counted:
the last units of one lot, one member's points, one credit line, one remote
cart paid twice, one debt settled twice.

Phase 3 — reconcile: client records = bills in the database = dashboard =
branch summaries = exports = tax report; stock = lots = movements; points =
ledger; credit within limit; document numbers unique and gap-free.

Writes a JSON report next to this script (multi_branch_report.json) and exits
non-zero if any check fails.
"""
import argparse
import collections
import concurrent.futures
import io
import json
import math
import os
import random
import re
import threading
import time
import zipfile
from xml.etree import ElementTree

from erp_api import api, me, sql, sql_value

SELLING = ["MES", "PHH", "PHS", "NPT", "KNP"]
lock = threading.Lock()
records = []          # successful sales seen by the clients
collections_made = [] # successful receivable payments
outcomes = collections.Counter()
unexpected = []       # responses that should never happen (5xx, malformed)
checks = []


def check(name, ok, detail=""):
    checks.append({"check": name, "ok": bool(ok), "detail": detail if not ok else ""})
    print(("PASS " if ok else "FAIL ") + name + ("" if ok else "  :: " + str(detail)[:600]))


def note_unexpected(label, resp):
    with lock:
        unexpected.append({"label": label, "status": resp.status, "body": str(resp.body)[:300]})


def money(value):
    return round(float(value or 0) + 1e-9, 2)


# ------------------------------------------------------------------ fixtures

def branch_ids():
    return {code: bid for bid, code in sql("SELECT id, code FROM branches WHERE active")}


def sellable_lots(branch_id):
    rows = sql(f"""
        SELECT il.id, il.product_id, il.remaining_quantity,
               COALESCE((SELECT pu.id::text FROM product_units pu WHERE pu.product_id = il.product_id AND NOT pu.is_base AND pu.active LIMIT 1), ''),
               COALESCE((SELECT pu.conversion_qty FROM product_units pu WHERE pu.product_id = il.product_id AND NOT pu.is_base AND pu.active LIMIT 1), 0)
        FROM inventory_lots il JOIN products p ON p.id = il.product_id
        WHERE il.branch_id = '{branch_id}' AND il.stock_bucket = 'real' AND il.remaining_quantity > 0 AND p.active
          AND (il.expires_on IS NULL OR il.expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)
        ORDER BY il.remaining_quantity DESC
        LIMIT 60""")
    return [{"lot": r[0], "product": r[1], "remaining": int(r[2]), "pack_unit": r[3], "pack_size": int(r[4] or 0)} for r in rows]


def customers_pool():
    members = [r[0] for r in sql("SELECT id FROM customers WHERE active AND customer_type = 'person' ORDER BY customer_code")]
    accounts = [r[0] for r in sql("SELECT id FROM customers WHERE active AND credit_limit > 0 ORDER BY customer_code")]
    return members, accounts


# ------------------------------------------------------------------ one sale

def ring_up(account, branch_id, branch_code, lots, members, accounts, rng, label="pos"):
    lines = []
    for lot in rng.sample(lots, k=min(len(lots), rng.randint(1, 3))):
        use_pack = lot["pack_unit"] and lot["remaining"] >= lot["pack_size"] * 2 and rng.random() < 0.25
        lines.append({
            "product_id": lot["product"], "inventory_lot_id": lot["lot"], "stock_bucket": "real",
            "unit_id": lot["pack_unit"] if use_pack else "",
            "quantity": 1 if use_pack else rng.randint(1, 3),
        })
    body = {"branch_id": branch_id, "items": lines}
    roll = rng.random()
    payment = rng.choice(["cash", "cash", "bank_transfer", "mixed"])
    if roll < 0.12 and accounts:
        body["customer_id"] = rng.choice(accounts)
        payment = "credit"
    elif roll < 0.55 and members:
        body["customer_id"] = rng.choice(members)
        if rng.random() < 0.3:
            body["redeem_points"] = 40
    if rng.random() < 0.15:
        body["bill_discount_amount"] = 5
    preview = api(account, "POST", "/invoices/preview", body)
    if not preview.ok():
        with lock:
            outcomes[f"preview {preview.status}"] += 1
        if preview.status >= 500:
            note_unexpected(f"{label} preview", preview)
        return None
    total = money(preview.body["summary"]["total_amount"])
    body["payment_type"] = payment
    if payment == "cash":
        body["tendered_amount"] = math.ceil(total / 100) * 100 or 100
    elif payment == "mixed":
        if total < 0.02:
            body["payment_type"] = "cash"
            body["tendered_amount"] = total
        else:
            transfer = money(math.floor(total * 50) / 100)
            body["transfer_amount"] = transfer
            body["tendered_amount"] = money(total - transfer)
    endpoint = "/admin/pos/checkout" if account in ("central", "super") else "/pos/checkout"
    resp = api(account, "POST", endpoint, body)
    key = f"{label} checkout {resp.status}"
    with lock:
        outcomes[key] += 1
    if resp.status >= 500 or (resp.ok() and not resp.body.get("invoice_id")):
        note_unexpected(f"{label} checkout", resp)
        return None
    if not resp.ok():
        return None
    record = {
        "branch": branch_code, "invoice_id": resp.body["invoice_id"], "invoice_number": resp.body["invoice_number"],
        "total": money(resp.body["total_amount"]), "cash": money(resp.body.get("cash_amount")),
        "transfer": money(resp.body.get("transfer_amount")), "sale_type": resp.body.get("sale_type", "cash"),
        "customer_id": body.get("customer_id", ""), "points_earned": resp.body.get("points_earned", 0),
        "points_redeemed": resp.body.get("points_redeemed", 0), "previewed_total": total, "via": label,
    }
    with lock:
        records.append(record)
    return record


def branch_worker(code, branch_id, sales, seed, members, accounts):
    rng = random.Random(seed)
    lots = sellable_lots(branch_id)
    if not lots:
        return
    for _ in range(sales):
        ring_up(code, branch_id, code, lots, members, accounts, rng)
        if rng.random() < 0.1:
            lots = sellable_lots(branch_id)


def central_worker(branches, sales, members, accounts):
    rng = random.Random(99)
    for index in range(sales):
        code = SELLING[index % len(SELLING)]
        lots = sellable_lots(branches[code])
        if lots:
            ring_up("central", branches[code], code, lots, members, accounts, rng, label="hq")


def collector_worker(accounts, rounds):
    rng = random.Random(7)
    for _ in range(rounds):
        time.sleep(0.3)
        account = rng.choice(accounts)
        owed = money(sql_value(f"""
            SELECT COALESCE(SUM(i.total_amount - COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id = i.id), 0)), 0)
            FROM invoices i WHERE i.customer_id = '{account}' AND i.payment_status <> 'paid' AND i.deleted_at IS NULL"""))
        if owed <= 0:
            continue
        amount = money(min(owed, rng.choice([owed, owed / 2, 100])))
        if amount <= 0:
            continue
        payer = rng.choice(["central", "MES", "PHH"])
        resp = api(payer, "POST", "/receivables/payments", {"customer_id": account, "payment_type": rng.choice(["cash", "bank_transfer"]), "amount": amount})
        with lock:
            outcomes[f"collect {resp.status}"] += 1
        if resp.ok():
            with lock:
                collections_made.append({"customer_id": account, "amount": money(resp.body["amount"]), "by": payer})
        elif resp.status >= 500:
            note_unexpected("collect", resp)


def transfer_worker(branches):
    """Moves stock MES → PHH while both branches are selling it."""
    lots = sellable_lots(branches["MES"])[:3]
    if not lots:
        return
    created = api("central", "POST", "/transfers", {
        "source_branch_id": branches["MES"], "destination_branch_id": branches["PHH"], "request_note": "QA โอนระหว่างขาย",
        "items": [{"product_id": lot["product"], "quantity": 1, "stock_bucket": "real"} for lot in lots]})
    with lock:
        outcomes[f"transfer create {created.status}"] += 1
    if not created.ok():
        if created.status >= 500:
            note_unexpected("transfer create", created)
        return
    transfer_id = created.body["id"]
    dispatched = api("central", "POST", f"/transfers/{transfer_id}/dispatch", {"pickup_name": "QA", "courier_name": "QA"})
    with lock:
        outcomes[f"transfer dispatch {dispatched.status}"] += 1
    if not dispatched.ok():
        if dispatched.status >= 500:
            note_unexpected("transfer dispatch", dispatched)
        return
    items = sql(f"SELECT id, quantity FROM transfer_items WHERE transfer_id = '{transfer_id}'")
    received = api("PHH", "POST", f"/transfers/{transfer_id}/receive", {"items": [{"item_id": i, "received_quantity": int(q), "discrepancy_note": ""} for i, q in items]})
    with lock:
        outcomes[f"transfer receive {received.status}"] += 1
    if received.status >= 500:
        note_unexpected("transfer receive", received)


# ------------------------------------------------------------------ races

def race(count, fn):
    with concurrent.futures.ThreadPoolExecutor(max_workers=count) as pool:
        return list(pool.map(lambda i: fn(i), range(count)))


def race_last_units(branches):
    """N+3 tills fight over a lot with N units left: exactly N may sell."""
    branch_id = branches["MES"]
    row = sql(f"""SELECT il.id, il.product_id, il.remaining_quantity FROM inventory_lots il
                  WHERE il.branch_id = '{branch_id}' AND il.stock_bucket = 'real' AND il.remaining_quantity BETWEEN 2 AND 6
                    AND (il.expires_on IS NULL OR il.expires_on >= CURRENT_DATE)
                  ORDER BY il.remaining_quantity LIMIT 1""")
    if not row:
        check("race: last units of a lot", False, "no small lot to race on")
        return
    lot, product, remaining = row[0][0], row[0][1], int(row[0][2])
    before_inventory = int(sql_value(f"SELECT qty_real FROM inventory WHERE branch_id = '{branch_id}' AND product_id = '{product}'"))

    def sell(_):
        return api("MES", "POST", "/pos/checkout", {"branch_id": branch_id, "payment_type": "bank_transfer",
                   "items": [{"product_id": product, "inventory_lot_id": lot, "quantity": 1, "stock_bucket": "real", "unit_id": ""}]})
    results = race(remaining + 3, sell)
    sold = [r for r in results if r.ok()]
    for r in sold:
        records.append({"branch": "MES", "invoice_id": r.body["invoice_id"], "invoice_number": r.body["invoice_number"],
                        "total": money(r.body["total_amount"]), "cash": 0, "transfer": money(r.body["transfer_amount"]),
                        "sale_type": "cash", "customer_id": "", "points_earned": 0, "points_redeemed": 0, "via": "race"})
    errors = [r for r in results if not r.ok()]
    left = int(sql_value(f"SELECT remaining_quantity FROM inventory_lots WHERE id = '{lot}'"))
    after_inventory = int(sql_value(f"SELECT qty_real FROM inventory WHERE branch_id = '{branch_id}' AND product_id = '{product}'"))
    check("race: last units of a lot sell exactly once each",
          len(sold) == remaining and left == 0 and all(r.status == 409 for r in errors) and after_inventory == before_inventory - remaining,
          {"remaining": remaining, "sold": len(sold), "left": left, "error_statuses": [r.status for r in errors],
           "inventory": [before_inventory, after_inventory]})


def race_points(branches):
    """One member, enough points for exactly one redemption, five tills at once."""
    member = sql_value("SELECT id FROM customers WHERE customer_type = 'person' AND active ORDER BY customer_code DESC LIMIT 1")
    balance = int(sql_value(f"SELECT points_balance FROM customers WHERE id = '{member}'"))
    target = 60  # one redemption of 40 fits, two do not
    adjust = api("central", "POST", f"/customers/{member}/points", {"points": target - balance, "note": "QA race setup"}) if balance != target else None
    if adjust is not None and not adjust.ok():
        check("race: points spent once", False, adjust)
        return
    tills = ["MES", "PHH", "PHS", "NPT", "KNP"]
    lots = {code: sellable_lots(branches[code]) for code in tills}

    def spend(i):
        code = tills[i]
        if not lots[code]:
            return None
        lot = max(lots[code], key=lambda l: l["remaining"])
        return code, api(code, "POST", "/pos/checkout", {"branch_id": branches[code], "customer_id": member, "redeem_points": 40,
                         "payment_type": "bank_transfer",
                         "items": [{"product_id": lot["product"], "inventory_lot_id": lot["lot"], "quantity": 1, "stock_bucket": "real", "unit_id": ""}]})
    results = [r for r in race(len(tills), spend) if r]
    won = [(code, r) for code, r in results if r.ok()]
    for code, r in won:
        records.append({"branch": code, "invoice_id": r.body["invoice_id"], "invoice_number": r.body["invoice_number"],
                        "total": money(r.body["total_amount"]), "cash": 0, "transfer": money(r.body["transfer_amount"]),
                        "sale_type": "cash", "customer_id": member, "points_earned": r.body.get("points_earned", 0),
                        "points_redeemed": 40, "via": "race"})
    final = int(sql_value(f"SELECT points_balance FROM customers WHERE id = '{member}'"))
    ledger = int(sql_value(f"SELECT COALESCE(SUM(points), 0) FROM loyalty_point_entries WHERE customer_id = '{member}'"))
    earned = sum(r.body.get("points_earned", 0) for _, r in won)
    check("race: one member's points spent at most once across five branches",
          len(won) == 1 and final == target - 40 + earned and final == ledger and all(r.status in (400, 409) for _, r in results if not r.ok()),
          {"winners": [c for c, _ in won], "final": final, "ledger": ledger, "statuses": [r.status for _, r in results]})


def race_credit(branches):
    """A credit line with room for two bills; six tills try at once."""
    created = api("central", "POST", "/customers", {"name": "QA วงเงินแข่งกัน %d" % int(time.time()), "customer_type": "business",
                  "phone": "07%08d" % (int(time.time() * 1000) % 100000000), "price_tier": "retail", "credit_limit": 1, "credit_days": 7})
    if not created.ok():
        check("race: credit line never exceeded", False, created)
        return
    account = created.body["id"]
    lot = max(sellable_lots(branches["PHS"]), key=lambda l: l["remaining"])
    item = {"product_id": lot["product"], "inventory_lot_id": lot["lot"], "quantity": 1, "stock_bucket": "real", "unit_id": ""}
    one_bill = money(api("PHS", "POST", "/invoices/preview", {"branch_id": branches["PHS"], "customer_id": account, "items": [item]}).body["summary"]["total_amount"])
    limit = money(one_bill * 2.5)
    api("central", "PUT", f"/customers/{account}", {"name": "QA วงเงินแข่งกัน", "customer_type": "business", "credit_limit": limit, "credit_days": 7})

    def buy(_):
        return api("PHS", "POST", "/pos/checkout", {"branch_id": branches["PHS"], "customer_id": account, "payment_type": "credit", "items": [item]})
    results = race(6, buy)
    won = [r for r in results if r.ok()]
    for r in won:
        records.append({"branch": "PHS", "invoice_id": r.body["invoice_id"], "invoice_number": r.body["invoice_number"],
                        "total": money(r.body["total_amount"]), "cash": 0, "transfer": 0, "sale_type": "credit",
                        "customer_id": account, "points_earned": r.body.get("points_earned", 0), "points_redeemed": 0, "via": "race"})
    owed = money(sql_value(f"SELECT COALESCE(SUM(total_amount), 0) FROM invoices WHERE customer_id = '{account}' AND deleted_at IS NULL"))
    check("race: six tills cannot push a credit line past its limit",
          len(won) == 2 and owed <= limit and all(r.status == 409 for r in results if not r.ok()),
          {"limit": limit, "bill": one_bill, "won": len(won), "owed": owed, "statuses": [r.status for r in results]})


def race_remote_double_pay(branches):
    """Head office leaves a cart at NPT's till; the till's pay button fires twice."""
    lot = max(sellable_lots(branches["NPT"]), key=lambda l: l["remaining"])
    saved = api("central", "PUT", "/admin/pos/remote-session", {"branch_id": branches["NPT"], "session_id": "", "cart_version": 0, "cart": {
        "lines": [{"product_id": lot["product"], "product_name": "QA", "sku": "QA", "inventory_lot_id": lot["lot"], "lot_number": "",
                   "stock_bucket": "real", "quantity": 1, "unit_id": "", "unit_price": 0, "discount_amount": 0}],
        "bill_discount_amount": 0, "full_tax_invoice": False, "customer_name": "", "customer_tax_id": "", "is_government_mode": False, "notes": "QA"}})
    if not saved.ok():
        check("race: a remote cart paid twice makes one bill", False, saved)
        return
    session_id = saved.body["id"]
    before = int(sql_value(f"SELECT COUNT(*) FROM invoices WHERE branch_id = '{branches['NPT']}'"))

    def pay(_):
        return api("NPT", "POST", "/pos/remote-session/checkout", {"session_id": session_id, "payment_type": "bank_transfer"})
    results = race(4, pay)
    after = int(sql_value(f"SELECT COUNT(*) FROM invoices WHERE branch_id = '{branches['NPT']}'"))
    ids = {r.body.get("invoice_id") for r in results if r.ok()}
    for r in results:
        if r.ok() and r.body.get("invoice_id") in ids:
            ids.discard(r.body["invoice_id"])
            records.append({"branch": "NPT", "invoice_id": r.body["invoice_id"], "invoice_number": r.body["invoice_number"],
                            "total": money(r.body["total_amount"]), "cash": 0, "transfer": money(r.body.get("transfer_amount")),
                            "sale_type": "cash", "customer_id": "", "points_earned": 0, "points_redeemed": 0, "via": "remote"})
    distinct = {r.body.get("invoice_id") for r in results if r.ok()}
    check("race: a remote cart paid four times at once makes exactly one bill",
          after - before == 1 and len(distinct) == 1 and all(r.status in (200, 201, 409) for r in results),
          {"new_invoices": after - before, "statuses": [r.status for r in results], "distinct_ids": len(distinct)})


def race_double_collect(branches):
    """Two tills settle the same debt in full at the same moment."""
    account = sql_value("SELECT id FROM customers WHERE credit_limit >= 10000 AND active ORDER BY customer_code LIMIT 1")
    lot = max(sellable_lots(branches["MES"]), key=lambda l: l["remaining"])
    sale = api("MES", "POST", "/pos/checkout", {"branch_id": branches["MES"], "customer_id": account, "payment_type": "credit",
               "items": [{"product_id": lot["product"], "inventory_lot_id": lot["lot"], "quantity": 1, "stock_bucket": "real", "unit_id": ""}]})
    if not sale.ok():
        check("race: one debt cannot be collected twice", False, sale)
        return
    records.append({"branch": "MES", "invoice_id": sale.body["invoice_id"], "invoice_number": sale.body["invoice_number"],
                    "total": money(sale.body["total_amount"]), "cash": 0, "transfer": 0, "sale_type": "credit",
                    "customer_id": account, "points_earned": sale.body.get("points_earned", 0), "points_redeemed": 0, "via": "race"})
    owed = money(sql_value(f"""SELECT COALESCE(SUM(i.total_amount - COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id = i.id), 0)), 0)
                              FROM invoices i WHERE i.customer_id = '{account}' AND i.payment_status <> 'paid' AND i.deleted_at IS NULL"""))

    def collect(i):
        return api(["MES", "PHH"][i], "POST", "/receivables/payments", {"customer_id": account, "payment_type": "cash", "amount": owed})
    results = race(2, collect)
    for r in results:
        if r.ok():
            collections_made.append({"customer_id": account, "amount": money(r.body["amount"]), "by": "race"})
    left = money(sql_value(f"""SELECT COALESCE(SUM(i.total_amount - COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id = i.id), 0)), 0)
                              FROM invoices i WHERE i.customer_id = '{account}' AND i.deleted_at IS NULL"""))
    check("race: a debt settled from two branches at once is collected once",
          sum(1 for r in results if r.ok()) == 1 and left == 0 and all(r.status == 400 for r in results if not r.ok()),
          {"owed": owed, "statuses": [r.status for r in results], "left": left})


# ------------------------------------------------------------------ reconcile

def xlsx_cells(blob):
    with zipfile.ZipFile(io.BytesIO(blob)) as archive:
        ns = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
        strings = []
        if "xl/sharedStrings.xml" in archive.namelist():
            root = ElementTree.fromstring(archive.read("xl/sharedStrings.xml"))
            strings = ["".join(t.text or "" for t in si.iter("{%s}t" % ns["m"])) for si in root.findall("m:si", ns)]
        sheet = ElementTree.fromstring(archive.read("xl/worksheets/sheet1.xml"))
        cells = {}
        for cell in sheet.iter("{%s}c" % ns["m"]):
            value = cell.find("m:v", ns)
            inline = cell.find("m:is", ns)
            if value is not None:
                text = value.text
                if cell.get("t") == "s":
                    text = strings[int(text)]
            elif inline is not None:
                text = "".join(t.text or "" for t in inline.iter("{%s}t" % ns["m"]))
            else:
                continue
            cells[cell.get("r")] = text
        return cells


def reconcile(branches, start, bangkok_today, branch_before):
    by_branch = collections.defaultdict(list)
    for record in records:
        by_branch[record["branch"]].append(record)

    # 1. every bill the clients saw exists once, with the same total
    ids = [r["invoice_id"] for r in records]
    check("every client-side sale is a distinct bill", len(ids) == len(set(ids)), len(ids) - len(set(ids)))
    db_rows = {r[0]: r for r in sql(f"""SELECT i.id, b.code, i.total_amount, i.subtotal, i.tax_amount, i.payment_status, i.sale_type
                                        FROM invoices i JOIN branches b ON b.id = i.branch_id
                                        WHERE i.created_at >= '{start}' AND i.deleted_at IS NULL AND i.invoice_status = 'issued'""")}
    missing = [r["invoice_number"] for r in records if r["invoice_id"] not in db_rows]
    mismatched = [r["invoice_number"] for r in records if r["invoice_id"] in db_rows and money(db_rows[r["invoice_id"]][2]) != r["total"]]
    extra = set(db_rows) - set(ids)
    check("database holds exactly the bills the tills were told about", not missing and not mismatched and not extra,
          {"missing": missing[:5], "mismatched": mismatched[:5], "extra": list(extra)[:5]})

    # 2. per-branch money in the window
    for code in SELLING:
        client_total = money(sum(r["total"] for r in by_branch[code]))
        db_total = money(sql_value(f"""SELECT COALESCE(SUM(total_amount), 0) FROM invoices
                                       WHERE branch_id = '{branches[code]}' AND created_at >= '{start}' AND deleted_at IS NULL AND invoice_status = 'issued'"""))
        check(f"{code}: sales total client = database ({db_total:,.2f})", client_total == db_total, {"client": client_total, "db": db_total})

    # 3. each bill adds up
    bad = sql(f"""SELECT i.invoice_number FROM invoices i
                  WHERE i.created_at >= '{start}' AND i.deleted_at IS NULL AND (
                      ROUND(i.subtotal + i.tax_amount, 2) <> i.total_amount
                   OR i.subtotal <> (SELECT COALESCE(SUM(line_subtotal), 0) FROM invoice_items WHERE invoice_id = i.id)
                   OR i.tax_amount <> (SELECT COALESCE(SUM(tax_amount), 0) FROM invoice_items WHERE invoice_id = i.id))""")
    check("every bill: lines = subtotal, subtotal + VAT = total", not bad, [r[0] for r in bad][:10])
    bad = sql(f"""SELECT i.invoice_number, i.payment_status, i.total_amount, COALESCE(p.amount, 0) FROM invoices i
                  LEFT JOIN (SELECT invoice_id, SUM(amount) amount FROM invoice_payments GROUP BY invoice_id) p ON p.invoice_id = i.id
                  WHERE i.created_at >= '{start}' AND i.deleted_at IS NULL AND (
                      (i.payment_status = 'paid' AND COALESCE(p.amount, 0) <> i.total_amount)
                   OR (i.payment_status = 'unpaid' AND COALESCE(p.amount, 0) <> 0)
                   OR (i.payment_status = 'partial' AND (COALESCE(p.amount, 0) <= 0 OR COALESCE(p.amount, 0) >= i.total_amount)))""")
    check("every bill: payments match its payment status", not bad, bad[:10])
    cash_sales_unpaid = sql(f"SELECT invoice_number FROM invoices WHERE created_at >= '{start}' AND sale_type = 'cash' AND payment_status <> 'paid' AND deleted_at IS NULL AND created_by IN (SELECT id FROM users WHERE email LIKE 'pos.%')")
    check("no till cash sale is left unpaid", not cash_sales_unpaid, cash_sales_unpaid[:5])

    # 4. money received matches what the tills and collectors took
    client_cash = money(sum(r["cash"] for r in records))
    client_transfer = money(sum(r["transfer"] for r in records))
    client_collected = money(sum(c["amount"] for c in collections_made))
    db_paid = sql(f"""SELECT COALESCE(SUM(amount) FILTER (WHERE payment_type = 'cash'), 0), COALESCE(SUM(amount) FILTER (WHERE payment_type = 'bank_transfer'), 0)
                      FROM invoice_payments WHERE created_at >= '{start}'""")[0]
    check("money in = cash + transfers at tills + debt collected",
          money(float(db_paid[0]) + float(db_paid[1])) == money(client_cash + client_transfer + client_collected),
          {"db_cash": db_paid[0], "db_transfer": db_paid[1], "tills": client_cash + client_transfer, "collected": client_collected})

    # 5. dashboards and summaries read the same numbers
    for code in SELLING:
        summary = api(code, "GET", "/dashboard/daily-sales").body
        metrics = {m["key"]: money(m["value"]) for m in summary["metrics"]}
        db = sql(f"""SELECT COALESCE(SUM(i.total_amount), 0), COUNT(*),
                            COALESCE(SUM(i.total_amount - COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id = i.id), 0)) FILTER (WHERE i.payment_status <> 'paid'), 0)
                     FROM invoices i
                     WHERE i.branch_id = '{branches[code]}' AND i.invoice_status = 'issued' AND i.deleted_at IS NULL
                       AND (i.issued_at AT TIME ZONE 'Asia/Bangkok')::date = '{bangkok_today}'""")[0]
        db_money = sql(f"""SELECT COALESCE(SUM(p.amount) FILTER (WHERE p.payment_type = 'cash'), 0), COALESCE(SUM(p.amount) FILTER (WHERE p.payment_type = 'bank_transfer'), 0)
                           FROM invoice_payments p JOIN invoices i ON i.id = p.invoice_id AND i.deleted_at IS NULL
                           WHERE i.branch_id = '{branches[code]}' AND (p.created_at AT TIME ZONE 'Asia/Bangkok')::date = '{bangkok_today}'""")[0]
        expected = {"sales_total": money(db[0]), "invoice_count": int(db[1]), "credit_outstanding": money(db[2]),
                    "cash_received": money(db_money[0]), "bank_received": money(db_money[1])}
        got = {k: metrics.get(k) for k in expected}
        got["invoice_count"] = int(got["invoice_count"] or 0)
        check(f"{code}: till's สรุปยอดขาย = database", got == expected, {"screen": got, "db": expected})

        export = api(code, "GET", "/dashboard/sales-export", query={"format": "xlsx", "start_date": bangkok_today, "end_date": bangkok_today})
        if export.ok():
            cells = xlsx_cells(export.body)
            sheet_total, sheet_count = money(cells.get("B3")), int(float(cells.get("D3") or 0))
            rows_total = money(sum(float(v) for k, v in cells.items() if re.fullmatch(r"F\d+", k) and int(k[1:]) >= 7))
            check(f"{code}: Excel export = screen", sheet_total == expected["sales_total"] and sheet_count == expected["invoice_count"] and rows_total == expected["sales_total"],
                  {"sheet_total": sheet_total, "rows_total": rows_total, "count": sheet_count, "expected": expected["sales_total"]})
        else:
            check(f"{code}: Excel export = screen", False, export)
        pdf = api(code, "GET", "/dashboard/sales-export", query={"format": "pdf", "start_date": bangkok_today, "end_date": bangkok_today})
        check(f"{code}: PDF export opens", pdf.ok() and bytes(pdf.body[:4]) == b"%PDF", pdf.status)

    # head office: consolidated = Σ branches
    central = api("central", "GET", "/dashboard/daily-sales").body
    central_total = money(next(m["value"] for m in central["metrics"] if m["key"] == "sales_total"))
    db_all = money(sql_value(f"""SELECT COALESCE(SUM(total_amount), 0) FROM invoices WHERE invoice_status = 'issued' AND deleted_at IS NULL
                                AND (issued_at AT TIME ZONE 'Asia/Bangkok')::date = '{bangkok_today}'"""))
    branch_sum = money(sum(money(sql_value(f"""SELECT COALESCE(SUM(total_amount), 0) FROM invoices WHERE branch_id = '{bid}' AND invoice_status = 'issued'
                                               AND deleted_at IS NULL AND (issued_at AT TIME ZONE 'Asia/Bangkok')::date = '{bangkok_today}'""")) for bid in branches.values()))
    check("head office total = Σ branch totals = database", central_total == db_all == branch_sum, {"central": central_total, "db": db_all, "branches": branch_sum})

    # Superadmin's dashboard sorts every bill of the day into a group; head
    # office's shows the money that arrived plus what is still owed.
    group_keys = ["transfer_abbreviated", "transfer_full_tax", "cash_full_tax", "mixed_abbreviated", "mixed_full_tax", "credit_sale",
                  "cash_ghost_hidden", "cash_repriced", "cash_mixed", "unpaid"]
    day_sql = ("created_at >= '{d}'::date - INTERVAL '7 hours' AND created_at < '{d}'::date + INTERVAL '17 hours'").format(d=bangkok_today)
    super_view = api("super", "GET", "/dashboard/daily-breakdown").body
    central_view = {b["branch_code"]: b for b in api("central", "GET", "/dashboard/daily-breakdown").body["branches"]}
    for branch in super_view["branches"]:
        code = branch["branch_code"]
        if code not in SELLING:
            continue
        db_branch = money(sql_value(f"""SELECT COALESCE(SUM(total_amount), 0) FROM invoices WHERE branch_id = '{branch["branch_id"]}'
                                       AND invoice_status = 'issued' AND deleted_at IS NULL AND {day_sql}"""))
        shown = money(sum(branch.get(k, {}).get("amount", 0) for k in group_keys))
        check(f"{code}: superadmin dashboard groups add up to the day's bills", shown == db_branch, {"groups": shown, "db": db_branch})
        tender = central_view[code]["tender"]
        money_in = money(tender["total_amount"] + tender.get("outstanding_amount", 0))
        check(f"{code}: head-office dashboard money in + still owed = the day's bills", money_in == db_branch,
              {"tender": tender, "db": db_branch})
    overall = money(sum(super_view["overall"].get(k, {}).get("amount", 0) for k in group_keys))
    check("dashboard overall = Σ branch breakdowns",
          overall == money(sum(sum(b.get(k, {}).get("amount", 0) for k in group_keys) for b in super_view["branches"])), overall)

    tax = {row["branch_name"]: row for row in api("central", "GET", "/reports/tax").body["items"]}
    for code in SELLING:
        name = sql_value(f"SELECT name FROM branches WHERE id = '{branches[code]}'")
        db = sql(f"""SELECT COUNT(*), COALESCE(SUM(subtotal), 0), COALESCE(SUM(tax_amount), 0), COALESCE(SUM(total_amount), 0)
                     FROM invoices WHERE branch_id = '{branches[code]}' AND invoice_status = 'issued' AND deleted_at IS NULL""")[0]
        row = tax.get(name, {})
        shown = (int(row.get("invoice_count", -1)), money(row.get("subtotal")), money(row.get("tax_amount")), money(row.get("total_amount")))
        check(f"{code}: tax report = database", shown == (int(db[0]), money(db[1]), money(db[2]), money(db[3])), {"report": shown, "db": db})

    # 6. stock: inventory = lots = movements, nothing negative
    bad = sql("""SELECT b.code, p.sku, i.qty_real, COALESCE(l.remaining, 0) FROM inventory i
                 JOIN branches b ON b.id = i.branch_id JOIN products p ON p.id = i.product_id
                 LEFT JOIN (SELECT branch_id, product_id, SUM(remaining_quantity) remaining FROM inventory_lots WHERE stock_bucket = 'real' GROUP BY 1, 2) l
                   ON l.branch_id = i.branch_id AND l.product_id = i.product_id
                 WHERE i.qty_real <> COALESCE(l.remaining, 0)""")
    check("stock on hand = Σ lot remaining, every branch and product", not bad, bad[:10])
    negative = sql("SELECT COUNT(*) FROM inventory WHERE qty_real < 0 OR qty_ghost < 0")[0][0]
    negative_lots = sql("SELECT COUNT(*) FROM inventory_lots WHERE remaining_quantity < 0")[0][0]
    check("no negative stock or lot", negative == "0" and negative_lots == "0", {"inventory": negative, "lots": negative_lots})
    moved = {(r[0], r[1]): int(r[2]) for r in sql(f"""SELECT branch_id, product_id, SUM(quantity_delta) FROM inventory_movements
                                                      WHERE created_at >= '{start}' AND stock_bucket = 'real' GROUP BY 1, 2""")}
    now = {(r[0], r[1]): int(r[2]) for r in sql("SELECT branch_id, product_id, qty_real FROM inventory")}
    drift = [(k, branch_before.get(k, 0), now.get(k, 0), delta) for k, delta in moved.items() if branch_before.get(k, 0) + delta != now.get(k, 0)]
    check("stock change since start = Σ movements since start", not drift, drift[:5])
    sold = {(r[0], r[1]): int(r[2]) for r in sql(f"""SELECT i.branch_id, ii.product_id, SUM(ii.quantity) FROM invoice_items ii JOIN invoices i ON i.id = ii.invoice_id
                                                     WHERE i.created_at >= '{start}' AND ii.stock_bucket = 'real' GROUP BY 1, 2""")}
    sale_moves = {(r[0], r[1]): -int(r[2]) for r in sql(f"""SELECT branch_id, product_id, SUM(quantity_delta) FROM inventory_movements
                                                             WHERE created_at >= '{start}' AND movement_type = 'sale' GROUP BY 1, 2""")}
    check("units on bills = units taken out by sale movements", sold == sale_moves,
          [(k, sold.get(k), sale_moves.get(k)) for k in set(sold) | set(sale_moves) if sold.get(k) != sale_moves.get(k)][:5])

    # 7. points and credit
    bad = sql("""SELECT c.customer_code, c.points_balance, COALESCE(SUM(e.points), 0) FROM customers c
                 LEFT JOIN loyalty_point_entries e ON e.customer_id = c.id GROUP BY c.id HAVING c.points_balance <> COALESCE(SUM(e.points), 0)""")
    check("every member: points balance = Σ points ledger", not bad, bad[:5])
    per_point = float(sql_value("SELECT setting_value FROM app_settings WHERE setting_key = 'loyalty_baht_per_point'") or 25)
    bad = sql(f"""SELECT invoice_number, total_amount, points_earned FROM invoices
                  WHERE created_at >= '{start}' AND customer_id IS NOT NULL AND deleted_at IS NULL
                    AND points_earned <> FLOOR(total_amount / {per_point})""")
    check("every member bill earned floor(total ÷ %g) points" % per_point, not bad, bad[:5])
    bad = sql(f"""SELECT i.invoice_number FROM invoices i WHERE i.created_at >= '{start}' AND i.points_redeemed > 0 AND NOT EXISTS (
                    SELECT 1 FROM loyalty_point_entries e WHERE e.invoice_id = i.id AND e.entry_type = 'redeem' AND e.points = -i.points_redeemed)""")
    check("every redemption is in the ledger", not bad, bad[:5])
    bad = sql("""SELECT c.customer_code, c.credit_limit, SUM(i.total_amount - COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id = i.id), 0))
                 FROM customers c JOIN invoices i ON i.customer_id = c.id AND i.payment_status <> 'paid' AND i.deleted_at IS NULL
                 GROUP BY c.id HAVING SUM(i.total_amount - COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id = i.id), 0)) > c.credit_limit + 0.001""")
    check("no customer owes more than their credit line", not bad, bad[:5])
    receivables = api("central", "GET", "/receivables").body
    db_owed = money(sql_value("""SELECT COALESCE(SUM(i.total_amount - COALESCE((SELECT SUM(amount) FROM invoice_payments WHERE invoice_id = i.id), 0)), 0)
                                FROM invoices i WHERE i.customer_id IS NOT NULL AND i.payment_status <> 'paid' AND i.deleted_at IS NULL AND i.invoice_status = 'issued'"""))
    check("ลูกหนี้ค้างชำระ screen = database", money(receivables["total_outstanding"]) == db_owed, {"screen": receivables["total_outstanding"], "db": db_owed})
    aging = money(sum(receivables["aging_totals"].values()))
    check("aging buckets add up to the total owed", aging == money(receivables["total_outstanding"]), aging)

    # 8. document numbers
    for code in SELLING:
        numbers = [r[0] for r in sql(f"""SELECT COALESCE(original_invoice_number, invoice_number) FROM invoices
                                         WHERE branch_id = '{branches[code]}' AND (issued_at AT TIME ZONE 'Asia/Bangkok')::date = '{bangkok_today}'""")]
        sequence = sorted(int(re.search(r"(\d{5})$", n).group(1)) for n in numbers if re.search(r"(\d{5})$", n))
        gaps = [n for n in range(sequence[0], sequence[-1] + 1) if n not in set(sequence)] if sequence else []
        check(f"{code}: today's bill numbers are unique and gap-free", len(sequence) == len(set(sequence)) and not gaps,
              {"count": len(sequence), "dupes": len(sequence) - len(set(sequence)), "gaps": gaps[:10]})


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--sales", type=int, default=30, help="sales per till thread")
    parser.add_argument("--threads", type=int, default=2, help="concurrent threads per branch till")
    args = parser.parse_args()

    branches = branch_ids()
    for account in ["super", "central"] + SELLING:
        me(account)
    members, accounts = customers_pool()
    start = sql_value("SELECT NOW()")
    bangkok_today = sql_value("SELECT (NOW() AT TIME ZONE 'Asia/Bangkok')::date")
    branch_before = {(r[0], r[1]): int(r[2]) for r in sql("SELECT branch_id, product_id, qty_real FROM inventory")}
    print(f"start {start} · {len(SELLING)} branches × {args.threads} threads × {args.sales} sales · {len(members)} members · {len(accounts)} credit accounts")

    began = time.time()
    jobs = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=len(SELLING) * args.threads + 4) as pool:
        for index, code in enumerate(SELLING):
            for thread in range(args.threads):
                jobs.append(pool.submit(branch_worker, code, branches[code], args.sales, index * 100 + thread, members, accounts))
        jobs.append(pool.submit(central_worker, branches, args.sales, members, accounts))
        jobs.append(pool.submit(collector_worker, accounts, 25))
        jobs.append(pool.submit(transfer_worker, branches))
        for job in jobs:
            job.result()
    elapsed = time.time() - began
    print(f"load phase: {len(records)} sales in {elapsed:.1f}s ({len(records) / max(elapsed, 0.001):.1f} sales/s) · outcomes {dict(outcomes)}")

    race_last_units(branches)
    race_points(branches)
    race_credit(branches)
    race_remote_double_pay(branches)
    race_double_collect(branches)

    check("no 5xx or malformed response during the run", not unexpected, unexpected[:5])
    reconcile(branches, start, bangkok_today, branch_before)

    failed = [c for c in checks if not c["ok"]]
    report = {
        "started_at": start, "elapsed_seconds": round(elapsed, 1), "sales": len(records), "collections": len(collections_made),
        "outcomes": dict(outcomes), "checks": checks, "failed": len(failed),
        "per_branch": {code: {"bills": sum(1 for r in records if r["branch"] == code),
                              "total": money(sum(r["total"] for r in records if r["branch"] == code))} for code in SELLING},
    }
    path = os.path.join(os.path.dirname(__file__), "multi_branch_report.json")
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(report, handle, ensure_ascii=False, indent=1, default=str)
    print(f"\n{len(checks) - len(failed)}/{len(checks)} checks passed · report {path}")
    raise SystemExit(1 if failed else 0)


if __name__ == "__main__":
    main()
