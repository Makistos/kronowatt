# Thin layer on top of the official TimescaleDB image (spec §9: use the
# official image for extension/version management rather than hand-rolling
# Postgres + extensions). This only adds the init scripts that provision
# the timescaledb extension on first container start.
FROM timescale/timescaledb:latest-pg16

COPY init/ /docker-entrypoint-initdb.d/
