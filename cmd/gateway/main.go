package main

import (
	"log"
	"net/http"
	"time"

	api "github.com/mind-engage/mindengage-lms/internal/api/http"
	auth "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"github.com/mind-engage/mindengage-lms/internal/exam"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	store := exam.NewInMemoryStore()
	authSvc := auth.NewAuthService("supersecret-dev-key") // change in env for real

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// public
	r.Post("/auth/login", auth.LoginHandler(authSvc))

	// protected
	r.Group(func(pr chi.Router) {
		pr.Use(auth.JWTMiddleware(authSvc))

		// teacher endpoints
		pr.Post("/exams", api.UploadExamHandler(store))
		pr.Get("/exams/{examID}", api.GetExamHandler(store)) // both can fetch

		// student endpoints
		pr.Post("/attempts", api.CreateAttemptHandler(store))
		pr.Post("/attempts/{attemptID}/responses", api.SaveResponsesHandler(store))
		pr.Post("/attempts/{attemptID}/submit", api.SubmitAttemptHandler(store))
		pr.Get("/attempts/{attemptID}", api.GetAttemptHandler(store))
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
