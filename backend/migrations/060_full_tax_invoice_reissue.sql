-- ใบกำกับภาษีอย่างย่อ -> ใบกำกับภาษีเต็มรูป, same day.
--
-- A customer handed an abbreviated tax invoice may come back for a full one.
-- The sale does not change; the document does — so the abbreviated bill is
-- CANCELLED and a full tax invoice is issued in its place, which is what a tax
-- audit expects to see. invoice_status already allows 'cancelled' and every
-- revenue and month-end query already filters on invoice_status='issued', so a
-- cancelled bill drops out of the books on its own without double counting.
--
-- These two columns are the paper trail joining the pair in both directions.
ALTER TABLE invoices
    ADD COLUMN IF NOT EXISTS replaced_by_invoice_id UUID NULL REFERENCES invoices(id),
    ADD COLUMN IF NOT EXISTS replaces_invoice_id UUID NULL REFERENCES invoices(id);

CREATE INDEX IF NOT EXISTS invoices_replaces_idx ON invoices (replaces_invoice_id)
    WHERE replaces_invoice_id IS NOT NULL;
