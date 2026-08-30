// Package config loads runtime configuration from environment variables.
// Env vars (not a YAML/config file) were chosen because the target
// deployment is a systemd service with an EnvironmentFile (see
// deployment/kronowatt-backend.service) — no config-file parser/dependency
// needed. Revisit if a collector ever needs config too structured for flat
// env vars (e.g. Cozify's per-device settings).
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	DBDSN            string
	HTTPAddr         string
	FMIStationFMISID string
	FMIPollInterval  time.Duration
	DiskCheckPath    string
}

func Load() (Config, error) {
	cfg := Config{
		DBDSN:            os.Getenv("KRONOWATT_DB_DSN"),
		HTTPAddr:         getEnvDefault("KRONOWATT_HTTP_ADDR", ":8080"),
		FMIStationFMISID: getEnvDefault("KRONOWATT_FMI_STATION_FMISID", "101786"),
		// Spec §9a: the target box has a single ~30GB SSD, so any path on
		// it reflects overall disk pressure — default to the root
		// filesystem rather than requiring one more env var to be set.
		DiskCheckPath: getEnvDefault("KRONOWATT_DISK_CHECK_PATH", "/"),
	}

	if cfg.DBDSN == "" {
		return Config{}, fmt.Errorf("KRONOWATT_DB_DSN must be set")
	}

	interval, err := parseDurationDefault("KRONOWATT_FMI_POLL_INTERVAL", 10*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.FMIPollInterval = interval

	return cfg, nil
}

func getEnvDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func parseDurationDefault(name string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return d, nil
}
