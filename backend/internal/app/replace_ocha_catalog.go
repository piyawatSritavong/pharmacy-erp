package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/platform"
)

const replaceOchaCatalogConfirmation = "REPLACE-OCHA-CATALOG"

type ReplaceOchaCatalogResult struct {
	DryRun      bool
	Before      map[string]int64
	DeletedRows map[string]int64
	Branches    int
	Categories  int
	Products    int
	Inventory   int
	Lots        int
	POSAccounts int
}

func ReplaceOchaCatalog(ctx context.Context, db *sql.DB, cfg config.Config, confirmation string) (ReplaceOchaCatalogResult, error) {
	result := ReplaceOchaCatalogResult{
		DryRun:      confirmation != replaceOchaCatalogConfirmation,
		Before:      map[string]int64{},
		DeletedRows: map[string]int64{},
	}
	if !result.DryRun && !cfg.AllowMasterDataReset {
		return result, fmt.Errorf("ALLOW_MASTER_DATA_RESET=true is required")
	}
	manifest, err := LoadOchaCatalog()
	if err != nil {
		return result, err
	}
	result.Branches = len(manifest.Branches)
	result.Categories = len(manifest.ProductCategories)
	result.Products = len(manifest.Products)
	result.Inventory = len(manifest.Inventory)
	result.Lots = len(manifest.InventoryLots)
	result.POSAccounts = len(ochaPOSAccounts)
	for _, table := range []string{"branches", "products", "product_categories", "inventory", "invoices", "quotations", "transfers", "purchase_orders", "report_definitions", "suppliers", "users"} {
		var count int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return result, fmt.Errorf("count %s before Ocha replacement: %w", table, err)
		}
		result.Before[table] = count
	}
	if result.DryRun {
		return result, nil
	}
	passwordHash, err := normalizedPOSPassword(cfg)
	if err != nil {
		return result, fmt.Errorf("hash POS seed password: %w", err)
	}
	err = platform.WithTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(867530902)`); err != nil {
			return err
		}
		var actorID, branchPOSRoleID string
		if err := tx.QueryRowContext(ctx, `
			SELECT u.id::text FROM users u
			INNER JOIN roles r ON r.id=u.role_id
			WHERE r.role_key='super_admin' AND u.active=TRUE
			ORDER BY u.created_at,u.id LIMIT 1
		`).Scan(&actorID); err != nil {
			return fmt.Errorf("active Super Admin is required before catalog replacement: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT id::text FROM roles WHERE role_key='branch_pos'`).Scan(&branchPOSRoleID); err != nil {
			return fmt.Errorf("branch_pos role is required before catalog replacement: %w", err)
		}

		for _, table := range operationalTables {
			deleted, err := tx.ExecContext(ctx, "DELETE FROM "+table)
			if err != nil {
				return fmt.Errorf("clear %s for Ocha replacement: %w", table, err)
			}
			count, err := deleted.RowsAffected()
			if err != nil {
				return err
			}
			result.DeletedRows[table] = count
		}
		deletedUsers, err := tx.ExecContext(ctx, `
			DELETE FROM users WHERE role_id=(SELECT id FROM roles WHERE role_key='branch_pos')
		`)
		if err != nil {
			return fmt.Errorf("clear old POS accounts: %w", err)
		}
		result.DeletedRows["branch_pos_users"], _ = deletedUsers.RowsAffected()

		for _, table := range []string{"inventory", "product_aliases", "branch_product_settings", "branch_product_prices", "document_sequences", "products", "product_categories", "branches"} {
			deleted, err := tx.ExecContext(ctx, "DELETE FROM "+table)
			if err != nil {
				return fmt.Errorf("clear %s for Ocha replacement: %w", table, err)
			}
			result.DeletedRows[table], _ = deleted.RowsAffected()
		}
		if err := insertOchaMasterDataTx(ctx, tx, manifest); err != nil {
			return err
		}
		if err := replacePOSAccountsTx(ctx, tx, branchPOSRoleID, passwordHash); err != nil {
			return err
		}
		if err := seedOchaOpeningLedgerTx(ctx, tx, actorID); err != nil {
			return err
		}
		if err := verifyOchaCatalogTx(ctx, tx, manifest); err != nil {
			return err
		}
		afterData, _ := json.Marshal(map[string]any{
			"schema_version": manifest.SchemaVersion,
			"branches":       len(manifest.Branches), "categories": len(manifest.ProductCategories),
			"products": len(manifest.Products), "inventory": len(manifest.Inventory),
			"source_records": manifest.Summary.SourceRecords,
		})
		_, err = tx.ExecContext(ctx, `
			INSERT INTO audit_logs (
				id,actor_id,branch_id,entity_type,entity_id,action,before_data,after_data,
				request_id,source_ip,user_agent,created_at
			) VALUES (gen_random_uuid(),$1,NULL,'catalog_import',NULL,'catalog.import.replace',$2::jsonb,$3::jsonb,'catalog-cli','local','replace-ocha-catalog',NOW())
		`, actorID, mustJSON(result.Before), string(afterData))
		return err
	})
	return result, err
}

func mustJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func (result ReplaceOchaCatalogResult) Lines() []string {
	lines := []string{fmt.Sprintf("mode=%s", map[bool]string{true: "dry-run", false: "replace"}[result.DryRun])}
	beforeNames := make([]string, 0, len(result.Before))
	for name := range result.Before {
		beforeNames = append(beforeNames, name)
	}
	sort.Strings(beforeNames)
	for _, name := range beforeNames {
		lines = append(lines, fmt.Sprintf("before.%s=%d", name, result.Before[name]))
	}
	if !result.DryRun {
		deletedNames := make([]string, 0, len(result.DeletedRows))
		for name := range result.DeletedRows {
			deletedNames = append(deletedNames, name)
		}
		sort.Strings(deletedNames)
		for _, name := range deletedNames {
			lines = append(lines, fmt.Sprintf("deleted.%s=%d", name, result.DeletedRows[name]))
		}
	}
	lines = append(lines,
		fmt.Sprintf("target.branches=%d", result.Branches),
		fmt.Sprintf("target.categories=%d", result.Categories),
		fmt.Sprintf("target.products=%d", result.Products),
		fmt.Sprintf("target.inventory=%d", result.Inventory),
		fmt.Sprintf("target.lots=%d", result.Lots),
		fmt.Sprintf("target.pos_accounts=%d", result.POSAccounts),
	)
	return lines
}
