package sales

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/modules/customers"
	"pharmacy-erp/backend/internal/modules/products"
	"pharmacy-erp/backend/internal/modules/purchasing"
	"pharmacy-erp/backend/internal/platform"
)

// Members, tier prices, points and credit through the real checkout
// transaction: what the till charges, what the ledger records, and that
// concurrent sales cannot spend the same points or the same credit twice.
func TestCustomerSalesAgainstConfiguredDatabase(t *testing.T) {
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
		Permissions: []string{"invoice.create.remote", "sales.discount.line", "customer.credit.manage", "payment.collect", "quotation.manage"}}
	meta := audit.LogEntry{ActorID: &userID}
	auditService := audit.NewService(db)
	purchaseService := purchasing.NewService(db, auditService)
	productService := products.NewService(db, auditService, nil)
	customerService := customers.NewService(db, auditService)
	salesService := NewService(db, auditService)

	unique := platform.MustUUID()
	supplierID, err := purchaseService.CreateSupplier(ctx, meta, purchasing.SupplierInput{SupplierCode: "CUS-" + unique[:8], LegalName: "Customer sales " + unique})
	if err != nil {
		t.Fatal(err)
	}
	poID, err := purchaseService.CreatePurchaseOrder(ctx, admin, meta, purchasing.PurchaseOrderInput{
		BranchID: branchID, SupplierID: supplierID, PurchasedAt: time.Now().UTC().Format(time.RFC3339), VATMode: "none",
		Items: []purchasing.PurchaseOrderLineInput{{NewProduct: &purchasing.NewProductInput{SKU: "CUS-" + unique[:8], Name: "Customer sales product",
			UnitName: "ชิ้น", BaseSellingPrice: 100, MaxDiscountAmount: 10}, StockBucket: "real", Quantity: 200, UnitCost: 50, LotNumber: "CUS-" + unique[:8]}},
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
	// The till prices from the branch price when one exists; pin it so the
	// arithmetic below is about tiers, not about a seeded branch override.
	if _, err := db.ExecContext(ctx, `DELETE FROM branch_product_prices WHERE product_id=$1`, productID); err != nil {
		t.Fatal(err)
	}
	if err := productService.SaveUnits(ctx, productID, meta, []products.UnitInput{
		{UnitName: "ชิ้น", ConversionQty: 1, IsBase: true},
		{UnitName: "แพ็ค", ConversionQty: 10},
	}); err != nil {
		t.Fatal(err)
	}
	var baseUnitID, packUnitID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM product_units WHERE product_id=$1 AND is_base`, productID).Scan(&baseUnitID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM product_units WHERE product_id=$1 AND NOT is_base`, productID).Scan(&packUnitID); err != nil {
		t.Fatal(err)
	}
	if err := productService.SavePriceTiers(ctx, admin, productID, meta, []products.PriceTierInput{
		{UnitID: baseUnitID, CustomerTier: "all", MinQuantity: 5, UnitPrice: 90},
		{UnitID: baseUnitID, CustomerTier: "wholesale", MinQuantity: 1, UnitPrice: 80},
		{UnitID: packUnitID, CustomerTier: "wholesale", MinQuantity: 1, UnitPrice: 750},
	}); err != nil {
		t.Fatal(err)
	}

	member, err := customerService.Create(ctx, admin, meta, customers.Input{Name: "Integration member", Phone: fmt.Sprintf("06%08d", time.Now().UnixNano()%100000000)})
	if err != nil {
		t.Fatal(err)
	}
	memberID := member["id"].(string)
	limit, days := 1000.0, 30
	account, err := customerService.Create(ctx, admin, meta, customers.Input{Name: "Integration wholesale", CustomerType: "business",
		PriceTier: "wholesale", CreditLimit: &limit, CreditDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	accountID := account["id"].(string)
	vatRate, err := platform.GetSettingFloat(ctx, db, "vat_rate", 7)
	if err != nil {
		t.Fatal(err)
	}
	gross := func(net float64) float64 { return platform.Round2(net + platform.Round2(net*vatRate/100)) }
	line := func(quantity int, unitID string) []LineInput {
		return []LineInput{{ProductID: productID, InventoryLotID: lotID, Quantity: quantity, UnitID: unitID, StockBucket: "real"}}
	}

	// Tier prices: 4 pieces at shelf price, 5 at the quantity break, a pack at
	// the wholesale pack price for the wholesale account.
	for _, tc := range []struct {
		name     string
		customer string
		quantity int
		unit     string
		subtotal float64
	}{
		{"shelf price below the break", "", 4, "", 400},
		{"quantity break at five", "", 5, "", 450},
		{"wholesale piece price", accountID, 1, "", 80},
		{"wholesale pack price", accountID, 1, packUnitID, 750},
	} {
		preview, err := salesService.PreviewSale(ctx, admin, branchID, false, line(tc.quantity, tc.unit), 0, cartOptions{CustomerID: tc.customer})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := preview["summary"].(map[string]any)["subtotal"].(float64); got != tc.subtotal {
			t.Fatalf("%s: subtotal %.2f, want %.2f", tc.name, got, tc.subtotal)
		}
	}

	// A member sale earns floor(total / 25) points; spending 40 takes 10 baht
	// off before VAT and both movements land in the ledger.
	if _, err := customerService.AdjustPoints(ctx, admin, meta, memberID, customers.PointsAdjustment{Points: 100, Note: "integration"}); err != nil {
		t.Fatal(err)
	}
	result, err := salesService.Checkout(ctx, admin, meta, CheckoutRequest{BranchID: branchID, CustomerID: memberID, RedeemPoints: 40,
		Items: line(2, ""), PaymentType: "bank_transfer"})
	if err != nil {
		t.Fatal(err)
	}
	wantTotal := gross(200 - 10)
	if result["total_amount"].(float64) != wantTotal {
		t.Fatalf("member total %.2f, want %.2f", result["total_amount"], wantTotal)
	}
	wantEarned := int(math.Floor(wantTotal / 25))
	if result["points_earned"].(int) != wantEarned || result["points_balance"].(int) != 100-40+wantEarned {
		t.Fatalf("points earned %v balance %v, want %d and %d", result["points_earned"], result["points_balance"], wantEarned, 100-40+wantEarned)
	}
	memberInvoice := result["invoice_id"].(string)

	// Five tills spend the member's remaining points at once; one redemption
	// of 40 fits, so exactly one may succeed.
	balance := 100 - 40 + wantEarned
	if _, err := customerService.AdjustPoints(ctx, admin, meta, memberID, customers.PointsAdjustment{Points: 50 - balance, Note: "race"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := salesService.Checkout(ctx, admin, meta, CheckoutRequest{BranchID: branchID, CustomerID: memberID, RedeemPoints: 40,
				Items: line(1, ""), PaymentType: "bank_transfer"}); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	var finalBalance, ledger int
	if err := db.QueryRowContext(ctx, `SELECT points_balance, (SELECT COALESCE(SUM(points),0) FROM loyalty_point_entries WHERE customer_id=$1) FROM customers WHERE id=$1`, memberID).Scan(&finalBalance, &ledger); err != nil {
		t.Fatal(err)
	}
	if successes != 1 || finalBalance != ledger {
		t.Fatalf("concurrent redemptions: %d succeeded, balance %d, ledger %d", successes, finalBalance, ledger)
	}

	// Credit: room for one pack bill (750 + VAT), not two, even when six
	// tills try at the same moment.
	successes = 0
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := salesService.Checkout(ctx, admin, meta, CheckoutRequest{BranchID: branchID, CustomerID: accountID,
				Items: line(1, packUnitID), PaymentType: "credit"}); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	var owed float64
	var status, saleType string
	var due time.Time
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(total_amount),0) FROM invoices WHERE customer_id=$1 AND deleted_at IS NULL`, accountID).Scan(&owed); err != nil {
		t.Fatal(err)
	}
	if successes != 1 || owed != gross(750) || owed > limit {
		t.Fatalf("concurrent credit sales: %d succeeded, owed %.2f, limit %.2f", successes, owed, limit)
	}
	if err := db.QueryRowContext(ctx, `SELECT payment_status, sale_type, due_date FROM invoices WHERE customer_id=$1`, accountID).Scan(&status, &saleType, &due); err != nil {
		t.Fatal(err)
	}
	if status != "unpaid" || saleType != "credit" || due.Sub(time.Now().UTC()) < 28*24*time.Hour {
		t.Fatalf("credit bill status %s type %s due %s", status, saleType, due)
	}

	// Collect in two parts: partial first, then the rest; overpaying is refused.
	if _, err := customerService.ReceivePayment(ctx, admin, meta, customers.ReceivePaymentInput{CustomerID: accountID, PaymentType: "cash", Amount: 100}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT payment_status FROM invoices WHERE customer_id=$1`, accountID).Scan(&status); err != nil || status != "partial" {
		t.Fatalf("after partial payment status %q err %v", status, err)
	}
	if _, err := customerService.ReceivePayment(ctx, admin, meta, customers.ReceivePaymentInput{CustomerID: accountID, PaymentType: "cash", Amount: owed}); err == nil {
		t.Fatal("overpayment was accepted")
	}
	if _, err := customerService.ReceivePayment(ctx, admin, meta, customers.ReceivePaymentInput{CustomerID: accountID, PaymentType: "bank_transfer", Amount: platform.Round2(owed - 100)}); err != nil {
		t.Fatal(err)
	}
	var paid float64
	if err := db.QueryRowContext(ctx, `SELECT i.payment_status, COALESCE(SUM(p.amount),0) FROM invoices i JOIN invoice_payments p ON p.invoice_id=i.id WHERE i.customer_id=$1 GROUP BY i.id`, accountID).Scan(&status, &paid); err != nil {
		t.Fatal(err)
	}
	if status != "paid" || paid != owed {
		t.Fatalf("after settling: status %s paid %.2f owed %.2f", status, paid, owed)
	}

	// A paid credit bill is never a month-end cash candidate.
	source, err := loadSaleTypeOf(ctx, db, accountID)
	if err != nil || source != "credit" {
		t.Fatalf("sale type after payment %q %v", source, err)
	}

	// Deleting the member's first bill returns the 40 points it spent and
	// takes back what it earned; the ledger stays equal to the balance.
	if err := db.QueryRowContext(ctx, `SELECT points_balance FROM customers WHERE id=$1`, memberID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	var number string
	if err := db.QueryRowContext(ctx, `SELECT invoice_number FROM invoices WHERE id=$1`, memberInvoice).Scan(&number); err != nil {
		t.Fatal(err)
	}
	if err := salesService.DeleteInvoice(ctx, admin, meta, memberInvoice, "ลบ "+number); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT points_balance, (SELECT COALESCE(SUM(points),0) FROM loyalty_point_entries WHERE customer_id=$1) FROM customers WHERE id=$1`, memberID).Scan(&finalBalance, &ledger); err != nil {
		t.Fatal(err)
	}
	if want := balance + 40 - wantEarned; finalBalance != ledger || finalBalance != want {
		t.Fatalf("after delete: balance %d ledger %d, want %d", finalBalance, ledger, want)
	}
}

func loadSaleTypeOf(ctx context.Context, db platform.DBTX, customerID string) (string, error) {
	var saleType string
	err := db.QueryRowContext(ctx, `SELECT sale_type FROM invoices WHERE customer_id=$1 LIMIT 1`, customerID).Scan(&saleType)
	return saleType, err
}
