-- Global Ghost Stock cut-over and date-range month-end reconciliation.
--
-- Ghost Stock is an internal bucket of the single active main warehouse.
-- Historical operational documents that used Ghost Stock at another branch
-- are removed in this migration by product decision.  The migration runner
-- wraps this entire file in one transaction, so any failed invariant rolls
-- the cut-over back in full.

DO $$
DECLARE
    warehouse_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO warehouse_count
    FROM branches
    WHERE branch_type='main_warehouse' AND active=TRUE;
    -- A pristine database creates branches in the application seed after all
    -- migrations have completed. Existing installations must already have
    -- exactly one warehouse; an empty installation is validated by the seed.
    IF warehouse_count NOT IN (0,1) THEN
        RAISE EXCEPTION 'global Ghost Stock requires exactly one active main warehouse (found %)', warehouse_count;
    END IF;
END $$;

-- Persist the derived tender class so the reconciliation filter has one
-- stable source of truth. invoice_payments remains the financial detail.
ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS payment_method TEXT NOT NULL DEFAULT 'unpaid';

UPDATE invoices i
SET payment_method = payment.method
FROM (
    SELECT invoice_id,
           CASE
               WHEN COUNT(*)=0 THEN 'unpaid'
               WHEN BOOL_AND(payment_type='cash') THEN 'cash'
               WHEN BOOL_AND(payment_type='bank_transfer') THEN 'bank_transfer'
               ELSE 'mixed'
           END AS method
    FROM invoice_payments
    GROUP BY invoice_id
) payment
WHERE payment.invoice_id=i.id;

ALTER TABLE invoices DROP CONSTRAINT IF EXISTS invoices_payment_method_check;
ALTER TABLE invoices
    ADD CONSTRAINT invoices_payment_method_check
    CHECK (payment_method IN ('cash','bank_transfer','mixed','unpaid'));

CREATE OR REPLACE FUNCTION refresh_invoice_payment_method(target_invoice UUID)
RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
    UPDATE invoices i
    SET payment_method = (
        SELECT CASE
			WHEN COUNT(*)=0 THEN 'unpaid'
            WHEN BOOL_AND(ip.payment_type='cash') THEN 'cash'
            WHEN BOOL_AND(ip.payment_type='bank_transfer') THEN 'bank_transfer'
            ELSE 'mixed'
        END
        FROM invoice_payments ip
        WHERE ip.invoice_id=target_invoice
    )
    WHERE i.id=target_invoice;
END $$;

CREATE OR REPLACE FUNCTION invoice_payment_method_trigger()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    PERFORM refresh_invoice_payment_method(COALESCE(NEW.invoice_id,OLD.invoice_id));
    IF TG_OP='UPDATE' AND NEW.invoice_id IS DISTINCT FROM OLD.invoice_id THEN
        PERFORM refresh_invoice_payment_method(OLD.invoice_id);
    END IF;
    RETURN COALESCE(NEW,OLD);
END $$;

DROP TRIGGER IF EXISTS trg_invoice_payment_method ON invoice_payments;
CREATE TRIGGER trg_invoice_payment_method
AFTER INSERT OR UPDATE OR DELETE ON invoice_payments
FOR EACH ROW EXECUTE FUNCTION invoice_payment_method_trigger();

ALTER TABLE month_end_reconciliations
    ADD COLUMN IF NOT EXISTS reconciliation_mode TEXT NOT NULL DEFAULT 'legacy_target',
    ADD COLUMN IF NOT EXISTS legacy_target_ignored BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE month_end_reconciliations DROP CONSTRAINT IF EXISTS month_end_reconciliations_mode_check;
ALTER TABLE month_end_reconciliations
    ADD CONSTRAINT month_end_reconciliations_mode_check
    CHECK (reconciliation_mode IN ('legacy_target','hide_all_cash_no_tax'));

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS month_end_reconciliation_scopes (
    reconciliation_id UUID NOT NULL REFERENCES month_end_reconciliations(id) ON DELETE CASCADE,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    date_from DATE NOT NULL,
    date_to DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (reconciliation_id,branch_id),
    CHECK (date_to >= date_from)
);

INSERT INTO month_end_reconciliation_scopes (reconciliation_id,branch_id,date_from,date_to,created_at)
SELECT r.id,branch_id,r.period_start,r.period_end,r.created_at
FROM month_end_reconciliations r
CROSS JOIN LATERAL UNNEST(r.branch_ids) AS branch_id
ON CONFLICT DO NOTHING;

ALTER TABLE month_end_reconciliation_scopes
    DROP CONSTRAINT IF EXISTS month_end_reconciliation_scopes_no_overlap;
ALTER TABLE month_end_reconciliation_scopes
    ADD CONSTRAINT month_end_reconciliation_scopes_no_overlap
    EXCLUDE USING gist (
        branch_id WITH =,
        daterange(date_from,date_to,'[]') WITH &&
    );

ALTER TABLE reconciliation_invoice_snapshots
    ADD COLUMN IF NOT EXISTS invoice_created_at TIMESTAMPTZ;
UPDATE reconciliation_invoice_snapshots snapshot
SET invoice_created_at=i.created_at
FROM invoices i
WHERE i.id=snapshot.invoice_id AND snapshot.invoice_created_at IS NULL;
ALTER TABLE reconciliation_invoice_snapshots
    ALTER COLUMN invoice_created_at SET NOT NULL;

ALTER TABLE reconciliation_item_snapshots
    ADD COLUMN IF NOT EXISTS effective_stock_bucket TEXT;
UPDATE reconciliation_item_snapshots
SET effective_stock_bucket=original_stock_bucket
WHERE effective_stock_bucket IS NULL;
ALTER TABLE reconciliation_item_snapshots
    ALTER COLUMN effective_stock_bucket SET NOT NULL;
ALTER TABLE reconciliation_item_snapshots
    DROP CONSTRAINT IF EXISTS reconciliation_item_snapshots_effective_bucket_check;
ALTER TABLE reconciliation_item_snapshots
    ADD CONSTRAINT reconciliation_item_snapshots_effective_bucket_check
    CHECK (effective_stock_bucket IN ('real','ghost'));

ALTER TABLE reconciliation_logs
    ADD COLUMN IF NOT EXISTS adjustment_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS movement_role TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS real_stock_deducted INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ghost_stock_deducted INTEGER NOT NULL DEFAULT 0;

ALTER TABLE reconciliation_logs DROP CONSTRAINT IF EXISTS reconciliation_logs_log_type_check;
ALTER TABLE reconciliation_logs
    ADD CONSTRAINT reconciliation_logs_log_type_check
    CHECK (log_type IN (
        'invoice_suppressed','invoice_renumbered','price_adjusted','stock_deducted',
        'stock_reversed','stock_received','stock_deficit'
    ));

ALTER TABLE reconciliation_logs
    DROP CONSTRAINT IF EXISTS reconciliation_logs_real_deducted_check,
    DROP CONSTRAINT IF EXISTS reconciliation_logs_ghost_deducted_check;
ALTER TABLE reconciliation_logs
    ADD CONSTRAINT reconciliation_logs_real_deducted_check CHECK (real_stock_deducted >= 0),
    ADD CONSTRAINT reconciliation_logs_ghost_deducted_check CHECK (ghost_stock_deducted >= 0);

-- Historical cycles may predate the explicit effective source column. Their
-- stock logs are the authoritative evidence that an item ended on Ghost.
UPDATE reconciliation_item_snapshots snapshot
SET effective_stock_bucket='ghost'
WHERE EXISTS (
    SELECT 1
    FROM reconciliation_logs log
    WHERE log.reconciliation_id=snapshot.reconciliation_id
      AND log.invoice_item_id=snapshot.invoice_item_id
      AND log.log_type='stock_deducted'
      AND log.stock_bucket='ghost'
);

ALTER TABLE transfers
    ADD COLUMN IF NOT EXISTS reconciliation_id UUID REFERENCES month_end_reconciliations(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS reference_invoice_id UUID REFERENCES invoices(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS system_generated BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS stock_adjustment_notes (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity INTEGER NOT NULL CHECK (quantity <> 0),
    stock_type TEXT NOT NULL CHECK (stock_type IN ('REAL','GHOST')),
    reason TEXT NOT NULL,
    reference_invoice_id UUID REFERENCES invoices(id) ON DELETE RESTRICT,
    reconciliation_id UUID REFERENCES month_end_reconciliations(id) ON DELETE RESTRICT,
    inventory_movement_id UUID REFERENCES inventory_movements(id) ON DELETE RESTRICT,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stock_adjustment_notes_scope
    ON stock_adjustment_notes (branch_id,stock_type,created_at DESC,id);
CREATE INDEX IF NOT EXISTS idx_stock_adjustment_notes_reconciliation
    ON stock_adjustment_notes (reconciliation_id,reference_invoice_id,created_at,id);

-- A deficit allocation is the portion of a Ghost deduction that could not be
-- attached to a positive FEFO lot. It is immutable evidence for the permanent
-- difference between aggregate Ghost quantity and remaining Ghost lots.
CREATE TABLE IF NOT EXISTS inventory_ghost_deficits (
    id UUID PRIMARY KEY,
    branch_id UUID NOT NULL REFERENCES branches(id) ON DELETE RESTRICT,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    reconciliation_id UUID NOT NULL REFERENCES month_end_reconciliations(id) ON DELETE RESTRICT,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,
    invoice_item_id UUID NOT NULL REFERENCES invoice_items(id) ON DELETE RESTRICT,
    inventory_movement_id UUID NOT NULL REFERENCES inventory_movements(id) ON DELETE RESTRICT,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inventory_ghost_deficits_stock
    ON inventory_ghost_deficits (branch_id,product_id,created_at,id);
CREATE INDEX IF NOT EXISTS idx_inventory_ghost_deficits_reconciliation
    ON inventory_ghost_deficits (reconciliation_id,invoice_id,invoice_item_id);

CREATE OR REPLACE FUNCTION reject_ghost_deficit_mutation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'inventory_ghost_deficits is immutable';
END $$;
DROP TRIGGER IF EXISTS trg_inventory_ghost_deficits_immutable ON inventory_ghost_deficits;
CREATE TRIGGER trg_inventory_ghost_deficits_immutable
BEFORE UPDATE OR DELETE ON inventory_ghost_deficits
FOR EACH ROW EXECUTE FUNCTION reject_ghost_deficit_mutation();

-- -------------------------------------------------------------------------
-- Irreversible product-approved purge of operational Ghost history outside
-- the warehouse. Mixed documents are removed as whole documents. Real lots
-- received by mixed PO/transfer documents are detached to an opening source
-- before the document header is deleted so surviving Real history stays valid.

CREATE TEMP TABLE purge_reconciliations ON COMMIT DROP AS
SELECT DISTINCT rl.reconciliation_id AS id
FROM reconciliation_logs rl
INNER JOIN branches b ON b.id=rl.branch_id
WHERE rl.stock_bucket='ghost' AND b.branch_type<>'main_warehouse';

CREATE TEMP TABLE purge_invoices ON COMMIT DROP AS
SELECT DISTINCT i.id
FROM invoices i
INNER JOIN branches b ON b.id=i.branch_id
LEFT JOIN invoice_items ii ON ii.invoice_id=i.id
LEFT JOIN reconciliation_logs rl ON rl.invoice_id=i.id
WHERE b.branch_type<>'main_warehouse'
  AND (ii.stock_bucket='ghost'
       OR rl.reconciliation_id IN (SELECT id FROM purge_reconciliations));

CREATE TEMP TABLE purge_quotations ON COMMIT DROP AS
SELECT DISTINCT q.id
FROM quotations q
INNER JOIN branches b ON b.id=q.branch_id
INNER JOIN quotation_items qi ON qi.quotation_id=q.id
WHERE b.branch_type<>'main_warehouse' AND qi.stock_bucket='ghost';

INSERT INTO purge_quotations(id)
SELECT DISTINCT i.source_quote_id
FROM invoices i
WHERE i.id IN (SELECT id FROM purge_invoices) AND i.source_quote_id IS NOT NULL
ON CONFLICT DO NOTHING;

CREATE TEMP TABLE purge_purchase_orders ON COMMIT DROP AS
SELECT DISTINCT po.id
FROM purchase_orders po
INNER JOIN branches b ON b.id=po.branch_id
INNER JOIN purchase_order_items poi ON poi.purchase_order_id=po.id
WHERE b.branch_type<>'main_warehouse' AND poi.stock_bucket='ghost';

CREATE TEMP TABLE purge_transfers ON COMMIT DROP AS
SELECT DISTINCT t.id
FROM transfers t
INNER JOIN transfer_items ti ON ti.transfer_id=t.id
INNER JOIN branches source ON source.id=t.source_branch_id
INNER JOIN branches destination ON destination.id=t.destination_branch_id
WHERE ti.stock_bucket='ghost'
  AND (source.branch_type<>'main_warehouse' OR destination.branch_type<>'main_warehouse');

CREATE TEMP TABLE purge_returns ON COMMIT DROP AS
SELECT DISTINCT pr.id
FROM product_returns pr
INNER JOIN branches b ON b.id=pr.branch_id
WHERE pr.stock_bucket='ghost' AND b.branch_type<>'main_warehouse';

CREATE TEMP TABLE purge_workpapers ON COMMIT DROP AS
SELECT DISTINCT w.id
FROM month_end_workpapers w
LEFT JOIN month_end_workpaper_lines l ON l.workpaper_id=w.id
LEFT JOIN branches b ON b.id=l.branch_id
WHERE (l.original_stock_bucket='ghost' AND b.branch_type<>'main_warehouse')
   OR l.invoice_id IN (SELECT id FROM purge_invoices);

-- Restore lots consumed by invoices that are about to cease to exist.
WITH restored AS (
    SELECT iml.inventory_lot_id,SUM(ABS(iml.quantity_delta))::integer quantity
    FROM inventory_movements movement
    INNER JOIN inventory_movement_lots iml ON iml.inventory_movement_id=movement.id
    WHERE movement.reference_type='invoice'
      AND movement.reference_id IN (SELECT id FROM purge_invoices)
      AND movement.quantity_delta<0
    GROUP BY iml.inventory_lot_id
)
UPDATE inventory_lots lot
SET remaining_quantity=LEAST(lot.received_quantity,lot.remaining_quantity+restored.quantity),
    updated_at=NOW()
FROM restored
WHERE lot.id=restored.inventory_lot_id;

-- Rebase surviving Real lots/movements that originated in mixed documents.
UPDATE inventory_lots
SET source_type='ghost_topology_rebase',source_id=NULL,source_item_id=NULL,updated_at=NOW()
WHERE stock_bucket='real'
  AND ((source_type='purchase_order' AND source_id IN (SELECT id FROM purge_purchase_orders))
    OR (source_type IN ('transfer','transfer_overage') AND source_id IN (SELECT id FROM purge_transfers)));

UPDATE inventory_movements
SET reference_type='ghost_topology_rebase',reference_id=NULL,
    note='Rebased Real Stock during global Ghost Stock cut-over'
WHERE stock_bucket='real'
  AND ((reference_type='purchase_order' AND reference_id IN (SELECT id FROM purge_purchase_orders))
    OR (reference_type='transfer' AND reference_id IN (SELECT id FROM purge_transfers)));

-- Remove workpaper/reconciliation children before invoice RESTRICT keys.
DELETE FROM inventory_reclassifications
WHERE workpaper_id IN (SELECT id FROM purge_workpapers);
DELETE FROM month_end_adjustments
WHERE workpaper_id IN (SELECT id FROM purge_workpapers);
DELETE FROM month_end_workpaper_lines
WHERE workpaper_id IN (SELECT id FROM purge_workpapers);
DELETE FROM month_end_calculation_runs
WHERE workpaper_id IN (SELECT id FROM purge_workpapers);
DELETE FROM month_end_workpapers
WHERE id IN (SELECT id FROM purge_workpapers);

DELETE FROM reconciliation_logs
WHERE reconciliation_id IN (SELECT id FROM purge_reconciliations)
   OR invoice_id IN (SELECT id FROM purge_invoices);
DELETE FROM reconciliation_item_snapshots
WHERE reconciliation_id IN (SELECT id FROM purge_reconciliations)
   OR invoice_id IN (SELECT id FROM purge_invoices);
DELETE FROM reconciliation_invoice_snapshots
WHERE reconciliation_id IN (SELECT id FROM purge_reconciliations)
   OR invoice_id IN (SELECT id FROM purge_invoices);
DELETE FROM month_end_reconciliation_scopes
WHERE reconciliation_id IN (SELECT id FROM purge_reconciliations);
DELETE FROM month_end_reconciliations
WHERE id IN (SELECT id FROM purge_reconciliations);

DELETE FROM return_events WHERE return_id IN (SELECT id FROM purge_returns);
DELETE FROM product_returns
WHERE id IN (SELECT id FROM purge_returns)
   OR original_invoice_item_id IN (
       SELECT id FROM invoice_items WHERE invoice_id IN (SELECT id FROM purge_invoices)
   );

DELETE FROM transfer_item_lot_allocations
WHERE transfer_item_id IN (SELECT id FROM transfer_items WHERE transfer_id IN (SELECT id FROM purge_transfers));
DELETE FROM transfer_events WHERE transfer_id IN (SELECT id FROM purge_transfers);
UPDATE stock_transfer_requests SET transfer_id=NULL
WHERE transfer_id IN (SELECT id FROM purge_transfers);
DELETE FROM transfer_items WHERE transfer_id IN (SELECT id FROM purge_transfers);
DELETE FROM transfers WHERE id IN (SELECT id FROM purge_transfers);

-- Ghost lots/movements outside WH have no surviving operational meaning.
DELETE FROM inventory_movement_lots
WHERE inventory_movement_id IN (
    SELECT movement.id FROM inventory_movements movement
    INNER JOIN branches b ON b.id=movement.branch_id
    WHERE movement.stock_bucket='ghost' AND b.branch_type<>'main_warehouse'
)
OR inventory_lot_id IN (
    SELECT lot.id FROM inventory_lots lot
    INNER JOIN branches b ON b.id=lot.branch_id
    WHERE lot.stock_bucket='ghost' AND b.branch_type<>'main_warehouse'
);
DELETE FROM inventory_movements movement
USING branches b
WHERE movement.branch_id=b.id
  AND movement.stock_bucket='ghost'
  AND b.branch_type<>'main_warehouse';
DELETE FROM inventory_lots lot
USING branches b
WHERE lot.branch_id=b.id
  AND lot.stock_bucket='ghost'
  AND b.branch_type<>'main_warehouse';

DELETE FROM inventory_movement_lots
WHERE inventory_movement_id IN (
    SELECT id FROM inventory_movements
    WHERE reference_type='invoice' AND reference_id IN (SELECT id FROM purge_invoices)
);
DELETE FROM inventory_movements
WHERE reference_type='invoice' AND reference_id IN (SELECT id FROM purge_invoices);

DELETE FROM audit_logs
WHERE entity_id IN (
    SELECT id FROM purge_invoices UNION ALL
    SELECT id FROM purge_quotations UNION ALL
    SELECT id FROM purge_purchase_orders UNION ALL
    SELECT id FROM purge_transfers UNION ALL
    SELECT id FROM purge_returns UNION ALL
    SELECT id FROM purge_workpapers UNION ALL
    SELECT id FROM purge_reconciliations
);

DELETE FROM invoices WHERE id IN (SELECT id FROM purge_invoices);
DELETE FROM quotation_items WHERE quotation_id IN (SELECT id FROM purge_quotations);
DELETE FROM quotations WHERE id IN (SELECT id FROM purge_quotations);

DELETE FROM inventory_movement_lots
WHERE inventory_lot_id IN (
    SELECT lot.id FROM inventory_lots lot
    WHERE lot.stock_bucket='ghost'
      AND lot.source_type='purchase_order'
      AND lot.source_id IN (SELECT id FROM purge_purchase_orders)
);
DELETE FROM inventory_lots
WHERE stock_bucket='ghost' AND source_type='purchase_order'
  AND source_id IN (SELECT id FROM purge_purchase_orders);
DELETE FROM inventory_movements
WHERE stock_bucket='ghost' AND reference_type='purchase_order'
  AND reference_id IN (SELECT id FROM purge_purchase_orders);
DELETE FROM purchase_order_events WHERE purchase_order_id IN (SELECT id FROM purge_purchase_orders);
DELETE FROM purchase_order_items WHERE purchase_order_id IN (SELECT id FROM purge_purchase_orders);
DELETE FROM purchase_orders WHERE id IN (SELECT id FROM purge_purchase_orders);

DELETE FROM stock_transfer_requests request
USING branches destination
WHERE request.destination_branch_id=destination.id
  AND request.approved_stock_bucket='ghost'
  AND destination.branch_type<>'main_warehouse';

UPDATE inventory i
SET qty_real=COALESCE((
        SELECT SUM(l.remaining_quantity)::integer FROM inventory_lots l
        WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='real'
    ),0),
    qty_ghost=CASE WHEN b.branch_type='main_warehouse' THEN COALESCE((
        SELECT SUM(l.remaining_quantity)::integer FROM inventory_lots l
        WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='ghost'
    ),0) ELSE 0 END,
    updated_at=NOW()
FROM branches b
WHERE b.id=i.branch_id;

-- -------------------------------------------------------------------------
-- Database-level topology guards for every future write path.

CREATE OR REPLACE FUNCTION enforce_ghost_warehouse_row()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    target_branch UUID;
    target_bucket TEXT;
    target_ghost INTEGER;
    is_warehouse BOOLEAN;
BEGIN
    IF TG_TABLE_NAME='inventory' THEN
        target_branch:=NEW.branch_id; target_ghost:=NEW.qty_ghost;
        IF target_ghost=0 THEN RETURN NEW; END IF;
    ELSIF TG_TABLE_NAME IN ('inventory_lots','inventory_movements') THEN
        target_branch:=NEW.branch_id; target_bucket:=NEW.stock_bucket;
        IF target_bucket<>'ghost' THEN RETURN NEW; END IF;
    ELSIF TG_TABLE_NAME='purchase_order_items' THEN
        target_bucket:=NEW.stock_bucket;
        IF target_bucket<>'ghost' THEN RETURN NEW; END IF;
        SELECT branch_id INTO target_branch FROM purchase_orders WHERE id=NEW.purchase_order_id;
    ELSIF TG_TABLE_NAME='inventory_reclassifications' THEN
        IF NEW.from_bucket<>'ghost' AND NEW.to_bucket<>'ghost' THEN RETURN NEW; END IF;
        target_branch:=NEW.branch_id;
    ELSE
        RETURN NEW;
    END IF;
    SELECT EXISTS(
        SELECT 1 FROM branches WHERE id=target_branch AND branch_type='main_warehouse' AND active=TRUE
    ) INTO is_warehouse;
    IF NOT is_warehouse THEN
        RAISE EXCEPTION 'Ghost Stock is allowed only at the active main warehouse';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_inventory_ghost_warehouse ON inventory;
CREATE TRIGGER trg_inventory_ghost_warehouse
BEFORE INSERT OR UPDATE OF branch_id,qty_ghost ON inventory
FOR EACH ROW EXECUTE FUNCTION enforce_ghost_warehouse_row();
DROP TRIGGER IF EXISTS trg_inventory_lots_ghost_warehouse ON inventory_lots;
CREATE TRIGGER trg_inventory_lots_ghost_warehouse
BEFORE INSERT OR UPDATE OF branch_id,stock_bucket ON inventory_lots
FOR EACH ROW EXECUTE FUNCTION enforce_ghost_warehouse_row();
DROP TRIGGER IF EXISTS trg_inventory_movements_ghost_warehouse ON inventory_movements;
CREATE TRIGGER trg_inventory_movements_ghost_warehouse
BEFORE INSERT OR UPDATE OF branch_id,stock_bucket ON inventory_movements
FOR EACH ROW EXECUTE FUNCTION enforce_ghost_warehouse_row();
DROP TRIGGER IF EXISTS trg_purchase_order_items_ghost_warehouse ON purchase_order_items;
CREATE TRIGGER trg_purchase_order_items_ghost_warehouse
BEFORE INSERT OR UPDATE OF purchase_order_id,stock_bucket ON purchase_order_items
FOR EACH ROW EXECUTE FUNCTION enforce_ghost_warehouse_row();
DROP TRIGGER IF EXISTS trg_inventory_reclassifications_ghost_warehouse ON inventory_reclassifications;
CREATE TRIGGER trg_inventory_reclassifications_ghost_warehouse
BEFORE INSERT OR UPDATE OF branch_id,from_bucket,to_bucket ON inventory_reclassifications
FOR EACH ROW EXECUTE FUNCTION enforce_ghost_warehouse_row();

CREATE OR REPLACE FUNCTION enforce_operational_ghost_warehouse()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    target_branch UUID;
    second_branch UUID;
    valid_topology BOOLEAN;
BEGIN
    IF NEW.stock_bucket<>'ghost' THEN
        RETURN NEW;
    END IF;
    IF TG_TABLE_NAME='invoice_items' THEN
        SELECT branch_id INTO target_branch FROM invoices WHERE id=NEW.invoice_id;
    ELSIF TG_TABLE_NAME='quotation_items' THEN
        SELECT branch_id INTO target_branch FROM quotations WHERE id=NEW.quotation_id;
    ELSIF TG_TABLE_NAME='product_returns' THEN
        target_branch:=NEW.branch_id;
    ELSIF TG_TABLE_NAME='transfer_items' THEN
        SELECT source_branch_id,destination_branch_id INTO target_branch,second_branch
        FROM transfers WHERE id=NEW.transfer_id;
    END IF;
    SELECT EXISTS(
        SELECT 1 FROM branches
        WHERE id=target_branch AND branch_type='main_warehouse' AND active=TRUE
    ) INTO valid_topology;
    IF NOT valid_topology THEN
        RAISE EXCEPTION 'non-warehouse operational documents must use Real Stock';
    END IF;
    IF second_branch IS NOT NULL THEN
        SELECT EXISTS(
            SELECT 1 FROM branches
            WHERE id=second_branch AND branch_type='main_warehouse' AND active=TRUE
        ) INTO valid_topology;
        IF NOT valid_topology THEN
            RAISE EXCEPTION 'Ghost Stock transfers cannot leave the active main warehouse';
        END IF;
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_invoice_items_no_direct_ghost ON invoice_items;
CREATE TRIGGER trg_invoice_items_no_direct_ghost
BEFORE INSERT OR UPDATE OF stock_bucket ON invoice_items
FOR EACH ROW EXECUTE FUNCTION enforce_operational_ghost_warehouse();
DROP TRIGGER IF EXISTS trg_quotation_items_no_direct_ghost ON quotation_items;
CREATE TRIGGER trg_quotation_items_no_direct_ghost
BEFORE INSERT OR UPDATE OF stock_bucket ON quotation_items
FOR EACH ROW EXECUTE FUNCTION enforce_operational_ghost_warehouse();
DROP TRIGGER IF EXISTS trg_transfer_items_no_direct_ghost ON transfer_items;
CREATE TRIGGER trg_transfer_items_no_direct_ghost
BEFORE INSERT OR UPDATE OF stock_bucket ON transfer_items
FOR EACH ROW EXECUTE FUNCTION enforce_operational_ghost_warehouse();
DROP TRIGGER IF EXISTS trg_product_returns_no_direct_ghost ON product_returns;
CREATE TRIGGER trg_product_returns_no_direct_ghost
BEFORE INSERT OR UPDATE OF stock_bucket ON product_returns
FOR EACH ROW EXECUTE FUNCTION enforce_operational_ghost_warehouse();

DO $$
DECLARE
    invalid_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO invalid_count
    FROM inventory i INNER JOIN branches b ON b.id=i.branch_id
    WHERE b.branch_type<>'main_warehouse' AND i.qty_ghost<>0;
    IF invalid_count<>0 THEN
        RAISE EXCEPTION 'global Ghost Stock cut-over left % non-warehouse balances',invalid_count;
    END IF;
    SELECT COUNT(*) INTO invalid_count
    FROM inventory_lots l INNER JOIN branches b ON b.id=l.branch_id
    WHERE b.branch_type<>'main_warehouse' AND l.stock_bucket='ghost';
    IF invalid_count<>0 THEN
        RAISE EXCEPTION 'global Ghost Stock cut-over left % non-warehouse Ghost lots',invalid_count;
    END IF;

    SELECT COUNT(*) INTO invalid_count
    FROM inventory_movements m INNER JOIN branches b ON b.id=m.branch_id
    WHERE b.branch_type<>'main_warehouse' AND m.stock_bucket='ghost';
    IF invalid_count<>0 THEN
        RAISE EXCEPTION 'global Ghost Stock cut-over left % non-warehouse Ghost movements',invalid_count;
    END IF;

    SELECT COUNT(*) INTO invalid_count
    FROM purchase_order_items item
    INNER JOIN purchase_orders po ON po.id=item.purchase_order_id
    INNER JOIN branches b ON b.id=po.branch_id
    WHERE item.stock_bucket='ghost' AND b.branch_type<>'main_warehouse';
    IF invalid_count<>0 THEN
        RAISE EXCEPTION 'global Ghost Stock cut-over left % non-warehouse Ghost purchase lines',invalid_count;
    END IF;

    SELECT COUNT(*) INTO invalid_count
    FROM inventory_reclassifications r INNER JOIN branches b ON b.id=r.branch_id
    WHERE (r.from_bucket='ghost' OR r.to_bucket='ghost')
      AND b.branch_type<>'main_warehouse';
    IF invalid_count<>0 THEN
        RAISE EXCEPTION 'global Ghost Stock cut-over left % non-warehouse Ghost reclassifications',invalid_count;
    END IF;

    SELECT
        (SELECT COUNT(*) FROM invoice_items item
         INNER JOIN invoices document ON document.id=item.invoice_id
         INNER JOIN branches b ON b.id=document.branch_id
         WHERE item.stock_bucket='ghost' AND b.branch_type<>'main_warehouse')
      + (SELECT COUNT(*) FROM quotation_items item
         INNER JOIN quotations document ON document.id=item.quotation_id
         INNER JOIN branches b ON b.id=document.branch_id
         WHERE item.stock_bucket='ghost' AND b.branch_type<>'main_warehouse')
      + (SELECT COUNT(*) FROM transfer_items item
         INNER JOIN transfers document ON document.id=item.transfer_id
         INNER JOIN branches source ON source.id=document.source_branch_id
         INNER JOIN branches destination ON destination.id=document.destination_branch_id
         WHERE item.stock_bucket='ghost'
           AND (source.branch_type<>'main_warehouse' OR destination.branch_type<>'main_warehouse'))
      + (SELECT COUNT(*) FROM product_returns item
         INNER JOIN branches b ON b.id=item.branch_id
         WHERE item.stock_bucket='ghost' AND b.branch_type<>'main_warehouse')
    INTO invalid_count;
    IF invalid_count<>0 THEN
        RAISE EXCEPTION 'global Ghost Stock cut-over left % non-warehouse operational Ghost lines',invalid_count;
    END IF;

    SELECT COUNT(*) INTO invalid_count
    FROM inventory i
    WHERE i.qty_real<>COALESCE((
              SELECT SUM(l.remaining_quantity)::integer
              FROM inventory_lots l
              WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='real'
          ),0)
       OR i.qty_ghost<>COALESCE((
              SELECT SUM(l.remaining_quantity)::integer
              FROM inventory_lots l
              WHERE l.branch_id=i.branch_id AND l.product_id=i.product_id AND l.stock_bucket='ghost'
          ),0);
    IF invalid_count<>0 THEN
        RAISE EXCEPTION 'global Ghost Stock cut-over left % inventory and lot aggregates out of balance',invalid_count;
    END IF;

    -- source_id is intentionally polymorphic, so verify the references that
    -- were affected by the purge explicitly; ordinary foreign keys verify the
    -- remaining relational graph at statement/transaction commit.
    SELECT COUNT(*) INTO invalid_count
    FROM inventory_lots l
    WHERE (l.source_type='purchase_order' AND l.source_id IS NOT NULL
           AND NOT EXISTS (SELECT 1 FROM purchase_orders po WHERE po.id=l.source_id))
       OR (l.source_type IN ('transfer','transfer_overage','month_end_return') AND l.source_id IS NOT NULL
           AND NOT EXISTS (SELECT 1 FROM transfers t WHERE t.id=l.source_id));
    IF invalid_count<>0 THEN
        RAISE EXCEPTION 'global Ghost Stock cut-over left % orphan lot sources',invalid_count;
    END IF;
END $$;
