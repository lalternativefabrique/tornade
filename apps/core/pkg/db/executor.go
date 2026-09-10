// Package db wires the application's Postgres connection pool and applies
// file-based SQL migrations at boot.
package db

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"path"
	"sort"

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

// Migrate applies every *.sql file under dir of fsys in lexicographic order.
// A missing directory is an error: silence there is how a database ends up
// without its tables and nobody notices until a request fails.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && path.Ext(e.Name()) == ".sql" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		b, err := fs.ReadFile(fsys, path.Join(dir, n))
		if err != nil {
			return fmt.Errorf("read %s: %w", n, err)
		}
		if _, err := pool.Exec(ctx, string(b)); err != nil {
			return fmt.Errorf("apply %s: %w", n, err)
		}
	}
	log.Printf("tornade: %d migration(s) applied", len(names))
	return nil
}
