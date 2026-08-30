ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS tax_invoice_type TEXT NOT NULL DEFAULT 'abbreviated'
        CHECK (tax_invoice_type IN ('abbreviated', 'full'));

UPDATE invoices
SET tax_invoice_type = 'full'
WHERE NULLIF(TRIM(COALESCE(customer_tax_id, '')), '') IS NOT NULL;

CREATE TABLE IF NOT EXISTS month_end_workpapers (
    id UUID PRIMARY KEY,
    workpaper_number TEXT NOT NULL UNIQUE,
    branch_id UUID REFERENCES branches(id) ON DELETE SET NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    target_revenue NUMERIC(14,2) NOT NULL CHECK (target_revenue >= 0),
    markup_percent NUMERIC(7,2) NOT NULL DEFAULT 5 CHECK (markup_percent BETWEEN 0 AND 1000),
    actual_revenue NUMERIC(14,2) NOT NULL CHECK (actual_revenue >= 0),
    actual_invoice_count INTEGER NOT NULL DEFAULT 0,
    full_tax_revenue NUMERIC(14,2) NOT NULL DEFAULT 0,
    full_tax_invoice_count INTEGER NOT NULL DEFAULT 0,
    cash_revenue NUMERIC(14,2) NOT NULL DEFAULT 0,
    cash_invoice_count INTEGER NOT NULL DEFAULT 0,
    requested_reduction NUMERIC(14,2) NOT NULL DEFAULT 0,
    ghost_reclassification_amount NUMERIC(14,2) NOT NULL DEFAULT 0,
    price_scenario_reduction_amount NUMERIC(14,2) NOT NULL DEFAULT 0,
    scenario_revenue NUMERIC(14,2) NOT NULL CHECK (scenario_revenue >= 0),
    unresolved_difference NUMERIC(14,2) NOT NULL DEFAULT 0,
    source_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'calculated' CHECK (status IN ('calculated', 'finalized')),
    disclaimer TEXT NOT NULL DEFAULT 'กระดาษทำการภายใน ไม่ใช่รายงานภาษี และไม่เปลี่ยนแปลงธุรกรรมต้นฉบับ',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    finalized_by UUID REFERENCES users(id) ON DELETE SET NULL,
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finalized_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (period_end >= period_start),
    CHECK (target_revenue <= actual_revenue)
);

CREATE TABLE IF NOT EXISTS month_end_workpaper_lines (
    id UUID PRIMARY KEY,
    workpaper_id UUID NOT NULL REFERENCES month_end_workpapers(id) ON DELETE CASCADE,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,
    invoice_item_id UUID NOT NULL REFERENCES invoice_items(id) ON DELETE RESTRICT,
    management_sequence INTEGER NOT NULL CHECK (management_sequence > 0),
    invoice_number_snapshot TEXT NOT NULL,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    branch_name_snapshot TEXT NOT NULL,
    issued_at_snapshot TIMESTAMPTZ NOT NULL,
    payment_type_snapshot TEXT NOT NULL,
    tax_invoice_type_snapshot TEXT NOT NULL CHECK (tax_invoice_type_snapshot IN ('abbreviated', 'full')),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    sku_snapshot TEXT NOT NULL,
    product_name_snapshot TEXT NOT NULL,
    original_stock_bucket TEXT NOT NULL CHECK (original_stock_bucket IN ('real', 'ghost')),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    allocated_ghost_quantity INTEGER NOT NULL DEFAULT 0 CHECK (allocated_ghost_quantity >= 0 AND allocated_ghost_quantity <= quantity),
    repriced_quantity INTEGER NOT NULL DEFAULT 0 CHECK (repriced_quantity >= 0 AND repriced_quantity <= quantity),
    original_unit_price NUMERIC(14,2) NOT NULL,
    cost_snapshot NUMERIC(14,2) NOT NULL,
    proposed_unit_price NUMERIC(14,2) NOT NULL,
    tax_rate NUMERIC(7,2) NOT NULL,
    original_line_total NUMERIC(14,2) NOT NULL,
    scenario_line_total NUMERIC(14,2) NOT NULL,
    adjustment_type TEXT NOT NULL CHECK (adjustment_type IN ('included', 'locked_full_tax', 'ghost_reclassification', 'price_scenario', 'ghost_and_price')),
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (allocated_ghost_quantity + repriced_quantity <= quantity),
    UNIQUE (workpaper_id, invoice_item_id)
);

CREATE INDEX IF NOT EXISTS idx_month_end_workpapers_period
    ON month_end_workpapers (period_start DESC, status, branch_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_month_end_workpapers_finalized_period
    ON month_end_workpapers (COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'::uuid), period_start)
    WHERE status = 'finalized';
CREATE INDEX IF NOT EXISTS idx_month_end_workpaper_lines_workpaper
    ON month_end_workpaper_lines (workpaper_id, management_sequence, issued_at_snapshot);

INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES (
    gen_random_uuid(),
    'month_end.manage',
    'จัดการสรุปสิ้นเดือน',
    'คำนวณ ตรวจสอบ และยืนยันกระดาษทำการปิดเดือนโดยไม่แก้ธุรกรรมต้นฉบับ',
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
INNER JOIN permissions p ON p.permission_key = 'month_end.manage'
WHERE r.role_key = 'super_admin'
ON CONFLICT DO NOTHING;
