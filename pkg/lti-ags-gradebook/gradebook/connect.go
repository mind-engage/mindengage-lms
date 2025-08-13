// pkg/lti-ags-gradebook/gradebook/connect.go
package gradebook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Connect opens a *sql.DB for the given driver and dsn and tunes basic pool/PRAGMA settings.
// Note: you still need to import the actual driver elsewhere (e.g. _ "github.com/lib/pq" or _ "modernc.org/sqlite").
func Connect(ctx context.Context, driver, dsn string) (*sql.DB, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	// Basic pool tuning; tweak as you like.
	switch normalizeDriver(driver) {
	case "sqlite", "sqlite3":
		// SQLite should not use many concurrent writers; keep pool small.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	default:
		// Reasonable defaults for Postgres; adjust to your env.
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(10)
	}
	db.SetConnMaxLifetime(30 * time.Minute)

	// Ping with context to fail fast.
	if err := pingContext(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	// SQLite PRAGMAs for reliability/perf
	if isSQLite(driver) {
		if _, err := db.ExecContext(ctx, `
			PRAGMA foreign_keys = ON;
			PRAGMA journal_mode = WAL;
			PRAGMA synchronous = NORMAL;
			PRAGMA busy_timeout = 5000;
		`); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("sqlite pragmas: %w", err)
		}
	}

	return db, nil
}

// Migrate applies the schema for the selected driver (idempotent CREATE IF NOT EXISTS).
func Migrate(ctx context.Context, db *sql.DB, driver string) error {
	var schema string
	switch normalizeDriver(driver) {
	case "postgres", "postgresql":
		schema = schemaPostgres
	case "sqlite", "sqlite3":
		schema = schemaSQLite
	default:
		return fmt.Errorf("unsupported driver %q (expected postgres/sqlite)", driver)
	}

	// Try to run as a single script first; if driver rejects multi statements, fall back to splitting.
	if _, err := db.ExecContext(ctx, schema); err != nil {
		// Fallback: naive split on semicolons. Good enough for these DDL statements.
		for _, stmt := range splitSQL(schema) {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, e := db.ExecContext(ctx, stmt); e != nil {
				return fmt.Errorf("migration failed at: %s\nerror: %w", firstLine(stmt), e)
			}
		}
	}
	return nil
}

// ConnectAndMigrate is a convenience that opens the DB and applies migrations.
func ConnectAndMigrate(ctx context.Context, driver, dsn string) (*sql.DB, error) {
	db, err := Connect(ctx, driver, dsn)
	if err != nil {
		return nil, err
	}
	if err := Migrate(ctx, db, driver); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func pingContext(ctx context.Context, db *sql.DB) error {
	done := make(chan error, 1)
	go func() { done <- db.Ping() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return errors.New("ping timeout/canceled")
	}
}

func normalizeDriver(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	switch d {
	case "pgx", "pgsql":
		return "postgres"
	}
	return d
}

func isSQLite(d string) bool {
	switch normalizeDriver(d) {
	case "sqlite", "sqlite3":
		return true
	default:
		return false
	}
}

// splitSQL naively splits on ';' boundaries.
// This is acceptable for our simple DDL (no procedures/functions).
func splitSQL(s string) []string {
	parts := strings.Split(s, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p+";")
		}
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ------------------------ Schemas ------------------------

const schemaPostgres = `
-- LTI platforms (per issuer/client)
CREATE TABLE IF NOT EXISTS lti_platforms (
  issuer              TEXT PRIMARY KEY,
  client_id           TEXT NOT NULL,
  token_url           TEXT NOT NULL,
  jwks_url            TEXT NOT NULL,
  auth_url            TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Launch context / resource link metadata captured at LTI launch
CREATE TABLE IF NOT EXISTS lti_links (
  id                  BIGSERIAL PRIMARY KEY,
  platform_issuer     TEXT NOT NULL REFERENCES lti_platforms(issuer) ON DELETE CASCADE,
  deployment_id       TEXT NOT NULL,
  context_id          TEXT NOT NULL,
  resource_link_id    TEXT NOT NULL,
  lineitems_url       TEXT,             -- from AGS service claim
  scopes              JSONB,            -- array of granted scopes
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(platform_issuer, deployment_id, context_id, resource_link_id)
);

-- Map platform user (launch sub) to local user id
CREATE TABLE IF NOT EXISTS lti_user_map (
  platform_issuer     TEXT NOT NULL,
  platform_sub        TEXT NOT NULL,
  local_user_id       TEXT NOT NULL,  -- align to your users.id type
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (platform_issuer, platform_sub)
);

-- One line-item per (exam x context) on a given platform
CREATE TABLE IF NOT EXISTS gradebook_lineitems (
  id                  BIGSERIAL PRIMARY KEY,
  exam_id             TEXT NOT NULL,  -- align to your exam id type
  platform_issuer     TEXT NOT NULL,
  deployment_id       TEXT NOT NULL,
  context_id          TEXT NOT NULL,
  resource_link_id    TEXT NOT NULL,
  label               TEXT NOT NULL,
  score_max           NUMERIC NOT NULL,
  line_item_url       TEXT NOT NULL,  -- absolute URL of created/reused line item
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (exam_id, platform_issuer, deployment_id, context_id, resource_link_id)
);

-- Status for passback per attempt
CREATE TABLE IF NOT EXISTS grade_sync_status (
  attempt_id          TEXT PRIMARY KEY, -- align to your attempt id type
  status              TEXT NOT NULL CHECK (status IN ('pending','ok','failed')),
  retries             INT NOT NULL DEFAULT 0,
  last_error          TEXT,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// SQLite schema uses compatible types and CURRENT_TIMESTAMP defaults.
// JSON is stored as TEXT with json_valid() check (requires JSON1, present in common builds).
const schemaSQLite = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS lti_platforms (
  issuer              TEXT PRIMARY KEY,
  client_id           TEXT NOT NULL,
  token_url           TEXT NOT NULL,
  jwks_url            TEXT NOT NULL,
  auth_url            TEXT NOT NULL,
  created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS lti_links (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  platform_issuer     TEXT NOT NULL REFERENCES lti_platforms(issuer) ON DELETE CASCADE,
  deployment_id       TEXT NOT NULL,
  context_id          TEXT NOT NULL,
  resource_link_id    TEXT NOT NULL,
  lineitems_url       TEXT,
  scopes              TEXT, -- JSON array as TEXT
  created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(platform_issuer, deployment_id, context_id, resource_link_id),
  CHECK (scopes IS NULL OR json_valid(scopes))
);

CREATE TABLE IF NOT EXISTS lti_user_map (
  platform_issuer     TEXT NOT NULL,
  platform_sub        TEXT NOT NULL,
  local_user_id       TEXT NOT NULL,
  created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (platform_issuer, platform_sub)
);

CREATE TABLE IF NOT EXISTS gradebook_lineitems (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  exam_id             TEXT NOT NULL,
  platform_issuer     TEXT NOT NULL,
  deployment_id       TEXT NOT NULL,
  context_id          TEXT NOT NULL,
  resource_link_id    TEXT NOT NULL,
  label               TEXT NOT NULL,
  score_max           REAL NOT NULL,
  line_item_url       TEXT NOT NULL,
  created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (exam_id, platform_issuer, deployment_id, context_id, resource_link_id)
);

CREATE TABLE IF NOT EXISTS grade_sync_status (
  attempt_id          TEXT PRIMARY KEY,
  status              TEXT NOT NULL CHECK (status IN ('pending','ok','failed')),
  retries             INTEGER NOT NULL DEFAULT 0,
  last_error          TEXT,
  updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`
