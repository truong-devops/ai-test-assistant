package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string, maxConnections int32) (*Postgres, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	config.MaxConns = maxConnections

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	db := &Postgres{pool: pool}
	if err := db.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

func (p *Postgres) Pool() *pgxpool.Pool {
	return p.pool
}

func (p *Postgres) Ping(ctx context.Context) error {
	if err := p.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (p *Postgres) Close() {
	p.pool.Close()
}
