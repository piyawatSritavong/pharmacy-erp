-- Central non-selling warehouse, branch price inheritance, and explicit sale lots.

ALTER TABLE branches
    ADD COLUMN IF NOT EXISTS sales_enabled BOOLEAN NOT NULL DEFAULT TRUE;

-- A main warehouse is a distribution node and must never issue sales documents.
UPDATE branches SET sales_enabled = FALSE WHERE branch_type = 'main_warehouse';

ALTER TABLE branches
    DROP CONSTRAINT IF EXISTS branches_main_warehouse_no_sales;
ALTER TABLE branches
    ADD CONSTRAINT branches_main_warehouse_no_sales
    CHECK (branch_type <> 'main_warehouse' OR sales_enabled = FALSE);

-- MES becomes a retail branch. WH is the single central warehouse.
UPDATE branches
SET branch_type = 'branch', sales_enabled = TRUE, updated_at = NOW()
WHERE code = 'MES';

INSERT INTO branches (
    id, code, name, address, branch_type, parent_branch_id,
    active, sales_enabled, created_at, updated_at
) SELECT
    'f0d67341-54ff-5475-8ca0-ed261f96c941', 'WH', 'โกดัง', '',
    'main_warehouse', NULL, TRUE, FALSE, NOW(), NOW()
WHERE EXISTS (SELECT 1 FROM branches WHERE code = 'MES')
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    branch_type = EXCLUDED.branch_type,
    parent_branch_id = NULL,
    active = TRUE,
    sales_enabled = FALSE,
    updated_at = NOW();

UPDATE branches
SET parent_branch_id = (SELECT id FROM branches WHERE code = 'WH'),
    sales_enabled = TRUE,
    updated_at = NOW()
WHERE code IN ('MES', 'PHH', 'PHS', 'NPT');

CREATE UNIQUE INDEX IF NOT EXISTS idx_branches_single_active_main_warehouse
    ON branches (branch_type)
    WHERE branch_type = 'main_warehouse' AND active = TRUE;

INSERT INTO document_sequences (
    id, branch_id, doc_type, prefix, next_number, is_locked, created_at, updated_at
)
SELECT gen_random_uuid(), b.id, sequence.doc_type, sequence.prefix, 1, FALSE, NOW(), NOW()
FROM branches b
CROSS JOIN (VALUES
    ('invoice'::text, 'BL'::text),
    ('quotation'::text, 'QT'::text),
    ('purchase_order'::text, 'PO'::text)
) AS sequence(doc_type, prefix)
WHERE b.code = 'WH'
ON CONFLICT (branch_id, doc_type) DO NOTHING;

-- Every global product belongs to the warehouse catalog, even with zero stock.
INSERT INTO inventory (
    id, branch_id, product_id, qty_real, qty_ghost, created_at, updated_at
)
SELECT gen_random_uuid(), warehouse.id, product.id, 0, 0, NOW(), NOW()
FROM branches warehouse
CROSS JOIN products product
WHERE warehouse.code = 'WH'
ON CONFLICT (branch_id, product_id) DO NOTHING;

-- Copy current remaining MES lots without moving or reducing MES stock.
INSERT INTO inventory_lots (
    id, branch_id, product_id, stock_bucket, lot_number, expires_on,
    received_quantity, remaining_quantity, unit_cost, source_type,
    source_id, source_item_id, origin_lot_id, received_at, created_at, updated_at
)
SELECT gen_random_uuid(), warehouse.id, source.product_id, source.stock_bucket,
       source.lot_number, source.expires_on, source.remaining_quantity,
       source.remaining_quantity, source.unit_cost, 'warehouse_bootstrap_copy',
       NULL, NULL, source.id, source.received_at, NOW(), NOW()
FROM inventory_lots source
INNER JOIN branches mes ON mes.id = source.branch_id AND mes.code = 'MES'
CROSS JOIN branches warehouse
WHERE warehouse.code = 'WH'
  AND source.remaining_quantity > 0
  AND NOT EXISTS (
      SELECT 1 FROM inventory_lots existing
      WHERE existing.branch_id = warehouse.id
        AND existing.origin_lot_id = source.id
        AND existing.source_type = 'warehouse_bootstrap_copy'
  );

UPDATE inventory target
SET qty_real = COALESCE((
        SELECT SUM(lot.remaining_quantity)::integer
        FROM inventory_lots lot
        WHERE lot.branch_id = target.branch_id
          AND lot.product_id = target.product_id
          AND lot.stock_bucket = 'real'
    ), 0),
    qty_ghost = COALESCE((
        SELECT SUM(lot.remaining_quantity)::integer
        FROM inventory_lots lot
        WHERE lot.branch_id = target.branch_id
          AND lot.product_id = target.product_id
          AND lot.stock_bucket = 'ghost'
    ), 0),
    updated_at = NOW()
FROM branches warehouse
WHERE warehouse.code = 'WH'
  AND target.branch_id = warehouse.id;

INSERT INTO inventory_movements (
    id, branch_id, product_id, movement_type, stock_bucket, quantity_delta,
    reference_type, reference_id, note, performed_by, created_at
)
SELECT gen_random_uuid(), lot.branch_id, lot.product_id, 'opening_balance',
       lot.stock_bucket, lot.remaining_quantity, 'warehouse_bootstrap_copy',
       lot.id, 'คัดลอกยอดคงเหลือเริ่มต้นจาก MES', NULL, NOW()
FROM inventory_lots lot
INNER JOIN branches warehouse ON warehouse.id = lot.branch_id AND warehouse.code = 'WH'
WHERE lot.source_type = 'warehouse_bootstrap_copy'
  AND NOT EXISTS (
      SELECT 1 FROM inventory_movements movement
      WHERE movement.reference_type = 'warehouse_bootstrap_copy'
        AND movement.reference_id = lot.id
  );

INSERT INTO inventory_movement_lots (
    id, inventory_movement_id, inventory_lot_id, quantity_delta, created_at
)
SELECT gen_random_uuid(), movement.id, lot.id, lot.remaining_quantity, NOW()
FROM inventory_movements movement
INNER JOIN inventory_lots lot ON lot.id = movement.reference_id
WHERE movement.reference_type = 'warehouse_bootstrap_copy'
ON CONFLICT (inventory_movement_id, inventory_lot_id) DO NOTHING;

ALTER TABLE invoice_items
    ADD COLUMN IF NOT EXISTS inventory_lot_id UUID REFERENCES inventory_lots(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS lot_number_snapshot TEXT,
    ADD COLUMN IF NOT EXISTS lot_received_at_snapshot TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS lot_expires_on_snapshot DATE;

CREATE INDEX IF NOT EXISTS idx_invoice_items_inventory_lot
    ON invoice_items (inventory_lot_id)
    WHERE inventory_lot_id IS NOT NULL;
