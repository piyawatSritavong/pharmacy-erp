CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY,
    role_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS permissions (
    id UUID PRIMARY KEY,
    permission_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE IF NOT EXISTS branches (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    address TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    role_id UUID NOT NULL REFERENCES roles(id),
    branch_id UUID REFERENCES branches(id),
    full_name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS app_settings (
    setting_key TEXT PRIMARY KEY,
    setting_value TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS document_sequences (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    doc_type TEXT NOT NULL,
    prefix TEXT NOT NULL,
    next_number BIGINT NOT NULL DEFAULT 1,
    is_locked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (branch_id, doc_type)
);

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY,
    sku TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    cost_price NUMERIC(12,2) NOT NULL,
    base_selling_price NUMERIC(12,2) NOT NULL,
    unit_name TEXT NOT NULL DEFAULT 'unit',
    tax_exempt BOOLEAN NOT NULL DEFAULT FALSE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS branch_product_prices (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    selling_price NUMERIC(12,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (branch_id, product_id)
);

CREATE TABLE IF NOT EXISTS product_aliases (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    branch_id UUID REFERENCES branches(id) ON DELETE CASCADE,
    alias_code TEXT NOT NULL UNIQUE,
    alias_name TEXT NOT NULL,
    default_government_price NUMERIC(12,2),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS inventory (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    qty_real INTEGER NOT NULL DEFAULT 0,
    qty_ghost INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (branch_id, product_id)
);

CREATE TABLE IF NOT EXISTS inventory_movements (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    movement_type TEXT NOT NULL,
    stock_bucket TEXT NOT NULL CHECK (stock_bucket IN ('real', 'ghost')),
    quantity_delta INTEGER NOT NULL,
    reference_type TEXT NOT NULL,
    reference_id UUID,
    note TEXT NOT NULL DEFAULT '',
    performed_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS quotations (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    quote_number TEXT NOT NULL UNIQUE,
    customer_name TEXT NOT NULL,
    customer_tax_id TEXT,
    status TEXT NOT NULL CHECK (status IN ('draft', 'converted', 'cancelled')),
    subtotal NUMERIC(12,2) NOT NULL,
    tax_rate NUMERIC(5,2) NOT NULL,
    tax_amount NUMERIC(12,2) NOT NULL,
    total_amount NUMERIC(12,2) NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    converted_invoice_id UUID,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS quotation_items (
    id UUID PRIMARY KEY,
    quotation_id UUID NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id),
    alias_id UUID REFERENCES product_aliases(id),
    display_name TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    stock_bucket TEXT NOT NULL CHECK (stock_bucket IN ('real', 'ghost')),
    unit_price NUMERIC(12,2) NOT NULL,
    line_subtotal NUMERIC(12,2) NOT NULL,
    price_source TEXT NOT NULL,
    override_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invoices (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    invoice_number TEXT NOT NULL UNIQUE,
    source_quote_id UUID REFERENCES quotations(id),
    customer_name TEXT NOT NULL,
    customer_tax_id TEXT,
    payment_status TEXT NOT NULL CHECK (payment_status IN ('unpaid', 'paid')),
    invoice_status TEXT NOT NULL CHECK (invoice_status IN ('issued', 'cancelled')),
    is_government_mode BOOLEAN NOT NULL DEFAULT FALSE,
    subtotal NUMERIC(12,2) NOT NULL,
    tax_rate NUMERIC(5,2) NOT NULL,
    tax_amount NUMERIC(12,2) NOT NULL,
    total_amount NUMERIC(12,2) NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    issued_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invoice_items (
    id UUID PRIMARY KEY,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id),
    alias_id UUID REFERENCES product_aliases(id),
    actual_product_name TEXT NOT NULL,
    display_name TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    stock_bucket TEXT NOT NULL CHECK (stock_bucket IN ('real', 'ghost')),
    unit_price NUMERIC(12,2) NOT NULL,
    line_subtotal NUMERIC(12,2) NOT NULL,
    tax_rate NUMERIC(5,2) NOT NULL,
    tax_amount NUMERIC(12,2) NOT NULL,
    line_total NUMERIC(12,2) NOT NULL,
    price_source TEXT NOT NULL,
    override_reason TEXT NOT NULL DEFAULT '',
    cost_snapshot NUMERIC(12,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invoice_payments (
    id UUID PRIMARY KEY,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    payment_type TEXT NOT NULL CHECK (payment_type IN ('cash', 'bank_transfer', 'check')),
    amount NUMERIC(12,2) NOT NULL,
    reference_code TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS checks (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    check_number TEXT NOT NULL UNIQUE,
    bank_name TEXT NOT NULL,
    payer_name TEXT NOT NULL,
    amount NUMERIC(12,2) NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'applied')),
    received_date DATE NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS payment_invoice_map (
    id UUID PRIMARY KEY,
    check_id UUID NOT NULL REFERENCES checks(id) ON DELETE CASCADE,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    applied_amount NUMERIC(12,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (check_id, invoice_id)
);

CREATE TABLE IF NOT EXISTS transfers (
    id UUID PRIMARY KEY,
    transfer_code TEXT NOT NULL UNIQUE,
    source_branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    destination_branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('requested', 'in_transit', 'completed')),
    request_note TEXT NOT NULL DEFAULT '',
    requested_by UUID NOT NULL REFERENCES users(id),
    dispatched_by UUID REFERENCES users(id),
    received_by UUID REFERENCES users(id),
    pickup_name TEXT NOT NULL DEFAULT '',
    courier_name TEXT NOT NULL DEFAULT '',
    qr_code TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL,
    dispatched_at TIMESTAMPTZ,
    received_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS transfer_items (
    id UUID PRIMARY KEY,
    transfer_id UUID NOT NULL REFERENCES transfers(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    stock_bucket TEXT NOT NULL CHECK (stock_bucket IN ('real', 'ghost')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS transfer_events (
    id UUID PRIMARY KEY,
    transfer_id UUID NOT NULL REFERENCES transfers(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    actor_id UUID REFERENCES users(id),
    event_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY,
    actor_id UUID REFERENCES users(id),
    branch_id UUID REFERENCES branches(id),
    entity_type TEXT NOT NULL,
    entity_id UUID,
    action TEXT NOT NULL,
    before_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    after_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_id TEXT NOT NULL DEFAULT '',
    source_ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS marketplace_providers (
    id UUID PRIMARY KEY,
    provider_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS marketplace_connections (
    id UUID PRIMARY KEY,
    provider_id UUID NOT NULL REFERENCES marketplace_providers(id) ON DELETE CASCADE,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    connection_name TEXT NOT NULL,
    credentials_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    settings_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL CHECK (status IN ('configured', 'disabled')),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS marketplace_orders (
    id UUID PRIMARY KEY,
    provider_id UUID NOT NULL REFERENCES marketplace_providers(id) ON DELETE CASCADE,
    connection_id UUID NOT NULL REFERENCES marketplace_connections(id) ON DELETE CASCADE,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
    external_order_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'processing', 'completed')),
    customer_name TEXT NOT NULL,
    order_total NUMERIC(12,2) NOT NULL,
    placed_at TIMESTAMPTZ NOT NULL,
    raw_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider_id, external_order_id)
);

CREATE TABLE IF NOT EXISTS marketplace_order_items (
    id UUID PRIMARY KEY,
    marketplace_order_id UUID NOT NULL REFERENCES marketplace_orders(id) ON DELETE CASCADE,
    product_id UUID REFERENCES products(id),
    sku TEXT NOT NULL,
    product_name TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price NUMERIC(12,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_branch_id ON users (branch_id);
CREATE INDEX IF NOT EXISTS idx_inventory_branch_product ON inventory (branch_id, product_id);
CREATE INDEX IF NOT EXISTS idx_inventory_movements_branch ON inventory_movements (branch_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_invoices_branch_status ON invoices (branch_id, payment_status, issued_at DESC);
CREATE INDEX IF NOT EXISTS idx_transfer_status ON transfers (status, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_checks_branch_status ON checks (branch_id, status);
CREATE INDEX IF NOT EXISTS idx_audit_logs_entity ON audit_logs (entity_type, entity_id, created_at DESC);
