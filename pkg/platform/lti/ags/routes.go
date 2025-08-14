package ags

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	mw "github.com/mind-engage/mindengage-lms/pkg/platform/lti/middleware"
)

func Routes(s *Server) http.Handler {
	r := chi.NewRouter()
	// Line items collection
	r.With(mw.RequireScopes("https://purl.imsglobal.org/spec/lti-ags/scope/lineitem.readonly")).
		Get("/contexts/{contextId}/line_items", s.GetLineItems)
	r.With(mw.RequireScopes("https://purl.imsglobal.org/spec/lti-ags/scope/lineitem")).
		Post("/contexts/{contextId}/line_items", s.PostLineItem)

	// Item
	r.With(mw.RequireScopes("https://purl.imsglobal.org/spec/lti-ags/scope/lineitem.readonly")).
		Get("/line_items/{id}", s.GetLineItem)
	r.With(mw.RequireScopes("https://purl.imsglobal.org/spec/lti-ags/scope/lineitem")).
		Put("/line_items/{id}", s.PutLineItem)
	r.With(mw.RequireScopes("https://purl.imsglobal.org/spec/lti-ags/scope/lineitem")).
		Delete("/line_items/{id}", s.DeleteLineItem)

	// Scores & Results
	r.With(mw.RequireScopes("https://purl.imsglobal.org/spec/lti-ags/scope/score")).
		Post("/line_items/{id}/scores", s.PostScore)
	r.With(mw.RequireScopes("https://purl.imsglobal.org/spec/lti-ags/scope/result.readonly")).
		Get("/line_items/{id}/results", s.GetResults)

	return r
}
