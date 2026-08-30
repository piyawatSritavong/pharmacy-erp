CREATE TABLE IF NOT EXISTS product_categories (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    color TEXT NOT NULL DEFAULT '#D71920',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO product_categories (id, name, color)
VALUES
    ('11111111-1111-4111-8111-111111111111', 'อุปกรณ์ผู้ป่วย', '#D71920'),
    ('22222222-2222-4222-8222-222222222222', 'ของใช้ประจำวัน', '#F7B32B'),
    ('33333333-3333-4333-8333-333333333333', 'เวชภัณฑ์', '#111111'),
    ('44444444-4444-4444-8444-444444444444', 'ยา', '#22A06B')
ON CONFLICT (name) DO NOTHING;

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS category_id UUID REFERENCES product_categories(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS barcode TEXT,
    ADD COLUMN IF NOT EXISTS image_storage_key TEXT,
    ADD COLUMN IF NOT EXISTS image_mime_type TEXT;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_id_fkey;
ALTER TABLE users ADD CONSTRAINT users_role_id_fkey
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_branch_id_fkey;
ALTER TABLE users ADD CONSTRAINT users_branch_id_fkey
    FOREIGN KEY (branch_id) REFERENCES branches(id) ON DELETE CASCADE;

ALTER TABLE inventory_movements DROP CONSTRAINT IF EXISTS inventory_movements_performed_by_fkey;
ALTER TABLE inventory_movements ADD CONSTRAINT inventory_movements_performed_by_fkey
    FOREIGN KEY (performed_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE quotations DROP CONSTRAINT IF EXISTS quotations_created_by_fkey;
ALTER TABLE quotations ADD CONSTRAINT quotations_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_created_by_fkey;
ALTER TABLE invoices ADD CONSTRAINT invoices_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE invoice_payments DROP CONSTRAINT IF EXISTS invoice_payments_created_by_fkey;
ALTER TABLE invoice_payments ADD CONSTRAINT invoice_payments_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE checks DROP CONSTRAINT IF EXISTS checks_created_by_fkey;
ALTER TABLE checks ADD CONSTRAINT checks_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE transfers DROP CONSTRAINT IF EXISTS transfers_requested_by_fkey;
ALTER TABLE transfers ADD CONSTRAINT transfers_requested_by_fkey
    FOREIGN KEY (requested_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE transfers DROP CONSTRAINT IF EXISTS transfers_dispatched_by_fkey;
ALTER TABLE transfers ADD CONSTRAINT transfers_dispatched_by_fkey
    FOREIGN KEY (dispatched_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE transfers DROP CONSTRAINT IF EXISTS transfers_received_by_fkey;
ALTER TABLE transfers ADD CONSTRAINT transfers_received_by_fkey
    FOREIGN KEY (received_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE transfer_events DROP CONSTRAINT IF EXISTS transfer_events_actor_id_fkey;
ALTER TABLE transfer_events ADD CONSTRAINT transfer_events_actor_id_fkey
    FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS audit_logs_actor_id_fkey;
ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_actor_id_fkey
    FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE marketplace_connections DROP CONSTRAINT IF EXISTS marketplace_connections_created_by_fkey;
ALTER TABLE marketplace_connections ADD CONSTRAINT marketplace_connections_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE installment_plans DROP CONSTRAINT IF EXISTS installment_plans_created_by_fkey;
ALTER TABLE installment_plans ADD CONSTRAINT installment_plans_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE installment_payments DROP CONSTRAINT IF EXISTS installment_payments_received_by_fkey;
ALTER TABLE installment_payments ADD CONSTRAINT installment_payments_received_by_fkey
    FOREIGN KEY (received_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE marketplace_order_items DROP CONSTRAINT IF EXISTS marketplace_order_items_product_id_fkey;
ALTER TABLE marketplace_order_items ADD CONSTRAINT marketplace_order_items_product_id_fkey
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_products_barcode_unique
    ON products (barcode)
    WHERE barcode IS NOT NULL AND barcode <> '';

UPDATE products
SET category_id = CASE
    WHEN sku LIKE 'BED-%' THEN '11111111-1111-4111-8111-111111111111'::uuid
    WHEN sku LIKE 'DIAPER-%' THEN '22222222-2222-4222-8222-222222222222'::uuid
    WHEN sku LIKE 'MASK-%' THEN '33333333-3333-4333-8333-333333333333'::uuid
    WHEN sku LIKE 'MED-%' THEN '44444444-4444-4444-8444-444444444444'::uuid
    ELSE category_id
END
WHERE category_id IS NULL;

-- Existing business history is preserved, but every actor reference to the
-- removed role is reassigned to the canonical super administrator first.
DO $$
DECLARE
    replacement_user UUID;
BEGIN
    SELECT u.id INTO replacement_user
    FROM users u
    INNER JOIN roles r ON r.id = u.role_id
    WHERE r.role_key = 'super_admin'
    ORDER BY u.created_at ASC
    LIMIT 1;

    IF replacement_user IS NOT NULL THEN
        UPDATE inventory_movements SET performed_by = replacement_user
        WHERE performed_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE quotations SET created_by = replacement_user
        WHERE created_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE invoices SET created_by = replacement_user
        WHERE created_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE invoice_payments SET created_by = replacement_user
        WHERE created_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE checks SET created_by = replacement_user
        WHERE created_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE transfers SET requested_by = replacement_user
        WHERE requested_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE transfers SET dispatched_by = replacement_user
        WHERE dispatched_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE transfers SET received_by = replacement_user
        WHERE received_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE transfer_events SET actor_id = replacement_user
        WHERE actor_id IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE audit_logs SET actor_id = replacement_user
        WHERE actor_id IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE marketplace_connections SET created_by = replacement_user
        WHERE created_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE installment_plans SET created_by = replacement_user
        WHERE created_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
        UPDATE installment_payments SET received_by = replacement_user
        WHERE received_by IN (SELECT u.id FROM users u INNER JOIN roles r ON r.id = u.role_id WHERE r.role_key = 'branch_admin');
    END IF;

    DELETE FROM users
    WHERE role_id IN (SELECT id FROM roles WHERE role_key = 'branch_admin');
    DELETE FROM roles WHERE role_key = 'branch_admin';
END $$;

DELETE FROM permissions
WHERE permission_key IN (
    'dashboard.view.branch',
    'inventory.manage.branch',
    'price.override.branch',
    'invoice.create.branch',
    'finance.manage.branch'
);
