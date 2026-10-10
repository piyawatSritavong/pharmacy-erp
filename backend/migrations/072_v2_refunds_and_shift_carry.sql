-- Additive V2 cancellation/refund and account shift evidence. V1 is untouched.
CREATE TABLE v2_cancellations (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL UNIQUE REFERENCES v2_operations(id),
    branch_id UUID NOT NULL REFERENCES branches(id),
    document_id UUID NOT NULL UNIQUE REFERENCES v2_documents(id),
    credit_document_id UUID REFERENCES v2_documents(id),
    refund_due_cents BIGINT NOT NULL CHECK(refund_due_cents>=0),
    stock_returned BOOLEAN NOT NULL,
    reference TEXT NOT NULL CHECK(LENGTH(BTRIM(reference))>0),
    reason TEXT NOT NULL CHECK(LENGTH(BTRIM(reason))>0),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_refund_settlements (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL UNIQUE REFERENCES v2_operations(id),
    cancellation_id UUID NOT NULL REFERENCES v2_cancellations(id),
    credit_use_id UUID UNIQUE REFERENCES v2_credit_uses(id),
    method TEXT NOT NULL CHECK(method IN ('cash','bank_transfer','goods')),
    amount_cents BIGINT NOT NULL CHECK(amount_cents>0),
    reference TEXT NOT NULL CHECK(LENGTH(BTRIM(reference))>0),
    reason TEXT NOT NULL CHECK(LENGTH(BTRIM(reason))>0),
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_refund_goods (
    id UUID PRIMARY KEY,
    settlement_id UUID NOT NULL REFERENCES v2_refund_settlements(id),
    product_id UUID NOT NULL REFERENCES products(id),
    unit_snapshot JSONB NOT NULL,
    unit_price_cents BIGINT NOT NULL CHECK(unit_price_cents>=0),
    vat_bps INTEGER NOT NULL CHECK(vat_bps BETWEEN 0 AND 10000),
    total_cents BIGINT NOT NULL CHECK(total_cents>=0)
);
CREATE TABLE v2_shift_issues (
    id UUID PRIMARY KEY,
    drawer_id UUID NOT NULL UNIQUE REFERENCES v2_drawers(id),
    branch_id UUID NOT NULL REFERENCES branches(id),
    account_id UUID NOT NULL REFERENCES users(id),
    amount_cents BIGINT NOT NULL CHECK(amount_cents<>0),
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_shift_issue_resolutions (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    issue_id UUID NOT NULL UNIQUE REFERENCES v2_shift_issues(id),
    reason TEXT NOT NULL CHECK(LENGTH(BTRIM(reason))>0),
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_shift_acknowledgements (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    drawer_id UUID NOT NULL REFERENCES v2_drawers(id),
    warnings JSONB NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- One open shift per account, independent of terminal and selected branch.
CREATE UNIQUE INDEX v2_drawer_open_account ON v2_drawers(opened_by) WHERE closed_at IS NULL;
CREATE TRIGGER v2_cancellations_immutable BEFORE UPDATE OR DELETE ON v2_cancellations FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_refund_settlements_immutable BEFORE UPDATE OR DELETE ON v2_refund_settlements FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_refund_goods_immutable BEFORE UPDATE OR DELETE ON v2_refund_goods FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_shift_issues_immutable BEFORE UPDATE OR DELETE ON v2_shift_issues FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_shift_issue_resolutions_immutable BEFORE UPDATE OR DELETE ON v2_shift_issue_resolutions FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_shift_acknowledgements_immutable BEFORE UPDATE OR DELETE ON v2_shift_acknowledgements FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
