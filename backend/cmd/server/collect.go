package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"kronowatt/backend/internal/collectors/weather"
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

	job := newWeatherJob(cfg, storage.NewWeatherRepository(pool), storage.NewCollectorRepository(pool))
	if err := job(ctx); err != nil {
		log.Fatalf("collect weather: %v", err)
	}
}

// newWeatherJob wires the weather collector's fetch/parse logic (which
// knows nothing about storage — spec §2.5) to the repository layer and
// collector health tracking, as a scheduler.Job.
func newWeatherJob(cfg config.Config, repo *storage.WeatherRepository, collectorRepo *storage.CollectorRepository) scheduler.Job {
	const name = "fmi_observation"
	client := &http.Client{Timeout: 30 * time.Second}

	return func(ctx context.Context) error {
		obs, err := weather.FetchObservations(ctx, client, cfg.FMIStationFMISID, time.Time{})
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
