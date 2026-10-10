-- The marketplace settings tab lists providers from this table, but a fresh
-- database had none, so the tab was empty and "test connection" could only
-- fail (QA 2026-09-02, MK-01..03). These are the channels the business named.
-- Connecting still only checks that the details are complete; no provider API
-- is called until the V5 marketplace work.
INSERT INTO marketplace_providers (id, provider_key, name, description, active, created_at, updated_at)
SELECT v.id::uuid, v.provider_key, v.name, v.description, TRUE, NOW(), NOW()
FROM (VALUES
    ('7a1f0c52-5b0e-4a64-9d1c-0d6c3a1e0001', 'shopee', 'Shopee', 'ช่องทางรับคำสั่งซื้อจาก Shopee'),
    ('7a1f0c52-5b0e-4a64-9d1c-0d6c3a1e0002', 'lazada', 'Lazada', 'ช่องทางรับคำสั่งซื้อจาก Lazada'),
    ('7a1f0c52-5b0e-4a64-9d1c-0d6c3a1e0003', 'tiktok-shop', 'TikTok Shop', 'ช่องทางรับคำสั่งซื้อจาก TikTok Shop'),
    ('7a1f0c52-5b0e-4a64-9d1c-0d6c3a1e0004', 'line-myshop', 'LINE MyShop', 'ช่องทางรับคำสั่งซื้อจาก LINE MyShop')
) AS v(id, provider_key, name, description)
WHERE NOT EXISTS (SELECT 1 FROM marketplace_providers p WHERE p.provider_key = v.provider_key);
