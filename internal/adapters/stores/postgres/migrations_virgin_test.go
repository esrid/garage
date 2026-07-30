package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Every other test in this package runs against whatever database TEST_DATABASE_DSN
// names, and that database keeps its schema between runs. That is exactly how a
// release-blocking bug shipped: 00008 recreated an index 00006 already owned, so
// migration 8 failed on any database that had never seen it — every fresh install
// — while every development database, migrated before the collision existed, kept
// working and kept the suite green.
//
// This test is the guard that was missing. It creates a genuinely empty database,
// migrates it from nothing, and refuses to pass unless it reaches the last
// migration on disk.
func TestMigrationsRunOnAVirginDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is required for the PostgreSQL integration test")
	}

	ctx := context.Background()
	// A name unique per run: a leftover from a crashed run must never be mistaken
	// for a virgin database, which is the whole point of the test.
	name := fmt.Sprintf("garage_virgin_%d", time.Now().UnixNano())

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to the server: %v", err)
	}
	// t.Cleanup, not defer: cleanups run last in, first out and after every defer,
	// so registering the close here keeps the pool alive for the drop below.
	t.Cleanup(admin.Close)

	// CREATE DATABASE takes no parameters and cannot run inside a transaction, so
	// the name is built above rather than passed in, and it is ours alone.
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create the virgin database: %v", err)
	}
	t.Cleanup(func() {
		// WITH (FORCE) drops the database even if a pooled connection lingers,
		// so a failure here never leaves a stray database behind.
		if _, dropErr := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); dropErr != nil {
			t.Errorf("drop the virgin database %s: %v", name, dropErr)
		}
	})

	virginDSN, err := withDatabase(dsn, name)
	if err != nil {
		t.Fatalf("build the virgin DSN: %v", err)
	}

	store, err := Open(ctx, virginDSN)
	if err != nil {
		t.Fatalf("Open() on a virgin database error = %v\nthe application exits at startup when this fails", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	want, err := lastMigrationVersion()
	if err != nil {
		t.Fatalf("read the embedded migrations: %v", err)
	}
	var got int64
	if err := store.pool.QueryRow(ctx, "SELECT max(version_id) FROM goose_db_version").Scan(&got); err != nil {
		t.Fatalf("read the applied version: %v", err)
	}
	if got != want {
		t.Errorf("applied version = %d, want %d: a migration on disk never ran", got, want)
	}

	// Opening again must be a no-op: a second instance starting on the same
	// database is the normal case on a deploy, not an error.
	second, err := Open(ctx, virginDSN)
	if err != nil {
		t.Fatalf("Open() on an already migrated database error = %v", err)
	}
	_ = second.Close()
}

// withDatabase swaps the database name of a DSN, keeping every other parameter.
func withDatabase(dsn, name string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + name
	return parsed.String(), nil
}

// lastMigrationVersion is the highest version embedded in the binary, read the
// same way goose reads it: the digits before the first underscore.
func lastMigrationVersion() (int64, error) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return 0, err
	}
	var last int64
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		digits, _, found := strings.Cut(name, "_")
		if !found {
			return 0, fmt.Errorf("migration %q has no version prefix", name)
		}
		// Base 10 explicitly: the names are zero padded, and a leading zero must
		// not be read as octal.
		version, err := strconv.ParseInt(digits, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("migration %q: %w", name, err)
		}
		last = max(last, version)
	}
	if last == 0 {
		return 0, fmt.Errorf("no migration found in the embedded filesystem")
	}
	return last, nil
}
