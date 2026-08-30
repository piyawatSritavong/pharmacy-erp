-- Atomic month-end reconciliation.
--
-- The operational inventory schema historically calls the private bucket
-- "ghost".  The API and audit trail expose it as "ghost"; keeping the
-- physical column name avoids a destructive rewrite of every stock ledger.

ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS request_full_tax_invoice BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS hidden_by_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS original_invoice_number TEXT;

-- A branch must not cascade-delete its invoice history. Branch removal now
-- fails until invoices have been retained/migrated explicitly.
ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_branch_id_fkey;
ALTER TABLE invoices
    ADD CONSTRAINT invoices_branch_id_fkey
    FOREIGN KEY (branch_id) REFERENCES branches(id) ON DELETE RESTRICT;

UPDATE invoices
SET request_full_tax_invoice = (tax_invoice_type = 'full')
WHERE request_full_tax_invoice IS DISTINCT FROM (tax_invoice_type = 'full');

ALTER TABLE invoice_items
    ADD COLUMN IF NOT EXISTS reconciliation_discount_amount NUMERIC(14,2) NOT NULL DEFAULT 0
        CHECK (reconciliation_discount_amount >= 0),
    ADD COLUMN IF NOT EXISTS reconciled_at TIMESTAMPTZ;

-- A hidden invoice retains its original number for the superadmin audit view.
-- Active documents alone must be unique so a surviving invoice can fill a
-- number released by a hidden one.
ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_invoice_number_key;
DROP INDEX IF EXISTS invoices_invoice_number_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_invoices_active_invoice_number
    ON invoices (invoice_number)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_invoices_visibility
    ON invoices (branch_id, issued_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_invoices_hidden
    ON invoices (deleted_at DESC, hidden_by_id)
    WHERE deleted_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS month_end_reconciliations (
    id UUID PRIMARY KEY,
    reconciliation_number TEXT NOT NULL UNIQUE,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    branch_ids UUID[] NOT NULL,
    target_revenue NUMERIC(14,2) NOT NULL CHECK (target_revenue >= 0),
    original_revenue NUMERIC(14,2) NOT NULL CHECK (original_revenue >= 0),
    suppressed_revenue NUMERIC(14,2) NOT NULL CHECK (suppressed_revenue >= 0),
    adjustment_reduction NUMERIC(14,2) NOT NULL CHECK (adjustment_reduction >= 0),
    final_revenue NUMERIC(14,2) NOT NULL CHECK (final_revenue >= 0),
    suppressed_invoice_count INTEGER NOT NULL DEFAULT 0 CHECK (suppressed_invoice_count >= 0),
    adjusted_item_count INTEGER NOT NULL DEFAULT 0 CHECK (adjusted_item_count >= 0),
    source_hash TEXT NOT NULL,
    finalized_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    finalized_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (period_end >= period_start),
    CHECK (ABS(final_revenue - target_revenue) < 0.01)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_month_end_reconciliations_branch_period
    ON month_end_reconciliations (period_start, period_end, branch_ids);
CREATE INDEX IF NOT EXISTS idx_month_end_reconciliations_finalized
    ON month_end_reconciliations (finalized_at DESC);

CREATE TABLE IF NOT EXISTS reconciliation_logs (
    id UUID PRIMARY KEY,
    reconciliation_id UUID NOT NULL REFERENCES month_end_reconciliations(id) ON DELETE RESTRICT,
    log_type TEXT NOT NULL CHECK (log_type IN (
        'invoice_suppressed', 'invoice_renumbered', 'price_adjusted', 'stock_deducted'
    )),
    invoice_id UUID REFERENCES invoices(id) ON DELETE RESTRICT,
    invoice_item_id UUID REFERENCES invoice_items(id) ON DELETE RESTRICT,
    branch_id UUID REFERENCES branches(id) ON DELETE RESTRICT,
    product_id UUID REFERENCES products(id) ON DELETE RESTRICT,
    original_invoice_number TEXT NOT NULL DEFAULT '',
    new_invoice_number TEXT NOT NULL DEFAULT '',
    old_unit_price NUMERIC(14,2),
    new_unit_price NUMERIC(14,2),
    variance_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (variance_amount >= 0),
    stock_bucket TEXT CHECK (stock_bucket IS NULL OR stock_bucket IN ('real', 'ghost')),
    stock_quantity INTEGER NOT NULL DEFAULT 0 CHECK (stock_quantity >= 0),
    before_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    after_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reconciliation_logs_reconciliation
    ON reconciliation_logs (reconciliation_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_reconciliation_logs_invoice
    ON reconciliation_logs (invoice_id, created_at DESC)
    WHERE invoice_id IS NOT NULL;

-- Month-end contains ghost-stock quantities and hidden invoice details.
-- It is role-restricted in HTTP middleware as well, but removing grants keeps
-- navigation and freshly issued JWTs aligned with that hard boundary.
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.role_key <> 'super_admin'
  AND (p.permission_key = 'month_end.manage' OR p.permission_key LIKE 'month_end.%');

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'super_admin'
  AND (p.permission_key = 'month_end.manage' OR p.permission_key LIKE 'month_end.%')
ON CONFLICT DO NOTHING;

-- The private bucket is a literal role boundary, not merely a configurable
-- permission. Remove older grants left by broad admin-role migrations.
DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND r.role_key <> 'super_admin'
  AND p.permission_key = 'inventory.ghost.manage';

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'super_admin'
  AND p.permission_key = 'inventory.ghost.manage'
ON CONFLICT DO NOTHING;
