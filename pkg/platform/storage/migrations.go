// pkg/platform/storage/migrations.go
package storage

import (
	"context"
	"fmt"
	"strings"
)

// Up applies (idempotent) DDL for the MindEngage LTI Platform.
// It creates the core multi-tenant tables needed for:
//   - issuer/keys (tenants, tenant_keys)
//   - tool registry & deployments (tools, deployments)
//   - LMS course/roster (contexts, enrollments)
//   - AGS data (platform_line_items, platform_results)
//   - replay protection & auditing (replay_state, audit)
//
// Call this once on startup (after Connect). Drivers supported: postgres|sqlite.
func Up(ctx context.Context, db *DB, driver string) error {
	if db == nil || db.SQL == nil {
		return fmt.Errorf("migrations: db is nil")
	}

	var schema string
	switch normalizeDriver(driver) {
	case "postgres":
		schema = schemaPostgres
	case "sqlite":
		schema = schemaSQLite
	default:
		return fmt.Errorf("migrations: unsupported driver %q (expected postgres|sqlite)", driver)
	}

	// Try to run as a single script; if the driver rejects multiple statements,
	// fall back to splitting on semicolons (sufficient for simple DDL).
	if _, err := db.SQL.ExecContext(ctx, schema); err != nil {
		for _, stmt := range splitSQL(schema) {
			trim := strings.TrimSpace(stmt)
			if trim == "" || trim == ";" {
				continue
			}
			if _, e := db.SQL.ExecContext(ctx, stmt); e != nil {
				return fmt.Errorf("migrations: failed at:\n%s\nerr: %w", firstLine(stmt), e)
			}
		}
	}
	return nil
}

/* ----------------------------- POSTGRES SCHEMA ----------------------------- */

const schemaPostgres = `
-- Tenants / Issuers ----------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenants (
  id                 TEXT PRIMARY KEY,
  issuer             TEXT NOT NULL UNIQUE,            -- https://{tenant}.lti.mindengage.com
  active_kid         TEXT,                            -- currently active signing key id
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tenant_keys (
  tenant_id          TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  kid                TEXT NOT NULL,
  public_jwk         JSONB NOT NULL,                  -- public part served in JWKS
  private_jwk_enc    TEXT NOT NULL,                   -- encrypted private JWK (BYOK/KMS)
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  rotates_at         TIMESTAMPTZ,
  PRIMARY KEY (tenant_id, kid)
);

-- Tool registry & deployments ------------------------------------------------
CREATE TABLE IF NOT EXISTS tools (
  client_id          TEXT PRIMARY KEY,
  tenant_id          TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name               TEXT NOT NULL,
  jwks_url           TEXT NOT NULL,
  redirect_uris      JSONB NOT NULL,                  -- array of strings
  allowed_scopes     JSONB NOT NULL,                  -- array of strings
  auth_methods       JSONB NOT NULL,                  -- e.g., ["private_key_jwt","client_secret_post"]
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS deployments (
  id                 TEXT PRIMARY KEY,                -- deployment_id
  tenant_id          TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  client_id          TEXT NOT NULL REFERENCES tools(client_id) ON DELETE CASCADE,
  context_id         TEXT NOT NULL,                   -- links an external tool instance to a course
  title              TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deployments_tenant_client_ctx_idx
  ON deployments (tenant_id, client_id, context_id);

-- LMS contexts & enrollments for NRPS ---------------------------------------
CREATE TABLE IF NOT EXISTS contexts (
  tenant_id          TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id                 TEXT NOT NULL,                   -- context id within tenant
  label              TEXT,
  title              TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, id)
);

CREATE TABLE IF NOT EXISTS enrollments (
  tenant_id          TEXT NOT NULL,
  context_id         TEXT NOT NULL,
  user_sub           TEXT NOT NULL,                   -- platform user ID (sub)
  role               TEXT NOT NULL,                   -- LTI/IMS role URI or mapped role
  name               TEXT,
  email              TEXT,
  status             TEXT,                            -- Active|Inactive|...
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, context_id, user_sub, role),
  FOREIGN KEY (tenant_id, context_id)
    REFERENCES contexts(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS enrollments_role_idx
  ON enrollments (tenant_id, context_id, role);

-- AGS line items & results ---------------------------------------------------
-- Line item "id" in AGS is a URL; we store it as TEXT and use it as the PK.
CREATE TABLE IF NOT EXISTS platform_line_items (
  id                 TEXT PRIMARY KEY,                -- absolute URL this platform returns
  tenant_id          TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  context_id         TEXT NOT NULL,
  resource_link_id   TEXT NOT NULL,
  resource_id        TEXT,                            -- tool-defined identifier grouping
  label              TEXT NOT NULL,
  score_max          NUMERIC NOT NULL,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, context_id)
    REFERENCES contexts(tenant_id, id) ON DELETE CASCADE
);

-- Ensure idempotency: one line item per (tenant,context,resource_link,resource_id)
CREATE UNIQUE INDEX IF NOT EXISTS pli_dedupe_idx
  ON platform_line_items (tenant_id, context_id, resource_link_id, COALESCE(resource_id,''));

CREATE TABLE IF NOT EXISTS platform_results (
  tenant_id          TEXT NOT NULL,
  line_item_id       TEXT NOT NULL REFERENCES platform_line_items(id) ON DELETE CASCADE,
  user_sub           TEXT NOT NULL,
  result_score       NUMERIC,
  result_maximum     NUMERIC,
  timestamp          TIMESTAMPTZ,
  comment            TEXT,
  PRIMARY KEY (tenant_id, line_item_id, user_sub)
);

-- Replay protection (state/nonce/jti) ---------------------------------------
CREATE TABLE IF NOT EXISTS replay_state (
  tenant_id          TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  kind               TEXT NOT NULL CHECK (kind IN ('state','nonce','jti')),
  value              TEXT NOT NULL,
  expires_at         TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant_id, kind, value)
);

-- Audit log ------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit (
  id                 BIGSERIAL PRIMARY KEY,
  ts                 TIMESTAMPTZ NOT NULL DEFAULT now(),
  tenant_id          TEXT,
  client_id          TEXT,
  action             TEXT NOT NULL,                   -- e.g., "token.granted", "ags.post_score"
  subject            TEXT NOT NULL,                   -- e.g., "{line_item_id}", "{user_sub}"
  details            JSONB                             -- arbitrary metadata
);

CREATE INDEX IF NOT EXISTS audit_tenant_ts_idx
  ON audit (tenant_id, ts DESC);
`

/* ------------------------------ SQLITE SCHEMA ------------------------------ */

const schemaSQLite = `
PRAGMA foreign_keys = ON;

-- Tenants / Issuers ----------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenants (
  id                 TEXT PRIMARY KEY,
  issuer             TEXT NOT NULL UNIQUE,
  active_kid         TEXT,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tenant_keys (
  tenant_id          TEXT NOT NULL,
  kid                TEXT NOT NULL,
  public_jwk         TEXT NOT NULL,                   -- JSON (public)
  private_jwk_enc    TEXT NOT NULL,                   -- encrypted private
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  rotates_at         DATETIME,
  PRIMARY KEY (tenant_id, kid),
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CHECK (json_valid(public_jwk))
);

-- Tool registry & deployments ------------------------------------------------
CREATE TABLE IF NOT EXISTS tools (
  client_id          TEXT PRIMARY KEY,
  tenant_id          TEXT NOT NULL,
  name               TEXT NOT NULL,
  jwks_url           TEXT NOT NULL,
  redirect_uris      TEXT NOT NULL,                   -- JSON array
  allowed_scopes     TEXT NOT NULL,                   -- JSON array
  auth_methods       TEXT NOT NULL,                   -- JSON array
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  CHECK (json_valid(redirect_uris)),
  CHECK (json_valid(allowed_scopes)),
  CHECK (json_valid(auth_methods))
);

CREATE TABLE IF NOT EXISTS deployments (
  id                 TEXT PRIMARY KEY,
  tenant_id          TEXT NOT NULL,
  client_id          TEXT NOT NULL,
  context_id         TEXT NOT NULL,
  title              TEXT,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
  FOREIGN KEY (client_id) REFERENCES tools(client_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS deployments_tenant_client_ctx_idx
  ON deployments (tenant_id, client_id, context_id);

-- LMS contexts & enrollments for NRPS ---------------------------------------
CREATE TABLE IF NOT EXISTS contexts (
  tenant_id          TEXT NOT NULL,
  id                 TEXT NOT NULL,
  label              TEXT,
  title              TEXT,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, id),
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS enrollments (
  tenant_id          TEXT NOT NULL,
  context_id         TEXT NOT NULL,
  user_sub           TEXT NOT NULL,
  role               TEXT NOT NULL,
  name               TEXT,
  email              TEXT,
  status             TEXT,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, context_id, user_sub, role),
  FOREIGN KEY (tenant_id, context_id)
    REFERENCES contexts(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS enrollments_role_idx
  ON enrollments (tenant_id, context_id, role);

-- AGS line items & results ---------------------------------------------------
CREATE TABLE IF NOT EXISTS platform_line_items (
  id                 TEXT PRIMARY KEY,                -- absolute URL this platform returns
  tenant_id          TEXT NOT NULL,
  context_id         TEXT NOT NULL,
  resource_link_id   TEXT NOT NULL,
  resource_id        TEXT,
  label              TEXT NOT NULL,
  score_max          REAL NOT NULL,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (tenant_id, context_id)
    REFERENCES contexts(tenant_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS pli_dedupe_idx
  ON platform_line_items (tenant_id, context_id, resource_link_id, IFNULL(resource_id,''));

CREATE TABLE IF NOT EXISTS platform_results (
  tenant_id          TEXT NOT NULL,
  line_item_id       TEXT NOT NULL,
  user_sub           TEXT NOT NULL,
  result_score       REAL,
  result_maximum     REAL,
  timestamp          DATETIME,
  comment            TEXT,
  PRIMARY KEY (tenant_id, line_item_id, user_sub),
  FOREIGN KEY (line_item_id) REFERENCES platform_line_items(id) ON DELETE CASCADE
);

-- Replay protection (state/nonce/jti) ---------------------------------------
CREATE TABLE IF NOT EXISTS replay_state (
  tenant_id          TEXT NOT NULL,
  kind               TEXT NOT NULL CHECK (kind IN ('state','nonce','jti')),
  value              TEXT NOT NULL,
  expires_at         DATETIME NOT NULL,
  PRIMARY KEY (tenant_id, kind, value),
  FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- Audit log ------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  ts                 DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  tenant_id          TEXT,
  client_id          TEXT,
  action             TEXT NOT NULL,
  subject            TEXT NOT NULL,
  details            TEXT,                            -- JSON
  CHECK (details IS NULL OR json_valid(details))
);

CREATE INDEX IF NOT EXISTS audit_tenant_ts_idx
  ON audit (tenant_id, ts DESC);
`

/* ------------------------------ LOCAL HELPERS ------------------------------ */

// splitSQL naively splits on ';' boundaries so we can run one statement at a time.
// This is acceptable for our simple DDL (no functions/procedures).
func splitSQL(s string) []string {
	raw := strings.Split(s, ";")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part+";")
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
