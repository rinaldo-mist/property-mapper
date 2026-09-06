// Package store owns the database connection and the generated query layer.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open builds a connection pool.
//
// Because the API runs as a long-lived container rather than a per-request
// function, this is an ordinary warm pool talking to Neon's DIRECT endpoint —
// prepared statements work, which is what lets the generated queries perform as
// designed. A serverless deployment would have to use Neon's PgBouncer endpoint
// with pgx in simple-protocol mode instead.
//
// Size the pool against Cloud Run's --max-instances: the product of the two must
// stay inside Neon's connection limit. pool_max_conns is set in DATABASE_URL, so
// both halves of that budget live in one place.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	// Conservative defaults, overridable via the DSN. MaxConnIdleTime matters on
	// Cloud Run: an instance that has been idle long enough to be scaled down
	// should not be holding Neon connections open.
	if cfg.MaxConns == 0 {
		cfg.MaxConns = 5
	}
	if cfg.MaxConnIdleTime == 0 {
		cfg.MaxConnIdleTime = 5 * time.Minute
	}
	if cfg.MaxConnLifetime == 0 {
		cfg.MaxConnLifetime = time.Hour
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
