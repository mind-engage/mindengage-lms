package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/mind-engage/mindengage-lms/services/assessment-service/internal/handlers"
	"github.com/mind-engage/mindengage-lms/services/assessment-service/pkg/qti"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	engine := qti.NewEngine()
	h := handlers.NewHandler(engine)

	r := chi.NewRouter()

	// LTI launch endpoint
	r.Post("/lti/launch", h.LaunchLTI)

	// QTI item management
	r.Route("/qti/items", func(r chi.Router) {
		r.Get("/", h.ListItems)
		r.Post("/", h.CreateItem)
		r.Route("/{itemID}", func(r chi.Router) {
			r.Get("/", h.GetItem)
			r.Post("/submit", h.SubmitItem)
		})
	})

	log.Printf("Assessment service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
