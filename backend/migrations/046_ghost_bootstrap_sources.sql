-- Migration 045 restricted Ghost Stock to purchase-order and month-end sources,
-- but it also blocked the bootstrap paths that create opening balances: a fresh
-- `seed` run fails on the first Ocha Ghost lot. Those writers never run from an
-- operational endpoint, so allow their fixed source names and keep the
-- operational restriction unchanged (manual adjust/receive/rebalance stay
-- rejected).

CREATE OR REPLACE FUNCTION enforce_ghost_movement_source()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.stock_bucket='ghost'
       AND NEW.reference_type NOT IN (
           'purchase_order',
           'month_end_reconciliation',
           -- bootstrap-only writers (seed / operational reset)
           'seed.opening_balance',
           'ocha_seed.opening_balance',
           'seed_relationship_repair',
           'operational_reset'
       ) THEN
        RAISE EXCEPTION 'Ghost Stock movements require purchase_order or month_end_reconciliation source';
    END IF;
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION enforce_ghost_lot_source()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.stock_bucket='ghost'
       AND NEW.source_type NOT IN (
           'purchase_order',
           -- bootstrap-only writers (seed / warehouse bootstrap copy)
           'ocha_seed_opening',
           'warehouse_bootstrap_copy'
       ) THEN
        RAISE EXCEPTION 'Ghost Stock lots require purchase_order source';
    END IF;
    RETURN NEW;
END $$;
