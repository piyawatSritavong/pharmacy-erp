-- A small or unavoidable variance is informational. Superadmin may explicitly
-- accept it during final confirmation; the target and final values remain in
-- the reconciliation record for audit comparison.
ALTER TABLE month_end_reconciliations
    DROP CONSTRAINT IF EXISTS month_end_reconciliations_check1;
