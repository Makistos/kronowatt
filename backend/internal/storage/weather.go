package storage

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"kronowatt/backend/internal/domain"
)

type WeatherRepository struct {
	pool *pgxpool.Pool
}

func NewWeatherRepository(pool *pgxpool.Pool) *WeatherRepository {
	return &WeatherRepository{pool: pool}
}

// UpsertObservations inserts observations, silently skipping ones already
// present for (time, station_fmisid) — spec §2.6 idempotent collection.
// FMI observations aren't revised after publication, so DO NOTHING (not a
// merge/update) is sufficient. Returns the number of rows actually
// inserted, for logging/observability.
func (r *WeatherRepository) UpsertObservations(ctx context.Context, obs []domain.WeatherObservation) (int64, error) {
	var inserted int64
	for _, o := range obs {
		tag, err := r.pool.Exec(ctx, `
			INSERT INTO weather_observation (
				time, station_fmisid, air_temperature, relative_humidity, dew_point,
				air_pressure, wind_speed, wind_direction, wind_gust, precipitation,
				cloud_cover, visibility, raw_payload, retrieved_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (time, station_fmisid) DO NOTHING
		`,
			o.Time, o.StationFMISID, o.AirTemperature, o.RelativeHumidity, o.DewPoint,
			o.AirPressure, o.WindSpeed, o.WindDirection, o.WindGust, o.Precipitation,
			o.CloudCover, o.Visibility, o.RawPayload, o.RetrievedAt,
		)
		if err != nil {
			return inserted, err
		}
		inserted += tag.RowsAffected()
	}
	return inserted, nil
}
