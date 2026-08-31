package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"kronowatt/backend/internal/config"
	"kronowatt/backend/internal/domain"
	"kronowatt/backend/internal/storage"
)

// runSeed loads frontend/scripts/generate-fake-data.mjs's output
// (frontend/static/fake-data/*.json) into the real database, so the API
// (and eventually the frontend, once it's switched over) has multi-year
// data to serve without needing real collectors for everything yet.
//
// This is dev/test tooling, not a spec-required feature — the fake-data
// generator's fixtures are the seed source specifically so there's one
// definition of "what the synthetic dataset looks like," not two.
func runSeed(args []string) {
	dir := "../frontend/static/fake-data"
	if len(args) > 0 {
		dir = args[0]
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

	manifest, err := readManifest(dir)
	if err != nil {
		log.Fatalf("read manifest: %v (did you run `npm run generate:fake-data` in frontend/?)", err)
	}

	electricityRepo := storage.NewElectricityRepository(pool)
	weatherRepo := storage.NewWeatherRepository(pool)
	spotPriceRepo := storage.NewSpotPriceRepository(pool)
	evRepo := storage.NewEVRepository(pool)

	for _, year := range manifest.Years {
		if err := seedElectricity(ctx, electricityRepo, dir, year); err != nil {
			log.Fatalf("seed electricity %d: %v", year, err)
		}
		if err := seedWeather(ctx, weatherRepo, dir, year); err != nil {
			log.Fatalf("seed weather %d: %v", year, err)
		}
		if err := seedSpotPrice(ctx, spotPriceRepo, dir, year); err != nil {
			log.Fatalf("seed spot price %d: %v", year, err)
		}
		if err := seedEV(ctx, evRepo, dir, year); err != nil {
			log.Fatalf("seed EV %d: %v", year, err)
		}
		log.Printf("seeded %d", year)
	}
}

type fakeDataManifest struct {
	Years []int `json:"years"`
}

func readManifest(dir string) (fakeDataManifest, error) {
	var m fakeDataManifest
	if err := readJSONFile(filepath.Join(dir, "manifest.json"), &m); err != nil {
		return fakeDataManifest{}, err
	}
	return m, nil
}

func readJSONFile(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}

// fake_seed is used as the source/station marker for every table seeded
// here, distinct from any real collector's source label (e.g. "cozify_ws",
// "101786", "defa_cloud") — so seeded rows are always identifiable and
// never silently collide with real collected data.
const fakeSeedSource = "fake_seed"

type fakeElectricityRow struct {
	Time      string  `json:"time"`
	PowerKW   float64 `json:"power_kw"`
	EnergyKWh float64 `json:"energy_kwh"`
}

func seedElectricity(ctx context.Context, repo *storage.ElectricityRepository, dir string, year int) error {
	var rows []fakeElectricityRow
	if err := readJSONFile(filepath.Join(dir, fmt.Sprintf("electricity_%d.json", year)), &rows); err != nil {
		return err
	}

	measurements := make([]domain.ElectricityMeasurement, 0, len(rows))
	var cumulativeKWh float64
	for _, row := range rows {
		t, err := time.Parse(time.RFC3339, row.Time)
		if err != nil {
			return fmt.Errorf("parse time %q: %w", row.Time, err)
		}
		cumulativeKWh += row.EnergyKWh
		ic := cumulativeKWh
		power := row.PowerKW
		measurements = append(measurements, domain.ElectricityMeasurement{
			Time:       t,
			Source:     fakeSeedSource,
			IC:         &ic,
			P:          []float64{power},
			InsertedAt: time.Now().UTC(),
		})
	}

	_, err := repo.UpsertMeasurements(ctx, measurements)
	return err
}

type fakeWeatherRow struct {
	Time         string  `json:"time"`
	TemperatureC float64 `json:"temperature_c"`
}

func seedWeather(ctx context.Context, repo *storage.WeatherRepository, dir string, year int) error {
	var rows []fakeWeatherRow
	if err := readJSONFile(filepath.Join(dir, fmt.Sprintf("weather_%d.json", year)), &rows); err != nil {
		return err
	}

	observations := make([]domain.WeatherObservation, 0, len(rows))
	for _, row := range rows {
		t, err := time.Parse(time.RFC3339, row.Time)
		if err != nil {
			return fmt.Errorf("parse time %q: %w", row.Time, err)
		}
		temp := row.TemperatureC
		observations = append(observations, domain.WeatherObservation{
			Time: t,
			// "fake" (not the real 101786 FMISID) keeps seeded synthetic
			// weather clearly separate from real FMI-collected data.
			StationFMISID:  "fake",
			AirTemperature: &temp,
			RetrievedAt:    time.Now().UTC(),
		})
	}

	_, err := repo.UpsertObservations(ctx, observations)
	return err
}

type fakeSpotPriceRow struct {
	Time        string  `json:"time"`
	PriceEURMWh float64 `json:"price_eur_mwh"`
}

func seedSpotPrice(ctx context.Context, repo *storage.SpotPriceRepository, dir string, year int) error {
	var rows []fakeSpotPriceRow
	if err := readJSONFile(filepath.Join(dir, fmt.Sprintf("spot_price_%d.json", year)), &rows); err != nil {
		return err
	}

	prices := make([]domain.SpotPrice, 0, len(rows))
	for _, row := range rows {
		t, err := time.Parse(time.RFC3339, row.Time)
		if err != nil {
			return fmt.Errorf("parse time %q: %w", row.Time, err)
		}
		prices = append(prices, domain.SpotPrice{
			IntervalStart: t,
			IntervalEnd:   t.Add(time.Hour), // fake data is hourly, not the real 15-min spec
			Price:         row.PriceEURMWh,
			Currency:      "EUR",
			Unit:          "EUR/MWh",
			Source:        fakeSeedSource,
			RetrievedAt:   time.Now().UTC(),
		})
	}

	_, err := repo.UpsertPrices(ctx, prices)
	return err
}

type fakeEVSession struct {
	StartTime      string  `json:"start_time"`
	EndTime        string  `json:"end_time"`
	EnergyKWh      float64 `json:"energy_kwh"`
	AveragePowerKW float64 `json:"average_power_kw"`
	MaximumPowerKW float64 `json:"maximum_power_kw"`
}

func seedEV(ctx context.Context, repo *storage.EVRepository, dir string, year int) error {
	var rows []fakeEVSession
	if err := readJSONFile(filepath.Join(dir, fmt.Sprintf("ev_sessions_%d.json", year)), &rows); err != nil {
		return err
	}

	sessions := make([]domain.EVChargingSession, 0, len(rows))
	for _, row := range rows {
		start, err := time.Parse(time.RFC3339, row.StartTime)
		if err != nil {
			return fmt.Errorf("parse start_time %q: %w", row.StartTime, err)
		}
		end, err := time.Parse(time.RFC3339, row.EndTime)
		if err != nil {
			return fmt.Errorf("parse end_time %q: %w", row.EndTime, err)
		}
		energy, avgPower, maxPower := row.EnergyKWh, row.AveragePowerKW, row.MaximumPowerKW
		sessions = append(sessions, domain.EVChargingSession{
			StartTime:      start,
			EndTime:        &end,
			EnergyKWh:      &energy,
			AveragePowerKW: &avgPower,
			MaximumPowerKW: &maxPower,
			// fake_seed, not the generator's own "defa_cloud" label — keeps
			// synthetic sessions distinguishable from real Defa data later.
			Source: fakeSeedSource,
		})
	}

	_, err := repo.UpsertSessions(ctx, sessions)
	return err
}
