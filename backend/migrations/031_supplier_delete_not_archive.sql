-- business-flow.md Global Rules: there is no "archive/เก็บ" action anywhere —
-- only CRUD. บริษัทคู่ค้า previously had an archive (active=FALSE) in place of
-- a real delete because purchase_orders.supplier_id was NOT NULL with
-- ON DELETE RESTRICT.
--
-- Purchase orders already snapshot everything they display about a supplier
-- (supplier_code_snapshot / supplier_name_snapshot / supplier_tax_id_snapshot /
-- supplier_address_snapshot, migration 019), so dropping the live reference
-- costs no document history. Make the reference nullable and self-clearing,
-- and a supplier can be genuinely deleted while every past PO still prints
-- exactly as it was issued.
ALTER TABLE purchase_orders
    ALTER COLUMN supplier_id DROP NOT NULL;

ALTER TABLE purchase_orders
    DROP CONSTRAINT IF EXISTS purchase_orders_supplier_id_fkey;
ALTER TABLE purchase_orders
    ADD CONSTRAINT purchase_orders_supplier_id_fkey
    FOREIGN KEY (supplier_id) REFERENCES suppliers(id) ON DELETE SET NULL;

-- Claims join suppliers with a LEFT JOIN, so a cleared reference just shows
-- no supplier name rather than breaking the row.
ALTER TABLE product_returns
    DROP CONSTRAINT IF EXISTS product_returns_supplier_id_fkey;
ALTER TABLE product_returns
    ADD CONSTRAINT product_returns_supplier_id_fkey
    FOREIGN KEY (supplier_id) REFERENCES suppliers(id) ON DELETE SET NULL;
