-- Indexes for the read paths that grew with the number of bills.
--
-- invoice_payments had no index on invoice_id, so every "how much of this bill
-- is paid" lookup scanned the whole payments table: the sales summary, the
-- receivables list and the payment-method trigger each did it once per bill.
-- Measured on a demo database with 2,658 bills, the summary's aggregate took
-- 257 ms; the cost grows with bills × payments, so a month of real trading
-- would have taken tens of seconds.
CREATE INDEX IF NOT EXISTS idx_invoice_payments_invoice
    ON invoice_payments (invoice_id);

-- The sales summary totals cash and transfers by when the money arrived.
CREATE INDEX IF NOT EXISTS idx_invoice_payments_created_at
    ON invoice_payments (created_at);

-- Head office reads every branch at once (dashboard, sales history, summary),
-- ordered and bounded by issue time. The existing indexes all lead with
-- branch_id or another column, so an all-branch date range read the table.
CREATE INDEX IF NOT EXISTS idx_invoices_issued
    ON invoices (issued_at DESC)
    WHERE invoice_status = 'issued';

-- Per-product reads: sales of a product, its stock card and its lots.
CREATE INDEX IF NOT EXISTS idx_invoice_items_product
    ON invoice_items (product_id);
CREATE INDEX IF NOT EXISTS idx_inventory_movements_product
    ON inventory_movements (product_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inventory_lots_product
    ON inventory_lots (product_id);
CREATE INDEX IF NOT EXISTS idx_inventory_product
    ON inventory (product_id);
