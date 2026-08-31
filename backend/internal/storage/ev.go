package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kronowatt/backend/internal/domain"
)

type EVRepository struct {
	pool *pgxpool.Pool
}

func NewEVRepository(pool *pgxpool.Pool) *EVRepository {
	return &EVRepository{pool: pool}
}

func (r *EVRepository) UpsertSessions(ctx context.Context, rows []domain.EVChargingSession) (int64, error) {
	var inserted int64
	for _, s := range rows {
		tag, err := r.pool.Exec(ctx, `
			INSERT INTO ev_charging_session (start_time, end_time, energy_kwh, average_power_kw, maximum_power_kw, source)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (source, start_time) DO NOTHING
		`, s.StartTime, s.EndTime, s.EnergyKWh, s.AveragePowerKW, s.MaximumPowerKW, s.Source)
		if err != nil {
			return inserted, err
		}
		inserted += tag.RowsAffected()
	}
	return inserted, nil
}

func (r *EVRepository) ListSessions(ctx context.Context, start, end time.Time) ([]domain.EVChargingSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT start_time, end_time, energy_kwh, average_power_kw, maximum_power_kw, source
		FROM ev_charging_session
		WHERE start_time >= $1 AND start_time < $2
		ORDER BY start_time
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.EVChargingSession
	for rows.Next() {
		var s domain.EVChargingSession
		if err := rows.Scan(&s.StartTime, &s.EndTime, &s.EnergyKWh, &s.AveragePowerKW, &s.MaximumPowerKW, &s.Source); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
