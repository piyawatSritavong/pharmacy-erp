package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/config"
	"pharmacy-erp/backend/internal/platform"
)

const inventorySeedMinimum = 10

type SeedInventoryFloorResult struct {
	MinimumQuantity    int
	PurchaseOrders     int
	PurchaseOrderItems int
	UnitsReceived      int
	RepairedBalances   int
}

func (result SeedInventoryFloorResult) Lines() []string {
	return []string{
		fmt.Sprintf("minimum Real per product/branch and Ghost per WH product: %d", result.MinimumQuantity),
		fmt.Sprintf("posted purchase orders: %d", result.PurchaseOrders),
		fmt.Sprintf("purchase order items/lots/movements: %d", result.PurchaseOrderItems),
		fmt.Sprintf("received units: %d", result.UnitsReceived),
		fmt.Sprintf("legacy inventory relationships repaired: %d", result.RepairedBalances),
	}
}

// SeedInventoryFloor non-destructively raises every active product in every
// active branch to the requested Real minimum and every active product at WH
// to the requested Ghost minimum. New quantities always enter through a posted
// purchase order and receive matching PO items, lots, inventory movements,
// movement-lot links, and aggregate inventory.
func SeedInventoryFloor(ctx context.Context, db *sql.DB, cfg config.Config) (SeedInventoryFloorResult, error) {
	result := SeedInventoryFloorResult{MinimumQuantity: inventorySeedMinimum}
	environment := strings.ToLower(strings.TrimSpace(cfg.AppEnv))
	if environment != "development" && environment != "test" {
		return result, fmt.Errorf("seed-inventory-floor is allowed only in development or test")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err = seedInventoryFloorTx(ctx, tx, inventorySeedMinimum)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func seedInventoryFloorTx(ctx context.Context, tx *sql.Tx, minimum int) (SeedInventoryFloorResult, error) {
	result := SeedInventoryFloorResult{MinimumQuantity: minimum}
	if minimum < 1 {
		return result, fmt.Errorf("inventory seed minimum must be positive")
	}

	var actorID string
	if err := tx.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local' AND active=TRUE LIMIT 1`).Scan(&actorID); err != nil {
		return result, fmt.Errorf("find superadmin for inventory seed: %w", err)
	}

	var supplierID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO suppliers (
			id,supplier_code,legal_name,tax_id,company_branch_type,company_branch_number,
			address_line,subdistrict,district,province,postal_code,contact_name,phone,email,
			payment_terms_days,notes,active,created_at,updated_at
		) VALUES (
			$1,'SEED-STOCK','บริษัททดสอบรับสินค้าเข้าสต๊อก',NULL,'head_office','',
			'ข้อมูลสำหรับทดสอบระบบ','','','','','','','',0,
			'สร้างโดย seed-inventory-floor',TRUE,NOW(),NOW()
		)
		ON CONFLICT (supplier_code) DO UPDATE
		SET active=TRUE,notes=EXCLUDED.notes,updated_at=NOW()
		RETURNING id::text
	`, platform.MustUUID()).Scan(&supplierID); err != nil {
		return result, fmt.Errorf("upsert inventory seed supplier: %w", err)
	}

	repaired, err := repairSeedInventoryRelationshipsTx(ctx, tx, actorID)
	if err != nil {
		return result, err
	}
	result.RepairedBalances = repaired

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory (id,branch_id,product_id,qty_real,qty_ghost,created_at,updated_at)
		SELECT gen_random_uuid(),b.id,p.id,0,0,NOW(),NOW()
		FROM branches b CROSS JOIN products p
		WHERE b.active=TRUE AND p.active=TRUE
		ON CONFLICT (branch_id,product_id) DO NOTHING
	`); err != nil {
		return result, fmt.Errorf("create missing product/branch inventory memberships: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		CREATE TEMP TABLE seed_inventory_floor_lines (
			item_id UUID PRIMARY KEY,
			lot_id UUID NOT NULL,
			movement_id UUID NOT NULL,
			purchase_order_id UUID,
			branch_id UUID NOT NULL,
			product_id UUID NOT NULL,
			product_sku TEXT NOT NULL,
			product_name TEXT NOT NULL,
			unit_name TEXT NOT NULL,
			stock_bucket TEXT NOT NULL,
			quantity INTEGER NOT NULL,
			unit_cost NUMERIC(12,2) NOT NULL
		) ON COMMIT DROP
	`); err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO seed_inventory_floor_lines (
			item_id,lot_id,movement_id,branch_id,product_id,product_sku,product_name,
			unit_name,stock_bucket,quantity,unit_cost
		)
		SELECT gen_random_uuid(),gen_random_uuid(),gen_random_uuid(),i.branch_id,i.product_id,
		       p.sku,p.name,p.unit_name,bucket.stock_bucket,$1-bucket.quantity,p.cost_price
		FROM inventory i
		INNER JOIN branches b ON b.id=i.branch_id AND b.active=TRUE
		INNER JOIN products p ON p.id=i.product_id AND p.active=TRUE
		CROSS JOIN LATERAL (
			VALUES
				('real'::text,COALESCE((SELECT SUM(l.remaining_quantity) FROM inventory_lots l WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='real' AND (l.expires_on IS NULL OR l.expires_on>=CURRENT_DATE)),0)::integer),
				('ghost'::text,LEAST(
					i.qty_ghost,
					COALESCE((SELECT SUM(l.remaining_quantity) FROM inventory_lots l WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='ghost' AND (l.expires_on IS NULL OR l.expires_on>=CURRENT_DATE)),0)::integer
				))
		) bucket(stock_bucket,quantity)
		WHERE bucket.quantity < $1
		  AND (bucket.stock_bucket='real' OR b.branch_type='main_warehouse')
	`, minimum); err != nil {
		return result, fmt.Errorf("calculate inventory floor shortages: %w", err)
	}

	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(quantity),0) FROM seed_inventory_floor_lines`).Scan(&result.PurchaseOrderItems, &result.UnitsReceived); err != nil {
		return result, err
	}
	branchRows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT l.branch_id::text,b.code
		FROM seed_inventory_floor_lines l INNER JOIN branches b ON b.id=l.branch_id
		ORDER BY b.code
	`)
	if err != nil {
		return result, err
	}
	type seedBranch struct{ id, code string }
	branches := []seedBranch{}
	for branchRows.Next() {
		var branch seedBranch
		if err := branchRows.Scan(&branch.id, &branch.code); err != nil {
			branchRows.Close()
			return result, err
		}
		branches = append(branches, branch)
	}
	if err := branchRows.Close(); err != nil {
		return result, err
	}

	purchasedAt := time.Now().UTC()
	for _, branch := range branches {
		poID := platform.MustUUID()
		poNumber, err := nextSeedPONumberTx(ctx, tx, branch.id, purchasedAt)
		if err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE seed_inventory_floor_lines SET purchase_order_id=$2 WHERE branch_id=$1`, branch.id, poID); err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_orders (
				id,branch_id,supplier_id,po_number,status,revision,purchased_at,due_date,posted_at,
				supplier_document_number,supplier_code_snapshot,supplier_name_snapshot,
				supplier_tax_id_snapshot,supplier_address_snapshot,supplier_contact_snapshot,
				supplier_phone_snapshot,supplier_email_snapshot,job_name,delivery_terms,
				vat_mode,vat_rate,subtotal,line_discount_total,header_discount,shipping_amount,
				tax_amount,total_amount,notes,created_by,updated_by,created_at,updated_at
			)
			SELECT $1,$2,$3,$4,'posted',1,$5::timestamptz,$5::timestamptz::date + 30,NOW(),$6,'SEED-STOCK',
			       'บริษัททดสอบรับสินค้าเข้าสต๊อก','','ข้อมูลสำหรับทดสอบระบบ','','','',
			       'เติมสต๊อกขั้นต่ำสำหรับทดสอบ','รับเข้าสต๊อกสาขา','none',0,
			       COALESCE(SUM(quantity*unit_cost),0),0,0,0,0,COALESCE(SUM(quantity*unit_cost),0),
			       'Seed เติม Real ทุกสาขาและ Ghost เฉพาะ WH ให้มีอย่างน้อย 10 ชิ้น',$7,$7,NOW(),NOW()
			FROM seed_inventory_floor_lines WHERE branch_id=$2
		`, poID, branch.id, supplierID, poNumber, purchasedAt, "SEED-"+purchasedAt.Format("20060102")+"-"+branch.code, actorID); err != nil {
			return result, fmt.Errorf("insert seed purchase order %s: %w", branch.code, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_order_items (
				id,purchase_order_id,product_id,product_sku_snapshot,product_name_snapshot,
				unit_name_snapshot,stock_bucket,received_quantity,unit_cost,line_discount,
				line_subtotal,tax_amount,line_total,lot_number,expires_on,created_at,updated_at
			)
			SELECT item_id,purchase_order_id,product_id,product_sku,product_name,unit_name,
			       stock_bucket,quantity,unit_cost,0,quantity*unit_cost,0,quantity*unit_cost,
			       'SEED-'||UPPER(LEFT(REPLACE(lot_id::text,'-',''),12)),CURRENT_DATE+730,NOW(),NOW()
			FROM seed_inventory_floor_lines WHERE branch_id=$1
		`, branch.id); err != nil {
			return result, fmt.Errorf("insert seed purchase order items %s: %w", branch.code, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_lots (
				id,branch_id,product_id,stock_bucket,lot_number,expires_on,received_quantity,
				remaining_quantity,unit_cost,source_type,source_id,source_item_id,received_at,created_at,updated_at
			)
			SELECT lot_id,branch_id,product_id,stock_bucket,
			       'SEED-'||UPPER(LEFT(REPLACE(lot_id::text,'-',''),12)),CURRENT_DATE+730,
			       quantity,quantity,unit_cost,'purchase_order',purchase_order_id,item_id,$2,NOW(),NOW()
			FROM seed_inventory_floor_lines WHERE branch_id=$1
		`, branch.id, purchasedAt); err != nil {
			return result, fmt.Errorf("insert seed inventory lots %s: %w", branch.code, err)
		}
		if _, err := tx.ExecContext(ctx, `
			WITH additions AS (
				SELECT branch_id,product_id,
				       COALESCE(SUM(quantity) FILTER (WHERE stock_bucket='real'),0) real_quantity,
				       COALESCE(SUM(quantity) FILTER (WHERE stock_bucket='ghost'),0) ghost_quantity
				FROM seed_inventory_floor_lines WHERE branch_id=$1 GROUP BY branch_id,product_id
			)
			UPDATE inventory i
			SET qty_real=i.qty_real+a.real_quantity,qty_ghost=i.qty_ghost+a.ghost_quantity,updated_at=NOW()
			FROM additions a WHERE i.branch_id=a.branch_id AND i.product_id=a.product_id
		`, branch.id); err != nil {
			return result, fmt.Errorf("update seed inventory aggregates %s: %w", branch.code, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movements (
				id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,
				reference_type,reference_id,note,performed_by,created_at
			)
			SELECT movement_id,branch_id,product_id,'purchase_receive',stock_bucket,quantity,
			       'purchase_order',purchase_order_id,$2,$3,NOW()
			FROM seed_inventory_floor_lines WHERE branch_id=$1
		`, branch.id, poNumber, actorID); err != nil {
			return result, fmt.Errorf("insert seed inventory movements %s: %w", branch.code, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movement_lots (
				id,inventory_movement_id,inventory_lot_id,quantity_delta,created_at
			)
			SELECT gen_random_uuid(),movement_id,lot_id,quantity,NOW()
			FROM seed_inventory_floor_lines WHERE branch_id=$1
		`, branch.id); err != nil {
			return result, fmt.Errorf("link seed movements to lots %s: %w", branch.code, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_order_events (id,purchase_order_id,event_type,revision,note,actor_id,event_at)
			VALUES (gen_random_uuid(),$1,'posted',1,'Seed เติมสต๊อกขั้นต่ำสำหรับทดสอบสรุปสิ้นเดือน',$2,NOW())
		`, poID, actorID); err != nil {
			return result, err
		}
		result.PurchaseOrders++
	}

	if err := verifyInventoryFloorTx(ctx, tx, minimum); err != nil {
		return result, err
	}
	return result, nil
}

func nextSeedPONumberTx(ctx context.Context, tx *sql.Tx, branchID string, purchasedAt time.Time) (string, error) {
	var branchCode, prefix string
	var next int64
	if err := tx.QueryRowContext(ctx, `SELECT code FROM branches WHERE id=$1`, branchID).Scan(&branchCode); err != nil {
		return "", err
	}
	err := tx.QueryRowContext(ctx, `
		SELECT prefix,next_number FROM document_sequences
		WHERE branch_id=$1 AND doc_type='purchase_order' FOR UPDATE
	`, branchID).Scan(&prefix, &next)
	if err == sql.ErrNoRows {
		prefix, next = "PO", 1
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_sequences (id,branch_id,doc_type,prefix,next_number,is_locked,created_at,updated_at)
			VALUES ($1,$2,'purchase_order',$3,1,FALSE,NOW(),NOW())
		`, platform.MustUUID(), branchID, prefix); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE document_sequences SET next_number=$2,updated_at=NOW() WHERE branch_id=$1 AND doc_type='purchase_order'`, branchID, next+1); err != nil {
		return "", err
	}
	return platform.FormatBranchDocumentNumber(branchCode, prefix, purchasedAt, next), nil
}

func repairSeedInventoryRelationshipsTx(ctx context.Context, tx *sql.Tx, actorID string) (int, error) {
	if _, err := tx.ExecContext(ctx, `
		CREATE TEMP TABLE seed_inventory_repairs ON COMMIT DROP AS
		WITH balances AS (
			SELECT i.branch_id,i.product_id,bucket.stock_bucket,bucket.inventory_quantity,
			       COALESCE((SELECT SUM(l.remaining_quantity) FROM inventory_lots l
			                 WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket=bucket.stock_bucket),0)::integer physical_lot_quantity,
			       CASE WHEN bucket.stock_bucket='ghost' THEN COALESCE((
			           SELECT SUM(deficit.quantity) FROM inventory_ghost_deficits deficit
			           WHERE deficit.branch_id=i.branch_id AND deficit.product_id=i.product_id
			       ),0)::integer ELSE 0 END deficit_quantity,
			       COALESCE((SELECT SUM(m.quantity_delta) FROM inventory_movements m
			                 WHERE m.branch_id=i.branch_id AND m.product_id=i.product_id AND m.stock_bucket=bucket.stock_bucket),0)::integer movement_quantity
			FROM inventory i
			INNER JOIN branches b ON b.id=i.branch_id
			CROSS JOIN LATERAL (VALUES ('real'::text,i.qty_real),('ghost'::text,i.qty_ghost)) bucket(stock_bucket,inventory_quantity)
			WHERE bucket.stock_bucket='real' OR b.branch_type='main_warehouse'
		), desired AS (
			SELECT *,physical_lot_quantity-deficit_quantity lot_quantity,
			       GREATEST(inventory_quantity,physical_lot_quantity-deficit_quantity,movement_quantity) desired_quantity
			FROM balances
		)
		SELECT gen_random_uuid() repair_lot_id,gen_random_uuid() repair_movement_id,
		       branch_id,product_id,stock_bucket,
		       desired_quantity-inventory_quantity inventory_addition,
		       desired_quantity+deficit_quantity-physical_lot_quantity lot_addition,
		       desired_quantity-movement_quantity movement_addition
		FROM desired
		WHERE inventory_quantity<>lot_quantity OR inventory_quantity<>movement_quantity
	`); err != nil {
		return 0, fmt.Errorf("calculate legacy inventory relationship repairs: %w", err)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM seed_inventory_repairs`).Scan(&count); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE inventory i
		SET qty_real=i.qty_real+CASE WHEN r.stock_bucket='real' THEN r.inventory_addition ELSE 0 END,
		    qty_ghost=i.qty_ghost+CASE WHEN r.stock_bucket='ghost' THEN r.inventory_addition ELSE 0 END,
		    updated_at=NOW()
		FROM seed_inventory_repairs r
		WHERE i.branch_id=r.branch_id AND i.product_id=r.product_id AND r.inventory_addition>0
	`); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_lots (
			id,branch_id,product_id,stock_bucket,lot_number,expires_on,received_quantity,
			remaining_quantity,unit_cost,source_type,received_at,created_at,updated_at
		)
		SELECT r.repair_lot_id,r.branch_id,r.product_id,r.stock_bucket,
		       'SEED-REPAIR-'||UPPER(LEFT(REPLACE(r.repair_lot_id::text,'-',''),8)),CURRENT_DATE+730,
		       r.lot_addition,r.lot_addition,p.cost_price,'seed_relationship_repair',NOW(),NOW(),NOW()
		FROM seed_inventory_repairs r INNER JOIN products p ON p.id=r.product_id
		WHERE r.lot_addition>0
	`); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_movements (
			id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,
			reference_type,note,performed_by,created_at
		)
		SELECT repair_movement_id,branch_id,product_id,'stock_reconciliation',stock_bucket,
		       movement_addition,'seed_relationship_repair','ซ่อมความสัมพันธ์ seed รุ่นเดิม',$1,NOW()
		FROM seed_inventory_repairs WHERE movement_addition>0
	`, actorID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_movement_lots (id,inventory_movement_id,inventory_lot_id,quantity_delta,created_at)
		SELECT gen_random_uuid(),r.repair_movement_id,
		       COALESCE(CASE WHEN r.lot_addition>0 THEN r.repair_lot_id END,existing_lot.id),
		       r.movement_addition,NOW()
		FROM seed_inventory_repairs r
		LEFT JOIN LATERAL (
			SELECT l.id FROM inventory_lots l
			WHERE l.branch_id=r.branch_id AND l.product_id=r.product_id AND l.stock_bucket=r.stock_bucket AND l.remaining_quantity>0
			ORDER BY l.received_at,l.id LIMIT 1
		) existing_lot ON TRUE
		WHERE r.movement_addition>0
		  AND COALESCE(CASE WHEN r.lot_addition>0 THEN r.repair_lot_id END,existing_lot.id) IS NOT NULL
		ON CONFLICT DO NOTHING
	`); err != nil {
		return 0, err
	}
	return count, nil
}

func verifyInventoryFloorTx(ctx context.Context, tx *sql.Tx, minimum int) error {
	var belowFloor, lotMismatches, ledgerMismatches, relationshipMismatches, nonWarehouseGhost int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM branches b CROSS JOIN products p
		LEFT JOIN inventory i ON i.branch_id=b.id AND i.product_id=p.id
		WHERE b.active=TRUE AND p.active=TRUE
		  AND (
			i.id IS NULL
			OR COALESCE((SELECT SUM(l.remaining_quantity) FROM inventory_lots l WHERE l.branch_id=b.id AND l.product_id=p.id AND l.stock_bucket='real' AND (l.expires_on IS NULL OR l.expires_on>=CURRENT_DATE)),0)<$1
			OR (b.branch_type='main_warehouse' AND (
				i.qty_ghost<$1 OR
				COALESCE((SELECT SUM(l.remaining_quantity) FROM inventory_lots l WHERE l.branch_id=b.id AND l.product_id=p.id AND l.stock_bucket='ghost' AND (l.expires_on IS NULL OR l.expires_on>=CURRENT_DATE)),0)<$1
			))
		  )
	`, minimum).Scan(&belowFloor); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM inventory i
		WHERE i.qty_real<>COALESCE((SELECT SUM(l.remaining_quantity) FROM inventory_lots l WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='real'),0)
		   OR i.qty_ghost<>COALESCE((SELECT SUM(l.remaining_quantity) FROM inventory_lots l WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='ghost'),0)
		                    -COALESCE((SELECT SUM(d.quantity) FROM inventory_ghost_deficits d WHERE d.branch_id=i.branch_id AND d.product_id=i.product_id),0)
	`).Scan(&lotMismatches); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM inventory i
		WHERE i.qty_real<>COALESCE((SELECT SUM(m.quantity_delta) FROM inventory_movements m WHERE m.branch_id=i.branch_id AND m.product_id=i.product_id AND m.stock_bucket='real'),0)
		   OR i.qty_ghost<>COALESCE((SELECT SUM(m.quantity_delta) FROM inventory_movements m WHERE m.branch_id=i.branch_id AND m.product_id=i.product_id AND m.stock_bucket='ghost'),0)
	`).Scan(&ledgerMismatches); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM branches b LEFT JOIN inventory i ON i.branch_id=b.id
		WHERE b.branch_type<>'main_warehouse' AND (
			COALESCE(i.qty_ghost,0)<>0 OR
			EXISTS(SELECT 1 FROM inventory_lots l WHERE l.branch_id=b.id AND l.stock_bucket='ghost') OR
			EXISTS(SELECT 1 FROM inventory_movements m WHERE m.branch_id=b.id AND m.stock_bucket='ghost')
		)
	`).Scan(&nonWarehouseGhost); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM seed_inventory_floor_lines l
		LEFT JOIN purchase_order_items poi ON poi.id=l.item_id AND poi.purchase_order_id=l.purchase_order_id
		LEFT JOIN inventory_lots lot ON lot.id=l.lot_id AND lot.source_id=l.purchase_order_id AND lot.source_item_id=l.item_id
		LEFT JOIN inventory_movements movement ON movement.id=l.movement_id AND movement.reference_id=l.purchase_order_id
		LEFT JOIN inventory_movement_lots link ON link.inventory_movement_id=l.movement_id AND link.inventory_lot_id=l.lot_id
		WHERE poi.id IS NULL OR lot.id IS NULL OR movement.id IS NULL OR link.id IS NULL
	`).Scan(&relationshipMismatches); err != nil {
		return err
	}
	if belowFloor != 0 || lotMismatches != 0 || ledgerMismatches != 0 || relationshipMismatches != 0 || nonWarehouseGhost != 0 {
		return fmt.Errorf("inventory floor verification failed: below=%d lot=%d ledger=%d relationship=%d non_wh_ghost=%d", belowFloor, lotMismatches, ledgerMismatches, relationshipMismatches, nonWarehouseGhost)
	}
	return nil
}
