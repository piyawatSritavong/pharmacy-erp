CREATE TABLE v2_promotions (
    id UUID PRIMARY KEY,
    branch_id UUID REFERENCES branches(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    promo_type TEXT NOT NULL CHECK(promo_type IN ('percent','amount','buy_x_get_y','bundle','bill_giveaway')),
    rule JSONB NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    revision INTEGER NOT NULL DEFAULT 1,
    CHECK(ends_on IS NULL OR ends_on>=starts_on)
);
CREATE UNIQUE INDEX v2_branch_promotion_code ON v2_promotions(branch_id,code) WHERE branch_id IS NOT NULL;
CREATE UNIQUE INDEX v2_central_promotion_code ON v2_promotions(code) WHERE branch_id IS NULL;
CREATE TABLE v2_promotion_versions (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    promotion_id UUID NOT NULL REFERENCES v2_promotions(id),
    snapshot JSONB NOT NULL,
    reason TEXT NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_document_promotions (
    id UUID PRIMARY KEY,
    document_id UUID NOT NULL UNIQUE REFERENCES v2_documents(id),
    promotion_id UUID NOT NULL REFERENCES v2_promotions(id),
    snapshot JSONB NOT NULL
);
CREATE TRIGGER v2_promotion_versions_immutable BEFORE UPDATE OR DELETE ON v2_promotion_versions FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_document_promotions_immutable BEFORE UPDATE OR DELETE ON v2_document_promotions FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
