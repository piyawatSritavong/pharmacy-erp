-- Month-end cost-markup rule.
--
-- A cash-only bill without a full tax invoice is hidden only when warehouse
-- Ghost Stock covers every line of it. The remaining cash bills stay in the
-- books, recorded at cost × (1 + markup) with the markup between 5% and 10%
-- (5% = ×1.05: 5,500 cost on a 10,000 sale is recorded as 5,775). Older
-- rounds keep their stored 0.
ALTER TABLE month_end_reconciliations
    DROP CONSTRAINT IF EXISTS month_end_reconciliations_adjustment_percent_check;
ALTER TABLE month_end_reconciliations
    ADD CONSTRAINT month_end_reconciliations_adjustment_percent_check
    CHECK (adjustment_percent >= 0 AND adjustment_percent <= 10);

ALTER TABLE month_end_reconciliations DROP CONSTRAINT IF EXISTS month_end_reconciliations_mode_check;
ALTER TABLE month_end_reconciliations
    ADD CONSTRAINT month_end_reconciliations_mode_check
    CHECK (reconciliation_mode IN ('legacy_target','hide_all_cash_no_tax','hide_ghost_covered_cost_markup'));

-- invoice_repriced records the bill-level before/after (totals and cash
-- payments) of a retained bill; price_adjusted keeps the per-line detail.
ALTER TABLE reconciliation_logs DROP CONSTRAINT IF EXISTS reconciliation_logs_log_type_check;
ALTER TABLE reconciliation_logs
    ADD CONSTRAINT reconciliation_logs_log_type_check
    CHECK (log_type IN (
        'invoice_suppressed','invoice_renumbered','invoice_repriced','price_adjusted','stock_deducted',
        'stock_reversed','stock_received','stock_deficit'
    ));
