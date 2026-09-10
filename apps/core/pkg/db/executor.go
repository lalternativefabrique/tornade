// Package db wires the application's Postgres connection pool. The schema is
// applied by the migrate job (apps/migrations) before this pool ever opens.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a pgx connection pool from a Postgres URL.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse postgres url: %w", err)
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}
