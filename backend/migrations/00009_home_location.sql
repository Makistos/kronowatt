-- +goose Up
-- Single-row settings table: the household's home coordinates and the FMI
-- station chosen for weather observations (spec §4.1's "User location...
-- used for forecasts" plus, now, for picking the observation station too
-- — see the "find nearest station" settings UI). A fixed id=1 enforces
-- "at most one row" without a separate admin/multi-tenant concept this
-- app doesn't have; the repository always upserts onto id=1.
CREATE TABLE home_location (
    id             INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    latitude       DOUBLE PRECISION NOT NULL,
    longitude      DOUBLE PRECISION NOT NULL,
    station_fmisid TEXT NOT NULL,
    station_name   TEXT NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS home_location;
