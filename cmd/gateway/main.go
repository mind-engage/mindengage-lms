package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	api "github.com/mind-engage/mindengage-lms/internal/api/http"
	"github.com/mind-engage/mindengage-lms/internal/auth/jwks"
	auth "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"github.com/mind-engage/mindengage-lms/internal/config"
	"github.com/mind-engage/mindengage-lms/internal/db"
	"github.com/mind-engage/mindengage-lms/internal/exam"
	"github.com/mind-engage/mindengage-lms/internal/lti"
	rbac "github.com/mind-engage/mindengage-lms/internal/rbac"
	storage "github.com/mind-engage/mindengage-lms/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func main() {
	cfg := config.FromEnv()

	// --- DB ---
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbh, err := db.Open(ctx, db.Driver(cfg.DBDriver), cfg.DBDSN)
	if err != nil {
		log.Fatalf("db open failed: %v", err)
	}
	store := exam.NewSQLStore(dbh, cfg.DBDriver)

	// --- Auth (local JWT for offline/dev) ---
	secret := getenvOr("AUTH_HMAC_SECRET", "supersecret-dev-key")
	authSvc := auth.NewAuthService(secret)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	if cfg.Mode == config.ModeOnline {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"https://your-frontend.example.com"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			ExposedHeaders:   []string{"Content-Length"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	} else {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"http://localhost:3000"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			ExposedHeaders:   []string{"Content-Length"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	// Local login (enabled in offline mode by default; can be enabled online via env)
	if cfg.EnableLocalAuth {
		r.Post("/auth/login", auth.LoginHandler(authSvc))
	}

	bs, err := storage.NewFSStore(cfg.BlobBasePath)
	if err != nil {
		log.Fatalf("blob store: %v", err)
	}
	// assets routes (protected)
	r.Group(func(pr chi.Router) {
		pr.Use(auth.JWTMiddleware(authSvc))
		pr.Route("/assets", func(ar chi.Router) {
			api.MountAssets(ar, bs)
		})
	})

	// Protected API (JWT → role in context → RBAC)
	r.Group(func(pr chi.Router) {
		pr.Use(auth.JWTMiddleware(authSvc))

		// Teacher-only: upload exam
		pr.With(rbac.Require("exam:create")).
			Post("/exams", api.UploadExamHandler(store))

		// Student/Teacher: fetch exam
		pr.With(rbac.Require("exam:view")).
			Get("/exams/{examID}", api.GetExamHandler(store))

		// Student flow
		pr.With(rbac.Require("attempt:create")).
			Post("/attempts", api.CreateAttemptHandler(store))
		pr.With(rbac.Require("attempt:save")).
			Post("/attempts/{attemptID}/responses", api.SaveResponsesHandler(store))
		pr.With(rbac.Require("attempt:submit")).
			Post("/attempts/{attemptID}/submit", api.SubmitAttemptHandler(store))
		// Basic guard: own-or-all can be tightened later with an owner check helper
		pr.With(rbac.RequireAny("attempt:view-own", "attempt:view-all")).
			Get("/attempts/{attemptID}", api.GetAttemptHandler(store))

			// Users (teacher/admin)
		pr.With(rbac.Require("users:bulk_upsert")).
			Post("/users/bulk", api.BulkUpsertUsersHandler(dbh)) // pass *sql.DB

		pr.With(rbac.Require("users:list")).
			Get("/users", api.ListUsersHandler(dbh))

		pr.With(rbac.Require("user:change_password")).
			Post("/users/change-password", api.ChangePasswordHandler(dbh))

		pr.With(rbac.Require("exam:create")).
			Post("/qti/import", api.ImportQTIHandler(store, bs))
		pr.With(rbac.Require("exam:export")).
			Get("/exams/{id}/export", api.ExportQTIHandler(store))
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })

	// --- Online-only surfaces (feature-flagged) ---

	// JWKS (for LTI deep-linking responses, client_assertion, etc.)
	if cfg.EnableJWKS {
		// Dev: serve empty JWKS for now; replace with real keys when ready
		r.Get("/.well-known/jwks.json", jwks.Handler(jwks.JWKS{Keys: []jwks.JWK{}}))
	}

	// LTI OIDC login + launch (stubs). Only mount when explicitly enabled.
	if cfg.EnableLTI && cfg.Mode == config.ModeOnline {
		r.Route("/lti", func(lr chi.Router) {
			// TODO: read platform auth URL from a registry (JSON/DB)
			lr.Get("/login", lti.OIDCLoginHandler("https://platform.example.com/oidc/auth"))
			lr.Post("/launch", lti.LaunchHandler())
		})
	}

	log.Printf("listening on %s (mode=%s, db=%s)", cfg.HTTPAddr, cfg.Mode, cfg.DBDriver)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, r))
}

func getenvOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
