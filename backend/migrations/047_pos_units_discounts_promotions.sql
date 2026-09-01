-- POS selling tools: multi-unit selling (wholesale <-> retail), line and bill
-- discounts, and promotions (buy X get Y, percent/amount off, bundle price,
-- bill-level giveaway).
--
-- Stock stays in the product's base unit. invoice_items.quantity therefore keeps
-- meaning "base units removed from stock", so every existing path that restores
-- or reports stock from it (returns, invoice delete, month-end, FDA) keeps
-- working untouched. The unit actually sold is recorded alongside it.

-- ---------------------------------------------------------------- units
CREATE TABLE IF NOT EXISTS product_units (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    unit_name TEXT NOT NULL,
    -- how many base units one of this unit contains (base unit = 1)
    conversion_qty INTEGER NOT NULL CHECK (conversion_qty > 0),
    is_base BOOLEAN NOT NULL DEFAULT FALSE,
    -- price for one of THIS unit; NULL = derive from base price x conversion_qty
    selling_price NUMERIC(12,2) CHECK (selling_price IS NULL OR selling_price >= 0),
    barcode TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, unit_name)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_product_units_one_base
    ON product_units (product_id) WHERE is_base;
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_units_barcode
    ON product_units (barcode) WHERE barcode IS NOT NULL AND barcode <> '';
CREATE INDEX IF NOT EXISTS idx_product_units_product ON product_units (product_id, sort_order);

-- The base unit must always convert 1:1, otherwise stock maths breaks.
ALTER TABLE product_units DROP CONSTRAINT IF EXISTS product_units_base_conversion_check;
ALTER TABLE product_units
    ADD CONSTRAINT product_units_base_conversion_check
    CHECK (NOT is_base OR conversion_qty = 1);

-- Every existing product keeps selling exactly as before: one base unit.
INSERT INTO product_units (id, product_id, unit_name, conversion_qty, is_base, selling_price, sort_order, active)
SELECT gen_random_uuid(), p.id, COALESCE(NULLIF(TRIM(p.unit_name), ''), 'ชิ้น'), 1, TRUE, NULL, 0, TRUE
FROM products p
WHERE NOT EXISTS (SELECT 1 FROM product_units u WHERE u.product_id = p.id)
ON CONFLICT DO NOTHING;

-- ----------------------------------------------------------- promotions
CREATE TABLE IF NOT EXISTS promotions (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    promo_type TEXT NOT NULL CHECK (promo_type IN ('buy_x_get_y','percent','amount','bundle','bill_giveaway')),
    -- NULL branch = applies to every branch
    branch_id UUID REFERENCES branches(id) ON DELETE CASCADE,
    starts_at DATE,
    ends_at DATE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    -- conditions
    min_quantity INTEGER NOT NULL DEFAULT 0 CHECK (min_quantity >= 0),
    min_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (min_amount >= 0),
    -- rewards (used per promo_type)
    discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (discount_percent >= 0 AND discount_percent <= 100),
    discount_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    bundle_price NUMERIC(14,2) CHECK (bundle_price IS NULL OR bundle_price >= 0),
    -- how many times one bill may earn this promotion (0 = unlimited)
    max_uses_per_bill INTEGER NOT NULL DEFAULT 0 CHECK (max_uses_per_bill >= 0),
    priority INTEGER NOT NULL DEFAULT 0,
    notes TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (starts_at IS NULL OR ends_at IS NULL OR ends_at >= starts_at)
);

CREATE INDEX IF NOT EXISTS idx_promotions_lookup ON promotions (active, branch_id, promo_type);

CREATE TABLE IF NOT EXISTS promotion_items (
    id UUID PRIMARY KEY,
    promotion_id UUID NOT NULL REFERENCES promotions(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    unit_id UUID REFERENCES product_units(id) ON DELETE SET NULL,
    -- condition: qty required to trigger; reward/bundle: qty granted or included
    quantity INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0),
    role TEXT NOT NULL CHECK (role IN ('condition','reward','bundle_item')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_promotion_items_promotion ON promotion_items (promotion_id, role);
CREATE INDEX IF NOT EXISTS idx_promotion_items_product ON promotion_items (product_id);

-- --------------------------------------------------- sales document columns
ALTER TABLE invoice_items
    ADD COLUMN IF NOT EXISTS unit_id UUID REFERENCES product_units(id),
    ADD COLUMN IF NOT EXISTS unit_name_snapshot TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS unit_conversion_qty INTEGER NOT NULL DEFAULT 1 CHECK (unit_conversion_qty > 0),
    -- quantity expressed in the unit the cashier picked (quantity = sold_quantity * unit_conversion_qty)
    ADD COLUMN IF NOT EXISTS sold_quantity INTEGER NOT NULL DEFAULT 0 CHECK (sold_quantity >= 0),
    ADD COLUMN IF NOT EXISTS sold_unit_price NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (sold_unit_price >= 0),
    -- discount typed by the cashier on this line
    ADD COLUMN IF NOT EXISTS discount_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    -- share of the bill-level discount allocated to this line (pro-rata)
    ADD COLUMN IF NOT EXISTS bill_discount_share NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (bill_discount_share >= 0),
    ADD COLUMN IF NOT EXISTS is_giveaway BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS promotion_id UUID REFERENCES promotions(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS promotion_name_snapshot TEXT NOT NULL DEFAULT '';

-- Historical rows sold in the base unit.
UPDATE invoice_items ii
SET sold_quantity = ii.quantity,
    sold_unit_price = ii.unit_price,
    unit_name_snapshot = COALESCE(NULLIF(TRIM(p.unit_name), ''), 'ชิ้น')
FROM products p
WHERE p.id = ii.product_id AND ii.sold_quantity = 0;

ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS bill_discount_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (bill_discount_amount >= 0),
    ADD COLUMN IF NOT EXISTS line_discount_total NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (line_discount_total >= 0),
    ADD COLUMN IF NOT EXISTS promotion_discount_total NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (promotion_discount_total >= 0),
    ADD COLUMN IF NOT EXISTS giveaway_cost_total NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (giveaway_cost_total >= 0);

ALTER TABLE quotation_items
    ADD COLUMN IF NOT EXISTS unit_id UUID REFERENCES product_units(id),
    ADD COLUMN IF NOT EXISTS unit_name_snapshot TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS unit_conversion_qty INTEGER NOT NULL DEFAULT 1 CHECK (unit_conversion_qty > 0),
    ADD COLUMN IF NOT EXISTS sold_quantity INTEGER NOT NULL DEFAULT 0 CHECK (sold_quantity >= 0),
    ADD COLUMN IF NOT EXISTS sold_unit_price NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (sold_unit_price >= 0),
    ADD COLUMN IF NOT EXISTS discount_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    ADD COLUMN IF NOT EXISTS is_giveaway BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS promotion_id UUID REFERENCES promotions(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS promotion_name_snapshot TEXT NOT NULL DEFAULT '';

UPDATE quotation_items qi
SET sold_quantity = qi.quantity,
    sold_unit_price = qi.unit_price,
    unit_name_snapshot = COALESCE(NULLIF(TRIM(p.unit_name), ''), 'ชิ้น')
FROM products p
WHERE p.id = qi.product_id AND qi.sold_quantity = 0;

-- ------------------------------------------------------------ permissions
INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES
    (gen_random_uuid(), 'promotion.manage', 'จัดการโปรโมชั่น', 'สร้างและแก้ไขโปรโมชั่นและของแถม', NOW(), NOW()),
    (gen_random_uuid(), 'promotion.view', 'ดูโปรโมชั่น', 'ดูรายการโปรโมชั่นที่ใช้ได้', NOW(), NOW()),
    (gen_random_uuid(), 'sales.discount.line', 'ส่วนลดหน้าร้าน', 'ให้ส่วนลดต่อบรรทัดและท้ายบิลภายในเพดานที่กำหนด', NOW(), NOW())
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
JOIN permissions p ON p.permission_key IN ('promotion.manage','promotion.view','sales.discount.line')
WHERE r.role_key IN ('super_admin','central_admin','admin','branch_admin')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
JOIN permissions p ON p.permission_key IN ('promotion.view','sales.discount.line')
WHERE r.role_key = 'branch_pos'
ON CONFLICT DO NOTHING;
