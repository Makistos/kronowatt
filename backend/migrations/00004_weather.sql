-- +goose Up
-- Spec §4.2: all parameters optional except time/station — missing one
-- must not fail the whole observation. `retrieved_at` is the local
-- collection time, kept separate from `time` (the source's observation
-- timestamp) per §4.2.
CREATE TABLE weather_observation (
    time              TIMESTAMPTZ NOT NULL,
    station_fmisid    TEXT NOT NULL,
    air_temperature   DOUBLE PRECISION,
    relative_humidity DOUBLE PRECISION,
    dew_point         DOUBLE PRECISION,
    air_pressure      DOUBLE PRECISION,
    wind_speed        DOUBLE PRECISION,
    wind_direction    DOUBLE PRECISION,
    wind_gust         DOUBLE PRECISION,
    precipitation     DOUBLE PRECISION,
    cloud_cover       DOUBLE PRECISION,
    visibility        DOUBLE PRECISION,
    raw_payload       JSONB,
    retrieved_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (time, station_fmisid)
);

SELECT create_hypertable('weather_observation', 'time');

ALTER TABLE weather_observation SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'station_fmisid',
    timescaledb.compress_orderby = 'time DESC'
);

-- Spec §9a: compress chunks older than ~7 days.
SELECT add_compression_policy('weather_observation', INTERVAL '7 days');

-- Spec §4.3: forecast versions are never overwritten (kept for later
-- forecast-accuracy evaluation), so uniqueness is per generation run, not
-- per target time. Hypertable partitions on `generated_at` (collection
-- time) since that's what drives the §9a retention/drop policy below, not
-- `target_time`.
CREATE TABLE weather_forecast (
    generated_at      TIMESTAMPTZ NOT NULL,
    target_time       TIMESTAMPTZ NOT NULL,
    provider          TEXT NOT NULL,
    model             TEXT,
    air_temperature   DOUBLE PRECISION,
    relative_humidity DOUBLE PRECISION,
    wind_speed        DOUBLE PRECISION,
    wind_direction    DOUBLE PRECISION,
    precipitation     DOUBLE PRECISION,
    cloud_cover       DOUBLE PRECISION,
    raw_payload       JSONB,
    retrieved_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (generated_at, target_time, provider)
);

SELECT create_hypertable('weather_forecast', 'generated_at');

-- Spec §9a: forecasts are retention-bounded (1-2 years), not compressed —
-- unlike observations, this table isn't in the "compressed after ~7 days"
-- bucket. Using 2 years, the upper end of the spec's stated range.
SELECT add_retention_policy('weather_forecast', INTERVAL '2 years');

-- +goose Down
DROP TABLE IF EXISTS weather_forecast;
DROP TABLE IF EXISTS weather_observation;
