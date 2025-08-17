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
	// No migrations: fresh DB assumed.
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

-- ===========================
-- Core users/exams/attempts
-- ===========================

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL CHECK (role IN ('student','teacher','admin')),
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

CREATE TABLE IF NOT EXISTS exams (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  time_limit_sec INTEGER NOT NULL,
  questions_json TEXT NOT NULL,
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
  profile TEXT NOT NULL DEFAULT '',
  policy_json TEXT NOT NULL DEFAULT ''  
);

-- ===========================
-- Courses / enrollment / LOBs
-- ===========================

CREATE TABLE IF NOT EXISTS courses (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_by TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE TABLE IF NOT EXISTS course_teachers (
  course_id  TEXT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  teacher_id TEXT NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
  role       TEXT NOT NULL DEFAULT 'co' CHECK (role IN ('owner','co')),
  PRIMARY KEY (course_id, teacher_id)
);
CREATE INDEX IF NOT EXISTS idx_teachers_course ON course_teachers(course_id, teacher_id);

CREATE TABLE IF NOT EXISTS course_students (
  course_id  TEXT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  student_id TEXT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
  status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','invited','dropped')),
  PRIMARY KEY (course_id, student_id)
);
CREATE INDEX IF NOT EXISTS idx_students_course ON course_students(course_id, student_id);

CREATE TABLE IF NOT EXISTS exam_offerings (
  id             TEXT PRIMARY KEY,
  exam_id        TEXT NOT NULL REFERENCES exams(id)    ON DELETE CASCADE,
  course_id      TEXT NOT NULL REFERENCES courses(id)  ON DELETE CASCADE,
  assigned_by    TEXT NOT NULL REFERENCES users(id),
  start_at       INTEGER,
  end_at         INTEGER,
  time_limit_sec INTEGER,
  max_attempts   INTEGER NOT NULL DEFAULT 1,
  visibility     TEXT NOT NULL DEFAULT 'course' CHECK (visibility IN ('course','public','link')),
  access_token   TEXT UNIQUE
);
CREATE INDEX IF NOT EXISTS idx_offerings_course ON exam_offerings(course_id);

-- Optional: ownership and invitations (future-friendly)
CREATE TABLE IF NOT EXISTS exam_owners (
  exam_id    TEXT NOT NULL REFERENCES exams(id)   ON DELETE CASCADE,
  teacher_id TEXT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
  PRIMARY KEY (exam_id, teacher_id)
);

CREATE TABLE IF NOT EXISTS teacher_invites (
  email TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
  expires_at INTEGER NOT NULL
);

-- ===========================
-- Attempts & event log
-- ===========================

CREATE TABLE IF NOT EXISTS attempts (
  id TEXT PRIMARY KEY,
  exam_id TEXT NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  status TEXT NOT NULL,
  score REAL NOT NULL DEFAULT 0,
  responses_json TEXT NOT NULL,
  started_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
  submitted_at INTEGER NOT NULL DEFAULT 0,
  module_index INTEGER NOT NULL DEFAULT 0,
  module_started_at BIGINT,
  module_deadline BIGINT,
  overall_deadline BIGINT,
  current_index INTEGER NOT NULL DEFAULT 0,
  max_reached_index INTEGER NOT NULL DEFAULT 0,
  offering_id TEXT REFERENCES exam_offerings(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS event_log (
  offset INTEGER PRIMARY KEY AUTOINCREMENT,
  site_id TEXT NOT NULL DEFAULT 'local',
  typ TEXT NOT NULL,
  key TEXT NOT NULL,
  data TEXT NOT NULL,
  created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);
`

const schemaPostgres = `
-- ===========================
-- Core users/exams/attempts
-- ===========================

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL CHECK (role IN ('student','teacher','admin')),
  created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW())::BIGINT)
);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

CREATE TABLE IF NOT EXISTS exams (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  time_limit_sec INTEGER NOT NULL,
  questions_json TEXT NOT NULL,
  created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW())::BIGINT),
  profile TEXT NOT NULL DEFAULT '',
  policy_json TEXT NOT NULL DEFAULT ''
);

-- ===========================
-- Courses / enrollment / LOBs
-- ===========================

CREATE TABLE IF NOT EXISTS courses (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_by TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW())::BIGINT)
);

CREATE TABLE IF NOT EXISTS course_teachers (
  course_id  TEXT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  teacher_id TEXT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
  role       TEXT NOT NULL DEFAULT 'co' CHECK (role IN ('owner','co')),
  PRIMARY KEY (course_id, teacher_id)
);
CREATE INDEX IF NOT EXISTS idx_teachers_course ON course_teachers(course_id, teacher_id);

CREATE TABLE IF NOT EXISTS course_students (
  course_id  TEXT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  student_id TEXT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
  status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','invited','dropped')),
  PRIMARY KEY (course_id, student_id)
);
CREATE INDEX IF NOT EXISTS idx_students_course ON course_students(course_id, student_id);

CREATE TABLE IF NOT EXISTS exam_offerings (
  id             TEXT PRIMARY KEY,
  exam_id        TEXT NOT NULL REFERENCES exams(id)   ON DELETE CASCADE,
  course_id      TEXT NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  assigned_by    TEXT NOT NULL REFERENCES users(id),
  start_at       BIGINT,
  end_at         BIGINT,
  time_limit_sec INTEGER,
  max_attempts   INTEGER NOT NULL DEFAULT 1,
  visibility     TEXT NOT NULL DEFAULT 'course' CHECK (visibility IN ('course','public','link')),
  access_token   TEXT UNIQUE
);
CREATE INDEX IF NOT EXISTS idx_offerings_course ON exam_offerings(course_id);

-- Optional: ownership and invitations (future-friendly)
CREATE TABLE IF NOT EXISTS exam_owners (
  exam_id    TEXT NOT NULL REFERENCES exams(id)   ON DELETE CASCADE,
  teacher_id TEXT NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
  PRIMARY KEY (exam_id, teacher_id)
);

CREATE TABLE IF NOT EXISTS teacher_invites (
  email TEXT PRIMARY KEY,
  created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW())::BIGINT),
  expires_at BIGINT NOT NULL
);

-- ===========================
-- Attempts & event log
-- ===========================

CREATE TABLE IF NOT EXISTS attempts (
  id TEXT PRIMARY KEY,
  exam_id TEXT NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  status TEXT NOT NULL,
  score DOUBLE PRECISION NOT NULL DEFAULT 0,
  responses_json TEXT NOT NULL,
  started_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW())::BIGINT),
  submitted_at BIGINT NOT NULL DEFAULT 0,
  module_index INTEGER NOT NULL DEFAULT 0,
  module_started_at BIGINT,
  module_deadline BIGINT,
  overall_deadline BIGINT,
  current_index INTEGER NOT NULL DEFAULT 0,
  max_reached_index INTEGER NOT NULL DEFAULT 0,  
  offering_id TEXT REFERENCES exam_offerings(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS event_log (
  offset BIGSERIAL PRIMARY KEY,
  site_id TEXT NOT NULL DEFAULT 'local',
  typ TEXT NOT NULL,
  key TEXT NOT NULL,
  data TEXT NOT NULL,
  created_at BIGINT NOT NULL DEFAULT (EXTRACT(EPOCH FROM NOW())::BIGINT)
);
`
