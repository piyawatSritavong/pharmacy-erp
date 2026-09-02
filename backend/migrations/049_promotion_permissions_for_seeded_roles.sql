-- Migration 047 granted the promotion and POS-discount permissions by joining
-- the roles table, but on a pristine installation the application seed creates
-- super_admin and branch_pos *after* migrations run, so those two roles were
-- skipped. Databases seeded before this migration therefore hid the promotions
-- page from Superadmin and blocked line discounts for the POS role.
-- The seed catalog now lists these keys as well; this repairs existing data.
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'super_admin'
  AND p.permission_key IN ('promotion.manage', 'promotion.view', 'sales.discount.line')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'branch_pos'
  AND p.permission_key IN ('promotion.view', 'sales.discount.line')
ON CONFLICT DO NOTHING;
