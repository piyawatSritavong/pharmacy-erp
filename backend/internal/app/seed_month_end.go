package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/platform"

	"github.com/lib/pq"
)

type monthEndSeedProduct struct {
	SKU     string
	Name    string
	Selling string
	Cost    string
	Real    int
	Ghost   int
}

var monthEndSeedProducts = []monthEndSeedProduct{
	{"A380-05", "A380-05", "13500.00", "9800.00", 30, 50},
	{"380-02", "380-02", "11900.00", "8200.00", 30, 50},
	{"C919-01", "C919-01", "6500.00", "3800.00", 30, 50},
	{"C919-02", "C919-02", "6500.00", "3650.00", 30, 50},
	{"C919-05", "C919-05", "7500.00", "3990.00", 30, 50},
	{"VC-887", "VC-887", "12500.00", "8888.00", 30, 50},
	{"VC-865M02", "VC-865M02", "11900.00", "7890.00", 30, 50},
	{"VC 976-07", "VC 976-07", "11900.00", "7765.00", 30, 50},
	{"VC 976-08", "VC 976-08", "10900.00", "6543.00", 30, 50},
	{"ไม้เท้า 02", "ไม้เท้า 02", "490.00", "300.00", 30, 50},
	{"ไม้เท้า 03", "ไม้เท้า 03", "490.00", "300.00", 30, 50},
	{"ไฟฟ้า 09", "ไฟฟ้า 09", "25900.00", "17900.00", 30, 50},
}

// SeedMonthEnd resets only the explicitly listed month-end demo products. It
// is guarded, backed up in app_settings, and transactional by design.
func SeedMonthEnd(ctx context.Context, db *sql.DB, cfg config.Config) error {
	environment := strings.ToLower(strings.TrimSpace(cfg.AppEnv))
	if environment != "development" && environment != "test" {
		return fmt.Errorf("seed-month-end is allowed only in development or test")
	}
	if !cfg.AllowDestructiveSeed {
		return fmt.Errorf("set ALLOW_DESTRUCTIVE_SEED=true to acknowledge the scoped inventory reset")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var branchID, actorID string
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE active=TRUE ORDER BY CASE WHEN branch_type='main_warehouse' THEN 0 ELSE 1 END, created_at LIMIT 1`).Scan(&branchID); err != nil {
		return fmt.Errorf("find main warehouse: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local' AND active=TRUE LIMIT 1`).Scan(&actorID); err != nil {
		return fmt.Errorf("find superadmin: %w", err)
	}

	skus := make([]string, 0, len(monthEndSeedProducts))
	for _, product := range monthEndSeedProducts {
		skus = append(skus, product.SKU)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT p.sku,p.name,p.cost_price::text,p.base_selling_price::text,
		       COALESCE(i.qty_real,0),COALESCE(i.qty_ghost,0)
		FROM products p LEFT JOIN inventory i ON i.product_id=p.id AND i.branch_id=$1
		WHERE p.sku = ANY($2)
		ORDER BY p.sku
	`, branchID, pq.Array(skus))
	if err != nil {
		return err
	}
	backup := []map[string]any{}
	for rows.Next() {
		var sku, name, cost, selling string
		var real, ghost int
		if err := rows.Scan(&sku, &name, &cost, &selling, &real, &ghost); err != nil {
			rows.Close()
			return err
		}
		backup = append(backup, map[string]any{"sku": sku, "name": name, "cost_price": cost, "selling_price": selling, "qty_real": real, "qty_ghost": ghost})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	backupJSON, err := json.Marshal(map[string]any{"created_at": time.Now().UTC(), "branch_id": branchID, "rows": backup})
	if err != nil {
		return err
	}
	backupKey := "month_end_seed_backup_" + time.Now().UTC().Format("20060102T150405.000000000")
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_settings(setting_key,setting_value,metadata,created_at,updated_at) VALUES($1,$2,$3::jsonb,NOW(),NOW())`, backupKey, "seed-month-end pre-reset backup", string(backupJSON)); err != nil {
		return err
	}

	for _, product := range monthEndSeedProducts {
		productID := platform.MustUUID()
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO products(id,sku,name,description,cost_price,base_selling_price,unit_name,tax_exempt,active,created_at,updated_at)
			VALUES($1,$2,$3,'ข้อมูลทดสอบสรุปสิ้นเดือน',$4::numeric,$5::numeric,'ชิ้น',FALSE,TRUE,NOW(),NOW())
			ON CONFLICT(sku) DO UPDATE SET name=EXCLUDED.name,cost_price=EXCLUDED.cost_price,base_selling_price=EXCLUDED.base_selling_price,active=TRUE,updated_at=NOW()
			RETURNING id::text
		`, productID, product.SKU, product.Name, product.Cost, product.Selling).Scan(&productID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory(id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at)
			SELECT gen_random_uuid(),branch.id,$1,0,0,NOW(),NOW()
			FROM branches branch WHERE branch.active=TRUE
			ON CONFLICT(branch_id,product_id) DO NOTHING
		`, productID); err != nil {
			return err
		}
	}
	// Use the same relational seed path as a fresh installation: each shortage
	// is a posted PO with an item, lot, movement, and movement-lot link. Ghost
	// is created only at WH; every active branch receives Real.
	if _, err := seedInventoryFloorTx(ctx, tx, inventorySeedMinimum); err != nil {
		return err
	}
	return tx.Commit()
}
