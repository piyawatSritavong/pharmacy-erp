-- Part B, Rule 3 correction — คณาเภสัช's exclusivity was only ever meant to
-- apply to the online/marketplace sales channel, never to in-store POS.
-- Migration 029 hijacked the existing `sales_enabled` flag (which means
-- "this branch can run POS / issue sales documents," introduced in
-- migration 022) to also mean "this is the only branch allowed to sell at
-- all," which incorrectly blocked POS checkout at MES/NPT/PHH/PHS. This
-- migration reverts that misuse and introduces a separate, purpose-built
-- flag for the online-only exclusivity rule.

-- Drop the "at most one sales_enabled branch" constraint — sales_enabled
-- goes back to meaning "can run POS / issue sales documents," which every
-- real storefront branch should have.
DROP INDEX IF EXISTS idx_branches_single_sales_enabled;

-- Restore in-store POS ability to every storefront branch migration 029
-- turned off. Warehouses stay excluded — branches_main_warehouse_no_sales
-- (migration 022) still enforces that.
UPDATE branches SET sales_enabled = TRUE, updated_at = NOW()
WHERE branch_type = 'branch' AND sales_enabled = FALSE;

-- New flag: only คณาเภสัช may sell through the online/marketplace channel.
-- Every other branch (including any created later) defaults to FALSE.
ALTER TABLE branches
    ADD COLUMN IF NOT EXISTS online_sales_enabled BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE branches SET online_sales_enabled = TRUE, updated_at = NOW()
WHERE code = 'KNP';

-- A warehouse can never sell online either, mirroring the in-store rule.
ALTER TABLE branches
    DROP CONSTRAINT IF EXISTS branches_main_warehouse_no_online_sales;
ALTER TABLE branches
    ADD CONSTRAINT branches_main_warehouse_no_online_sales
    CHECK (branch_type <> 'main_warehouse' OR online_sales_enabled = FALSE);
