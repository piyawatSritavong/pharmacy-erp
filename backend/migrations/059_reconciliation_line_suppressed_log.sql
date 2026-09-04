-- A close can now strike a single line off a bill that survives, which is a new
-- kind of event: not "this bill was hidden" and not "this line was repriced".
-- The log_type check predates that case, so widen it rather than overload an
-- existing name — the report and the audit trail both read log_type to say what
-- happened, and calling a struck line a repricing would misreport it.
ALTER TABLE reconciliation_logs DROP CONSTRAINT IF EXISTS reconciliation_logs_log_type_check;

ALTER TABLE reconciliation_logs ADD CONSTRAINT reconciliation_logs_log_type_check
    CHECK (log_type = ANY (ARRAY[
        'invoice_suppressed'::text,
        'invoice_renumbered'::text,
        'invoice_repriced'::text,
        'price_adjusted'::text,
        'line_suppressed'::text,
        'stock_deducted'::text,
        'stock_reversed'::text,
        'stock_received'::text,
        'stock_deficit'::text
    ]));
