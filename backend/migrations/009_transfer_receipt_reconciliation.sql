ALTER TABLE transfer_items
    ADD COLUMN IF NOT EXISTS received_quantity INTEGER CHECK (received_quantity >= 0),
    ADD COLUMN IF NOT EXISTS discrepancy_note TEXT NOT NULL DEFAULT '';

ALTER TABLE transfers
    DROP COLUMN IF EXISTS qr_code;

CREATE INDEX IF NOT EXISTS idx_transfer_items_transfer_product
    ON transfer_items (transfer_id, product_id);
