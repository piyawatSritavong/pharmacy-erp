package v2

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

type PromotionMember struct {
	ProductID string `json:"product_id"`
	UnitID    string `json:"unit_id"`
	Quantity  int64  `json:"quantity"`
}
type PromotionRule struct {
	MinimumBaseQuantity int64             `json:"min_base_quantity"`
	MinimumAmountCents  int64             `json:"min_amount_cents"`
	DiscountBPS         int64             `json:"discount_bps"`
	DiscountCents       int64             `json:"discount_cents"`
	BundlePriceCents    int64             `json:"bundle_price_cents"`
	MaxUses             int64             `json:"max_uses"`
	Conditions          []PromotionMember `json:"conditions"`
	Rewards             []PromotionMember `json:"rewards"`
}
type PromotionInput struct {
	BranchID string        `json:"branch_id"`
	ID       string        `json:"id"`
	Scope    string        `json:"scope"`
	Code     string        `json:"code"`
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	Rule     PromotionRule `json:"rule"`
	StartsOn string        `json:"starts_on"`
	EndsOn   string        `json:"ends_on"`
	Active   bool          `json:"active"`
	Revision int           `json:"revision"`
	Reason   string        `json:"reason"`
}

const promotionsQuery = `SELECT jsonb_build_object('items',COALESCE(jsonb_agg(row_to_json(p) ORDER BY p.name),'[]'::jsonb)) FROM v2_promotions p WHERE branch_id=$1 OR branch_id IS NULL`

func (s *Service) savePromotion(ctx context.Context, tx *sql.Tx, a Actor, in PromotionInput) (any, error) {
	b, err := branch(ctx, tx, a.User, in.BranchID)
	if err != nil {
		return nil, err
	}
	if in.Scope != "branch" && in.Scope != "central" {
		return nil, bad("ระบุ scope branch หรือ central")
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "v2-policy:"+b); err != nil {
		return nil, err
	}
	if in.Scope == "branch" {
		var allowed bool
		if err = tx.QueryRowContext(ctx, `SELECT allow_branch_promotions FROM v2_branch_policies WHERE branch_id=$1`, b).Scan(&allowed); err != nil || !allowed {
			return nil, conflict("สาขานี้ไม่ได้เปิดสิทธิ์สร้างหรือแก้โปรโมชั่น")
		}
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Code) == "" || strings.TrimSpace(in.Reason) == "" {
		return nil, bad("ระบุชื่อ รหัส และเหตุผลเปลี่ยนโปรโมชั่น")
	}
	if err = date(in.StartsOn); err != nil {
		return nil, err
	}
	if in.EndsOn != "" {
		if err = date(in.EndsOn); err != nil {
			return nil, err
		}
		if in.EndsOn < in.StartsOn {
			return nil, bad("วันที่โปรโมชั่นไม่ถูกต้อง")
		}
	}
	r := in.Rule
	if r.MinimumBaseQuantity < 0 || r.MinimumBaseQuantity > maxInteger || r.MinimumAmountCents < 0 || r.MinimumAmountCents > maxInteger || r.DiscountBPS < 0 || r.DiscountBPS > 10000 || r.DiscountCents < 0 || r.DiscountCents > maxInteger || r.BundlePriceCents < 0 || r.BundlePriceCents > maxInteger || r.MaxUses < 0 || r.MaxUses > 10000 || len(r.Conditions) > 50 || len(r.Rewards) > 50 {
		return nil, bad("เงื่อนไขโปรโมชั่นเกินขอบเขต")
	}
	switch in.Type {
	case "percent":
		if r.DiscountBPS == 0 {
			return nil, bad("ระบุเปอร์เซ็นต์ลด")
		}
	case "amount":
		if r.DiscountCents == 0 {
			return nil, bad("ระบุยอดลด")
		}
	case "bundle":
		if len(r.Conditions) < 2 {
			return nil, bad("bundle ต้องมีสินค้าอย่างน้อยสองชนิด")
		}
	case "buy_x_get_y":
		if len(r.Conditions) == 0 || len(r.Rewards) == 0 {
			return nil, bad("ระบุสินค้าซื้อและของแถม")
		}
	case "bill_giveaway":
		if r.MinimumAmountCents <= 0 || len(r.Rewards) == 0 {
			return nil, bad("ระบุยอดขั้นต่ำและของแถม")
		}
	default:
		return nil, bad("ประเภทโปรโมชั่นไม่ถูกต้อง")
	}
	for _, group := range [][]PromotionMember{r.Conditions, r.Rewards} {
		seen := map[string]bool{}
		for _, m := range group {
			if seen[m.ProductID] {
				return nil, bad("ห้ามสินค้าเดียวกันซ้ำภายในกลุ่มโปรโมชั่น")
			}
			seen[m.ProductID] = true
			if _, err = resolveUnit(ctx, tx, m.ProductID, m.UnitID, m.Quantity); err != nil {
				return nil, err
			}
		}
	}
	scope := ""
	if in.Scope == "branch" {
		scope = b
	}
	if in.ID == "" {
		in.ID = newID()
		_, err = tx.ExecContext(ctx, `INSERT INTO v2_promotions(id,branch_id,code,name,promo_type,rule,starts_on,ends_on,active) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7::date,NULLIF($8,'')::date,$9)`, in.ID, optionalID(scope), strings.TrimSpace(in.Code), in.Name, in.Type, rawJSON(r), in.StartsOn, in.EndsOn, in.Active)
	} else {
		result, e := tx.ExecContext(ctx, `UPDATE v2_promotions SET code=$3,name=$4,promo_type=$5,rule=$6::jsonb,starts_on=$7::date,ends_on=NULLIF($8,'')::date,active=$9,revision=revision+1 WHERE id=$1 AND branch_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND revision=$10`, in.ID, scope, strings.TrimSpace(in.Code), in.Name, in.Type, rawJSON(r), in.StartsOn, in.EndsOn, in.Active, in.Revision)
		if e != nil {
			return nil, e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return nil, e
		}
		if n != 1 {
			return nil, conflict("โปรโมชั่นเปลี่ยนแล้ว หรือ scope ไม่ตรง")
		}
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO v2_promotion_versions(id,operation_id,promotion_id,snapshot,reason,actor_id) SELECT $1,$2,id,row_to_json(p),$4,$5 FROM v2_promotions p WHERE id=$3`, newID(), a.OperationID, in.ID, in.Reason, a.User.ID)
	if err != nil {
		return nil, err
	}
	return map[string]string{"id": in.ID}, emit(ctx, tx, a, "promotion.changed", map[string]string{"id": in.ID, "branch_id": b, "scope": in.Scope})
}

// V2 pilot selects one promotion explicitly. It never silently stacks several
// promotions and stores the rule used on the immutable document.
func applyPromotion(ctx context.Context, tx *sql.Tx, b, id, on string, lines []pricedLine) ([]pricedLine, json.RawMessage, error) {
	var kind string
	var ruleBytes, snapshot []byte
	if err := tx.QueryRowContext(ctx, `SELECT promo_type,rule,row_to_json(p) FROM v2_promotions p WHERE id=$1 AND (branch_id=$2 OR branch_id IS NULL) AND active=TRUE AND starts_on<=$3::date AND (ends_on IS NULL OR ends_on>=$3::date) FOR SHARE`, id, b, on).Scan(&kind, &ruleBytes, &snapshot); err != nil {
		return nil, nil, conflict("โปรโมชั่นไม่พร้อมใช้ในสาขาหรือวันที่นี้")
	}
	var rule PromotionRule
	if err := json.Unmarshal(ruleBytes, &rule); err != nil {
		return nil, nil, err
	}
	quantities := map[string]int64{}
	netByProduct := map[string]int64{}
	nets := make([]int64, len(lines))
	var bill, baseQty int64
	for i, l := range lines {
		gross, err := multiply(l.Price, l.Unit.Quantity)
		if err != nil {
			return nil, nil, err
		}
		nets[i] = gross - l.Discount
		quantities[l.ProductID], err = add(quantities[l.ProductID], l.Unit.BaseQuantity)
		if err != nil {
			return nil, nil, err
		}
		netByProduct[l.ProductID], err = add(netByProduct[l.ProductID], nets[i])
		if err != nil {
			return nil, nil, err
		}
		bill, err = add(bill, nets[i])
		if err != nil {
			return nil, nil, err
		}
		baseQty, err = add(baseQty, l.Unit.BaseQuantity)
		if err != nil {
			return nil, nil, err
		}
	}
	required := map[string]int64{}
	times := maxInteger
	for _, m := range rule.Conditions {
		u, err := resolveUnit(ctx, tx, m.ProductID, m.UnitID, m.Quantity)
		if err != nil {
			return nil, nil, err
		}
		required[m.ProductID] = u.BaseQuantity
		possible := quantities[m.ProductID] / u.BaseQuantity
		if possible < times {
			times = possible
		}
	}
	if kind == "bill_giveaway" {
		if bill < rule.MinimumAmountCents || baseQty < rule.MinimumBaseQuantity {
			return nil, nil, conflict("ยอดไม่ถึงเงื่อนไขของแถม")
		}
		times = 1
	}
	if kind == "buy_x_get_y" || kind == "bundle" {
		if times <= 0 || times == maxInteger {
			return nil, nil, conflict("จำนวนสินค้าไม่ถึงเงื่อนไขโปรโมชั่น")
		}
		if rule.MaxUses > 0 && times > rule.MaxUses {
			times = rule.MaxUses
		}
	}
	indexes := []int{}
	var eligible, eligibleQty int64
	for i, l := range lines {
		if len(required) > 0 && required[l.ProductID] == 0 {
			continue
		}
		if nets[i] == 0 {
			continue
		}
		indexes = append(indexes, i)
		var err error
		eligible, err = add(eligible, nets[i])
		if err != nil {
			return nil, nil, err
		}
		eligibleQty, err = add(eligibleQty, l.Unit.BaseQuantity)
		if err != nil {
			return nil, nil, err
		}
	}
	var discount int64
	if kind == "percent" || kind == "amount" {
		if eligible <= 0 || eligible < rule.MinimumAmountCents || eligibleQty < rule.MinimumBaseQuantity {
			return nil, nil, conflict("ยอดไม่ถึงเงื่อนไขส่วนลดโปรโมชั่น")
		}
		discount = rule.DiscountCents
		if kind == "percent" {
			var err error
			discount, err = proportion(eligible, rule.DiscountBPS, 10000)
			if err != nil {
				return nil, nil, err
			}
		}
		if discount > eligible {
			discount = eligible
		}
	}
	if kind == "bundle" {
		var bundleValue int64
		for p, q := range required {
			needed, err := multiply(q, times)
			if err != nil {
				return nil, nil, err
			}
			part, err := proportion(netByProduct[p], needed, quantities[p])
			if err != nil {
				return nil, nil, err
			}
			bundleValue, err = add(bundleValue, part)
			if err != nil {
				return nil, nil, err
			}
		}
		price, err := multiply(rule.BundlePriceCents, times)
		if err != nil {
			return nil, nil, err
		}
		if price >= bundleValue {
			return nil, nil, conflict("ราคา bundle ไม่ต่ำกว่ายอดที่ซื้อ")
		}
		discount = bundleValue - price
	}
	remaining, remainingNet := discount, eligible
	for _, i := range indexes {
		part := int64(0)
		if remaining > 0 {
			var err error
			part, err = proportion(remaining, nets[i], remainingNet)
			if err != nil {
				return nil, nil, err
			}
		}
		remaining -= part
		remainingNet -= nets[i]
		lines[i].Discount += part
		lines[i].PriceSource += ";promotion:" + id
		var err error
		lines[i].VAT, err = proportion(nets[i]-part, lines[i].VATBPS, 10000)
		if err != nil {
			return nil, nil, err
		}
		lines[i].Total, err = add(nets[i]-part, lines[i].VAT)
		if err != nil {
			return nil, nil, err
		}
	}
	if kind == "buy_x_get_y" || kind == "bill_giveaway" {
		for _, m := range rule.Rewards {
			qty, err := multiply(m.Quantity, times)
			if err != nil {
				return nil, nil, err
			}
			u, err := resolveUnit(ctx, tx, m.ProductID, m.UnitID, qty)
			if err != nil {
				return nil, nil, err
			}
			var name string
			if err = tx.QueryRowContext(ctx, `SELECT name FROM products WHERE id=$1`, m.ProductID).Scan(&name); err != nil {
				return nil, nil, err
			}
			lines = append(lines, pricedLine{ID: newID(), ProductID: m.ProductID, Description: name, PriceSource: "giveaway:" + id, Unit: u})
		}
	}
	var evidence map[string]any
	if err := json.Unmarshal(snapshot, &evidence); err != nil {
		return nil, nil, err
	}
	evidence["applied_base_requirements"] = required
	evidence["applied_times"] = times
	if times == maxInteger {
		evidence["applied_times"] = 1
	}
	evidence["applied_discount_cents"] = discount
	evidence["stacking_policy"] = "one_selected_promotion"
	return lines, json.RawMessage(rawJSON(evidence)), nil
}
