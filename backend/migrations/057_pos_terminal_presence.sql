-- Which branch tills are actually open right now.
--
-- Head office may only remote a sale into a branch whose POS is running: the
-- whole point is that the customer pays at that till, so pushing a cart at a
-- closed shop just strands the bill. The POS already polls for a waiting cart
-- every few seconds while its sales screen is open, so that poll doubles as the
-- heartbeat — presence therefore means "the till is on the sales screen", which
-- is exactly the condition that matters, not merely "someone is logged in".
CREATE TABLE IF NOT EXISTS pos_terminal_presence (
    branch_id UUID PRIMARY KEY REFERENCES branches(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS pos_terminal_presence_last_seen_idx
    ON pos_terminal_presence (last_seen_at DESC);
