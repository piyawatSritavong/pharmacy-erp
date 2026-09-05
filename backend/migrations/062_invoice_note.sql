-- A bill can now carry a note, and the first thing to write one is the full tax
-- invoice a customer comes back for. The replacement has to say on its face
-- which slip it replaces: the two documents describe one sale, and a customer
-- holding the new one should not have to take that on trust.
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';
