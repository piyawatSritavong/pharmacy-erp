package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/platform"
)

type seedRole struct {
	ID     string
	Key    string
	Name   string
	Portal string
	Scope  string
}

// MinSeedPasswordLength is the floor for the accounts the seed creates. They
// are the first credentials on a new system and two of them carry global
// scope, so they are not a place for something short.
const MinSeedPasswordLength = 16

// ErrAlreadySeeded reports that the database already has users and the seed
// did nothing.
//
// It is a distinct error rather than a silent success because the two outcomes
// are indistinguishable from the outside and lead to opposite next steps: one
// means the accounts are ready, the other means somebody else's accounts are
// already there and this run changed nothing.
var ErrAlreadySeeded = errors.New("database already has users; seed made no changes")

// validateSeedPasswords refuses before anything is opened or written.
//
// The admin password used to be the string literal "DevPassword123!", compiled
// in, shared by superadmin@erp.local and admin.central@erp.local — both global
// scope — and printed on the public login page. No environment variable could
// change it. The POS password had an environment variable but fell back to the
// same literal when unset, so forgetting it was silent.
func validateSeedPasswords(cfg config.Config) error {
	for _, candidate := range []struct {
		variable string
		value    string
		accounts string
	}{
		{"SEED_ADMIN_PASSWORD", cfg.SeedAdminPassword, "superadmin@erp.local, admin.central@erp.local"},
		{"SEED_POS_PASSWORD", cfg.SeedPOSPassword, "pos.*@erp.local"},
	} {
		value := strings.TrimSpace(candidate.value)
		if value == "" {
			return fmt.Errorf("%s is not set; it becomes the password for %s and has no default", candidate.variable, candidate.accounts)
		}
		if len([]rune(value)) < MinSeedPasswordLength {
			return fmt.Errorf("%s is shorter than %d characters; it becomes the password for %s", candidate.variable, MinSeedPasswordLength, candidate.accounts)
		}
	}
	if strings.TrimSpace(cfg.SeedAdminPassword) == strings.TrimSpace(cfg.SeedPOSPassword) {
		return errors.New("SEED_ADMIN_PASSWORD and SEED_POS_PASSWORD are the same; the tills would hold the head-office password")
	}
	return nil
}

func Seed(ctx context.Context, db *sql.DB, cfg config.Config) error {
	// Before the transaction, before the connection is used for anything: a
	// seed that cannot set a real password should not have started.
	if err := validateSeedPasswords(cfg); err != nil {
		return err
	}

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
		if err = tx.Commit(); err != nil {
			return err
		}
		err = ErrAlreadySeeded
		return err
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
			// A branch runs its own promotions; the service pins every write to
			// the branch the user belongs to.
			"promotion.view", "promotion.manage", "sales.discount.line",
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
	// seedInventoryFloorTx used to run here. It invents a supplier
	// ("บริษัททดสอบรับสินค้าเข้าสต๊อก") and posts purchase orders against it to
	// give every product a floor quantity — fabricated purchasing history, in a
	// customer's own supplier list and purchase reports, created by a command
	// whose job is the catalog. It remains available as `seed-inventory-floor`
	// for development, where that is what you want.
	err = tx.Commit()
	return err

	// The demo fixtures that used to follow here could never run: this seed
	// commits and returns above them. `go vet` says so, and that stops
	// `go test ./...` from running at all, so the dead branch is gone rather
	// than carried. SeedDemo below is the demo path that is actually reached.
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
