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
	ID   string
	Key  string
	Name string
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
		{ID: platform.MustUUID(), Key: "super_admin", Name: "Super Admin"},
		{ID: platform.MustUUID(), Key: "branch_admin", Name: "Branch Admin"},
		{ID: platform.MustUUID(), Key: "branch_pos", Name: "Branch POS"},
	}

	permissions := []struct {
		ID          string
		Key         string
		Name        string
		Description string
	}{
		{platform.MustUUID(), "dashboard.view.global", "Global Dashboard", "View organization dashboard"},
		{platform.MustUUID(), "dashboard.view.branch", "Branch Dashboard", "View branch dashboard"},
		{platform.MustUUID(), "dashboard.view.self", "Daily Sales Dashboard", "View personal sales dashboard"},
		{platform.MustUUID(), "products.manage", "Manage Products", "Create and update product catalog"},
		{platform.MustUUID(), "products.view", "View Products", "View product catalog"},
		{platform.MustUUID(), "inventory.manage.global", "Manage Global Inventory", "Manage inventory across branches"},
		{platform.MustUUID(), "inventory.manage.branch", "Manage Branch Inventory", "Adjust branch inventory"},
		{platform.MustUUID(), "inventory.view.branch", "View Branch Inventory", "View inventory in assigned branch"},
		{platform.MustUUID(), "inventory.rebalance", "Rebalance Inventory", "Move stock between real and ghost"},
		{platform.MustUUID(), "inventory.receive", "Receive Inventory", "Receive incoming stock into real and ghost buckets"},
		{platform.MustUUID(), "installment.view", "View Installments", "View installment plans and payments"},
		{platform.MustUUID(), "installment.manage", "Manage Installments", "Create installment plans for invoices"},
		{platform.MustUUID(), "installment.collect", "Collect Installment", "Record installment payments"},
		{platform.MustUUID(), "price.override.global", "Global Price Override", "Override price centrally"},
		{platform.MustUUID(), "price.override.branch", "Branch Price Override", "Override branch prices"},
		{platform.MustUUID(), "price.override.pos", "POS Price Override", "Override prices from POS"},
		{platform.MustUUID(), "government.manage_alias", "Manage Government Alias", "Manage alias mapping"},
		{platform.MustUUID(), "government.use", "Use Government Mode", "Sell with alias invoice display"},
		{platform.MustUUID(), "invoice.sequence.manage", "Manage Invoice Sequence", "Manage fixed invoice sequence"},
		{platform.MustUUID(), "invoice.create.branch", "Create Branch Invoice", "Create invoice from branch admin"},
		{platform.MustUUID(), "invoice.create.pos", "Create POS Invoice", "Create invoice from POS"},
		{platform.MustUUID(), "invoice.view", "View Invoices", "View branch invoices"},
		{platform.MustUUID(), "invoice.reprint", "Reprint Invoice", "Reprint invoice"},
		{platform.MustUUID(), "quotation.manage", "Manage Quotation", "Create and convert quotations"},
		{platform.MustUUID(), "transfer.approve", "Approve Transfer", "See all transfer requests"},
		{platform.MustUUID(), "transfer.request", "Request Transfer", "Create transfer request"},
		{platform.MustUUID(), "transfer.dispatch", "Dispatch Transfer", "Dispatch branch transfer"},
		{platform.MustUUID(), "transfer.receive", "Receive Transfer", "Receive branch transfer"},
		{platform.MustUUID(), "finance.manage.global", "Global Finance", "Manage checks globally"},
		{platform.MustUUID(), "finance.manage.branch", "Branch Finance", "Manage branch checks"},
		{platform.MustUUID(), "payment.collect", "Collect Payment", "Collect cash and bank transfers"},
		{platform.MustUUID(), "reports.view.global", "Global Reports", "View global finance reports"},
		{platform.MustUUID(), "settings.manage", "Manage Settings", "Manage org settings"},
		{platform.MustUUID(), "users.manage", "Manage Users", "Manage users and roles"},
		{platform.MustUUID(), "audit.view.global", "View Audit Logs", "View audit logs"},
		{platform.MustUUID(), "marketplace.manage.global", "Manage Marketplace", "Manage marketplace providers and connections"},
		{platform.MustUUID(), "marketplace.view.branch", "View Marketplace", "View marketplace orders"},
	}

	for _, role := range roles {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO roles (id, role_key, name, active, is_system, created_at, updated_at)
			VALUES ($1, $2, $3, TRUE, TRUE, NOW(), NOW())
		`, role.ID, role.Key, role.Name); err != nil {
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
			"inventory.receive", "price.override.global", "government.manage_alias", "government.use", "invoice.sequence.manage", "invoice.view",
			"invoice.reprint", "quotation.manage", "installment.view", "installment.manage", "installment.collect",
			"transfer.approve", "transfer.request", "transfer.dispatch", "transfer.receive",
			"finance.manage.global", "payment.collect", "reports.view.global", "settings.manage", "users.manage",
			"audit.view.global", "marketplace.manage.global", "marketplace.view.branch",
		},
		"branch_admin": {
			"dashboard.view.branch", "products.view", "inventory.manage.branch", "inventory.view.branch", "inventory.rebalance",
			"inventory.receive", "price.override.branch", "government.use", "invoice.create.branch", "invoice.view", "invoice.reprint",
			"quotation.manage", "installment.view", "installment.manage", "installment.collect",
			"transfer.request", "transfer.dispatch", "finance.manage.branch", "marketplace.view.branch",
		},
		"branch_pos": {
			"dashboard.view.self", "products.view", "inventory.view.branch", "price.override.pos", "government.use",
			"invoice.create.pos", "invoice.view", "installment.view", "installment.collect", "transfer.receive", "payment.collect",
		},
	}

	roleByKey := map[string]string{}
	for _, role := range roles {
		roleByKey[role.Key] = role.ID
	}

	for roleKey, keys := range rolePermissionKeys {
		for _, permissionKey := range keys {
			if _, err = tx.ExecContext(ctx, `
				INSERT INTO role_permissions (role_id, permission_id, created_at)
				VALUES ($1, $2, NOW())
				ON CONFLICT DO NOTHING
			`, roleByKey[roleKey], permissionByKey[permissionKey]); err != nil {
				return err
			}
		}
	}

	branchOneID := platform.MustUUID()
	branchTwoID := platform.MustUUID()
	for _, branch := range []struct {
		ID      string
		Code    string
		Name    string
		Address string
	}{
		{branchOneID, "MNS", "มนัสการแพทย์", "99 ถนนสุขุมวิท กรุงเทพมหานคร"},
		{branchTwoID, "KNP", "คณาเภสัช", "88 ถนนพระราม 2 สมุทรสาคร"},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO branches (id, code, name, address, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, TRUE, NOW(), NOW())
		`, branch.ID, branch.Code, branch.Name, branch.Address); err != nil {
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
		{"vat_rate", fmt.Sprintf("%.2f", cfg.DefaultVATPct), map[string]any{"label": "VAT percentage", "type": "number"}},
		{"company_name", "Pharmacy ERP Demo", map[string]any{"label": "Company Name"}},
		{"company_tax_id", "0105559999999", map[string]any{"label": "Company Tax ID"}},
		{"company_address", "สำนักงานใหญ่ 99 ถนนสุขุมวิท กรุงเทพมหานคร 10110", map[string]any{"label": "Company Address"}},
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
		Name        string
		Description string
		Cost        float64
		Price       float64
		Retail      float64
		Installment float64
		Unit        string
		TaxExempt   bool
	}{
		{productIDs[0], "BED-001", "เตียงผู้ป่วยปรับระดับ", "เตียงผู้ป่วยแบบมือหมุน", 4200, 5500, 5800, 6200, "unit", false},
		{productIDs[1], "DIAPER-001", "ผ้าอ้อมผู้ใหญ่", "ผ้าอ้อมผู้ใหญ่แบบกลางคืน", 180, 250, 269, 320, "pack", false},
		{productIDs[2], "MASK-001", "หน้ากากอนามัย", "หน้ากาก 3 ชั้น", 55, 89, 99, 0, "box", false},
		{productIDs[3], "MED-001", "ยาพาราเซตามอล", "500mg", 18, 35, 39, 0, "box", true},
	}

	for _, product := range products {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO products (id, sku, name, description, cost_price, base_selling_price, retail_price, installment_price, unit_name, tax_exempt, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, TRUE, NOW(), NOW())
		`, product.ID, product.SKU, product.Name, product.Description, product.Cost, product.Price, product.Retail, product.Installment, product.Unit, product.TaxExempt); err != nil {
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
	adminHash, err := bcrypt.GenerateFromPassword([]byte("DevPassword123!"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	posHash, err := bcrypt.GenerateFromPassword([]byte("DevPassword123!"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	superUserID := platform.MustUUID()
	adminUserID := platform.MustUUID()
	posUserID := platform.MustUUID()
	for _, user := range []struct {
		ID       string
		Name     string
		Email    string
		Hash     string
		RoleID   string
		BranchID any
	}{
		{superUserID, "ERP Super Admin", "superadmin@erp.local", string(superHash), roleByKey["super_admin"], nil},
		{adminUserID, "Khana Branch Admin", "branchadmin@erp.local", string(adminHash), roleByKey["branch_admin"], branchOneID},
		{posUserID, "Khana POS Staff", "pos@erp.local", string(posHash), roleByKey["branch_pos"], branchOneID},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO users (id, role_id, branch_id, full_name, email, password_hash, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, TRUE, NOW(), NOW())
		`, user.ID, user.RoleID, user.BranchID, user.Name, user.Email, user.Hash); err != nil {
			return err
		}
	}

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
	`, quoteID, branchOneID, quoteNumber, adminUserID, quoteExpiresAt, quoteCreatedAt); err != nil {
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
	`, openInvoiceID, branchOneID, openInvoiceNumber, adminUserID, openInvoiceIssuedAt, paidInvoiceID, paidInvoiceNumber, posUserID, paidInvoiceIssuedAt, posTodayInvoiceID, posTodayInvoiceNumber, posTodayIssuedAt); err != nil {
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
		VALUES ($1, $2, 'CHK-0001', 'Krungthai', 'องค์การบริหารส่วนตำบลสุขใจ', 5564.00, 'pending', CURRENT_DATE, $3, NOW(), NOW())
	`, checkID, branchOneID, adminUserID); err != nil {
		return err
	}

	transferID := platform.MustUUID()
	transferCode := "TRF-MNS-KNP-0001"
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO transfers (id, transfer_code, source_branch_id, destination_branch_id, status, request_note, requested_by, pickup_name, courier_name, qr_code, requested_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'in_transit', 'ขอโอนหน้ากากอนามัยด่วน', $5, 'Somchai Receiver', 'ERP Courier', $2, NOW() - INTERVAL '5 hour', NOW(), NOW())
	`, transferID, transferCode, branchOneID, branchTwoID, adminUserID); err != nil {
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
		{"requested", "Branch requested masks", now.Add(-8 * time.Hour)},
		{"in_transit", "Shipment left branch", now.Add(-5 * time.Hour)},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, platform.MustUUID(), transferID, event.Status, event.Note, adminUserID, event.Time); err != nil {
			return err
		}
	}

	receiptTransferID := platform.MustUUID()
	receiptTransferCode := "TRF-KNP-MNS-0002"
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO transfers (id, transfer_code, source_branch_id, destination_branch_id, status, request_note, requested_by, pickup_name, courier_name, qr_code, requested_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'in_transit', 'ส่งผ้าอ้อมผู้ใหญ่กลับสาขา', $5, 'Niran Receiver', 'ERP Courier', $2, NOW() - INTERVAL '3 hour', NOW(), NOW())
	`, receiptTransferID, receiptTransferCode, branchTwoID, branchOneID, adminUserID); err != nil {
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
		{"requested", "Branch requested diapers", now.Add(-6 * time.Hour)},
		{"in_transit", "Shipment left source branch", now.Add(-3 * time.Hour)},
	} {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO transfer_events (id, transfer_id, status, note, actor_id, event_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, platform.MustUUID(), receiptTransferID, event.Status, event.Note, adminUserID, event.Time); err != nil {
			return err
		}
	}

	dispatchTransferID := platform.MustUUID()
	dispatchTransferCode := "TRF-MNS-KNP-0003"
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO transfers (id, transfer_code, source_branch_id, destination_branch_id, status, request_note, requested_by, pickup_name, courier_name, qr_code, requested_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'requested', 'รอส่งเตียงผู้ป่วยไปสาขาปลายทาง', $5, '', '', $2, NOW() - INTERVAL '1 hour', NOW(), NOW())
	`, dispatchTransferID, dispatchTransferCode, branchOneID, branchTwoID, adminUserID); err != nil {
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
		VALUES ($1, $2, 'requested', 'Transfer queued for dispatch', $3, $4)
	`, platform.MustUUID(), dispatchTransferID, adminUserID, now.Add(-1*time.Hour)); err != nil {
		return err
	}

	providerID := platform.MustUUID()
	connectionID := platform.MustUUID()
	orderID := platform.MustUUID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO marketplace_providers (id, provider_key, name, description, active, created_at, updated_at)
		VALUES ($1, 'health-mart', 'Health Mart', 'Provider-ready marketplace inbox', TRUE, NOW(), NOW())
	`, providerID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO marketplace_connections (id, provider_id, branch_id, connection_name, credentials_json, settings_json, status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, 'Branch MNS Connector', '{}'::jsonb, '{"webhook_enabled":true}'::jsonb, 'configured', $4, NOW(), NOW())
	`, connectionID, providerID, branchOneID, adminUserID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO marketplace_orders (id, provider_id, connection_id, branch_id, external_order_id, status, customer_name, order_total, placed_at, raw_payload, created_at)
		VALUES ($1, $2, $3, $4, 'HM-1001', 'queued', 'HealthMart Customer', 178.00, NOW() - INTERVAL '3 hour', '{"channel":"health-mart","notes":"awaiting branch confirmation"}'::jsonb, NOW())
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
	`, platform.MustUUID(), adminUserID, branchOneID, openInvoiceID, platform.MustUUID(), transferID, platform.MustUUID(), receiptTransferID, platform.MustUUID(), dispatchTransferID, platform.MustJSON(map[string]any{"invoice_number": openInvoiceNumber})); err != nil {
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
	`, activePlanInvoiceID, branchOneID, activePlanInvoiceNumber, adminUserID, activePlanIssuedAt, completedPlanInvoiceID, completedPlanInvoiceNumber, completedPlanIssuedAt); err != nil {
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
	`, activePlanID, activePlanInvoiceID, branchOneID, adminUserID, activePlanIssuedAt, completedPlanID, completedPlanInvoiceID, completedPlanIssuedAt); err != nil {
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
		VALUES ($1, $2, 'CHK-0002', 'Kasikorn', 'รพ.สต.บางน้ำใส', 350.00, 'applied', CURRENT_DATE - 2, $3, NOW(), NOW())
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
		`, platform.MustUUID(), m.BranchID, m.ProductID, m.Type, m.Bucket, m.Delta, m.RefType, m.RefID, m.Note, adminUserID, m.At); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs (id, actor_id, branch_id, entity_type, entity_id, action, before_data, after_data, request_id, source_ip, user_agent, created_at)
		VALUES
			($1, $2, $3, 'installment_plan', $4, 'installment.plan_create', '{}'::jsonb, '{"months":6,"total_amount":6634.00}'::jsonb, 'seed-run', '127.0.0.1', 'seed', $5),
			($6, $2, $3, 'installment_plan', $7, 'installment.plan_create', '{}'::jsonb, '{"months":2,"total_amount":684.80}'::jsonb, 'seed-run', '127.0.0.1', 'seed', $8)
	`, platform.MustUUID(), adminUserID, branchOneID, activePlanID, activePlanIssuedAt, platform.MustUUID(), completedPlanID, completedPlanIssuedAt); err != nil {
		return err
	}

	return tx.Commit()
}
