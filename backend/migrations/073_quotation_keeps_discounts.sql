-- A quotation is a price promise. It used to keep only the per-base-unit price,
-- so converting it re-priced every line from scratch: the cashier's line
-- discounts and the bill discount were lost and the invoice came out higher
-- than the quotation the customer accepted (QA 2026-09-02, S-13p2).
--
-- quotation_items already has the unit/discount columns from migration 047;
-- these two carry what conversion needs to rebuild the same cart.
ALTER TABLE quotations
    ADD COLUMN IF NOT EXISTS bill_discount_amount NUMERIC(12,2) NOT NULL DEFAULT 0
        CHECK (bill_discount_amount >= 0);

-- discount_amount is the line's whole discount (cashier + promotion), the same
-- meaning it has on invoice_items. manual_discount_amount is the cashier's part
-- alone: promotions are evaluated again when the quotation becomes an invoice,
-- so feeding the whole discount back in would count the promotion twice.
ALTER TABLE quotation_items
    ADD COLUMN IF NOT EXISTS manual_discount_amount NUMERIC(14,2) NOT NULL DEFAULT 0
        CHECK (manual_discount_amount >= 0);
