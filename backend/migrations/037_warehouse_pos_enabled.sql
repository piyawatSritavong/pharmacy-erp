-- The warehouse sells too.
--
-- โกดัง is not a storefront — it is the distribution hub every branch draws
-- from — but sales reps and delivery agents collect and pay for stock there in
-- person, so it needs a working till. Until now a CHECK constraint made that
-- impossible: a main_warehouse was forbidden from having sales_enabled.
--
-- Dropping that constraint is preferable to the alternative of re-typing โกดัง
-- as an ordinary 'branch': the warehouse identity is load-bearing elsewhere —
-- it is what every branch's คลังหลักต้นสังกัด points at, what the
-- single-active-warehouse index protects, and what makes product pricing there
-- read from the central catalogue instead of a per-branch override.
--
-- Online selling stays closed at the warehouse: branches_main_warehouse_no_online_sales
-- is deliberately left in place, so this opens the counter, not the storefront.
ALTER TABLE branches DROP CONSTRAINT IF EXISTS branches_main_warehouse_no_sales;

UPDATE branches
   SET branch_type = 'main_warehouse',
       sales_enabled = TRUE,
       parent_branch_id = NULL
 WHERE code = 'WH';
