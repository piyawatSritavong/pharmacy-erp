-- Exact before-state snapshots for the superadmin month-end comparison report.
-- The private stock bucket is named "ghost" in storage, APIs, logs, and UI.

ALTER TABLE reconciliation_logs
    DROP CONSTRAINT IF EXISTS reconciliation_logs_stock_bucket_check;

UPDATE reconciliation_logs
SET stock_bucket = 'ghost'
WHERE stock_bucket = ('res' || 'erved');

ALTER TABLE reconciliation_logs
    ADD CONSTRAINT reconciliation_logs_stock_bucket_check
    CHECK (stock_bucket IS NULL OR stock_bucket IN ('real', 'ghost'));

UPDATE permissions
SET name = 'จัดการสต๊อกผี',
    description = 'ดูและจัดการสต๊อกผีภายใน'
WHERE permission_key = 'inventory.ghost.manage';

CREATE TABLE IF NOT EXISTS reconciliation_invoice_snapshots (
    reconciliation_id UUID NOT NULL REFERENCES month_end_reconciliations(id) ON DELETE RESTRICT,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    original_invoice_number TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    payment_method TEXT NOT NULL CHECK (payment_method IN ('cash', 'bank_transfer', 'mixed', 'unpaid')),
    request_full_tax_invoice BOOLEAN NOT NULL,
    original_subtotal NUMERIC(14,2) NOT NULL,
    original_tax_amount NUMERIC(14,2) NOT NULL,
    original_total_amount NUMERIC(14,2) NOT NULL,
    original_invoice_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (reconciliation_id, invoice_id)
);

CREATE INDEX IF NOT EXISTS idx_reconciliation_invoice_snapshots_scope
    ON reconciliation_invoice_snapshots (reconciliation_id, branch_id, issued_at, original_invoice_number);

CREATE TABLE IF NOT EXISTS reconciliation_item_snapshots (
    reconciliation_id UUID NOT NULL,
    invoice_item_id UUID NOT NULL REFERENCES invoice_items(id) ON DELETE RESTRICT,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    product_name TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    original_unit_price NUMERIC(14,2) NOT NULL,
    original_line_subtotal NUMERIC(14,2) NOT NULL,
    original_tax_amount NUMERIC(14,2) NOT NULL,
    original_line_total NUMERIC(14,2) NOT NULL,
    original_stock_bucket TEXT NOT NULL CHECK (original_stock_bucket IN ('real', 'ghost')),
    original_item_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (reconciliation_id, invoice_item_id),
    FOREIGN KEY (reconciliation_id, invoice_id)
        REFERENCES reconciliation_invoice_snapshots(reconciliation_id, invoice_id)
        ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_reconciliation_item_snapshots_invoice
    ON reconciliation_item_snapshots (reconciliation_id, invoice_id, invoice_item_id);

-- Best-effort snapshots for cycles finalized before this migration. Every
-- invoice touched by the reconciliation already carries original_invoice_number;
-- price logs restore the original line values for adjusted items.
WITH payment_summary AS (
    SELECT ip.invoice_id,
           COUNT(*) AS payment_count,
           BOOL_AND(ip.payment_type = 'cash') AS cash_only,
           BOOL_AND(ip.payment_type = 'bank_transfer') AS transfer_only,
           SUM(ip.amount) AS paid_amount
    FROM invoice_payments ip
    GROUP BY ip.invoice_id
), price_deltas AS (
    SELECT rl.reconciliation_id, rl.invoice_id,
           COALESCE(SUM(
               COALESCE((rl.before_data->>'line_total')::numeric, 0)
               - COALESCE((rl.after_data->>'line_total')::numeric, 0)
           ), 0) AS total_delta
    FROM reconciliation_logs rl
    WHERE rl.log_type = 'price_adjusted'
    GROUP BY rl.reconciliation_id, rl.invoice_id
)
INSERT INTO reconciliation_invoice_snapshots (
    reconciliation_id, invoice_id, branch_id, original_invoice_number, issued_at,
    payment_method, request_full_tax_invoice, original_subtotal, original_tax_amount,
    original_total_amount, original_invoice_data, created_at
)
SELECT r.id, i.id, i.branch_id, COALESCE(i.original_invoice_number, i.invoice_number), i.issued_at,
       CASE
           WHEN COALESCE(ps.payment_count, 0) = 0 OR COALESCE(ps.paid_amount, 0) < i.total_amount THEN 'unpaid'
           WHEN ps.cash_only THEN 'cash'
           WHEN ps.transfer_only THEN 'bank_transfer'
           ELSE 'mixed'
       END,
       i.request_full_tax_invoice,
       i.subtotal,
       i.tax_amount,
       i.total_amount + COALESCE(pd.total_delta, 0),
       jsonb_build_object(
           'invoice_number', COALESCE(i.original_invoice_number, i.invoice_number),
           'subtotal', i.subtotal,
           'tax_amount', i.tax_amount,
           'total_amount', i.total_amount + COALESCE(pd.total_delta, 0),
           'invoice_status', i.invoice_status,
           'payment_status', i.payment_status
       ),
       r.finalized_at
FROM month_end_reconciliations r
INNER JOIN invoices i
        ON i.branch_id = ANY(r.branch_ids)
       AND i.issued_at >= (r.period_start::timestamp AT TIME ZONE 'Asia/Bangkok')
       AND i.issued_at < ((r.period_end + 1)::timestamp AT TIME ZONE 'Asia/Bangkok')
LEFT JOIN payment_summary ps ON ps.invoice_id = i.id
LEFT JOIN price_deltas pd ON pd.reconciliation_id = r.id AND pd.invoice_id = i.id
WHERE i.original_invoice_number IS NOT NULL
   OR EXISTS (
       SELECT 1 FROM reconciliation_logs rl
       WHERE rl.reconciliation_id = r.id AND rl.invoice_id = i.id
   )
ON CONFLICT DO NOTHING;

WITH price_logs AS (
    SELECT DISTINCT ON (rl.reconciliation_id, rl.invoice_item_id)
           rl.reconciliation_id, rl.invoice_item_id,
           rl.old_unit_price,
           (rl.before_data->>'line_total')::numeric AS original_line_total
    FROM reconciliation_logs rl
    WHERE rl.log_type = 'price_adjusted' AND rl.invoice_item_id IS NOT NULL
    ORDER BY rl.reconciliation_id, rl.invoice_item_id, rl.created_at DESC, rl.id DESC
)
INSERT INTO reconciliation_item_snapshots (
    reconciliation_id, invoice_item_id, invoice_id, product_id, product_name,
    quantity, original_unit_price, original_line_subtotal, original_tax_amount,
    original_line_total, original_stock_bucket, original_item_data, created_at
)
SELECT ris.reconciliation_id, ii.id, ii.invoice_id, ii.product_id, ii.actual_product_name,
       ii.quantity,
       COALESCE(pl.old_unit_price, ii.unit_price),
       COALESCE(pl.old_unit_price, ii.unit_price) * ii.quantity,
       GREATEST(COALESCE(pl.original_line_total, ii.line_total)
                - (COALESCE(pl.old_unit_price, ii.unit_price) * ii.quantity), 0),
       COALESCE(pl.original_line_total, ii.line_total),
       ii.stock_bucket,
       jsonb_build_object(
           'product_name', ii.actual_product_name,
           'quantity', ii.quantity,
           'unit_price', COALESCE(pl.old_unit_price, ii.unit_price),
           'line_total', COALESCE(pl.original_line_total, ii.line_total),
           'stock_bucket', ii.stock_bucket
       ),
       ris.created_at
FROM reconciliation_invoice_snapshots ris
INNER JOIN invoice_items ii ON ii.invoice_id = ris.invoice_id
LEFT JOIN price_logs pl
       ON pl.reconciliation_id = ris.reconciliation_id
      AND pl.invoice_item_id = ii.id
ON CONFLICT DO NOTHING;
