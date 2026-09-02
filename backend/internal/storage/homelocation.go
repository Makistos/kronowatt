package storage

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kronowatt/backend/internal/domain"
)

type HomeLocationRepository struct {
	pool *pgxpool.Pool
}

func NewHomeLocationRepository(pool *pgxpool.Pool) *HomeLocationRepository {
	return &HomeLocationRepository{pool: pool}
}

// Get returns the configured home location, or (zero value, false, nil) if
// none has been set yet — a fresh install has no row until the settings UI
// saves one.
func (r *HomeLocationRepository) Get(ctx context.Context) (domain.HomeLocation, bool, error) {
	var loc domain.HomeLocation
	err := r.pool.QueryRow(ctx, `
		SELECT latitude, longitude, station_fmisid, station_name, updated_at
		FROM home_location WHERE id = 1
	`).Scan(&loc.Latitude, &loc.Longitude, &loc.StationFMISID, &loc.StationName, &loc.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.HomeLocation{}, false, nil
	}
	if err != nil {
		return domain.HomeLocation{}, false, err
	}
	return loc, true, nil
}

// Upsert replaces the single home_location row — always a full overwrite,
// not a partial update, since there's exactly one row and no history to
// preserve (unlike contracts, spec §2.4 doesn't apply here).
func (r *HomeLocationRepository) Upsert(ctx context.Context, loc domain.HomeLocation) (domain.HomeLocation, error) {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO home_location (id, latitude, longitude, station_fmisid, station_name, updated_at)
		VALUES (1, $1, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE SET
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			station_fmisid = EXCLUDED.station_fmisid,
			station_name = EXCLUDED.station_name,
			updated_at = now()
		RETURNING latitude, longitude, station_fmisid, station_name, updated_at
	`, loc.Latitude, loc.Longitude, loc.StationFMISID, loc.StationName).
		Scan(&loc.Latitude, &loc.Longitude, &loc.StationFMISID, &loc.StationName, &loc.UpdatedAt)
	if err != nil {
		return domain.HomeLocation{}, err
	}
	return loc, nil
}
