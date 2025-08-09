package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	api "github.com/mind-engage/mindengage-lms/internal/api/http"
	auth "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"github.com/mind-engage/mindengage-lms/internal/db"
	"github.com/mind-engage/mindengage-lms/internal/exam"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	rbac "github.com/mind-engage/mindengage-lms/internal/rbac"
	// your file is still at this path
)

func main() {
	// DB selection via env
	driver := os.Getenv("DB_DRIVER") // "sqlite" (default) or "postgres"
	if driver == "" {
		driver = "sqlite"
	}
	dsn := os.Getenv("DB_DSN")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var store exam.Store
	switch driver {
	case "sqlite", "postgres":
		dbh, err := db.Open(ctx, db.Driver(driver), dsn)
		if err != nil {
			log.Fatalf("db open failed: %v", err)
		}
		store = exam.NewSQLStore(dbh, driver)
	default:
		log.Fatalf("unsupported DB_DRIVER: %s", driver)
	}

	authSvc := auth.NewAuthService(os.Getenv("AUTH_HMAC_SECRET"))
	if os.Getenv("AUTH_HMAC_SECRET") == "" {
		// dev default; replace in prod
		authSvc = auth.NewAuthService("supersecret-dev-key")
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// public login
	r.Post("/auth/login", auth.LoginHandler(authSvc))

	// protected
	r.Group(func(pr chi.Router) {
		pr.Use(auth.JWTMiddleware(authSvc))
		// teacher
		pr.With(rbac.Require("exam:create")).
			Post("/exams", api.UploadExamHandler(store))

		// student/teacher fetch
		pr.With(rbac.Require("exam:view")).
			Get("/exams/{examID}", api.GetExamHandler(store))

		// student flow
		pr.With(rbac.Require("attempt:create")).
			Post("/attempts", api.CreateAttemptHandler(store))
		pr.With(rbac.Require("attempt:save")).
			Post("/attempts/{attemptID}/responses", api.SaveResponsesHandler(store))
		pr.With(rbac.Require("attempt:submit")).
			Post("/attempts/{attemptID}/submit", api.SubmitAttemptHandler(store))
		pr.With(rbac.RequireAny("attempt:view-own", "attempt:view-all")).
			Get("/attempts/{attemptID}", api.GetAttemptHandler(store))
	})

	log.Printf("listening on :8080 (db=%s)", driver)
	log.Fatal(http.ListenAndServe(":8080", r))
}
