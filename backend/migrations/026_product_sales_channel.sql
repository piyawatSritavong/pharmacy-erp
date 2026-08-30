-- Part B, Rule 2 (Central Product Catalog): products is already the single
-- cross-branch table every branch pulls from — the missing piece was a page
-- to browse/manage it directly, not a new data layer. This adds the one
-- genuinely new field the rule's "equipment, medicine, and online-sale items
-- together" phrasing calls for: which sales channel(s) a product is eligible
-- for, surfaced as a facet on the new /product-catalog page.
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS sales_channel TEXT NOT NULL DEFAULT 'in_store'
    CHECK (sales_channel IN ('in_store', 'online', 'both'));
