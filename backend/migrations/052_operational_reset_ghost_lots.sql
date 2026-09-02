-- `reset-operational-data` rebuilds one lot per stock bucket from the surviving
-- inventory row, tagging it 'operational_reset'. Migration 046 allowed that
-- source for Ghost *movements* but not for Ghost *lots*, so the command failed
-- on any database holding Ghost Stock — the reset could never run.
-- Ghost quantity is unchanged by the rebuild: it restates what the inventory
-- row already says, exactly like the other bootstrap writers listed here.
CREATE OR REPLACE FUNCTION enforce_ghost_lot_source()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.stock_bucket='ghost'
       AND NEW.source_type NOT IN (
           'purchase_order',
           -- bootstrap-only writers (seed / warehouse bootstrap copy / reset)
           'ocha_seed_opening',
           'warehouse_bootstrap_copy',
           'operational_reset'
       ) THEN
        RAISE EXCEPTION 'Ghost Stock lots require purchase_order source';
    END IF;
    RETURN NEW;
END $$;
