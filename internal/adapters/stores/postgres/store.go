package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const connectTimeout = 5 * time.Second

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// Parse errors can echo the supplied connection string, which may contain
		// a password. Keep the operational error safe for logs.
		return nil, errors.New("postgres: invalid DATABASE_DSN")
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	if err := runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	return s.pool.Ping(pingCtx)
}

func (s *Store) Close() error {
	s.pool.Close()
	return nil
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) (err error) {
	migrationFS, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("postgres: migration filesystem: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrationFS,
		goose.WithDisableGlobalRegistry(true),
		goose.WithLogger(goose.NopLogger()),
	)
	if err != nil {
		providerErr := fmt.Errorf("postgres: migration provider: %w", err)
		if closeErr := db.Close(); closeErr != nil {
			return errors.Join(providerErr, fmt.Errorf("postgres: close migration database: %w", closeErr))
		}
		return providerErr
	}
	defer func() {
		if closeErr := provider.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("postgres: close migration provider: %w", closeErr))
		}
	}()

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("postgres: migrations: %w", err)
	}
	return nil
}
