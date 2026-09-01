package sales

import (
	"database/sql"
	"testing"
)

func line(productID string, quantity int, subtotal float64) pricedLine {
	return pricedLine{ProductID: productID, Quantity: quantity, LineSubtotal: subtotal, ConversionQty: 1}
}

func TestEvaluatePromotionsBuyXGetY(t *testing.T) {
	promo := promotion{
		ID: "p1", Code: "B10G1", Name: "ซื้อ 10 แถม 1", Type: "buy_x_get_y",
		Conditions: []promotionItem{{ProductID: "A", Quantity: 10}},
		Rewards:    []promotionItem{{ProductID: "A", ProductName: "ยาแก้ไข้", Quantity: 1}},
	}
	outcome := evaluatePromotions([]promotion{promo}, []pricedLine{line("A", 25, 250)})
	if len(outcome.Giveaways) != 1 || outcome.Giveaways[0].Quantity != 2 {
		t.Fatalf("expected 2 free units for 25 bought, got %#v", outcome.Giveaways)
	}
}

func TestEvaluatePromotionsBuyXGetYRespectsBillCap(t *testing.T) {
	promo := promotion{
		ID: "p1", Type: "buy_x_get_y", MaxUsesPerBill: 1,
		Conditions: []promotionItem{{ProductID: "A", Quantity: 10}},
		Rewards:    []promotionItem{{ProductID: "A", Quantity: 1}},
	}
	outcome := evaluatePromotions([]promotion{promo}, []pricedLine{line("A", 30, 300)})
	if len(outcome.Giveaways) != 1 || outcome.Giveaways[0].Quantity != 1 {
		t.Fatalf("cap should limit giveaways to 1, got %#v", outcome.Giveaways)
	}
}

func TestEvaluatePromotionsPercentSpreadsAcrossMatchingLines(t *testing.T) {
	promo := promotion{
		ID: "p2", Type: "percent", DiscountPercent: 10,
		Conditions: []promotionItem{{ProductID: "A", Quantity: 1}, {ProductID: "B", Quantity: 1}},
	}
	lines := []pricedLine{line("A", 1, 100), line("B", 1, 300), line("C", 1, 500)}
	outcome := evaluatePromotions([]promotion{promo}, lines)
	total := outcome.LineDiscounts[0] + outcome.LineDiscounts[1]
	if total != 40 {
		t.Fatalf("expected 40 baht off the 400 baht eligible value, got %.2f", total)
	}
	if _, discounted := outcome.LineDiscounts[2]; discounted {
		t.Fatal("product outside the promotion must not be discounted")
	}
}

func TestEvaluatePromotionsPercentHonoursMinimumAmount(t *testing.T) {
	promo := promotion{ID: "p3", Type: "percent", DiscountPercent: 10, MinAmount: 500}
	outcome := evaluatePromotions([]promotion{promo}, []pricedLine{line("A", 1, 100)})
	if len(outcome.LineDiscounts) != 0 {
		t.Fatalf("bill below the minimum must not earn the promotion: %#v", outcome.LineDiscounts)
	}
}

func TestEvaluatePromotionsBundlePrice(t *testing.T) {
	promo := promotion{
		ID: "p4", Type: "bundle", BundlePrice: sql.NullFloat64{Float64: 150, Valid: true},
		BundleItems: []promotionItem{{ProductID: "A", Quantity: 1}, {ProductID: "B", Quantity: 1}},
	}
	// A costs 100, B costs 100 => bundle of 150 saves 50.
	outcome := evaluatePromotions([]promotion{promo}, []pricedLine{line("A", 1, 100), line("B", 1, 100)})
	total := outcome.LineDiscounts[0] + outcome.LineDiscounts[1]
	if total != 50 {
		t.Fatalf("expected 50 baht bundle saving, got %.2f", total)
	}
}

func TestEvaluatePromotionsBillGiveaway(t *testing.T) {
	promo := promotion{
		ID: "p5", Type: "bill_giveaway", MinAmount: 500, MaxUsesPerBill: 1,
		Rewards: []promotionItem{{ProductID: "Z", ProductName: "ผ้าเช็ดมือ", Quantity: 1}},
	}
	outcome := evaluatePromotions([]promotion{promo}, []pricedLine{line("A", 1, 1200)})
	if len(outcome.Giveaways) != 1 || outcome.Giveaways[0].ProductID != "Z" {
		t.Fatalf("bill over the threshold should earn the giveaway, got %#v", outcome.Giveaways)
	}
}

func TestSpreadKeepsTotalExact(t *testing.T) {
	weights := []float64{33.33, 33.33, 33.34}
	target := map[int]float64{}
	spread(target, []int{0, 1, 2}, weights, 10)
	total := target[0] + target[1] + target[2]
	if total != 10 {
		t.Fatalf("allocated parts must sum to the whole, got %.2f", total)
	}
}

func TestLineConversionDefaultsToOne(t *testing.T) {
	if got := lineConversion(pricedLine{}); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
	if got := lineConversion(pricedLine{ConversionQty: 12}); got != 12 {
		t.Fatalf("expected 12, got %d", got)
	}
}
