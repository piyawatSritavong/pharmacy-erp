-- Legacy installations stored their opening quantities directly in inventory.
-- Add only the difference so rebuilding from the movement ledger is lossless.
WITH canonical_actor AS (
    SELECT u.id
    FROM users u
    INNER JOIN roles r ON r.id = u.role_id
    WHERE r.role_key = 'super_admin'
    ORDER BY u.created_at ASC
    LIMIT 1
), opening_rows AS (
    SELECT
        i.branch_id,
        i.product_id,
        bucket.stock_bucket,
        CASE bucket.stock_bucket
            WHEN 'real' THEN i.qty_real
            ELSE i.qty_ghost
        END - COALESCE(SUM(m.quantity_delta), 0)::int AS quantity_delta
    FROM inventory i
    CROSS JOIN (VALUES ('real'), ('ghost')) AS bucket(stock_bucket)
    LEFT JOIN inventory_movements m
        ON m.branch_id = i.branch_id
       AND m.product_id = i.product_id
       AND m.stock_bucket = bucket.stock_bucket
    GROUP BY i.branch_id, i.product_id, i.qty_real, i.qty_ghost, bucket.stock_bucket
)
INSERT INTO inventory_movements (
    id,
    branch_id,
    product_id,
    movement_type,
    stock_bucket,
    quantity_delta,
    reference_type,
    note,
    performed_by,
    created_at
)
SELECT
    gen_random_uuid(),
    opening.branch_id,
    opening.product_id,
    'opening_balance',
    opening.stock_bucket,
    opening.quantity_delta,
    'migration.opening_balance',
    'ยกยอดเริ่มต้นจากระบบเดิม',
    actor.id,
    NOW()
FROM opening_rows opening
LEFT JOIN canonical_actor actor ON TRUE
WHERE opening.quantity_delta <> 0;
