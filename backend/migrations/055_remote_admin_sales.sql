-- Head office (central_admin) can open a sale in the name of a branch it picks:
-- the customer either pays at that branch's POS (remote) or through head office
-- for pickup at a nearby branch. Either way it is an ordinary bill of that
-- branch, so this only needs a permission, granted to super_admin and
-- central_admin. The POS role keeps selling its own branch via invoice.create.pos.
INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES (gen_random_uuid(), 'invoice.create.remote', 'ขายหน้าร้านแทนสาขา', 'สำนักงานใหญ่เปิดการขายในนามสาขาที่เลือก', NOW(), NOW())
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r CROSS JOIN permissions p
WHERE r.role_key IN ('super_admin', 'central_admin') AND p.permission_key = 'invoice.create.remote'
ON CONFLICT DO NOTHING;
