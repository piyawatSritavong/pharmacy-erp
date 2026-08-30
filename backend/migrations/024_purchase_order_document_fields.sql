-- D4: the PO print/PDF layout is being rebuilt to match a formal business
-- document (PO-Form.jpg) — buyer tax ID, due date, job/order name, delivery
-- terms, and a full supplier contact breakdown weren't captured before.

ALTER TABLE branches
    ADD COLUMN IF NOT EXISTS tax_id TEXT NOT NULL DEFAULT '';

ALTER TABLE purchase_orders
    ADD COLUMN IF NOT EXISTS due_date DATE,
    ADD COLUMN IF NOT EXISTS job_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS delivery_terms TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS supplier_phone_snapshot TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS supplier_email_snapshot TEXT NOT NULL DEFAULT '';
