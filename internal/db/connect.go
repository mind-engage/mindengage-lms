package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // driver: pgx
	_ "modernc.org/sqlite"             // driver: sqlite
)

type Driver string

const (
	DriverSQLite   Driver = "sqlite"
	DriverPostgres Driver = "postgres"
)

// Open opens a DB and ensures schema exists.
func Open(ctx context.Context, driver Driver, dsn string) (*sql.DB, error) {
	var drvName string
	switch driver {
	case DriverSQLite:
		drvName = "sqlite" // modernc driver
		if dsn == "" {
			dsn = "file:mindengage.db?cache=shared&mode=rwc&_pragma=busy_timeout(5000)"
		}
	case DriverPostgres:
		drvName = "pgx" // pgx stdlib driver
		if dsn == "" {
			dsn = "postgres://localhost:5432/mindengage?sslmode=disable"
		}
	default:
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}

	db, err := sql.Open(drvName, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	if err := ensureSchema(ctx, db, driver); err != nil {
		return nil, err
	}
	return db, nil
}

func ensureSchema(ctx context.Context, db *sql.DB, driver Driver) error {
	var schema string
	switch driver {
	case DriverSQLite:
		schema = schemaSQLite
	case DriverPostgres:
		schema = schemaPostgres
	}
	_, err := db.ExecContext(ctx, schema)
	return err
}

const schemaSQLite = `
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS exams (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  time_limit_sec INTEGER NOT NULL,
  questions_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  profile TEXT NOT NULL DEFAULT '',
  policy_json TEXT NOT NULL DEFAULT ''  
);

CREATE TABLE IF NOT EXISTS attempts (
  id TEXT PRIMARY KEY,
  exam_id TEXT NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  status TEXT NOT NULL,
  score REAL NOT NULL DEFAULT 0,
  responses_json TEXT NOT NULL,
  started_at INTEGER NOT NULL,
  submitted_at INTEGER,

  module_index INTEGER NOT NULL DEFAULT 0,
  module_started_at BIGINT,
  module_deadline BIGINT,
  overall_deadline BIGINT
);

CREATE TABLE IF NOT EXISTS event_log (
  offset INTEGER PRIMARY KEY AUTOINCREMENT, -- BIGSERIAL in Postgres
  site_id TEXT NOT NULL DEFAULT 'local',     -- or cfg.SiteID later
  typ TEXT NOT NULL,                         -- e.g., AttemptSubmitted
  key TEXT NOT NULL,                         -- natural key: attemptID
  data TEXT NOT NULL,                        -- JSON payload
  created_at INTEGER NOT NULL
);

`

const schemaPostgres = `
CREATE TABLE IF NOT EXISTS exams (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  time_limit_sec INTEGER NOT NULL,
  questions_json TEXT NOT NULL,
  created_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS attempts (
  id TEXT PRIMARY KEY,
  exam_id TEXT NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  status TEXT NOT NULL,
  score DOUBLE PRECISION NOT NULL DEFAULT 0,
  responses_json TEXT NOT NULL,
  started_at BIGINT NOT NULL,
  submitted_at BIGINT
);

CREATE TABLE IF NOT EXISTS event_log (
  offset BIGSERIAL PRIMARY KEY,
  site_id TEXT NOT NULL DEFAULT 'local',
  typ TEXT NOT NULL,
  key TEXT NOT NULL,
  data TEXT NOT NULL,
  created_at BIGINT NOT NULL
);

`
