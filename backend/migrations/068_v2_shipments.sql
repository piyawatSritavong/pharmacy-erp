CREATE TABLE v2_shipments (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    source_branch_id UUID NOT NULL REFERENCES branches(id),
    destination_branch_id UUID NOT NULL REFERENCES branches(id),
    reference TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('dispatched','partial','received','cancelled')),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK(source_branch_id<>destination_branch_id)
);
CREATE TABLE v2_shipment_lines (
    id UUID PRIMARY KEY,
    shipment_id UUID NOT NULL REFERENCES v2_shipments(id),
    source_lot_id UUID NOT NULL REFERENCES v2_lots(id),
    product_id UUID NOT NULL REFERENCES products(id),
    base_quantity BIGINT NOT NULL CHECK(base_quantity>0),
    value_cents BIGINT NOT NULL CHECK(value_cents>=0),
    unit_snapshot JSONB NOT NULL
);
CREATE TABLE v2_shipment_receipts (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    shipment_line_id UUID NOT NULL REFERENCES v2_shipment_lines(id),
    received_lot_id UUID NOT NULL REFERENCES v2_lots(id),
    base_quantity BIGINT NOT NULL CHECK(base_quantity>0),
    value_cents BIGINT NOT NULL CHECK(value_cents>=0),
    receipt_kind TEXT NOT NULL CHECK(receipt_kind IN ('destination','return_to_source')),
    reason TEXT NOT NULL,
    received_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(operation_id,shipment_line_id)
);
CREATE TRIGGER v2_shipment_lines_immutable BEFORE UPDATE OR DELETE ON v2_shipment_lines FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_shipment_receipts_immutable BEFORE UPDATE OR DELETE ON v2_shipment_receipts FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
