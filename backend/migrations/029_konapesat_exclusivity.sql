-- Part B, Rule 3 — creates the คณาเภสัช branch (see migration 030 for the
-- corrected version of this rule).
--
-- SUPERSEDED: the sales_enabled changes below (making คณาเภสัช the only
-- branch that can sell AT ALL, in-store included) were wrong and are
-- reverted by migration 030, which restores in-store POS to every branch
-- and adds a separate `online_sales_enabled` flag scoped to คณาเภสัช only.
-- Kept as-written since it already ran on deployed databases; read it
-- alongside 030, not in isolation.
--
-- The user's decision that still stands: คณาเภสัช is a new branch (not a
-- repurposed existing one), parented under the central warehouse the same
-- way the original seed fixture structured it.
INSERT INTO branches (id, code, name, address, branch_type, parent_branch_id, active, sales_enabled, created_at, updated_at)
SELECT gen_random_uuid(), 'KNP', 'คณาเภสัช', '', 'branch', wh.id, TRUE, TRUE, NOW(), NOW()
FROM branches wh
WHERE wh.branch_type = 'main_warehouse'
  AND NOT EXISTS (SELECT 1 FROM branches WHERE code = 'KNP')
LIMIT 1;

-- Every new branch needs its own document numbering sequences — without
-- these, invoice/quotation creation fails outright (nextDocumentNumber()
-- requires a pre-existing row, it doesn't lazily create one). Matches every
-- other branch's starting prefix/number.
INSERT INTO document_sequences (id, branch_id, doc_type, prefix, next_number, is_locked, created_at, updated_at)
SELECT gen_random_uuid(), b.id, seq.doc_type, seq.prefix, 1, FALSE, NOW(), NOW()
FROM branches b
CROSS JOIN (VALUES ('invoice', 'BL'), ('quotation', 'QT')) AS seq(doc_type, prefix)
WHERE b.code = 'KNP'
ON CONFLICT (branch_id, doc_type) DO NOTHING;

-- Every other branch (including the other existing storefronts) becomes
-- transfer-only. main_warehouse branches are already sales_enabled=FALSE
-- (enforced by migration 022's CHECK constraint) so this only actually
-- changes the branches that were previously allowed to sell.
UPDATE branches SET sales_enabled = FALSE, updated_at = NOW()
WHERE code <> 'KNP' AND sales_enabled = TRUE;

-- Make exclusivity structural, not just a convention someone could
-- accidentally break later — at most one sales_enabled branch at a time.
CREATE UNIQUE INDEX IF NOT EXISTS idx_branches_single_sales_enabled
    ON branches (sales_enabled) WHERE sales_enabled = TRUE;
