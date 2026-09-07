package monthend

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// The ways a day's takings can end up, named for what the operator sees on the
// dashboard. The first five are money the close will not touch; the last three
// are the cash-without-a-full-tax-invoice pool it will, split by what Ghost
// Stock can cover.
const (
	groupTransferAbbreviated = "transfer_abbreviated"
	groupTransferFullTax     = "transfer_full_tax"
	groupCashFullTax         = "cash_full_tax"
	groupMixedAbbreviated    = "mixed_abbreviated"
	groupMixedFullTax        = "mixed_full_tax"
	groupCashGhostHidden     = "cash_ghost_hidden"
	groupCashRepriced        = "cash_repriced"
	groupCashMixed           = "cash_mixed"
	groupUnpaid              = "unpaid"
)

// realGroups is what "ยอดรวมจริง" adds up. The two mixed-tender groups are here
// because a bill settled part in cash and part by transfer is not a candidate —
// the rule only acts on cash-only bills — and leaving them out of every group
// silently dropped them off the page along with their money.
var realGroups = []string{
	groupTransferAbbreviated, groupTransferFullTax, groupCashFullTax,
	groupMixedAbbreviated, groupMixedFullTax,
}
var closeGroups = []string{groupCashGhostHidden, groupCashRepriced, groupCashMixed}

// breakdownTally accumulates in satang, so a day of rounding cannot drift.
//
// Every group carries both sides of the close: what was rung up, and what
// survives it. Before the round is closed the second figure is a projection;
// after it, it is what actually happened. The shape is the same either way, so
// the screen reads the same and only its heading changes.
type breakdownTally struct {
	before      map[string]int64
	after       map[string]int64
	beforeCount map[string]int
	afterCount  map[string]int
	// Tender is the split central_admin works from: a mixed bill puts its cash
	// part on one side and its transfer part on the other, so the two columns
	// add up to the money that actually arrived.
	cashCents     int64
	transferCents int64
	tenderCount   int
}

func newBreakdownTally() *breakdownTally {
	return &breakdownTally{
		before: map[string]int64{}, after: map[string]int64{},
		beforeCount: map[string]int{}, afterCount: map[string]int{},
	}
}

func (t *breakdownTally) add(group string, before, after int64, survives bool) {
	t.before[group] += before
	t.beforeCount[group]++
	if survives {
		t.after[group] += after
		t.afterCount[group]++
	}
}

func (t *breakdownTally) addTender(cash, transfer int64) {
	t.cashCents += cash
	t.transferCents += transfer
	t.tenderCount++
}

func groupEntry(before, after int64, beforeCount, afterCount int) map[string]any {
	return map[string]any{
		"amount":           centsToFloat(before),
		"invoice_count":    beforeCount,
		"after_amount":     centsToFloat(after),
		"after_count":      afterCount,
		"difference":       centsToFloat(before - after),
		"difference_count": beforeCount - afterCount,
		// Kept for the pre-close reading, where "what it would become" is the
		// natural label rather than "what it became".
		"adjusted_amount": centsToFloat(after),
		"reduction":       centsToFloat(before - after),
	}
}

// render turns the tally into the payload. Groups the caller may not see are
// dropped rather than zeroed, so a missing number never reads as "there were
// none".
func (t *breakdownTally) render(groups []string) map[string]any {
	out := map[string]any{}
	var realBefore, realAfter, closeBefore, closeAfter int64
	var realBeforeCount, realAfterCount, closeBeforeCount, closeAfterCount int
	for _, group := range groups {
		out[group] = groupEntry(t.before[group], t.after[group], t.beforeCount[group], t.afterCount[group])
		switch {
		case contains(realGroups, group):
			realBefore += t.before[group]
			realAfter += t.after[group]
			realBeforeCount += t.beforeCount[group]
			realAfterCount += t.afterCount[group]
		case contains(closeGroups, group):
			closeBefore += t.before[group]
			closeAfter += t.after[group]
			closeBeforeCount += t.beforeCount[group]
			closeAfterCount += t.afterCount[group]
		}
	}
	out["real_total"] = groupEntry(realBefore, realAfter, realBeforeCount, realAfterCount)
	if contains(groups, groupCashGhostHidden) {
		out["close_total"] = groupEntry(closeBefore, closeAfter, closeBeforeCount, closeAfterCount)
		out["day_total"] = groupEntry(realBefore+closeBefore, realAfter+closeAfter,
			realBeforeCount+closeBeforeCount, realAfterCount+closeAfterCount)
	}
	out["tender"] = map[string]any{
		"cash_amount":     centsToFloat(t.cashCents),
		"transfer_amount": centsToFloat(t.transferCents),
		"total_amount":    centsToFloat(t.cashCents + t.transferCents),
		"invoice_count":   t.tenderCount,
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

// DailyBreakdown is the dashboard's picture of a period — by default today.
//
// Before the round is closed it answers the question the close will ask, but
// asked now and against live Ghost Stock: of what has been rung up, how much
// would be left alone, removed, or re-recorded at cost plus the markup. It runs
// the close's own source load and plan rather than a second query of its own,
// so the dashboard and สรุปสิ้นเดือน cannot drift apart.
//
// Once the round is closed the same shape is filled from what the close
// recorded instead. A hidden bill is soft-deleted and a repriced one has been
// rewritten, so reading the live rows alone would show the day shrinking to
// nothing; the snapshot the close froze is what the "before" column is for, and
// it is why the day stays auditable after the fact.
//
// Nothing is snapshotted by this endpoint. Bills carry issued_at, so an open day
// can be recomputed exactly whenever it is asked for; the only stored history is
// the one the close itself writes.
func (s *Service) DailyBreakdown(ctx context.Context, user platform.AuthUser, input ReconciliationInput) (map[string]any, error) {
	// Ghost Stock is Superadmin-only, and three of the groups are defined by it.
	// central_admin gets the tender split and is not told the rest exists.
	showClose := user.RoleKey == "super_admin"
	groups := append([]string{}, realGroups...)
	if showClose {
		groups = append(groups, closeGroups...)
	}
	groups = append(groups, groupUnpaid)

	periodStart, periodEnd, _, _, err := reconciliationRange(input)
	if err != nil {
		return nil, err
	}
	branchIDs, err := s.breakdownBranchIDs(ctx, input.BranchIDs)
	if err != nil {
		return nil, err
	}

	round, err := s.closedRoundCovering(ctx, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}

	overall := newBreakdownTally()
	byBranch := map[string]*breakdownTally{}
	branchInfo := map[string][3]string{}
	// Every branch in scope appears, at zero if it sold nothing — a branch that
	// vanishes on a quiet morning reads as a broken page, not a quiet morning.
	if err := s.seedBreakdownBranches(ctx, branchIDs, byBranch, branchInfo); err != nil {
		return nil, err
	}
	tallyFor := func(branchID string) *breakdownTally {
		tally, ok := byBranch[branchID]
		if !ok {
			tally = newBreakdownTally()
			byBranch[branchID] = tally
		}
		return tally
	}

	markup := input.AdjustmentPercent
	if markup <= 0 {
		markup = 5
	}
	if round != nil {
		markup = round.markupPercent
		if err := s.fillFromClosedRound(ctx, round.id, branchIDs, periodStart, periodEnd, overall, tallyFor); err != nil {
			return nil, err
		}
	} else if err := s.fillFromLivePlan(ctx, input, overall, tallyFor); err != nil {
		return nil, err
	}

	// The close only ever looks at paid bills, so unpaid ones are invisible to
	// both paths above. On a dashboard that would silently drop them from the
	// day, which is the shape of the discrepancy this screen exists to avoid.
	if err := s.addUnpaidToBreakdown(ctx, branchIDs, periodStart, periodEnd, overall, tallyFor); err != nil {
		return nil, err
	}
	if err := s.addTenderSplit(ctx, branchIDs, periodStart, periodEnd, overall, tallyFor); err != nil {
		return nil, err
	}

	ordered := make([]string, 0, len(byBranch))
	for id := range byBranch {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := byBranch[ordered[i]], byBranch[ordered[j]]
		leftTotal, rightTotal := int64(0), int64(0)
		for _, group := range groups {
			leftTotal += left.before[group]
			rightTotal += right.before[group]
		}
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return branchInfo[ordered[i]][2] < branchInfo[ordered[j]][2]
	})
	branches := make([]map[string]any, 0, len(ordered))
	for _, id := range ordered {
		entry := byBranch[id].render(groups)
		entry["branch_id"] = branchInfo[id][0]
		entry["branch_code"] = branchInfo[id][1]
		entry["branch_name"] = branchInfo[id][2]
		branches = append(branches, entry)
	}

	// PeriodStart/End are UTC instants bounding a Bangkok day, so they must be
	// read back in Bangkok or the answer names the day before.
	bangkok := time.FixedZone("Asia/Bangkok", 7*60*60)
	payload := map[string]any{
		"date_from":      periodStart.In(bangkok).Format("2006-01-02"),
		"date_to":        periodEnd.In(bangkok).AddDate(0, 0, -1).Format("2006-01-02"),
		"markup_percent": markup,
		"shows_close":    showClose,
		"closed":         round != nil,
		"overall":        overall.render(groups),
		"branches":       branches,
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
	}
	if round != nil && showClose {
		payload["reconciliation_number"] = round.number
		payload["reconciliation_id"] = round.id
		payload["period_start"] = round.periodStart.In(bangkok).Format("2006-01-02")
		payload["period_end"] = round.periodEnd.In(bangkok).Format("2006-01-02")
	}
	return payload, nil
}

type closedRound struct {
	id            string
	number        string
	periodStart   time.Time
	periodEnd     time.Time
	markupPercent float64
}

// closedRoundCovering finds the round that already settled this window. Only a
// round that covers the whole window counts: a half-closed range would mix a
// record with a projection and the two columns would mean different things in
// the same row.
func (s *Service) closedRoundCovering(ctx context.Context, periodStart, periodEnd time.Time) (*closedRound, error) {
	bangkok := time.FixedZone("Asia/Bangkok", 7*60*60)
	from := periodStart.In(bangkok).Format("2006-01-02")
	to := periodEnd.In(bangkok).AddDate(0, 0, -1).Format("2006-01-02")
	round := closedRound{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, reconciliation_number, period_start, period_end, adjustment_percent
		FROM month_end_reconciliations
		WHERE period_start <= $1::date AND period_end >= $2::date
		ORDER BY finalized_at DESC LIMIT 1
	`, from, to).Scan(&round.id, &round.number, &round.periodStart, &round.periodEnd, &round.markupPercent)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &round, nil
}

// fillFromClosedRound reads what the close did rather than predicting it again.
// A bill's group is a fact recorded across three places: the snapshot says how
// it was paid for and what it was worth, deleted_at says whether it survived,
// and a struck line says the close took part of it away.
func (s *Service) fillFromClosedRound(ctx context.Context, reconciliationID string, branchIDs []string, periodStart, periodEnd time.Time, overall *breakdownTally, tallyFor func(string) *breakdownTally) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.branch_id::text,
		       s.payment_method,
		       s.request_full_tax_invoice,
		       s.original_total_amount,
		       COALESCE(i.total_amount, 0),
		       i.deleted_at IS NOT NULL AS removed,
		       EXISTS(
		           SELECT 1 FROM invoice_items x
		           WHERE x.invoice_id = s.invoice_id AND x.reconciliation_removed_at IS NOT NULL
		       ) AS partly_removed
		FROM reconciliation_invoice_snapshots s
		INNER JOIN invoices i ON i.id = s.invoice_id
		WHERE s.reconciliation_id = $1
		  AND s.branch_id = ANY($2::uuid[])
		  AND s.invoice_created_at >= $3 AND s.invoice_created_at < $4
	`, reconciliationID, pq.Array(branchIDs), periodStart, periodEnd)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var branchID, paymentMethod string
		var fullTax, removed, partlyRemoved bool
		var before, after float64
		if err := rows.Scan(&branchID, &paymentMethod, &fullTax, &before, &after, &removed, &partlyRemoved); err != nil {
			return err
		}
		group := recordedGroup(paymentMethod, fullTax, removed, partlyRemoved, before, after)
		if group == "" {
			continue
		}
		beforeCents := centsFromFloat(before)
		afterCents := centsFromFloat(after)
		if removed {
			afterCents = 0
		}
		overall.add(group, beforeCents, afterCents, !removed)
		tallyFor(branchID).add(group, beforeCents, afterCents, !removed)
	}
	return rows.Err()
}

// recordedGroup names the box a bill sits in once the close has run.
func recordedGroup(paymentMethod string, fullTax, removed, partlyRemoved bool, before, after float64) string {
	if paymentMethod == "cash" && !fullTax {
		switch {
		case removed:
			return groupCashGhostHidden
		case partlyRemoved:
			// Some lines struck, the rest re-recorded — one bill, both rules.
			return groupCashMixed
		default:
			// Repriced, or a candidate the markup happened to leave alone.
			return groupCashRepriced
		}
	}
	switch paymentMethod {
	case "bank_transfer":
		if fullTax {
			return groupTransferFullTax
		}
		return groupTransferAbbreviated
	case "cash":
		return groupCashFullTax
	case "mixed":
		if fullTax {
			return groupMixedFullTax
		}
		return groupMixedAbbreviated
	}
	return ""
}

// fillFromLivePlan runs the close's own plan against a period it has not settled
// yet, so the dashboard shows what would happen if the round were closed now.
func (s *Service) fillFromLivePlan(ctx context.Context, input ReconciliationInput, overall *breakdownTally, tallyFor func(string) *breakdownTally) error {
	source, err := s.loadReconciliationSource(ctx, s.db, input)
	if err != nil {
		return err
	}
	if _, err := applyCostMarkupPlan(&source, input.AdjustmentPercent); err != nil {
		return err
	}
	for _, invoice := range source.Invoices {
		group := plannedGroup(invoice)
		if group == "" {
			continue
		}
		before := centsFromFloat(invoice.TotalAmount)
		after := centsFromFloat(invoice.FinalTotal)
		survives := !invoice.WillSuppress
		if !survives {
			after = 0
		}
		overall.add(group, before, after, survives)
		tallyFor(invoice.BranchID).add(group, before, after, survives)
	}
	return nil
}

// plannedGroup reads back the decision the plan just made about a bill.
func plannedGroup(invoice *reconciliationInvoice) string {
	if invoice.SuppressionCandidate {
		switch {
		case invoice.WillSuppress:
			return groupCashGhostHidden
		case invoice.SuppressedItemCount > 0:
			return groupCashMixed
		default:
			return groupCashRepriced
		}
	}
	return recordedGroup(invoice.PaymentMethod, invoice.RequestFullTaxInvoice, false, false, 0, 0)
}

func (s *Service) breakdownBranchIDs(ctx context.Context, requested []string) ([]string, error) {
	branchIDs, err := canonicalBranchIDs(requested)
	if err != nil {
		return nil, err
	}
	if len(branchIDs) > 0 {
		return branchIDs, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id::text FROM branches
		WHERE active = TRUE AND sales_enabled = TRUE AND branch_type <> 'main_warehouse'
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		branchIDs = append(branchIDs, id)
	}
	return branchIDs, rows.Err()
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

func (s *Service) addUnpaidToBreakdown(ctx context.Context, branchIDs []string, periodStart, periodEnd time.Time, overall *breakdownTally, tallyFor func(string) *breakdownTally) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.branch_id::text, i.total_amount
		FROM invoices i
		WHERE i.deleted_at IS NULL
		  AND i.invoice_status = 'issued'
		  AND i.payment_status <> 'paid'
		  AND i.created_at >= $1 AND i.created_at < $2
		  AND i.branch_id = ANY($3::uuid[])
	`, periodStart, periodEnd, pq.Array(branchIDs))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var branchID string
		var amount float64
		if err := rows.Scan(&branchID, &amount); err != nil {
			return err
		}
		cents := centsFromFloat(amount)
		overall.add(groupUnpaid, cents, cents, true)
		tallyFor(branchID).add(groupUnpaid, cents, cents, true)
	}
	return rows.Err()
}

// addTenderSplit is what central_admin's dashboard is built from: the money that
// actually arrived, split by how it arrived. A bill settled 100 in cash and 50
// by transfer contributes to both columns, which is why this reads
// invoice_payments rather than the bill's single payment_method.
//
// Only live bills count. central_admin works from the adjusted books, so a bill
// the close removed is simply not there.
func (s *Service) addTenderSplit(ctx context.Context, branchIDs []string, periodStart, periodEnd time.Time, overall *breakdownTally, tallyFor func(string) *breakdownTally) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.branch_id::text,
		       COALESCE(SUM(p.amount) FILTER (WHERE p.payment_type = 'cash'), 0),
		       COALESCE(SUM(p.amount) FILTER (WHERE p.payment_type = 'bank_transfer'), 0)
		FROM invoices i
		INNER JOIN invoice_payments p ON p.invoice_id = i.id
		WHERE i.deleted_at IS NULL
		  AND i.invoice_status = 'issued'
		  AND i.created_at >= $1 AND i.created_at < $2
		  AND i.branch_id = ANY($3::uuid[])
		GROUP BY i.id, i.branch_id
	`, periodStart, periodEnd, pq.Array(branchIDs))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var branchID string
		var cash, transfer float64
		if err := rows.Scan(&branchID, &cash, &transfer); err != nil {
			return err
		}
		cashCents, transferCents := centsFromFloat(cash), centsFromFloat(transfer)
		overall.addTender(cashCents, transferCents)
		tallyFor(branchID).addTender(cashCents, transferCents)
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
