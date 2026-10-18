-- Member levels and price levels.
--
-- A walk-in customer pays price level 1, the shelf price. A member has a level
-- from 1 to 5, and each member level buys at one price level: by default a
-- level-1 member pays price level 2, level 2 pays level 3, and so on, capped at
-- 5. The shop can rename the levels and change which price each one gets.
--
-- A product may carry its own price for levels 2-5, per selling unit. Where it
-- does not, a level's default discount (a percentage off level 1, zero unless
-- set) applies. A member is charged the lowest of their level and every level
-- below it, so moving a member up never makes anything dearer.

CREATE TABLE IF NOT EXISTS price_levels (
    level SMALLINT PRIMARY KEY CHECK (level BETWEEN 1 AND 5),
    name TEXT NOT NULL,
    default_discount_percent NUMERIC(5,2) NOT NULL DEFAULT 0
        CHECK (default_discount_percent >= 0 AND default_discount_percent < 100),
    updated_by UUID REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Level 1 is the shelf price itself and is never discounted here.
    CHECK (level > 1 OR default_discount_percent = 0)
);

INSERT INTO price_levels (level, name) VALUES
    (1, 'ราคาทั่วไป'),
    (2, 'ราคาสมาชิก'),
    (3, 'ราคาคนรู้จัก'),
    (4, 'ราคาคนสนิท'),
    (5, 'ราคาพิเศษ')
ON CONFLICT (level) DO NOTHING;

CREATE TABLE IF NOT EXISTS member_levels (
    level SMALLINT PRIMARY KEY CHECK (level BETWEEN 1 AND 5),
    name TEXT NOT NULL,
    price_level SMALLINT NOT NULL REFERENCES price_levels(level),
    updated_by UUID REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO member_levels (level, name, price_level) VALUES
    (1, 'สมาชิก', 2),
    (2, 'คนรู้จัก / เพื่อนแนะนำ', 3),
    (3, 'เพื่อน / ญาติ / คนสนิท', 4),
    (4, 'VIP', 5),
    (5, 'พิเศษ', 5)
ON CONFLICT (level) DO NOTHING;

ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS member_level SMALLINT NOT NULL DEFAULT 1
        REFERENCES member_levels(level);
CREATE INDEX IF NOT EXISTS idx_customers_member_level ON customers (member_level);

CREATE TABLE IF NOT EXISTS product_level_prices (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    -- NULL prices the base unit, as product_price_tiers does.
    unit_id UUID REFERENCES product_units(id) ON DELETE CASCADE,
    price_level SMALLINT NOT NULL CHECK (price_level BETWEEN 2 AND 5) REFERENCES price_levels(level),
    unit_price NUMERIC(12,2) NOT NULL CHECK (unit_price > 0),
    updated_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS product_level_prices_unique
    ON product_level_prices (product_id, COALESCE(unit_id, '00000000-0000-0000-0000-000000000000'::uuid), price_level);

-- Raising a member's level hands them a cheaper price, so it is head office's
-- call, not the till's. Fresh databases get the key from the seed lists too.
INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at) VALUES
    ('c0a1e0b2-75a1-4c5e-9b11-000000000080', 'customer.level.manage', 'กำหนดระดับสมาชิก',
     'เลื่อนหรือลดระดับสมาชิก และตั้งชื่อระดับกับระดับราคาที่แต่ละระดับได้รับ', NOW(), NOW())
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r CROSS JOIN permissions p
WHERE r.role_key IN ('super_admin', 'central_admin')
  AND p.permission_key = 'customer.level.manage'
ON CONFLICT DO NOTHING;
