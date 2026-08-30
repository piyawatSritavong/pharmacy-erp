ALTER TABLE month_end_workpapers
    DROP CONSTRAINT IF EXISTS month_end_workpapers_status_check;

ALTER TABLE month_end_workpapers
    ADD COLUMN IF NOT EXISTS cancelled_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancel_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE month_end_workpapers
    ADD CONSTRAINT month_end_workpapers_status_check
        CHECK (status IN ('OPEN', 'DRAFT', 'CALCULATING', 'PENDING_APPROVAL', 'APPROVED', 'CLOSED', 'REOPENED', 'FAILED', 'CANCELLED'));

CREATE INDEX IF NOT EXISTS idx_month_end_workpapers_cancelled
    ON month_end_workpapers (cancelled_at DESC)
    WHERE status = 'CANCELLED';
