CREATE TABLE v2_discount_approvals (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    branch_id UUID NOT NULL REFERENCES branches(id),
    cart_hash TEXT NOT NULL,
    reason TEXT NOT NULL,
    approved_by UUID NOT NULL REFERENCES users(id),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_discount_approval_uses (
    approval_id UUID PRIMARY KEY REFERENCES v2_discount_approvals(id),
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    used_by UUID NOT NULL REFERENCES users(id),
    used_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TRIGGER v2_discount_approvals_immutable BEFORE UPDATE OR DELETE ON v2_discount_approvals FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_discount_approval_uses_immutable BEFORE UPDATE OR DELETE ON v2_discount_approval_uses FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
