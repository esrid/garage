package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestOpenRunsMigrationsAndCanReopen(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is required for the PostgreSQL integration test")
	}

	ctx := context.Background()
	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	assertMigrationAppliedOnce(t, ctx, store)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	assertMigrationAppliedOnce(t, ctx, reopened)
	if err := reopened.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func assertMigrationAppliedOnce(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	var count int
	const query = `SELECT COUNT(*) FROM goose_db_version WHERE version_id = 1 AND is_applied`
	if err := store.pool.QueryRow(ctx, query).Scan(&count); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if count != 1 {
		t.Fatalf("applied migration version 1 count = %d, want 1", count)
	}
}

func TestOpenRejectsInvalidDSN(t *testing.T) {
	const secret = "must-not-appear"
	_, err := Open(context.Background(), "://invalid:"+secret)
	if err == nil {
		t.Fatal("Open() error = nil, want invalid DSN error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Open() error exposes DATABASE_DSN: %v", err)
	}
}
