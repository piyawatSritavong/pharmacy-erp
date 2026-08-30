-- business-flow.md, สต๊อกจริง/สต๊อกผี: "แสดง ข้อมูลทุกอย่างตาม รายการสินค้า
-- แต่สามารถเพิ่มรูปใหม่ได้" — a branch keeps the catalog's images and can add
-- its own on top (รูปภาพ: catalog has the main image; stock pages show
-- catalog images + branch-added ones).
--
-- branch_id NULL  = catalog image, visible from every branch (today's rows).
-- branch_id SET   = added at that branch, visible only there.
ALTER TABLE product_images
    ADD COLUMN IF NOT EXISTS branch_id UUID REFERENCES branches(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_product_images_product_branch
    ON product_images (product_id, branch_id);

-- The catalog's primary image is the product's own; a branch-added image must
-- never claim is_primary, or it would hijack the thumbnail shown everywhere.
ALTER TABLE product_images
    DROP CONSTRAINT IF EXISTS product_images_branch_not_primary;
ALTER TABLE product_images
    ADD CONSTRAINT product_images_branch_not_primary
    CHECK (branch_id IS NULL OR is_primary = FALSE);
