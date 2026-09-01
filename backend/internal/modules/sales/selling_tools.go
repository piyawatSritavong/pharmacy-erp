package sales

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"
)

// Selling tools shared by preview, invoice and POS checkout: multi-unit selling,
// cashier discounts, and promotions. Stock is always counted in the product's
// base unit, so every quantity that leaves this file for the ledger is a base
// quantity; the unit the cashier picked travels alongside for the receipt.

type unitInfo struct {
	ID         string
	Name       string
	Conversion int
	Price      float64
}

// resolveUnit loads the selling unit for a line. An empty unitID means the base
// unit, which keeps older clients working unchanged.
func resolveUnit(ctx context.Context, db platform.DBTX, productID, unitID string, basePrice float64, fallbackName string) (unitInfo, error) {
	unitID = strings.TrimSpace(unitID)
	var (
		id            string
		name          string
		conversionQty int
		sellingPrice  sql.NullFloat64
	)
	query := `
		SELECT id::text, unit_name, conversion_qty, selling_price
		FROM product_units
		WHERE product_id = $1 AND active = TRUE AND `
	arg := any(productID)
	var err error
	if unitID == "" {
		err = db.QueryRowContext(ctx, query+`is_base`, arg).Scan(&id, &name, &conversionQty, &sellingPrice)
	} else {
		err = db.QueryRowContext(ctx, query+`id = $2`, arg, unitID).Scan(&id, &name, &conversionQty, &sellingPrice)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			if unitID == "" {
				// Product predates the unit table; treat it as a 1:1 base unit.
				return unitInfo{Name: fallbackName, Conversion: 1, Price: basePrice}, nil
			}
			return unitInfo{}, platform.NewError(http.StatusBadRequest, "หน่วยนับที่เลือกใช้กับสินค้านี้ไม่ได้")
		}
		return unitInfo{}, err
	}
	if conversionQty <= 0 {
		conversionQty = 1
	}
	price := platform.Round2(basePrice * float64(conversionQty))
	if sellingPrice.Valid {
		price = platform.Round2(sellingPrice.Float64)
	}
	return unitInfo{ID: id, Name: name, Conversion: conversionQty, Price: price}, nil
}

// lotAllocation is one FEFO slice of stock reserved for a promotional giveaway.
type lotAllocation struct {
	LotID      string
	LotNumber  string
	Quantity   int
	UnitCost   float64
	ReceivedAt time.Time
	ExpiresOn  sql.NullTime
}

// allocateFEFO reserves `needed` base units across the branch's sellable lots,
// oldest expiry first, so giveaways consume stock exactly like a sold line.
func allocateFEFO(ctx context.Context, db platform.DBTX, branchID, productID, productName string, needed int) ([]lotAllocation, error) {
	if needed <= 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id::text, lot_number, remaining_quantity, unit_cost, received_at, expires_on
		FROM inventory_lots
		WHERE branch_id=$1 AND product_id=$2 AND stock_bucket='real' AND remaining_quantity>0
		  AND (expires_on IS NULL OR expires_on >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)
		ORDER BY expires_on NULLS LAST, received_at, id
	`, branchID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allocations := []lotAllocation{}
	remaining := needed
	for rows.Next() && remaining > 0 {
		var allocation lotAllocation
		var available int
		if err := rows.Scan(&allocation.LotID, &allocation.LotNumber, &available,
			&allocation.UnitCost, &allocation.ReceivedAt, &allocation.ExpiresOn); err != nil {
			return nil, err
		}
		take := available
		if take > remaining {
			take = remaining
		}
		allocation.Quantity = take
		remaining -= take
		allocations = append(allocations, allocation)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if remaining > 0 {
		return nil, platform.NewError(http.StatusConflict,
			fmt.Sprintf("สต๊อกของแถม %s ไม่เพียงพอ (ขาดอีก %d)", productName, remaining))
	}
	return allocations, nil
}

// promotion is one active campaign plus the products it references.
type promotion struct {
	ID              string
	Code            string
	Name            string
	Type            string
	MinQuantity     int
	MinAmount       float64
	DiscountPercent float64
	DiscountAmount  float64
	BundlePrice     sql.NullFloat64
	MaxUsesPerBill  int
	Priority        int
	Conditions      []promotionItem
	Rewards         []promotionItem
	BundleItems     []promotionItem
}

type promotionItem struct {
	ProductID   string
	ProductName string
	Quantity    int // expressed in base units
}

func loadActivePromotions(ctx context.Context, db platform.DBTX, branchID string) ([]promotion, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id::text, code, name, promo_type, min_quantity, min_amount, discount_percent,
		       discount_amount, bundle_price, max_uses_per_bill, priority
		FROM promotions
		WHERE active
		  AND (branch_id IS NULL OR branch_id = $1)
		  AND (starts_at IS NULL OR starts_at <= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)
		  AND (ends_at IS NULL OR ends_at >= (NOW() AT TIME ZONE 'Asia/Bangkok')::date)
		ORDER BY priority DESC, created_at
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	promotions := []promotion{}
	index := map[string]int{}
	ids := []string{}
	for rows.Next() {
		var item promotion
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Type, &item.MinQuantity,
			&item.MinAmount, &item.DiscountPercent, &item.DiscountAmount, &item.BundlePrice,
			&item.MaxUsesPerBill, &item.Priority); err != nil {
			return nil, err
		}
		index[item.ID] = len(promotions)
		ids = append(ids, item.ID)
		promotions = append(promotions, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(promotions) == 0 {
		return promotions, nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
	}
	itemRows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT pi.promotion_id::text, pi.product_id::text, p.name, pi.role,
		       pi.quantity * COALESCE(u.conversion_qty, 1)
		FROM promotion_items pi
		INNER JOIN products p ON p.id = pi.product_id
		LEFT JOIN product_units u ON u.id = pi.unit_id
		WHERE pi.promotion_id IN (%s)
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()
	for itemRows.Next() {
		var promotionID, productID, productName, role string
		var quantity int
		if err := itemRows.Scan(&promotionID, &productID, &productName, &role, &quantity); err != nil {
			return nil, err
		}
		position, ok := index[promotionID]
		if !ok {
			continue
		}
		entry := promotionItem{ProductID: productID, ProductName: productName, Quantity: quantity}
		switch role {
		case "condition":
			promotions[position].Conditions = append(promotions[position].Conditions, entry)
		case "reward":
			promotions[position].Rewards = append(promotions[position].Rewards, entry)
		case "bundle_item":
			promotions[position].BundleItems = append(promotions[position].BundleItems, entry)
		}
	}
	return promotions, itemRows.Err()
}

// giveawaySpec is a reward the cart earned, still to be turned into priced lines.
type giveawaySpec struct {
	ProductID     string
	ProductName   string
	Quantity      int
	PromotionID   string
	PromotionName string
}

// promotionOutcome is what the campaigns did to one cart.
type promotionOutcome struct {
	LineDiscounts map[int]float64 // index into the priced lines -> discount baht
	Giveaways     []giveawaySpec
	Applied       []map[string]any
}

func timesAllowed(times, maxUses int) int {
	if maxUses > 0 && times > maxUses {
		return maxUses
	}
	return times
}

// evaluatePromotions decides which campaigns the cart earns. It never touches
// stock; it only reports discounts per line and the giveaways to add.
func evaluatePromotions(promotions []promotion, lines []pricedLine) promotionOutcome {
	outcome := promotionOutcome{LineDiscounts: map[int]float64{}}

	// Sellable value per line after cashier discounts, used for pro-rata maths.
	lineNet := make([]float64, len(lines))
	billTotal := 0.0
	quantityByProduct := map[string]int{}
	netByProduct := map[string]float64{}
	for index, line := range lines {
		if line.IsGiveaway {
			continue
		}
		net := platform.Round2(line.LineSubtotal)
		lineNet[index] = net
		billTotal = platform.Round2(billTotal + net)
		quantityByProduct[line.ProductID] += line.Quantity
		netByProduct[line.ProductID] = platform.Round2(netByProduct[line.ProductID] + net)
	}

	for _, promo := range promotions {
		switch promo.Type {
		case "percent", "amount":
			matched := map[string]bool{}
			for _, condition := range promo.Conditions {
				matched[condition.ProductID] = true
			}
			eligibleTotal := 0.0
			eligibleQuantity := 0
			indexes := []int{}
			for index, line := range lines {
				if line.IsGiveaway || lineNet[index] <= 0 {
					continue
				}
				if len(matched) > 0 && !matched[line.ProductID] {
					continue
				}
				eligibleTotal = platform.Round2(eligibleTotal + lineNet[index])
				eligibleQuantity += line.Quantity
				indexes = append(indexes, index)
			}
			if len(indexes) == 0 || eligibleTotal <= 0 {
				continue
			}
			if promo.MinQuantity > 0 && eligibleQuantity < promo.MinQuantity {
				continue
			}
			if promo.MinAmount > 0 && eligibleTotal < promo.MinAmount {
				continue
			}
			discount := promo.DiscountAmount
			if promo.Type == "percent" {
				discount = platform.Round2(eligibleTotal * promo.DiscountPercent / 100)
			}
			if discount > eligibleTotal {
				discount = eligibleTotal
			}
			if discount <= 0 {
				continue
			}
			spread(outcome.LineDiscounts, indexes, lineNet, discount)
			outcome.Applied = append(outcome.Applied, map[string]any{
				"promotion_id": promo.ID, "code": promo.Code, "name": promo.Name,
				"promo_type": promo.Type, "discount_amount": discount,
			})

		case "buy_x_get_y":
			times := math.MaxInt32
			for _, condition := range promo.Conditions {
				available := quantityByProduct[condition.ProductID]
				if condition.Quantity <= 0 || available < condition.Quantity {
					times = 0
					break
				}
				if possible := available / condition.Quantity; possible < times {
					times = possible
				}
			}
			if times <= 0 || times == math.MaxInt32 {
				continue
			}
			times = timesAllowed(times, promo.MaxUsesPerBill)
			for _, reward := range promo.Rewards {
				outcome.Giveaways = append(outcome.Giveaways, giveawaySpec{
					ProductID: reward.ProductID, ProductName: reward.ProductName,
					Quantity: reward.Quantity * times, PromotionID: promo.ID, PromotionName: promo.Name,
				})
			}
			outcome.Applied = append(outcome.Applied, map[string]any{
				"promotion_id": promo.ID, "code": promo.Code, "name": promo.Name,
				"promo_type": promo.Type, "times": times,
			})

		case "bundle":
			if !promo.BundlePrice.Valid || len(promo.BundleItems) < 2 {
				continue
			}
			times := math.MaxInt32
			for _, bundleItem := range promo.BundleItems {
				available := quantityByProduct[bundleItem.ProductID]
				if bundleItem.Quantity <= 0 || available < bundleItem.Quantity {
					times = 0
					break
				}
				if possible := available / bundleItem.Quantity; possible < times {
					times = possible
				}
			}
			if times <= 0 || times == math.MaxInt32 {
				continue
			}
			times = timesAllowed(times, promo.MaxUsesPerBill)

			bundleValue := 0.0
			indexes := []int{}
			member := map[string]int{}
			for _, bundleItem := range promo.BundleItems {
				member[bundleItem.ProductID] = bundleItem.Quantity
			}
			for index, line := range lines {
				if line.IsGiveaway || lineNet[index] <= 0 {
					continue
				}
				required, ok := member[line.ProductID]
				if !ok || required <= 0 {
					continue
				}
				indexes = append(indexes, index)
			}
			for productID, required := range member {
				unitValue := 0.0
				if quantityByProduct[productID] > 0 {
					unitValue = netByProduct[productID] / float64(quantityByProduct[productID])
				}
				bundleValue = platform.Round2(bundleValue + unitValue*float64(required*times))
			}
			discount := platform.Round2(bundleValue - platform.Round2(promo.BundlePrice.Float64*float64(times)))
			if discount <= 0 || len(indexes) == 0 {
				continue
			}
			spread(outcome.LineDiscounts, indexes, lineNet, discount)
			outcome.Applied = append(outcome.Applied, map[string]any{
				"promotion_id": promo.ID, "code": promo.Code, "name": promo.Name,
				"promo_type": promo.Type, "times": times, "discount_amount": discount,
			})

		case "bill_giveaway":
			if promo.MinAmount <= 0 || billTotal < promo.MinAmount {
				continue
			}
			times := int(billTotal / promo.MinAmount)
			if times <= 0 {
				continue
			}
			times = timesAllowed(times, promo.MaxUsesPerBill)
			for _, reward := range promo.Rewards {
				outcome.Giveaways = append(outcome.Giveaways, giveawaySpec{
					ProductID: reward.ProductID, ProductName: reward.ProductName,
					Quantity: reward.Quantity * times, PromotionID: promo.ID, PromotionName: promo.Name,
				})
			}
			outcome.Applied = append(outcome.Applied, map[string]any{
				"promotion_id": promo.ID, "code": promo.Code, "name": promo.Name,
				"promo_type": promo.Type, "times": times,
			})
		}
	}
	return outcome
}

// spread splits an amount across lines in proportion to their value, giving the
// rounding remainder to the largest line so the parts always sum to the whole.
func spread(target map[int]float64, indexes []int, weights []float64, amount float64) {
	total := 0.0
	for _, index := range indexes {
		total += weights[index]
	}
	if total <= 0 || amount <= 0 {
		return
	}
	sorted := append([]int(nil), indexes...)
	sort.Slice(sorted, func(i, j int) bool { return weights[sorted[i]] > weights[sorted[j]] })

	allocated := 0.0
	for position, index := range sorted {
		share := platform.Round2(amount * weights[index] / total)
		if position == len(sorted)-1 {
			share = platform.Round2(amount - allocated)
		}
		if share < 0 {
			share = 0
		}
		if share > weights[index] {
			share = weights[index]
		}
		allocated = platform.Round2(allocated + share)
		target[index] = platform.Round2(target[index] + share)
	}
}

// cartResult is the fully priced cart: cashier lines plus any promotional
// giveaway lines, with every discount already applied.
type cartResult struct {
	Lines            []pricedLine
	Subtotal         float64
	TaxAmount        float64
	TotalAmount      float64
	LineDiscount     float64
	BillDiscount     float64
	PromotionDiscount float64
	GiveawayCost     float64
	AppliedPromotions []map[string]any
}

// priceCart prices the cashier's lines, lets promotions add giveaways and
// discounts, then spreads the bill discount across the remaining value. VAT is
// always charged on what the customer actually pays.
func (s *Service) priceCart(ctx context.Context, db platform.DBTX, user platform.AuthUser, branchID string,
	isGovernment bool, items []LineInput, billDiscount float64, vatRate float64, requireLot bool) (cartResult, error) {

	lines, _, _, _, err := s.priceLines(ctx, db, user, branchID, isGovernment, items, vatRate, requireLot)
	if err != nil {
		return cartResult{}, err
	}

	result := cartResult{Lines: lines, AppliedPromotions: []map[string]any{}}
	for _, line := range lines {
		result.LineDiscount = platform.Round2(result.LineDiscount + line.DiscountAmount)
	}

	promotions, err := loadActivePromotions(ctx, db, branchID)
	if err != nil {
		return cartResult{}, err
	}
	if len(promotions) > 0 {
		outcome := evaluatePromotions(promotions, result.Lines)
		result.AppliedPromotions = outcome.Applied
		for index, discount := range outcome.LineDiscounts {
			if discount <= 0 || index >= len(result.Lines) {
				continue
			}
			line := result.Lines[index]
			if discount > line.LineSubtotal {
				discount = line.LineSubtotal
			}
			line.DiscountAmount = platform.Round2(line.DiscountAmount + discount)
			line.LineSubtotal = platform.Round2(line.LineSubtotal - discount)
			result.Lines[index] = line
			result.PromotionDiscount = platform.Round2(result.PromotionDiscount + discount)
		}
		// Giveaways consume real stock exactly like a sold line, priced at zero
		// with the lot cost kept so the margin report shows what was given away.
		for _, giveaway := range outcome.Giveaways {
			if giveaway.Quantity <= 0 {
				continue
			}
			allocations, err := allocateFEFO(ctx, db, branchID, giveaway.ProductID, giveaway.ProductName, giveaway.Quantity)
			if err != nil {
				return cartResult{}, err
			}
			for _, allocation := range allocations {
				result.Lines = append(result.Lines, pricedLine{
					ProductID:      giveaway.ProductID,
					InventoryLotID: allocation.LotID,
					LotNumber:      allocation.LotNumber,
					LotReceivedAt:  allocation.ReceivedAt,
					LotExpiresOn:   allocation.ExpiresOn,
					ProductName:    giveaway.ProductName,
					DisplayName:    giveaway.ProductName + " (ของแถม)",
					Quantity:       allocation.Quantity,
					StockBucket:    "real",
					UnitPrice:      0,
					LineSubtotal:   0,
					TaxRate:        0,
					TaxAmount:      0,
					LineTotal:      0,
					CostSnapshot:   allocation.UnitCost,
					PriceSource:    "promotion_giveaway",
					UnitName:       "",
					ConversionQty:  1,
					SoldQuantity:   allocation.Quantity,
					SoldUnitPrice:  0,
					IsGiveaway:     true,
					PromotionID:    giveaway.PromotionID,
					PromotionName:  giveaway.PromotionName,
				})
				result.GiveawayCost = platform.Round2(result.GiveawayCost + allocation.UnitCost*float64(allocation.Quantity))
			}
		}
	}

	// Bill-level discount, spread over the lines that still carry value.
	billDiscount = platform.Round2(billDiscount)
	if billDiscount < 0 {
		return cartResult{}, platform.NewError(http.StatusBadRequest, "ส่วนลดท้ายบิลต้องไม่ติดลบ")
	}
	if billDiscount > 0 {
		if !platform.HasPermission(user, "sales.discount.line") {
			return cartResult{}, platform.NewError(http.StatusForbidden, "ไม่มีสิทธิ์ให้ส่วนลดท้ายบิล")
		}
		weights := make([]float64, len(result.Lines))
		indexes := []int{}
		remainingCap := 0.0
		payable := 0.0
		for index, line := range result.Lines {
			if line.IsGiveaway || line.LineSubtotal <= 0 {
				continue
			}
			weights[index] = line.LineSubtotal
			indexes = append(indexes, index)
			payable = platform.Round2(payable + line.LineSubtotal)
			remainingCap = platform.Round2(remainingCap + line.DiscountCeilingRemaining)
		}
		if billDiscount > payable {
			return cartResult{}, platform.NewError(http.StatusBadRequest, "ส่วนลดท้ายบิลมากกว่ายอดบิล")
		}
		if !platform.HasPermission(user, "price.override.global") && billDiscount > remainingCap {
			return cartResult{}, platform.NewError(http.StatusForbidden,
				fmt.Sprintf("ส่วนลดท้ายบิลเกินเพดานรวม %.2f บาท", remainingCap))
		}
		shares := map[int]float64{}
		spread(shares, indexes, weights, billDiscount)
		for index, share := range shares {
			line := result.Lines[index]
			line.BillDiscountShare = platform.Round2(share)
			line.LineSubtotal = platform.Round2(line.LineSubtotal - share)
			result.Lines[index] = line
		}
		result.BillDiscount = billDiscount
	}

	// Final VAT and totals from the net line values.
	result.Subtotal, result.TaxAmount, result.TotalAmount = 0, 0, 0
	for index, line := range result.Lines {
		line.TaxAmount = platform.Round2(line.LineSubtotal * line.TaxRate / 100)
		line.LineTotal = platform.Round2(line.LineSubtotal + line.TaxAmount)
		result.Lines[index] = line
		result.Subtotal = platform.Round2(result.Subtotal + line.LineSubtotal)
		result.TaxAmount = platform.Round2(result.TaxAmount + line.TaxAmount)
	}
	result.TotalAmount = platform.Round2(result.Subtotal + result.TaxAmount)
	return result, nil
}

// lineConversion guards against a zero multiplier reaching the CHECK constraint
// on legacy rows priced before units existed.
func lineConversion(line pricedLine) int {
	if line.ConversionQty <= 0 {
		return 1
	}
	return line.ConversionQty
}
