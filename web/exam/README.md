# MindEngage — Student Web (web/exam)

Student-facing React app for taking exams and assignments against the MindEngage gateway API.

⚡ Vite + React + MUI

🔐 JWT dev login (stub)

📝 Autosave responses

⏱ Time limit & time left indicator

📄 Scan upload to attempts

🔎 Uses /exams API to list available exams

This app lives in a monorepo. The Go backend (gateway) runs from cmd/gateway.


## Prerequisites

Node 18+ (or 20+)

npm (bundled with Node)

Go 1.21+ (to run the gateway)

Quick start (dev)
1) Run the gateway API (from repo root)

go run ./cmd/gateway

Defaults:

MODE=offline

SQLite DB

CORS allows http://localhost:3000 (adjust if you use port 5173)

2) Run the web app (from web/exam)

## install deps
npm install

## dev (Vite default port 5173)
npm run dev

## OR bind to port 3000 to match the gateway’s default CORS:
npm run dev -- --port 3000

If your API base isn’t the default, create web/exam/.env.local:

VITE_API_BASE=http://localhost:8080

Build & preview

## Build to dist/
npm run build

## Serve the production build locally
npm run preview

Deploy the contents of dist/ behind any static file server or CDN.
Login (dev)

Gateway issues a dev JWT when username === password and role ∈ {student, teacher}.

    Student: student / student

    Teacher: teacher / teacher

(Admin appears in the UI but dev auth accepts only student/teacher.)
Typical flow

    Teacher uploads an exam
```
TOK=$(curl -s -X POST http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"teacher","password":"teacher","role":"teacher"}' | jq -r .access_token)
```

```
curl -s -X POST http://localhost:8080/exams \
  -H "Authorization: Bearer $TOK" \
  -H 'Content-Type: application/json' \
  --data @exam-101.json
```
    Student signs in in this app.

    Student selects an exam from GET /exams (or enters ID):

        Supports search: GET /exams?q=algebra

        Returns: [{ id, title, time_limit_sec, created_at, profile }]

    Load exam → Start attempt → Answer → Submit

        Autosave runs as you type

        “Time left” chip appears next to the time limit and in the app bar

        On expiry, the app auto-submits

    (Optional) Upload scan of work:
```
curl -s -X POST "http://localhost:8080/assets/$ATTEMPT" \
  -H "Authorization: Bearer $STOK" \
  -F "file=@math-scan.png"
```

Timer notes

    UI shows Time Limit (minutes) and Time Left.

    Countdown starts when an attempt is created, using exam.time_limit_sec.

    If you define only module limits in policy.sections[].modules[], ensure the backend derives/sums an overall time_limit_sec so the UI has a value to show. (Server-side enforcement remains authoritative.)

Environment variables (frontend)

    VITE_API_BASE — Gateway base URL (default http://localhost:8080)

Create .env.local in web/exam/ for local overrides.
PWA / meta

    index.html and public/manifest.webmanifest are tuned for PWA installs.

    Place icons under public/icons/ (include maskable variants).

    Ensure the theme color matches your MUI primary (default #3f51b5).

Troubleshooting

    CORS errors in dev
    Either run npm run dev -- --port 3000 or add http://localhost:5173 to the backend CORS allowlist.

    Time limit shows 0
    Your exam JSON lacks top-level time_limit_sec. Provide it or enable backend logic to sum module limits from policy.

    DB schema errors (e.g., “no column named profile”)
    Start with a fresh DB or run the migration that adds profile, policy_json to exams and timing columns to attempts.

npm scripts

    dev — Vite dev server

    build — Production build (to dist/)

    preview — Preview the production build locally

License

MIT — see LICENSE.