#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${1:-mindengage-lms}"

# --- helper functions ---
mkd() { mkdir -p "$1"; }
touchkeep() { mkd "$1"; : > "$1/.gitkeep"; }
touchfile() { mkd "$(dirname "$1")"; : > "$1"; }

# --- top-level ---
mkd "$ROOT_DIR"
cd "$ROOT_DIR"

# cmd services
touchfile cmd/gateway/main.go
touchfile cmd/lti-gateway/main.go
touchfile cmd/auth/main.go
touchfile cmd/exam/main.go
touchfile cmd/grading/main.go
touchfile cmd/sync/main.go
touchfile cmd/tools/main.go

# internal/auth
touchfile internal/auth/oidc/oidc.go
touchfile internal/auth/jwks/jwks.go
touchfile internal/auth/middleware/middleware.go

# internal/lti
touchfile internal/lti/oidc_login.go
touchfile internal/lti/launch_handler.go
touchfile internal/lti/ags.go
touchfile internal/lti/nrps.go
touchfile internal/lti/deep_linking.go

# internal/exam
touchfile internal/exam/models.go
touchfile internal/exam/repo.go
touchfile internal/exam/service.go
touchfile internal/exam/validator.go

# internal/grading
touchfile internal/grading/engine.go
touchfile internal/grading/numeric.go
touchfile internal/grading/textmatch.go
touchfile internal/grading/rubric.go
touchfile internal/grading/ocr/tesseract.go

# internal/storage
touchfile internal/storage/blob.go
touchfile internal/storage/minio.go
touchfile internal/storage/gcs.go
touchfile internal/storage/fs.go

# internal/sync
touchfile internal/sync/eventlog.go
touchfile internal/sync/replicator.go
touchfile internal/sync/conflict.go

# internal/api
touchfile internal/api/http/student_handlers.go
touchfile internal/api/http/teacher_handlers.go
touchkeep internal/api/grpc

# internal/db
touchkeep internal/db/migrations
touchfile internal/db/postgres.go
touchfile internal/db/sqlite.go

# misc internal dirs
touchkeep internal/rbac
touchkeep internal/telemetry
touchkeep internal/config

# pkg
touchfile pkg/crypto/crypto.go
touchfile pkg/httpx/httpx.go
touchfile pkg/types/types.go

# api-specs
touchfile api-specs/rest.yaml
touchkeep api-specs/proto

# web, docker, helm
touchkeep web
touchkeep docker
touchkeep helm

# root files
touch Makefile
touch .gitignore
touch README.md

echo "✅ mindengage-lms scaffold created at $(pwd)"
