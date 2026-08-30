-- Ghost Stock is a read-only operational view. From this cut-over onward its
-- quantity may change only through the purchase-order lifecycle or month-end
-- reconciliation. Existing lots and movements are retained unchanged.

CREATE OR REPLACE FUNCTION reject_operational_ghost_document()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.stock_bucket='ghost' THEN
        RAISE EXCEPTION 'Ghost Stock cannot be used by sales, quotations, transfers, or returns';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_invoice_items_no_direct_ghost ON invoice_items;
CREATE TRIGGER trg_invoice_items_no_direct_ghost
BEFORE INSERT OR UPDATE ON invoice_items
FOR EACH ROW EXECUTE FUNCTION reject_operational_ghost_document();

DROP TRIGGER IF EXISTS trg_quotation_items_no_direct_ghost ON quotation_items;
CREATE TRIGGER trg_quotation_items_no_direct_ghost
BEFORE INSERT OR UPDATE ON quotation_items
FOR EACH ROW EXECUTE FUNCTION reject_operational_ghost_document();

DROP TRIGGER IF EXISTS trg_transfer_items_no_direct_ghost ON transfer_items;
CREATE TRIGGER trg_transfer_items_no_direct_ghost
BEFORE INSERT OR UPDATE ON transfer_items
FOR EACH ROW EXECUTE FUNCTION reject_operational_ghost_document();

DROP TRIGGER IF EXISTS trg_product_returns_no_direct_ghost ON product_returns;
CREATE TRIGGER trg_product_returns_no_direct_ghost
BEFORE INSERT OR UPDATE ON product_returns
FOR EACH ROW EXECUTE FUNCTION reject_operational_ghost_document();

CREATE OR REPLACE FUNCTION enforce_ghost_movement_source()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.stock_bucket='ghost'
       AND NEW.reference_type NOT IN ('purchase_order','month_end_reconciliation') THEN
        RAISE EXCEPTION 'Ghost Stock movements require purchase_order or month_end_reconciliation source';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_inventory_movements_ghost_source ON inventory_movements;
CREATE TRIGGER trg_inventory_movements_ghost_source
BEFORE INSERT OR UPDATE OF branch_id,stock_bucket,reference_type ON inventory_movements
FOR EACH ROW EXECUTE FUNCTION enforce_ghost_movement_source();

CREATE OR REPLACE FUNCTION enforce_ghost_lot_source()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.stock_bucket='ghost' AND NEW.source_type<>'purchase_order' THEN
        RAISE EXCEPTION 'Ghost Stock lots require purchase_order source';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_inventory_lots_ghost_source ON inventory_lots;
CREATE TRIGGER trg_inventory_lots_ghost_source
BEFORE INSERT OR UPDATE OF branch_id,stock_bucket,source_type ON inventory_lots
FOR EACH ROW EXECUTE FUNCTION enforce_ghost_lot_source();

CREATE OR REPLACE FUNCTION reject_ghost_reclassification()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.from_bucket='ghost' OR NEW.to_bucket='ghost' THEN
        RAISE EXCEPTION 'Ghost Stock reclassification is disabled';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_inventory_reclassifications_ghost_warehouse ON inventory_reclassifications;
CREATE TRIGGER trg_inventory_reclassifications_ghost_warehouse
BEFORE INSERT OR UPDATE ON inventory_reclassifications
FOR EACH ROW EXECUTE FUNCTION reject_ghost_reclassification();
