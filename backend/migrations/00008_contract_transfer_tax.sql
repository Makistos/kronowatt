-- +goose Up
-- The settings UI (spec §6/§43 Step 6) needs separate electricity and
-- transfer tax fields, not one combined figure — 00006_contracts.sql's
-- single `taxes` column becomes "electricity tax"; this adds its transfer
-- counterpart rather than overloading `other_fees` (that column stays
-- available for genuinely miscellaneous fees, per spec's original field
-- list).
ALTER TABLE contract_price_period ADD COLUMN transfer_tax NUMERIC;

-- +goose Down
ALTER TABLE contract_price_period DROP COLUMN transfer_tax;
