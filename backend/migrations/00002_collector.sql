-- +goose Up
-- Per-collector health/status state (spec §11, feeds the §35 health
-- endpoint). Small, mutable, not a time series — plain table.
CREATE TABLE collector (
    name            TEXT PRIMARY KEY,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    status          TEXT NOT NULL DEFAULT 'unknown'
                        CHECK (status IN ('unknown', 'ok', 'degraded', 'error', 'disabled')),
    last_success_at TIMESTAMPTZ,
    last_error      TEXT,
    last_error_at   TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS collector;
