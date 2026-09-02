package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"kronowatt/backend/internal/collectors/fmi"
	"kronowatt/backend/internal/config"
	"kronowatt/backend/internal/scheduler"
	"kronowatt/backend/internal/storage"
)

// runCollect handles `kronowatt-server collect <name>` — a one-shot run for
// manual testing, independent of the continuous server loop.
func runCollect(args []string) {
	if len(args) != 1 || args[0] != "weather" {
		log.Fatal("usage: kronowatt-server collect weather")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx := context.Background()
	pool, err := storage.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	job := newFMIJob(cfg, storage.NewWeatherRepository(pool), storage.NewCollectorRepository(pool), storage.NewHomeLocationRepository(pool))
	if err := job(ctx); err != nil {
		log.Fatalf("collect weather: %v", err)
	}
}

// newFMIJob wires the FMI collector's fetch/parse logic (which knows
// nothing about storage — spec §2.5) to the repository layer and collector
// health tracking, as a scheduler.Job. The station to poll is resolved
// fresh on every run from home_location (set via the settings UI's
// "find nearest station" flow), not baked in once at startup — so changing
// it takes effect on the next poll, no restart needed. Falls back to
// cfg.FMIStationFMISID (the original static default) until a home location
// has ever been saved.
func newFMIJob(cfg config.Config, repo *storage.WeatherRepository, collectorRepo *storage.CollectorRepository, homeLocationRepo *storage.HomeLocationRepository) scheduler.Job {
	const name = "fmi_observation"
	client := &http.Client{Timeout: 30 * time.Second}

	return func(ctx context.Context) error {
		fmisid := cfg.FMIStationFMISID
		if loc, ok, err := homeLocationRepo.Get(ctx); err != nil {
			slog.Default().With("collector", name).Error("load home location failed, using default station", "error", err)
		} else if ok {
			fmisid = loc.StationFMISID
		}

		obs, err := fmi.FetchObservations(ctx, client, fmisid, time.Time{})
		if err != nil {
			_ = collectorRepo.RecordError(ctx, name, err)
			return fmt.Errorf("fetch: %w", err)
		}

		inserted, err := repo.UpsertObservations(ctx, obs)
		if err != nil {
			_ = collectorRepo.RecordError(ctx, name, err)
			return fmt.Errorf("store: %w", err)
		}

		if err := collectorRepo.RecordSuccess(ctx, name); err != nil {
			slog.Default().With("collector", name).Error("record health failed", "error", err)
		}

		slog.Default().With("collector", name).Info("collected", "fetched", len(obs), "inserted", inserted)
		return nil
	}
}
