package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kronowatt/backend/internal/domain"
)

type SpotPriceRepository struct {
	pool *pgxpool.Pool
}

func NewSpotPriceRepository(pool *pgxpool.Pool) *SpotPriceRepository {
	return &SpotPriceRepository{pool: pool}
}

func (r *SpotPriceRepository) UpsertPrices(ctx context.Context, rows []domain.SpotPrice) (int64, error) {
	var inserted int64
	for _, p := range rows {
		tag, err := r.pool.Exec(ctx, `
			INSERT INTO spot_price (interval_start, interval_end, price, currency, unit, source, retrieved_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (interval_start, source) DO NOTHING
		`, p.IntervalStart, p.IntervalEnd, p.Price, p.Currency, p.Unit, p.Source, p.RetrievedAt)
		if err != nil {
			return inserted, err
		}
		inserted += tag.RowsAffected()
	}
	return inserted, nil
}

func (r *SpotPriceRepository) ListPrices(ctx context.Context, start, end time.Time) ([]domain.SpotPrice, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT interval_start, interval_end, price, currency, unit, source, retrieved_at
		FROM spot_price
		WHERE interval_start >= $1 AND interval_start < $2
		ORDER BY interval_start
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.SpotPrice
	for rows.Next() {
		var p domain.SpotPrice
		if err := rows.Scan(&p.IntervalStart, &p.IntervalEnd, &p.Price, &p.Currency, &p.Unit, &p.Source, &p.RetrievedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
