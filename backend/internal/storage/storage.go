// Package storage holds the repository layer — all direct SQL against the
// schema in backend/migrations. Collectors and the API depend on this
// package's types, never on pgx directly (spec §2.5: hide provider/storage
// details behind interfaces).
package storage

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by a repository's update/lookup-by-id methods
// when no row matches — callers (the API layer) map it to a 404.
var ErrNotFound = errors.New("not found")

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, dsn)
}
