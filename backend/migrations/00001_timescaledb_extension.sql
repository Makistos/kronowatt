-- +goose Up
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- +goose Down
-- Deliberately no-op: dropping the extension would cascade-drop every
-- hypertable built on top of it.
