package main

import (
	"bytes"
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	api "github.com/mind-engage/mindengage-lms/internal/api/http"
	"github.com/mind-engage/mindengage-lms/internal/auth/jwks"
	auth "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"github.com/mind-engage/mindengage-lms/internal/config"
	"github.com/mind-engage/mindengage-lms/internal/db"
	"github.com/mind-engage/mindengage-lms/internal/exam"
	"github.com/mind-engage/mindengage-lms/internal/grading"
	"github.com/mind-engage/mindengage-lms/internal/lti"
	rbac "github.com/mind-engage/mindengage-lms/internal/rbac"
	storage "github.com/mind-engage/mindengage-lms/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	_ "github.com/mind-engage/mindengage-lms/internal/formats/act"
	_ "github.com/mind-engage/mindengage-lms/internal/formats/jee"
	_ "github.com/mind-engage/mindengage-lms/internal/formats/sat"
	_ "github.com/mind-engage/mindengage-lms/internal/formats/stem"
)

// Embed built static assets (copied here during build)
// Structure:
//
//	cmd/gateway/static/exam
//	cmd/gateway/static/teacher
//	cmd/gateway/static/admin
//	cmd/gateway/static/home
//
//go:embed static/**
var staticFS embed.FS

func main() {
	cfg := config.FromEnv()

	// --- DB ---
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbh, err := db.Open(ctx, db.Driver(cfg.DBDriver), cfg.DBDSN)
	if err != nil {
		log.Fatalf("db open failed: %v", err)
	}
	store := exam.NewSQLStore(dbh, cfg.DBDriver, grading.NewDefaultGrader())

	// --- Auth ---
	secret := getenvOr("AUTH_HMAC_SECRET", "supersecret-dev-key")
	authSvc := auth.NewAuthService(secret)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(securityHeaders())

	// --- CORS ---
	if cfg.Mode == config.ModeOnline {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"https://your-frontend.example.com", "https://lms.mindengage.ai"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			ExposedHeaders:   []string{"Content-Length"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	} else {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:3010", "http://localhost:3020"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			ExposedHeaders:   []string{"Content-Length"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	// --- Health ---
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	// --- JWKS ---
	if cfg.EnableJWKS {
		r.Get("/.well-known/jwks.json", jwks.Handler(jwks.JWKS{Keys: []jwks.JWK{}}))
	}

	// --- LTI ---
	if cfg.EnableLTI && cfg.Mode == config.ModeOnline {
		r.Route("/lti", func(lr chi.Router) {
			lr.Get("/login", lti.OIDCLoginHandler("https://platform.example.com/oidc/auth"))
			lr.Post("/launch", lti.LaunchHandler())
		})
	}

	// ======================
	// API under /api prefix
	// ======================
	r.Route("/api", func(apiR chi.Router) {
		if cfg.EnableLocalAuth {
			apiR.Post("/auth/login", auth.LoginHandler(authSvc, cfg))
		}

		bs, err := storage.NewFSStore(cfg.BlobBasePath)
		if err != nil {
			log.Fatalf("blob store: %v", err)
		}
		apiR.Group(func(pr chi.Router) {
			pr.Use(auth.JWTMiddleware(authSvc))
			pr.Route("/assets", func(ar chi.Router) {
				api.MountAssets(ar, bs)
			})
		})

		apiR.Group(func(pr chi.Router) {
			pr.Use(auth.JWTMiddleware(authSvc))
			pr.With(rbac.Require("exam:create")).
				Post("/exams", api.UploadExamHandler(store))
			pr.With(rbac.Require("exam:view")).
				Get("/exams/{examID}", api.GetExamHandler(store))
			pr.With(rbac.Require("attempt:create")).
				Post("/attempts", api.CreateAttemptHandler(store))
			pr.With(rbac.Require("attempt:save")).
				Post("/attempts/{attemptID}/responses", api.SaveResponsesHandler(store))
			pr.With(rbac.Require("attempt:submit")).
				Post("/attempts/{attemptID}/submit", api.SubmitAttemptHandler(store))
			pr.With(rbac.RequireAny("attempt:view-own", "attempt:view-all")).
				Get("/attempts/{attemptID}", api.GetAttemptHandler(store))
			pr.With(rbac.Require("users:bulk_upsert")).
				Post("/users/bulk", api.BulkUpsertUsersHandler(dbh))
			pr.With(rbac.Require("users:list")).
				Get("/users", api.ListUsersHandler(dbh))
			pr.With(rbac.Require("user:change_password")).
				Post("/users/change-password", api.ChangePasswordHandler(dbh))
			pr.With(rbac.Require("exam:create")).
				Post("/qti/import", api.ImportQTIHandler(store, bs))
			pr.With(rbac.Require("exam:export")).
				Get("/exams/{id}/export", api.ExportQTIHandler(store))

			pr.With(rbac.Require("exam:view")).
				Get("/exams", api.ListExamsHandler(store))

			pr.With(rbac.Require("attempt:save")).
				Post("/attempts/{attemptID}/next-module", api.NextModuleHandler(store))
		})
	})

	// =====================================
	// Static SPAs from embedded static dir
	// =====================================
	mountStatic(r, "/", "static/home")
	mountSPA(r, "/exam/", "static/exam")
	mountSPA(r, "/teacher/", "static/teacher")
	mountSPA(r, "/admin/", "static/admin")

	log.Printf("listening on %s (mode=%s, db=%s)", cfg.HTTPAddr, cfg.Mode, cfg.DBDriver)
	log.Fatal(http.ListenAndServe(cfg.HTTPAddr, r))
}

func getenvOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func securityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			next.ServeHTTP(w, r)
		})
	}
}

func mountStatic(r chi.Router, prefix, dir string) {
	sub, _ := fs.Sub(staticFS, dir)
	r.Get(prefix, func(w http.ResponseWriter, req *http.Request) {
		data, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.NotFound(w, req)
			return
		}
		http.ServeContent(w, req, "index.html", time.Time{}, bytes.NewReader(data))
	})
	r.Handle(prefix+"*", http.StripPrefix(prefix, http.FileServer(http.FS(sub))))
}

func mountSPA(r chi.Router, prefix, dir string) {
	// Ensure canonical prefix ends with a slash
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	// Redirect no-slash -> slash (e.g., /exam -> /exam/)
	noslash := strings.TrimSuffix(prefix, "/")
	r.Get(noslash, func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, prefix, http.StatusPermanentRedirect)
	})

	sub, _ := fs.Sub(staticFS, dir)
	fileServer := http.FileServer(http.FS(sub))

	// Serve index at the section root (/exam/)
	r.HandleFunc(prefix, func(w http.ResponseWriter, req *http.Request) {
		serveIndex(sub, w, req)
	})

	// Serve files under /exam/* with SPA fallback
	r.Handle(prefix+"*", http.StripPrefix(prefix, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, prefix)
		// Long cache for hashed assets
		if strings.HasPrefix(path, "assets/") || hasStaticExt(path) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		// Try static file first
		if f, err := sub.Open(path); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, req)
			return
		}
		// SPA fallback for "virtual" routes (no extension)
		if !strings.Contains(path, ".") {
			serveIndex(sub, w, req)
			return
		}
		http.NotFound(w, req)
	})))
}
func serveIndex(sub fs.FS, w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(data))
}

func hasStaticExt(path string) bool {
	return strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".css") ||
		strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".jpg") ||
		strings.HasSuffix(path, ".jpeg") ||
		strings.HasSuffix(path, ".gif") ||
		strings.HasSuffix(path, ".svg") ||
		strings.HasSuffix(path, ".ico") ||
		strings.HasSuffix(path, ".woff") ||
		strings.HasSuffix(path, ".woff2") ||
		strings.HasSuffix(path, ".ttf")
}
