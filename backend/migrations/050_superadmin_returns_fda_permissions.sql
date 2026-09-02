-- Same fresh-install gap as migration 049, for two older grants.
--
-- Migrations 027 and 028 granted 'fda.manage' and 'returns.manage' to
-- 'super_admin' by joining the roles table, but on a pristine installation the
-- application seed creates that role only after all migrations have run, so the
-- grant matched no row. The result: Superadmin was redirected away from the
-- claims and FDA report pages that those migrations meant it to own.
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'super_admin'
  AND p.permission_key IN ('returns.manage', 'fda.manage')
ON CONFLICT DO NOTHING;
