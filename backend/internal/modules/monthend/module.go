package monthend

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

const disclaimer = "กระดาษทำการภายใน ไม่ใช่รายงานภาษี และไม่เปลี่ยนแปลงธุรกรรมต้นฉบับ"

type Input struct {
	Month         string  `json:"month"`
	BranchID      string  `json:"branch_id"`
	TargetRevenue float64 `json:"target_revenue"`
	MarkupPercent float64 `json:"markup_percent"`
}

type Line struct {
	ID                     string    `json:"id,omitempty"`
	InvoiceID              string    `json:"invoice_id"`
	InvoiceItemID          string    `json:"invoice_item_id"`
	ManagementSequence     int       `json:"management_sequence"`
	InvoiceNumber          string    `json:"invoice_number"`
	BranchID               string    `json:"branch_id"`
	BranchName             string    `json:"branch_name"`
	IssuedAt               time.Time `json:"issued_at"`
	PaymentType            string    `json:"payment_type"`
	CashPaymentAmount      float64   `json:"cash_payment_amount"`
	TransferPaymentAmount  float64   `json:"transfer_payment_amount"`
	TaxInvoiceType         string    `json:"tax_invoice_type"`
	ProductID              string    `json:"product_id"`
	SKU                    string    `json:"sku"`
	ProductName            string    `json:"product_name"`
	OriginalStockBucket    string    `json:"original_stock_bucket"`
	Quantity               int       `json:"quantity"`
	GhostAvailable         int       `json:"ghost_available_snapshot,omitempty"`
	AllocatedGhostQuantity int       `json:"allocated_ghost_quantity"`
	RepricedQuantity       int       `json:"repriced_quantity"`
	OriginalUnitPrice      float64   `json:"original_unit_price"`
	CostSnapshot           float64   `json:"cost_snapshot"`
	ProposedUnitPrice      float64   `json:"proposed_unit_price"`
	TaxRate                float64   `json:"tax_rate"`
	OriginalLineTotal      float64   `json:"original_line_total"`
	ScenarioLineTotal      float64   `json:"scenario_line_total"`
	OriginalGrossLineTotal float64   `json:"original_gross_line_total"`
	ScenarioGrossLineTotal float64   `json:"scenario_gross_line_total"`
	GhostReductionAmount   float64   `json:"ghost_reclassification_reduction"`
	PriceReductionAmount   float64   `json:"price_scenario_reduction"`
	AdjustmentType         string    `json:"adjustment_type"`
	Note                   string    `json:"note"`
	Included               bool      `json:"included"`
	Reason                 string    `json:"reason,omitempty"`
	RejectedReason         string    `json:"rejected_reason,omitempty"`
	invoiceTotal           float64
	invoiceUpdatedAt       time.Time
}

type Calculation struct {
	BranchID                    string  `json:"branch_id"`
	BranchName                  string  `json:"branch_name"`
	PeriodStart                 string  `json:"period_start"`
	PeriodEnd                   string  `json:"period_end"`
	TargetRevenue               float64 `json:"target_revenue"`
	MarkupPercent               float64 `json:"markup_percent"`
	ActualRevenue               float64 `json:"actual_revenue"`
	ActualInvoiceCount          int     `json:"actual_invoice_count"`
	FullTaxRevenue              float64 `json:"full_tax_revenue"`
	FullTaxInvoiceCount         int     `json:"full_tax_invoice_count"`
	CashRevenue                 float64 `json:"cash_revenue"`
	CashInvoiceCount            int     `json:"cash_invoice_count"`
	RequestedReduction          float64 `json:"requested_reduction"`
	GhostReclassificationAmount float64 `json:"ghost_reclassification_amount"`
	PriceScenarioReduction      float64 `json:"price_scenario_reduction_amount"`
	ScenarioRevenue             float64 `json:"scenario_revenue"`
	UnresolvedDifference        float64 `json:"unresolved_difference"`
	SourceHash                  string  `json:"source_hash"`
	Disclaimer                  string  `json:"disclaimer"`
	Lines                       []Line  `json:"transactions"`
}

type Service struct {
	db    *sql.DB
	audit *audit.Service
}

func NewService(db *sql.DB, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func (s *Service) Preview(ctx context.Context, input Input) (Calculation, error) {
	return s.buildCalculation(ctx, s.db, input)
}

func (s *Service) Source(ctx context.Context, month, branchID string) (Calculation, error) {
	calculation, err := s.buildCalculation(ctx, s.db, Input{Month: month, BranchID: branchID, MarkupPercent: 5})
	if err != nil {
		return Calculation{}, err
	}
	calculation.TargetRevenue = calculation.ActualRevenue
	calculation.RequestedReduction = 0
	calculation.GhostReclassificationAmount = 0
	calculation.PriceScenarioReduction = 0
	calculation.ScenarioRevenue = calculation.ActualRevenue
	calculation.UnresolvedDifference = 0
	for index := range calculation.Lines {
		line := &calculation.Lines[index]
		line.AllocatedGhostQuantity = 0
		line.RepricedQuantity = 0
		line.ProposedUnitPrice = line.OriginalUnitPrice
		line.ScenarioLineTotal = line.OriginalLineTotal
		populateLineSemantics(line)
		line.AdjustmentType = "included"
		line.Note = ""
		line.Included = true
		if line.TaxInvoiceType == "full" {
			line.AdjustmentType = "locked_full_tax"
			line.Note = "ใบกำกับภาษีเต็มรูปถูกล็อกและคงอยู่ในยอดจริง"
		}
	}
	return calculation, nil
}

func (s *Service) Create(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, input Input) (string, error) {
	workpaperID := platform.MustUUID()
	number := platform.GenerateReadableCode("MEC")
	err := platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		calculation, err := s.buildCalculation(ctx, tx, input)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO month_end_workpapers (
				id, workpaper_number, branch_id, period_start, period_end, target_revenue,
				markup_percent, actual_revenue, actual_invoice_count, full_tax_revenue,
				full_tax_invoice_count, cash_revenue, cash_invoice_count, requested_reduction,
				ghost_reclassification_amount, price_scenario_reduction_amount, scenario_revenue,
				unresolved_difference, source_hash, status, disclaimer, created_by,
				calculated_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
				$15, $16, $17, $18, $19, 'DRAFT', $20, $21, NOW(), NOW(), NOW()
			)
		`, workpaperID, number, platform.NullUUID(stringPointer(calculation.BranchID)), calculation.PeriodStart,
			calculation.PeriodEnd, calculation.TargetRevenue, calculation.MarkupPercent,
			calculation.ActualRevenue, calculation.ActualInvoiceCount, calculation.FullTaxRevenue,
			calculation.FullTaxInvoiceCount, calculation.CashRevenue, calculation.CashInvoiceCount,
			calculation.RequestedReduction, calculation.GhostReclassificationAmount,
			calculation.PriceScenarioReduction, calculation.ScenarioRevenue,
			calculation.UnresolvedDifference, calculation.SourceHash, disclaimer, user.ID); err != nil {
			return err
		}

		for _, line := range calculation.Lines {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO month_end_workpaper_lines (
					id, workpaper_id, invoice_id, invoice_item_id, management_sequence,
					invoice_number_snapshot, branch_id, branch_name_snapshot, issued_at_snapshot,
					payment_type_snapshot, cash_payment_amount_snapshot,
					bank_transfer_payment_amount_snapshot, tax_invoice_type_snapshot, product_id, sku_snapshot,
					product_name_snapshot, original_stock_bucket, quantity,
					allocated_ghost_quantity, repriced_quantity, original_unit_price, cost_snapshot,
					proposed_unit_price, tax_rate, original_line_total, scenario_line_total,
					adjustment_type, note, created_at
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
					$14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, NOW()
				)
			`, platform.MustUUID(), workpaperID, line.InvoiceID, line.InvoiceItemID,
				line.ManagementSequence, line.InvoiceNumber, line.BranchID, line.BranchName,
				line.IssuedAt, line.PaymentType, line.CashPaymentAmount, line.TransferPaymentAmount,
				line.TaxInvoiceType, line.ProductID, line.SKU,
				line.ProductName, line.OriginalStockBucket, line.Quantity,
				line.AllocatedGhostQuantity, line.RepricedQuantity, line.OriginalUnitPrice,
				line.CostSnapshot, line.ProposedUnitPrice, line.TaxRate, line.OriginalLineTotal,
				line.ScenarioLineTotal, line.AdjustmentType, line.Note); err != nil {
				return err
			}
		}

		meta.EntityType = "month_end_workpaper"
		meta.EntityID = &workpaperID
		meta.Action = "month_end.calculate"
		meta.After = map[string]any{
			"workpaper_number": number, "period_start": calculation.PeriodStart,
			"period_end": calculation.PeriodEnd, "actual_revenue": calculation.ActualRevenue,
			"target_revenue": calculation.TargetRevenue, "scenario_revenue": calculation.ScenarioRevenue,
		}
		return s.audit.Log(ctx, tx, meta)
	})
	return workpaperID, err
}

func (s *Service) Finalize(ctx context.Context, user platform.AuthUser, meta audit.LogEntry, id string) error {
	return platform.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var status, sourceHash, branchID, periodStart, version string
		var targetRevenue, markupPercent float64
		if err := tx.QueryRowContext(ctx, `
			SELECT status, source_hash, COALESCE(branch_id::text, ''), period_start::text,
			       target_revenue, markup_percent, calculation_version
			FROM month_end_workpapers
			WHERE id = $1
			FOR UPDATE
		`, id).Scan(&status, &sourceHash, &branchID, &periodStart, &targetRevenue, &markupPercent, &version); err != nil {
			if err == sql.ErrNoRows {
				return platform.NewError(http.StatusNotFound, "ไม่พบกระดาษทำการปิดเดือน")
			}
			return err
		}
		if status != "APPROVED" && status != "DRAFT" {
			return platform.NewError(http.StatusConflict, "กระดาษทำการนี้ยืนยันแล้ว")
		}
		if version != calculationVersion {
			return platform.NewError(http.StatusConflict, "ผลคำนวณเป็นเวอร์ชันเก่า กรุณาคำนวณใหม่ก่อนยืนยัน")
		}
		var finalizedExists bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM month_end_workpapers
				WHERE status = 'CLOSED' AND period_start = $1::date
				  AND COALESCE(branch_id::text, '') = $2 AND id <> $3
			)
		`, periodStart, branchID, id).Scan(&finalizedExists); err != nil {
			return err
		}
		if finalizedExists {
			return platform.NewError(http.StatusConflict, "เดือนและขอบเขตสาขานี้มีรายการปิดเดือนที่ยืนยันแล้ว")
		}

		calculation, err := s.buildCalculation(ctx, tx, Input{
			Month: periodStart[:7], BranchID: branchID, TargetRevenue: targetRevenue, MarkupPercent: markupPercent,
		})
		if err != nil {
			return err
		}
		if calculation.SourceHash != sourceHash {
			return platform.NewError(http.StatusConflict, "ข้อมูลขายหรือสต๊อกเปลี่ยนหลังคำนวณ กรุณาสร้างผลคำนวณใหม่")
		}

		rows, err := tx.QueryContext(ctx, `
			SELECT branch_id::text, product_id::text, SUM(allocated_ghost_quantity)::int
			FROM month_end_workpaper_lines
			WHERE workpaper_id = $1 AND included = TRUE AND allocated_ghost_quantity > 0
			GROUP BY branch_id, product_id
			ORDER BY branch_id, product_id
		`, id)
		if err != nil {
			return err
		}
		type reclassification struct {
			branchID, productID string
			quantity            int
		}
		entries := []reclassification{}
		for rows.Next() {
			var entry reclassification
			if err := rows.Scan(&entry.branchID, &entry.productID, &entry.quantity); err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, entry)
		}
		if err := rows.Close(); err != nil {
			return err
		}

		for _, entry := range entries {
			var qtyReal, qtyGhost int
			if err := tx.QueryRowContext(ctx, `
				SELECT qty_real, qty_ghost FROM inventory
				WHERE branch_id = $1 AND product_id = $2 FOR UPDATE
			`, entry.branchID, entry.productID).Scan(&qtyReal, &qtyGhost); err != nil {
				return err
			}
			if qtyGhost < entry.quantity {
				return platform.NewError(http.StatusConflict, "สต๊อกผีไม่พอสำหรับยืนยัน กรุณาคำนวณใหม่")
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE inventory SET qty_real = qty_real + $3, qty_ghost = qty_ghost - $3, updated_at = NOW()
				WHERE branch_id = $1 AND product_id = $2
			`, entry.branchID, entry.productID, entry.quantity); err != nil {
				return err
			}
			outID, inID := platform.MustUUID(), platform.MustUUID()
			for _, movement := range []struct {
				id, bucket string
				delta      int
			}{{outID, "ghost", -entry.quantity}, {inID, "real", entry.quantity}} {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO inventory_movements (
						id, branch_id, product_id, movement_type, stock_bucket, quantity_delta,
						reference_type, reference_id, note, performed_by, created_at
					) VALUES ($1, $2, $3, 'month_end_reclassification', $4, $5,
					          'month_end_workpaper', $6, $7, $8, NOW())
				`, movement.id, entry.branchID, entry.productID, movement.bucket,
					movement.delta, id, disclaimer, user.ID); err != nil {
					return err
				}
			}
			if err := reclassifyLots(ctx, tx, entry.branchID, entry.productID, "ghost", "real", entry.quantity, outID, inID); err != nil {
				return err
			}
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE month_end_workpapers
			SET status = 'CLOSED', current_step = 6, finalized_by = $2, finalized_at = NOW(),
			    closed_by = $2, closed_at = NOW(), updated_at = NOW()
			WHERE id = $1
		`, id, user.ID); err != nil {
			return err
		}
		meta.EntityType = "month_end_workpaper"
		meta.EntityID = &id
		meta.Action = "month_end.finalize"
		meta.Before = map[string]any{"status": status}
		meta.After = map[string]any{"status": "CLOSED", "stock_reclassification_count": len(entries)}
		return s.audit.Log(ctx, tx, meta)
	})
}

func (s *Service) buildCalculation(ctx context.Context, db platform.DBTX, input Input) (Calculation, error) {
	monthStart, err := time.Parse("2006-01", strings.TrimSpace(input.Month))
	if err != nil {
		return Calculation{}, platform.NewError(http.StatusBadRequest, "กรุณาเลือกเดือนให้ถูกต้อง")
	}
	monthEndExclusive := monthStart.AddDate(0, 1, 0)
	if input.TargetRevenue < 0 {
		return Calculation{}, platform.NewError(http.StatusBadRequest, "ยอดที่ต้องการแสดงต้องไม่น้อยกว่าศูนย์")
	}
	if input.MarkupPercent < 0 || input.MarkupPercent > 1000 {
		return Calculation{}, platform.NewError(http.StatusBadRequest, "เปอร์เซ็นต์กำไรต้องอยู่ระหว่าง 0 ถึง 1000")
	}
	calculation := Calculation{
		BranchID: input.BranchID, BranchName: "ทุกสาขา", PeriodStart: monthStart.Format("2006-01-02"),
		PeriodEnd: monthEndExclusive.AddDate(0, 0, -1).Format("2006-01-02"), TargetRevenue: platform.Round2(input.TargetRevenue),
		MarkupPercent: platform.Round2(input.MarkupPercent), Disclaimer: disclaimer, Lines: []Line{},
	}
	if strings.TrimSpace(input.BranchID) != "" {
		if err := db.QueryRowContext(ctx, `SELECT name FROM branches WHERE id = $1 AND active = TRUE`, input.BranchID).Scan(&calculation.BranchName); err != nil {
			if err == sql.ErrNoRows {
				return Calculation{}, platform.NewError(http.StatusBadRequest, "ไม่พบสาขาที่เลือก")
			}
			return Calculation{}, err
		}
	}

	args := []any{monthStart, monthEndExclusive}
	branchCondition := ""
	if strings.TrimSpace(input.BranchID) != "" {
		args = append(args, input.BranchID)
		branchCondition = " AND i.branch_id = $3"
	}
	rows, err := db.QueryContext(ctx, `
		SELECT i.id::text, ii.id::text, i.invoice_number, i.branch_id::text, b.name,
		       i.issued_at, i.total_amount, i.updated_at,
		       CASE
		         WHEN COUNT(ip.id) = 0 THEN 'unpaid'
		         WHEN COALESCE(SUM(ip.amount), 0) < i.total_amount THEN 'unpaid'
		         WHEN BOOL_AND(ip.payment_type = 'cash') AND SUM(ip.amount) >= i.total_amount THEN 'cash'
		         WHEN BOOL_AND(ip.payment_type = 'bank_transfer') AND SUM(ip.amount) >= i.total_amount THEN 'bank_transfer'
		         ELSE 'mixed'
		       END AS payment_type,
		       COALESCE(SUM(ip.amount) FILTER (WHERE ip.payment_type = 'cash'), 0),
		       COALESCE(SUM(ip.amount) FILTER (WHERE ip.payment_type = 'bank_transfer'), 0),
		       i.tax_invoice_type, ii.product_id::text, p.sku, ii.actual_product_name,
		       ii.stock_bucket, ii.quantity, ii.unit_price, ii.cost_snapshot, ii.tax_rate,
		       ii.line_total, COALESCE(inv.qty_ghost, 0)
		FROM invoices i
		INNER JOIN invoice_items ii ON ii.invoice_id = i.id
		INNER JOIN branches b ON b.id = i.branch_id
		INNER JOIN products p ON p.id = ii.product_id
		LEFT JOIN invoice_payments ip ON ip.invoice_id = i.id
		LEFT JOIN inventory inv ON inv.branch_id = i.branch_id AND inv.product_id = ii.product_id
		WHERE i.invoice_status = 'issued' AND i.deleted_at IS NULL AND i.issued_at >= $1 AND i.issued_at < $2`+branchCondition+`
		GROUP BY i.id, ii.id, b.name, p.sku, inv.qty_ghost
		ORDER BY i.issued_at, i.invoice_number, ii.created_at, ii.id
	`, args...)
	if err != nil {
		return Calculation{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var line Line
		if err := rows.Scan(
			&line.InvoiceID, &line.InvoiceItemID, &line.InvoiceNumber, &line.BranchID,
			&line.BranchName, &line.IssuedAt, &line.invoiceTotal, &line.invoiceUpdatedAt,
			&line.PaymentType, &line.CashPaymentAmount, &line.TransferPaymentAmount,
			&line.TaxInvoiceType, &line.ProductID, &line.SKU,
			&line.ProductName, &line.OriginalStockBucket, &line.Quantity,
			&line.OriginalUnitPrice, &line.CostSnapshot, &line.TaxRate,
			&line.OriginalLineTotal, &line.GhostAvailable,
		); err != nil {
			return Calculation{}, err
		}
		line.ProposedUnitPrice = line.OriginalUnitPrice
		line.ScenarioLineTotal = line.OriginalLineTotal
		line.AdjustmentType = "included"
		line.Included = true
		if line.TaxInvoiceType == "full" {
			line.AdjustmentType = "locked_full_tax"
			line.Note = "ใบกำกับภาษีเต็มรูปถูกล็อกและคงอยู่ในยอดจริง"
		}
		calculation.Lines = append(calculation.Lines, line)
	}
	if err := rows.Err(); err != nil {
		return Calculation{}, err
	}

	invoiceSeen := map[string]bool{}
	fullTaxSeen := map[string]bool{}
	cashSeen := map[string]bool{}
	sequenceByInvoice := map[string]int{}
	for index := range calculation.Lines {
		line := &calculation.Lines[index]
		if !invoiceSeen[line.InvoiceID] {
			invoiceSeen[line.InvoiceID] = true
			calculation.ActualRevenue += line.invoiceTotal
			calculation.ActualInvoiceCount++
			sequenceByInvoice[line.InvoiceID] = len(sequenceByInvoice) + 1
		}
		line.ManagementSequence = sequenceByInvoice[line.InvoiceID]
		if line.TaxInvoiceType == "full" && !fullTaxSeen[line.InvoiceID] {
			fullTaxSeen[line.InvoiceID] = true
			calculation.FullTaxRevenue += line.invoiceTotal
			calculation.FullTaxInvoiceCount++
		}
		if (line.PaymentType == "cash" || line.PaymentType == "mixed") &&
			line.CashPaymentAmount > 0 && !cashSeen[line.InvoiceID] {
			cashSeen[line.InvoiceID] = true
			calculation.CashRevenue += line.CashPaymentAmount
			calculation.CashInvoiceCount++
		}
	}
	calculation.ActualRevenue = platform.Round2(calculation.ActualRevenue)
	calculation.FullTaxRevenue = platform.Round2(calculation.FullTaxRevenue)
	calculation.CashRevenue = platform.Round2(calculation.CashRevenue)
	if calculation.TargetRevenue > calculation.ActualRevenue {
		return Calculation{}, platform.NewError(http.StatusBadRequest, "ยอดที่ต้องการแสดงต้องไม่เกินยอดขายจริง")
	}
	calculateScenario(&calculation)
	calculation.SourceHash = sourceHash(calculation.Lines)
	return calculation, nil
}

func calculateScenario(calculation *Calculation) {
	result := calculateDeterministicWithStrategy(
		*calculation,
		centsFromFloat(calculation.TargetRevenue),
		centsFromFloat(calculation.MarkupPercent),
		"closest_then_oldest",
	)
	*calculation = result.Calculation
}

func closestQuantity(remaining, unitAmount float64, limit int) int {
	if remaining <= 0 || unitAmount <= 0 || limit <= 0 {
		return 0
	}
	base := int(math.Floor(remaining / unitAmount))
	if base > limit {
		base = limit
	}
	if base < 0 {
		base = 0
	}
	best := base
	if base < limit {
		withoutExtra := math.Abs(remaining - float64(base)*unitAmount)
		withExtra := math.Abs(remaining - float64(base+1)*unitAmount)
		if withExtra < withoutExtra {
			best = base + 1
		}
	}
	return best
}

func sourceHash(lines []Line) string {
	hash := sha256.New()
	for _, line := range lines {
		_, _ = fmt.Fprintf(hash, "%s|%s|%s|%.2f|%s|%s|%.2f|%.2f|%s|%d|%.2f|%.2f|%.2f|%.2f|%d|%s;",
			line.InvoiceID, line.InvoiceItemID, line.InvoiceNumber, line.invoiceTotal,
			line.invoiceUpdatedAt.UTC().Format(time.RFC3339Nano), line.PaymentType,
			line.CashPaymentAmount, line.TransferPaymentAmount, line.TaxInvoiceType,
			line.Quantity, line.OriginalUnitPrice, line.CostSnapshot,
			line.OriginalLineTotal, line.TaxRate, line.GhostAvailable, line.OriginalStockBucket)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *Service) List(ctx context.Context, includeCancelled bool) ([]map[string]any, error) {
	query := `
		SELECT w.id::text, w.workpaper_number, COALESCE(w.branch_id::text, ''),
		       COALESCE(b.name, 'ทุกสาขา'), w.period_start, w.period_end,
		       w.actual_revenue, w.target_revenue, w.scenario_revenue,
		       w.unresolved_difference, w.status, w.created_at, w.finalized_at,
		       w.lock_version, w.cancelled_at, w.cancel_reason
		FROM month_end_workpapers w
		LEFT JOIN branches b ON b.id = w.branch_id
	`
	if !includeCancelled {
		query += " WHERE w.status <> 'CANCELLED'"
	}
	query += `
		ORDER BY w.period_start DESC, w.created_at DESC
		LIMIT 200
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, number, branchID, branchName, status string
		var periodStart, periodEnd, createdAt time.Time
		var actual, target, scenario, difference float64
		var lockVersion int
		var finalizedAt, cancelledAt sql.NullTime
		var cancelReason string
		if err := rows.Scan(&id, &number, &branchID, &branchName, &periodStart, &periodEnd,
			&actual, &target, &scenario, &difference, &status, &createdAt, &finalizedAt,
			&lockVersion, &cancelledAt, &cancelReason); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id": id, "workpaper_number": number, "branch_id": branchID, "branch_name": branchName,
			"period_start": periodStart.Format("2006-01-02"), "period_end": periodEnd.Format("2006-01-02"),
			"actual_revenue": actual, "target_revenue": target, "scenario_revenue": scenario,
			"unresolved_difference": difference, "status": status, "created_at": createdAt,
			"lock_version": lockVersion,
		}
		if finalizedAt.Valid {
			item["finalized_at"] = finalizedAt.Time
		}
		if cancelledAt.Valid {
			item["cancelled_at"] = cancelledAt.Time
			item["cancel_reason"] = cancelReason
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (map[string]any, error) {
	var number, branchID, branchName, periodStart, periodEnd, sourceHash, status, workpaperDisclaimer string
	var target, markup, actual, fullTax, cash, requested, ghost, repricing, scenario, difference float64
	var invoiceCount, fullTaxCount, cashCount int
	var createdAt time.Time
	var finalizedAt sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
		SELECT w.workpaper_number, COALESCE(w.branch_id::text, ''), COALESCE(b.name, 'ทุกสาขา'),
		       w.period_start::text, w.period_end::text, w.target_revenue, w.markup_percent,
		       w.actual_revenue, w.actual_invoice_count, w.full_tax_revenue,
		       w.full_tax_invoice_count, w.cash_revenue, w.cash_invoice_count,
		       w.requested_reduction, w.ghost_reclassification_amount,
		       w.price_scenario_reduction_amount, w.scenario_revenue,
		       w.unresolved_difference, w.source_hash, w.status, w.disclaimer,
		       w.created_at, w.finalized_at
		FROM month_end_workpapers w
		LEFT JOIN branches b ON b.id = w.branch_id
		WHERE w.id = $1
	`, id).Scan(&number, &branchID, &branchName, &periodStart, &periodEnd, &target, &markup,
		&actual, &invoiceCount, &fullTax, &fullTaxCount, &cash, &cashCount, &requested,
		&ghost, &repricing, &scenario, &difference, &sourceHash, &status,
		&workpaperDisclaimer, &createdAt, &finalizedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, platform.NewError(http.StatusNotFound, "ไม่พบกระดาษทำการปิดเดือน")
		}
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, invoice_id::text, invoice_item_id::text, management_sequence,
		       invoice_number_snapshot, branch_id::text, branch_name_snapshot,
		       issued_at_snapshot, payment_type_snapshot, cash_payment_amount_snapshot,
		       bank_transfer_payment_amount_snapshot, tax_invoice_type_snapshot,
		       product_id::text, sku_snapshot, product_name_snapshot, original_stock_bucket,
		       quantity, allocated_ghost_quantity, repriced_quantity, original_unit_price,
		       cost_snapshot, proposed_unit_price, tax_rate, original_line_total,
		       scenario_line_total, adjustment_type, note, included, reason, rejected_reason
		FROM month_end_workpaper_lines
		WHERE workpaper_id = $1
		ORDER BY management_sequence, issued_at_snapshot, invoice_item_id
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := []Line{}
	for rows.Next() {
		var line Line
		if err := rows.Scan(&line.ID, &line.InvoiceID, &line.InvoiceItemID, &line.ManagementSequence,
			&line.InvoiceNumber, &line.BranchID, &line.BranchName, &line.IssuedAt,
			&line.PaymentType, &line.CashPaymentAmount, &line.TransferPaymentAmount,
			&line.TaxInvoiceType, &line.ProductID, &line.SKU,
			&line.ProductName, &line.OriginalStockBucket, &line.Quantity,
			&line.AllocatedGhostQuantity, &line.RepricedQuantity, &line.OriginalUnitPrice,
			&line.CostSnapshot, &line.ProposedUnitPrice, &line.TaxRate,
			&line.OriginalLineTotal, &line.ScenarioLineTotal, &line.AdjustmentType,
			&line.Note, &line.Included, &line.Reason, &line.RejectedReason); err != nil {
			return nil, err
		}
		populateLineSemantics(&line)
		lines = append(lines, line)
	}
	item := map[string]any{
		"id": id, "workpaper_number": number, "branch_id": branchID, "branch_name": branchName,
		"period_start": periodStart, "period_end": periodEnd, "target_revenue": target,
		"markup_percent": markup, "actual_revenue": actual, "actual_invoice_count": invoiceCount,
		"full_tax_revenue": fullTax, "full_tax_invoice_count": fullTaxCount,
		"cash_revenue": cash, "cash_invoice_count": cashCount, "requested_reduction": requested,
		"ghost_reclassification_amount": ghost, "price_scenario_reduction_amount": repricing,
		"scenario_revenue": scenario, "unresolved_difference": difference, "source_hash": sourceHash,
		"status": status, "disclaimer": workpaperDisclaimer, "created_at": createdAt,
		"transactions": lines,
	}
	if finalizedAt.Valid {
		item["finalized_at"] = finalizedAt.Time
	}
	return item, rows.Err()
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// Sorted keys keep grouped stock operations deterministic when this helper is
// reused by tests or future exports.
func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Preview(c echo.Context) error {
	var input Input
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลคำนวณไม่ถูกต้อง"))
	}
	result, err := h.service.Preview(c.Request().Context(), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) Source(c echo.Context) error {
	result, err := h.service.Source(c.Request().Context(), c.QueryParam("month"), c.QueryParam("branch_id"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) Create(c echo.Context) error {
	var input Input
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "ข้อมูลคำนวณไม่ถูกต้อง"))
	}
	id, err := h.service.Create(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	item, err := h.service.Get(c.Request().Context(), id)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, map[string]any{"item": item, "message": "บันทึกผลคำนวณปิดเดือนแล้ว"})
}

func (h *Handler) List(c echo.Context) error {
	rawIncludeCancelled := strings.TrimSpace(c.QueryParam("include_cancelled"))
	if rawIncludeCancelled != "" && !strings.EqualFold(rawIncludeCancelled, "true") && !strings.EqualFold(rawIncludeCancelled, "false") {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "include_cancelled ต้องเป็น true หรือ false"))
	}
	includeCancelled := strings.EqualFold(rawIncludeCancelled, "true")
	items, err := h.service.List(c.Request().Context(), includeCancelled)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Get(c echo.Context) error {
	item, err := h.service.Get(c.Request().Context(), c.Param("workpaperID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, item)
}

func (h *Handler) Finalize(c echo.Context) error {
	if err := h.service.Finalize(c.Request().Context(), platform.CurrentUser(c), audit.MetaFromContext(c), c.Param("workpaperID")); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	item, err := h.service.Get(c.Request().Context(), c.Param("workpaperID"))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"item": item, "message": "ยืนยันปิดเดือนและบันทึกการจำแนกสต๊อกแล้ว"})
}
