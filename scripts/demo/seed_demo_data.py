"""Seed the data a customer demo needs on top of a freshly seeded database.

  - stock at คณาเภสัช (KNP), moved from the warehouse with a real transfer
  - selling units (ชิ้น / แพ็ค 10) and tier prices on a set of products
  - members, and three wholesale accounts with credit lines
  - promotions: a member-only discount, buy 2 get 1, and a bill giveaway

Safe to run more than once: existing members (by phone), promotions (by code)
and KNP stock are left alone. Never run it against production.

  PYTHONPATH=scripts/qa python3 scripts/demo/seed_demo_data.py
"""
import sys

from erp_api import api, sql, sql_value

failures = []


def must(label, resp, ok=(200, 201)):
    if resp.status not in ok:
        failures.append(label)
        print("FAIL", label, resp)
    return resp


def branch_ids():
    return {code: bid for bid, code in sql("SELECT id, code FROM branches WHERE active")}


def pick_products():
    rows = sql("""
        SELECT p.id, p.name, p.base_selling_price, p.unit_name
        FROM products p
        WHERE p.active AND NOT p.tax_exempt
          AND (SELECT COUNT(*) FROM inventory i JOIN branches b ON b.id = i.branch_id
               WHERE i.product_id = p.id AND i.qty_real >= 5 AND b.code IN ('MES','PHH','PHS','NPT')) = 4
          AND EXISTS (SELECT 1 FROM inventory i JOIN branches b ON b.id = i.branch_id
                      WHERE i.product_id = p.id AND b.code = 'WH' AND i.qty_real >= 20)
        ORDER BY p.sku
        LIMIT 12""")
    return [{"id": r[0], "name": r[1], "price": float(r[2]), "unit": r[3] or "ชิ้น"} for r in rows]


def stock_knp(branches, products):
    have = int(sql_value(f"SELECT COUNT(*) FROM inventory WHERE branch_id = '{branches['KNP']}' AND qty_real > 0") or 0)
    if have >= len(products):
        print("KNP already stocked:", have, "products")
        return
    created = must("create transfer WH→KNP", api("central", "POST", "/transfers", {
        "source_branch_id": branches["WH"], "destination_branch_id": branches["KNP"],
        "request_note": "สต๊อกตั้งต้นสำหรับ demo",
        "items": [{"product_id": p["id"], "quantity": 10, "stock_bucket": "real"} for p in products],
    }))
    transfer_id = (created.body or {}).get("id")
    if not transfer_id:
        return
    must("dispatch transfer", api("central", "POST", f"/transfers/{transfer_id}/dispatch", {"pickup_name": "พนักงานคลัง", "courier_name": "รถบริษัท"}))
    items = sql(f"SELECT id, quantity FROM transfer_items WHERE transfer_id = '{transfer_id}'")
    must("KNP receives transfer", api("KNP", "POST", f"/transfers/{transfer_id}/receive", {
        "items": [{"item_id": item_id, "received_quantity": int(qty), "discrepancy_note": ""} for item_id, qty in items],
    }))


def units_and_tiers(products):
    for product in products[:6]:
        price = product["price"]
        must(f"units {product['name']}", api("super", "PUT", f"/products/{product['id']}/units", {"units": [
            {"unit_name": product["unit"], "conversion_qty": 1, "is_base": True},
            {"unit_name": "แพ็ค 10", "conversion_qty": 10, "is_base": False, "selling_price": round(price * 10 * 0.95, 2)},
        ]}))
        units = api("super", "GET", f"/products/{product['id']}/units").body["items"]
        base = next(u for u in units if u["is_base"])
        pack = next(u for u in units if not u["is_base"])
        must(f"tiers {product['name']}", api("central", "PUT", f"/products/{product['id']}/price-tiers", {"tiers": [
            {"unit_id": base["id"], "customer_tier": "all", "min_quantity": 5, "unit_price": round(price * 0.95, 2)},
            {"unit_id": base["id"], "customer_tier": "wholesale", "min_quantity": 1, "unit_price": round(price * 0.88, 2)},
            {"unit_id": pack["id"], "customer_tier": "wholesale", "min_quantity": 1, "unit_price": round(price * 10 * 0.85, 2)},
        ]}))


MEMBERS = [
    ("สมชาย ใจดี", "0811000001"), ("สมหญิง รักสุขภาพ", "0811000002"), ("วิชัย มั่นคง", "0811000003"),
    ("มาลี ศรีสุข", "0811000004"), ("ประเสริฐ ทองดี", "0811000005"), ("นภา แสงทอง", "0811000006"),
    ("อนันต์ พูนผล", "0811000007"), ("กมลา บุญมา", "0811000008"), ("ธนา วงศ์ใหญ่", "0811000009"),
    ("ปรียา สายสุข", "0811000010"), ("สุรชัย เพชรงาม", "0811000011"), ("วรรณา ทิพย์รส", "0811000012"),
    ("ชัยวัฒน์ ดวงดี", "0811000013"), ("จันทร์เพ็ญ แก้วใส", "0811000014"), ("ศักดิ์ชัย ใจกล้า", "0811000015"),
    ("รัตนา มีสุข", "0811000016"), ("บุญเลิศ ศรีทอง", "0811000017"), ("อรุณี ฟ้าใส", "0811000018"),
    ("เกรียงไกร ยอดเยี่ยม", "0811000019"), ("พิมพ์ใจ รุ่งเรือง", "0811000020"),
]
WHOLESALE = [
    ("คลินิกเวชกรรมสุขใจ", "0821000001", "0105560000011", 50000, 30),
    ("ร้านขายยาเภสัชดี (ขายส่ง)", "0821000002", "0105560000022", 30000, 45),
    ("ศูนย์ดูแลผู้สูงอายุบ้านอบอุ่น", "0821000003", "0105560000033", 20000, 15),
]


def customers():
    existing = {row[0] for row in sql("SELECT phone FROM customers")}
    for name, phone in MEMBERS:
        if phone not in existing:
            must(f"member {name}", api("MES", "POST", "/customers", {"name": name, "phone": phone}))
    for name, phone, tax_id, limit, days in WHOLESALE:
        if phone not in existing:
            must(f"wholesale {name}", api("central", "POST", "/customers", {
                "name": name, "phone": phone, "tax_id": tax_id, "customer_type": "business",
                "address": "กรุงเทพมหานคร", "price_tier": "wholesale", "credit_limit": limit, "credit_days": days,
            }))


def promotions(products):
    existing = {row[0] for row in sql("SELECT code FROM promotions")}
    if "MEMBER10" not in existing:
        must("promotion MEMBER10", api("central", "POST", "/promotions", {
            "code": "MEMBER10", "name": "สมาชิกลด 10%", "promo_type": "percent", "discount_percent": 10,
            "members_only": True, "priority": 10,
            "items": [{"product_id": p["id"], "quantity": 1, "role": "condition"} for p in products[6:9]],
        }))
    if "BUY2GET1" not in existing:
        must("promotion BUY2GET1", api("central", "POST", "/promotions", {
            "code": "BUY2GET1", "name": f"ซื้อ {products[9]['name']} 2 แถม 1", "promo_type": "buy_x_get_y",
            "max_uses_per_bill": 2, "priority": 5,
            "items": [{"product_id": products[9]["id"], "quantity": 2, "role": "condition"},
                      {"product_id": products[9]["id"], "quantity": 1, "role": "reward"}],
        }))
    if "BILL3000" not in existing:
        must("promotion BILL3000", api("central", "POST", "/promotions", {
            "code": "BILL3000", "name": "ซื้อครบ 3,000 รับของแถม", "promo_type": "bill_giveaway", "min_amount": 3000,
            "max_uses_per_bill": 1, "priority": 1,
            "items": [{"product_id": products[10]["id"], "quantity": 1, "role": "reward"}],
        }))


def main():
    branches = branch_ids()
    products = pick_products()
    if len(products) < 11:
        print("not enough stocked products to seed the demo:", len(products))
        sys.exit(1)
    stock_knp(branches, products)
    units_and_tiers(products)
    customers()
    promotions(products)
    print("demo products:", ", ".join(p["name"] for p in products))
    print("done with %d failure(s)" % len(failures))
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
