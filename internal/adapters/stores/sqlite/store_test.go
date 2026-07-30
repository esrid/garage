package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenRunsMigrationsAndCanReopen(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "app.db")

	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	var migrationCount int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM goose_db_version").Scan(&migrationCount); err != nil {
		t.Fatalf("query migration table: %v", err)
	}
	if migrationCount == 0 {
		t.Fatal("migration table is empty")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestMemoryDatabaseUsesOneConnection(t *testing.T) {
	store, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if got := store.db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}
}

func TestWithinTransaction(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.ExecContext(ctx, "CREATE TABLE items (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	if err := store.withinTransaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO items (id) VALUES (1)")
		return err
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}

	rollbackErr := errors.New("rollback")
	err = store.withinTransaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO items (id) VALUES (2)"); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback transaction error = %v", err)
	}

	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM items").Scan(&count); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if count != 1 {
		t.Fatalf("item count = %d, want 1", count)
	}
}

func TestUniqueViolation(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.ExecContext(ctx, "CREATE TABLE users (email TEXT UNIQUE)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO users (email) VALUES ('a@example.com')"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = store.db.ExecContext(ctx, "INSERT INTO users (email) VALUES ('a@example.com')")
	if !isUniqueViolation(err) {
		t.Fatalf("isUniqueViolation(%v) = false", err)
	}
}
