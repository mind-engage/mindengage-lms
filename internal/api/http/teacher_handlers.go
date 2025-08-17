package http

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	authmw "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
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

func UploadExamHandler(store exam.Store, db *sql.DB, authSvc *authmw.AuthService) http.HandlerFunc {
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
			if err := formats.ValidatePolicy(e.Profile, &pol); err != nil {
				http.Error(w, "policy validation failed: "+err.Error(), http.StatusBadRequest)
				return
			}
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

		// Derive total time from policy if not explicitly set.
		if e.TimeLimitSec == 0 && len(e.PolicyRaw) > 0 {
			var pol formats.Policy
			_ = json.Unmarshal(e.PolicyRaw, &pol)
			if tl := totalTimeFromPolicy(pol); tl > 0 {
				e.TimeLimitSec = tl
			}
		}

		// Save exam (upsert depending on store implementation)
		if err := store.PutExam(e); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Insert exam ownership for the uploading teacher/admin.
		// JWT middleware is in place for this route, so we should have a subject.
		if db != nil && authSvc != nil {
			if sub, _ := subjectAndRole(authSvc, r); strings.TrimSpace(sub) != "" {
				// Portable UPSERT for Postgres + modern SQLite:
				if _, err := db.ExecContext(
					r.Context(),
					`INSERT INTO exam_owners (exam_id, teacher_id) VALUES ($1, $2)
					 ON CONFLICT (exam_id, teacher_id) DO NOTHING`,
					e.ID, sub,
				); err != nil {
					// If ownership write fails, surface it — permissions later depend on this row.
					http.Error(w, "exam saved but owner insert failed: "+err.Error(), http.StatusInternalServerError)
					return
				}
			}
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

// subjectAndRole extracts (sub, role) from Authorization using the same service
// your other handlers use. Returns ("","") if missing/invalid.
func subjectAndRole(authSvc *authmw.AuthService, r *http.Request) (string, string) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", ""
	}
	claims, err := authSvc.Parse(strings.TrimPrefix(h, "Bearer "))
	if err != nil {
		return "", ""
	}
	return claims.Sub, claims.Role
}

// DeleteExamHandler deletes an exam if and only if:
//   - caller is admin OR is listed as an owner in exam_owners (teacher),
//   - and there are NO attempts for this exam (any offering/any course).
//
// It also removes related ownership rows and offerings in the same tx.
// (Your FKs would cascade exam_offerings on exam delete, but we do it explicitly.)
func DeleteExamHandler(db *sql.DB, authSvc *authmw.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		examID := strings.TrimSpace(chi.URLParam(r, "examID"))
		if examID == "" {
			http.Error(w, "examID required", http.StatusBadRequest)
			return
		}

		sub, role := subjectAndRole(authSvc, r)
		isAdmin := role == "admin"

		// Ensure exam exists
		var exists bool
		if err := db.QueryRowContext(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM exams WHERE id=$1)`, examID).Scan(&exists); err != nil {
			http.Error(w, "lookup exam", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Ownership check for teachers (admins bypass)
		if !isAdmin {
			var owner bool
			_ = db.QueryRowContext(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM exam_owners WHERE exam_id=$1 AND teacher_id=$2)`,
				examID, sub,
			).Scan(&owner)
			if !owner {
				http.Error(w, "forbidden (not owner)", http.StatusForbidden)
				return
			}
		}

		// Ref guard: any attempts for this exam?
		var attemptsCnt int
		_ = db.QueryRowContext(r.Context(),
			`SELECT COUNT(1) FROM attempts WHERE exam_id=$1`, examID,
		).Scan(&attemptsCnt)
		if attemptsCnt > 0 {
			http.Error(w, "cannot delete: attempts exist (archive instead)", http.StatusConflict)
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "tx begin", http.StatusInternalServerError)
			return
		}
		defer func() { _ = tx.Rollback() }()

		// Clean related rows (order is conservative; FKs would also handle this)
		if _, err := tx.ExecContext(r.Context(),
			`DELETE FROM exam_offerings WHERE exam_id=$1`, examID); err != nil {
			http.Error(w, "delete offerings", http.StatusInternalServerError)
			return
		}
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM exam_owners WHERE exam_id=$1`, examID)

		if _, err := tx.ExecContext(r.Context(),
			`DELETE FROM exams WHERE id=$1`, examID); err != nil {
			http.Error(w, "delete exam", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "tx commit", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DeleteCourseHandler deletes a course if and only if:
//   - caller is admin OR is an owner teacher of the course (role='owner'),
//   - and there are NO attempts in any offering of this course.
//
// It also removes enrollments/co-teachers/offerings before deleting the course.
func DeleteCourseHandler(db *sql.DB, authSvc *authmw.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		courseID := strings.TrimSpace(chi.URLParam(r, "courseID"))
		if courseID == "" {
			http.Error(w, "courseID required", http.StatusBadRequest)
			return
		}

		sub, role := subjectAndRole(authSvc, r)
		isAdmin := role == "admin"

		// Ensure course exists
		var exists bool
		if err := db.QueryRowContext(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM courses WHERE id=$1)`, courseID).Scan(&exists); err != nil {
			http.Error(w, "lookup course", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Ownership check (admins bypass). Require role='owner' on this course.
		if !isAdmin {
			var isOwner bool
			_ = db.QueryRowContext(r.Context(),
				`SELECT EXISTS(SELECT 1 FROM course_teachers WHERE course_id=$1 AND teacher_id=$2 AND role='owner')`,
				courseID, sub,
			).Scan(&isOwner)
			if !isOwner {
				http.Error(w, "forbidden (not course owner)", http.StatusForbidden)
				return
			}
		}

		// Any attempts tied to offerings of this course?
		var attemptsCnt int
		_ = db.QueryRowContext(r.Context(), `
			SELECT COUNT(1)
			FROM attempts a
			JOIN exam_offerings o ON a.offering_id = o.id
			WHERE o.course_id = $1
		`, courseID).Scan(&attemptsCnt)
		if attemptsCnt > 0 {
			http.Error(w, "cannot delete: attempts exist (end/archive instead)", http.StatusConflict)
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "tx begin", http.StatusInternalServerError)
			return
		}
		defer func() { _ = tx.Rollback() }()

		// Remove enrollments / co-teachers and offerings
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM course_students WHERE course_id=$1`, courseID)
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM course_teachers WHERE course_id=$1`, courseID)
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM exam_offerings WHERE course_id=$1`, courseID); err != nil {
			http.Error(w, "delete offerings", http.StatusInternalServerError)
			return
		}

		// Finally the course
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM courses WHERE id=$1`, courseID); err != nil {
			http.Error(w, "delete course", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "tx commit", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
