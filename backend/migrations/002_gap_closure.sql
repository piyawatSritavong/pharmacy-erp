ALTER TABLE roles
    ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS is_system BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_invoices_created_by_issued_at ON invoices (created_by, issued_at DESC);
CREATE INDEX IF NOT EXISTS idx_invoice_payments_created_by_created_at ON invoice_payments (created_by, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_branch_created_at ON audit_logs (branch_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_created_at ON audit_logs (action, created_at DESC);
