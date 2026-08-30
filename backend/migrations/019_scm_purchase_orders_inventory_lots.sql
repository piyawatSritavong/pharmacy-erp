-- SCM purchasing, suppliers, branch product policies and inventory lot tracking.

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS max_discount_amount NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (max_discount_amount >= 0),
    ADD COLUMN IF NOT EXISTS low_stock_real_threshold INTEGER NOT NULL DEFAULT 0 CHECK (low_stock_real_threshold >= 0),
    ADD COLUMN IF NOT EXISTS low_stock_ghost_threshold INTEGER NOT NULL DEFAULT 0 CHECK (low_stock_ghost_threshold >= 0),
    ADD COLUMN IF NOT EXISTS tracks_expiry BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS expiry_warning_days INTEGER NOT NULL DEFAULT 30 CHECK (expiry_warning_days BETWEEN 0 AND 3650);

CREATE TABLE IF NOT EXISTS branch_product_settings (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    max_discount_amount NUMERIC(12,2) CHECK (max_discount_amount IS NULL OR max_discount_amount >= 0),
    low_stock_real_threshold INTEGER CHECK (low_stock_real_threshold IS NULL OR low_stock_real_threshold >= 0),
    low_stock_ghost_threshold INTEGER CHECK (low_stock_ghost_threshold IS NULL OR low_stock_ghost_threshold >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (branch_id, product_id)
);

CREATE TABLE IF NOT EXISTS suppliers (
    id UUID PRIMARY KEY,
    supplier_code TEXT NOT NULL UNIQUE,
    legal_name TEXT NOT NULL,
    tax_id TEXT,
    company_branch_type TEXT NOT NULL DEFAULT 'head_office'
        CHECK (company_branch_type IN ('head_office', 'branch')),
    company_branch_number TEXT NOT NULL DEFAULT '',
    address_line TEXT NOT NULL DEFAULT '',
    subdistrict TEXT NOT NULL DEFAULT '',
    district TEXT NOT NULL DEFAULT '',
    province TEXT NOT NULL DEFAULT '',
    postal_code TEXT NOT NULL DEFAULT '',
    contact_name TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    payment_terms_days INTEGER NOT NULL DEFAULT 0 CHECK (payment_terms_days >= 0),
    notes TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_suppliers_legal_name_ci
    ON suppliers (LOWER(BTRIM(legal_name)));
CREATE UNIQUE INDEX IF NOT EXISTS idx_suppliers_tax_id_unique
    ON suppliers (tax_id)
    WHERE tax_id IS NOT NULL AND BTRIM(tax_id) <> '';
CREATE INDEX IF NOT EXISTS idx_suppliers_active_name
    ON suppliers (active DESC, legal_name, id);

CREATE TABLE IF NOT EXISTS purchase_orders (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    supplier_id UUID NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    po_number TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'posted' CHECK (status IN ('posted', 'cancelled')),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    purchased_at TIMESTAMPTZ NOT NULL,
    posted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    supplier_document_number TEXT NOT NULL DEFAULT '',
    supplier_code_snapshot TEXT NOT NULL,
    supplier_name_snapshot TEXT NOT NULL,
    supplier_tax_id_snapshot TEXT NOT NULL DEFAULT '',
    supplier_address_snapshot TEXT NOT NULL DEFAULT '',
    supplier_contact_snapshot TEXT NOT NULL DEFAULT '',
    vat_mode TEXT NOT NULL DEFAULT 'exclusive' CHECK (vat_mode IN ('none', 'exclusive', 'inclusive')),
    vat_rate NUMERIC(5,2) NOT NULL DEFAULT 7 CHECK (vat_rate BETWEEN 0 AND 100),
    subtotal NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (subtotal >= 0),
    line_discount_total NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (line_discount_total >= 0),
    header_discount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (header_discount >= 0),
    shipping_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (shipping_amount >= 0),
    tax_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    total_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    notes TEXT NOT NULL DEFAULT '',
    correction_reason TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    cancelled_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_purchase_orders_branch_purchased
    ON purchase_orders (branch_id, purchased_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_purchase_orders_supplier_purchased
    ON purchase_orders (supplier_id, purchased_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_purchase_orders_status_purchased
    ON purchase_orders (status, purchased_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS purchase_order_items (
    id UUID PRIMARY KEY,
    purchase_order_id UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE RESTRICT,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    product_sku_snapshot TEXT NOT NULL,
    product_name_snapshot TEXT NOT NULL,
    unit_name_snapshot TEXT NOT NULL,
    stock_bucket TEXT NOT NULL CHECK (stock_bucket IN ('real', 'ghost')),
    received_quantity INTEGER NOT NULL CHECK (received_quantity > 0),
    unit_cost NUMERIC(12,2) NOT NULL CHECK (unit_cost >= 0),
    line_discount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (line_discount >= 0),
    line_subtotal NUMERIC(14,2) NOT NULL CHECK (line_subtotal >= 0),
    tax_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    line_total NUMERIC(14,2) NOT NULL CHECK (line_total >= 0),
    lot_number TEXT NOT NULL,
    expires_on DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_purchase_order_items_order
    ON purchase_order_items (purchase_order_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_purchase_order_items_product
    ON purchase_order_items (product_id, purchase_order_id);

CREATE TABLE IF NOT EXISTS purchase_order_events (
    id UUID PRIMARY KEY,
    purchase_order_id UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (event_type IN ('posted', 'corrected', 'cancelled')),
    revision INTEGER NOT NULL CHECK (revision > 0),
    note TEXT NOT NULL DEFAULT '',
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    event_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_purchase_order_events_order_time
    ON purchase_order_events (purchase_order_id, event_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS inventory_lots (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    stock_bucket TEXT NOT NULL CHECK (stock_bucket IN ('real', 'ghost')),
    lot_number TEXT NOT NULL,
    expires_on DATE,
    received_quantity INTEGER NOT NULL CHECK (received_quantity >= 0),
    remaining_quantity INTEGER NOT NULL CHECK (remaining_quantity >= 0),
    unit_cost NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (unit_cost >= 0),
    source_type TEXT NOT NULL,
    source_id UUID,
    source_item_id UUID,
    origin_lot_id UUID REFERENCES inventory_lots(id) ON DELETE SET NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (remaining_quantity <= received_quantity)
);

CREATE INDEX IF NOT EXISTS idx_inventory_lots_fefo
    ON inventory_lots (branch_id, product_id, stock_bucket, expires_on, received_at, id)
    WHERE remaining_quantity > 0;
CREATE INDEX IF NOT EXISTS idx_inventory_lots_source_item
    ON inventory_lots (source_item_id)
    WHERE source_item_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS inventory_movement_lots (
    id UUID PRIMARY KEY,
    inventory_movement_id UUID NOT NULL REFERENCES inventory_movements(id) ON DELETE CASCADE,
    inventory_lot_id UUID NOT NULL REFERENCES inventory_lots(id) ON DELETE RESTRICT,
    quantity_delta INTEGER NOT NULL CHECK (quantity_delta <> 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (inventory_movement_id, inventory_lot_id)
);

CREATE INDEX IF NOT EXISTS idx_inventory_movement_lots_lot
    ON inventory_movement_lots (inventory_lot_id, created_at DESC);

CREATE TABLE IF NOT EXISTS transfer_item_lot_allocations (
    id UUID PRIMARY KEY,
    transfer_item_id UUID NOT NULL REFERENCES transfer_items(id) ON DELETE CASCADE,
    source_lot_id UUID NOT NULL REFERENCES inventory_lots(id) ON DELETE RESTRICT,
    destination_lot_id UUID REFERENCES inventory_lots(id) ON DELETE RESTRICT,
    dispatched_quantity INTEGER NOT NULL CHECK (dispatched_quantity > 0),
    received_quantity INTEGER NOT NULL DEFAULT 0 CHECK (received_quantity >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (transfer_item_id, source_lot_id)
);

-- Existing inventory becomes a non-expiring opening lot without creating a
-- second movement or changing any aggregate quantity.
INSERT INTO inventory_lots (
    id, branch_id, product_id, stock_bucket, lot_number, expires_on,
    received_quantity, remaining_quantity, unit_cost, source_type,
    source_id, source_item_id, received_at, created_at, updated_at
)
SELECT
    gen_random_uuid(), i.branch_id, i.product_id, bucket.stock_bucket,
    'OPENING-' || UPPER(SUBSTRING(REPLACE(i.id::text, '-', '') FROM 1 FOR 10)), NULL,
    bucket.quantity, bucket.quantity, p.cost_price, 'migration_opening',
    NULL, NULL, i.created_at, NOW(), NOW()
FROM inventory i
INNER JOIN products p ON p.id = i.product_id
CROSS JOIN LATERAL (
    VALUES ('real'::text, i.qty_real), ('ghost'::text, i.qty_ghost)
) AS bucket(stock_bucket, quantity)
WHERE bucket.quantity > 0
  AND NOT EXISTS (
      SELECT 1 FROM inventory_lots il
      WHERE il.branch_id = i.branch_id
        AND il.product_id = i.product_id
        AND il.stock_bucket = bucket.stock_bucket
  );

INSERT INTO document_sequences (
    id, branch_id, doc_type, prefix, next_number, is_locked, created_at, updated_at
)
SELECT gen_random_uuid(), b.id, 'purchase_order', 'PO', 1, FALSE, NOW(), NOW()
FROM branches b
ON CONFLICT (branch_id, doc_type) DO NOTHING;

INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES
    (gen_random_uuid(), 'suppliers.view.global', 'ดูบริษัทคู่ค้า', 'ดูบริษัทคู่ค้าส่วนกลาง', NOW(), NOW()),
    (gen_random_uuid(), 'suppliers.manage.global', 'จัดการบริษัทคู่ค้า', 'สร้าง แก้ไข และเก็บบริษัทคู่ค้า', NOW(), NOW()),
    (gen_random_uuid(), 'purchase_orders.view.global', 'ดูใบสั่งซื้อเข้า', 'ดูประวัติใบสั่งซื้อเข้าทุกสาขา', NOW(), NOW()),
    (gen_random_uuid(), 'purchase_orders.manage.global', 'จัดการใบสั่งซื้อเข้า', 'สร้าง แก้ไข และยกเลิกใบสั่งซื้อเข้า', NOW(), NOW())
ON CONFLICT (permission_key) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
INNER JOIN permissions p ON p.permission_key IN (
    'suppliers.view.global', 'suppliers.manage.global',
    'purchase_orders.view.global', 'purchase_orders.manage.global'
)
WHERE r.role_key = 'super_admin'
ON CONFLICT DO NOTHING;
