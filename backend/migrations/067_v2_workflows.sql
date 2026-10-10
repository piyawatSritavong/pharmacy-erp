-- V2-only workflow projections and append-only evidence. No V1 changes.
ALTER TABLE v2_stock_events ADD COLUMN sequence_no BIGINT GENERATED ALWAYS AS IDENTITY;
CREATE TABLE v2_stock_counts (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id),
    reference TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','counted','confirmed','cancelled')),
    revision INTEGER NOT NULL DEFAULT 1,
    reason TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL REFERENCES users(id),
    confirmed_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ
);
CREATE TABLE v2_count_snapshots (
    id UUID PRIMARY KEY,
    count_id UUID NOT NULL REFERENCES v2_stock_counts(id),
    lot_id UUID NOT NULL REFERENCES v2_lots(id),
    product_id UUID NOT NULL REFERENCES products(id),
    quantity BIGINT NOT NULL CHECK(quantity>=0),
    reserved_quantity BIGINT NOT NULL CHECK(reserved_quantity>=0),
    lot_state TEXT NOT NULL,
    expires_on DATE,
    UNIQUE(count_id,lot_id)
);
CREATE TABLE v2_count_observations (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    count_id UUID NOT NULL REFERENCES v2_stock_counts(id),
    snapshot_id UUID NOT NULL REFERENCES v2_count_snapshots(id),
    expected_quantity BIGINT NOT NULL CHECK(expected_quantity>=0),
    checkpoint BIGINT NOT NULL,
    counted_quantity BIGINT NOT NULL CHECK(counted_quantity>=0),
    increase_cost_cents BIGINT NOT NULL CHECK(increase_cost_cents>=0),
    reason TEXT NOT NULL,
    observed_by UUID NOT NULL REFERENCES users(id),
    observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(count_id,snapshot_id)
);
CREATE TABLE v2_stock_returns (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    branch_id UUID NOT NULL REFERENCES branches(id),
    source_event_id UUID NOT NULL REFERENCES v2_stock_events(id),
    returned_lot_id UUID NOT NULL REFERENCES v2_lots(id),
    base_quantity BIGINT NOT NULL CHECK(base_quantity>0),
    value_cents BIGINT NOT NULL CHECK(value_cents>=0),
    reason TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(operation_id,source_event_id)
);
CREATE TABLE v2_credit_uses (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    source_document_id UUID NOT NULL REFERENCES v2_documents(id),
    target_document_id UUID REFERENCES v2_documents(id),
    kind TEXT NOT NULL CHECK(kind IN ('apply','refund')),
    amount_cents BIGINT NOT NULL CHECK(amount_cents>0),
    method TEXT CHECK(method IN ('cash','bank_transfer')),
    reference TEXT NOT NULL,
    reason TEXT NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK((kind='apply' AND target_document_id IS NOT NULL AND method IS NULL) OR
          (kind='refund' AND target_document_id IS NULL AND method IS NOT NULL))
);
CREATE TABLE v2_notifications (
    id UUID PRIMARY KEY,
    outbox_id UUID NOT NULL UNIQUE REFERENCES v2_outbox(id),
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE v2_drawer_events ADD COLUMN credit_use_id UUID REFERENCES v2_credit_uses(id);
ALTER TABLE v2_outbox ADD COLUMN lease_token UUID;
CREATE INDEX v2_outbox_ready ON v2_outbox(available_at,created_at) WHERE status IN ('pending','processing');
CREATE INDEX v2_stock_events_lot ON v2_stock_events(lot_id,created_at);
CREATE INDEX v2_money_events_document ON v2_money_events(document_id,effective_on);
CREATE INDEX v2_returns_source ON v2_stock_returns(source_event_id);
CREATE TRIGGER v2_count_snapshot_immutable BEFORE UPDATE OR DELETE ON v2_count_snapshots FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_count_observation_immutable BEFORE UPDATE OR DELETE ON v2_count_observations FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_stock_returns_immutable BEFORE UPDATE OR DELETE ON v2_stock_returns FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_credit_uses_immutable BEFORE UPDATE OR DELETE ON v2_credit_uses FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
