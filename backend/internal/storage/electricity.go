package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kronowatt/backend/internal/domain"
)

type ElectricityRepository struct {
	pool *pgxpool.Pool
}

func NewElectricityRepository(pool *pgxpool.Pool) *ElectricityRepository {
	return &ElectricityRepository{pool: pool}
}

// UpsertMeasurements inserts measurements, skipping ones already present
// for `time` — spec §2.6. UNIQUE(time) alone (not (time, source)) is
// deliberate: see the 00003 migration's comment on why source must not be
// part of the key.
func (r *ElectricityRepository) UpsertMeasurements(ctx context.Context, rows []domain.ElectricityMeasurement) (int64, error) {
	var inserted int64
	for _, m := range rows {
		tag, err := r.pool.Exec(ctx, `
			INSERT INTO electricity_measurement (
				time, source, ic, ec, ric, rec, p, pi, pe, r, ri, re, u, i, raw_payload, inserted_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			ON CONFLICT (time) DO NOTHING
		`,
			m.Time, m.Source, m.IC, m.EC, m.RIC, m.REC, m.P, m.PI, m.PE, m.R, m.RI, m.RE, m.U, m.I,
			m.RawPayload, m.InsertedAt,
		)
		if err != nil {
			return inserted, err
		}
		inserted += tag.RowsAffected()
	}
	return inserted, nil
}

func (r *ElectricityRepository) ListMeasurements(ctx context.Context, start, end time.Time) ([]domain.ElectricityMeasurement, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT time, source, ic, ec, ric, rec, p, pi, pe, r, ri, re, u, i, inserted_at
		FROM electricity_measurement
		WHERE time >= $1 AND time < $2
		ORDER BY time
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.ElectricityMeasurement
	for rows.Next() {
		var m domain.ElectricityMeasurement
		if err := rows.Scan(
			&m.Time, &m.Source, &m.IC, &m.EC, &m.RIC, &m.REC, &m.P, &m.PI, &m.PE, &m.R, &m.RI, &m.RE, &m.U, &m.I,
			&m.InsertedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DistinctYears returns the UTC years present in electricity_measurement,
// ascending. Electricity is the "spine" dataset the dashboard treats as
// primary, so it's the one used to answer "what years have data" — used by
// the frontend instead of hardcoding a year list.
func (r *ElectricityRepository) DistinctYears(ctx context.Context) ([]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT EXTRACT(YEAR FROM time)::int AS year
		FROM electricity_measurement
		ORDER BY year
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int
	for rows.Next() {
		var y int
		if err := rows.Scan(&y); err != nil {
			return nil, err
		}
		out = append(out, y)
	}
	return out, rows.Err()
}
