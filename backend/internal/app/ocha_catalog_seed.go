package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path"
	"pharmacy-erp/backend/internal/platform/objectstore"
	"strings"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/platform"

	"golang.org/x/crypto/bcrypt"
)

type branchAccount struct {
	BranchCode string
	Email      string
	FullName   string
}

// One till per selling branch. The warehouse is a distribution hub, not a
// storefront, so it has no POS account.
var ochaPOSAccounts = []branchAccount{
	{BranchCode: "MES", Email: "pos.mes@erp.local", FullName: "พนักงานขายสาขา MES"},
	{BranchCode: "PHH", Email: "pos.phahol@erp.local", FullName: "พนักงานขายสาขาหน้ารพ.พหลฯ"},
	{BranchCode: "PHS", Email: "pos.phasuk@erp.local", FullName: "พนักงานขายสาขาหน้าตลาดผาสุก"},
	{BranchCode: "NPT", Email: "pos.nakhonpathom@erp.local", FullName: "พนักงานขายสาขาจังหวัดนครปฐม"},
	{BranchCode: "KNP", Email: "pos.knp@erp.local", FullName: "พนักงานขาย คณาเภสัช"},
}

func seedFreshOchaCatalogTx(ctx context.Context, tx *sql.Tx, cfg config.Config, roleByKey map[string]string) error {
	manifest, err := LoadOchaCatalog()
	if err != nil {
		return err
	}
	// Migrations install four legacy categories before fresh seed. A new empty
	// database has no business references yet, so replace those placeholders.
	if _, err := tx.ExecContext(ctx, `DELETE FROM product_categories`); err != nil {
		return fmt.Errorf("clear migration product categories: %w", err)
	}
	if err := insertOchaMasterDataTx(ctx, tx, manifest); err != nil {
		return err
	}
	if err := seedDefaultAppSettingsTx(ctx, tx, cfg); err != nil {
		return err
	}

	superHash, err := bcrypt.GenerateFromPassword([]byte("DevPassword123!"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	posHash, err := bcrypt.GenerateFromPassword([]byte(cfg.SeedPOSPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash SEED_POS_PASSWORD: %w", err)
	}
	superUserID := platform.MustUUID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, role_id, branch_id, full_name, email, password_hash, active, created_at, updated_at)
		VALUES ($1, $2, NULL, 'ผู้ดูแลระบบ ERP', 'superadmin@erp.local', $3, TRUE, NOW(), NOW())
	`, superUserID, roleByKey["super_admin"], string(superHash)); err != nil {
		return fmt.Errorf("seed super administrator: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (id,role_id,branch_id,full_name,email,password_hash,active,created_at,updated_at)
		SELECT $1,role.id,NULL,'แอดมินกลาง','admin.central@erp.local',$2,TRUE,NOW(),NOW()
		FROM roles role WHERE role.role_key='central_admin'
	`, platform.MustUUID(), string(superHash)); err != nil {
		return fmt.Errorf("seed central administrator: %w", err)
	}
	if err := replacePOSAccountsTx(ctx, tx, roleByKey["branch_pos"], string(posHash)); err != nil {
		return err
	}
	if err := seedOchaOpeningLedgerTx(ctx, tx, superUserID); err != nil {
		return err
	}
	return verifyOchaCatalogTx(ctx, tx, manifest)
}

func insertOchaMasterDataTx(ctx context.Context, tx *sql.Tx, manifest OchaCatalogManifest) error {
	primaryImages := map[string]OchaSeedProductImage{}
	warehouseBranches := map[string]bool{}
	operationalBranches := append([]OchaSeedBranch{}, manifest.Branches...)
	var warehouseID string
	for _, branch := range operationalBranches {
		warehouseBranches[branch.ID] = branch.Active && branch.BranchType == "main_warehouse"
		if warehouseBranches[branch.ID] {
			warehouseID = branch.ID
		}
	}
	if warehouseID != "" {
		parentID := warehouseID
		operationalBranches = append(operationalBranches, OchaSeedBranch{
			ID: "b8aaf520-9204-54d8-bb4e-b745a12dcd94", Code: "KNP", Name: "คณาเภสัช",
			BranchType: "branch", ParentBranchID: &parentID, Active: true, SalesEnabled: true,
		})
	}
	for _, image := range manifest.ProductImages {
		if image.IsPrimary {
			primaryImages[image.ProductID] = image
		}
	}
	for _, branch := range operationalBranches {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO branches (id, code, name, address, branch_type, parent_branch_id, active, sales_enabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		`, branch.ID, branch.Code, branch.Name, branch.Address, branch.BranchType, platform.NullUUID(branch.ParentBranchID), branch.Active, branch.SalesEnabled); err != nil {
			return fmt.Errorf("insert Ocha branch %s: %w", branch.Code, err)
		}
		for _, document := range []struct {
			Type   string
			Prefix string
		}{{"invoice", "BL"}, {"quotation", "QT"}, {"purchase_order", "PO"}} {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO document_sequences (id, branch_id, doc_type, prefix, next_number, is_locked, created_at, updated_at)
				VALUES ($1, $2, $3, $4, 1, FALSE, NOW(), NOW())
			`, platform.MustUUID(), branch.ID, document.Type, document.Prefix); err != nil {
				return fmt.Errorf("insert %s sequence for %s: %w", document.Type, branch.Code, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE branches SET online_sales_enabled=(code='KNP')`); err != nil {
		return fmt.Errorf("set online sales branch: %w", err)
	}
	for _, category := range manifest.ProductCategories {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_categories (id, name, color, active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
		`, category.ID, category.Name, category.Color, category.Active); err != nil {
			return fmt.Errorf("insert Ocha category %s: %w", category.Name, err)
		}
	}
	for _, product := range manifest.Products {
		primary := primaryImages[product.ID]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO products (
				id, sku, barcode, category_id, name, description, cost_price,
				base_selling_price, unit_name, max_discount_amount,
				low_stock_real_threshold, low_stock_ghost_threshold, tracks_expiry,
				expiry_warning_days, tax_exempt, active, image_storage_key, image_mime_type,
				created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,NULLIF($17,''),NULLIF($18,''),NOW(),NOW())
		`, product.ID, product.SKU, product.Barcode, platform.NullUUID(product.CategoryID), product.Name,
			product.Description, product.CostPrice, product.BaseSellingPrice,
			product.UnitName, product.MaxDiscountAmount, product.LowStockRealThreshold,
			product.LowStockGhostThreshold, product.TracksExpiry, product.ExpiryWarningDays,
			product.TaxExempt, product.Active, primary.StorageKey, primary.MimeType); err != nil {
			return fmt.Errorf("insert Ocha product %s: %w", product.SKU, err)
		}
	}
	for _, image := range manifest.ProductImages {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_images (
				id,product_id,storage_key,mime_type,sha256,source_url,source_branch_code,
				source_row_index,source_name,alt_text,is_primary,sort_order,created_at,updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW(),NOW())
		`, image.ID, image.ProductID, image.StorageKey, image.MimeType, image.SHA256,
			image.SourceURL, image.SourceBranchCode, image.SourceRowIndex, image.SourceName,
			image.AltText, image.IsPrimary, image.SortOrder); err != nil {
			return fmt.Errorf("insert Ocha product image %s: %w", image.ID, err)
		}
	}
	for _, alias := range manifest.ProductNameAliases {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_source_aliases (
				id,product_id,alias_name,normalized_alias,source_branch_code,source_row_index,created_at
			) VALUES ($1,$2,$3,$4,$5,$6,NOW())
		`, alias.ID, alias.ProductID, alias.AliasName, alias.NormalizedAlias, alias.SourceBranchCode, alias.SourceRowIndex); err != nil {
			return fmt.Errorf("insert Ocha product source alias %s: %w", alias.ID, err)
		}
	}
	for _, price := range manifest.BranchProductPrices {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO branch_product_prices (id, branch_id, product_id, selling_price, created_at, updated_at)
			VALUES ($1,$2,$3,$4,NOW(),NOW())
		`, price.ID, price.BranchID, price.ProductID, price.SellingPrice); err != nil {
			return fmt.Errorf("insert Ocha branch price: %w", err)
		}
	}
	for _, setting := range manifest.BranchProductSettings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO branch_product_settings (
				id, branch_id, product_id, max_discount_amount,
				low_stock_real_threshold, low_stock_ghost_threshold, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,NOW(),NOW())
		`, setting.ID, setting.BranchID, setting.ProductID, setting.MaxDiscountAmount,
			setting.LowStockRealThreshold, setting.LowStockGhostThreshold); err != nil {
			return fmt.Errorf("insert Ocha branch product setting: %w", err)
		}
	}
	for _, item := range manifest.Inventory {
		ghostQuantity := item.QtyGhost
		if !warehouseBranches[item.BranchID] {
			ghostQuantity = 0
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory (id, branch_id, product_id, qty_real, qty_ghost, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,NOW(),NOW())
		`, item.ID, item.BranchID, item.ProductID, item.QtyReal, ghostQuantity); err != nil {
			return fmt.Errorf("insert Ocha inventory: %w", err)
		}
	}
	for _, lot := range manifest.InventoryLots {
		if lot.StockBucket == "ghost" && !warehouseBranches[lot.BranchID] {
			continue
		}
		originLotID := lot.OriginLotID
		sourceType := lot.SourceType
		if lot.StockBucket == "ghost" {
			// The source-branch Ghost lots are intentionally omitted. Keep the
			// surviving WH quantity as an opening Ghost lot with no foreign origin.
			originLotID = nil
			if sourceType == "warehouse_bootstrap_copy" {
				sourceType = "ocha_seed_opening"
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_lots (
				id, branch_id, product_id, stock_bucket, lot_number, expires_on,
				received_quantity, remaining_quantity, unit_cost, source_type,
				source_id, source_item_id, origin_lot_id, received_at, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW(),NOW())
		`, lot.ID, lot.BranchID, lot.ProductID, lot.StockBucket, lot.LotNumber, lot.ExpiresOn,
			lot.ReceivedQuantity, lot.RemainingQuantity, lot.UnitCost, sourceType,
			platform.NullUUID(lot.SourceID), platform.NullUUID(lot.SourceItemID),
			platform.NullUUID(originLotID), lot.ReceivedAt); err != nil {
			return fmt.Errorf("insert Ocha inventory lot %s: %w", lot.LotNumber, err)
		}
	}
	return nil
}

// InstallOchaImageAssets pushes the catalog photographs into object storage.
//
// The pictures are embedded in the binary, not read off a disk, which is what
// makes this reproducible anywhere: the same command run against an empty
// bucket puts back exactly what the manifest describes, whether or not any
// machine still has the old uploads directory.
//
// Objects already present are skipped, so re-running costs one HEAD per image
// rather than re-sending ninety megabytes.
func InstallOchaImageAssets(ctx context.Context, images *objectstore.Client, manifest OchaCatalogManifest) (uploaded int, skipped int, err error) {
	if !images.Configured() {
		return 0, 0, objectstore.ErrNotConfigured
	}
	seen := map[string]bool{}
	for _, image := range manifest.ProductImages {
		if seen[image.StorageKey] {
			continue
		}
		seen[image.StorageKey] = true

		present, err := images.Exists(ctx, image.StorageKey)
		if err != nil {
			return uploaded, skipped, fmt.Errorf("check Ocha image %s: %w", image.StorageKey, err)
		}
		if present {
			skipped++
			continue
		}

		payload, err := ochaImageAssets.ReadFile("seeddata/ocha_images/" + path.Base(image.AssetName))
		if err != nil {
			return uploaded, skipped, fmt.Errorf("read embedded Ocha image %s: %w", image.AssetName, err)
		}
		if err := images.Upload(ctx, image.StorageKey, payload, image.MimeType); err != nil {
			return uploaded, skipped, fmt.Errorf("upload Ocha image %s: %w", image.AssetName, err)
		}
		uploaded++
		if uploaded%50 == 0 {
			log.Printf("  ... %d uploaded", uploaded)
		}
	}
	return uploaded, skipped, nil
}

func replacePOSAccountsTx(ctx context.Context, tx *sql.Tx, roleID string, passwordHash string) error {
	branchIDs := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT code, id::text FROM branches WHERE code IN ('MES','PHH','PHS','NPT','KNP','WH')`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var code, id string
		if err := rows.Scan(&code, &id); err != nil {
			rows.Close()
			return err
		}
		branchIDs[code] = id
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, account := range ochaPOSAccounts {
		branchID, ok := branchIDs[account.BranchCode]
		if !ok {
			return fmt.Errorf("missing branch %s for POS account", account.BranchCode)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO users (id, role_id, branch_id, full_name, email, password_hash, active, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,TRUE,NOW(),NOW())
		`, platform.MustUUID(), roleID, branchID, account.FullName, account.Email, passwordHash); err != nil {
			return fmt.Errorf("insert POS account %s: %w", account.Email, err)
		}
	}

	return nil
}

func seedOchaOpeningLedgerTx(ctx context.Context, tx *sql.Tx, actorID string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (
			id, branch_id, product_id, movement_type, stock_bucket, quantity_delta,
			reference_type, note, performed_by, created_at
		)
		SELECT gen_random_uuid(), i.branch_id, i.product_id, 'opening_balance', bucket.stock_bucket,
		       bucket.quantity, 'ocha_seed.opening_balance', 'ยอดตั้งต้นจากข้อมูล Ocha POS', $1, NOW()
		FROM inventory i
		CROSS JOIN LATERAL (VALUES ('real'::text, i.qty_real), ('ghost'::text, i.qty_ghost)) bucket(stock_bucket, quantity)
		WHERE bucket.quantity > 0
	`, actorID); err != nil {
		return fmt.Errorf("insert Ocha opening movements: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_movement_lots (
			id, inventory_movement_id, inventory_lot_id, quantity_delta, created_at
		)
		SELECT gen_random_uuid(), movement.id, lot.id, lot.remaining_quantity, NOW()
		FROM inventory_movements movement
		INNER JOIN inventory_lots lot
		  ON lot.branch_id=movement.branch_id AND lot.product_id=movement.product_id
		 AND lot.stock_bucket=movement.stock_bucket
		WHERE movement.reference_type='ocha_seed.opening_balance'
		  AND lot.source_type IN ('ocha_seed_opening','warehouse_bootstrap_copy')
	`); err != nil {
		return fmt.Errorf("link Ocha opening movements to lots: %w", err)
	}
	return nil
}

func seedDefaultAppSettingsTx(ctx context.Context, tx *sql.Tx, cfg config.Config) error {
	settings := []struct {
		Key      string
		Value    string
		Metadata map[string]any
	}{
		{"vat_rate", fmt.Sprintf("%.2f", cfg.DefaultVATPct), map[string]any{"label": "อัตราภาษีมูลค่าเพิ่ม", "type": "number"}},
		{"company_name", "ระบบบริหารร้านขายยา PharmaPOS", map[string]any{"label": "ชื่อกิจการ"}},
		{"company_tax_id", "0105559999999", map[string]any{"label": "เลขประจำตัวผู้เสียภาษี"}},
		{"company_address", "", map[string]any{"label": "ที่อยู่กิจการ"}},
	}
	for _, setting := range settings {
		metadata, _ := json.Marshal(setting.Metadata)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_settings (setting_key, setting_value, metadata, created_at, updated_at)
			VALUES ($1,$2,$3::jsonb,NOW(),NOW())
			ON CONFLICT (setting_key) DO NOTHING
		`, setting.Key, setting.Value, string(metadata)); err != nil {
			return err
		}
	}
	return nil
}

func verifyOchaCatalogTx(ctx context.Context, tx *sql.Tx, manifest OchaCatalogManifest) error {
	checks := []struct {
		Label    string
		Query    string
		Expected int
	}{
		{"branches", `SELECT COUNT(*) FROM branches`, len(manifest.Branches) + 1},
		{"categories", `SELECT COUNT(*) FROM product_categories`, len(manifest.ProductCategories)},
		{"products", `SELECT COUNT(*) FROM products`, len(manifest.Products)},
		{"inventory", `SELECT COUNT(*) FROM inventory`, len(manifest.Inventory)},
		{"POS accounts", `SELECT COUNT(*) FROM users u INNER JOIN roles r ON r.id=u.role_id WHERE r.role_key='branch_pos'`, len(ochaPOSAccounts)},
		{"roles", `SELECT COUNT(*) FROM roles`, 3},
		{"user accounts", `SELECT COUNT(*) FROM users`, len(ochaPOSAccounts) + 2},
	}
	for _, check := range checks {
		var count int
		if err := tx.QueryRowContext(ctx, check.Query).Scan(&count); err != nil {
			return err
		}
		if count != check.Expected {
			return fmt.Errorf("Ocha catalog verification failed for %s: expected %d, got %d", check.Label, check.Expected, count)
		}
	}
	var inventoryMismatches, ledgerMismatches int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM inventory i
		WHERE i.qty_real <> COALESCE((SELECT SUM(l.remaining_quantity) FROM inventory_lots l WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='real'),0)
		   OR i.qty_ghost <> COALESCE((SELECT SUM(l.remaining_quantity) FROM inventory_lots l WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='ghost'),0)
	`).Scan(&inventoryMismatches); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM inventory i
		WHERE i.qty_real <> COALESCE((SELECT SUM(m.quantity_delta) FROM inventory_movements m WHERE m.branch_id=i.branch_id AND m.product_id=i.product_id AND m.stock_bucket='real'),0)
		   OR i.qty_ghost <> COALESCE((SELECT SUM(m.quantity_delta) FROM inventory_movements m WHERE m.branch_id=i.branch_id AND m.product_id=i.product_id AND m.stock_bucket='ghost'),0)
	`).Scan(&ledgerMismatches); err != nil {
		return err
	}
	if inventoryMismatches != 0 || ledgerMismatches != 0 {
		return fmt.Errorf("Ocha inventory verification failed: lot=%d ledger=%d", inventoryMismatches, ledgerMismatches)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO app_settings (setting_key,setting_value,metadata,created_at,updated_at)
		VALUES ('catalog_seed_version',$1,$2::jsonb,NOW(),NOW())
		ON CONFLICT (setting_key) DO UPDATE SET setting_value=EXCLUDED.setting_value,metadata=EXCLUDED.metadata,updated_at=NOW()
	`, fmt.Sprintf("%d", manifest.SchemaVersion), fmt.Sprintf(`{"source":"ocha_mhtml","products":%d,"inventory":%d,"random_seed":%q}`, len(manifest.Products), len(manifest.Inventory), manifest.RandomSeed)); err != nil {
		return err
	}
	return nil
}

func normalizedPOSPassword(cfg config.Config) (string, error) {
	password := strings.TrimSpace(cfg.SeedPOSPassword)
	if password == "" {
		password = "DevPassword123!"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
