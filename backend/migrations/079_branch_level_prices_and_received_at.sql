-- Price levels belong to the product, and a branch prices them for itself.
--
-- A row with branch_id NULL is the catalog's price for that level (set from
-- the product catalog, like the warehouse shelf price). A row with a branch
-- is that branch's own price for the level and wins over the catalog row,
-- the same way branch_product_prices wins over base_selling_price.
ALTER TABLE product_level_prices
    ADD COLUMN IF NOT EXISTS branch_id UUID REFERENCES branches(id) ON DELETE CASCADE;

DROP INDEX IF EXISTS product_level_prices_unique;
CREATE UNIQUE INDEX IF NOT EXISTS product_level_prices_unique
    ON product_level_prices (
        product_id,
        COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(unit_id, '00000000-0000-0000-0000-000000000000'::uuid),
        price_level
    );

-- When the money was taken, apart from when the row was written. They are the
-- same moment for every payment taken at a till; they differ only for sales
-- brought in from a shop's previous system, which were paid on their own day
-- and recorded later. created_at stays the time the row was written.
ALTER TABLE invoice_payments
    ADD COLUMN IF NOT EXISTS received_at TIMESTAMPTZ;
UPDATE invoice_payments SET received_at = created_at WHERE received_at IS NULL;
ALTER TABLE invoice_payments
    ALTER COLUMN received_at SET DEFAULT NOW(),
    ALTER COLUMN received_at SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_invoice_payments_received_at ON invoice_payments (received_at);
