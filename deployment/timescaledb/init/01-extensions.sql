-- Runs once, on first container start, against POSTGRES_DB.
-- Hypertables, compression policies, and retention policies are the
-- application's concern and belong in backend/migrations (spec §9a, §43
-- Step 2), not here — this only makes the extension available.
CREATE EXTENSION IF NOT EXISTS timescaledb;
