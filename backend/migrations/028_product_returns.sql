-- Part B, Rule 4 — POS return → back-office claim → supplier replacement.
-- status is the current state; return_events is its timeline (same pattern
-- transfers/transfer_events already established for a multi-step workflow).
CREATE TABLE IF NOT EXISTS product_returns (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id),
    original_invoice_item_id UUID NOT NULL REFERENCES invoice_items(id),
    product_id UUID NOT NULL REFERENCES products(id),
    stock_bucket TEXT NOT NULL CHECK (stock_bucket IN ('real', 'ghost')),
    quantity INT NOT NULL CHECK (quantity > 0),
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending_claim'
        CHECK (status IN ('pending_claim', 'sent_to_supplier', 'resolved_case_a', 'resolved_case_b', 'rejected')),
    supplier_id UUID REFERENCES suppliers(id),
    -- Case A: same model back from the supplier, direct 1-for-1 restock.
    -- Case B: model discontinued — original stays written off (it already
    -- left inventory when the POS replacement was issued), and
    -- replacement_product_id is a *different* model received in separately.
    resolution_case TEXT CHECK (resolution_case IN ('a', 'b')),
    replacement_product_id UUID REFERENCES products(id),
    -- Captured at Case B resolution time (the replacement product's own
    -- cost_price) so there's a real cost fact on record even though the
    -- accounting *treatment* — does this flow through month-end, is it
    -- always catalog cost vs a negotiated one-off — is still an open
    -- question per the Part B proposal.
    replacement_cost_snapshot NUMERIC(12, 2),
    claim_sent_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_product_returns_status ON product_returns(status);
CREATE INDEX IF NOT EXISTS idx_product_returns_branch ON product_returns(branch_id);

CREATE TABLE IF NOT EXISTS return_events (
    id UUID PRIMARY KEY,
    return_id UUID NOT NULL REFERENCES product_returns(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    actor_id UUID REFERENCES users(id),
    event_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES (gen_random_uuid(), 'returns.manage', 'จัดการเคลม/คืนสินค้า', 'ตรวจสอบคำขอคืนสินค้า ส่งเคลมให้คู่ค้า และปิดเคลม', NOW(), NOW())
ON CONFLICT (permission_key) DO NOTHING;

-- super_admin, admin (general back-office ops), and office (already owns
-- the supplier relationship via suppliers.manage.global) can review claims.
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key IN ('super_admin', 'admin', 'office') AND p.permission_key = 'returns.manage'
ON CONFLICT DO NOTHING;
