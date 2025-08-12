# MindEngage LMS

An AI-first Learning Management System by **MindEngage**, designed for both **local LAN-only** deployments and **internet-connected** environments.  
Supports AI-driven content generation, offline-first sync, and industry standards like LTI, QTI, and xAPI.

---

## Table of Contents
- [Overview](#overview)
- [Features](#features)
- [Dual-Mode Operation](#dual-mode-operation)
- [Architecture](#architecture)
- [Development Status](#development-status)
- [Deployment](#deployment)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

MindEngage LMS enables:
- AI-generated courses, quizzes, and exams.
- Seamless **offline-first** operation for LAN deployments.
- Secure **online** integration with external platforms via LTI 1.3 and OIDC.
- Flexible grading with OCR, numeric, and text-matching engines.

---

## Features

- **Dual-Mode Deployment**  
  - **Offline**: Operates entirely in LAN mode with local authentication and storage.  
  - **Online**: Adds OIDC, LTI, and sync to cloud services.
  
- **Assessment & Grading**  
  - QTI-compatible engine.
  - Auto-grading with OCR and rubric-based evaluation.
  
- **User Management**  
  - Teacher/admin can bulk-import students.
  - Role-based access control (RBAC).
  - Optional password reset flows.

- **Content Management**  
  - Markdown/HTML lesson editing.
  - File uploads (local FS, MinIO, S3, GCS).

- **Standards Support**  
  - QTI 3.0, LTI 1.3 Advantage, xAPI, OneRoster.

---

## Dual-Mode Operation

| Mode    | Description | Authentication | Storage | Sync |
|---------|-------------|----------------|---------|------|
| Offline | Runs entirely in a LAN; no internet required. | Local JWT (teacher/student) | Local FS + SQLite/Postgres | Disabled |
| Online  | Full internet-enabled deployment. | OIDC / LTI / JWT | Cloud Storage + Postgres | Enabled |

Switching mode is **runtime configurable** via environment variables (`MODE=offline` or `MODE=online`).

---

## Architecture

A full architecture diagram and sequence flows are documented in:  
📄 [`docs/architecture.md`](docs/architecture.md)

---

## Development Status

Current implemented modules:
- **API Gateway**: Chi-based HTTP router with JWT middleware.
- **RBAC**: Role-based route protection.
- **Exam Service**: Create, view, and submit attempts.
- **Grading Engine**: OCR + numeric + text match.
- **Storage Service**: Local FS uploads.
- **Offline Auth**: Local JWT-based login.
- **Online Hooks**: Stubs for LTI 1.3 and JWKS.

Planned:
- Sync service for online mode.
- OIDC-based authentication.
- Fully integrated LTI AGS/NRPS.

---

## Deployment

### Prerequisites
- Go 1.21+
- SQLite or PostgreSQL
- (Online mode) Access to OIDC/LTI platforms.

### Run (Offline Mode)

Generate admin password and hash it.
```
htpasswd -bnBC 12 admin 'YourStrongPassword' | cut -d: -f2
```

Set env variables using either .env.lan(offline) or .env.online(online) template

```
MODE=offline
ADMIN_USER=admin
ADMIN_PASS_HASH='$2y$12$pyZAiWaTfVtM7UElIRStvOC3gNbnp70nmQU4eYopLGBfCJr1DOvji'

HTTP_ADDR=0.0.0.0:8080

DB_DRIVER=sqlite
DB_DSN=file:mindengage.db?cache=shared&mode=rwc&_pragma=busy_timeout(5000)

BLOB_DRIVER=fs
BLOB_BASE_PATH=./data

ENABLE_LOCAL_AUTH=1
ENABLE_LTI=0
ENABLE_JWKS=0

```

## Running with docker

```
docker run -p 8080:8080 \
  -e DB_DRIVER=sqlite \
  -e DB_DSN='file:/tmp/mindengage.db?cache=shared&mode=rwc' \
  -e BLOB_BASE_PATH=/tmp/assets \
  mindengale-lms
```

```bash

go run ./cmd/gateway
```

## Contributing

    Fork the repo.

    Create a feature branch.

    Write tests for your changes.

    Submit a PR.

License

This project is licensed under the MIT License. See LICENSE for details.