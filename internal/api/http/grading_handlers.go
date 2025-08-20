package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	authmw "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"github.com/mind-engage/mindengage-lms/internal/exam"
)

type applyGradesReq struct {
	Items    map[string]exam.ManualGradeInput `json:"items"`              // question_id -> grade
	Finalize bool                             `json:"finalize,omitempty"` // optional
}

// GET /attempts/{attemptID}/grading
func GetAttemptGradingHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attemptID := strings.TrimSpace(chi.URLParam(r, "attemptID"))
		if attemptID == "" {
			http.Error(w, "attemptID required", http.StatusBadRequest)
			return
		}
		items, err := store.GetAttemptItems(r.Context(), attemptID)
		if err != nil {
			http.Error(w, "grading items: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(items)
	}
}

// POST /attempts/{attemptID}/grading
func ApplyAttemptGradingHandler(store exam.Store, authSvc *authmw.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		attemptID := strings.TrimSpace(chi.URLParam(r, "attemptID"))
		if attemptID == "" {
			http.Error(w, "attemptID required", http.StatusBadRequest)
			return
		}
		var req applyGradesReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		sub, _ := subjectAndRole(authSvc, r)
		a, err := store.ApplyManualGrades(r.Context(), attemptID, req.Items, sub, req.Finalize)
		if err != nil {
			http.Error(w, "apply grades: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(a)
	}
}
