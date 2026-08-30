-- +goose Up
-- Spec §7.3. Field list is provisional pending verification against
-- ha-defa-power's actual entities (§7.3, §37) — do not assume parity with
-- these columns until confirmed against the real CloudCharge API.
--
-- Not a hypertable: sessions are sparse (charging events only) and mutable
-- (a session's end_time/energy accumulate while charging is in progress),
-- which fits a regular table far better than immutable hypertable chunks.
CREATE TABLE ev_charging_session (
    id                BIGSERIAL PRIMARY KEY,
    start_time        TIMESTAMPTZ NOT NULL,
    end_time          TIMESTAMPTZ,
    energy_kwh        DOUBLE PRECISION,
    average_power_kw  DOUBLE PRECISION,
    maximum_power_kw  DOUBLE PRECISION,
    source            TEXT NOT NULL DEFAULT 'defa_cloud',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source, start_time)
);

-- Genuinely a time series (power sampled while charging), so modeled as a
-- hypertable like the other measurement tables for consistency. No
-- compression policy: spec §9a's compression bucket doesn't list EV data,
-- and volume is negligible ("tens of MB/year", sparse/charging-only).
CREATE TABLE ev_measurement (
    time         TIMESTAMPTZ NOT NULL,
    power_kw     DOUBLE PRECISION,
    charging     BOOLEAN NOT NULL,
    raw_payload  JSONB, -- never store the auth token here (spec §7.4)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (time)
);

SELECT create_hypertable('ev_measurement', 'time');

-- +goose Down
DROP TABLE IF EXISTS ev_measurement;
DROP TABLE IF EXISTS ev_charging_session;
