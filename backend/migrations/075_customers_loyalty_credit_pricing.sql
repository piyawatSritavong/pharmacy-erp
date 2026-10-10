-- Customers for the branches' own tills: members who collect and spend points,
-- wholesale buyers who get a tier price, and accounts allowed to buy on credit.
--
-- One customer is shared by every branch, so a member registered at MES can
-- earn and redeem at PHH. The V2 pilot keeps its own v2_customers; the two are
-- unified at the V2 cutover, not here.

CREATE TABLE IF NOT EXISTS customers (
    id UUID PRIMARY KEY,
    customer_code TEXT NOT NULL UNIQUE,
    customer_type TEXT NOT NULL DEFAULT 'person' CHECK (customer_type IN ('person', 'business')),
    name TEXT NOT NULL CHECK (LENGTH(BTRIM(name)) > 0),
    phone TEXT NOT NULL DEFAULT '',
    tax_id TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    -- retail pays the shelf price; wholesale also gets the wholesale tier
    -- prices set on the product (product_price_tiers.customer_tier).
    price_tier TEXT NOT NULL DEFAULT 'retail' CHECK (price_tier IN ('retail', 'wholesale')),
    -- 0 means no credit: the customer pays at the till like anyone else.
    credit_limit NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (credit_limit >= 0),
    credit_days INTEGER NOT NULL DEFAULT 0 CHECK (credit_days BETWEEN 0 AND 365),
    -- Cached running balance of loyalty_point_entries, updated in the same
    -- transaction as every entry; the ledger is the record.
    points_balance INTEGER NOT NULL DEFAULT 0 CHECK (points_balance >= 0),
    home_branch_id UUID REFERENCES branches(id),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A phone number identifies a member at the till, so it may belong to one
-- customer only. Customers without a phone (a company account) are fine.
CREATE UNIQUE INDEX IF NOT EXISTS idx_customers_phone_unique ON customers (phone) WHERE phone <> '';
CREATE INDEX IF NOT EXISTS idx_customers_name ON customers (LOWER(name));
CREATE SEQUENCE IF NOT EXISTS customer_code_seq START 1;

CREATE TABLE IF NOT EXISTS loyalty_point_entries (
    id UUID PRIMARY KEY,
    customer_id UUID NOT NULL REFERENCES customers(id),
    branch_id UUID REFERENCES branches(id),
    invoice_id UUID REFERENCES invoices(id),
    entry_type TEXT NOT NULL CHECK (entry_type IN ('earn', 'redeem', 'earn_reversal', 'redeem_reversal', 'adjust')),
    points INTEGER NOT NULL CHECK (points <> 0),
    balance_after INTEGER NOT NULL CHECK (balance_after >= 0),
    note TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_loyalty_entries_customer ON loyalty_point_entries (customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_loyalty_entries_invoice ON loyalty_point_entries (invoice_id) WHERE invoice_id IS NOT NULL;

-- Bills remember who bought, what the visit did to their points, and, for a
-- credit sale, when the money is due. sale_type tells a credit bill apart from
-- a cash bill that simply has not been collected yet (a back-office invoice).
ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id),
    ADD COLUMN IF NOT EXISTS sale_type TEXT NOT NULL DEFAULT 'cash',
    ADD COLUMN IF NOT EXISTS due_date DATE,
    ADD COLUMN IF NOT EXISTS points_earned INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS points_redeemed INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS points_discount NUMERIC(12,2) NOT NULL DEFAULT 0;

ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_sale_type_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_sale_type_check CHECK (sale_type IN ('cash', 'credit'));
ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_points_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_points_check
    CHECK (points_earned >= 0 AND points_redeemed >= 0 AND points_discount >= 0);
-- A credit bill can be paid off in parts.
ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_payment_status_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_payment_status_check
    CHECK (payment_status IN ('unpaid', 'partial', 'paid'));

CREATE INDEX IF NOT EXISTS idx_invoices_customer ON invoices (customer_id, issued_at DESC) WHERE customer_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_invoices_receivable ON invoices (due_date)
    WHERE payment_status <> 'paid' AND deleted_at IS NULL AND invoice_status = 'issued';

-- The points discount is spread over the lines like the bill discount, but
-- kept apart from it: it is the member spending points, not the cashier
-- giving money away, so it neither needs a discount permission nor counts
-- against the discount ceiling.
ALTER TABLE invoice_items
    ADD COLUMN IF NOT EXISTS points_discount_share NUMERIC(14,2) NOT NULL DEFAULT 0
        CHECK (points_discount_share >= 0);

-- Which branch took a payment. A credit bill of PHH may be paid off at MES;
-- the money is in MES's drawer that day. NULL on rows written before this
-- column existed, which were always taken at the bill's own branch.
ALTER TABLE invoice_payments
    ADD COLUMN IF NOT EXISTS received_branch_id UUID REFERENCES branches(id);

-- Tier prices: a quantity break ("10 boxes or more: 95 each") open to everyone,
-- or a wholesale price only wholesale customers get. A row prices one selling
-- unit; NULL unit_id means the product's base unit. NULL branch_id applies at
-- every branch, a branch row at that branch only.
CREATE TABLE IF NOT EXISTS product_price_tiers (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    unit_id UUID REFERENCES product_units(id) ON DELETE CASCADE,
    branch_id UUID REFERENCES branches(id) ON DELETE CASCADE,
    customer_tier TEXT NOT NULL DEFAULT 'all' CHECK (customer_tier IN ('all', 'wholesale')),
    min_quantity INTEGER NOT NULL DEFAULT 1 CHECK (min_quantity >= 1),
    unit_price NUMERIC(12,2) NOT NULL CHECK (unit_price >= 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_price_tiers_product ON product_price_tiers (product_id) WHERE active;

-- A parked bill remembers the member it was being rung up for.
ALTER TABLE parked_bills
    ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id);

-- Promotions may be kept for members only.
ALTER TABLE promotions
    ADD COLUMN IF NOT EXISTS members_only BOOLEAN NOT NULL DEFAULT FALSE;

-- Loyalty rules. Defaults: 1 point per 25 baht paid; 1 point is worth 0.25
-- baht (1% back); at least 40 points (10 baht) per redemption.
INSERT INTO app_settings (setting_key, setting_value, metadata, created_at, updated_at)
VALUES
    ('loyalty_baht_per_point', '25', '{}'::jsonb, NOW(), NOW()),
    ('loyalty_point_value', '0.25', '{}'::jsonb, NOW(), NOW()),
    ('loyalty_min_redeem_points', '40', '{}'::jsonb, NOW(), NOW())
ON CONFLICT (setting_key) DO NOTHING;

-- Permissions. Granting by role_key here reaches databases that already have
-- their roles; a fresh database gets them from the seed catalog, which lists
-- the same keys (see internal/app/seed.go).
INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at) VALUES
    ('c0a1e0b2-75a1-4c5e-9b11-000000000075', 'customer.view', 'ดูลูกค้าสมาชิก', 'ค้นหาลูกค้า ดูแต้มและยอดค้างชำระ', NOW(), NOW()),
    ('c0a1e0b2-75a1-4c5e-9b11-000000000076', 'customer.manage', 'สมัครและแก้ไขลูกค้า', 'สมัครสมาชิกและแก้ไขข้อมูลติดต่อของลูกค้า', NOW(), NOW()),
    ('c0a1e0b2-75a1-4c5e-9b11-000000000077', 'customer.credit.manage', 'กำหนดเครดิตและระดับราคา', 'กำหนดวงเงินเครดิต จำนวนวันเครดิต ระดับราคาส่ง และปรับแต้ม', NOW(), NOW()),
    ('c0a1e0b2-75a1-4c5e-9b11-000000000078', 'receivable.view', 'ดูลูกหนี้ค้างชำระ', 'ดูบิลเครดิตที่ยังไม่ได้รับเงินและอายุหนี้', NOW(), NOW()),
    ('c0a1e0b2-75a1-4c5e-9b11-000000000079', 'price_tier.manage', 'กำหนดราคาส่ง', 'กำหนดราคาตามจำนวนและราคาสำหรับลูกค้าขายส่ง', NOW(), NOW())
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r CROSS JOIN permissions p
WHERE r.role_key IN ('super_admin', 'central_admin')
  AND p.permission_key IN ('customer.view', 'customer.manage', 'customer.credit.manage', 'receivable.view', 'price_tier.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r CROSS JOIN permissions p
WHERE r.role_key = 'branch_pos'
  AND p.permission_key IN ('customer.view', 'customer.manage', 'receivable.view')
ON CONFLICT DO NOTHING;
