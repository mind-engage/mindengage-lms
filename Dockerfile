# syntax=docker/dockerfile:1.7-labs
ARG GO_VERSION=1.23.2

############################################
# ---------- UI build (Node) --------------
############################################
FROM node:20-alpine AS ui
WORKDIR /repo

# exam (served at /exam, same-origin /api)
COPY web/exam/package*.json web/exam/
RUN --mount=type=cache,target=/root/.npm cd web/exam && npm ci
COPY web/exam web/exam
RUN --mount=type=cache,target=/root/.npm sh -lc 'cd web/exam && PUBLIC_URL=/exam REACT_APP_API_BASE=/api npm run build || npm run build'

# teacher (served at /teacher)
COPY web/teacher/package*.json web/teacher/
RUN --mount=type=cache,target=/root/.npm cd web/teacher && npm ci
COPY web/teacher web/teacher
RUN --mount=type=cache,target=/root/.npm sh -lc 'cd web/teacher && PUBLIC_URL=/teacher REACT_APP_API_BASE=/api npm run build'

# admin (served at /admin)
COPY web/admin/package*.json web/admin/
RUN --mount=type=cache,target=/root/.npm cd web/admin && npm ci
COPY web/admin web/admin
RUN --mount=type=cache,target=/root/.npm sh -lc 'cd web/admin && PUBLIC_URL=/admin REACT_APP_API_BASE=/api npm run build'

# quiz (served at /quiz)
COPY web/quiz/package*.json web/quiz/
RUN --mount=type=cache,target=/root/.npm cd web/quiz && npm ci
COPY web/quiz web/quiz
RUN --mount=type=cache,target=/root/.npm sh -lc 'cd web/quiz && PUBLIC_URL=/quiz REACT_APP_API_BASE=/api npm run build'

# home (served at /)
COPY web/home/package*.json web/home/
RUN --mount=type=cache,target=/root/.npm cd web/home && npm ci
COPY web/home web/home
RUN --mount=type=cache,target=/root/.npm sh -lc 'cd web/home && PUBLIC_URL=/ npm run build'

############################################
# ---------- Go build ---------------------
############################################
FROM golang:${GO_VERSION}-alpine AS gobuild
ENV GOTOOLCHAIN=auto
ENV CGO_ENABLED=0
WORKDIR /repo

# Go deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Source
COPY . .

# Place built UIs where go:embed expects them (no brace expansion)
RUN rm -rf cmd/gateway/static && \
    mkdir -p cmd/gateway/static/exam && \
    mkdir -p cmd/gateway/static/teacher && \
    mkdir -p cmd/gateway/static/admin && \
    mkdir -p cmd/gateway/static/quiz && \
    mkdir -p cmd/gateway/static/home

COPY --from=ui /repo/web/exam/build/    cmd/gateway/static/exam/
COPY --from=ui /repo/web/teacher/build/ cmd/gateway/static/teacher/
COPY --from=ui /repo/web/admin/build/   cmd/gateway/static/admin/
COPY --from=ui /repo/web/quiz/build/   cmd/gateway/static/quiz/
COPY --from=ui /repo/web/home/build/    cmd/gateway/static/home/

# Safety: ensure each dir has at least one file so //go:embed "static/**" never errors
RUN for d in exam teacher admin home; do \
      if [ -z "$(find cmd/gateway/static/$d -type f 2>/dev/null)" ]; then \
        printf '<!doctype html><meta charset="utf-8"><title>MindEngage %s</title>' "$d" > cmd/gateway/static/$d/index.html; \
      fi; \
    done

# Build binary
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /bin/gateway ./cmd/gateway

############################################
# ---------- Final image ------------------
############################################
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=gobuild /bin/gateway /app/gateway
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/gateway"]
