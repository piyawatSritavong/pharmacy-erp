-- แอดมินกลาง: a central (all-branch) administrator that can see สต๊อกจริง but
-- not สต๊อกผี.
--
-- Until now both stock screens were gated by the single permission
-- inventory.manage.global, so there was no way to grant one without the other.
-- This splits ghost stock onto its own permission. Every existing role that
-- could reach สต๊อกผี is granted it here, so nothing changes for them — only
-- the new role is missing it.
INSERT INTO permissions (id, permission_key, name, description, created_at)
VALUES (
    gen_random_uuid(), 'inventory.ghost.manage', 'จัดการสต๊อกผี',
    'เข้าถึงเมนูสต๊อกผี — ดู รับเข้า และปรับยอดสต๊อกผี', NOW()
)
ON CONFLICT (permission_key) DO NOTHING;

-- Preserve current behaviour for everyone who already had it.
INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE p.permission_key = 'inventory.ghost.manage'
  AND EXISTS (
      SELECT 1 FROM role_permissions rp
      INNER JOIN permissions existing ON existing.id = rp.permission_id
      WHERE rp.role_id = r.id AND existing.permission_key = 'inventory.manage.global'
  )
ON CONFLICT DO NOTHING;

-- The role itself: same reach as แอดมิน but across every branch (scope=global),
-- minus ghost stock. users.manage/settings.manage stay out for the same reason
-- they are out of แอดมิน — an account that can edit roles can grant itself
-- anything, which would make this restriction meaningless.
INSERT INTO roles (id, role_key, name, active, is_system, portal, scope, created_at, updated_at)
VALUES (gen_random_uuid(), 'central_admin', 'แอดมินกลาง', TRUE, FALSE, 'backoffice', 'global', NOW(), NOW())
ON CONFLICT (role_key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT target.id, p.id, NOW()
FROM roles target
CROSS JOIN permissions p
WHERE target.role_key = 'central_admin'
  AND p.permission_key <> 'inventory.ghost.manage'
  AND EXISTS (
      SELECT 1 FROM role_permissions rp
      INNER JOIN roles source ON source.id = rp.role_id
      WHERE source.role_key = 'admin' AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- Account. Password hash is copied from an existing dev account, as in
-- migration 036 — bcrypt cannot be produced in plain SQL, and every dev login
-- shares one password. No-ops on a fresh database, where the seed builds users.
INSERT INTO users (id, role_id, branch_id, full_name, email, password_hash, active, created_at, updated_at)
SELECT gen_random_uuid(), r.id, NULL, 'แอดมินกลาง', 'admin.central@erp.local', template.password_hash, TRUE, NOW(), NOW()
FROM roles r
CROSS JOIN LATERAL (SELECT password_hash FROM users WHERE email = 'superadmin@erp.local' LIMIT 1) template
WHERE r.role_key = 'central_admin'
ON CONFLICT (email) DO NOTHING;
