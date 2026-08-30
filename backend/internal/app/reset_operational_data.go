package app

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/platform"
)

const operationalResetConfirmation = "RESET-OPERATIONAL-DATA"

type ResetOperationalDataResult struct {
	DryRun           bool
	DeletedRows      map[string]int64
	ProductCount     int64
	InventoryRows    int64
	RealQuantity     int64
	GhostQuantity    int64
	OpeningMovements int64
}

var operationalTables = []string{
	"inventory_reclassifications",
	"month_end_adjustments",
	"month_end_calculation_runs",
	"month_end_workpaper_lines",
	"month_end_workpapers",
	"inventory_movement_lots",
	"transfer_item_lot_allocations",
	"invoice_payments",
	"invoice_items",
	"invoices",
	"quotation_items",
	"quotations",
	"transfer_events",
	"transfer_items",
	"stock_transfer_requests",
	"transfers",
	"purchase_order_events",
	"purchase_order_items",
	"purchase_orders",
	"marketplace_order_items",
	"marketplace_orders",
	"marketplace_connections",
	"inventory_lots",
	"audit_logs",
	"inventory_movements",
}

func ResetOperationalData(ctx context.Context, db *sql.DB, cfg config.Config, confirmation string) (ResetOperationalDataResult, error) {
	result := ResetOperationalDataResult{DryRun: confirmation != operationalResetConfirmation, DeletedRows: map[string]int64{}}
	if !result.DryRun && !cfg.AllowOperationalDataReset {
		return result, fmt.Errorf("ALLOW_OPERATIONAL_DATA_RESET=true is required")
	}
	if err := loadOperationalResetSnapshot(ctx, db, &result); err != nil {
		return result, err
	}
	if result.DryRun {
		return result, nil
	}

	err := platform.WithTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(867530901)`); err != nil {
			return err
		}
		for _, table := range operationalTables {
			deleted, err := tx.ExecContext(ctx, "DELETE FROM "+table)
			if err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
			count, err := deleted.RowsAffected()
			if err != nil {
				return err
			}
			result.DeletedRows[table] = count
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE document_sequences
			SET next_number = 1, is_locked = FALSE, updated_at = NOW()
		`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_lots (
				id, branch_id, product_id, stock_bucket, lot_number, expires_on,
				received_quantity, remaining_quantity, unit_cost, source_type,
				source_id, source_item_id, received_at, created_at, updated_at
			)
			SELECT gen_random_uuid(), i.branch_id, i.product_id, bucket.stock_bucket,
			       'RESET-' || UPPER(SUBSTRING(REPLACE(i.id::text, '-', '') FROM 1 FOR 10)), NULL,
			       bucket.quantity, bucket.quantity, p.cost_price, 'operational_reset',
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
		if _, err := tx.ExecContext(ctx, `
			WITH actor AS (
				SELECT u.id
				FROM users u
				INNER JOIN roles r ON r.id = u.role_id
				WHERE r.role_key = 'super_admin' AND u.active = TRUE
				ORDER BY u.created_at, u.id
				LIMIT 1
			), opening AS (
				SELECT i.branch_id, i.product_id, bucket.stock_bucket,
				       CASE bucket.stock_bucket WHEN 'real' THEN i.qty_real ELSE i.qty_ghost END AS quantity
				FROM inventory i
				CROSS JOIN (VALUES ('real'), ('ghost')) AS bucket(stock_bucket)
			)
			INSERT INTO inventory_movements (
				id, branch_id, product_id, movement_type, stock_bucket, quantity_delta,
				reference_type, note, performed_by, created_at
			)
			SELECT gen_random_uuid(), opening.branch_id, opening.product_id, 'opening_balance',
			       opening.stock_bucket, opening.quantity, 'operational_reset',
			       'ยกยอดคงเหลือหลังล้างข้อมูลธุรกรรม', actor.id, NOW()
			FROM opening
			LEFT JOIN actor ON TRUE
			WHERE opening.quantity <> 0
		`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movement_lots (
				id, inventory_movement_id, inventory_lot_id, quantity_delta, created_at
			)
			SELECT gen_random_uuid(), m.id, l.id, m.quantity_delta, NOW()
			FROM inventory_movements m
			INNER JOIN inventory_lots l
			  ON l.branch_id=m.branch_id AND l.product_id=m.product_id
			 AND l.stock_bucket=m.stock_bucket AND l.source_type='operational_reset'
			WHERE m.reference_type='operational_reset'
		`); err != nil {
			return err
		}

		var mismatches int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM inventory i
			WHERE i.qty_real <> COALESCE((
				SELECT SUM(m.quantity_delta) FROM inventory_movements m
				WHERE m.branch_id = i.branch_id AND m.product_id = i.product_id AND m.stock_bucket = 'real'
			), 0)
			OR i.qty_ghost <> COALESCE((
				SELECT SUM(m.quantity_delta) FROM inventory_movements m
				WHERE m.branch_id = i.branch_id AND m.product_id = i.product_id AND m.stock_bucket = 'ghost'
			), 0)
		`).Scan(&mismatches); err != nil {
			return err
		}
		if mismatches != 0 {
			return fmt.Errorf("inventory ledger verification failed: %d mismatches", mismatches)
		}
		return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM inventory_movements`).Scan(&result.OpeningMovements)
	})
	return result, err
}

func loadOperationalResetSnapshot(ctx context.Context, db *sql.DB, result *ResetOperationalDataResult) error {
	for _, table := range operationalTables {
		var count int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}
		result.DeletedRows[table] = count
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products`).Scan(&result.ProductCount); err != nil {
		return err
	}
	return db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(qty_real), 0), COALESCE(SUM(qty_ghost), 0)
		FROM inventory
	`).Scan(&result.InventoryRows, &result.RealQuantity, &result.GhostQuantity)
}

func (r ResetOperationalDataResult) Lines() []string {
	lines := []string{
		fmt.Sprintf("products=%d", r.ProductCount),
		fmt.Sprintf("inventory_rows=%d real=%d ghost=%d", r.InventoryRows, r.RealQuantity, r.GhostQuantity),
	}
	names := make([]string, 0, len(r.DeletedRows))
	for name := range r.DeletedRows {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s=%d", name, r.DeletedRows[name]))
	}
	if !r.DryRun {
		lines = append(lines, fmt.Sprintf("opening_movements=%d", r.OpeningMovements))
	}
	return lines
}
