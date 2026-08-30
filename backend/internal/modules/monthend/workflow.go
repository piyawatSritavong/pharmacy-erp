package monthend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

const calculationVersion = "3.1.0"

type PeriodInput struct {
	Month               string `json:"month"`
	BranchID            string `json:"branch_id"`
	TargetRevenue       string `json:"target_revenue"`
	MarkupPercent       string `json:"markup_percent"`
	Notes               string `json:"notes"`
	SimulationReason    string `json:"simulation_reason"`
	SupportDocumentRef  string `json:"support_document_ref"`
	CalculationStrategy string `json:"calculation_strategy"`
	IdempotencyKey      string `json:"idempotency_key"`
	Confirmation        string `json:"confirmation"`
	Reason              string `json:"reason"`
	LockVersion         int    `json:"lock_version"`
}

type ValidationIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Count    int    `json:"count"`
}

type deterministicResult struct {
	Calculation
	MinimumAchievable int64 `json:"-"`
	MaximumAchievable int64 `json:"-"`
	AffectedInvoices  int   `json:"-"`
	AffectedItems     int   `json:"-"`
	GPBefore          int64 `json:"-"`
	GPAfter           int64 `json:"-"`
}

func parseDecimalCents(value string) (int64, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if value == "" {
		return 0, nil
	}
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid decimal")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	fraction := "00"
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) > 2 {
			return 0, fmt.Errorf("more than two decimal places")
		}
		fraction += strings.Repeat("0", 2-len(fraction))
	}
	frac, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, err
	}
	result := whole*100 + frac
	if negative {
		result = -result
	}
	return result, nil
}

func centsFromFloat(value float64) int64 {
	cents, _ := parseDecimalCents(strconv.FormatFloat(value, 'f', 2, 64))
	return cents
}

func centsToFloat(value int64) float64 {
	parsed, _ := strconv.ParseFloat(fmt.Sprintf("%d.%02d", value/100, abs64(value%100)), 64)
	if value < 0 && value > -100 {
		return -parsed
	}
	return parsed
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func roundedRatio(numerator, denominator int64) int64 {
	if denominator <= 0 {
		return 0
	}
	if numerator >= 0 {
		return (numerator + denominator/2) / denominator
	}
	return (numerator - denominator/2) / denominator
}

func closestQuantityCents(remaining, unitAmount int64, limit int) int {
	if remaining <= 0 || unitAmount <= 0 || limit <= 0 {
		return 0
	}
	base := int(remaining / unitAmount)
	if base > limit {
		base = limit
	}
	best := base
	if base < limit && abs64(remaining-int64(base+1)*unitAmount) < abs64(remaining-int64(base)*unitAmount) {
		best = base + 1
	}
	return best
}

type allocationChoice struct {
	lineIndex int
	quantity  int
	unitValue int64
	gap       int64
	phase     string
}

func calculateDeterministic(source Calculation, targetCents, markupBasisPoints int64) deterministicResult {
	return calculateDeterministicWithStrategy(source, targetCents, markupBasisPoints, "closest_then_oldest")
}

func calculateDeterministicWithStrategy(source Calculation, targetCents, markupBasisPoints int64, strategy string) deterministicResult {
	return calculateDeterministicInternal(source, targetCents, markupBasisPoints, strategy, true)
}

// calculateDeterministicInternal keeps all monetary arithmetic in integer
// satang. Float fields are populated only at the JSON/database boundary.
func calculateDeterministicInternal(source Calculation, targetCents, markupBasisPoints int64, strategy string, calculateBound bool) deterministicResult {
	source.Lines = append([]Line(nil), source.Lines...)
	result := deterministicResult{Calculation: source}
	result.TargetRevenue = centsToFloat(targetCents)
	result.MarkupPercent = centsToFloat(markupBasisPoints)
	if strategy != "oldest_first" {
		strategy = "closest_then_oldest"
	}
	actualCents := centsFromFloat(source.ActualRevenue)
	remaining := actualCents - targetCents
	if remaining < 0 {
		remaining = 0
	}
	requested := remaining
	available := map[string]int{}
	cashBudget := map[string]int64{}
	invoiceLineTotals := map[string]int64{}
	for _, line := range result.Lines {
		invoiceLineTotals[line.InvoiceID] += centsFromFloat(line.OriginalLineTotal)
		if amount := centsFromFloat(line.CashPaymentAmount); amount > cashBudget[line.InvoiceID] {
			cashBudget[line.InvoiceID] = amount
		}
	}
	for i := range result.Lines {
		line := &result.Lines[i]
		if cashBudget[line.InvoiceID] == 0 && line.PaymentType == "cash" {
			cashBudget[line.InvoiceID] = invoiceLineTotals[line.InvoiceID]
		}
		line.AllocatedGhostQuantity = 0
		line.RepricedQuantity = 0
		line.ProposedUnitPrice = line.OriginalUnitPrice
		line.ScenarioLineTotal = line.OriginalLineTotal
		line.GhostReductionAmount = 0
		line.PriceReductionAmount = 0
		line.Included = true
		line.AdjustmentType = "included"
		line.Note = ""
		if line.TaxInvoiceType == "full" {
			line.AdjustmentType = "locked_full_tax"
			line.Note = "ใบกำกับภาษีเต็มรูปถูกล็อกและไม่อยู่ในข้อเสนอปรับปรุง"
		}
		key := line.BranchID + ":" + line.ProductID
		if _, exists := available[key]; !exists {
			available[key] = line.GhostAvailable
		}
		result.GPBefore += centsFromFloat(line.OriginalLineTotal) - centsFromFloat(line.CostSnapshot)*int64(line.Quantity)
	}

	usedCashBudget := map[string]int64{}
	isCandidate := func(line *Line) bool {
		return (line.PaymentType == "cash" || line.PaymentType == "mixed") &&
			cashBudget[line.InvoiceID] > 0 &&
			line.TaxInvoiceType != "full" &&
			line.OriginalStockBucket == "real" &&
			line.Quantity > 0
	}
	pickChoice := func() (allocationChoice, bool) {
		best := allocationChoice{lineIndex: -1, gap: abs64(remaining)}
		for i := range result.Lines {
			line := &result.Lines[i]
			if !isCandidate(line) {
				continue
			}
			for _, phase := range []string{"ghost", "price"} {
				var unitValue int64
				var limit int
				if phase == "ghost" {
					key := line.BranchID + ":" + line.ProductID
					unitValue = roundedRatio(centsFromFloat(line.OriginalLineTotal), int64(line.Quantity))
					limit = minInt(line.Quantity-line.AllocatedGhostQuantity-line.RepricedQuantity, available[key])
				} else {
					cost := centsFromFloat(line.CostSnapshot)
					proposedUnit := roundedRatio(cost*(10000+markupBasisPoints), 10000)
					originalUnit := roundedRatio(centsFromFloat(line.OriginalLineTotal), int64(line.Quantity))
					taxBasisPoints := centsFromFloat(line.TaxRate)
					proposedTotal := roundedRatio(proposedUnit*(10000+taxBasisPoints), 10000)
					unitValue = originalUnit - proposedTotal
					limit = line.Quantity - line.AllocatedGhostQuantity - line.RepricedQuantity
				}
				budgetLeft := cashBudget[line.InvoiceID] - usedCashBudget[line.InvoiceID]
				if unitValue <= 0 || limit <= 0 || budgetLeft < unitValue {
					continue
				}
				// Allocate one unit per decision so a large single choice cannot
				// hide a closer combination of stock and price actions.
				gap := abs64(remaining - unitValue)
				if gap > abs64(remaining) {
					continue
				}
				choice := allocationChoice{lineIndex: i, quantity: 1, unitValue: unitValue, gap: gap, phase: phase}
				if strategy == "oldest_first" {
					return choice, true
				}
				if best.lineIndex < 0 || choice.gap < best.gap ||
					(choice.gap == best.gap && choice.phase == "ghost" && best.phase != "ghost") {
					best = choice
				}
			}
		}
		return best, best.lineIndex >= 0
	}

	ghostReduction := int64(0)
	priceReduction := int64(0)
	for remaining > 0 {
		choice, ok := pickChoice()
		if !ok {
			break
		}
		line := &result.Lines[choice.lineIndex]
		reduction := choice.unitValue * int64(choice.quantity)
		line.ScenarioLineTotal = centsToFloat(centsFromFloat(line.ScenarioLineTotal) - reduction)
		if choice.phase == "ghost" {
			key := line.BranchID + ":" + line.ProductID
			line.AllocatedGhostQuantity += choice.quantity
			line.GhostReductionAmount = centsToFloat(centsFromFloat(line.GhostReductionAmount) + reduction)
			line.AdjustmentType = "ghost_reclassification"
			line.Note = "ข้อเสนอจำแนกการใช้สินค้าไปยังสต๊อกรอง โดยไม่แก้ใบขายต้นฉบับ"
			available[key] -= choice.quantity
			ghostReduction += reduction
		} else {
			cost := centsFromFloat(line.CostSnapshot)
			proposedUnit := roundedRatio(cost*(10000+markupBasisPoints), 10000)
			line.RepricedQuantity += choice.quantity
			line.ProposedUnitPrice = centsToFloat(proposedUnit)
			line.PriceReductionAmount = centsToFloat(centsFromFloat(line.PriceReductionAmount) + reduction)
			if line.AllocatedGhostQuantity > 0 {
				line.AdjustmentType = "ghost_and_price"
			} else {
				line.AdjustmentType = "price_scenario"
			}
			line.Note = fmt.Sprintf("ข้อมูลจำลองราคาต้นทุน + %.2f%%; ไม่เขียนกลับใบขาย", centsToFloat(markupBasisPoints))
			priceReduction += reduction
		}
		usedCashBudget[line.InvoiceID] += reduction
		remaining -= reduction
	}

	affectedInvoices := map[string]bool{}
	for _, line := range result.Lines {
		lineTotal := centsFromFloat(line.ScenarioLineTotal)
		accountingQuantity := line.Quantity - line.AllocatedGhostQuantity
		result.GPAfter += lineTotal - centsFromFloat(line.CostSnapshot)*int64(accountingQuantity)
		if line.AllocatedGhostQuantity > 0 || line.RepricedQuantity > 0 {
			affectedInvoices[line.InvoiceID] = true
			result.AffectedItems++
		}
	}
	for index := range result.Lines {
		populateLineSemantics(&result.Lines[index])
	}
	result.AffectedInvoices = len(affectedInvoices)
	result.RequestedReduction = centsToFloat(requested)
	result.GhostReclassificationAmount = centsToFloat(ghostReduction)
	result.PriceScenarioReduction = centsToFloat(priceReduction)
	scenario := actualCents - ghostReduction - priceReduction
	if scenario < 0 {
		scenario = 0
	}
	result.ScenarioRevenue = centsToFloat(scenario)
	result.UnresolvedDifference = centsToFloat(scenario - targetCents)
	result.MaximumAchievable = actualCents
	result.MinimumAchievable = scenario
	if calculateBound {
		bound := calculateDeterministicInternal(source, 0, markupBasisPoints, strategy, false)
		result.MinimumAchievable = centsFromFloat(bound.ScenarioRevenue)
	}
	return result
}

func populateLineSemantics(line *Line) {
	line.OriginalGrossLineTotal = line.OriginalLineTotal
	line.ScenarioGrossLineTotal = line.ScenarioLineTotal
	if line.Quantity <= 0 {
		line.GhostReductionAmount = 0
		line.PriceReductionAmount = 0
		return
	}
	originalGrossUnit := roundedRatio(centsFromFloat(line.OriginalLineTotal), int64(line.Quantity))
	ghostReduction := originalGrossUnit * int64(line.AllocatedGhostQuantity)
	proposedUnit := centsFromFloat(line.ProposedUnitPrice)
	proposedGrossUnit := roundedRatio(proposedUnit*(10000+centsFromFloat(line.TaxRate)), 10000)
	priceUnitReduction := originalGrossUnit - proposedGrossUnit
	if priceUnitReduction < 0 {
		priceUnitReduction = 0
	}
	line.GhostReductionAmount = centsToFloat(ghostReduction)
	line.PriceReductionAmount = centsToFloat(priceUnitReduction * int64(line.RepricedQuantity))
}

func (s *Service) CreatePeriod(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input PeriodInput) (map[string]any, error) {
	input.Month = strings.TrimSpace(input.Month)
	source, err := s.Source(ctx, input.Month, strings.TrimSpace(input.BranchID))
	if err != nil {
		return nil, err
	}
	id, err := s.Create(ctx, user, meta, Input{Month: input.Month, BranchID: input.BranchID, TargetRevenue: source.ActualRevenue, MarkupPercent: 5})
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE month_end_workpapers SET current_step = 1, notes = $2, simulation_reason = $3, support_document_ref = $4, calculation_strategy = COALESCE(NULLIF($5,''), 'closest_then_oldest'), validation_summary = $6::jsonb, updated_at = NOW() WHERE id = $1`, id, input.Notes, input.SimulationReason, input.SupportDocumentRef, input.CalculationStrategy, platform.MustJSON([]ValidationIssue{}))
	if err != nil {
		return nil, err
	}
	return s.GetPeriod(ctx, id)
}

func (s *Service) UpdatePeriod(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id string, input PeriodInput) (map[string]any, error) {
	target, err := parseDecimalCents(input.TargetRevenue)
	if err != nil || target < 0 {
		return nil, platform.NewError(http.StatusBadRequest, "ยอดเป้าหมายต้องเป็นจำนวนเงินที่ถูกต้อง")
	}
	markup, err := parseDecimalCents(input.MarkupPercent)
	if err != nil || markup < 0 || markup > 100000 {
		return nil, platform.NewError(http.StatusBadRequest, "กำไรต้องอยู่ระหว่าง 0 ถึง 1000 เปอร์เซ็นต์")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE month_end_workpapers SET target_revenue = $2, markup_percent = $3, notes = $4,
			simulation_reason = $5, support_document_ref = $6,
			calculation_strategy = COALESCE(NULLIF($7,''), calculation_strategy), current_step = GREATEST(current_step, 2),
			lock_version = lock_version + 1, updated_at = NOW()
		WHERE id = $1 AND status IN ('OPEN','DRAFT','FAILED') AND ($8 = 0 OR lock_version = $8)
	`, id, centsToFloat(target), centsToFloat(markup), input.Notes, input.SimulationReason, input.SupportDocumentRef, input.CalculationStrategy, input.LockVersion)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return nil, platform.NewError(http.StatusConflict, "รอบบัญชีถูกแก้ไขหรือไม่อยู่ในสถานะที่บันทึกได้ กรุณาโหลดใหม่")
	}
	meta.EntityType, meta.EntityID, meta.Action = "month_end_workpaper", &id, "month_end.save_draft"
	meta.After = map[string]any{"target_revenue": centsToFloat(target), "markup_percent": centsToFloat(markup)}
	if err := s.audit.Log(ctx, s.db, meta); err != nil {
		return nil, err
	}
	return s.GetPeriod(ctx, id)
}

func (s *Service) ValidatePeriod(ctx context.Context, id string) ([]ValidationIssue, error) {
	var branchID, periodStart, periodEnd, status string
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(branch_id::text,''), period_start::text, period_end::text, status FROM month_end_workpapers WHERE id=$1`, id).Scan(&branchID, &periodStart, &periodEnd, &status); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบรอบบัญชี")
		}
		return nil, err
	}
	if status == "CANCELLED" {
		return nil, platform.NewError(http.StatusConflict, "รอบบัญชีฉบับร่างนี้ถูกยกเลิกแล้ว")
	}
	issues := []ValidationIssue{}
	addCount := func(code, severity, message, query string, args ...any) error {
		var count int
		if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			issues = append(issues, ValidationIssue{Code: code, Severity: severity, Message: message, Count: count})
		}
		return nil
	}
	if err := addCount("OVERLAPPING_CLOSED_PERIOD", "error", "มีรอบบัญชีที่ปิดแล้วทับช่วงเวลาเดียวกัน", `SELECT COUNT(*) FROM month_end_workpapers WHERE id<>$1 AND status='CLOSED' AND period_start <= $3::date AND period_end >= $2::date AND COALESCE(branch_id::text,'')=$4`, id, periodStart, periodEnd, branchID); err != nil {
		return nil, err
	}
	branchClause, args := "", []any{periodStart, periodEnd}
	if branchID != "" {
		branchClause = " AND branch_id=$3"
		args = append(args, branchID)
	}
	if err := addCount("UNPAID_INVOICES", "warning", "พบใบขายที่ยังชำระไม่ครบ", `SELECT COUNT(*) FROM invoices WHERE deleted_at IS NULL AND issued_at >= $1::date AND issued_at < ($2::date + INTERVAL '1 day') AND invoice_status='issued' AND payment_status<>'paid'`+branchClause, args...); err != nil {
		return nil, err
	}
	if err := addCount("CANCELLED_INVOICES", "warning", "พบใบขายยกเลิกในรอบนี้", `SELECT COUNT(*) FROM invoices WHERE deleted_at IS NULL AND issued_at >= $1::date AND issued_at < ($2::date + INTERVAL '1 day') AND invoice_status='cancelled'`+branchClause, args...); err != nil {
		return nil, err
	}
	if err := addCount("PAYMENT_MISMATCH", "error", "ยอดชำระไม่ตรงกับยอดใบขาย", `SELECT COUNT(*) FROM invoices i LEFT JOIN (SELECT invoice_id,SUM(amount) amount FROM invoice_payments GROUP BY invoice_id) p ON p.invoice_id=i.id WHERE i.deleted_at IS NULL AND i.issued_at >= $1::date AND i.issued_at < ($2::date + INTERVAL '1 day') AND i.invoice_status='issued' AND i.payment_status='paid' AND COALESCE(p.amount,0)<>i.total_amount`+strings.ReplaceAll(branchClause, "branch_id", "i.branch_id"), args...); err != nil {
		return nil, err
	}
	if err := addCount("NEGATIVE_STOCK", "error", "พบสต๊อกติดลบ", `SELECT COUNT(*) FROM inventory WHERE (qty_real < 0 OR qty_ghost < 0)`+func() string {
		if branchID != "" {
			return " AND branch_id=$1"
		}
		return ""
	}(), func() []any {
		if branchID != "" {
			return []any{branchID}
		}
		return nil
	}()...); err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		issues = append(issues, ValidationIssue{Code: "READY", Severity: "success", Message: "ไม่พบข้อผิดพลาดที่ขัดขวางการคำนวณ", Count: 0})
	}
	_, err := s.db.ExecContext(ctx, `UPDATE month_end_workpapers SET validation_summary=$2::jsonb, current_step=GREATEST(current_step,3), updated_at=NOW() WHERE id=$1`, id, platform.MustJSON(issues))
	return issues, err
}

func (s *Service) CalculatePeriod(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id string, input PeriodInput) (map[string]any, error) {
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		return nil, platform.NewError(http.StatusBadRequest, "ต้องระบุ idempotency_key")
	}
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status, periodStart, branchID, targetText, markupText, strategy string
		if err := tx.QueryRowContext(ctx, `SELECT status,period_start::text,COALESCE(branch_id::text,''),target_revenue::text,markup_percent::text,calculation_strategy FROM month_end_workpapers WHERE id=$1 FOR UPDATE`, id).Scan(&status, &periodStart, &branchID, &targetText, &markupText, &strategy); err != nil {
			return err
		}
		var existing bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM month_end_calculation_runs WHERE workpaper_id=$1 AND idempotency_key=$2)`, id, key).Scan(&existing); err != nil {
			return err
		}
		if existing {
			return nil
		}
		if status != "DRAFT" && status != "FAILED" && status != "OPEN" {
			return platform.NewError(http.StatusConflict, "สถานะปัจจุบันไม่สามารถคำนวณใหม่ได้")
		}
		target, err := parseDecimalCents(targetText)
		if err != nil {
			return err
		}
		markup, err := parseDecimalCents(markupText)
		if err != nil {
			return err
		}
		source, err := s.buildCalculation(ctx, tx, Input{Month: periodStart[:7], BranchID: branchID, TargetRevenue: centsToFloat(target), MarkupPercent: centsToFloat(markup)})
		if err != nil {
			return err
		}
		result := calculateDeterministicWithStrategy(source, target, markup, strategy)
		if _, err := tx.ExecContext(ctx, `DELETE FROM month_end_adjustments WHERE workpaper_id=$1 AND status='proposed'`, id); err != nil {
			return err
		}
		for _, line := range result.Lines {
			if _, err := tx.ExecContext(ctx, `UPDATE month_end_workpaper_lines SET allocated_ghost_quantity=$3,repriced_quantity=$4,proposed_unit_price=$5,scenario_line_total=$6,adjustment_type=$7,note=$8,included=TRUE,before_data=$9::jsonb,after_data=$10::jsonb,updated_at=NOW() WHERE workpaper_id=$1 AND invoice_item_id=$2`, id, line.InvoiceItemID, line.AllocatedGhostQuantity, line.RepricedQuantity, line.ProposedUnitPrice, line.ScenarioLineTotal, line.AdjustmentType, line.Note, platform.MustJSON(map[string]any{"stock_bucket": line.OriginalStockBucket, "unit_price": line.OriginalUnitPrice}), platform.MustJSON(map[string]any{"allocated_secondary_quantity": line.AllocatedGhostQuantity, "simulated_unit_price": line.ProposedUnitPrice})); err != nil {
				return err
			}
			if line.AllocatedGhostQuantity == 0 && line.RepricedQuantity == 0 {
				continue
			}
			typeName := "price_simulation"
			difference := centsFromFloat(line.OriginalLineTotal) - centsFromFloat(line.ScenarioLineTotal)
			if line.AllocatedGhostQuantity > 0 {
				typeName = "stock_reclassification"
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO month_end_adjustments(id,adjustment_number,workpaper_id,invoice_id,invoice_item_id,adjustment_type,before_data,after_data,difference_amount,reason,remark,status,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9,$10,$11,'proposed',$12,NOW())`, platform.MustUUID(), platform.GenerateReadableCode("MEA"), id, line.InvoiceID, line.InvoiceItemID, typeName, platform.MustJSON(map[string]any{"bucket": line.OriginalStockBucket, "unit_price": line.OriginalUnitPrice}), platform.MustJSON(map[string]any{"secondary_quantity": line.AllocatedGhostQuantity, "simulated_unit_price": line.ProposedUnitPrice}), centsToFloat(difference), "ข้อเสนอจากเครื่องมือคำนวณ "+calculationVersion, line.Note, user.ID); err != nil {
				return err
			}
		}
		summary := map[string]any{"minimum_achievable": centsToFloat(result.MinimumAchievable), "maximum_achievable": centsToFloat(result.MaximumAchievable), "affected_invoice_count": result.AffectedInvoices, "affected_item_count": result.AffectedItems, "gp_before": centsToFloat(result.GPBefore), "gp_after": centsToFloat(result.GPAfter), "exact_target": result.UnresolvedDifference == 0, "calculation_version": calculationVersion, "strategy": strategy}
		if _, err := tx.ExecContext(ctx, `UPDATE month_end_workpapers SET actual_revenue=$2,actual_invoice_count=$3,full_tax_revenue=$4,full_tax_invoice_count=$5,cash_revenue=$6,cash_invoice_count=$7,requested_reduction=$8,ghost_reclassification_amount=$9,price_scenario_reduction_amount=$10,scenario_revenue=$11,approved_accounting_revenue=$11,unresolved_difference=$12,source_hash=$13,gp_before=$14,gp_after=$15,result_summary=$16::jsonb,status='DRAFT',current_step=5,calculation_version=$17,calculated_at=NOW(),lock_version=lock_version+1,updated_at=NOW() WHERE id=$1`, id, result.ActualRevenue, result.ActualInvoiceCount, result.FullTaxRevenue, result.FullTaxInvoiceCount, result.CashRevenue, result.CashInvoiceCount, result.RequestedReduction, result.GhostReclassificationAmount, result.PriceScenarioReduction, result.ScenarioRevenue, result.UnresolvedDifference, result.SourceHash, centsToFloat(result.GPBefore), centsToFloat(result.GPAfter), platform.MustJSON(summary), calculationVersion); err != nil {
			return err
		}
		runID := platform.MustUUID()
		if _, err := tx.ExecContext(ctx, `INSERT INTO month_end_calculation_runs(id,workpaper_id,idempotency_key,calculation_version,strategy,input_data,result_data,source_hash,status,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8,'completed',$9,NOW())`, runID, id, key, calculationVersion, strategy, platform.MustJSON(input), platform.MustJSON(summary), result.SourceHash, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE month_end_adjustments SET calculation_run_id=$2 WHERE workpaper_id=$1 AND status='proposed' AND calculation_run_id IS NULL`, id, runID); err != nil {
			return err
		}
		meta.EntityType, meta.EntityID, meta.Action = "month_end_workpaper", &id, "month_end.calculate"
		meta.After = summary
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.GetPeriod(ctx, id)
}

func (s *Service) SetProposalIncluded(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, periodID, lineID string, included bool, reason string) (map[string]any, error) {
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE month_end_workpaper_lines l SET included=$3,reason=$4,updated_at=NOW() FROM month_end_workpapers w WHERE l.workpaper_id=w.id AND l.id=$2 AND w.id=$1 AND w.status='DRAFT'`, periodID, lineID, included, reason)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return platform.NewError(http.StatusConflict, "ไม่พบข้อเสนอหรือรอบบัญชีถูกล็อก")
		}
		if _, err := tx.ExecContext(ctx, `
			WITH totals AS (
				SELECT
					COALESCE(SUM(CASE WHEN included THEN original_line_total-scenario_line_total ELSE 0 END),0) AS reduction,
					COALESCE(SUM(CASE WHEN included THEN ROUND(original_line_total/quantity,2)*allocated_ghost_quantity ELSE 0 END),0) AS ghost_reduction,
					COALESCE(SUM(
						(CASE WHEN included THEN scenario_line_total ELSE original_line_total END)
						- cost_snapshot*(quantity-CASE WHEN included THEN allocated_ghost_quantity ELSE 0 END)
					),0) AS gp_after,
					COUNT(*) FILTER (WHERE included AND (allocated_ghost_quantity>0 OR repriced_quantity>0)) AS affected_items,
					COUNT(DISTINCT invoice_id) FILTER (WHERE included AND (allocated_ghost_quantity>0 OR repriced_quantity>0)) AS affected_invoices
				FROM month_end_workpaper_lines WHERE workpaper_id=$1
			)
			UPDATE month_end_workpapers w SET scenario_revenue=w.actual_revenue-t.reduction,
				approved_accounting_revenue=w.actual_revenue-t.reduction,
				unresolved_difference=(w.actual_revenue-t.reduction)-w.target_revenue,
				ghost_reclassification_amount=t.ghost_reduction,
				price_scenario_reduction_amount=t.reduction-t.ghost_reduction,
				gp_after=t.gp_after,
				result_summary=w.result_summary || jsonb_build_object(
					'affected_invoice_count',t.affected_invoices,
					'affected_item_count',t.affected_items,
					'gp_after',t.gp_after,
					'exact_target',(w.actual_revenue-t.reduction)=w.target_revenue
				),
				lock_version=lock_version+1,updated_at=NOW()
			FROM totals t WHERE w.id=$1
		`, periodID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE month_end_adjustments a SET status=CASE WHEN $3 THEN 'proposed' ELSE 'rejected' END,remark=$4 FROM month_end_workpaper_lines l WHERE l.id=$2 AND a.workpaper_id=$1 AND a.invoice_item_id=l.invoice_item_id AND a.status IN ('proposed','rejected')`, periodID, lineID, included, reason); err != nil {
			return err
		}
		meta.EntityType, meta.EntityID, meta.Action = "month_end_workpaper_line", &lineID, "month_end.adjust"
		meta.After = map[string]any{"included": included, "reason": reason, "period_id": periodID}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.GetPeriod(ctx, periodID)
}

func (s *Service) ChangeStatus(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id, action string, input PeriodInput) (map[string]any, error) {
	targetStatus, currentStep, requiredStatus := "", 6, "DRAFT"
	switch action {
	case "submit":
		targetStatus = "PENDING_APPROVAL"
	case "approve":
		targetStatus = "APPROVED"
		requiredStatus = "PENDING_APPROVAL"
		currentStep = 6
	default:
		return nil, platform.NewError(http.StatusBadRequest, "การดำเนินการไม่ถูกต้อง")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE month_end_workpapers SET status=$2,current_step=$3,approved_by=CASE WHEN $2='APPROVED' THEN $4 ELSE approved_by END,approved_at=CASE WHEN $2='APPROVED' THEN NOW() ELSE approved_at END,lock_version=lock_version+1,updated_at=NOW() WHERE id=$1 AND status=$5 AND calculation_version=$6`, id, targetStatus, currentStep, user.ID, requiredStatus, calculationVersion)
	if err != nil {
		return nil, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil, platform.NewError(http.StatusConflict, "สถานะหรือเวอร์ชันการคำนวณเปลี่ยนแล้ว กรุณาคำนวณใหม่และโหลดข้อมูลอีกครั้ง")
	}
	meta.EntityType, meta.EntityID, meta.Action = "month_end_workpaper", &id, "month_end."+action
	meta.Before = map[string]any{"status": requiredStatus}
	meta.After = map[string]any{"status": targetStatus}
	if err = s.audit.Log(ctx, s.db, meta); err != nil {
		return nil, err
	}
	return s.GetPeriod(ctx, id)
}

func validateDraftCancellation(status, workpaperNumber string, currentLockVersion int, input PeriodInput) error {
	if status != "DRAFT" {
		return platform.NewError(http.StatusConflict, "ยกเลิกได้เฉพาะรอบบัญชีฉบับร่าง")
	}
	if input.LockVersion <= 0 || input.LockVersion != currentLockVersion {
		return platform.NewError(http.StatusConflict, "รอบบัญชีถูกแก้ไขแล้ว กรุณาโหลดข้อมูลใหม่")
	}
	if strings.TrimSpace(input.Confirmation) != workpaperNumber {
		return platform.NewError(http.StatusBadRequest, "กรุณาพิมพ์เลขรอบบัญชีให้ตรง")
	}
	if strings.TrimSpace(input.Reason) == "" {
		return platform.NewError(http.StatusBadRequest, "ต้องระบุเหตุผลการลบร่าง")
	}
	return nil
}

func (s *Service) CancelDraft(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id string, input PeriodInput) (map[string]any, error) {
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var workpaperNumber, status string
		var lockVersion int
		if err := tx.QueryRowContext(ctx, `
			SELECT workpaper_number, status, lock_version
			FROM month_end_workpapers
			WHERE id = $1
			FOR UPDATE
		`, id).Scan(&workpaperNumber, &status, &lockVersion); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบรอบบัญชี")
			}
			return err
		}
		if err := validateDraftCancellation(status, workpaperNumber, lockVersion, input); err != nil {
			return err
		}

		reason := strings.TrimSpace(input.Reason)
		adjustmentRemark := "ยกเลิกรอบบัญชีฉบับร่าง: " + reason
		if _, err := tx.ExecContext(ctx, `
			UPDATE month_end_adjustments
			SET status = 'rejected',
			    remark = CASE WHEN remark = '' THEN $2 ELSE remark || ' · ' || $2 END
			WHERE workpaper_id = $1 AND status = 'proposed'
		`, id, adjustmentRemark); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE month_end_workpapers
			SET status = 'CANCELLED', cancelled_by = $2, cancelled_at = NOW(),
			    cancel_reason = $3, lock_version = lock_version + 1, updated_at = NOW()
			WHERE id = $1
		`, id, user.ID, reason); err != nil {
			return err
		}

		meta.EntityType, meta.EntityID, meta.Action = "month_end_workpaper", &id, "month_end.cancel_draft"
		meta.Before = map[string]any{"status": status, "lock_version": lockVersion}
		meta.After = map[string]any{"status": "CANCELLED", "reason": reason, "lock_version": lockVersion + 1}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.GetPeriod(ctx, id)
}

func (s *Service) ClosePeriod(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id string, input PeriodInput) (map[string]any, error) {
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status, periodStart, sourceHash, branchID, targetText, markupText, version string
		if err := tx.QueryRowContext(ctx, `SELECT status,period_start::text,source_hash,COALESCE(branch_id::text,''),target_revenue::text,markup_percent::text,calculation_version FROM month_end_workpapers WHERE id=$1 FOR UPDATE`, id).Scan(&status, &periodStart, &sourceHash, &branchID, &targetText, &markupText, &version); err != nil {
			return err
		}
		if status != "APPROVED" {
			return platform.NewError(http.StatusConflict, "ต้องอนุมัติรายการก่อนปิดรอบ")
		}
		if version != calculationVersion {
			return platform.NewError(http.StatusConflict, "ผลคำนวณเป็นเวอร์ชันเก่า กรุณาคำนวณใหม่ก่อนปิดรอบ")
		}
		expected := "ยืนยันปิดรอบ " + periodStart[:7]
		if strings.TrimSpace(input.Confirmation) != expected {
			return platform.NewError(http.StatusBadRequest, "กรุณาพิมพ์ข้อความยืนยันให้ตรง: "+expected)
		}
		target, err := parseDecimalCents(targetText)
		if err != nil {
			return err
		}
		markup, err := parseDecimalCents(markupText)
		if err != nil {
			return err
		}
		current, err := s.buildCalculation(ctx, tx, Input{Month: periodStart[:7], BranchID: branchID, TargetRevenue: centsToFloat(target), MarkupPercent: centsToFloat(markup)})
		if err != nil {
			return err
		}
		if current.SourceHash != sourceHash {
			return platform.NewError(http.StatusConflict, "ข้อมูลขายหรือสต๊อกเปลี่ยนหลังคำนวณ กรุณาคำนวณใหม่ก่อนปิดรอบ")
		}
		rows, err := tx.QueryContext(ctx, `SELECT l.id::text,l.branch_id::text,l.product_id::text,l.invoice_id::text,l.invoice_item_id::text,l.allocated_ghost_quantity,l.original_stock_bucket FROM month_end_workpaper_lines l WHERE l.workpaper_id=$1 AND l.included=TRUE AND l.allocated_ghost_quantity>0 ORDER BY l.branch_id,l.product_id,l.id`, id)
		if err != nil {
			return err
		}
		type entry struct {
			lineID, branchID, productID, invoiceID, itemID, bucket string
			quantity                                               int
		}
		entries := []entry{}
		for rows.Next() {
			var e entry
			if err := rows.Scan(&e.lineID, &e.branchID, &e.productID, &e.invoiceID, &e.itemID, &e.quantity, &e.bucket); err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, e := range entries {
			var real, ghost int
			if err := tx.QueryRowContext(ctx, `SELECT qty_real,qty_ghost FROM inventory WHERE branch_id=$1 AND product_id=$2 FOR UPDATE`, e.branchID, e.productID).Scan(&real, &ghost); err != nil {
				return err
			}
			if ghost < e.quantity {
				return platform.NewError(http.StatusConflict, "สต๊อกรองไม่พอ กรุณาคำนวณใหม่")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE inventory SET qty_real=qty_real+$3,qty_ghost=qty_ghost-$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, e.branchID, e.productID, e.quantity); err != nil {
				return err
			}
			outID, inID := platform.MustUUID(), platform.MustUUID()
			for _, m := range []struct {
				id, bucket string
				delta      int
			}{{outID, "ghost", -e.quantity}, {inID, "real", e.quantity}} {
				if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_movements(id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at) VALUES($1,$2,$3,'month_end_reclassification',$4,$5,'month_end_workpaper',$6,$7,$8,NOW())`, m.id, e.branchID, e.productID, m.bucket, m.delta, id, disclaimer, user.ID); err != nil {
					return err
				}
			}
			if err := reclassifyLots(ctx, tx, e.branchID, e.productID, "ghost", "real", e.quantity, outID, inID); err != nil {
				return err
			}
			var adjustmentID string
			if err := tx.QueryRowContext(ctx, `SELECT id::text FROM month_end_adjustments WHERE workpaper_id=$1 AND invoice_item_id=$2 AND adjustment_type='stock_reclassification' AND status='proposed' ORDER BY created_at DESC LIMIT 1`, id, e.itemID).Scan(&adjustmentID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE month_end_adjustments SET status='approved',approved_by=$2,approved_at=NOW() WHERE id=$1`, adjustmentID, user.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_reclassifications(id,workpaper_id,adjustment_id,branch_id,product_id,from_bucket,to_bucket,quantity,status,movement_out_id,movement_in_id,created_by,created_at) VALUES($1,$2,$3,$4,$5,'ghost','real',$6,'approved',$7,$8,$9,NOW())`, platform.MustUUID(), id, adjustmentID, e.branchID, e.productID, e.quantity, outID, inID, user.ID); err != nil {
				return err
			}
		}
		checksumBytes := sha256.Sum256([]byte(id + "|" + sourceHash + "|" + time.Now().UTC().Format(time.RFC3339Nano)))
		checksum := hex.EncodeToString(checksumBytes[:])
		if _, err := tx.ExecContext(ctx, `UPDATE month_end_adjustments SET status='approved',approved_by=$2,approved_at=NOW() WHERE workpaper_id=$1 AND status='proposed'`, id, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE month_end_workpapers SET status='CLOSED',current_step=6,closed_by=$2,closed_at=NOW(),finalized_by=$2,finalized_at=NOW(),checksum=$3,lock_version=lock_version+1,updated_at=NOW() WHERE id=$1`, id, user.ID, checksum); err != nil {
			return err
		}
		meta.EntityType, meta.EntityID, meta.Action = "month_end_workpaper", &id, "month_end.close"
		meta.Before = map[string]any{"status": "APPROVED"}
		meta.After = map[string]any{"status": "CLOSED", "checksum": checksum, "reclassifications": len(entries)}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.GetPeriod(ctx, id)
}

func (s *Service) ReopenPeriod(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id string, input PeriodInput) (map[string]any, error) {
	if strings.TrimSpace(input.Reason) == "" {
		return nil, platform.NewError(http.StatusBadRequest, "ต้องระบุเหตุผลการเปิดรอบใหม่")
	}
	newID := platform.MustUUID()
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM month_end_workpapers WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
			return err
		}
		if status != "CLOSED" {
			return platform.NewError(http.StatusConflict, "เปิดใหม่ได้เฉพาะรอบที่ปิดแล้ว")
		}
		rows, err := tx.QueryContext(ctx, `SELECT id::text,branch_id::text,product_id::text,quantity FROM inventory_reclassifications WHERE workpaper_id=$1 AND status='approved' FOR UPDATE`, id)
		if err != nil {
			return err
		}
		type rev struct {
			id, branch, product string
			qty                 int
		}
		items := []rev{}
		for rows.Next() {
			var r rev
			if err := rows.Scan(&r.id, &r.branch, &r.product, &r.qty); err != nil {
				rows.Close()
				return err
			}
			items = append(items, r)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, r := range items {
			var real int
			if err := tx.QueryRowContext(ctx, `SELECT qty_real FROM inventory WHERE branch_id=$1 AND product_id=$2 FOR UPDATE`, r.branch, r.product).Scan(&real); err != nil {
				return err
			}
			if real < r.qty {
				return platform.NewError(http.StatusConflict, "สต๊อกหลักไม่พอสำหรับย้อนรายการ")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE inventory SET qty_real=qty_real-$3,qty_ghost=qty_ghost+$3,updated_at=NOW() WHERE branch_id=$1 AND product_id=$2`, r.branch, r.product, r.qty); err != nil {
				return err
			}
			outID, inID := platform.MustUUID(), platform.MustUUID()
			for _, m := range []struct {
				id, bucket string
				delta      int
			}{{outID, "real", -r.qty}, {inID, "ghost", r.qty}} {
				if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_movements(id,branch_id,product_id,movement_type,stock_bucket,quantity_delta,reference_type,reference_id,note,performed_by,created_at) VALUES($1,$2,$3,'month_end_reversal',$4,$5,'month_end_workpaper',$6,$7,$8,NOW())`, m.id, r.branch, r.product, m.bucket, m.delta, id, input.Reason, user.ID); err != nil {
					return err
				}
			}
			if err := reclassifyLots(ctx, tx, r.branch, r.product, "real", "ghost", r.qty, outID, inID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE inventory_reclassifications SET status='reversed',reversed_by=$2,reversed_at=NOW() WHERE id=$1`, r.id, user.ID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE month_end_adjustments SET status='reversed',reversed_at=NOW() WHERE workpaper_id=$1 AND status='approved'`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE month_end_workpapers SET status='REOPENED',reopened_by=$2,reopened_at=NOW(),reopen_reason=$3,updated_at=NOW() WHERE id=$1`, id, user.ID, input.Reason); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO month_end_workpapers(id,workpaper_number,branch_id,period_start,period_end,target_revenue,markup_percent,actual_revenue,actual_invoice_count,full_tax_revenue,full_tax_invoice_count,cash_revenue,cash_invoice_count,requested_reduction,ghost_reclassification_amount,price_scenario_reduction_amount,scenario_revenue,unresolved_difference,source_hash,status,disclaimer,created_by,calculated_at,created_at,updated_at,current_step,revision,parent_period_id,notes,simulation_reason,support_document_ref,calculation_strategy,calculation_version,approved_accounting_revenue,gp_before,gp_after,validation_summary,result_summary) SELECT $2,workpaper_number||'-R'||(revision+1),branch_id,period_start,period_end,target_revenue,markup_percent,actual_revenue,actual_invoice_count,full_tax_revenue,full_tax_invoice_count,cash_revenue,cash_invoice_count,requested_reduction,ghost_reclassification_amount,price_scenario_reduction_amount,scenario_revenue,unresolved_difference,source_hash,'DRAFT',disclaimer,$3,NOW(),NOW(),NOW(),3,revision+1,$1,notes,simulation_reason,support_document_ref,calculation_strategy,calculation_version,approved_accounting_revenue,gp_before,gp_after,validation_summary,result_summary FROM month_end_workpapers WHERE id=$1`, id, newID, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO month_end_workpaper_lines(id,workpaper_id,invoice_id,invoice_item_id,management_sequence,invoice_number_snapshot,branch_id,branch_name_snapshot,issued_at_snapshot,payment_type_snapshot,cash_payment_amount_snapshot,bank_transfer_payment_amount_snapshot,tax_invoice_type_snapshot,product_id,sku_snapshot,product_name_snapshot,original_stock_bucket,quantity,allocated_ghost_quantity,repriced_quantity,original_unit_price,cost_snapshot,proposed_unit_price,tax_rate,original_line_total,scenario_line_total,adjustment_type,note,created_at,included,reason,supporting_document_ref,before_data,after_data,rejected_reason,updated_at) SELECT gen_random_uuid(),$2,invoice_id,invoice_item_id,management_sequence,invoice_number_snapshot,branch_id,branch_name_snapshot,issued_at_snapshot,payment_type_snapshot,cash_payment_amount_snapshot,bank_transfer_payment_amount_snapshot,tax_invoice_type_snapshot,product_id,sku_snapshot,product_name_snapshot,original_stock_bucket,quantity,allocated_ghost_quantity,repriced_quantity,original_unit_price,cost_snapshot,proposed_unit_price,tax_rate,original_line_total,scenario_line_total,adjustment_type,note,NOW(),included,reason,supporting_document_ref,before_data,after_data,rejected_reason,NOW() FROM month_end_workpaper_lines WHERE workpaper_id=$1`, id, newID); err != nil {
			return err
		}
		meta.EntityType, meta.EntityID, meta.Action = "month_end_workpaper", &id, "month_end.reopen"
		meta.Before = map[string]any{"status": "CLOSED"}
		meta.After = map[string]any{"status": "REOPENED", "new_revision_id": newID, "reason": input.Reason}
		return s.audit.Log(ctx, tx, meta)
	})
	if err != nil {
		return nil, err
	}
	return s.GetPeriod(ctx, newID)
}

func (s *Service) GetPeriod(ctx context.Context, id string) (map[string]any, error) {
	item, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	var currentStep, revision, lockVersion int
	var parentID, notes, reason, doc, strategy, version, validation, result, checksum, reopenReason, cancelReason string
	var approvedAt, closedAt, reopenedAt, cancelledAt sql.NullTime
	err = s.db.QueryRowContext(ctx, `SELECT current_step,revision,COALESCE(parent_period_id::text,''),notes,simulation_reason,support_document_ref,calculation_strategy,calculation_version,validation_summary::text,result_summary::text,checksum,reopen_reason,lock_version,approved_at,closed_at,reopened_at,cancelled_at,cancel_reason FROM month_end_workpapers WHERE id=$1`, id).Scan(&currentStep, &revision, &parentID, &notes, &reason, &doc, &strategy, &version, &validation, &result, &checksum, &reopenReason, &lockVersion, &approvedAt, &closedAt, &reopenedAt, &cancelledAt, &cancelReason)
	if err != nil {
		return nil, err
	}
	item["current_step"], item["revision"], item["parent_period_id"], item["notes"], item["simulation_reason"], item["support_document_ref"] = currentStep, revision, parentID, notes, reason, doc
	item["calculation_strategy"], item["calculation_version"], item["checksum"], item["reopen_reason"], item["lock_version"] = strategy, version, checksum, reopenReason, lockVersion
	var validationValue, resultValue any
	_ = json.Unmarshal([]byte(validation), &validationValue)
	_ = json.Unmarshal([]byte(result), &resultValue)
	item["validation_summary"], item["result_summary"] = validationValue, resultValue
	if approvedAt.Valid {
		item["approved_at"] = approvedAt.Time
	}
	if closedAt.Valid {
		item["closed_at"] = closedAt.Time
	}
	if reopenedAt.Valid {
		item["reopened_at"] = reopenedAt.Time
	}
	if cancelledAt.Valid {
		item["cancelled_at"] = cancelledAt.Time
		item["cancel_reason"] = cancelReason
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id::text,adjustment_number,adjustment_type,difference_amount,reason,remark,supporting_document_ref,status,created_at,approved_at,before_data::text,after_data::text FROM month_end_adjustments WHERE workpaper_id=$1 ORDER BY created_at,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	adjustments := []map[string]any{}
	for rows.Next() {
		var aid, number, kind, reason, remark, doc, status, before, after string
		var diff float64
		var created time.Time
		var approved sql.NullTime
		if err := rows.Scan(&aid, &number, &kind, &diff, &reason, &remark, &doc, &status, &created, &approved, &before, &after); err != nil {
			return nil, err
		}
		row := map[string]any{"id": aid, "adjustment_number": number, "adjustment_type": kind, "difference_amount": diff, "reason": reason, "remark": remark, "supporting_document_ref": doc, "status": status, "created_at": created, "before_data": before, "after_data": after}
		if approved.Valid {
			row["approved_at"] = approved.Time
		}
		adjustments = append(adjustments, row)
	}
	item["adjustments"] = adjustments
	return item, rows.Err()
}

func (s *Service) PeriodAudit(ctx context.Context, id string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id::text,a.action,COALESCE(u.full_name,''),a.before_data::text,a.after_data::text,
		       a.request_id,a.source_ip,a.created_at
		FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_id
		WHERE (a.entity_type='month_end_workpaper' AND a.entity_id=$1)
		   OR (a.entity_type='month_end_workpaper_line' AND (a.after_data->>'period_id')=$1::text)
		ORDER BY a.created_at,a.id
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var eventID, action, actor, before, after, requestID, sourceIP string
		var created time.Time
		if err := rows.Scan(&eventID, &action, &actor, &before, &after, &requestID, &sourceIP, &created); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"id": eventID, "action": action, "actor_name": actor, "before_data": before, "after_data": after, "request_id": requestID, "source_ip": sourceIP, "created_at": created})
	}
	return items, rows.Err()
}

func (s *Service) ExportSimulationCSV(ctx context.Context, id string) ([]byte, string, error) {
	item, err := s.GetPeriod(ctx, id)
	if err != nil {
		return nil, "", err
	}
	buffer := bytes.NewBuffer(nil)
	buffer.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(buffer)
	_ = writer.Write([]string{"ข้อมูลจำลอง — ไม่ใช่รายงานทางบัญชีหรือภาษี"})
	_ = writer.Write([]string{"เลขกระดาษทำการ", textValue(item["workpaper_number"]), "สถานะ", textValue(item["status"]), "รอบ", textValue(item["period_start"]) + " ถึง " + textValue(item["period_end"])})
	_ = writer.Write([]string{"เลขใบขาย", "วันที่", "สาขา", "SKU", "สินค้า", "จำนวน", "ยอดจริง", "ยอดจำลอง", "ประเภทข้อเสนอ", "เลือกใช้"})
	for _, line := range item["transactions"].([]Line) {
		_ = writer.Write([]string{line.InvoiceNumber, line.IssuedAt.Format(time.RFC3339), line.BranchName, line.SKU, line.ProductName, strconv.Itoa(line.Quantity), fmt.Sprintf("%.2f", line.OriginalLineTotal), fmt.Sprintf("%.2f", line.ScenarioLineTotal), line.AdjustmentType, strconv.FormatBool(line.Included)})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), "month-end-simulation-" + textValue(item["period_start"])[0:7] + ".csv", nil
}

func textValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

type WorkflowHandler struct{ service *Service }

func NewWorkflowHandler(service *Service) *WorkflowHandler { return &WorkflowHandler{service: service} }
func bindPeriod(c echo.Context) (PeriodInput, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&raw); err != nil {
		return PeriodInput{}, err
	}
	var input PeriodInput
	decodeString := func(key string) *string {
		value, ok := raw[key]
		if !ok {
			return nil
		}
		var text string
		if json.Unmarshal(value, &text) == nil {
			return &text
		}
		var number json.Number
		if json.Unmarshal(value, &number) == nil {
			text = number.String()
			return &text
		}
		return nil
	}
	for key, target := range map[string]*string{"month": &input.Month, "branch_id": &input.BranchID, "target_revenue": &input.TargetRevenue, "markup_percent": &input.MarkupPercent, "notes": &input.Notes, "simulation_reason": &input.SimulationReason, "support_document_ref": &input.SupportDocumentRef, "calculation_strategy": &input.CalculationStrategy, "idempotency_key": &input.IdempotencyKey, "confirmation": &input.Confirmation, "reason": &input.Reason} {
		if value := decodeString(key); value != nil {
			*target = *value
		}
	}
	if value, ok := raw["lock_version"]; ok {
		_ = json.Unmarshal(value, &input.LockVersion)
	}
	return input, nil
}
func (h *WorkflowHandler) Create(c echo.Context) error {
	input, err := bindPeriod(c)
	if err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลรอบบัญชีไม่ถูกต้อง"))
	}
	item, err := h.service.CreatePeriod(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"item": item, "message": "สร้างร่างรอบบัญชีแล้ว"})
}
func (h *WorkflowHandler) Update(c echo.Context) error {
	input, err := bindPeriod(c)
	if err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลร่างไม่ถูกต้อง"))
	}
	item, err := h.service.UpdatePeriod(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("periodID"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"item": item, "message": "บันทึกร่างแล้ว"})
}
func (h *WorkflowHandler) Get(c echo.Context) error {
	item, err := h.service.GetPeriod(c.Request().Context(), c.Param("periodID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}
func (h *WorkflowHandler) Validate(c echo.Context) error {
	issues, err := h.service.ValidatePeriod(c.Request().Context(), c.Param("periodID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"issues": issues})
}
func (h *WorkflowHandler) Calculate(c echo.Context) error {
	input, err := bindPeriod(c)
	if err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลคำนวณไม่ถูกต้อง"))
	}
	item, err := h.service.CalculatePeriod(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("periodID"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"item": item, "message": "คำนวณและบันทึกข้อเสนอแล้ว"})
}
func (h *WorkflowHandler) Toggle(c echo.Context) error {
	var input struct {
		Included bool   `json:"included"`
		Reason   string `json:"reason"`
	}
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลข้อเสนอไม่ถูกต้อง"))
	}
	item, err := h.service.SetProposalIncluded(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("periodID"), c.Param("lineID"), input.Included, input.Reason)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"item": item, "message": "บันทึกการเลือกข้อเสนอแล้ว"})
}
func (h *WorkflowHandler) Status(action string) echo.HandlerFunc {
	return func(c echo.Context) error {
		input, _ := bindPeriod(c)
		item, err := h.service.ChangeStatus(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("periodID"), action, input)
		if err != nil {
			return platform.HandleHTTPError(c, err)
		}
		return platform.JSON(c, http.StatusOK, map[string]any{"item": item, "message": "อัปเดตสถานะรอบบัญชีแล้ว"})
	}
}
func (h *WorkflowHandler) Close(c echo.Context) error {
	input, err := bindPeriod(c)
	if err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลยืนยันไม่ถูกต้อง"))
	}
	item, err := h.service.ClosePeriod(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("periodID"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"item": item, "message": "ปิดและล็อกรอบบัญชีแล้ว"})
}
func (h *WorkflowHandler) Reopen(c echo.Context) error {
	input, err := bindPeriod(c)
	if err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลเปิดรอบไม่ถูกต้อง"))
	}
	item, err := h.service.ReopenPeriod(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("periodID"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"item": item, "message": "ย้อนรายการและสร้าง revision ใหม่แล้ว"})
}

func (h *WorkflowHandler) CancelDraft(c echo.Context) error {
	input, err := bindPeriod(c)
	if err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลยืนยันการลบร่างไม่ถูกต้อง"))
	}
	item, err := h.service.CancelDraft(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("periodID"), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"item": item, "message": "ยกเลิกรอบบัญชีฉบับร่างแล้ว"})
}

func (h *WorkflowHandler) Audit(c echo.Context) error {
	items, err := h.service.PeriodAudit(c.Request().Context(), c.Param("periodID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *WorkflowHandler) Export(c echo.Context) error {
	data, filename, err := h.service.ExportSimulationCSV(c.Request().Context(), c.Param("periodID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+filename+`"`)
	return c.Blob(http.StatusOK, "text/csv; charset=utf-8", data)
}
