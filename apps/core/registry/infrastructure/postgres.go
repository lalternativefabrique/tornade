// Package infrastructure persists App aggregates in Postgres and seals their
// key.
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lalternativefabrique/tornade/core/registry/domain"
)

var ErrNotFound = errors.New("registry: app not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const columns = `name, secret, last4, previous_secret, rotated_at, revoked_at, created_at, updated_at`

func (r *Repository) Get(ctx context.Context, name string) (*domain.App, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columns+` FROM registry_apps WHERE name = $1`, name)
	a, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (r *Repository) List(ctx context.Context) ([]*domain.App, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columns+` FROM registry_apps ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []*domain.App
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

// Save writes the aggregate whole: a rotation or a revocation is one row.
func (r *Repository) Save(ctx context.Context, a *domain.App) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO registry_apps (`+columns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (name) DO UPDATE SET
			secret = EXCLUDED.secret,
			last4 = EXCLUDED.last4,
			previous_secret = EXCLUDED.previous_secret,
			rotated_at = EXCLUDED.rotated_at,
			revoked_at = EXCLUDED.revoked_at,
			updated_at = EXCLUDED.updated_at`,
		a.Name, a.Current, a.Last4, nullBytes(a.Previous),
		a.RotatedAt, a.RevokedAt, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("registry: save %s: %w", a.Name, err)
	}
	return nil
}

func scan(row pgx.Row) (*domain.App, error) {
	var a domain.App
	if err := row.Scan(&a.Name, &a.Current, &a.Last4, &a.Previous,
		&a.RotatedAt, &a.RevokedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
