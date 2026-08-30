-- Operational refactor: government documents, stock transfer requests,
-- and retirement of checks, installments, and external accounting.

ALTER TABLE quotations
    ADD COLUMN IF NOT EXISTS is_government_mode BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE quotations q
SET is_government_mode = TRUE
WHERE EXISTS (
    SELECT 1
    FROM quotation_items qi
    WHERE qi.quotation_id = q.id
      AND (qi.alias_id IS NOT NULL OR qi.price_source = 'government_alias_default')
);

CREATE INDEX IF NOT EXISTS idx_quotations_government_created
    ON quotations (is_government_mode, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_invoices_government_issued
    ON invoices (is_government_mode, issued_at DESC);

CREATE TABLE IF NOT EXISTS stock_transfer_requests (
    id UUID PRIMARY KEY,
    destination_branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    requested_quantity INTEGER NOT NULL CHECK (requested_quantity > 0),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    source_branch_id UUID REFERENCES branches(id) ON DELETE SET NULL,
    approved_stock_bucket TEXT CHECK (approved_stock_bucket IN ('real', 'ghost')),
    transfer_id UUID UNIQUE REFERENCES transfers(id) ON DELETE SET NULL,
    requested_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO stock_transfer_requests (
    id, destination_branch_id, product_id, requested_quantity, status,
    approved_stock_bucket, requested_by, reviewed_by, reviewed_at, created_at, updated_at
)
SELECT
    id, branch_id, product_id, requested_quantity, status,
    approved_stock_bucket, requested_by, reviewed_by, reviewed_at, created_at, updated_at
FROM inventory_receipt_requests
ON CONFLICT (id) DO NOTHING;

DROP TABLE IF EXISTS inventory_receipt_requests;

CREATE UNIQUE INDEX IF NOT EXISTS idx_stock_transfer_requests_pending_product
    ON stock_transfer_requests (destination_branch_id, product_id)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_stock_transfer_requests_status_created
    ON stock_transfer_requests (status, created_at DESC);

-- Preserve the correct paid/unpaid status before retiring installment state.
UPDATE invoices i
SET payment_status = CASE
    WHEN COALESCE((
        SELECT SUM(ip.amount)
        FROM invoice_payments ip
        WHERE ip.invoice_id = i.id
          AND ip.payment_type IN ('cash', 'bank_transfer')
    ), 0) >= i.total_amount THEN 'paid'
    ELSE 'unpaid'
END,
updated_at = NOW()
WHERE i.payment_status = 'installment';

DROP TABLE IF EXISTS installment_requests;
DROP TABLE IF EXISTS installment_payments;
DROP TABLE IF EXISTS installment_plans;

DELETE FROM invoice_payments WHERE payment_type = 'check';
DROP TABLE IF EXISTS payment_invoice_map;
DROP TABLE IF EXISTS checks;

UPDATE invoices i
SET payment_status = CASE
    WHEN COALESCE((
        SELECT SUM(ip.amount)
        FROM invoice_payments ip
        WHERE ip.invoice_id = i.id
    ), 0) >= i.total_amount THEN 'paid'
    ELSE 'unpaid'
END,
updated_at = NOW();

ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_payment_status_check;
ALTER TABLE invoices
    ADD CONSTRAINT invoices_payment_status_check
    CHECK (payment_status IN ('unpaid', 'paid'));

ALTER TABLE invoice_payments DROP CONSTRAINT IF EXISTS invoice_payments_payment_type_check;
ALTER TABLE invoice_payments
    ADD CONSTRAINT invoice_payments_payment_type_check
    CHECK (payment_type IN ('cash', 'bank_transfer'));

ALTER TABLE products DROP COLUMN IF EXISTS installment_price;

DROP TABLE IF EXISTS accounting_workpapers;
DROP TABLE IF EXISTS other_income_entries;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.permission_key IN (
      'installment.view',
      'installment.manage',
      'installment.collect',
      'installment.request',
      'finance.manage.global',
      'inventory.receive.request'
  );

DELETE FROM permissions
WHERE permission_key IN (
    'installment.view',
    'installment.manage',
    'installment.collect',
    'installment.request',
    'finance.manage.global',
    'inventory.receive.request'
);

INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES (
    gen_random_uuid(),
    'transfer.request.branch',
    'ส่งคำขอโอนสินค้า',
    'ส่งคำขอโอนสินค้าจากสาขาหน้าร้านเพื่อให้ผู้ดูแลเลือกต้นทาง',
    NOW(),
    NOW()
)
ON CONFLICT (permission_key) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
INNER JOIN permissions p ON p.permission_key = 'transfer.request.branch'
WHERE r.role_key = 'branch_pos'
ON CONFLICT DO NOTHING;
