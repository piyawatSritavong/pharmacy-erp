INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
VALUES (
    gen_random_uuid(),
    'reports.generate.global',
    'สร้างรายงานแบบกำหนดเอง',
    'สร้าง บันทึก และปักหมุดรายงานแบบกำหนดเองจากข้อมูลทุกสาขา',
    NOW(),
    NOW()
)
ON CONFLICT (permission_key) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    updated_at = NOW();

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
INNER JOIN permissions p ON p.permission_key = 'reports.generate.global'
WHERE r.role_key = 'super_admin'
ON CONFLICT (role_id, permission_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS report_definitions (
    id UUID PRIMARY KEY,
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    definition_version INTEGER NOT NULL DEFAULT 1,
    definition JSONB NOT NULL,
    is_pinned BOOLEAN NOT NULL DEFAULT FALSE,
    pin_order INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT report_definitions_name_not_blank CHECK (BTRIM(name) <> ''),
    CONSTRAINT report_definitions_name_length CHECK (CHAR_LENGTH(name) <= 120),
    CONSTRAINT report_definitions_description_length CHECK (CHAR_LENGTH(description) <= 500),
    CONSTRAINT report_definitions_version_positive CHECK (definition_version > 0),
    CONSTRAINT report_definitions_pin_state CHECK (
        (is_pinned = TRUE AND pin_order IS NOT NULL AND pin_order >= 0)
        OR (is_pinned = FALSE AND pin_order IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_report_definitions_owner_name
    ON report_definitions (owner_user_id, LOWER(BTRIM(name)));
CREATE UNIQUE INDEX IF NOT EXISTS idx_report_definitions_owner_pin_order
    ON report_definitions (owner_user_id, pin_order)
    WHERE is_pinned = TRUE;
CREATE INDEX IF NOT EXISTS idx_report_definitions_owner_updated
    ON report_definitions (owner_user_id, is_pinned DESC, pin_order, updated_at DESC);
