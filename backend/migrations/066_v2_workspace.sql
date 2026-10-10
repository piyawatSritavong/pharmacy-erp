-- Additive V2 pilot workspace. No V1 table, trigger, or month-end rule changes.
CREATE TABLE v2_branch_policies (
    branch_id UUID PRIMARY KEY REFERENCES branches(id) ON DELETE RESTRICT,
    cost_method TEXT NOT NULL CHECK (cost_method IN ('fifo','moving_average')),
    allow_branch_promotions BOOLEAN NOT NULL DEFAULT FALSE,
    max_discount_bps INTEGER NOT NULL DEFAULT 0 CHECK (max_discount_bps BETWEEN 0 AND 10000),
    updated_by UUID NOT NULL REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_operations (
    id UUID PRIMARY KEY,
    actor_id UUID NOT NULL REFERENCES users(id),
    action TEXT NOT NULL,
    request_key TEXT NOT NULL CHECK (LENGTH(request_key) BETWEEN 8 AND 128),
    request_hash TEXT NOT NULL,
    result JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(actor_id,action,request_key)
);
CREATE TABLE v2_customers (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id),
    customer_code TEXT NOT NULL,
    customer_type TEXT NOT NULL CHECK (customer_type IN ('person','business')),
    name TEXT NOT NULL CHECK (LENGTH(BTRIM(name))>0),
    tax_id TEXT NOT NULL DEFAULT '',
    billing_address TEXT NOT NULL DEFAULT '',
    shipping_address TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    credit_days INTEGER NOT NULL DEFAULT 0 CHECK (credit_days BETWEEN 0 AND 3650),
    credit_limit_cents BIGINT NOT NULL DEFAULT 0 CHECK (credit_limit_cents>=0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    revision INTEGER NOT NULL DEFAULT 1,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(branch_id,customer_code)
);
CREATE UNIQUE INDEX v2_customer_business_tax_unique ON v2_customers(branch_id,tax_id)
    WHERE customer_type='business' AND tax_id<>'';
CREATE TABLE v2_stock_accounts (
    branch_id UUID NOT NULL REFERENCES branches(id),
    product_id UUID NOT NULL REFERENCES products(id),
    base_quantity BIGINT NOT NULL DEFAULT 0 CHECK(base_quantity>=0),
    value_cents BIGINT NOT NULL DEFAULT 0 CHECK(value_cents>=0),
    PRIMARY KEY(branch_id,product_id)
);
CREATE TABLE v2_lots (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id),
    product_id UUID NOT NULL REFERENCES products(id),
    lot_number TEXT NOT NULL,
    expires_on DATE,
    state TEXT NOT NULL DEFAULT 'available' CHECK(state IN ('available','quarantined','recalled')),
    received_quantity BIGINT NOT NULL CHECK(received_quantity>0),
    remaining_quantity BIGINT NOT NULL CHECK(remaining_quantity>=0),
    landed_cost_cents BIGINT NOT NULL CHECK(landed_cost_cents>=0),
    origin_lot_id UUID REFERENCES v2_lots(id),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID NOT NULL REFERENCES users(id)
);
CREATE INDEX v2_lot_fefo ON v2_lots(branch_id,product_id,expires_on,received_at,id);
CREATE TABLE v2_cost_layers (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id),
    product_id UUID NOT NULL REFERENCES products(id),
    lot_id UUID NOT NULL REFERENCES v2_lots(id),
    remaining_quantity BIGINT NOT NULL CHECK(remaining_quantity>=0),
    remaining_value_cents BIGINT NOT NULL CHECK(remaining_value_cents>=0),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_reservations (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id),
    reference TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','released','consumed','expired')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_reservation_lines (
    id UUID PRIMARY KEY,
    reservation_id UUID NOT NULL REFERENCES v2_reservations(id),
    lot_id UUID NOT NULL REFERENCES v2_lots(id),
    product_id UUID NOT NULL REFERENCES products(id),
    base_quantity BIGINT NOT NULL CHECK(base_quantity>0),
    unit_snapshot JSONB NOT NULL
);
CREATE TABLE v2_stock_events (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    branch_id UUID NOT NULL REFERENCES branches(id),
    product_id UUID NOT NULL REFERENCES products(id),
    lot_id UUID REFERENCES v2_lots(id),
    event_type TEXT NOT NULL,
    quantity_delta BIGINT NOT NULL,
    value_delta_cents BIGINT NOT NULL,
    reference TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    unit_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_lot_events (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    lot_id UUID NOT NULL REFERENCES v2_lots(id),
    from_state TEXT NOT NULL,
    to_state TEXT NOT NULL,
    reason TEXT NOT NULL CHECK(LENGTH(BTRIM(reason))>0),
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_documents (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    branch_id UUID NOT NULL REFERENCES branches(id),
    document_number TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK(kind IN ('quotation','ar_invoice','ap_bill','ar_credit','ap_credit')),
    status TEXT NOT NULL CHECK(status IN ('draft','issued','converted','cancelled')),
    customer_id UUID REFERENCES v2_customers(id),
    supplier_id UUID REFERENCES suppliers(id),
    counterparty_snapshot JSONB NOT NULL,
    source_document_id UUID REFERENCES v2_documents(id),
    total_cents BIGINT NOT NULL CHECK(total_cents>=0),
    vat_cents BIGINT NOT NULL DEFAULT 0 CHECK(vat_cents>=0),
    discount_cents BIGINT NOT NULL DEFAULT 0 CHECK(discount_cents>=0),
    issued_on DATE NOT NULL,
    due_on DATE NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK((customer_id IS NOT NULL) <> (supplier_id IS NOT NULL))
);
CREATE TABLE v2_document_lines (
    id UUID PRIMARY KEY,
    document_id UUID NOT NULL REFERENCES v2_documents(id),
    product_id UUID NOT NULL REFERENCES products(id),
    source_line_id UUID REFERENCES v2_document_lines(id),
    description TEXT NOT NULL,
    quantity BIGINT NOT NULL CHECK(quantity>0),
    base_quantity BIGINT NOT NULL CHECK(base_quantity>0),
    unit_snapshot JSONB NOT NULL,
    unit_price_cents BIGINT NOT NULL CHECK(unit_price_cents>=0),
    discount_cents BIGINT NOT NULL CHECK(discount_cents>=0),
    vat_bps INTEGER NOT NULL CHECK(vat_bps BETWEEN 0 AND 10000),
    vat_cents BIGINT NOT NULL CHECK(vat_cents>=0),
    total_cents BIGINT NOT NULL CHECK(total_cents>=0),
    price_source TEXT NOT NULL,
    UNIQUE(document_id,source_line_id)
);
CREATE TABLE v2_payments (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    branch_id UUID NOT NULL REFERENCES branches(id),
    direction TEXT NOT NULL CHECK(direction IN ('receive','pay')),
    customer_id UUID REFERENCES v2_customers(id),
    supplier_id UUID REFERENCES suppliers(id),
    method TEXT NOT NULL CHECK(method IN ('cash','bank_transfer','cheque')),
    amount_cents BIGINT NOT NULL CHECK(amount_cents>0),
    reference TEXT NOT NULL DEFAULT '',
    paid_on DATE NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK((customer_id IS NOT NULL) <> (supplier_id IS NOT NULL))
);
CREATE TABLE v2_allocations (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    payment_id UUID NOT NULL REFERENCES v2_payments(id),
    document_id UUID NOT NULL REFERENCES v2_documents(id),
    amount_cents BIGINT NOT NULL CHECK(amount_cents>0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(operation_id,payment_id,document_id)
);
CREATE TABLE v2_cheques (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id),
    payment_id UUID NOT NULL UNIQUE REFERENCES v2_payments(id),
    cheque_number TEXT NOT NULL,
    bank TEXT NOT NULL,
    due_on DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'received' CHECK(status IN ('received','deposited','cleared','bounced','cancelled')),
    revision INTEGER NOT NULL DEFAULT 1,
    UNIQUE(branch_id,bank,cheque_number)
);
CREATE TABLE v2_money_events (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    branch_id UUID NOT NULL REFERENCES branches(id),
    document_id UUID REFERENCES v2_documents(id),
    payment_id UUID REFERENCES v2_payments(id),
    cheque_id UUID REFERENCES v2_cheques(id),
    event_type TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    effective_on DATE NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE v2_price_rules (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id),
    customer_id UUID REFERENCES v2_customers(id),
    product_id UUID NOT NULL REFERENCES products(id),
    unit_id UUID REFERENCES product_units(id),
    min_base_quantity BIGINT NOT NULL DEFAULT 1 CHECK(min_base_quantity>0),
    unit_price_cents BIGINT NOT NULL CHECK(unit_price_cents>=0),
    starts_on DATE NOT NULL,
    ends_on DATE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID NOT NULL REFERENCES users(id),
    CHECK(ends_on IS NULL OR ends_on>=starts_on)
);
CREATE TABLE v2_outbox (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','done','dead')),
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_until TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(operation_id,event_type)
);
CREATE TABLE v2_expiry_tasks (
    id UUID PRIMARY KEY,
    lot_id UUID NOT NULL REFERENCES v2_lots(id),
    band_months INTEGER NOT NULL CHECK(band_months IN (0,3,6,9)),
    status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','acknowledged','closed')),
    assigned_to UUID REFERENCES users(id),
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(lot_id,band_months)
);
CREATE TABLE v2_drawers (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id),
    opened_by UUID NOT NULL REFERENCES users(id),
    opening_cents BIGINT NOT NULL CHECK(opening_cents>=0),
    opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ,
    counted_cents BIGINT,
    expected_cents BIGINT,
    close_reason TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX v2_drawer_open ON v2_drawers(branch_id,opened_by) WHERE closed_at IS NULL;
CREATE TABLE v2_drawer_events (
    id UUID PRIMARY KEY,
    operation_id UUID NOT NULL REFERENCES v2_operations(id),
    drawer_id UUID NOT NULL REFERENCES v2_drawers(id),
    payment_id UUID UNIQUE REFERENCES v2_payments(id),
    amount_cents BIGINT NOT NULL,
    reason TEXT NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE FUNCTION v2_reject_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'V2 evidence is append-only; use a reversal event'; END;
$$;
CREATE TRIGGER v2_stock_events_immutable BEFORE UPDATE OR DELETE ON v2_stock_events FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_lot_events_immutable BEFORE UPDATE OR DELETE ON v2_lot_events FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_money_events_immutable BEFORE UPDATE OR DELETE ON v2_money_events FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_payments_immutable BEFORE UPDATE OR DELETE ON v2_payments FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_allocations_immutable BEFORE UPDATE OR DELETE ON v2_allocations FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_document_lines_immutable BEFORE UPDATE OR DELETE ON v2_document_lines FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE TRIGGER v2_drawer_events_immutable BEFORE UPDATE OR DELETE ON v2_drawer_events FOR EACH ROW EXECUTE FUNCTION v2_reject_evidence_mutation();
CREATE FUNCTION v2_preserve_document() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'V2 documents cannot be deleted'; END IF;
    IF (to_jsonb(NEW)-'status') IS DISTINCT FROM (to_jsonb(OLD)-'status') THEN
        RAISE EXCEPTION 'V2 document facts cannot be changed; create a revision or credit note';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER v2_document_facts_immutable BEFORE UPDATE OR DELETE ON v2_documents FOR EACH ROW EXECUTE FUNCTION v2_preserve_document();
