CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_products_name_trgm
    ON products USING gin (LOWER(name) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_products_sku_trgm
    ON products USING gin (LOWER(sku) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_products_barcode_trgm
    ON products USING gin (LOWER(COALESCE(barcode, '')) gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_products_category_active_name
    ON products (category_id, active, name, id);
CREATE INDEX IF NOT EXISTS idx_inventory_branch_product
    ON inventory (branch_id, product_id);
