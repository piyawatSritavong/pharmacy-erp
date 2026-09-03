-- รีโมตหน้าร้าน: head office builds a cart in a branch's name and the branch's
-- own POS collects the money. The cart lives here between the two screens, so
-- it survives a refresh on either side and leaves a record of who opened it.
--
-- The cart is stored, not the operator's clicks: mirroring pointer positions
-- would break on a different screen size, a different scroll offset, or a
-- product moving in the grid, and would leave nothing to audit.
CREATE TABLE IF NOT EXISTS remote_sale_sessions (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    operator_id UUID NOT NULL REFERENCES users(id),
    -- open: waiting at the branch till · completed: paid · cancelled: withdrawn
    status TEXT NOT NULL DEFAULT 'open',
    cart JSONB NOT NULL DEFAULT '{}'::jsonb,
    invoice_id UUID NULL REFERENCES invoices(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A branch has at most one cart waiting at its till at a time; head office
-- keeps editing that one rather than stacking up sessions the cashier must
-- choose between.
CREATE UNIQUE INDEX IF NOT EXISTS remote_sale_sessions_open_per_branch
    ON remote_sale_sessions (branch_id)
    WHERE status = 'open';

CREATE INDEX IF NOT EXISTS remote_sale_sessions_branch_status_idx
    ON remote_sale_sessions (branch_id, status, updated_at DESC);
