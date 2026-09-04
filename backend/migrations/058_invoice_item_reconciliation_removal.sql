-- A month-end close can now strike single lines off a bill: the lines the
-- warehouse holds Ghost Stock for go back to it, and the bill survives holding
-- only what is left. (Before this, a bill was hidden whole or repriced whole.)
--
-- The struck line is FLAGGED, never deleted. reconciliation_item_snapshots and
-- reconciliation_logs both hold RESTRICT foreign keys onto invoice_items
-- precisely so the audit trail cannot be destroyed — and deleting the row would
-- destroy the evidence of what the close did. Every screen that shows a bill
-- filters these out, so the customer's copy shows one line while the close can
-- still be traced back line by line.
ALTER TABLE invoice_items
    ADD COLUMN IF NOT EXISTS reconciliation_removed_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS invoice_items_active_idx
    ON invoice_items (invoice_id)
    WHERE reconciliation_removed_at IS NULL;
