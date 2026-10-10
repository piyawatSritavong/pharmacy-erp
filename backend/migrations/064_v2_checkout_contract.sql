-- V2-P0 cart/checkout compatibility fields.
-- Existing parked bills default to a zero bill discount. Existing completed
-- remote sessions remain readable but do not have a replayable result because
-- they predate the idempotency contract.

ALTER TABLE parked_bills
    ADD COLUMN IF NOT EXISTS bill_discount_amount NUMERIC(14,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'open',
    ADD COLUMN IF NOT EXISTS claim_token UUID,
    ADD COLUMN IF NOT EXISTS claimed_by UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS consumed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS consumed_invoice_id UUID REFERENCES invoices(id);

ALTER TABLE parked_bills
    DROP CONSTRAINT IF EXISTS parked_bills_status_chk;

ALTER TABLE parked_bills
    ADD CONSTRAINT parked_bills_status_chk
    CHECK (status IN ('open','claimed','consumed','abandoned'));

CREATE INDEX IF NOT EXISTS parked_bills_branch_status_idx
    ON parked_bills (branch_id, status, expires_at DESC);

ALTER TABLE remote_sale_sessions
    ADD COLUMN IF NOT EXISTS checkout_request_hash TEXT,
    ADD COLUMN IF NOT EXISTS checkout_result JSONB,
    ADD COLUMN IF NOT EXISTS cart_version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE remote_sale_sessions
    DROP CONSTRAINT IF EXISTS remote_sale_sessions_cart_version_positive_chk;

ALTER TABLE remote_sale_sessions
    ADD CONSTRAINT remote_sale_sessions_cart_version_positive_chk CHECK (cart_version > 0);

ALTER TABLE remote_sale_sessions
    DROP CONSTRAINT IF EXISTS remote_sale_sessions_checkout_pair_chk;

ALTER TABLE remote_sale_sessions
    ADD CONSTRAINT remote_sale_sessions_checkout_pair_chk CHECK (
        (checkout_request_hash IS NULL AND checkout_result IS NULL)
        OR
        (checkout_request_hash IS NOT NULL AND checkout_result IS NOT NULL)
    );
