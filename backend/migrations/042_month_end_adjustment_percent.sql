-- Records the user-approved ceiling used by the automatic month-end price
-- adjustment planner. A retained eligible invoice can receive a smaller
-- effective discount, but never more than this percentage.
ALTER TABLE month_end_reconciliations
    ADD COLUMN IF NOT EXISTS adjustment_percent NUMERIC(5,2) NOT NULL DEFAULT 0
        CHECK (adjustment_percent >= 0 AND adjustment_percent <= 5);
