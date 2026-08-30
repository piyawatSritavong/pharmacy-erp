-- พักบิล — a POS-only checkout suspend buffer.
--
-- Deliberately NOT a quotation: no document number, no stock movement, no
-- approval, no accounting effect. It is the cashier's "hold this cart while
-- the customer keeps shopping" scratchpad and nothing more.
--
-- Items live in JSONB rather than a child table on purpose: the row is a
-- short-lived snapshot (24h), and POS checkout re-validates every product,
-- lot, price and quantity from scratch when the bill is resumed — so
-- referential integrity here would buy nothing and cost a join.
CREATE TABLE IF NOT EXISTS parked_bills (
    id               UUID PRIMARY KEY,
    branch_id        UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    -- The POS account that parked it; shown so a colleague knows whose bill
    -- it is during a shift handover.
    created_by       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Customers are free text throughout this system (there is no customers
    -- table), so these mirror invoices.customer_name / customer_tax_id.
    customer_name    TEXT NOT NULL DEFAULT '',
    customer_tax_id  TEXT NOT NULL DEFAULT '',
    full_tax_invoice BOOLEAN NOT NULL DEFAULT FALSE,
    note             TEXT NOT NULL DEFAULT '',
    -- [{product_id, inventory_lot_id, quantity, unit_price, discount_amount,
    --   product_name, sku, lot_number}]
    items            JSONB NOT NULL,
    item_count       INTEGER NOT NULL DEFAULT 0,
    estimated_total  NUMERIC(12,2) NOT NULL DEFAULT 0,
    -- Auto-expiry: rows past this are hidden from the list and purged lazily
    -- on the next read, so no scheduler is required.
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT parked_bills_items_is_array CHECK (jsonb_typeof(items) = 'array')
);

CREATE INDEX IF NOT EXISTS idx_parked_bills_branch_active
    ON parked_bills (branch_id, expires_at DESC);
