ALTER TABLE branches
    ADD COLUMN IF NOT EXISTS branch_type TEXT NOT NULL DEFAULT 'branch'
        CHECK (branch_type IN ('main_warehouse', 'branch')),
    ADD COLUMN IF NOT EXISTS parent_branch_id UUID REFERENCES branches(id) ON DELETE SET NULL;

ALTER TABLE branches
    ADD CONSTRAINT branches_parent_not_self CHECK (parent_branch_id IS NULL OR parent_branch_id <> id);

CREATE INDEX IF NOT EXISTS idx_branches_type_parent
    ON branches (branch_type, parent_branch_id, active);

CREATE TABLE IF NOT EXISTS other_income_entries (
    id UUID PRIMARY KEY,
    entry_number TEXT NOT NULL UNIQUE,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    income_date DATE NOT NULL,
    category TEXT NOT NULL,
    description TEXT NOT NULL,
    payment_method TEXT NOT NULL CHECK (payment_method IN ('cash', 'bank_transfer', 'check', 'other')),
    amount NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    reference_code TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'posted' CHECK (status IN ('posted', 'void')),
    void_reason TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    voided_by UUID REFERENCES users(id) ON DELETE SET NULL,
    voided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_other_income_branch_date
    ON other_income_entries (branch_id, income_date DESC, status);
CREATE INDEX IF NOT EXISTS idx_other_income_status_date
    ON other_income_entries (status, income_date DESC);

CREATE TABLE IF NOT EXISTS accounting_workpapers (
    id UUID PRIMARY KEY,
    workpaper_number TEXT NOT NULL UNIQUE,
    branch_id UUID REFERENCES branches(id) ON DELETE SET NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    invoice_count INTEGER NOT NULL DEFAULT 0,
    other_income_count INTEGER NOT NULL DEFAULT 0,
    system_invoice_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    system_other_income_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    system_cash_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    system_bank_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    system_check_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    system_other_payment_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    income_adjustment NUMERIC(12,2) NOT NULL DEFAULT 0,
    cash_adjustment NUMERIC(12,2) NOT NULL DEFAULT 0,
    bank_adjustment NUMERIC(12,2) NOT NULL DEFAULT 0,
    check_adjustment NUMERIC(12,2) NOT NULL DEFAULT 0,
    other_payment_adjustment NUMERIC(12,2) NOT NULL DEFAULT 0,
    adjusted_income_amount NUMERIC(12,2) NOT NULL CHECK (adjusted_income_amount >= 0),
    adjusted_cash_amount NUMERIC(12,2) NOT NULL CHECK (adjusted_cash_amount >= 0),
    adjusted_bank_amount NUMERIC(12,2) NOT NULL CHECK (adjusted_bank_amount >= 0),
    adjusted_check_amount NUMERIC(12,2) NOT NULL CHECK (adjusted_check_amount >= 0),
    adjusted_other_payment_amount NUMERIC(12,2) NOT NULL CHECK (adjusted_other_payment_amount >= 0),
    invoice_allocation_percent NUMERIC(5,2) NOT NULL DEFAULT 80 CHECK (invoice_allocation_percent BETWEEN 0 AND 100),
    other_allocation_percent NUMERIC(5,2) NOT NULL DEFAULT 20 CHECK (other_allocation_percent BETWEEN 0 AND 100),
    invoice_allocation_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    other_allocation_amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    adjustment_reason TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'finalized', 'void')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    finalized_by UUID REFERENCES users(id) ON DELETE SET NULL,
    voided_by UUID REFERENCES users(id) ON DELETE SET NULL,
    finalized_at TIMESTAMPTZ,
    voided_at TIMESTAMPTZ,
    void_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (period_end >= period_start),
    CHECK (ROUND(invoice_allocation_percent + other_allocation_percent, 2) = 100)
);

CREATE INDEX IF NOT EXISTS idx_accounting_workpapers_period
    ON accounting_workpapers (period_start DESC, period_end DESC, status);
CREATE INDEX IF NOT EXISTS idx_accounting_workpapers_branch_period
    ON accounting_workpapers (branch_id, period_start DESC, period_end DESC);

INSERT INTO app_settings (setting_key, setting_value, metadata, created_at, updated_at)
VALUES
    ('accounting_invoice_allocation_percent', '80', '{"label":"สัดส่วนออกเอกสาร","type":"number"}'::jsonb, NOW(), NOW()),
    ('accounting_other_allocation_percent', '20', '{"label":"สัดส่วนอื่น","type":"number"}'::jsonb, NOW(), NOW())
ON CONFLICT (setting_key) DO NOTHING;
