-- Let a claim reach Ghost Stock, by way of the case the spec asks for and this
-- codebase never built: "สาขาแต่ละสาขา เคลมหรือคืนสินค้า กับบริษัทที่ซื้อขายโดยตรง"
-- — a claim raised against stock on the shelf, with no customer sale behind it.
--
-- Until now every claim had to start from an invoice item, and a sale can only
-- ever be drawn from real stock, so a Ghost unit that arrived defective on a
-- purchase order had no way out of inventory: 045 let Ghost in through
-- receiving and out through the month-end close, and nothing else. The result
-- was that returns/module.go carried a qty_ghost branch at every step that no
-- request could reach.
--
-- Ghost stays barred from sales, quotations and transfers — 045's triggers on
-- those tables are untouched. It is admissible here only on a stock claim,
-- which is a movement between the shelf and the supplier and never appears on
-- a customer document. The role check lives in Go: super_admin only.

ALTER TABLE product_returns
    ALTER COLUMN original_invoice_item_id DROP NOT NULL;

ALTER TABLE product_returns
    ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT 'pos_return';

ALTER TABLE product_returns
    DROP CONSTRAINT IF EXISTS product_returns_origin_check;
ALTER TABLE product_returns
    ADD CONSTRAINT product_returns_origin_check
    CHECK (origin IN ('pos_return', 'stock_claim'));

-- A POS return is what a customer handed back, so it must name the line it was
-- sold on. A stock claim never has one — requiring the pairing keeps the two
-- kinds from blurring into each other later.
ALTER TABLE product_returns
    DROP CONSTRAINT IF EXISTS product_returns_origin_source_check;
ALTER TABLE product_returns
    ADD CONSTRAINT product_returns_origin_source_check
    CHECK (
        (origin = 'pos_return' AND original_invoice_item_id IS NOT NULL)
     OR (origin = 'stock_claim' AND original_invoice_item_id IS NULL)
    );

ALTER TABLE product_returns
    DROP CONSTRAINT IF EXISTS product_returns_ghost_origin_check;
ALTER TABLE product_returns
    ADD CONSTRAINT product_returns_ghost_origin_check
    CHECK (stock_bucket <> 'ghost' OR origin = 'stock_claim');

-- 045 pointed product_returns at the shared reject-any-ghost trigger. Give the
-- table its own, so the other three keep the blanket ban.
CREATE OR REPLACE FUNCTION reject_pos_return_ghost()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.stock_bucket = 'ghost' AND NEW.origin <> 'stock_claim' THEN
        RAISE EXCEPTION 'Ghost Stock cannot be used by a POS customer return';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_product_returns_no_direct_ghost ON product_returns;
CREATE TRIGGER trg_product_returns_no_direct_ghost
BEFORE INSERT OR UPDATE ON product_returns
FOR EACH ROW EXECUTE FUNCTION reject_pos_return_ghost();

CREATE INDEX IF NOT EXISTS idx_product_returns_origin_bucket
    ON product_returns (origin, stock_bucket);

-- 045/046 also limit which reference_type may carry a Ghost movement, so the
-- claim has to be named there or the deduction is refused at the last step.
-- Ghost still cannot move for a sale, a quotation or a transfer — the list
-- grows by exactly one source, and it is the one that sends goods back to the
-- supplier they came from.
CREATE OR REPLACE FUNCTION enforce_ghost_movement_source()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.stock_bucket='ghost'
       AND NEW.reference_type NOT IN (
           'purchase_order',
           'month_end_reconciliation',
           'product_return',
           -- bootstrap-only writers (seed / operational reset)
           'seed.opening_balance',
           'ocha_seed.opening_balance',
           'seed_relationship_repair',
           'operational_reset'
       ) THEN
        RAISE EXCEPTION 'Ghost Stock movements require purchase_order, month_end_reconciliation or product_return source';
    END IF;
    RETURN NEW;
END $$;

-- The lot a supplier's replacement arrives on needs the same permission. A
-- claim replacement is a receipt in every way that matters — goods arrive from
-- the supplier and open a new lot — so it joins purchase_order as a source
-- Ghost lots may come from.
CREATE OR REPLACE FUNCTION enforce_ghost_lot_source()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.stock_bucket='ghost'
       AND NEW.source_type NOT IN (
           'purchase_order',
           'product_return',
           -- bootstrap-only writers (seed / warehouse bootstrap copy / reset)
           'ocha_seed_opening',
           'warehouse_bootstrap_copy',
           'operational_reset'
       ) THEN
        RAISE EXCEPTION 'Ghost Stock lots require purchase_order or product_return source';
    END IF;
    RETURN NEW;
END $$;
