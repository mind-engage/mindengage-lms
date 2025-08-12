# MindEngage Teacher UI

A React + MUI app for teachers to manage exam content and roster ops, and to quickly preview/run exams. It shares the same look-and-feel as the Student app.

> **Status:** MVP. Some advanced actions depend on backend endpoints/RBAC and may be read‑only in this build.

---

## Features (MVP)

* **Auth:** Local JWT login (dev stub) with role `teacher`.
* **Exams:** Search/list (`GET /exams`), open preview (`GET /exams/{id}`), **import QTI** (`POST /qti/import`), **export QTI** (`GET /exams/{id}/export`).
* **Authoring:** Upload/save exams (`POST /exams`) when you have `exam:create`.
* **Attempts (read-only):** Open by ID (`GET /attempts/{id}`) when you have `attempt:view-all`.
* **Roster (optional):** Bulk CSV/JSON user upsert (`POST /users/bulk`), list users (`GET /users`).

RBAC defaults (see `internal/rbac/rules.go`):

* `teacher`: `exam:create`, `exam:view`, `exam:export`, `attempt:view-all`, `attempt:grade`, `users:bulk_upsert`, `users:list`.
* `student`: `attempt:create|save|submit|view-own`, `exam:view`, `user:change_password`.

---

## Prerequisites

* **Node.js** ≥ 18
* LMS backend running (defaults to `http://localhost:8080`)
* **CORS** must allow your Teacher UI origin (see below)

---

## Quick start

```bash
# from repo root
cd web/teacher
npm install

# dev (Vite) on port 3010
npm run dev -- --port 3010
```

Open [http://localhost:3010](http://localhost:3010)

### Configure API base

Create `.env.development` (or `.env`) in `web/teacher`:

```
VITE_API_BASE=http://localhost:8080
```

The app reads this via `import.meta.env.VITE_API_BASE`.

### CORS (important)

In offline mode, the gateway currently allows only `http://localhost:3000` by default. If Teacher runs on **3010**, update `cmd/gateway/main.go`:

```go
r.Use(cors.Handler(cors.Options{
  AllowedOrigins: []string{
    "http://localhost:3000", // student
    "http://localhost:3010", // teacher
  },
  AllowedMethods:   []string{"GET","POST","PUT","PATCH","DELETE","OPTIONS"},
  AllowedHeaders:   []string{"Authorization","Content-Type"},
  ExposedHeaders:   []string{"Content-Length"},
  AllowCredentials: true,
  MaxAge:           300,
}))
```

Rebuild/restart the gateway after editing.

> In online mode, add your deployed teacher origin to `AllowedOrigins`.

---

## Auth & Login (dev stub)

Local login is stubbed in `internal/auth/middleware/middleware.go`:

```go
valid := (req.Username == req.Password) && (req.Role == "teacher" || req.Role == "student")
```

That means:

* **Works:** `teacher/teacher` (role `teacher`), `student/student` (role `student`)
* **Fails:** `admin/admin` → 401 (unless you patch the login to allow admin)

For production, disable the stub (`ENABLE_LOCAL_AUTH=0`) and/or integrate real auth.

---

## Scripts

```bash
# dev server (choose a port or configure vite.config.ts)
npm run dev -- --port 3010

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
  server: { port: 3010 }
})
```

---

## UI dependencies

This app uses **MUI v5**:

```bash
npm install @mui/material @emotion/react @emotion/styled @mui/icons-material
```

If you see `Cannot find module '@mui/icons-material/*'`, install the icons package and restart the dev server.

---

## Endpoints used

| Area     | Method & Path                       | Notes                                     |
| -------- | ----------------------------------- | ----------------------------------------- |
| Auth     | `POST /auth/login`                  | Local JWT (dev); sends role `teacher`     |
| Exams    | `GET /exams[?q=&limit=&offset=]`    | Search/list                               |
| Exams    | `GET /exams/{id}`                   | Preview (student‑safe)                    |
| Exams    | `POST /exams`                       | Upload/save exam (requires `exam:create`) |
| QTI      | `POST /qti/import`                  | Import a QTI package (.zip)               |
| QTI      | `GET /exams/{id}/export?format=qti` | Export QTI zip (requires `exam:export`)   |
| Attempts | `GET /attempts/{id}`                | Admin/teacher read; own-or-all by RBAC    |
| Users    | `POST /users/bulk`                  | Bulk upsert (teacher role allowed)        |
| Users    | `GET /users[?role=]`                | List users                                |
| Assets   | `POST /assets/{attemptID}`          | Upload scan/assets for an attempt         |

> The Teacher UI also reuses some student-runner UX for exam previewing. Attempt creation/submission is typically a **student** capability.

---

## Folder layout (suggested)

```
web/
  teacher/
    src/
      App.tsx           # entry that renders TeacherApp
      TeacherApp.tsx    # main component (from this repo)
    index.html
    package.json
    vite.config.ts
    .env(.development)
```

---

## Troubleshooting

* **CORS error**: Add `http://localhost:3010` to `AllowedOrigins` in gateway; restart server.
* **401 Unauthorized** on admin login: expected with the dev stub. Use `teacher/teacher` or patch the login for admin.
* **Icons not found**: install `@mui/icons-material` and restart.
* **QTI import errors**: ensure the zip contains `imsmanifest.xml` and supported item files.

---

## Security notes

* Do not expose the dev login in production. Use proper auth (users table + bcrypt/SSO).
* Keep RBAC strict—teacher role should not get `*` permissions.
* Limit who can access Teacher UI (network policy/VPN) for pre‑prod environments.

---

## License

MIT (or your project’s license).
