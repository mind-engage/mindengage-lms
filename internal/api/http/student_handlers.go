package http

import (
	"encoding/json"
	"net/http"

	"github.com/mind-engage/mindengage-lms/internal/exam"

	"github.com/go-chi/chi/v5"
)

func CreateAttemptHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ExamID string `json:"exam_id"`
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		if req.ExamID == "" || req.UserID == "" {
			http.Error(w, "exam_id and user_id required", 400)
			return
		}
		a, err := store.NewAttempt(req.ExamID, req.UserID)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(a)
	}
}

func SaveResponsesHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "attemptID")
		var resp map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		a, err := store.SaveResponses(id, resp)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(a)
	}
}

func SubmitAttemptHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "attemptID")
		a, err := store.Submit(id)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(a)
	}
}

func GetAttemptHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "attemptID")
		a, err := store.GetAttempt(id)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		_ = json.NewEncoder(w).Encode(a)
	}
}
