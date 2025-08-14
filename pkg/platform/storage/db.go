// pkg/platform/storage/db.go
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DB is a thin wrapper around *sql.DB so we can hang helpers off it.
type DB struct {
	SQL *sql.DB
}

// Connect opens a database connection, tunes the pool, applies driver-specific
// pragmas (for SQLite), and verifies connectivity with PingContext.
// You must import the driver in your main package, e.g.:
//
//	_ "github.com/lib/pq"        // registers "postgres"
//	_ "modernc.org/sqlite"       // registers "sqlite"
func Connect(ctx context.Context, driver, dsn string) (*DB, error) {
	if strings.TrimSpace(driver) == "" {
		return nil, errors.New("storage: driver is required")
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}

	// Pool tuning defaults
	tunePool(normalizeDriver(driver), db)

	// Verify connectivity
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage: ping: %w", err)
	}

	// Driver-specific setup
	if isSQLite(driver) {
		if err := applySQLitePragmas(ctx, db); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	return &DB{SQL: db}, nil
}

// Close closes the underlying *sql.DB (safe to call multiple times).
func (d *DB) Close() error {
	if d == nil || d.SQL == nil {
		return nil
	}
	return d.SQL.Close()
}

// Ping checks connectivity using PingContext on the underlying DB.
func (d *DB) Ping(ctx context.Context) error {
	if d == nil || d.SQL == nil {
		return errors.New("storage: DB is nil")
	}
	return d.SQL.PingContext(ctx)
}

// WithTx starts a transaction, runs fn, and commits if fn returns nil.
// If fn returns an error, the transaction is rolled back and that error is returned.
// If commit fails, the commit error is returned.
//
// Typical usage:
//
//	err := storage.WithTx(ctx, db, nil, func(tx *sql.Tx) error {
//	    // use tx.ExecContext / tx.QueryContext ...
//	    return nil
//	})
func WithTx(ctx context.Context, d *DB, opts *sql.TxOptions, fn func(*sql.Tx) error) (err error) {
	if d == nil || d.SQL == nil {
		return errors.New("storage: DB is nil")
	}
	tx, err := d.SQL.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("storage: begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		if e := tx.Commit(); e != nil {
			err = fmt.Errorf("storage: commit: %w", e)
		}
	}()
	err = fn(tx)
	return
}

// tunePool sets conservative defaults and allows the driver to override.
func tunePool(driver string, db *sql.DB) {
	// Reasonable defaults for server databases.
	maxOpen := 20
	maxIdle := 10
	connLife := 45 * time.Minute
	idleLife := 15 * time.Minute

	switch driver {
	case "sqlite", "sqlite3":
		// SQLite (single writer): keep the pool tiny to avoid busy errors.
		maxOpen = 1
		maxIdle = 1
		connLife = 0 // unlimited
		idleLife = 0 // unlimited
	case "postgres":
		// Postgres defaults above are fine; adjust if needed.
	default:
		// Other drivers stick to defaults.
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(connLife)
	db.SetConnMaxIdleTime(idleLife)
}

// applySQLitePragmas applies WAL and other reliability-focused pragmas.
// Works with common SQLite builds (modernc.org/sqlite, mattn/go-sqlite3).
func applySQLitePragmas(ctx context.Context, db *sql.DB) error {
	// We intentionally ignore the result rows; errors are important.
	// Use separate Execs to ensure all are applied even if one is a no-op.
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",   // better concurrency/durability
		"PRAGMA synchronous = NORMAL;", // balance perf and safety
		"PRAGMA busy_timeout = 5000;",  // ms
		"PRAGMA temp_store = MEMORY;",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("storage: sqlite pragma %q: %w", p, err)
		}
	}
	return nil
}

// normalizeDriver maps common aliases to canonical names.
func normalizeDriver(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	switch d {
	case "pg", "pgsql", "pgx":
		return "postgres"
	case "sqlite3":
		return "sqlite"
	default:
		return d
	}
}

func isSQLite(d string) bool {
	switch normalizeDriver(d) {
	case "sqlite", "sqlite3":
		return true
	default:
		return false
	}
}
