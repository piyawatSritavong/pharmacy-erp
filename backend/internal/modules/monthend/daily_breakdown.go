package monthend

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// The six ways a day's takings can end up, named for what the operator sees on
// the dashboard. The first three are money the close will not touch; the last
// three are the cash-without-a-full-tax-invoice pool it will, split by what
// Ghost Stock can cover.
const (
	groupTransferAbbreviated = "transfer_abbreviated"
	groupTransferFullTax     = "transfer_full_tax"
	groupCashFullTax         = "cash_full_tax"
	groupCashGhostHidden     = "cash_ghost_hidden"
	groupCashRepriced        = "cash_repriced"
	groupCashMixed           = "cash_mixed"
	groupUnpaid              = "unpaid"
)

// realGroups is what "ยอดรวมจริง" adds up, and the only half central_admin is
// ever shown — the other three name Ghost Stock by definition.
var realGroups = []string{groupTransferAbbreviated, groupTransferFullTax, groupCashFullTax}
var closeGroups = []string{groupCashGhostHidden, groupCashRepriced, groupCashMixed}

// breakdownTally accumulates in satang, so a day of rounding cannot drift.
type breakdownTally struct {
	original map[string]int64
	adjusted map[string]int64
	count    map[string]int
}

func newBreakdownTally() *breakdownTally {
	return &breakdownTally{original: map[string]int64{}, adjusted: map[string]int64{}, count: map[string]int{}}
}

func (t *breakdownTally) add(group string, original, adjusted int64) {
	t.original[group] += original
	t.adjusted[group] += adjusted
	t.count[group]++
}

// render turns the tally into the payload, summing the two headline figures as
// it goes. Groups the caller may not see are dropped rather than zeroed, so a
// missing number never reads as "there were none".
func (t *breakdownTally) render(groups []string) map[string]any {
	out := map[string]any{}
	var realTotal, realCount int64
	var closeOriginal, closeAdjusted, closeCount int64
	for _, group := range groups {
		entry := map[string]any{
			"amount":        centsToFloat(t.original[group]),
			"invoice_count": t.count[group],
		}
		// Only the repriceable groups carry a second figure; on the rest the
		// bill is either kept whole or removed whole.
		if group == groupCashRepriced || group == groupCashMixed {
			entry["adjusted_amount"] = centsToFloat(t.adjusted[group])
			entry["reduction"] = centsToFloat(t.original[group] - t.adjusted[group])
		}
		out[group] = entry
		switch {
		case contains(realGroups, group):
			realTotal += t.original[group]
			realCount += int64(t.count[group])
		case contains(closeGroups, group):
			closeOriginal += t.original[group]
			closeAdjusted += t.adjusted[group]
			closeCount += int64(t.count[group])
		}
	}
	out["real_total"] = map[string]any{
		"amount":        centsToFloat(realTotal),
		"invoice_count": realCount,
	}
	if contains(groups, groupCashGhostHidden) {
		out["close_total"] = map[string]any{
			"amount":          centsToFloat(closeOriginal),
			"adjusted_amount": centsToFloat(closeAdjusted),
			"reduction":       centsToFloat(closeOriginal - closeAdjusted),
			"invoice_count":   closeCount,
		}
	}
	return out
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// DailyBreakdown is the dashboard's running picture of a period — by default
// today. It answers the question the close will ask at the end of the month,
// but asked now and against live Ghost Stock: of what has been rung up, how
// much the close would leave alone, how much it would remove, and how much it
// would re-record at cost plus the markup.
//
// It runs the close's own source load and plan rather than a second query of
// its own, so the dashboard and สรุปสิ้นเดือน cannot drift apart — they did
// once already, and the fix is to have one implementation, not two that agree.
//
// Nothing here is a snapshot. Bills carry issued_at, so any day can be
// recomputed exactly whenever it is asked for; a stored daily total would only
// add a second number to disagree with.
func (s *Service) DailyBreakdown(ctx context.Context, user platform.AuthUser, input ReconciliationInput) (map[string]any, error) {
	// Ghost Stock is Superadmin-only, and three of the six groups are defined
	// by it. central_admin gets the real half and is not told the rest exists.
	showClose := user.RoleKey == "super_admin"
	groups := append([]string{}, realGroups...)
	if showClose {
		groups = append(groups, closeGroups...)
	}
	groups = append(groups, groupUnpaid)

	source, err := s.loadReconciliationSource(ctx, s.db, input)
	if err != nil {
		return nil, err
	}
	if _, err := applyCostMarkupPlan(&source, input.AdjustmentPercent); err != nil {
		return nil, err
	}

	overall := newBreakdownTally()
	byBranch := map[string]*breakdownTally{}
	branchInfo := map[string][3]string{}
	// Every branch in scope appears, at zero if it sold nothing — a branch that
	// vanishes on a quiet morning reads as a broken page, not a quiet morning.
	if err := s.seedBreakdownBranches(ctx, source.BranchIDs, byBranch, branchInfo); err != nil {
		return nil, err
	}
	tallyFor := func(invoice *reconciliationInvoice) *breakdownTally {
		tally, ok := byBranch[invoice.BranchID]
		if !ok {
			tally = newBreakdownTally()
			byBranch[invoice.BranchID] = tally
			branchInfo[invoice.BranchID] = [3]string{invoice.BranchID, invoice.BranchCode, invoice.BranchName}
		}
		return tally
	}

	for _, invoice := range source.Invoices {
		group := classifyForDashboard(invoice)
		if group == "" {
			continue
		}
		original := centsFromFloat(invoice.TotalAmount)
		adjusted := centsFromFloat(invoice.FinalTotal)
		if group == groupCashGhostHidden {
			// The whole bill goes, so what it would be worth afterwards is zero
			// however the plan left FinalTotal.
			adjusted = 0
		}
		overall.add(group, original, adjusted)
		tallyFor(invoice).add(group, original, adjusted)
	}

	// The close only ever looks at paid bills, so unpaid ones are invisible to
	// the source above. On a dashboard that would silently drop them from the
	// day, which is the shape of the discrepancy this screen exists to avoid.
	if err := s.addUnpaidToBreakdown(ctx, source, overall, byBranch, branchInfo); err != nil {
		return nil, err
	}

	branchIDs := make([]string, 0, len(byBranch))
	for id := range byBranch {
		branchIDs = append(branchIDs, id)
	}
	sort.Slice(branchIDs, func(i, j int) bool {
		left, right := byBranch[branchIDs[i]], byBranch[branchIDs[j]]
		leftTotal, rightTotal := int64(0), int64(0)
		for _, group := range groups {
			leftTotal += left.original[group]
			rightTotal += right.original[group]
		}
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return branchInfo[branchIDs[i]][2] < branchInfo[branchIDs[j]][2]
	})
	branches := make([]map[string]any, 0, len(branchIDs))
	for _, id := range branchIDs {
		entry := byBranch[id].render(groups)
		entry["branch_id"] = branchInfo[id][0]
		entry["branch_code"] = branchInfo[id][1]
		entry["branch_name"] = branchInfo[id][2]
		branches = append(branches, entry)
	}

	markup := input.AdjustmentPercent
	if markup <= 0 {
		markup = 5
	}
	// PeriodStart/End are UTC instants bounding a Bangkok day, so they must be
	// read back in Bangkok or the answer names the day before.
	bangkok := time.FixedZone("Asia/Bangkok", 7*60*60)
	return map[string]any{
		"date_from":      source.PeriodStart.In(bangkok).Format("2006-01-02"),
		"date_to":        source.PeriodEnd.In(bangkok).AddDate(0, 0, -1).Format("2006-01-02"),
		"markup_percent": markup,
		"shows_close":    showClose,
		"overall":        overall.render(groups),
		"branches":       branches,
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// classifyForDashboard names the box a bill belongs in. The close has already
// decided what it will do with each one; this only reads that decision back.
func classifyForDashboard(invoice *reconciliationInvoice) string {
	if invoice.SuppressionCandidate {
		switch {
		case invoice.WillSuppress:
			return groupCashGhostHidden
		case invoice.SuppressedItemCount > 0:
			// Some lines removed, the rest re-recorded — one bill, both rules.
			return groupCashMixed
		default:
			return groupCashRepriced
		}
	}
	switch invoice.PaymentMethod {
	case "bank_transfer":
		if invoice.RequestFullTaxInvoice {
			return groupTransferFullTax
		}
		return groupTransferAbbreviated
	case "cash":
		// Cash without a full tax invoice is always a candidate above, so this
		// is the full-tax case.
		return groupCashFullTax
	}
	return ""
}

func (s *Service) seedBreakdownBranches(ctx context.Context, branchIDs []string, byBranch map[string]*breakdownTally, branchInfo map[string][3]string) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text, code, name FROM branches WHERE id = ANY($1::uuid[]) ORDER BY name
	`, pq.Array(branchIDs))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, code, name string
		if err := rows.Scan(&id, &code, &name); err != nil {
			return err
		}
		byBranch[id] = newBreakdownTally()
		branchInfo[id] = [3]string{id, code, name}
	}
	return rows.Err()
}

func (s *Service) addUnpaidToBreakdown(ctx context.Context, source reconciliationSource, overall *breakdownTally, byBranch map[string]*breakdownTally, branchInfo map[string][3]string) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.branch_id::text, b.code, b.name, i.total_amount
		FROM invoices i
		INNER JOIN branches b ON b.id = i.branch_id
		WHERE i.deleted_at IS NULL
		  AND i.invoice_status = 'issued'
		  AND i.payment_status <> 'paid'
		  AND i.created_at >= $1 AND i.created_at < $2
		  AND i.branch_id = ANY($3::uuid[])
	`, source.PeriodStart, source.PeriodEnd, pq.Array(source.BranchIDs))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var branchID, code, name string
		var amount float64
		if err := rows.Scan(&branchID, &code, &name, &amount); err != nil {
			return err
		}
		cents := centsFromFloat(amount)
		overall.add(groupUnpaid, cents, cents)
		tally, ok := byBranch[branchID]
		if !ok {
			tally = newBreakdownTally()
			byBranch[branchID] = tally
			branchInfo[branchID] = [3]string{branchID, code, name}
		}
		tally.add(groupUnpaid, cents, cents)
	}
	return rows.Err()
}

func (h *Handler) DailyBreakdown(c echo.Context) error {
	input := ReconciliationInput{
		DateFrom:          strings.TrimSpace(c.QueryParam("date_from")),
		DateTo:            strings.TrimSpace(c.QueryParam("date_to")),
		AdjustmentPercent: 5,
	}
	if raw := strings.TrimSpace(c.QueryParam("markup_percent")); raw != "" {
		if value, err := strconv.ParseFloat(raw, 64); err == nil && value > 0 {
			input.AdjustmentPercent = value
		}
	}
	if branch := strings.TrimSpace(c.QueryParam("branch_id")); branch != "" {
		input.BranchIDs = []string{branch}
	}
	// No window asked for means today, in Bangkok — the dashboard's default is
	// "what has happened so far today", not "everything ever sold".
	if input.DateFrom == "" && input.DateTo == "" {
		today := time.Now().In(time.FixedZone("Asia/Bangkok", 7*60*60)).Format("2006-01-02")
		input.DateFrom, input.DateTo = today, today
	}
	result, err := h.service.DailyBreakdown(c.Request().Context(), platform.CurrentUser(c), input)
	if err != nil {
		return platform.HandleHTTPError(c, platform.WrapError(http.StatusInternalServerError, "โหลดสรุปยอดรายวันไม่สำเร็จ", err))
	}
	return platform.JSON(c, http.StatusOK, result)
}
