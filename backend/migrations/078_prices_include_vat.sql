-- Whether shelf prices already include VAT.
--
-- Off (the default, as before): a price is before VAT and the bill adds VAT
-- on top. On: every price the shop types — shelf, unit, tier and level
-- prices, discounts — is what the customer pays, and the bill takes the VAT
-- out of it. Stored amounts stay before-VAT either way, so documents, reports
-- and the month-end close read them the same.
INSERT INTO app_settings (setting_key, setting_value, metadata, created_at, updated_at)
VALUES ('prices_include_vat', 'false', '{"label": "ราคาขายรวม VAT แล้ว", "type": "boolean"}'::jsonb, NOW(), NOW())
ON CONFLICT (setting_key) DO NOTHING;
