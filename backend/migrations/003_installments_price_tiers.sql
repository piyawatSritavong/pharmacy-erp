-- Feature port from DockBill: installment billing, 3-tier pricing, stock receiving.

-- 3-tier pricing: base_selling_price remains the cash price;
-- 0 means the tier is not set and sales fall back to the branch/base price.
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS retail_price NUMERIC(12,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS installment_price NUMERIC(12,2) NOT NULL DEFAULT 0;

-- Allow invoices to carry an active installment plan.
ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_payment_status_check;
ALTER TABLE invoices
    ADD CONSTRAINT invoices_payment_status_check
    CHECK (payment_status IN ('unpaid', 'paid', 'installment'));

CREATE TABLE IF NOT EXISTS installment_plans (
    id UUID PRIMARY KEY,
    invoice_id UUID NOT NULL UNIQUE REFERENCES invoices(id) ON DELETE CASCADE,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    months INTEGER NOT NULL CHECK (months > 0),
    monthly_amount NUMERIC(12,2) NOT NULL,
    total_amount NUMERIC(12,2) NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'cancelled')),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS installment_payments (
    id UUID PRIMARY KEY,
    plan_id UUID NOT NULL REFERENCES installment_plans(id) ON DELETE CASCADE,
    seq_number INTEGER NOT NULL CHECK (seq_number > 0),
    due_date DATE NOT NULL,
    amount NUMERIC(12,2) NOT NULL,
    paid_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    paid_at TIMESTAMPTZ,
    status TEXT NOT NULL CHECK (status IN ('pending', 'paid')),
    received_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (plan_id, seq_number)
);

CREATE INDEX IF NOT EXISTS idx_installment_plans_branch_status ON installment_plans (branch_id, status);
CREATE INDEX IF NOT EXISTS idx_installment_payments_status_due ON installment_payments (status, due_date);

-- New permissions. Seed is skipped on databases that already have users,
-- so existing deployments pick these up here; fresh databases get the role
-- mappings from seed (inserts below are idempotent both ways).
INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES
    (gen_random_uuid(), 'installment.view', 'View Installments', 'View installment plans and payments', NOW(), NOW()),
    (gen_random_uuid(), 'installment.manage', 'Manage Installments', 'Create installment plans for invoices', NOW(), NOW()),
    (gen_random_uuid(), 'installment.collect', 'Collect Installment', 'Record installment payments', NOW(), NOW()),
    (gen_random_uuid(), 'inventory.receive', 'Receive Inventory', 'Receive incoming stock into real and ghost buckets', NOW(), NOW())
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
JOIN permissions p ON p.permission_key IN ('installment.view', 'installment.manage', 'installment.collect', 'inventory.receive')
WHERE r.role_key IN ('super_admin', 'branch_admin')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
JOIN permissions p ON p.permission_key IN ('installment.view', 'installment.collect')
WHERE r.role_key = 'branch_pos'
ON CONFLICT DO NOTHING;
