package http

import (
	"encoding/json"
	"net/http"

	"github.com/mind-engage/mindengage-lms/internal/exam"

	"github.com/go-chi/chi/v5"
)

func UploadExamHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var e exam.Exam
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if e.ID == "" {
			http.Error(w, "id required", 400)
			return
		}
		if err := store.PutExam(e); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "id": e.ID})
	}
}

func GetExamHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "examID")
		e, err := store.GetExam(id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		_ = json.NewEncoder(w).Encode(e)
	}
}
