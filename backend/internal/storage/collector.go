package storage

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CollectorRepository tracks per-collector health state (spec §11 collector
// table, feeds the §35 health endpoint).
type CollectorRepository struct {
	pool *pgxpool.Pool
}

func NewCollectorRepository(pool *pgxpool.Pool) *CollectorRepository {
	return &CollectorRepository{pool: pool}
}

func (r *CollectorRepository) RecordSuccess(ctx context.Context, name string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO collector (name, status, last_success_at, updated_at)
		VALUES ($1, 'ok', now(), now())
		ON CONFLICT (name) DO UPDATE SET
			status = 'ok',
			last_success_at = now(),
			updated_at = now()
	`, name)
	return err
}

func (r *CollectorRepository) RecordError(ctx context.Context, name string, collectErr error) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO collector (name, status, last_error, last_error_at, updated_at)
		VALUES ($1, 'error', $2, now(), now())
		ON CONFLICT (name) DO UPDATE SET
			status = 'error',
			last_error = $2,
			last_error_at = now(),
			updated_at = now()
	`, name, collectErr.Error())
	return err
}
