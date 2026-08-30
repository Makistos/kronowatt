// Package storage holds the repository layer — all direct SQL against the
// schema in backend/migrations. Collectors and the API depend on this
// package's types, never on pgx directly (spec §2.5: hide provider/storage
// details behind interfaces).
package storage

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, dsn)
}
