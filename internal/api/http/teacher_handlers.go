package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mind-engage/mindengage-lms/internal/exam"
	"github.com/mind-engage/mindengage-lms/internal/formats"
)

// ---- Adapters to satisfy formats.ExamLike without changing exam package ----

type examAdapter struct{ e *exam.Exam }

func (x examAdapter) GetID() string    { return x.e.ID }
func (x examAdapter) GetTitle() string { return x.e.Title }
func (x examAdapter) GetQuestions() []formats.QuestionLike {
	out := make([]formats.QuestionLike, len(x.e.Questions))
	for i := range x.e.Questions {
		out[i] = questionAdapter{q: &x.e.Questions[i]}
	}
	return out
}

type questionAdapter struct{ q *exam.Question }

func (q questionAdapter) GetID() string   { return q.q.ID }
func (q questionAdapter) GetType() string { return q.q.Type }
func (q questionAdapter) GetChoices() []formats.ChoiceLike {
	out := make([]formats.ChoiceLike, len(q.q.Choices))
	for i := range q.q.Choices {
		out[i] = choiceAdapter{c: &q.q.Choices[i]}
	}
	return out
}

type choiceAdapter struct{ c *exam.Choice }

func (c choiceAdapter) GetID() string { return c.c.ID }

// ---------------------------------------------------------------------------

func totalTimeFromPolicy(pol formats.Policy) int {
	sum := 0
	for _, s := range pol.Sections {
		for _, m := range s.Modules {
			if m.TimeLimitSec > 0 {
				sum += m.TimeLimitSec
			}
		}
	}
	return sum
}

func UploadExamHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var e exam.Exam
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if e.ID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}

		// If a profile/policy is provided, validate it before saving.
		if e.Profile != "" && len(e.PolicyRaw) > 0 {
			var pol formats.Policy
			if err := json.Unmarshal(e.PolicyRaw, &pol); err != nil {
				http.Error(w, "invalid policy json: "+err.Error(), http.StatusBadRequest)
				return
			}
			// Basic policy sanity.
			if err := formats.ValidatePolicy(e.Profile, &pol); err != nil {
				http.Error(w, "policy validation failed: "+err.Error(), http.StatusBadRequest)
				return
			}
			// Profile-specific rules via adapter; use wrappers to satisfy interfaces.
			if a, ok := formats.Lookup(e.Profile); ok {
				if err := a.Validate(examAdapter{e: &e}, pol); err != nil {
					http.Error(w, "profile validation failed: "+err.Error(), http.StatusBadRequest)
					return
				}
			} else {
				http.Error(w, "unknown profile: "+e.Profile, http.StatusBadRequest)
				return
			}
		}

		if e.TimeLimitSec == 0 && len(e.PolicyRaw) > 0 {
			var pol formats.Policy
			_ = json.Unmarshal(e.PolicyRaw, &pol)
			if tl := totalTimeFromPolicy(pol); tl > 0 {
				e.TimeLimitSec = tl
			}
		}

		if err := store.PutExam(e); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "id": e.ID})
	}
}

func GetExamHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "examID")
		e, err := store.GetExam(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(e)
	}
}
