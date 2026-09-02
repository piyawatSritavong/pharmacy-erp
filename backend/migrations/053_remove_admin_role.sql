-- Collapse to three roles and seven accounts.
--
-- The branch-scoped 'admin' role and its per-branch accounts are retired, along
-- with the warehouse till account. What remains: super_admin, central_admin
-- (admin.central) and one branch_pos per selling branch — seven accounts for
-- the five branches that sell (KNP, MES, NPT, PHH, PHS).
--
-- Documents made by the retiring accounts move to superadmin rather than being
-- destroyed with them (several FKs are ON DELETE RESTRICT and would otherwise
-- block the DELETE); mirrors what migration 036 did for the accounts it retired.
DO $$
DECLARE
    keeper UUID;
    retiring UUID[];
BEGIN
    SELECT id INTO keeper FROM users WHERE email = 'superadmin@erp.local';
    SELECT array_agg(u.id) INTO retiring
      FROM users u
      LEFT JOIN roles r ON r.id = u.role_id
     WHERE r.role_key = 'admin' OR u.email = 'pos.warehouse@erp.local';
    IF keeper IS NULL OR retiring IS NULL THEN
        RETURN;  -- fresh database: the seed never creates these accounts
    END IF;

    UPDATE purchase_orders   SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE purchase_orders   SET updated_by   = keeper WHERE updated_by   = ANY(retiring);
    UPDATE purchase_orders   SET cancelled_by = keeper WHERE cancelled_by = ANY(retiring);
    UPDATE purchase_order_events SET actor_id = keeper WHERE actor_id     = ANY(retiring);
    UPDATE product_returns   SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE return_events     SET actor_id     = keeper WHERE actor_id     = ANY(retiring);
    UPDATE invoices          SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE invoices          SET hidden_by_id = keeper WHERE hidden_by_id = ANY(retiring);
    UPDATE invoice_payments  SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE quotations        SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE inventory_movements SET performed_by = keeper WHERE performed_by = ANY(retiring);
    UPDATE inventory_ghost_deficits SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE stock_adjustment_notes   SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE transfers         SET requested_by  = keeper WHERE requested_by  = ANY(retiring);
    UPDATE transfers         SET dispatched_by = keeper WHERE dispatched_by = ANY(retiring);
    UPDATE transfers         SET received_by   = keeper WHERE received_by   = ANY(retiring);
    UPDATE transfer_events   SET actor_id      = keeper WHERE actor_id      = ANY(retiring);
    UPDATE stock_transfer_requests SET requested_by = keeper WHERE requested_by = ANY(retiring);
    UPDATE stock_transfer_requests SET reviewed_by  = keeper WHERE reviewed_by  = ANY(retiring);
    UPDATE audit_logs        SET actor_id     = keeper WHERE actor_id     = ANY(retiring);
    UPDATE marketplace_connections SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE report_definitions SET owner_user_id = keeper WHERE owner_user_id = ANY(retiring);
    UPDATE parked_bills      SET created_by   = keeper WHERE created_by   = ANY(retiring);
    UPDATE inventory_reclassifications SET created_by  = keeper WHERE created_by  = ANY(retiring);
    UPDATE inventory_reclassifications SET reversed_by = keeper WHERE reversed_by = ANY(retiring);
    UPDATE month_end_workpapers SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE month_end_workpapers SET approved_by = keeper WHERE approved_by = ANY(retiring);
    UPDATE month_end_workpapers SET closed_by = keeper WHERE closed_by = ANY(retiring);
    UPDATE month_end_workpapers SET cancelled_by = keeper WHERE cancelled_by = ANY(retiring);
    UPDATE month_end_workpapers SET finalized_by = keeper WHERE finalized_by = ANY(retiring);
    UPDATE month_end_workpapers SET reopened_by = keeper WHERE reopened_by = ANY(retiring);
    UPDATE month_end_calculation_runs SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE month_end_adjustments SET created_by = keeper WHERE created_by = ANY(retiring);
    UPDATE month_end_adjustments SET approved_by = keeper WHERE approved_by = ANY(retiring);
    UPDATE month_end_reconciliations SET finalized_by = keeper WHERE finalized_by = ANY(retiring);

    DELETE FROM users WHERE id = ANY(retiring);
END $$;

DELETE FROM role_permissions WHERE role_id = (SELECT id FROM roles WHERE role_key = 'admin');
DELETE FROM roles WHERE role_key = 'admin';
