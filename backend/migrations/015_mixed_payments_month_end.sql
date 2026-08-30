ALTER TABLE month_end_workpaper_lines
    ADD COLUMN IF NOT EXISTS cash_payment_amount_snapshot NUMERIC(14,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS bank_transfer_payment_amount_snapshot NUMERIC(14,2) NOT NULL DEFAULT 0;

ALTER TABLE month_end_workpapers
    ALTER COLUMN calculation_version SET DEFAULT '3.0.0';

WITH payment_totals AS (
    SELECT
        invoice_id,
        COALESCE(SUM(amount) FILTER (WHERE payment_type = 'cash'), 0) AS cash_amount,
        COALESCE(SUM(amount) FILTER (WHERE payment_type = 'bank_transfer'), 0) AS transfer_amount
    FROM invoice_payments
    GROUP BY invoice_id
)
UPDATE month_end_workpaper_lines AS line
SET
    cash_payment_amount_snapshot = payment_totals.cash_amount,
    bank_transfer_payment_amount_snapshot = payment_totals.transfer_amount,
    updated_at = NOW()
FROM payment_totals
INNER JOIN month_end_workpapers AS workpaper
    ON workpaper.status <> 'CLOSED'
WHERE line.workpaper_id = workpaper.id
  AND line.invoice_id = payment_totals.invoice_id;
