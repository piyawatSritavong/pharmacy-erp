-- Superadmin may open the two branch-operations pages (สรุปยอดขาย and
-- เช็กสต๊อก) that were previously reachable by POS and branch roles only.
-- Same fresh-install caveat as migrations 049 and 050: the seed catalog lists
-- these keys as well, because 'super_admin' does not exist yet while
-- migrations run on a pristine database.
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'super_admin'
  AND p.permission_key IN ('dashboard.view.self', 'inventory.view.branch')
ON CONFLICT DO NOTHING;
