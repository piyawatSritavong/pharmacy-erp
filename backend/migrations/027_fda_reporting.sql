-- Part B, Rule 1 (อย. — Thai FDA): products has no way to flag/register a
-- product against an อย. filing today, and there's nowhere to record the
-- company's own license number the submission document needs.
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS requires_fda_report BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS fda_registration_no TEXT;

INSERT INTO app_settings (setting_key, setting_value, metadata, created_at, updated_at)
VALUES ('fda_license_no', '', '{"label": "เลขที่ใบอนุญาต อย."}'::jsonb, NOW(), NOW())
ON CONFLICT (setting_key) DO NOTHING;

-- New permission, gated the same way the rest of the report-generation
-- surface is (super_admin gets everything already via seed.go; admin is
-- granted explicitly below since D11's migration 025 predates this permission).
INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES (gen_random_uuid(), 'fda.manage', 'จัดการรายงาน อย.', 'เลือกสินค้าและสร้างเอกสารนำส่ง อย.', NOW(), NOW())
ON CONFLICT (permission_key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key IN ('super_admin', 'admin') AND p.permission_key = 'fda.manage'
ON CONFLICT DO NOTHING;
