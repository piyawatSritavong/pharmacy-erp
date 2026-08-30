-- D11: role management. Adds two axes to `roles` so the backend can stop
-- hardcoding "super_admin" / "branch_pos" string checks:
--   portal — 'backoffice' (can log into the admin web app) vs 'pos' (POS-only
--            shell, matches AppShell's existing role_key==branch_pos branch).
--   scope  — 'global' (not tied to one branch) vs 'branch' (branch_id
--            required on the user, matches the existing super_admin/branch_pos
--            branch_id validation in users/module.go).
-- Then seeds three fixed role presets (Admin / Office / Branch Head) per the
-- user's D11 decision: fixed presets, not free-form role creation, with
-- role-level (not per-user) permission overrides.

ALTER TABLE roles
    ADD COLUMN IF NOT EXISTS portal TEXT NOT NULL DEFAULT 'backoffice' CHECK (portal IN ('backoffice', 'pos')),
    ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'global' CHECK (scope IN ('global', 'branch'));

UPDATE roles SET portal = 'pos', scope = 'branch' WHERE role_key = 'branch_pos';
UPDATE roles SET portal = 'backoffice', scope = 'global' WHERE role_key = 'super_admin';

-- super_admin and branch_pos stay is_system=TRUE (already set by
-- 002_gap_closure.sql) — the new UpdateRolePermissions endpoint refuses to
-- touch is_system roles, since super_admin's set must always cover "everything
-- except POS" and branch_pos's set is tightly coupled to POS-specific
-- frontend assumptions. These three presets are is_system=FALSE so their
-- permission set can be edited from Settings → ผู้ใช้ → บทบาทและสิทธิ์.
INSERT INTO roles (id, role_key, name, active, is_system, portal, scope, created_at, updated_at)
VALUES
    (gen_random_uuid(), 'admin', 'แอดมิน', TRUE, FALSE, 'backoffice', 'global', NOW(), NOW()),
    (gen_random_uuid(), 'office', 'ฝ่ายสำนักงาน', TRUE, FALSE, 'backoffice', 'global', NOW(), NOW()),
    (gen_random_uuid(), 'branch_head', 'หัวหน้าสาขา', TRUE, FALSE, 'backoffice', 'branch', NOW(), NOW())
ON CONFLICT (role_key) DO NOTHING;

-- Default permission sets, scoped down from super_admin per role's intended
-- responsibility (all editable later from the UI — these are starting
-- points, not fixed contracts).
--
-- admin: everything super_admin has except users.manage — runs the whole
-- business day-to-day but can't provision/edit other accounts or roles.
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'admin'
  AND p.permission_key IN (
    'dashboard.view.global', 'products.manage', 'products.view', 'inventory.manage.global', 'inventory.rebalance',
    'inventory.receive', 'price.override.global', 'government.manage_alias', 'government.use', 'invoice.sequence.manage',
    'invoice.view', 'invoice.reprint', 'quotation.manage',
    'transfer.approve', 'transfer.request', 'transfer.dispatch', 'transfer.receive',
    'payment.collect', 'reports.view.global', 'reports.generate.global',
    'month_end.manage', 'month_end.view', 'month_end.create', 'month_end.calculate', 'month_end.adjust',
    'month_end.approve', 'month_end.close', 'month_end.reopen', 'month_end.export', 'settings.manage',
    'audit.view.global', 'marketplace.manage.global', 'marketplace.view.branch',
    'suppliers.view.global', 'suppliers.manage.global', 'purchase_orders.view.global', 'purchase_orders.manage.global'
  )
ON CONFLICT DO NOTHING;

-- office: back-office paperwork (documents, purchasing, reports, month-end
-- drafting) — no physical stock adjustment, no user/branch/marketplace admin,
-- and no month-end approve/close/reopen (segregation of duties from admin).
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'office'
  AND p.permission_key IN (
    'dashboard.view.global', 'products.view', 'invoice.view', 'invoice.reprint', 'invoice.sequence.manage',
    'quotation.manage', 'payment.collect', 'government.use', 'government.manage_alias',
    'reports.view.global', 'reports.generate.global',
    'month_end.view', 'month_end.create', 'month_end.calculate', 'month_end.adjust', 'month_end.export',
    'suppliers.view.global', 'suppliers.manage.global', 'purchase_orders.view.global', 'purchase_orders.manage.global',
    'marketplace.view.branch', 'audit.view.global'
  )
ON CONFLICT DO NOTHING;

-- branch_head: supervises one branch — sees and can act on that branch's
-- stock, transfers, and sales, but nothing global (settings/users/suppliers/
-- purchasing/reports-builder/other branches).
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'branch_head'
  AND p.permission_key IN (
    'dashboard.view.self', 'products.view', 'inventory.view.branch', 'inventory.rebalance', 'inventory.receive',
    'transfer.request.branch', 'transfer.receive', 'invoice.view', 'invoice.reprint', 'payment.collect',
    'government.use', 'marketplace.view.branch'
  )
ON CONFLICT DO NOTHING;
