# MindEngage Admin UI

A lightweight administration console for the MindEngage LMS backend. It uses React + MUI, mirrors the Student/Teacher look & feel, and focuses on platform/operations tasks (users, exams, attempts, integrations, system status).

> **Status:** MVP UI scaffold. Some actions are read‑only or disabled until corresponding backend endpoints exist.

---

## Features (MVP)

* **Auth:** Local JWT login (see *Auth & Roles*).
* **System:** Probe `/healthz`, `/readyz`, basic CORS reachability to `/exams`.
* **Users:** List (with role filter), bulk CSV/JSON upsert to `POST /users/bulk`.
* **Exams:** Search/list `GET /exams`, preview `GET /exams/{id}`, import QTI `POST /qti/import`, export QTI `GET /exams/{id}/export`.
* **Attempts:** Open by ID `GET /attempts/{id}`, view raw responses/score.
* **Integrations:** Probe JWKS (`/.well-known/jwks.json`) and LTI login (`/lti/login`) availability.

Planned next: lock/archive/delete exams, reset/force-submit/invalidate attempts, LTI platform registry & JWKS key management.

---

## Prerequisites

* **Node.js** ≥ 18
* **Backend (gateway)** running (defaults to `http://localhost:8080`)
* CORS must allow your Admin UI origin (see *CORS* below)

---

## Quick start

```bash
# from repo root
cd web/admin
npm install

# dev (Vite) on port 3020
npm run dev -- --port 3020
```

Open [http://localhost:3020](http://localhost:3020)

### Configure API base

Create `.env.development` (or `.env`) in `web/admin`:

```
VITE_API_BASE=http://localhost:8080
```

The app reads this via `import.meta.env.VITE_API_BASE`.

### CORS (important)

The gateway currently hard-codes allowed origins:

* **offline mode:** `http://localhost:3000`
* **online mode:** `https://your-frontend.example.com`

If you run Admin on another port (e.g., **3020**), add it in `cmd/gateway/main.go`:

```go
r.Use(cors.Handler(cors.Options{
  AllowedOrigins: []string{
    "http://localhost:3000", // student/teacher
    "http://localhost:3010", // teacher (if used)
    "http://localhost:3020", // admin
  },
  AllowedMethods: []string{"GET","POST","PUT","PATCH","DELETE","OPTIONS"},
  AllowedHeaders: []string{"Authorization","Content-Type"},
  ExposedHeaders: []string{"Content-Length"},
  AllowCredentials: true,
  MaxAge: 300,
}))
```

Rebuild/restart the gateway after editing.

> Alternative: temporarily run Admin on **3000** to match the default offline CORS until you update the server.

---

## Auth & Roles

Local login is **stubbed** for development in `internal/auth/middleware/middleware.go`:

* Accepts `teacher/teacher` with role `"teacher"`
* Accepts `student/student` with role `"student"`
* **Does NOT** accept `admin/admin` by default → you'll get **401**

### Option A — Test Admin UI using a teacher token

Use the Admin UI login form with username/password **`teacher/teacher`** and role **teacher** (the Admin UI will still work for read‑only endpoints).

### Option B — Enable secure admin login from config (recommended)

Implement the small patch to support `ADMIN_USER` + `ADMIN_PASS_HASH` (bcrypt) and pass `cfg` into `LoginHandler`. Summary:

1. Add fields to `Config` (`AdminUser`, `AdminPassHash`) in `internal/config/config.go` and read from env.
2. In `LoginHandler`, if `req.Role == "admin"`, compare `req.Username` and `req.Password` with the configured values using `bcrypt.CompareHashAndPassword`.
3. Wire route: `r.Post("/auth/login", auth.LoginHandler(authSvc, cfg))`.
4. Set env and restart:

```bash
export ADMIN_USER=admin
export ADMIN_PASS_HASH='$2y$12$...bcrypt-hash-here...'
export ENABLE_LOCAL_AUTH=1
```

See the repo notes / PR template for the full diff.

---

## Scripts

```bash
# dev server (choose a port or configure vite.config.ts)
npm run dev -- --port 3020

# production build
npm run build
# preview a production build
npm run preview
```

If you prefer hardcoding the dev port, create `vite.config.ts`:

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
export default defineConfig({
  plugins: [react()],
  server: { port: 3020 }
})
```

---

## Installing UI deps

This app uses **MUI v5**:

```bash
npm install @mui/material @emotion/react @emotion/styled @mui/icons-material
```

If you see `Cannot find module '@mui/icons-material/*'`, install the icons package and restart.

---

## Endpoints used

| Area     | Method & Path                       | Notes                               |
| -------- | ----------------------------------- | ----------------------------------- |
| Health   | `GET /healthz`, `GET /readyz`       | Status probes                       |
| Users    | `GET /users[?role=]`                | List users                          |
| Users    | `POST /users/bulk`                  | CSV/JSON bulk upsert                |
| Exams    | `GET /exams[?q=&limit=&offset=]`    | Search/list exams                   |
| Exams    | `GET /exams/{id}`                   | Preview (student‑safe)              |
| Exams    | `POST /qti/import`                  | Upload QTI (.zip)                   |
| Exams    | `GET /exams/{id}/export?format=qti` | Download QTI zip                    |
| Attempts | `GET /attempts/{id}`                | View attempt (raw responses, score) |
| Auth     | `POST /auth/login`                  | Local JWT (dev)                     |

> Some admin actions are intentionally disabled in the UI until backend endpoints exist (lock/archive/delete exams; reset/force‑submit/invalidate attempts).

---

## Folder layout (suggested)

```
web/
  admin/
    src/
      App.tsx           # entry that renders AdminApp
      AdminApp.tsx      # the main component (from this repo)
    index.html
    package.json
    vite.config.ts
    .env(.development)
```

If you’re embedding into an existing workspace, you can also export `AdminApp` and mount it from your shell or router.

---

## Troubleshooting

* **401 Unauthorized on login**

  * Using `role: "admin"` without the config patch will fail by design. Use `teacher/teacher` or implement Option B above.
* **CORS error** in browser console

  * Add your Admin origin (e.g., `http://localhost:3020`) to `AllowedOrigins` in the gateway; restart the server.
* **Icons not found**

  * Install `@mui/icons-material` and restart the dev server.
* **QTI import fails**

  * Ensure the zip contains a valid `imsmanifest.xml` and item files the minimal parser supports.

---

## Security notes

* Keep local auth (`/auth/login`) **disabled** in production unless guarded behind SSO/VPN and strong secrets.
* Never commit plaintext admin passwords. Use **bcrypt** hash via `ADMIN_PASS_HASH`.
* Restrict Admin UI exposure (separate hostname, network policy, or VPN) and expand RBAC for non‑wildcard admin roles in production.

---

## License

MIT (or your project’s license).
