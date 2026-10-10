CREATE TABLE v2_quotation_revisions (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    previous_id UUID NOT NULL UNIQUE REFERENCES v2_documents(id),
    next_id UUID NOT NULL UNIQUE REFERENCES v2_documents(id),
    reason TEXT NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TRIGGER v2_quotation_revisions_immutable BEFORE UPDATE OR DELETE ON v2_quotation_revisions FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
