-- A6 (report-slot / pin system): a pinned report now targets a specific
-- page (by NavigationItem key) instead of always rendering back onto the
-- Generate Report page. Existing pins default to "generate_report", which
-- is exactly where they already render today, so this is a no-op for them.
ALTER TABLE report_definitions
    ADD COLUMN IF NOT EXISTS pin_target_key TEXT NOT NULL DEFAULT 'generate_report';

DROP INDEX IF EXISTS idx_report_definitions_owner_pin_order;
CREATE UNIQUE INDEX IF NOT EXISTS idx_report_definitions_owner_pin_target_order
    ON report_definitions (owner_user_id, pin_target_key, pin_order)
    WHERE is_pinned = TRUE;
