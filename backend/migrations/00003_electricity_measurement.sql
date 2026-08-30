-- +goose Up
-- Fields per spec §3.2 (HAN_METER_MESSAGE / /meter). Array columns hold the
-- device's per-phase arrays (total + up to 3 phases) as-is rather than
-- fanning them out into fixed phase columns, since phase count and which
-- fields are populated is device/firmware dependent (§3.2: "store whatever
-- the live response actually contains").
--
-- `time` is the device sample timestamp (`ts`), not the collection time.
-- UNIQUE(time) (not (time, source)) is deliberate: a live sample and a
-- history-backfilled sample for the same device timestamp are the *same*
-- logical measurement (spec §2.6) regardless of which endpoint produced it
-- — the repository layer's ON CONFLICT handling decides precedence.
CREATE TABLE electricity_measurement (
    time         TIMESTAMPTZ NOT NULL,
    source       TEXT NOT NULL, -- e.g. 'cozify_ws', 'cozify_history_hourly'
    ic           DOUBLE PRECISION, -- cumulative imported energy
    ec           DOUBLE PRECISION, -- cumulative exported energy
    ric          DOUBLE PRECISION, -- cumulative reactive imported energy
    rec          DOUBLE PRECISION, -- cumulative reactive exported energy
    p            DOUBLE PRECISION[], -- instantaneous active power (total + phases)
    pi           DOUBLE PRECISION[], -- instantaneous active import power
    pe           DOUBLE PRECISION[], -- instantaneous active export power
    r            DOUBLE PRECISION[], -- instantaneous reactive power
    ri           DOUBLE PRECISION[], -- reactive import power
    re           DOUBLE PRECISION[], -- reactive export power
    u            DOUBLE PRECISION[], -- phase voltages
    i            DOUBLE PRECISION[], -- phase currents
    raw_payload  JSONB,
    inserted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (time)
);

SELECT create_hypertable('electricity_measurement', 'time');

ALTER TABLE electricity_measurement SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'source',
    timescaledb.compress_orderby = 'time DESC'
);

-- Spec §9a: compress chunks older than ~7 days.
SELECT add_compression_policy('electricity_measurement', INTERVAL '7 days');

-- +goose Down
DROP TABLE IF EXISTS electricity_measurement;
