-- Re-assert the permission sets for แอดมิน and แอดมินกลาง.
--
-- The role editor (PUT /roles/:id/permissions) replaces a role's whole set, so
-- one stray save can silently strip a role — แอดมิน lost every inventory.*
-- permission that way, which took สต๊อกจริง and stock adjustment with it.
-- Declaring both sets here restores them and makes the intent reproducible
-- rather than something that has to be re-clicked correctly.
--
-- Rule for both roles: every permission EXCEPT
--   users.manage, settings.manage, invoice.sequence.manage,
--   marketplace.manage.global   — these live inside ตั้งค่า, and a role that can
--                                 edit roles could grant itself anything;
--   invoice.create.pos, price.override.pos — POS portal only.
-- แอดมินกลาง additionally does not get inventory.ghost.manage: it is the role
-- that sees สต๊อกจริง without สต๊อกผี.
DELETE FROM role_permissions
 WHERE role_id IN (SELECT id FROM roles WHERE role_key IN ('admin', 'central_admin'));

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key IN ('admin', 'central_admin')
  AND p.permission_key NOT IN (
    'users.manage', 'settings.manage', 'invoice.sequence.manage',
    'marketplace.manage.global', 'invoice.create.pos', 'price.override.pos'
  )
  AND NOT (r.role_key = 'central_admin' AND p.permission_key = 'inventory.ghost.manage')
ON CONFLICT DO NOTHING;
