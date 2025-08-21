package http

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

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
		if strings.TrimSpace(e.ID) == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}

		// Validate policy/profile if present (unchanged)
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

		// Derive total time from policy if not explicitly set (unchanged)
		if e.TimeLimitSec == 0 && len(e.PolicyRaw) > 0 {
			var pol formats.Policy
			_ = json.Unmarshal(e.PolicyRaw, &pol)
			sum := 0
			for _, s := range pol.Sections {
				for _, m := range s.Modules {
					if m.TimeLimitSec > 0 {
						sum += m.TimeLimitSec
					}
				}
			}
			if sum > 0 {
				e.TimeLimitSec = sum
			}
		}

		sub, role := subjectAndRole(authSvc, r)
		isAdmin := role == "admin"

		// Does an exam with this ID already exist?
		var exists bool
		if err := db.QueryRowContext(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM exams WHERE id=$1)`, e.ID).Scan(&exists); err != nil {
			http.Error(w, "lookup exam: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if !exists {
			// Fresh create
			if err := store.PutExam(e); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = db.ExecContext(r.Context(),
				`INSERT INTO exam_owners (exam_id, teacher_id) VALUES ($1,$2)
				 ON CONFLICT (exam_id, teacher_id) DO NOTHING`,
				e.ID, sub,
			)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "created",
				"id":     e.ID,
			})
			return
		}

		// Exists: determine if caller is an owner
		var isOwner bool
		_ = db.QueryRowContext(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM exam_owners WHERE exam_id=$1 AND teacher_id=$2)`,
			e.ID, sub,
		).Scan(&isOwner)

		// Overwrite intent?
		overwrite := r.URL.Query().Get("overwrite") == "1" ||
			strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Allow-Overwrite")), "1")

		if overwrite {
			// Only owner or admin may overwrite an existing exam id
			if !(isOwner || isAdmin) {
				http.Error(w, "conflict: exam exists and you are not an owner (use fork)", http.StatusConflict)
				return
			}
			if err := store.PutExam(e); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = db.ExecContext(r.Context(),
				`INSERT INTO exam_owners (exam_id, teacher_id) VALUES ($1,$2)
				 ON CONFLICT (exam_id, teacher_id) DO NOTHING`,
				e.ID, sub,
			)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status": "updated",
				"id":     e.ID,
			})
			return
		}

		// Not overwriting: fork under a new ID to avoid clobbering
		oldID := e.ID
		e.ID = forkExamID(oldID, sub)
		if err := store.PutExam(e); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = db.ExecContext(r.Context(),
			`INSERT INTO exam_owners (exam_id, teacher_id) VALUES ($1,$2)
			 ON CONFLICT (exam_id, teacher_id) DO NOTHING`,
			e.ID, sub,
		)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":      "forked",
			"id":          e.ID,
			"forked_from": oldID,
		})
	}
}

// forkExamID generates a collision-resistant new exam id derived from a base.
func forkExamID(base, owner string) string {
	b := strings.TrimSpace(base)
	if b == "" {
		b = "exam"
	}
	// keep id readable but unique per uploader and second
	ownershort := owner
	if len(ownershort) > 8 {
		ownershort = ownershort[:8]
	}
	return b + "-" + ownershort + "-" + strconv.FormatInt(time.Now().Unix(), 10)
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
// DeleteExamHandler: remove the attempts-count guard
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

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "tx begin", http.StatusInternalServerError)
			return
		}
		defer func() { _ = tx.Rollback() }()

		// Optional explicit cleanup (FKs already handle offerings/attempts via CASCADE)
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM exam_offerings WHERE exam_id=$1`, examID)
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM exam_owners   WHERE exam_id=$1`, examID)

		if _, err := tx.ExecContext(r.Context(), `DELETE FROM exams WHERE id=$1`, examID); err != nil {
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
// DeleteCourseHandler: delete attempts for this course's offerings first
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

		// Ownership check (admins bypass). Require role='owner'
		if !isAdmin {
			var isOwner bool
			_ = db.QueryRowContext(r.Context(),
				`SELECT EXISTS(
			 SELECT 1 FROM course_teachers
			 WHERE course_id=$1 AND teacher_id=$2 AND role='owner'
		   )`, courseID, sub,
			).Scan(&isOwner)
			if !isOwner {
				http.Error(w, "forbidden (not course owner)", http.StatusForbidden)
				return
			}
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "tx begin", http.StatusInternalServerError)
			return
		}
		defer func() { _ = tx.Rollback() }()

		// 1) Delete attempts that belong to offerings of this course
		if _, err := tx.ExecContext(r.Context(), `
		DELETE FROM attempts
		WHERE offering_id IN (SELECT id FROM exam_offerings WHERE course_id=$1)
	  `, courseID); err != nil {
			http.Error(w, "delete attempts", http.StatusInternalServerError)
			return
		}

		// 2) Remove enrollments / co-teachers
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM course_students WHERE course_id=$1`, courseID)
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM course_teachers WHERE course_id=$1`, courseID)

		// 3) Delete offerings (FK from attempts was handled in step 1)
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM exam_offerings WHERE course_id=$1`, courseID); err != nil {
			http.Error(w, "delete offerings", http.StatusInternalServerError)
			return
		}

		// 4) Finally delete the course
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
