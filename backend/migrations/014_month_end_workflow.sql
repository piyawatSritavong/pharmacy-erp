DROP INDEX IF EXISTS idx_month_end_workpapers_finalized_period;

ALTER TABLE month_end_workpapers DROP CONSTRAINT IF EXISTS month_end_workpapers_status_check;
UPDATE month_end_workpapers SET status = 'DRAFT' WHERE status = 'calculated';
UPDATE month_end_workpapers SET status = 'CLOSED' WHERE status = 'finalized';

ALTER TABLE month_end_workpapers
    ADD COLUMN IF NOT EXISTS current_step INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS revision INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS parent_period_id UUID REFERENCES month_end_workpapers(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS simulation_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS support_document_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS calculation_strategy TEXT NOT NULL DEFAULT 'closest_then_oldest',
    ADD COLUMN IF NOT EXISTS calculation_version TEXT NOT NULL DEFAULT '2.0.0',
    ADD COLUMN IF NOT EXISTS approved_accounting_revenue NUMERIC(14,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gp_before NUMERIC(14,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gp_after NUMERIC(14,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS validation_summary JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS result_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS approved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS closed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS closed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reopened_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS reopened_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reopen_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS checksum TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lock_version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE month_end_workpapers
    ADD CONSTRAINT month_end_workpapers_status_check
        CHECK (status IN ('OPEN', 'DRAFT', 'CALCULATING', 'PENDING_APPROVAL', 'APPROVED', 'CLOSED', 'REOPENED', 'FAILED')),
    ADD CONSTRAINT month_end_workpapers_current_step_check CHECK (current_step BETWEEN 1 AND 6),
    ADD CONSTRAINT month_end_workpapers_revision_check CHECK (revision > 0),
    ADD CONSTRAINT month_end_workpapers_lock_version_check CHECK (lock_version > 0);

UPDATE month_end_workpapers
SET current_step = CASE WHEN status = 'CLOSED' THEN 6 ELSE 3 END,
    approved_accounting_revenue = scenario_revenue,
    approved_at = CASE WHEN status = 'CLOSED' THEN finalized_at ELSE NULL END,
    approved_by = CASE WHEN status = 'CLOSED' THEN finalized_by ELSE NULL END,
    closed_at = CASE WHEN status = 'CLOSED' THEN finalized_at ELSE NULL END,
    closed_by = CASE WHEN status = 'CLOSED' THEN finalized_by ELSE NULL END;

ALTER TABLE month_end_workpaper_lines
    ADD COLUMN IF NOT EXISTS included BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS supporting_document_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS before_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS after_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS rejected_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE TABLE IF NOT EXISTS month_end_calculation_runs (
    id UUID PRIMARY KEY,
    workpaper_id UUID NOT NULL REFERENCES month_end_workpapers(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    calculation_version TEXT NOT NULL,
    strategy TEXT NOT NULL,
    input_data JSONB NOT NULL,
    result_data JSONB NOT NULL,
    source_hash TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('completed', 'failed')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workpaper_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS month_end_adjustments (
    id UUID PRIMARY KEY,
    adjustment_number TEXT NOT NULL UNIQUE,
    workpaper_id UUID NOT NULL REFERENCES month_end_workpapers(id) ON DELETE RESTRICT,
    calculation_run_id UUID REFERENCES month_end_calculation_runs(id) ON DELETE SET NULL,
    invoice_id UUID REFERENCES invoices(id) ON DELETE RESTRICT,
    invoice_item_id UUID REFERENCES invoice_items(id) ON DELETE RESTRICT,
    adjustment_type TEXT NOT NULL CHECK (adjustment_type IN ('stock_reclassification', 'price_simulation')),
    before_data JSONB NOT NULL,
    after_data JSONB NOT NULL,
    difference_amount NUMERIC(14,2) NOT NULL DEFAULT 0,
    reason TEXT NOT NULL,
    remark TEXT NOT NULL DEFAULT '',
    supporting_document_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('proposed', 'approved', 'reversed', 'rejected')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at TIMESTAMPTZ,
    reversed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS inventory_reclassifications (
    id UUID PRIMARY KEY,
    workpaper_id UUID NOT NULL REFERENCES month_end_workpapers(id) ON DELETE RESTRICT,
    adjustment_id UUID NOT NULL REFERENCES month_end_adjustments(id) ON DELETE RESTRICT,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    from_bucket TEXT NOT NULL CHECK (from_bucket IN ('real', 'ghost')),
    to_bucket TEXT NOT NULL CHECK (to_bucket IN ('real', 'ghost')),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    status TEXT NOT NULL CHECK (status IN ('approved', 'reversed')),
    movement_out_id UUID REFERENCES inventory_movements(id) ON DELETE RESTRICT,
    movement_in_id UUID REFERENCES inventory_movements(id) ON DELETE RESTRICT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reversed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reversed_at TIMESTAMPTZ,
    UNIQUE (adjustment_id)
);

CREATE INDEX IF NOT EXISTS idx_month_end_runs_workpaper
    ON month_end_calculation_runs (workpaper_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_month_end_adjustments_workpaper
    ON month_end_adjustments (workpaper_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_inventory_reclassifications_workpaper
    ON inventory_reclassifications (workpaper_id, status);
CREATE INDEX IF NOT EXISTS idx_month_end_lines_filter
    ON month_end_workpaper_lines (workpaper_id, included, payment_type_snapshot, tax_invoice_type_snapshot);
CREATE UNIQUE INDEX IF NOT EXISTS idx_month_end_workpapers_closed_period
    ON month_end_workpapers (COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'::uuid), period_start)
    WHERE status = 'CLOSED';

INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
SELECT gen_random_uuid(), item.permission_key, item.name, item.description, NOW(), NOW()
FROM (VALUES
    ('month_end.view', 'ดูสรุปสิ้นเดือน', 'ดูรอบบัญชีและผลคำนวณสิ้นเดือน'),
    ('month_end.create', 'สร้างรอบสิ้นเดือน', 'สร้างและบันทึกร่างรอบบัญชี'),
    ('month_end.calculate', 'คำนวณสิ้นเดือน', 'ตรวจสอบและคำนวณข้อเสนอปรับปรุง'),
    ('month_end.adjust', 'ปรับปรุงสิ้นเดือน', 'เลือกและแก้ไขข้อเสนอปรับปรุง'),
    ('month_end.approve', 'อนุมัติสิ้นเดือน', 'อนุมัติรายการปรับปรุงรอบบัญชี'),
    ('month_end.close', 'ปิดรอบสิ้นเดือน', 'ยืนยันและล็อกรอบบัญชี'),
    ('month_end.reopen', 'เปิดรอบสิ้นเดือนใหม่', 'ย้อนรายการและสร้าง revision ใหม่'),
    ('month_end.export', 'ส่งออกสรุปสิ้นเดือน', 'ส่งออกข้อมูลจำลองและประวัติการคำนวณ')
) AS item(permission_key, name, description)
ON CONFLICT (permission_key) DO UPDATE
SET name = EXCLUDED.name, description = EXCLUDED.description, updated_at = NOW();

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'super_admin' AND p.permission_key LIKE 'month_end.%'
ON CONFLICT DO NOTHING;
