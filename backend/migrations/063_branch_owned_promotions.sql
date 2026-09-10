-- Let a branch run its own promotions.
--
-- The table has carried branch_id since 047, and both the POS and the listing
-- already read it — a NULL branch means head office set it for everyone. Two
-- things stopped a branch from actually using any of that:
--
--   * promotion.manage was granted to roles that no longer exist (admin,
--     branch_admin) and never to branch_pos, so the till could see promotions
--     and never write one;
--   * the promo code was unique across the whole company, so the second branch
--     to think of "SUMMER" was told the code was taken — by a shop it cannot
--     see, for a promotion it cannot read.
--
-- Codes are now unique within a branch, and separately within head office's own
-- set. Two branches may both run "SUMMER" and neither has to know.

INSERT INTO role_permissions (role_id, permission_id, created_at)
SELECT r.id, p.id, NOW()
FROM roles r
CROSS JOIN permissions p
WHERE r.role_key = 'branch_pos' AND p.permission_key = 'promotion.manage'
ON CONFLICT DO NOTHING;

ALTER TABLE promotions DROP CONSTRAINT IF EXISTS promotions_code_key;

-- COALESCE rather than a partial index pair: NULL never equals NULL in a unique
-- index, so head office's promotions would otherwise have no uniqueness at all.
DROP INDEX IF EXISTS idx_promotions_code_per_owner;
CREATE UNIQUE INDEX idx_promotions_code_per_owner
    ON promotions (COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'::uuid), code);
