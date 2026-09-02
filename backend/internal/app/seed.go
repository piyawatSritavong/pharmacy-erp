package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/platform"

	"golang.org/x/crypto/bcrypt"
)

type seedRole struct {
	ID     string
	Key    string
	Name   string
	Portal string
	Scope  string
}

func Seed(ctx context.Context, db *sql.DB, cfg config.Config) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var usersCount int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&usersCount); err != nil {
		return err
	}
	if usersCount > 0 {
		return tx.Commit()
	}

	roles := []seedRole{
		{ID: platform.MustUUID(), Key: "super_admin", Name: "ผู้ดูแลระบบ", Portal: "backoffice", Scope: "global"},
		{ID: platform.MustUUID(), Key: "branch_pos", Name: "พนักงานขายหน้าร้าน", Portal: "pos", Scope: "branch"},
	}

	permissions := []struct {
		ID          string
		Key         string
		Name        string
		Description string
	}{
		{platform.MustUUID(), "dashboard.view.global", "ดูแดชบอร์ดส่วนกลาง", "ดูภาพรวมทุกสาขา"},
		{platform.MustUUID(), "dashboard.view.self", "ดูสรุปยอดขายของตนเอง", "ดูยอดขายรายวันของผู้ใช้ปัจจุบัน"},
		{platform.MustUUID(), "products.manage", "จัดการสินค้า", "เพิ่ม แก้ไข และลบสินค้า"},
		{platform.MustUUID(), "products.view", "ดูสินค้า", "ดูรายการสินค้า"},
		{platform.MustUUID(), "inventory.manage.global", "จัดการสต๊อกทุกสาขา", "ปรับสต๊อกของทุกสาขา"},
		{platform.MustUUID(), "inventory.view.branch", "ดูสต๊อกสาขา", "ดูสต๊อกของสาขาที่กำหนด"},
		{platform.MustUUID(), "inventory.rebalance", "ย้ายประเภทสต๊อก", "ย้ายระหว่างสต๊อกจริงและสต๊อกผี"},
		{platform.MustUUID(), "inventory.receive", "รับสินค้าเข้า", "รับสินค้าเข้าสต๊อกจริงและสต๊อกผี"},
		{platform.MustUUID(), "inventory.ghost.manage", "จัดการสต๊อกผี", "ดูและจัดการสต๊อกผีภายใน"},
		{platform.MustUUID(), "transfer.request.branch", "ส่งคำขอโอนสินค้า", "ส่งคำขอสินค้าเพื่อให้ผู้ดูแลเลือกสาขาต้นทาง"},
		{platform.MustUUID(), "price.override.global", "กำหนดราคาพิเศษส่วนกลาง", "กำหนดราคาพิเศษสำหรับทุกสาขา"},
		{platform.MustUUID(), "price.override.pos", "กำหนดราคาพิเศษหน้าร้าน", "กำหนดราคาพิเศษจากจุดขาย"},
		{platform.MustUUID(), "government.manage_alias", "จัดการชื่อสินค้าสำหรับราชการ", "กำหนดชื่อสินค้าที่ใช้ในเอกสารราชการ"},
		{platform.MustUUID(), "government.use", "ใช้โหมดราชการ", "ขายสินค้าโดยใช้ชื่อสำหรับเอกสารราชการ"},
		{platform.MustUUID(), "invoice.sequence.manage", "จัดการเลขที่เอกสาร", "กำหนดคำนำหน้าและเลขถัดไป"},
		{platform.MustUUID(), "invoice.create.pos", "สร้างใบขายหน้าร้าน", "สร้างใบขายจากจุดขาย"},
		{platform.MustUUID(), "invoice.create.remote", "ขายหน้าร้านแทนสาขา", "สำนักงานใหญ่เปิดการขายในนามสาขาที่เลือก"},
		{platform.MustUUID(), "invoice.view", "ดูใบขาย", "ดูใบขายที่มีสิทธิ์เข้าถึง"},
		{platform.MustUUID(), "invoice.reprint", "พิมพ์ใบขายซ้ำ", "เปิดและพิมพ์ใบขายย้อนหลัง"},
		{platform.MustUUID(), "quotation.manage", "จัดการใบเสนอราคา", "สร้าง แปลง และลบใบเสนอราคา"},
		{platform.MustUUID(), "transfer.approve", "ดูแลการโอนสินค้า", "ดูรายการโอนสินค้าทุกสาขา"},
		{platform.MustUUID(), "transfer.request", "ขอโอนสินค้า", "สร้างรายการโอนสินค้า"},
		{platform.MustUUID(), "transfer.dispatch", "ส่งสินค้าโอน", "ยืนยันส่งสินค้าจากต้นทาง"},
		{platform.MustUUID(), "transfer.receive", "รับสินค้าโอน", "ยืนยันรับสินค้าที่ปลายทาง"},
		{platform.MustUUID(), "payment.collect", "รับชำระเงิน", "รับเงินสดและเงินโอน"},
		{platform.MustUUID(), "reports.view.global", "ดูรายงานส่วนกลาง", "ดูรายงานภาษีและกำไรขาดทุน"},
		{platform.MustUUID(), "suppliers.view.global", "ดูบริษัทคู่ค้า", "ดูบริษัทคู่ค้าส่วนกลาง"},
		{platform.MustUUID(), "suppliers.manage.global", "จัดการบริษัทคู่ค้า", "สร้าง แก้ไข และเก็บบริษัทคู่ค้า"},
		{platform.MustUUID(), "purchase_orders.view.global", "ดูใบสั่งซื้อเข้า", "ดูประวัติใบสั่งซื้อเข้าทุกสาขา"},
		{platform.MustUUID(), "purchase_orders.manage.global", "จัดการใบสั่งซื้อเข้า", "สร้าง แก้ไข และยกเลิกใบสั่งซื้อเข้า"},
		{platform.MustUUID(), "month_end.manage", "จัดการสรุปสิ้นเดือน", "คำนวณ ตรวจสอบ และยืนยันกระดาษทำการปิดเดือน"},
		{platform.MustUUID(), "month_end.view", "ดูสรุปสิ้นเดือน", "ดูรอบบัญชีและผลคำนวณสิ้นเดือน"},
		{platform.MustUUID(), "month_end.create", "สร้างรอบสิ้นเดือน", "สร้างและบันทึกร่างรอบบัญชี"},
		{platform.MustUUID(), "month_end.calculate", "คำนวณสิ้นเดือน", "ตรวจสอบและคำนวณข้อเสนอปรับปรุง"},
		{platform.MustUUID(), "month_end.adjust", "ปรับปรุงสิ้นเดือน", "เลือกและแก้ไขข้อเสนอปรับปรุง"},
		{platform.MustUUID(), "month_end.approve", "อนุมัติสิ้นเดือน", "อนุมัติรายการปรับปรุงรอบบัญชี"},
		{platform.MustUUID(), "month_end.close", "ปิดรอบสิ้นเดือน", "ยืนยันและล็อกรอบบัญชี"},
		{platform.MustUUID(), "month_end.reopen", "เปิดรอบสิ้นเดือนใหม่", "ย้อนรายการและสร้าง revision ใหม่"},
		{platform.MustUUID(), "month_end.export", "ส่งออกสรุปสิ้นเดือน", "ส่งออกข้อมูลจำลองและประวัติการคำนวณ"},
		{platform.MustUUID(), "settings.manage", "จัดการตั้งค่า", "จัดการการตั้งค่าของระบบ"},
		{platform.MustUUID(), "users.manage", "จัดการผู้ใช้", "เพิ่ม แก้ไข และลบผู้ใช้"},
		{platform.MustUUID(), "audit.view.global", "ดูประวัติการทำงาน", "ดูประวัติการเปลี่ยนแปลงของระบบ"},
		{platform.MustUUID(), "marketplace.manage.global", "จัดการตลาดออนไลน์", "จัดการผู้ให้บริการและการเชื่อมต่อตลาดออนไลน์"},
		{platform.MustUUID(), "marketplace.view.branch", "ดูคำสั่งซื้อตลาดออนไลน์", "ดูคำสั่งซื้อจากตลาดออนไลน์"},
	}

	for _, role := range roles {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO roles (id, role_key, name, active, is_system, portal, scope, created_at, updated_at)
			VALUES ($1, $2, $3, TRUE, TRUE, $4, $5, NOW(), NOW())
		`, role.ID, role.Key, role.Name, role.Portal, role.Scope); err != nil {
			return err
		}
	}

	for _, permission := range permissions {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO permissions (id, permission_key, name, description, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
			ON CONFLICT (permission_key) DO NOTHING
		`, permission.ID, permission.Key, permission.Name, permission.Description); err != nil {
			return err
		}
	}

	// Migrations may have inserted some permissions already (with their own
	// ids), so map keys to the ids that actually landed in the database.
	permissionByKey := map[string]string{}
	permissionRows, err := tx.QueryContext(ctx, `SELECT id, permission_key FROM permissions`)
	if err != nil {
		return err
	}
	for permissionRows.Next() {
		var id, key string
		if err = permissionRows.Scan(&id, &key); err != nil {
			permissionRows.Close()
			return err
		}
		permissionByKey[key] = id
	}
	if err = permissionRows.Err(); err != nil {
		permissionRows.Close()
		return err
	}
	permissionRows.Close()

	rolePermissionKeys := map[string][]string{
		"super_admin": {
			"dashboard.view.global", "products.manage", "products.view", "inventory.manage.global", "inventory.rebalance",
			"inventory.receive", "inventory.ghost.manage", "price.override.global", "government.manage_alias", "government.use", "invoice.sequence.manage", "invoice.view",
			"invoice.reprint", "quotation.manage",
			"transfer.approve", "transfer.request", "transfer.dispatch", "transfer.receive",
			"payment.collect", "reports.view.global", "invoice.create.remote", "month_end.manage", "month_end.view", "month_end.create",
			"month_end.calculate", "month_end.adjust", "month_end.approve", "month_end.close", "month_end.reopen", "month_end.export", "settings.manage", "users.manage",
			"audit.view.global", "marketplace.manage.global", "marketplace.view.branch",
			"suppliers.view.global", "suppliers.manage.global", "purchase_orders.view.global", "purchase_orders.manage.global",
			"promotion.manage", "promotion.view", "sales.discount.line",
			"returns.manage", "fda.manage",
			// Superadmin also opens the two branch-operations pages.
			"dashboard.view.self", "inventory.view.branch",
		},
		"branch_pos": {
			"dashboard.view.self", "products.view", "inventory.view.branch", "price.override.pos", "government.use",
			"transfer.request.branch", "invoice.create.pos", "invoice.view", "transfer.receive", "payment.collect",
			"promotion.view", "sales.discount.line",
		},
	}

	roleByKey := map[string]string{}
	for _, role := range roles {
		roleByKey[role.Key] = role.ID
	}

	for roleKey, keys := range rolePermissionKeys {
		for _, permissionKey := range keys {
			// Some permissions are created by migrations rather than the seed
			// catalog above; a missing id means the key is misspelled.
			if permissionByKey[permissionKey] == "" {
				return fmt.Errorf("seed role %s: unknown permission %q", roleKey, permissionKey)
			}
			if _, err = tx.ExecContext(ctx, `
				INSERT INTO role_permissions (role_id, permission_id, created_at)
				VALUES ($1, $2, NOW())
				ON CONFLICT DO NOTHING
			`, roleByKey[roleKey], permissionByKey[permissionKey]); err != nil {
				return err
			}
		}
	}

	// On a pristine database the admin presets are created by migrations before
	// the application permission catalog is seeded. Re-apply their non-sensitive
	// defaults here so fresh installations do not end up with an incomplete
	// navigation. Month-End and Ghost remain literal superadmin boundaries.
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO role_permissions (role_id,permission_id,created_at)
		SELECT role.id,permission.id,NOW()
		FROM roles role CROSS JOIN permissions permission
		WHERE role.role_key = 'central_admin'
		  AND permission.permission_key NOT IN (
			'users.manage','settings.manage','invoice.sequence.manage',
			'marketplace.manage.global','invoice.create.pos','price.override.pos',
			'inventory.ghost.manage','month_end.manage'
		  )
		  AND permission.permission_key NOT LIKE 'month_end.%'
		ON CONFLICT DO NOTHING
	`); err != nil {
		return err
	}

	if err = seedFreshOchaCatalogTx(ctx, tx, cfg, roleByKey); err != nil {
		return err
	}
	if _, err = seedInventoryFloorTx(ctx, tx, inventorySeedMinimum); err != nil {
		return err
	}
	err = tx.Commit()
	return err

	branchOneID := platform.MustUUID()
	branchTwoID := platform.MustUUID()
	for _, branch := range []struct {
		ID       string
		Code     string
		Name     string
		Address  string
		Type     string
		ParentID *string
	}{
		{branchOneID, "MNS", "มนัสการแพทย์", "99 ถนนสุขุมวิท กรุงเทพมหานคร", "main_warehouse", nil},
		{branchTwoID, "KNP", "คณาเภสัช", "88 ถนนพระราม 2 สมุทรสาคร", "branch", &branchOneID},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO branches (id, code, name, address, branch_type, parent_branch_id, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, TRUE, NOW(), NOW())
		`, branch.ID, branch.Code, branch.Name, branch.Address, branch.Type, platform.NullUUID(branch.ParentID)); err != nil {
			return err
		}
	}

	for _, item := range []struct {
		ID       string
		BranchID string
		DocType  string
		Prefix   string
		Next     int
		IsLocked bool
	}{
		{platform.MustUUID(), branchOneID, "invoice", "BL", 6, false},
		{platform.MustUUID(), branchTwoID, "invoice", "BL", 2, false},
		{platform.MustUUID(), branchOneID, "quotation", "QT", 2, false},
		{platform.MustUUID(), branchTwoID, "quotation", "QT", 1, false},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO document_sequences (id, branch_id, doc_type, prefix, next_number, is_locked, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		`, item.ID, item.BranchID, item.DocType, item.Prefix, item.Next, item.IsLocked); err != nil {
			return err
		}
	}

	for _, setting := range []struct {
		Key      string
		Value    string
		Metadata map[string]any
	}{
		{"vat_rate", fmt.Sprintf("%.2f", cfg.DefaultVATPct), map[string]any{"label": "อัตราภาษีมูลค่าเพิ่ม", "type": "number"}},
		{"company_name", "ระบบบริหารร้านขายยา PharmaPOS", map[string]any{"label": "ชื่อกิจการ"}},
		{"company_tax_id", "0105559999999", map[string]any{"label": "เลขประจำตัวผู้เสียภาษี"}},
		{"company_address", "สำนักงานใหญ่ 99 ถนนสุขุมวิท กรุงเทพมหานคร 10110", map[string]any{"label": "ที่อยู่กิจการ"}},
	} {
		metadata, _ := json.Marshal(setting.Metadata)
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO app_settings (setting_key, setting_value, metadata, created_at, updated_at)
			VALUES ($1, $2, $3::jsonb, NOW(), NOW())
		`, setting.Key, setting.Value, string(metadata)); err != nil {
			return err
		}
	}

	productIDs := []string{platform.MustUUID(), platform.MustUUID(), platform.MustUUID(), platform.MustUUID()}
	products := []struct {
		ID          string
		SKU         string
		Barcode     string
		CategoryID  string
		Name        string
		Description string
		Cost        float64
		Price       float64
		Unit        string
		TaxExempt   bool
	}{
		{productIDs[0], "BED-001", "8850000000011", "11111111-1111-4111-8111-111111111111", "เตียงผู้ป่วยปรับระดับ", "เตียงผู้ป่วยแบบมือหมุน", 4200, 5500, "ชิ้น", false},
		{productIDs[1], "DIAPER-001", "8850000000028", "22222222-2222-4222-8222-222222222222", "ผ้าอ้อมผู้ใหญ่", "ผ้าอ้อมผู้ใหญ่แบบกลางคืน", 180, 250, "แพ็ก", false},
		{productIDs[2], "MASK-001", "8850000000035", "33333333-3333-4333-8333-333333333333", "หน้ากากอนามัย", "หน้ากาก 3 ชั้น", 55, 89, "กล่อง", false},
		{productIDs[3], "MED-001", "8850000000042", "44444444-4444-4444-8444-444444444444", "ยาพาราเซตามอล", "500 มก.", 18, 35, "กล่อง", true},
	}

	for _, product := range products {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO products (id, sku, barcode, category_id, name, description, cost_price, base_selling_price, unit_name, tax_exempt, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, TRUE, NOW(), NOW())
		`, product.ID, product.SKU, product.Barcode, product.CategoryID, product.Name, product.Description, product.Cost, product.Price, product.Unit, product.TaxExempt); err != nil {
			return err
		}
	}

	for _, row := range []struct {
		ID       string
		BranchID string
		Product  string
		Price    float64
	}{
		{platform.MustUUID(), branchOneID, productIDs[0], 5400},
		{platform.MustUUID(), branchOneID, productIDs[1], 239},
		{platform.MustUUID(), branchTwoID, productIDs[1], 245},
		{platform.MustUUID(), branchTwoID, productIDs[2], 92},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO branch_product_prices (id, branch_id, product_id, selling_price, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
		`, row.ID, row.BranchID, row.Product, row.Price); err != nil {
			return err
		}
	}

	for _, alias := range []struct {
		ID        string
		ProductID string
		AliasCode string
		AliasName string
		GovPrice  float64
		BranchID  any
	}{
		{platform.MustUUID(), productIDs[0], "GOV-BED-01", "ผ้าอ้อมผู้ป่วย", 5200, nil},
		{platform.MustUUID(), productIDs[2], "GOV-MASK-01", "อุปกรณ์ป้องกันทางการแพทย์", 80, nil},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO product_aliases (id, product_id, branch_id, alias_code, alias_name, default_government_price, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, TRUE, NOW(), NOW())
		`, alias.ID, alias.ProductID, alias.BranchID, alias.AliasCode, alias.AliasName, alias.GovPrice); err != nil {
			return err
		}
	}

	for _, row := range []struct {
		ID       string
		BranchID string
		Product  string
		RealQty  int
		GhostQty int
	}{
		{platform.MustUUID(), branchOneID, productIDs[0], 5, 1},
		{platform.MustUUID(), branchOneID, productIDs[1], 50, 20},
		{platform.MustUUID(), branchOneID, productIDs[2], 80, 10},
		{platform.MustUUID(), branchOneID, productIDs[3], 100, 0},
		{platform.MustUUID(), branchTwoID, productIDs[0], 2, 0},
		{platform.MustUUID(), branchTwoID, productIDs[1], 40, 15},
		{platform.MustUUID(), branchTwoID, productIDs[2], 60, 5},
		{platform.MustUUID(), branchTwoID, productIDs[3], 120, 4},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO inventory (id, branch_id, product_id, qty_real, qty_ghost, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		`, row.ID, row.BranchID, row.Product, row.RealQty, row.GhostQty); err != nil {
			return err
		}
	}

	superHash, err := bcrypt.GenerateFromPassword([]byte("DevPassword123!"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	posHash, err := bcrypt.GenerateFromPassword([]byte("DevPassword123!"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	superUserID := platform.MustUUID()
	posUserID := platform.MustUUID()
	for _, user := range []struct {
		ID       string
		Name     string
		Email    string
		Hash     string
		RoleID   string
		BranchID any
	}{
		{superUserID, "ผู้ดูแลระบบ ERP", "superadmin@erp.local", string(superHash), roleByKey["super_admin"], nil},
		{posUserID, "พนักงานขายสาขามนัสการแพทย์", "pos@erp.local", string(posHash), roleByKey["branch_pos"], branchOneID},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO users (id, role_id, branch_id, full_name, email, password_hash, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, TRUE, NOW(), NOW())
		`, user.ID, user.RoleID, user.BranchID, user.Name, user.Email, user.Hash); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (
			id, branch_id, product_id, movement_type, stock_bucket, quantity_delta,
			reference_type, note, performed_by, created_at
		)
		SELECT gen_random_uuid(), i.branch_id, i.product_id, 'opening_balance',
		       bucket.stock_bucket,
		       CASE bucket.stock_bucket WHEN 'real' THEN i.qty_real ELSE i.qty_ghost END,
		       'seed.opening_balance', 'ยอดตั้งต้นจากข้อมูลสินค้า', $1, NOW()
		FROM inventory i
		CROSS JOIN (VALUES ('real'), ('ghost')) AS bucket(stock_bucket)
		WHERE CASE bucket.stock_bucket WHEN 'real' THEN i.qty_real ELSE i.qty_ghost END <> 0
	`, superUserID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_lots (
			id, branch_id, product_id, stock_bucket, lot_number, expires_on,
			received_quantity, remaining_quantity, unit_cost, source_type,
			source_id, source_item_id, received_at, created_at, updated_at
		)
		SELECT gen_random_uuid(), i.branch_id, i.product_id, bucket.stock_bucket,
		       'SEED-' || UPPER(SUBSTRING(REPLACE(i.id::text, '-', '') FROM 1 FOR 10)), NULL,
		       bucket.quantity, bucket.quantity, p.cost_price, 'seed_opening',
		       NULL, NULL, NOW(), NOW(), NOW()
		FROM inventory i
		INNER JOIN products p ON p.id=i.product_id
		CROSS JOIN LATERAL (
			VALUES ('real'::text, i.qty_real), ('ghost'::text, i.qty_ghost)
		) AS bucket(stock_bucket, quantity)
		WHERE bucket.quantity > 0
	`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_movement_lots (
			id, inventory_movement_id, inventory_lot_id, quantity_delta, created_at
		)
		SELECT gen_random_uuid(), m.id, l.id, m.quantity_delta, NOW()
		FROM inventory_movements m
		INNER JOIN inventory_lots l
		  ON l.branch_id=m.branch_id AND l.product_id=m.product_id
		 AND l.stock_bucket=m.stock_bucket AND l.source_type='seed_opening'
		WHERE m.reference_type='seed.opening_balance'
	`); err != nil {
		return err
	}

	return tx.Commit()

	// Legacy demo fixtures remain below for source compatibility with explicit
	// development helpers, but the normal seed path intentionally stops above.
	now := time.Now().UTC()
	quoteCreatedAt := now.Add(-12 * time.Hour)
	quoteExpiresAt := quoteCreatedAt.AddDate(0, 0, 30)
	openInvoiceIssuedAt := now.AddDate(0, 0, -2)
	paidInvoiceIssuedAt := now.AddDate(0, 0, -1)
	posTodayIssuedAt := now.Add(-2 * time.Hour)
	quoteNumber := platform.FormatSalesDocNumber("QT", quoteCreatedAt, 1)
	openInvoiceNumber := platform.FormatSalesDocNumber("BL", openInvoiceIssuedAt, 1)
	paidInvoiceNumber := platform.FormatSalesDocNumber("BL", paidInvoiceIssuedAt, 2)
	posTodayInvoiceNumber := platform.FormatSalesDocNumber("BL", posTodayIssuedAt, 3)

	quoteID := platform.MustUUID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO quotations (id, branch_id, quote_number, customer_name, customer_tax_id, status, subtotal, tax_rate, tax_amount, total_amount, created_by, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'คลินิกสุขภาพดี', '0105567000012', 'draft', 478.00, 7.00, 33.46, 511.46, $4, $5, $6, $6)
	`, quoteID, branchOneID, quoteNumber, superUserID, quoteExpiresAt, quoteCreatedAt); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO quotation_items (id, quotation_id, product_id, alias_id, display_name, quantity, stock_bucket, unit_price, line_subtotal, price_source, override_reason, created_at)
		VALUES
			($1, $2, $3, NULL, 'หน้ากากอนามัย', 2, 'real', 89.00, 178.00, 'branch_price', '', NOW()),
			($4, $2, $5, NULL, 'ยาพาราเซตามอล', 10, 'real', 30.00, 300.00, 'override', 'โปรโมชันลูกค้าประจำ', NOW())
	`, platform.MustUUID(), quoteID, productIDs[2], platform.MustUUID(), productIDs[3]); err != nil {
		return err
	}

	openInvoiceID := platform.MustUUID()
	paidInvoiceID := platform.MustUUID()
	posTodayInvoiceID := platform.MustUUID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoices (id, branch_id, invoice_number, customer_name, customer_tax_id, payment_status, invoice_status, is_government_mode, subtotal, tax_rate, tax_amount, total_amount, created_by, issued_at, created_at, updated_at)
		VALUES
			($1, $2, $3, 'องค์การบริหารส่วนตำบลสุขใจ', '0107567000001', 'unpaid', 'issued', TRUE, 5200.00, 7.00, 364.00, 5564.00, $4, $5, $5, $5),
			($6, $2, $7, 'ลูกค้าหน้าร้าน', NULL, 'paid', 'issued', FALSE, 245.00, 7.00, 17.15, 262.15, $8, $9, $9, $9),
			($10, $2, $11, 'ขายหน้าร้านประจำวัน', NULL, 'paid', 'issued', FALSE, 478.00, 7.00, 33.46, 511.46, $8, $12, $12, $12)
	`, openInvoiceID, branchOneID, openInvoiceNumber, superUserID, openInvoiceIssuedAt, paidInvoiceID, paidInvoiceNumber, posUserID, paidInvoiceIssuedAt, posTodayInvoiceID, posTodayInvoiceNumber, posTodayIssuedAt); err != nil {
		return err
	}

	var bedAliasID string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM product_aliases WHERE product_id = $1 LIMIT 1`, productIDs[0]).Scan(&bedAliasID); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoice_items (id, invoice_id, product_id, alias_id, actual_product_name, display_name, quantity, stock_bucket, unit_price, line_subtotal, tax_rate, tax_amount, line_total, price_source, override_reason, cost_snapshot, created_at)
		VALUES
			($1, $2, $3, $4, 'เตียงผู้ป่วยปรับระดับ', 'ผ้าอ้อมผู้ป่วย', 1, 'real', 5200.00, 5200.00, 7.00, 364.00, 5564.00, 'government_alias_default', 'งบเบิกจ่าย รพ.สต.', 4200.00, NOW()),
			($5, $6, $7, NULL, 'ผ้าอ้อมผู้ใหญ่', 'ผ้าอ้อมผู้ใหญ่', 1, 'real', 245.00, 245.00, 7.00, 17.15, 262.15, 'branch_price', '', 180.00, NOW()),
			($8, $9, $10, NULL, 'หน้ากากอนามัย', 'หน้ากากอนามัย', 2, 'real', 89.00, 178.00, 7.00, 12.46, 190.46, 'branch_price', '', 55.00, NOW()),
			($11, $9, $12, NULL, 'ยาพาราเซตามอล', 'ยาพาราเซตามอล', 10, 'real', 30.00, 300.00, 7.00, 21.00, 321.00, 'override', 'โปรโมชันรายวัน', 18.00, NOW())
	`, platform.MustUUID(), openInvoiceID, productIDs[0], bedAliasID, platform.MustUUID(), paidInvoiceID, productIDs[1], platform.MustUUID(), posTodayInvoiceID, productIDs[2], platform.MustUUID(), productIDs[3]); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoice_payments (id, invoice_id, payment_type, amount, reference_code, notes, created_by, created_at)
		VALUES
			($1, $2, 'cash', 262.15, 'CASH-0001', 'รับเงินสดหน้าร้าน', $3, NOW() - INTERVAL '1 day'),
			($4, $5, 'bank_transfer', 511.46, 'TRX-0003', 'รับโอนประจำวัน', $3, NOW() - INTERVAL '90 minute')
	`, platform.MustUUID(), paidInvoiceID, posUserID, platform.MustUUID(), posTodayInvoiceID); err != nil {
		return err
	}

	checkID := platform.MustUUID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO checks (id, branch_id, check_number, bank_name, payer_name, amount, status, received_date, created_by, created_at, updated_at)
		VALUES ($1, $2, 'CHK-0001', 'ธนาคารกรุงไทย', 'องค์การบริหารส่วนตำบลสุขใจ', 5564.00, 'pending', CURRENT_DATE, $3, NOW(), NOW())
	`, checkID, branchOneID, superUserID); err != nil {
		return err
	}

	transferID := platform.MustUUID()
	transferCode := "TRF-MNS-KNP-0001"
	if _, err = tx.ExecContext(ctx, `
			INSERT INTO transfers (id, transfer_code, source_branch_id, destination_branch_id, status, request_note, requested_by, pickup_name, courier_name, requested_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'in_transit', 'ขอโอนหน้ากากอนามัยด่วน', $5, 'สมชาย ผู้รับสินค้า', 'ขนส่ง ERP', NOW() - INTERVAL '5 hour', NOW(), NOW())
	`, transferID, transferCode, branchOneID, branchTwoID, superUserID); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO transfer_items (id, transfer_id, product_id, quantity, stock_bucket, created_at)
		VALUES ($1, $2, $3, 10, 'real', NOW())
	`, platform.MustUUID(), transferID, productIDs[2]); err != nil {
		return err
	}

	for _, event := range []struct {
		Status string
		Note   string
		Time   time.Time
	}{
		{"requested", "สาขาขอโอนหน้ากากอนามัย", now.Add(-8 * time.Hour)},
		{"in_transit", "สินค้าออกจากสาขาต้นทางแล้ว", now.Add(-5 * time.Hour)},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, platform.MustUUID(), transferID, event.Status, event.Note, superUserID, event.Time); err != nil {
			return err
		}
	}

	receiptTransferID := platform.MustUUID()
	receiptTransferCode := "TRF-KNP-MNS-0002"
	if _, err = tx.ExecContext(ctx, `
			INSERT INTO transfers (id, transfer_code, source_branch_id, destination_branch_id, status, request_note, requested_by, pickup_name, courier_name, requested_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'in_transit', 'ส่งผ้าอ้อมผู้ใหญ่กลับสาขา', $5, 'นิรันดร์ ผู้รับสินค้า', 'ขนส่ง ERP', NOW() - INTERVAL '3 hour', NOW(), NOW())
	`, receiptTransferID, receiptTransferCode, branchTwoID, branchOneID, superUserID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO transfer_items (id, transfer_id, product_id, quantity, stock_bucket, created_at)
		VALUES ($1, $2, $3, 5, 'real', NOW())
	`, platform.MustUUID(), receiptTransferID, productIDs[1]); err != nil {
		return err
	}
	for _, event := range []struct {
		Status string
		Note   string
		Time   time.Time
	}{
		{"requested", "สาขาขอโอนผ้าอ้อมผู้ใหญ่", now.Add(-6 * time.Hour)},
		{"in_transit", "สินค้าออกจากสาขาต้นทางแล้ว", now.Add(-3 * time.Hour)},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, platform.MustUUID(), receiptTransferID, event.Status, event.Note, superUserID, event.Time); err != nil {
			return err
		}
	}

	dispatchTransferID := platform.MustUUID()
	dispatchTransferCode := "TRF-MNS-KNP-0003"
	if _, err = tx.ExecContext(ctx, `
			INSERT INTO transfers (id, transfer_code, source_branch_id, destination_branch_id, status, request_note, requested_by, pickup_name, courier_name, requested_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'requested', 'รอส่งเตียงผู้ป่วยไปสาขาปลายทาง', $5, '', '', NOW() - INTERVAL '1 hour', NOW(), NOW())
	`, dispatchTransferID, dispatchTransferCode, branchOneID, branchTwoID, superUserID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO transfer_items (id, transfer_id, product_id, quantity, stock_bucket, created_at)
		VALUES ($1, $2, $3, 1, 'ghost', NOW())
	`, platform.MustUUID(), dispatchTransferID, productIDs[0]); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
		VALUES ($1, $2, 'requested', 'รายการโอนรอยืนยันส่งสินค้า', $3, $4)
	`, platform.MustUUID(), dispatchTransferID, superUserID, now.Add(-1*time.Hour)); err != nil {
		return err
	}

	providerID := platform.MustUUID()
	connectionID := platform.MustUUID()
	orderID := platform.MustUUID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO marketplace_providers (id, provider_key, name, description, active, created_at, updated_at)
		VALUES ($1, 'health-mart', 'Health Mart', 'ช่องทางรับคำสั่งซื้อจากตลาดออนไลน์', TRUE, NOW(), NOW())
	`, providerID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO marketplace_connections (id, provider_id, branch_id, connection_name, credentials_json, settings_json, status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, 'การเชื่อมต่อสาขา MNS', '{}'::jsonb, '{"webhook_enabled":true}'::jsonb, 'configured', $4, NOW(), NOW())
	`, connectionID, providerID, branchOneID, superUserID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO marketplace_orders (id, provider_id, connection_id, branch_id, external_order_id, status, customer_name, order_total, placed_at, raw_payload, created_at)
		VALUES ($1, $2, $3, $4, 'HM-1001', 'queued', 'ลูกค้า Health Mart', 178.00, NOW() - INTERVAL '3 hour', '{"channel":"health-mart","notes":"รอยืนยันจากสาขา"}'::jsonb, NOW())
	`, orderID, providerID, connectionID, branchOneID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO marketplace_order_items (id, marketplace_order_id, product_id, sku, product_name, quantity, unit_price, created_at)
		VALUES ($1, $2, $3, 'MASK-001', 'หน้ากากอนามัย', 2, 89.00, NOW())
	`, platform.MustUUID(), orderID, productIDs[2]); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs (id, actor_id, branch_id, entity_type, entity_id, action, before_data, after_data, request_id, source_ip, user_agent, created_at)
		VALUES
			($1, $2, $3, 'invoice', $4, 'seed.create', '{}'::jsonb, $11::jsonb, 'seed-run', '127.0.0.1', 'seed', NOW()),
			($5, $2, $3, 'transfer', $6, 'seed.create', '{}'::jsonb, '{"transfer_code":"TRF-MNS-KNP-0001"}'::jsonb, 'seed-run', '127.0.0.1', 'seed', NOW()),
			($7, $2, $3, 'transfer', $8, 'seed.create', '{}'::jsonb, '{"transfer_code":"TRF-KNP-MNS-0002"}'::jsonb, 'seed-run', '127.0.0.1', 'seed', NOW()),
			($9, $2, $3, 'transfer', $10, 'seed.create', '{}'::jsonb, '{"transfer_code":"TRF-MNS-KNP-0003"}'::jsonb, 'seed-run', '127.0.0.1', 'seed', NOW())
	`, platform.MustUUID(), superUserID, branchOneID, openInvoiceID, platform.MustUUID(), transferID, platform.MustUUID(), receiptTransferID, platform.MustUUID(), dispatchTransferID, platform.MustJSON(map[string]any{"invoice_number": openInvoiceNumber})); err != nil {
		return err
	}

	// --- Installment billing demo (DockBill feature port) ---
	// Invoice #4: active plan, one installment paid, one overdue.
	// Invoice #5: completed plan (ghost-bucket sale), fully paid.
	activePlanIssuedAt := now.AddDate(0, 0, -70)
	completedPlanIssuedAt := now.AddDate(0, 0, -75)
	activePlanInvoiceNumber := platform.FormatSalesDocNumber("BL", activePlanIssuedAt, 4)
	completedPlanInvoiceNumber := platform.FormatSalesDocNumber("BL", completedPlanIssuedAt, 5)
	activePlanInvoiceID := platform.MustUUID()
	completedPlanInvoiceID := platform.MustUUID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoices (id, branch_id, invoice_number, customer_name, customer_tax_id, payment_status, invoice_status, is_government_mode, subtotal, tax_rate, tax_amount, total_amount, created_by, issued_at, created_at, updated_at)
		VALUES
			($1, $2, $3, 'คุณสมชาย ใจดี (ผ่อนชำระ)', NULL, 'installment', 'issued', FALSE, 6200.00, 7.00, 434.00, 6634.00, $4, $5, $5, $5),
			($6, $2, $7, 'คุณวิภา รักสุขภาพ (ผ่อนครบแล้ว)', NULL, 'paid', 'issued', FALSE, 640.00, 7.00, 44.80, 684.80, $4, $8, $8, $8)
	`, activePlanInvoiceID, branchOneID, activePlanInvoiceNumber, superUserID, activePlanIssuedAt, completedPlanInvoiceID, completedPlanInvoiceNumber, completedPlanIssuedAt); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoice_items (id, invoice_id, product_id, alias_id, actual_product_name, display_name, quantity, stock_bucket, unit_price, line_subtotal, tax_rate, tax_amount, line_total, price_source, override_reason, cost_snapshot, created_at)
		VALUES
			($1, $2, $3, NULL, 'เตียงผู้ป่วยปรับระดับ', 'เตียงผู้ป่วยปรับระดับ', 1, 'real', 6200.00, 6200.00, 7.00, 434.00, 6634.00, 'installment_tier', '', 4200.00, NOW()),
			($4, $5, $6, NULL, 'ผ้าอ้อมผู้ใหญ่', 'ผ้าอ้อมผู้ใหญ่', 2, 'ghost', 320.00, 640.00, 7.00, 44.80, 684.80, 'installment_tier', '', 180.00, NOW())
	`, platform.MustUUID(), activePlanInvoiceID, productIDs[0], platform.MustUUID(), completedPlanInvoiceID, productIDs[1]); err != nil {
		return err
	}

	activePlanID := platform.MustUUID()
	completedPlanID := platform.MustUUID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO installment_plans (id, invoice_id, branch_id, months, monthly_amount, total_amount, status, created_by, created_at, updated_at)
		VALUES
			($1, $2, $3, 6, 1105.67, 6634.00, 'active', $4, $5, $5),
			($6, $7, $3, 2, 342.40, 684.80, 'completed', $4, $8, $8)
	`, activePlanID, activePlanInvoiceID, branchOneID, superUserID, activePlanIssuedAt, completedPlanID, completedPlanInvoiceID, completedPlanIssuedAt); err != nil {
		return err
	}

	// Active plan: seq 1 paid, seq 2 due 10 days ago (shows as overdue), rest pending.
	for seq := 1; seq <= 6; seq++ {
		amount := 1105.67
		if seq == 6 {
			amount = 1105.65
		}
		paid := 0.0
		status := "pending"
		var paidAt any
		var receivedBy any
		if seq == 1 {
			paid = amount
			status = "paid"
			paidAt = now.AddDate(0, 0, -39)
			receivedBy = posUserID
		}
		dueDate := now.AddDate(0, 0, -40+(seq-1)*30)
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO installment_payments (id, plan_id, seq_number, due_date, amount, paid_amount, paid_at, status, received_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
		`, platform.MustUUID(), activePlanID, seq, dueDate.Format("2006-01-02"), amount, paid, paidAt, status, receivedBy, activePlanIssuedAt); err != nil {
			return err
		}
	}

	for seq, paidDaysAgo := range map[int]int{1: 44, 2: 14} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO installment_payments (id, plan_id, seq_number, due_date, amount, paid_amount, paid_at, status, received_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 342.40, 342.40, $5, 'paid', $6, $7, $7)
		`, platform.MustUUID(), completedPlanID, seq, now.AddDate(0, 0, -paidDaysAgo-1).Format("2006-01-02"), now.AddDate(0, 0, -paidDaysAgo), posUserID, completedPlanIssuedAt); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoice_payments (id, invoice_id, payment_type, amount, reference_code, notes, created_by, created_at)
		VALUES
			($1, $2, 'cash', 1105.67, 'INST-0001-1', 'รับชำระงวดที่ 1', $3, $4),
			($5, $6, 'cash', 342.40, 'INST-0002-1', 'รับชำระงวดที่ 1', $3, $7),
			($8, $6, 'bank_transfer', 342.40, 'INST-0002-2', 'รับชำระงวดสุดท้าย ปิดแผนผ่อน', $3, $9)
	`, platform.MustUUID(), activePlanInvoiceID, posUserID, now.AddDate(0, 0, -39), platform.MustUUID(), completedPlanInvoiceID, now.AddDate(0, 0, -45), platform.MustUUID(), now.AddDate(0, 0, -14)); err != nil {
		return err
	}

	// --- Branch 2 invoice settled by an applied check (payment_invoice_map demo) ---
	checkInvoiceIssuedAt := now.AddDate(0, 0, -3)
	checkInvoiceNumber := platform.FormatSalesDocNumber("BL", checkInvoiceIssuedAt, 1)
	checkInvoiceID := platform.MustUUID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoices (id, branch_id, invoice_number, customer_name, customer_tax_id, payment_status, invoice_status, is_government_mode, subtotal, tax_rate, tax_amount, total_amount, created_by, issued_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'รพ.สต.บางน้ำใส', '0994000158888', 'paid', 'issued', FALSE, 350.00, 7.00, 0.00, 350.00, $4, $5, $5, $5)
	`, checkInvoiceID, branchTwoID, checkInvoiceNumber, superUserID, checkInvoiceIssuedAt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoice_items (id, invoice_id, product_id, alias_id, actual_product_name, display_name, quantity, stock_bucket, unit_price, line_subtotal, tax_rate, tax_amount, line_total, price_source, override_reason, cost_snapshot, created_at)
		VALUES ($1, $2, $3, NULL, 'ยาพาราเซตามอล', 'ยาพาราเซตามอล', 10, 'real', 35.00, 350.00, 0.00, 0.00, 350.00, 'branch_price', '', 18.00, NOW())
	`, platform.MustUUID(), checkInvoiceID, productIDs[3]); err != nil {
		return err
	}
	appliedCheckID := platform.MustUUID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO checks (id, branch_id, check_number, bank_name, payer_name, amount, status, received_date, created_by, created_at, updated_at)
		VALUES ($1, $2, 'CHK-0002', 'ธนาคารกสิกรไทย', 'รพ.สต.บางน้ำใส', 350.00, 'applied', CURRENT_DATE - 2, $3, NOW(), NOW())
	`, appliedCheckID, branchTwoID, superUserID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO payment_invoice_map (id, check_id, invoice_id, applied_amount, created_at)
		VALUES ($1, $2, $3, 350.00, NOW() - INTERVAL '2 day')
	`, platform.MustUUID(), appliedCheckID, checkInvoiceID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoice_payments (id, invoice_id, payment_type, amount, reference_code, notes, created_by, created_at)
		VALUES ($1, $2, 'check', 350.00, 'CHK-0002', 'ตัดชำระด้วยเช็ค', $3, NOW() - INTERVAL '2 day')
	`, platform.MustUUID(), checkInvoiceID, superUserID); err != nil {
		return err
	}

	// --- Inventory movement history that reconciles to the stock levels above ---
	receivedAt := now.AddDate(0, 0, -80)
	type movement struct {
		BranchID  string
		ProductID string
		Type      string
		Bucket    string
		Delta     int
		RefType   string
		RefID     any
		Note      string
		At        time.Time
	}
	movements := []movement{
		{branchOneID, productIDs[0], "receive", "real", 7, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchOneID, productIDs[0], "receive", "ghost", 1, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchOneID, productIDs[1], "receive", "real", 53, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchOneID, productIDs[1], "receive", "ghost", 20, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchOneID, productIDs[2], "receive", "real", 82, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchOneID, productIDs[2], "receive", "ghost", 10, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchOneID, productIDs[3], "receive", "real", 110, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchOneID, productIDs[1], "rebalance", "real", -2, "inventory.rebalance", nil, "ปรับสัดส่วนขายเงินสด", now.AddDate(0, 0, -50)},
		{branchOneID, productIDs[1], "rebalance", "ghost", 2, "inventory.rebalance", nil, "ปรับสัดส่วนขายเงินสด", now.AddDate(0, 0, -50)},
		{branchOneID, productIDs[0], "sale", "real", -1, "invoice", openInvoiceID, "", openInvoiceIssuedAt},
		{branchOneID, productIDs[0], "sale", "real", -1, "invoice", activePlanInvoiceID, "", activePlanIssuedAt},
		{branchOneID, productIDs[1], "sale", "ghost", -2, "invoice", completedPlanInvoiceID, "", completedPlanIssuedAt},
		{branchOneID, productIDs[1], "sale", "real", -1, "invoice", paidInvoiceID, "", paidInvoiceIssuedAt},
		{branchOneID, productIDs[2], "sale", "real", -2, "invoice", posTodayInvoiceID, "", posTodayIssuedAt},
		{branchOneID, productIDs[3], "sale", "real", -10, "invoice", posTodayInvoiceID, "", posTodayIssuedAt},
		{branchTwoID, productIDs[0], "receive", "real", 2, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchTwoID, productIDs[1], "receive", "real", 40, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchTwoID, productIDs[1], "receive", "ghost", 15, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchTwoID, productIDs[2], "receive", "real", 60, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchTwoID, productIDs[2], "receive", "ghost", 5, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchTwoID, productIDs[3], "receive", "real", 130, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchTwoID, productIDs[3], "receive", "ghost", 4, "inventory.receive", nil, "รับสินค้าเข้าคลังรอบแรก", receivedAt},
		{branchTwoID, productIDs[3], "sale", "real", -10, "invoice", checkInvoiceID, "", checkInvoiceIssuedAt},
	}
	for _, m := range movements {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, platform.MustUUID(), m.BranchID, m.ProductID, m.Type, m.Bucket, m.Delta, m.RefType, m.RefID, m.Note, superUserID, m.At); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs (id, actor_id, branch_id, entity_type, entity_id, action, before_data, after_data, request_id, source_ip, user_agent, created_at)
		VALUES
			($1, $2, $3, 'installment_plan', $4, 'installment.plan_create', '{}'::jsonb, '{"months":6,"total_amount":6634.00}'::jsonb, 'seed-run', '127.0.0.1', 'seed', $5),
			($6, $2, $3, 'installment_plan', $7, 'installment.plan_create', '{}'::jsonb, '{"months":2,"total_amount":684.80}'::jsonb, 'seed-run', '127.0.0.1', 'seed', $8)
	`, platform.MustUUID(), superUserID, branchOneID, activePlanID, activePlanIssuedAt, platform.MustUUID(), completedPlanID, completedPlanIssuedAt); err != nil {
		return err
	}

	return tx.Commit()
}

func SeedDemo(ctx context.Context, db *sql.DB, cfg config.Config) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var branchOneID, branchTwoID, superUserID, posUserID string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM branches WHERE code = 'MNS' LIMIT 1`).Scan(&branchOneID); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM branches WHERE code = 'KNP' LIMIT 1`).Scan(&branchTwoID); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email = 'superadmin@erp.local' LIMIT 1`).Scan(&superUserID); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email = 'pos@erp.local' LIMIT 1`).Scan(&posUserID); err != nil {
		return err
	}

	productIDs := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT id, sku FROM products WHERE sku IN ('BED-001', 'DIAPER-001', 'MASK-001', 'MED-001')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, sku string
		if err = rows.Scan(&id, &sku); err != nil {
			return err
		}
		productIDs[sku] = id
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, sku := range []string{"BED-001", "DIAPER-001", "MASK-001", "MED-001"} {
		if _, ok := productIDs[sku]; !ok {
			return fmt.Errorf("missing demo product %s", sku)
		}
	}

	tables := []string{
		"month_end_workpaper_lines",
		"month_end_workpapers",
		"payment_invoice_map",
		"invoice_payments",
		"installment_payments",
		"installment_plans",
		"invoice_items",
		"invoices",
		"quotation_items",
		"quotations",
		"transfer_events",
		"transfer_items",
		"transfers",
		"checks",
		"inventory_movements",
	}
	for _, table := range tables {
		if _, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return err
		}
	}

	inventoryRows := []struct {
		branchID, productID string
		real, ghost         int
	}{
		{branchOneID, productIDs["DIAPER-001"], 47, 20},
		{branchOneID, productIDs["MASK-001"], 78, 10},
		{branchOneID, productIDs["MED-001"], 99, 0},
		{branchOneID, productIDs["BED-001"], 5, 1},
		{branchTwoID, productIDs["DIAPER-001"], 40, 15},
		{branchTwoID, productIDs["MASK-001"], 60, 5},
		{branchTwoID, productIDs["MED-001"], 120, 4},
	}
	for _, row := range inventoryRows {
		if _, err = tx.ExecContext(ctx, `UPDATE inventory SET qty_real = $1, qty_ghost = $2, updated_at = NOW() WHERE branch_id = $3 AND product_id = $4`, row.real, row.ghost, row.branchID, row.productID); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	issuedAt1 := platform.InBangkok(now).AddDate(0, 0, -3)
	issuedAt2 := platform.InBangkok(now).AddDate(0, 0, -2)
	issuedAt3 := platform.InBangkok(now).AddDate(0, 0, -1)
	invoice1ID := platform.MustUUID()
	invoice2ID := platform.MustUUID()
	invoice3ID := platform.MustUUID()
	invoiceNumber1 := platform.FormatSalesDocNumber("BL", issuedAt1, 1)
	invoiceNumber2 := platform.FormatSalesDocNumber("BL", issuedAt2, 2)
	invoiceNumber3 := platform.FormatSalesDocNumber("BL", issuedAt3, 3)

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoices (id, branch_id, invoice_number, customer_name, customer_tax_id, payment_status, invoice_status, is_government_mode, tax_invoice_type, subtotal, tax_rate, tax_amount, total_amount, created_by, issued_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'paid', 'issued', FALSE, 'abbreviated', 300.00, 7.00, 21.00, 321.00, $6, $7, $7, $7)
	`, invoice1ID, branchOneID, invoiceNumber1, "ลูกค้าหน้าร้าน 1", sql.NullString{}, posUserID, issuedAt1); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoices (id, branch_id, invoice_number, customer_name, customer_tax_id, payment_status, invoice_status, is_government_mode, tax_invoice_type, subtotal, tax_rate, tax_amount, total_amount, created_by, issued_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'paid', 'issued', FALSE, 'abbreviated', 180.00, 7.00, 12.60, 192.60, $6, $7, $7, $7)
	`, invoice2ID, branchOneID, invoiceNumber2, "ลูกค้าหน้าร้าน 2", sql.NullString{}, posUserID, issuedAt2); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoices (id, branch_id, invoice_number, customer_name, customer_tax_id, payment_status, invoice_status, is_government_mode, tax_invoice_type, subtotal, tax_rate, tax_amount, total_amount, created_by, issued_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'paid', 'issued', FALSE, 'full', 35.00, 7.00, 2.45, 37.45, $6, $7, $7, $7)
	`, invoice3ID, branchOneID, invoiceNumber3, "ลูกค้าหน้าร้าน 3", "0101234567890", posUserID, issuedAt3); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoice_items (id, invoice_id, product_id, alias_id, actual_product_name, display_name, quantity, stock_bucket, unit_price, line_subtotal, tax_rate, tax_amount, line_total, price_source, override_reason, cost_snapshot, created_at)
		VALUES ($1, $2, $3, NULL, 'ผ้าอ้อมผู้ใหญ่', 'ผ้าอ้อมผู้ใหญ่', 3, 'real', 100.00, 300.00, 7.00, 21.00, 321.00, 'branch_price', '', 80.00, NOW())
	`, platform.MustUUID(), invoice1ID, productIDs["DIAPER-001"]); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoice_items (id, invoice_id, product_id, alias_id, actual_product_name, display_name, quantity, stock_bucket, unit_price, line_subtotal, tax_rate, tax_amount, line_total, price_source, override_reason, cost_snapshot, created_at)
		VALUES ($1, $2, $3, NULL, 'หน้ากากอนามัย', 'หน้ากากอนามัย', 2, 'real', 90.00, 180.00, 7.00, 12.60, 192.60, 'branch_price', '', 55.00, NOW())
	`, platform.MustUUID(), invoice2ID, productIDs["MASK-001"]); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO invoice_items (id, invoice_id, product_id, alias_id, actual_product_name, display_name, quantity, stock_bucket, unit_price, line_subtotal, tax_rate, tax_amount, line_total, price_source, override_reason, cost_snapshot, created_at)
		VALUES ($1, $2, $3, NULL, 'ยาพาราเซตามอล', 'ยาพาราเซตามอล', 1, 'real', 35.00, 35.00, 7.00, 2.45, 37.45, 'branch_price', '', 18.00, NOW())
	`, platform.MustUUID(), invoice3ID, productIDs["MED-001"]); err != nil {
		return err
	}

	for _, payment := range []struct {
		invoiceID string
		amount    float64
		note      string
	}{
		{invoice1ID, 321.00, "รับเงินสดหน้าร้าน"},
		{invoice2ID, 192.60, "รับเงินสดหน้าร้าน"},
		{invoice3ID, 37.45, "รับเงินสดหน้าร้าน"},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO invoice_payments (id, invoice_id, payment_type, amount, reference_code, notes, created_by, created_at)
			VALUES ($1, $2, 'cash', $3, $4, $5, $6, NOW())
		`, platform.MustUUID(), payment.invoiceID, payment.amount, fmt.Sprintf("CASH-%s", payment.invoiceID[:8]), payment.note, posUserID); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
		VALUES ($1, $2, $3, 'sale', 'real', -3, 'invoice', $4, 'ขายผ้าอ้อมผู้ใหญ่', $5, $6)
	`, platform.MustUUID(), branchOneID, productIDs["DIAPER-001"], invoice1ID, posUserID, issuedAt1); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
		VALUES ($1, $2, $3, 'sale', 'real', -2, 'invoice', $4, 'ขายหน้ากากอนามัย', $5, $6)
	`, platform.MustUUID(), branchOneID, productIDs["MASK-001"], invoice2ID, posUserID, issuedAt2); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (id, branch_id, product_id, movement_type, stock_bucket, quantity_delta, reference_type, reference_id, note, performed_by, created_at)
		VALUES ($1, $2, $3, 'sale', 'real', -1, 'invoice', $4, 'ขายยาพาราเซตามอล', $5, $6)
	`, platform.MustUUID(), branchOneID, productIDs["MED-001"], invoice3ID, posUserID, issuedAt3); err != nil {
		return err
	}

	return tx.Commit()
}
