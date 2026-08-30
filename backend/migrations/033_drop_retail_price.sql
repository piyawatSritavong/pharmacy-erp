-- business-flow.md pricing rule: there is ONE sale price —
-- "ราคาขายตั้งต้น" on the catalog, with an optional per-branch override
-- ("ราคาขายเฉพาะสาขา") that overrides that same field. "ราคาปลีก" was a
-- separate third price and is no longer read by any code path (all Go
-- structs, SQL and UI references were removed alongside this migration).
--
-- The column is NOT empty (≈692 seeded products carry a retail value roughly
-- 12% above base_selling_price, imported from the Ocha catalog), so the data
-- is snapshotted before the column is dropped rather than silently destroyed.
-- base_selling_price stays authoritative; this table exists only so the old
-- values can be inspected or restored if the pricing decision is ever
-- revisited. It is safe to drop once no longer wanted.
CREATE TABLE IF NOT EXISTS retired_product_retail_prices (
    product_id         UUID PRIMARY KEY,
    sku                TEXT NOT NULL,
    base_selling_price NUMERIC(12,2) NOT NULL,
    retail_price       NUMERIC(12,2) NOT NULL,
    retired_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE retired_product_retail_prices IS
    'Snapshot of products.retail_price taken before the column was dropped (migration 033). business-flow.md defines a single sale price; retail_price is obsolete. Reference only — nothing reads this table.';

INSERT INTO retired_product_retail_prices (product_id, sku, base_selling_price, retail_price)
SELECT id, sku, base_selling_price, retail_price
FROM products
WHERE retail_price IS NOT NULL AND retail_price <> 0
ON CONFLICT (product_id) DO NOTHING;

ALTER TABLE products DROP COLUMN IF EXISTS retail_price;
