package parkedbills

import (
	"context"
	"os"
	"testing"

	"pharmacy-erp/backend/internal/database"
	"pharmacy-erp/backend/internal/platform"
)

func TestParkedBillRoundTripAndSingleClaimAgainstConfiguredDatabase(t *testing.T) {
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
	var userID, otherUserID, branchID, otherBranchID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE email='superadmin@erp.local'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM users WHERE id<>$1 AND active=TRUE ORDER BY id LIMIT 1`, userID).Scan(&otherUserID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE active=TRUE AND sales_enabled=TRUE ORDER BY id LIMIT 1`).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM branches WHERE active=TRUE AND sales_enabled=TRUE AND id<>$1 ORDER BY id LIMIT 1`, branchID).Scan(&otherBranchID); err != nil {
		t.Fatal(err)
	}
	user := platform.AuthUser{ID: userID, BranchID: &branchID}
	service := NewService(db)
	created, err := service.Create(ctx, user, Input{
		CustomerName: "Parked round trip", CustomerTaxID: "0100000000001",
		FullTaxInvoice: true, Note: "keep all fields", BillDiscountAmount: 10,
		Items: []Item{{
			ProductID: platform.MustUUID(), InventoryLotID: platform.MustUUID(),
			Quantity: 2, UnitID: platform.MustUUID(), UnitName: "กล่อง",
			ConversionQty: 12, SoldQuantity: 2, UnitPrice: 100,
			DiscountAmount: 5, ProductName: "Round trip item", SKU: "PARK-RT",
		}},
	})
	if err != nil {
		t.Fatalf("create parked bill: %v", err)
	}
	parkedID := created["id"].(string)

	got, err := service.Get(ctx, user, parkedID)
	if err != nil {
		t.Fatalf("get parked bill: %v", err)
	}
	items := got["items"].([]Item)
	if len(items) != 1 || items[0].UnitName != "กล่อง" || items[0].ConversionQty != 12 || items[0].SoldQuantity != 2 || items[0].DiscountAmount != 5 {
		t.Fatalf("parked line did not round trip: %#v", items)
	}
	if got["bill_discount_amount"].(float64) != 10 || got["note"] != "keep all fields" || got["customer_tax_id"] != "0100000000001" {
		t.Fatalf("parked header did not round trip: %#v", got)
	}

	otherUser := user
	otherUser.BranchID = &otherBranchID
	if _, err := service.Get(ctx, otherUser, parkedID); err == nil {
		t.Fatal("expected another branch to be unable to read parked bill")
	}

	type claimResult struct {
		item map[string]any
		err  error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	for range 2 {
		go func() {
			<-start
			item, claimErr := service.Claim(ctx, user, parkedID)
			results <- claimResult{item: item, err: claimErr}
		}()
	}
	close(start)
	successes, failures := 0, 0
	claimToken := ""
	for range 2 {
		result := <-results
		if result.err != nil {
			failures++
			continue
		}
		if result.item["claim_token"] == "" {
			t.Fatal("successful claim did not return token")
		}
		claimToken = result.item["claim_token"].(string)
		successes++
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected one parked claim winner, got success=%d failure=%d", successes, failures)
	}
	if _, err := service.GetClaimed(ctx, user, parkedID, claimToken); err != nil {
		t.Fatalf("owner could not reload claimed bill after navigation: %v", err)
	}
	if _, err := service.GetClaimed(ctx, user, parkedID, platform.MustUUID()); err == nil {
		t.Fatal("expected an invalid claim token to be rejected")
	}
	otherSameBranchUser := user
	otherSameBranchUser.ID = otherUserID
	if _, err := service.GetClaimed(ctx, otherSameBranchUser, parkedID, claimToken); err == nil {
		t.Fatal("expected another same-branch account to be unable to reload the claim")
	}
	if _, err := db.ExecContext(ctx, `UPDATE parked_bills SET claimed_at=NOW()-INTERVAL '16 minutes' WHERE id=$1`, parkedID); err != nil {
		t.Fatalf("age claim for recovery test: %v", err)
	}
	reclaimed, err := service.Claim(ctx, otherSameBranchUser, parkedID)
	if err != nil {
		t.Fatalf("recover stale claim: %v", err)
	}
	reclaimedToken, _ := reclaimed["claim_token"].(string)
	if reclaimedToken == "" || reclaimedToken == claimToken {
		t.Fatalf("stale claim recovery did not rotate token: old=%q new=%q", claimToken, reclaimedToken)
	}
	if err := service.Delete(ctx, user, parkedID, claimToken); err == nil {
		t.Fatal("expected previous claim owner/token to lose control after recovery")
	}
	if err := service.Delete(ctx, otherSameBranchUser, parkedID, reclaimedToken); err != nil {
		t.Fatalf("abandon parked bill: %v", err)
	}
	if _, err := service.Get(ctx, user, parkedID); err == nil {
		t.Fatal("expected abandoned parked bill to be unavailable")
	}
}
