-- Collapse the role model to three roles and the account list to thirteen.
--
--   ผู้ดูแลระบบ (super_admin) — global, every branch
--   พนักงานขายหน้าร้าน (branch_pos) — POS portal, own branch
--   แอดมิน (admin) — back-office, own branch
--
-- `admin` changes meaning here: it was a global back-office role held by one
-- central account; it becomes the branch-scoped administrator each branch has
-- its own copy of. branch_head, office and the short-lived branch_admin role
-- (migration 035) all fold into it.

-- ---------------------------------------------------------------- accounts
-- Documents created by the four retired accounts move to superadmin rather
-- than being destroyed with them. purchase_orders.created_by is ON DELETE
-- RESTRICT, so without this the DELETE below simply fails; the rest would
-- have cascaded, taking 51 purchase orders, 22 invoices and 431 stock
-- movements with them.
DO $$
DECLARE
    keeper UUID;
    retiring UUID[];
BEGIN
    SELECT id INTO keeper FROM users WHERE email = 'superadmin@erp.local';
    SELECT array_agg(id) INTO retiring FROM users
     WHERE email IN ('admin@erp.local', 'office@erp.local', 'head.mes@erp.local', 'head.npt@erp.local');
    IF keeper IS NULL OR retiring IS NULL THEN
        RETURN;                       -- fresh database: the seed builds these
    END IF;

    UPDATE purchase_orders   SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE purchase_orders   SET updated_by   = keeper WHERE updated_by   = ANY(retiring);
    UPDATE purchase_orders   SET cancelled_by = keeper WHERE cancelled_by = ANY(retiring);
    UPDATE purchase_order_events SET actor_id = keeper WHERE actor_id     = ANY(retiring);
    UPDATE product_returns   SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE return_events     SET actor_id     = keeper WHERE actor_id     = ANY(retiring);
    UPDATE invoices          SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE invoice_payments  SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE quotations        SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE inventory_movements SET performed_by = keeper WHERE performed_by = ANY(retiring);
    UPDATE transfers         SET requested_by  = keeper WHERE requested_by  = ANY(retiring);
    UPDATE transfers         SET dispatched_by = keeper WHERE dispatched_by = ANY(retiring);
    UPDATE transfers         SET received_by   = keeper WHERE received_by   = ANY(retiring);
    UPDATE transfer_events   SET actor_id      = keeper WHERE actor_id      = ANY(retiring);
    UPDATE stock_transfer_requests SET requested_by = keeper WHERE requested_by = ANY(retiring);
    UPDATE stock_transfer_requests SET reviewed_by  = keeper WHERE reviewed_by  = ANY(retiring);
    UPDATE audit_logs        SET actor_id     = keeper WHERE actor_id     = ANY(retiring);
    UPDATE marketplace_connections SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE report_definitions SET owner_user_id = keeper WHERE owner_user_id = ANY(retiring);
    UPDATE parked_bills      SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE inventory_reclassifications SET created_by  = keeper WHERE created_by  = ANY(retiring);
    UPDATE inventory_reclassifications SET reversed_by = keeper WHERE reversed_by = ANY(retiring);
    UPDATE month_end_workpapers SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE month_end_workpapers SET approved_by = keeper WHERE approved_by = ANY(retiring);
    UPDATE month_end_workpapers SET closed_by = keeper WHERE closed_by = ANY(retiring);
    UPDATE month_end_workpapers SET cancelled_by = keeper WHERE cancelled_by = ANY(retiring);
    UPDATE month_end_workpapers SET finalized_by = keeper WHERE finalized_by = ANY(retiring);
    UPDATE month_end_workpapers SET reopened_by = keeper WHERE reopened_by = ANY(retiring);
    UPDATE month_end_calculation_runs SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE month_end_adjustments SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE month_end_adjustments SET approved_by = keeper WHERE approved_by = ANY(retiring);

    DELETE FROM users WHERE id = ANY(retiring);
END $$;

-- ------------------------------------------------------------------- roles
UPDATE roles SET scope = 'branch', portal = 'backoffice', name = 'แอดมิน'
 WHERE role_key = 'admin';

-- Every back-office permission except the six that either belong to the POS
-- portal or sit inside ตั้งค่า — a branch admin that can edit users could
-- promote itself out of its own branch, which would make "own branch only"
-- meaningless.
DELETE FROM role_permissions
 WHERE role_id = (SELECT id FROM roles WHERE role_key = 'admin');
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r CROSS JOIN permissions p
WHERE r.role_key = 'admin'
  AND p.permission_key NOT IN (
    'users.manage', 'settings.manage',          -- ตั้งค่า: accounts and branches
    'invoice.sequence.manage',                  -- ตั้งค่า: document numbering
    'marketplace.manage.global',                -- ตั้งค่า: online marketplace
    'invoice.create.pos', 'price.override.pos'  -- POS portal only
  )
ON CONFLICT DO NOTHING;

-- Branch admins created on the interim branch_admin role move across.
UPDATE users SET role_id = (SELECT id FROM roles WHERE role_key = 'admin')
 WHERE role_id IN (SELECT id FROM roles WHERE role_key IN ('branch_admin', 'branch_head', 'office'));

DELETE FROM roles WHERE role_key IN ('branch_admin', 'branch_head', 'office');

-- --------------------------------------------------------------- new users
-- (The warehouse till itself is enabled in migration 037, which has to drop a
-- CHECK constraint before the branch can sell.)

-- Password hash is copied from an existing account rather than hard-coded —
-- every dev account shares one password, and a bcrypt hash cannot be produced
-- in plain SQL. No-ops on a fresh database, where the seed creates these.
INSERT INTO users (id, role_id, branch_id, full_name, email, password_hash, active, created_at, updated_at)
SELECT gen_random_uuid(), r.id, b.id, seed.full_name, seed.email, template.password_hash, TRUE, NOW(), NOW()
FROM (VALUES
    ('branch_pos', 'WH',  'pos.warehouse@erp.local',   'พนักงานขายสาขา โกดัง'),
    ('admin',      'WH',  'admin.warehouse@erp.local', 'แอดมินระบบสาขา โกดัง'),
    ('admin',      'KNP', 'admin.knp@erp.local',       'แอดมินระบบสาขา คณาเภสัช')
) AS seed(role_key, branch_code, email, full_name)
JOIN roles r ON r.role_key = seed.role_key
JOIN branches b ON b.code = seed.branch_code
CROSS JOIN LATERAL (SELECT password_hash FROM users WHERE email = 'superadmin@erp.local' LIMIT 1) template
ON CONFLICT (email) DO NOTHING;
