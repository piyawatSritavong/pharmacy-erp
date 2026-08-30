package stocklot

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"
)

type Lot struct {
	ID                string
	BranchID          string
	ProductID         string
	StockBucket       string
	LotNumber         string
	ExpiresOn         sql.NullTime
	ReceivedQuantity  int
	RemainingQuantity int
	UnitCost          float64
	ReceivedAt        time.Time
}

type Allocation struct {
	Lot      Lot
	Quantity int
}

func ValidateBucket(bucket string) error {
	if bucket != "real" && bucket != "ghost" {
		return platform.NewError(http.StatusBadRequest, "invalid stock bucket")
	}
	return nil
}

func Create(ctx context.Context, tx *sql.Tx, lot Lot, sourceType string, sourceID, sourceItemID, originLotID *string) (string, error) {
	if err := ValidateBucket(lot.StockBucket); err != nil {
		return "", err
	}
	if lot.ReceivedQuantity <= 0 || lot.RemainingQuantity < 0 || lot.RemainingQuantity > lot.ReceivedQuantity {
		return "", platform.NewError(http.StatusBadRequest, "จำนวน lot ไม่ถูกต้อง")
	}
	id := lot.ID
	if strings.TrimSpace(id) == "" {
		id = platform.MustUUID()
	}
	lotNumber := strings.TrimSpace(lot.LotNumber)
	if lotNumber == "" {
		lotNumber = platform.GenerateReadableCode("LOT")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_lots (
			id, branch_id, product_id, stock_bucket, lot_number, expires_on,
			received_quantity, remaining_quantity, unit_cost, source_type,
			source_id, source_item_id, origin_lot_id, received_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW(),NOW())
	`, id, lot.BranchID, lot.ProductID, lot.StockBucket, lotNumber, lot.ExpiresOn,
		lot.ReceivedQuantity, lot.RemainingQuantity, platform.Round2(lot.UnitCost), sourceType,
		platform.NullUUID(sourceID), platform.NullUUID(sourceItemID), platform.NullUUID(originLotID), resolveReceivedAt(lot.ReceivedAt))
	return id, err
}

func resolveReceivedAt(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

// AllocateFEFO locks and decrements usable lots. Expired lots are deliberately
// excluded from normal sales and transfer dispatches.
func AllocateFEFO(ctx context.Context, tx *sql.Tx, branchID, productID, bucket string, quantity int) ([]Allocation, error) {
	if err := ValidateBucket(bucket); err != nil {
		return nil, err
	}
	if quantity <= 0 {
		return nil, platform.NewError(http.StatusBadRequest, "quantity must be greater than zero")
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id::text, branch_id::text, product_id::text, stock_bucket, lot_number,
		       expires_on, received_quantity, remaining_quantity, unit_cost, received_at
		FROM inventory_lots
		WHERE branch_id = $1 AND product_id = $2 AND stock_bucket = $3
		  AND remaining_quantity > 0
		  AND (expires_on IS NULL OR expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)
		ORDER BY expires_on ASC NULLS LAST, received_at ASC, id ASC
		FOR UPDATE
	`, branchID, productID, bucket)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	remaining := quantity
	allocations := []Allocation{}
	for rows.Next() {
		var lot Lot
		if err := rows.Scan(&lot.ID, &lot.BranchID, &lot.ProductID, &lot.StockBucket, &lot.LotNumber,
			&lot.ExpiresOn, &lot.ReceivedQuantity, &lot.RemainingQuantity, &lot.UnitCost, &lot.ReceivedAt); err != nil {
			return nil, err
		}
		// Continue consuming the cursor after the requested quantity is covered.
		// PostgreSQL cannot execute the decrement statements on this transaction
		// while unread rows are still streaming on its single connection.
		if remaining <= 0 {
			continue
		}
		allocated := lot.RemainingQuantity
		if allocated > remaining {
			allocated = remaining
		}
		allocations = append(allocations, Allocation{Lot: lot, Quantity: allocated})
		remaining -= allocated
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if remaining > 0 {
		return nil, platform.NewError(http.StatusConflict, fmt.Sprintf("สต๊อกที่ขายได้ไม่เพียงพอ (ขาด %d ชิ้น หรือมีสินค้าเลยวันหมดอายุ)", remaining))
	}
	for _, allocation := range allocations {
		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory_lots
			SET remaining_quantity = remaining_quantity - $2, updated_at = NOW()
			WHERE id = $1
		`, allocation.Lot.ID, allocation.Quantity); err != nil {
			return nil, err
		}
	}
	return allocations, nil
}

func AttachMovement(ctx context.Context, tx *sql.Tx, movementID string, allocations []Allocation, sign int) error {
	for _, allocation := range allocations {
		delta := allocation.Quantity * sign
		if delta == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_movement_lots (
				id, inventory_movement_id, inventory_lot_id, quantity_delta, created_at
			) VALUES ($1,$2,$3,$4,NOW())
		`, platform.MustUUID(), movementID, allocation.Lot.ID, delta); err != nil {
			return err
		}
	}
	return nil
}

func RestoreAllocations(ctx context.Context, tx *sql.Tx, allocations []Allocation) error {
	for _, allocation := range allocations {
		if _, err := tx.ExecContext(ctx, `
			UPDATE inventory_lots
			SET remaining_quantity = remaining_quantity + $2, updated_at = NOW()
			WHERE id = $1 AND remaining_quantity + $2 <= received_quantity
		`, allocation.Lot.ID, allocation.Quantity); err != nil {
			return err
		}
	}
	return nil
}

func LoadMovementAllocations(ctx context.Context, tx *sql.Tx, movementID string) ([]Allocation, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT il.id::text, il.branch_id::text, il.product_id::text, il.stock_bucket,
		       il.lot_number, il.expires_on, il.received_quantity, il.remaining_quantity,
		       il.unit_cost, il.received_at, ABS(iml.quantity_delta)
		FROM inventory_movement_lots iml
		INNER JOIN inventory_lots il ON il.id = iml.inventory_lot_id
		WHERE iml.inventory_movement_id = $1
		ORDER BY il.expires_on ASC NULLS LAST, il.received_at, il.id
	`, movementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Allocation{}
	for rows.Next() {
		var item Allocation
		if err := rows.Scan(&item.Lot.ID, &item.Lot.BranchID, &item.Lot.ProductID, &item.Lot.StockBucket,
			&item.Lot.LotNumber, &item.Lot.ExpiresOn, &item.Lot.ReceivedQuantity,
			&item.Lot.RemainingQuantity, &item.Lot.UnitCost, &item.Lot.ReceivedAt, &item.Quantity); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func WeightedCost(allocations []Allocation) float64 {
	var total float64
	var quantity int
	for _, allocation := range allocations {
		total += allocation.Lot.UnitCost * float64(allocation.Quantity)
		quantity += allocation.Quantity
	}
	if quantity == 0 {
		return 0
	}
	return platform.Round2(total / float64(quantity))
}
