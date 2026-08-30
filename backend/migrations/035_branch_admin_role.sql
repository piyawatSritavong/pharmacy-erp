-- Branch administrators (แอดมินระบบสาขา).
--
-- Why a new role rather than making `admin` branch-scoped: a role carries the
-- scope, so flipping `admin` to scope='branch' would force admin@erp.local to
-- pick a branch and would narrow it to that branch's data. That account is
-- deliberately central, so branch administrators get their own role instead.
--
-- Scope rule after this migration: every account must name a branch except the
-- three central back-office roles (super_admin, admin, office) — enforced by
-- validateRoleAssignment, which rejects a blank branch on any scope='branch'
-- role and rejects a branch on any scope='global' one.
INSERT INTO roles (id, role_key, name, active, is_system, portal, scope, created_at, updated_at)
VALUES (gen_random_uuid(), 'branch_admin', 'แอดมินระบบสาขา', TRUE, FALSE, 'backoffice', 'branch', NOW(), NOW())
ON CONFLICT (role_key) DO NOTHING;

-- Permission set: full administrative control INSIDE one branch.
--
-- Every key below is one the backend actually confines to the caller's own
-- branch — either through validateBranchScope (inventory receive/adjust/
-- rebalance) or through a `user.Scope <> 'global'` filter on the query
-- (invoices, claims, transfers). The branch admin therefore administers its
-- own branch and cannot read or touch another's.
--
-- Deliberately NOT granted, because these are global in the code and would
-- silently hand a branch admin every other branch's data:
--   purchase_orders.*.global, suppliers.*.global — no user-branch filter at
--       all; the list is filtered only by an optional branch_id query param.
--   reports.*.global, report-builder                   — no branch filter.
--   audit.view.global, dashboard.view.global           — whole-company views.
--   transfer.approve / transfer.dispatch               — lifts the branch
--       filter off the transfer list and approves other branches' requests.
--   products.manage, settings.manage, users.manage,
--   month_end.*, marketplace.manage.global,
--   price.override.global, invoice.sequence.manage     — system-wide master
--       data and configuration; these stay with the central roles.
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'branch_admin'
  AND p.permission_key IN (
    'dashboard.view.self',
    'products.view',
    'inventory.view.branch', 'inventory.manage.global', 'inventory.receive', 'inventory.rebalance',
    'invoice.view', 'invoice.reprint', 'quotation.manage', 'payment.collect',
    'returns.manage',
    'transfer.request.branch', 'transfer.receive',
    'government.use',
    'marketplace.view.branch'
  )
ON CONFLICT DO NOTHING;
