package sales

import (
	"context"
	"os"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/products"
	"pharmacy-erp/backend/internal/modules/purchasing"
	"pharmacy-erp/backend/internal/platform"
)

func TestSalesFEFOExpiryAndInvoiceRestoreAgainstConfiguredDatabase(t *testing.T) {
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
	var userID, branchID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE active=TRUE AND sales_enabled=TRUE ORDER BY created_at,id LIMIT 1`).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	user := platform.AuthUser{ID: userID, RoleKey: "super_admin", Portal: "backoffice", Scope: "global", Permissions: []string{"price.override.global", "quotation.manage"}}
	meta := audit.LogEntry{ActorID: &userID}
	auditService := audit.NewService(db)
	purchaseService := purchasing.NewService(db, auditService)
	unique := platform.MustUUID()
	supplierID, err := purchaseService.CreateSupplier(ctx, meta, purchasing.SupplierInput{SupplierCode: "SALE-" + unique[:8], LegalName: "Sales Lot Integration " + unique})
	if err != nil {
		t.Fatal(err)
	}
	firstPO, err := purchaseService.CreatePurchaseOrder(ctx, user, meta, purchasing.PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, PurchasedAt: time.Now().UTC().Format(time.RFC3339), VATMode: "none",
		Items: []purchasing.PurchaseOrderLineInput{{NewProduct: &purchasing.NewProductInput{SKU: "FEFO-" + unique[:8], Name: "FEFO integration product", UnitName: "ชิ้น", BaseSellingPrice: 100, MaxDiscountAmount: 20, TracksExpiry: true, ExpiryWarningDays: 30}, StockBucket: "real", Quantity: 2, UnitCost: 50, LotNumber: "EARLY-" + unique[:8], ExpiresOn: time.Now().AddDate(0, 2, 0).Format("2006-01-02")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstDetail, err := purchaseService.GetPurchaseOrder(ctx, user, firstPO)
	if err != nil {
		t.Fatal(err)
	}
	productID := firstDetail["items"].([]map[string]any)[0]["product_id"].(string)
	var warehouseMemberships int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM inventory i
		INNER JOIN branches b ON b.id=i.branch_id
		WHERE b.branch_type='main_warehouse' AND b.active=TRUE
		  AND i.product_id=$1 AND i.qty_real=0 AND i.qty_ghost=0
	`, productID).Scan(&warehouseMemberships); err != nil {
		t.Fatal(err)
	}
	if warehouseMemberships != 1 {
		t.Fatalf("expected a retail PO inline product to join the warehouse catalog at zero stock, got %d memberships", warehouseMemberships)
	}
	_, err = purchaseService.CreatePurchaseOrder(ctx, user, meta, purchasing.PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, PurchasedAt: time.Now().UTC().Format(time.RFC3339), VATMode: "none",
		Items: []purchasing.PurchaseOrderLineInput{{ProductID: productID, StockBucket: "real", Quantity: 3, UnitCost: 55, LotNumber: "LATE-" + unique[:8], ExpiresOn: time.Now().AddDate(0, 5, 0).Format("2006-01-02")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	salesService := NewService(db, auditService)
	tooLow := 70.0
	allowedFloor := 80.0
	posUser := platform.AuthUser{ID: userID, RoleKey: "branch_pos", Portal: "pos", Scope: "branch", BranchID: &branchID, Permissions: []string{"price.override.pos", "invoice.create.pos"}}
	if _, err := salesService.Preview(ctx, posUser, branchID, false, []LineInput{{ProductID: productID, Quantity: 1, StockBucket: "real", OverrideUnitPrice: &tooLow}}, 0); err == nil {
		t.Fatal("expected POS discount below the effective floor to be rejected")
	}
	if _, err := salesService.Preview(ctx, posUser, branchID, false, []LineInput{{ProductID: productID, Quantity: 1, StockBucket: "real", OverrideUnitPrice: &allowedFloor}}, 0); err != nil {
		t.Fatalf("expected POS discount at the floor to pass: %v", err)
	}
	adminOverride := user
	adminOverride.Permissions = []string{"price.override.global"}
	if _, err := salesService.Preview(ctx, adminOverride, branchID, false, []LineInput{{ProductID: productID, Quantity: 1, StockBucket: "real", OverrideUnitPrice: &tooLow}}, 0); err == nil {
		t.Fatal("expected super admin override below the floor to require a reason")
	}
	if _, err := salesService.Preview(ctx, adminOverride, branchID, false, []LineInput{{ProductID: productID, Quantity: 1, StockBucket: "real", OverrideUnitPrice: &tooLow, OverrideReason: "approved integration exception"}}, 0); err != nil {
		t.Fatalf("expected reasoned super admin override to pass: %v", err)
	}
	var earlyLotID, lateLotID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM inventory_lots WHERE product_id=$1 AND branch_id=$2 AND lot_number=$3`, productID, branchID, "EARLY-"+unique[:8]).Scan(&earlyLotID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM inventory_lots WHERE product_id=$1 AND branch_id=$2 AND lot_number=$3`, productID, branchID, "LATE-"+unique[:8]).Scan(&lateLotID); err != nil {
		t.Fatal(err)
	}
	posOptions, err := salesService.LotOptions(ctx, posUser, branchID, productID, "real")
	if err != nil || len(posOptions) != 2 {
		t.Fatalf("load POS lot options: count=%d err=%v", len(posOptions), err)
	}
	for _, key := range []string{"remaining_quantity", "unit_cost", "source_type", "po_number", "supplier_name"} {
		if _, exposed := posOptions[0][key]; exposed {
			t.Fatalf("POS lot option exposed restricted field %q", key)
		}
	}
	adminOptions, err := salesService.LotOptions(ctx, user, branchID, productID, "real")
	if err != nil || len(adminOptions) != 2 {
		t.Fatalf("load admin lot options: count=%d err=%v", len(adminOptions), err)
	}
	for _, key := range []string{"remaining_quantity", "unit_cost", "source_type", "po_number", "supplier_name"} {
		if _, visible := adminOptions[0][key]; !visible {
			t.Fatalf("admin lot option is missing field %q", key)
		}
	}

	productService := products.NewService(db, auditService, t.TempDir())
	settings, err := productService.GetBranchSettings(ctx, user, productID, branchID)
	if err != nil {
		t.Fatal(err)
	}
	if settings["selling_price"] != nil || settings["effective_selling_price"].(float64) != 100 || settings["selling_price_source"] != "warehouse" {
		t.Fatalf("expected live warehouse price inheritance, got %#v", settings)
	}
	overridePrice := 125.0
	if err := productService.UpdateBranchSettings(ctx, user, productID, branchID, meta, products.BranchSettingsInput{SellingPrice: products.OptionalFloat{Present: true, Value: &overridePrice}}); err != nil {
		t.Fatalf("set branch price override: %v", err)
	}
	settings, err = productService.GetBranchSettings(ctx, user, productID, branchID)
	if err != nil || settings["effective_selling_price"].(float64) != 125 || settings["selling_price_source"] != "branch_override" {
		t.Fatalf("expected branch price override, got %#v err=%v", settings, err)
	}
	if err := productService.UpdateBranchSettings(ctx, user, productID, branchID, meta, products.BranchSettingsInput{SellingPrice: products.OptionalFloat{Present: true}}); err != nil {
		t.Fatalf("reset branch price override: %v", err)
	}
	settings, err = productService.GetBranchSettings(ctx, user, productID, branchID)
	if err != nil || settings["effective_selling_price"].(float64) != 100 || settings["selling_price_source"] != "warehouse" {
		t.Fatalf("expected reset to warehouse price, got %#v err=%v", settings, err)
	}
	var warehouseID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE branch_type='main_warehouse' AND active=TRUE`).Scan(&warehouseID); err != nil {
		t.Fatal(err)
	}
	if err := productService.UpdateBranchSettings(ctx, user, productID, warehouseID, meta, products.BranchSettingsInput{SellingPrice: products.OptionalFloat{Present: true, Value: &overridePrice}}); err == nil {
		t.Fatal("expected warehouse branch price override to be rejected")
	}
	if _, err := salesService.CreateInvoice(ctx, user, meta, InvoiceRequest{BranchID: warehouseID, CustomerName: "warehouse sale must fail", Items: []LineInput{{ProductID: productID, InventoryLotID: earlyLotID, Quantity: 1, StockBucket: "real"}}}); err == nil {
		t.Fatal("expected all warehouse sales to be rejected")
	}
	invoiceID, err := salesService.CreateInvoice(ctx, user, meta, InvoiceRequest{BranchID: branchID, CustomerName: "Lot selection test", Items: []LineInput{
		{ProductID: productID, InventoryLotID: earlyLotID, Quantity: 2, StockBucket: "real"},
		{ProductID: productID, InventoryLotID: lateLotID, Quantity: 1, StockBucket: "real"},
	}})
	if err != nil {
		t.Fatalf("create selected-lot invoice: %v", err)
	}
	var earlyRemaining, lateRemaining int
	if err := db.QueryRowContext(ctx, `SELECT remaining_quantity FROM inventory_lots WHERE product_id=$1 AND lot_number=$2`, productID, "EARLY-"+unique[:8]).Scan(&earlyRemaining); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT remaining_quantity FROM inventory_lots WHERE product_id=$1 AND lot_number=$2`, productID, "LATE-"+unique[:8]).Scan(&lateRemaining); err != nil {
		t.Fatal(err)
	}
	if earlyRemaining != 0 || lateRemaining != 2 {
		t.Fatalf("expected selected lot allocation 0/2, got %d/%d", earlyRemaining, lateRemaining)
	}
	var distinctCosts int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT cost_snapshot) FROM invoice_items WHERE invoice_id=$1`, invoiceID).Scan(&distinctCosts); err != nil {
		t.Fatal(err)
	}
	if distinctCosts != 2 {
		t.Fatalf("expected separate lot costs on invoice lines, got %d", distinctCosts)
	}
	if err := salesService.DeleteInvoice(ctx, user, meta, invoiceID, ""); err != nil {
		t.Fatalf("delete invoice and restore lots: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT remaining_quantity FROM inventory_lots WHERE product_id=$1 AND lot_number=$2`, productID, "EARLY-"+unique[:8]).Scan(&earlyRemaining); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT remaining_quantity FROM inventory_lots WHERE product_id=$1 AND lot_number=$2`, productID, "LATE-"+unique[:8]).Scan(&lateRemaining); err != nil {
		t.Fatal(err)
	}
	if earlyRemaining != 2 || lateRemaining != 3 {
		t.Fatalf("expected original lots restored to 2/3, got %d/%d", earlyRemaining, lateRemaining)
	}

	quotationID, err := salesService.CreateQuotation(ctx, user, meta, QuoteRequest{BranchID: branchID, CustomerName: "Split lots quotation", Items: []LineInput{{ProductID: productID, Quantity: 3, StockBucket: "real"}}})
	if err != nil {
		t.Fatalf("create quotation without reserving a lot: %v", err)
	}
	quotation, err := salesService.GetQuotation(ctx, user, quotationID)
	if err != nil {
		t.Fatal(err)
	}
	quotationItems := quotation["items"].([]map[string]any)
	quotationItemID := quotationItems[0]["id"].(string)
	convertedInvoiceID, err := salesService.ConvertQuotation(ctx, user, meta, quotationID, ConvertQuotationRequest{Allocations: []QuotationLotAllocation{
		{QuotationItemID: quotationItemID, InventoryLotID: earlyLotID, Quantity: 1},
		{QuotationItemID: quotationItemID, InventoryLotID: lateLotID, Quantity: 2},
	}})
	if err != nil {
		t.Fatalf("convert quotation with split lot allocations: %v", err)
	}
	var convertedLines, convertedLots int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(DISTINCT inventory_lot_id) FROM invoice_items WHERE invoice_id=$1`, convertedInvoiceID).Scan(&convertedLines, &convertedLots); err != nil {
		t.Fatal(err)
	}
	if convertedLines != 2 || convertedLots != 2 {
		t.Fatalf("expected two invoice lines for two selected lots, got lines=%d lots=%d", convertedLines, convertedLots)
	}
	if err := salesService.DeleteInvoice(ctx, user, meta, convertedInvoiceID, ""); err != nil {
		t.Fatalf("delete converted invoice and restore selected lots: %v", err)
	}

	type concurrentSaleResult struct {
		invoiceID string
		err       error
	}
	startConcurrentSales := make(chan struct{})
	concurrentResults := make(chan concurrentSaleResult, 2)
	for range 2 {
		go func() {
			<-startConcurrentSales
			id, saleErr := salesService.CreateInvoice(ctx, user, meta, InvoiceRequest{BranchID: branchID, CustomerName: "concurrent selected lot", Items: []LineInput{{ProductID: productID, InventoryLotID: lateLotID, Quantity: 2, StockBucket: "real"}}})
			concurrentResults <- concurrentSaleResult{invoiceID: id, err: saleErr}
		}()
	}
	close(startConcurrentSales)
	successfulConcurrentInvoices := []string{}
	failedConcurrentSales := 0
	for range 2 {
		result := <-concurrentResults
		if result.err != nil {
			failedConcurrentSales++
			continue
		}
		successfulConcurrentInvoices = append(successfulConcurrentInvoices, result.invoiceID)
	}
	if len(successfulConcurrentInvoices) != 1 || failedConcurrentSales != 1 {
		t.Fatalf("expected one concurrent selected-lot sale to succeed and one to fail, got success=%d failed=%d", len(successfulConcurrentInvoices), failedConcurrentSales)
	}
	if err := db.QueryRowContext(ctx, `SELECT remaining_quantity FROM inventory_lots WHERE id=$1`, lateLotID).Scan(&lateRemaining); err != nil {
		t.Fatal(err)
	}
	if lateRemaining != 1 {
		t.Fatalf("expected selected lot to retain one item after concurrent sales, got %d", lateRemaining)
	}
	if err := salesService.DeleteInvoice(ctx, user, meta, successfulConcurrentInvoices[0], ""); err != nil {
		t.Fatalf("delete successful concurrent invoice: %v", err)
	}

	expiredPO, err := purchaseService.CreatePurchaseOrder(ctx, user, meta, purchasing.PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, PurchasedAt: "2025-01-01T10:00:00+07:00", VATMode: "none",
		Items: []purchasing.PurchaseOrderLineInput{{NewProduct: &purchasing.NewProductInput{SKU: "EXP-" + unique[:8], Name: "Expired integration product", UnitName: "ชิ้น", BaseSellingPrice: 20, TracksExpiry: true}, StockBucket: "real", Quantity: 1, UnitCost: 10, LotNumber: "EXPIRED-" + unique[:8], ExpiresOn: "2025-02-01"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	expiredDetail, err := purchaseService.GetPurchaseOrder(ctx, user, expiredPO)
	if err != nil {
		t.Fatal(err)
	}
	expiredProductID := expiredDetail["items"].([]map[string]any)[0]["product_id"].(string)
	var expiredLotID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM inventory_lots WHERE product_id=$1 AND branch_id=$2`, expiredProductID, branchID).Scan(&expiredLotID); err != nil {
		t.Fatal(err)
	}
	if _, err := salesService.CreateInvoice(ctx, user, meta, InvoiceRequest{BranchID: branchID, CustomerName: "expired test", Items: []LineInput{{ProductID: expiredProductID, InventoryLotID: expiredLotID, Quantity: 1, StockBucket: "real"}}}); err == nil {
		t.Fatal("expected an expired-only lot to be unavailable for sale")
	}
}
