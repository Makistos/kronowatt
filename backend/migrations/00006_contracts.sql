-- +goose Up
-- Spec §6: manually entered, historical data immutable unless explicitly
-- edited. Not a hypertable — this is a handful of admin-entered rows, not
-- an append-only stream.
--
-- Pricing fields (energy price, spot margin, monthly fee, transfer price,
-- taxes, VAT, other fees) live on contract_price_period rather than here,
-- since §2.4 requires the price rule *valid at a given timestamp* — a
-- contract can span multiple periods with different terms (e.g. a price
-- change mid-contract), and "effective price determined by timestamp"
-- (§6) only makes sense if pricing is period-scoped, not contract-scoped.
CREATE TABLE electricity_contract (
    id            BIGSERIAL PRIMARY KEY,
    supplier      TEXT NOT NULL,
    contract_name TEXT NOT NULL,
    pricing_model TEXT NOT NULL CHECK (pricing_model IN ('fixed', 'spot', 'hybrid')),
    valid_from    TIMESTAMPTZ NOT NULL,
    valid_to      TIMESTAMPTZ,
    notes         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE contract_price_period (
    id             BIGSERIAL PRIMARY KEY,
    contract_id    BIGINT NOT NULL REFERENCES electricity_contract (id) ON DELETE CASCADE,
    period_start   TIMESTAMPTZ NOT NULL,
    period_end     TIMESTAMPTZ,
    energy_price   NUMERIC, -- per pricing_model: fixed energy price
    spot_margin    NUMERIC, -- per pricing_model: margin added to spot price
    monthly_fee    NUMERIC,
    transfer_price NUMERIC,
    taxes          NUMERIC,
    vat            NUMERIC,
    other_fees     NUMERIC,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (contract_id, period_start)
);

-- Supports "find the period effective at timestamp T for this contract":
-- WHERE contract_id = ? AND period_start <= T ORDER BY period_start DESC LIMIT 1.
CREATE INDEX contract_price_period_contract_start_idx
    ON contract_price_period (contract_id, period_start DESC);

-- Supports "find the contract effective at timestamp T".
CREATE INDEX electricity_contract_validity_idx
    ON electricity_contract (valid_from, valid_to);

-- +goose Down
DROP TABLE IF EXISTS contract_price_period;
DROP TABLE IF EXISTS electricity_contract;
