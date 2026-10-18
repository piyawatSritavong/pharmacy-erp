package sales

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/customers"
	"pharmacy-erp/backend/internal/modules/products"
	"pharmacy-erp/backend/internal/modules/purchasing"
	"pharmacy-erp/backend/internal/platform"
)

// Member levels buy at price levels: a walk-in pays the shelf price, a member
// pays the level their member level maps to, never more than a lower level,
// and a pack falls back to the base unit's level price times its size.
func TestMemberLevelPricesAgainstConfiguredDatabase(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	db, err := database.OpenTest(databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	var userID, branchID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE active AND sales_enabled ORDER BY created_at, id LIMIT 1`).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	admin := platform.AuthUser{ID: userID, RoleKey: "super_admin", Portal: "backoffice", Scope: "global",
		Permissions: []string{"customer.credit.manage", "customer.level.manage", "price_tier.manage"}}
	till := platform.AuthUser{ID: userID, RoleKey: "branch_pos", Portal: "pos", Scope: "branch", BranchID: &branchID,
		Permissions: []string{"customer.manage"}}
	meta := audit.LogEntry{ActorID: &userID}
	auditService := audit.NewService(db)
	purchaseService := purchasing.NewService(db, auditService)
	productService := products.NewService(db, auditService, nil)
	customerService := customers.NewService(db, auditService)
	salesService := NewService(db, auditService)

	unique := platform.MustUUID()
	supplierID, err := purchaseService.CreateSupplier(ctx, meta, purchasing.SupplierInput{SupplierCode: "LVL-" + unique[:8], LegalName: "Level " + unique})
	if err != nil {
		t.Fatal(err)
	}
	poID, err := purchaseService.CreatePurchaseOrder(ctx, admin, meta, purchasing.PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, PurchasedAt: time.Now().UTC().Format(time.RFC3339), VATMode: "none",
		Items: []purchasing.PurchaseOrderLineInput{{NewProduct: &purchasing.NewProductInput{SKU: "LVL-" + unique[:8], Name: "Level product",
			UnitName: "ชิ้น", BaseSellingPrice: 100}, StockBucket: "real", Quantity: 100, UnitCost: 50, LotNumber: "LVL-" + unique[:8]}},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := purchaseService.GetPurchaseOrder(ctx, admin, poID)
	if err != nil {
		t.Fatal(err)
	}
	productID := detail["items"].([]map[string]any)[0]["product_id"].(string)
	var lotID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM inventory_lots WHERE product_id=$1 AND branch_id=$2`, productID, branchID).Scan(&lotID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM branch_product_prices WHERE product_id=$1`, productID); err != nil {
		t.Fatal(err)
	}
	if err := productService.SaveUnits(ctx, productID, meta, []products.UnitInput{
		{UnitName: "ชิ้น", ConversionQty: 1, IsBase: true},
		{UnitName: "แพ็ค", ConversionQty: 10},
	}); err != nil {
		t.Fatal(err)
	}
	var packUnitID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM product_units WHERE product_id=$1 AND NOT is_base`, productID).Scan(&packUnitID); err != nil {
		t.Fatal(err)
	}
	// Level 2 = 95, level 3 = 90, level 4 dearer than level 3 (a typing slip
	// the till must not pass on), level 5 = 80. All on the base unit.
	if err := productService.SaveLevelPrices(ctx, admin, productID, meta, []products.LevelPriceInput{
		{PriceLevel: 2, UnitPrice: 95}, {PriceLevel: 3, UnitPrice: 90}, {PriceLevel: 4, UnitPrice: 92}, {PriceLevel: 5, UnitPrice: 80},
	}); err != nil {
		t.Fatal(err)
	}

	// The till may register a member at level 1 only.
	phone := func() string { return fmt.Sprintf("07%08d", time.Now().UnixNano()%100000000) }
	two := 2
	if _, err := customerService.Create(ctx, till, meta, customers.Input{Name: "Till raised", Phone: phone(), MemberLevel: &two}); err == nil {
		t.Fatal("a till must not register a member above level 1")
	}
	members := map[int]string{}
	for level := 1; level <= 4; level++ {
		value := level
		created, err := customerService.Create(ctx, admin, meta, customers.Input{Name: fmt.Sprintf("Level %d", level), Phone: phone(), MemberLevel: &value})
		if err != nil {
			t.Fatal(err)
		}
		members[level] = created["id"].(string)
	}
	one := 1
	if err := customerService.Update(ctx, till, meta, members[2], customers.Input{Name: "Level 2", Phone: "", MemberLevel: &one}); err == nil {
		t.Fatal("a till must not change a member's level")
	}

	price := func(customerID, unitID string) (float64, string) {
		t.Helper()
		preview, err := salesService.PreviewSale(ctx, admin, branchID, false,
			[]LineInput{{ProductID: productID, InventoryLotID: lotID, Quantity: 1, UnitID: unitID, StockBucket: "real"}}, 0,
			cartOptions{CustomerID: customerID})
		if err != nil {
			t.Fatal(err)
		}
		lines := preview["lines"].([]map[string]any)
		return preview["summary"].(map[string]any)["subtotal"].(float64), fmt.Sprint(lines[0]["price_source"])
	}
	// Default mapping: member level N buys at price level N+1.
	for _, tc := range []struct {
		name     string
		customer string
		unit     string
		want     float64
		source   string
	}{
		{"walk-in pays the shelf price", "", "", 100, "branch_price"},
		{"member level 1 pays price level 2", members[1], "", 95, "price_level_2"},
		{"member level 2 pays price level 3", members[2], "", 90, "price_level_3"},
		{"a dearer level 4 price is not charged", members[3], "", 90, "price_level_3"},
		{"member level 4 pays price level 5", members[4], "", 80, "price_level_5"},
		{"a pack falls back to the piece level price times ten", members[2], packUnitID, 900, "price_level_3"},
	} {
		got, source := price(tc.customer, tc.unit)
		if got != tc.want || source != tc.source {
			t.Fatalf("%s: %.2f via %s, want %.2f via %s", tc.name, got, source, tc.want, tc.source)
		}
	}

	// A level default discount fills in where a product has no price of its own.
	settings, err := customerService.GetLevelSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	original := settings
	// A defer, not t.Cleanup: cleanups run after the deferred db.Close above.
	defer func() {
		if err := customerService.SaveLevelSettings(context.Background(), admin, meta, original); err != nil {
			t.Errorf("restore level settings: %v", err)
		}
	}()
	changed := customers.LevelSettings{MemberLevels: append([]customers.MemberLevel{}, settings.MemberLevels...),
		PriceLevels: append([]customers.PriceLevel{}, settings.PriceLevels...)}
	for index := range changed.PriceLevels {
		if changed.PriceLevels[index].Level == 2 {
			changed.PriceLevels[index].DefaultDiscountPercent = 10
		}
	}
	if err := customerService.SaveLevelSettings(ctx, admin, meta, changed); err != nil {
		t.Fatal(err)
	}
	if err := productService.SaveLevelPrices(ctx, admin, productID, meta, nil); err != nil {
		t.Fatal(err)
	}
	if got, source := price(members[1], ""); got != 90 || source != "price_level_2" {
		t.Fatalf("default 10%% off level 2: %.2f via %s, want 90.00 via price_level_2", got, source)
	}
	if got, _ := price("", ""); got != 100 {
		t.Fatalf("a walk-in gets no level discount, paid %.2f", got)
	}
}
