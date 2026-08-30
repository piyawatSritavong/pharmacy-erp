package monthend

import (
	"context"
	"database/sql"

	"pharmacy-erp/backend/internal/modules/stocklot"
)

func reclassifyLots(ctx context.Context, tx *sql.Tx, branchID, productID, fromBucket, toBucket string, quantity int, outMovementID, inMovementID string) error {
	allocations, err := stocklot.AllocateFEFO(ctx, tx, branchID, productID, fromBucket, quantity)
	if err != nil {
		return err
	}
	if err := stocklot.AttachMovement(ctx, tx, outMovementID, allocations, -1); err != nil {
		return err
	}
	destination := []stocklot.Allocation{}
	for _, allocation := range allocations {
		originID := allocation.Lot.ID
		lotID, err := stocklot.Create(ctx, tx, stocklot.Lot{BranchID: branchID, ProductID: productID, StockBucket: toBucket, LotNumber: allocation.Lot.LotNumber, ExpiresOn: allocation.Lot.ExpiresOn, ReceivedQuantity: allocation.Quantity, RemainingQuantity: allocation.Quantity, UnitCost: allocation.Lot.UnitCost}, "month_end_reclassification", nil, nil, &originID)
		if err != nil {
			return err
		}
		destination = append(destination, stocklot.Allocation{Lot: stocklot.Lot{ID: lotID}, Quantity: allocation.Quantity})
	}
	return stocklot.AttachMovement(ctx, tx, inMovementID, destination, 1)
}
