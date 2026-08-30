CREATE TABLE IF NOT EXISTS product_source_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    alias_name TEXT NOT NULL,
    normalized_alias TEXT NOT NULL,
    source_branch_code TEXT,
    source_row_index INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, normalized_alias)
);

CREATE INDEX IF NOT EXISTS idx_product_source_aliases_search
    ON product_source_aliases USING GIN (LOWER(alias_name) gin_trgm_ops);
