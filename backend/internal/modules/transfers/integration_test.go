package transfers

import (
	"context"
	"os"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/purchasing"
	"pharmacy-erp/backend/internal/platform"
)

func TestTransferPreservesPurchaseLotAgainstConfiguredDatabase(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.Open(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	var userID, sourceBranchID, destinationBranchID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE code='WH' AND active=TRUE`).Scan(&sourceBranchID); err != nil {
		t.Fatalf("load central warehouse: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE active=TRUE AND sales_enabled=TRUE ORDER BY code LIMIT 1`).Scan(&destinationBranchID); err != nil {
		t.Fatalf("load retail destination branch: %v", err)
	}
	auditService := audit.NewService(db)
	user := platform.AuthUser{ID: userID, RoleKey: "super_admin", Portal: "backoffice", Scope: "global", Permissions: []string{"transfer.request", "transfer.dispatch", "transfer.approve", "transfer.receive"}}
	meta := audit.LogEntry{ActorID: &userID}
	unique := platform.MustUUID()
	purchaseService := purchasing.NewService(db, auditService)
	supplierID, err := purchaseService.CreateSupplier(ctx, meta, purchasing.SupplierInput{SupplierCode: "TRF-" + unique[:8], LegalName: "Transfer Integration " + unique})
	if err != nil {
		t.Fatal(err)
	}
	poID, err := purchaseService.CreatePurchaseOrder(ctx, user, meta, purchasing.PurchaseOrderInput{
		BranchID: sourceBranchID, SupplierID: supplierID, PurchasedAt: time.Now().UTC().Format(time.RFC3339), VATMode: "none",
		Items: []purchasing.PurchaseOrderLineInput{{NewProduct: &purchasing.NewProductInput{SKU: "TRF-PROD-" + unique[:8], Name: "Warehouse transfer integration " + unique, UnitName: "ชิ้น", BaseSellingPrice: 84, TracksExpiry: true}, StockBucket: "real", Quantity: 2, UnitCost: 42, LotNumber: "TRANSFER-" + unique[:8], ExpiresOn: time.Now().AddDate(1, 0, 0).Format("2006-01-02")}},
	})
	if err != nil {
		t.Fatalf("create source PO: %v", err)
	}
	poDetail, err := purchaseService.GetPurchaseOrder(ctx, user, poID)
	if err != nil {
		t.Fatalf("load source PO: %v", err)
	}
	productID := poDetail["items"].([]map[string]any)[0]["product_id"].(string)
	transferService := NewService(db, auditService)
	transferID, err := transferService.Create(ctx, user, meta, CreateTransferRequest{SourceBranchID: sourceBranchID, DestinationBranchID: destinationBranchID, RequestNote: "lot integration", Items: []TransferLine{{ProductID: productID, Quantity: 1, StockBucket: "real"}}})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}
	if err := transferService.Dispatch(ctx, user, meta, transferID, DispatchRequest{PickupName: "Integration"}); err != nil {
		t.Fatalf("dispatch transfer: %v", err)
	}
	var itemID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM transfer_items WHERE transfer_id=$1`, transferID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := transferService.Receive(ctx, user, meta, transferID, ReceiveRequest{Items: []ReceiveLineRequest{{ItemID: itemID, ReceivedQuantity: 1}}}); err != nil {
		t.Fatalf("receive transfer: %v", err)
	}
	var sourceRemaining, destinationRemaining int
	var sourceLotNumber, destinationLotNumber, sourceExpiry, destinationExpiry, originLotID, sourceLotID string
	if err := db.QueryRowContext(ctx, `SELECT id::text,lot_number,expires_on::text,remaining_quantity FROM inventory_lots WHERE source_id=$1 AND source_type='purchase_order'`, poID).Scan(&sourceLotID, &sourceLotNumber, &sourceExpiry, &sourceRemaining); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT lot_number,expires_on::text,remaining_quantity,origin_lot_id::text FROM inventory_lots WHERE source_id=$1 AND source_type='transfer'`, transferID).Scan(&destinationLotNumber, &destinationExpiry, &destinationRemaining, &originLotID); err != nil {
		t.Fatal(err)
	}
	if sourceRemaining != 1 || destinationRemaining != 1 || sourceLotNumber != destinationLotNumber || sourceExpiry != destinationExpiry || originLotID != sourceLotID {
		t.Fatalf("lot lineage was not preserved: source=%s/%s/%d destination=%s/%s/%d origin=%s", sourceLotNumber, sourceExpiry, sourceRemaining, destinationLotNumber, destinationExpiry, destinationRemaining, originLotID)
	}
}
