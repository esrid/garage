package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/samber/oops"
	modernsqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func (s *Store) withinTransaction(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return oops.Code("db_begin_failed").Wrap(err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback()
			panic(recovered)
		} else if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return oops.Code("db_commit_failed").Wrap(err)
	}
	return nil
}

func decorateError(err error, operation string) error {
	if err == nil {
		return nil
	}
	wrapped := oops.Code("database_error").With("op", operation)
	var sqliteErr *modernsqlite.Error
	if errors.As(err, &sqliteErr) {
		return wrapped.With("sqlite_code", sqliteErr.Code()).Wrap(err)
	}
	return wrapped.Wrap(err)
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr *modernsqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE ||
			sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func collectRows[T any](rows *sql.Rows, operation string, scan func(*sql.Rows) (T, error)) ([]T, error) {
	defer func() { _ = rows.Close() }()
	var values []T
	for rows.Next() {
		value, err := scan(rows)
		if err != nil {
			return nil, decorateError(err, operation)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, decorateError(err, operation)
	}
	return values, nil
}

func queryRow[T any](row *sql.Row, notFound error, operation string, scan func(*sql.Row) (T, error)) (T, error) {
	value, err := scan(row)
	if err != nil {
		var zero T
		if errors.Is(err, sql.ErrNoRows) {
			return zero, notFound
		}
		return zero, decorateError(err, operation)
	}
	return value, nil
}
