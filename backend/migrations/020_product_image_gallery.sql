CREATE TABLE IF NOT EXISTS product_images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    storage_key TEXT NOT NULL,
    mime_type TEXT NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    sha256 CHAR(64),
    source_url TEXT,
    source_branch_code TEXT,
    source_row_index INTEGER,
    source_name TEXT,
    alt_text TEXT NOT NULL DEFAULT '',
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_id, storage_key),
    UNIQUE (product_id, sort_order)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_product_images_primary
    ON product_images(product_id) WHERE is_primary = TRUE;
CREATE INDEX IF NOT EXISTS idx_product_images_product_order
    ON product_images(product_id, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_product_images_storage_key
    ON product_images(storage_key);

INSERT INTO product_images (
    product_id, storage_key, mime_type, alt_text, is_primary, sort_order
)
SELECT id, image_storage_key, COALESCE(NULLIF(image_mime_type, ''), 'image/jpeg'), name, TRUE, 0
FROM products
WHERE COALESCE(image_storage_key, '') <> ''
ON CONFLICT (product_id, storage_key) DO NOTHING;
