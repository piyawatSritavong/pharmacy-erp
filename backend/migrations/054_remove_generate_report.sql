-- Generate Report (the custom report builder) is removed. Its saved reports,
-- pins and the permission that gated it go with it.
DELETE FROM role_permissions
 WHERE permission_id IN (SELECT id FROM permissions WHERE permission_key = 'reports.generate.global');
DELETE FROM permissions WHERE permission_key = 'reports.generate.global';
DROP TABLE IF EXISTS report_definitions CASCADE;
