-- +goose Up
-- Spec §5: model as 15-minute intervals, not points. `retrieved_at` is
-- separate from the interval itself to distinguish "when the price
-- applies" from "when the application learned it".
CREATE TABLE spot_price (
    interval_start TIMESTAMPTZ NOT NULL,
    interval_end   TIMESTAMPTZ NOT NULL,
    price          NUMERIC NOT NULL,
    currency       TEXT NOT NULL,
    unit           TEXT NOT NULL, -- e.g. 'EUR/MWh', 'c/kWh'
    source         TEXT NOT NULL,
    retrieved_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (interval_start, source)
);

SELECT create_hypertable('spot_price', 'interval_start');

ALTER TABLE spot_price SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'source',
    timescaledb.compress_orderby = 'interval_start DESC'
);

-- Spec §9a: permanent, compressed after ~7 days.
SELECT add_compression_policy('spot_price', INTERVAL '7 days');

-- +goose Down
DROP TABLE IF EXISTS spot_price;
