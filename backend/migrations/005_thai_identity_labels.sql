UPDATE roles SET name = 'ผู้ดูแลระบบ' WHERE role_key = 'super_admin';
UPDATE roles SET name = 'พนักงานขายหน้าร้าน' WHERE role_key = 'branch_pos';

UPDATE users SET full_name = 'ผู้ดูแลระบบส่วนกลาง' WHERE LOWER(email) = 'superadmin@erp.local';
UPDATE users SET full_name = 'พนักงานขายหน้าร้าน' WHERE LOWER(email) = 'pos@erp.local';
