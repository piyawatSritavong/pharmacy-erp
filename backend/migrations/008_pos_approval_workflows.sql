INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES
    ('81111111-1111-4111-8111-111111111111', 'inventory.receive.request', 'ส่งคำขอรับสินค้า', 'ส่งคำขอรับสินค้าเข้าจากหน้าร้านเพื่อให้ผู้ดูแลตรวจสอบ', NOW(), NOW()),
    ('82222222-2222-4222-8222-222222222222', 'installment.request', 'ส่งคำขอผ่อนชำระ', 'ส่งคำขอสร้างแผนผ่อนชำระจากหน้าร้าน', NOW(), NOW())
ON CONFLICT (permission_key) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
INNER JOIN permissions p ON p.permission_key IN ('inventory.receive.request', 'installment.request')
WHERE r.role_key = 'branch_pos'
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS inventory_receipt_requests (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    requested_quantity INTEGER NOT NULL CHECK (requested_quantity > 0),
    snapshot_qty_real INTEGER NOT NULL,
    snapshot_qty_ghost INTEGER NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    approved_quantity INTEGER CHECK (approved_quantity > 0),
    approved_stock_bucket TEXT CHECK (approved_stock_bucket IN ('real', 'ghost')),
    review_note TEXT NOT NULL DEFAULT '',
    requested_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_receipt_requests_pending_product
    ON inventory_receipt_requests (branch_id, product_id)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_inventory_receipt_requests_status_created
    ON inventory_receipt_requests (status, created_at DESC);

CREATE TABLE IF NOT EXISTS installment_requests (
    id UUID PRIMARY KEY,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    months INTEGER NOT NULL CHECK (months BETWEEN 1 AND 60),
    first_due_date DATE NOT NULL,
    requested_total NUMERIC(12,2) NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    review_note TEXT NOT NULL DEFAULT '',
    approved_plan_id UUID REFERENCES installment_plans(id) ON DELETE SET NULL,
    requested_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_installment_requests_pending_invoice
    ON installment_requests (invoice_id)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_installment_requests_status_created
    ON installment_requests (status, created_at DESC);
