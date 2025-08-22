package http

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	nethttp "net/http"

	"github.com/go-chi/chi/v5"
	authmw "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
)

// Handlers only — routes remain in main.go

type Course struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func CreateCourseHandler(dbh *sql.DB, authSvc *authmw.AuthService) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		sub, role := subjectFromBearer(authSvc, r)
		if sub == "" {
			nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
			return
		}
		if role != "teacher" && role != "admin" {
			nethttp.Error(w, "forbidden", nethttp.StatusForbidden)
			return
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			nethttp.Error(w, "bad json", nethttp.StatusBadRequest)
			return
		}
		courseID := "c-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		if _, err := dbh.Exec(`INSERT INTO courses (id, name, created_by) VALUES ($1, $2, $3)`, courseID, req.Name, sub); err != nil {
			nethttp.Error(w, "db error", nethttp.StatusInternalServerError)
			return
		}
		// creator becomes owner teacher
		_, _ = dbh.Exec(`INSERT INTO course_teachers (course_id, teacher_id, role) VALUES ($1, $2, 'owner') ON CONFLICT DO NOTHING`, courseID, sub)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": courseID, "name": req.Name})
	}
}

func ListCoursesHandler(db *sql.DB, authSvc *authmw.AuthService) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		sub, role := subjectFromBearer(authSvc, r)
		if sub == "" {
			nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
			return
		}

		// Filters
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		teacherID := strings.TrimSpace(r.URL.Query().Get("teacher_id"))
		studentID := strings.TrimSpace(r.URL.Query().Get("student_id"))
		all := r.URL.Query().Get("all") == "1"

		limit := 50
		offset := 0
		if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
			if v > 200 {
				v = 200
			}
			limit = v
		}
		if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
			offset = v
		}

		var (
			sqlStr string
			args   []any
		)

		addNameFilter := func(base string, argStart int) (string, []any) {
			// argStart = next placeholder index (1-based)
			if q != "" {
				base += fmt.Sprintf(" AND c.name ILIKE '%%' || $%d || '%%' ", argStart)
				return base, []any{q}
			}
			return base, nil
		}

		switch role {
		case "admin":
			switch {
			case all:
				sqlStr = `
					SELECT c.id, c.name
					  FROM courses c
					 WHERE 1=1`
				var extra []any
				sqlStr, extra = addNameFilter(sqlStr, 1)
				args = append(args, extra...)
				args = append(args, limit, offset)
				sqlStr += ` ORDER BY c.created_at DESC LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))

			case teacherID != "":
				sqlStr = `
					SELECT c.id, c.name
					  FROM courses c
					  JOIN course_teachers t ON t.course_id=c.id
					 WHERE t.teacher_id=$1`
				var extra []any
				sqlStr, extra = addNameFilter(sqlStr, 2)
				args = append(args, teacherID)
				args = append(args, extra...)
				args = append(args, limit, offset)
				sqlStr += ` ORDER BY c.created_at DESC LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))

			case studentID != "":
				sqlStr = `
					SELECT c.id, c.name
					  FROM courses c
					  JOIN course_students s ON s.course_id=c.id
					 WHERE s.student_id=$1 AND s.status='active'`
				var extra []any
				sqlStr, extra = addNameFilter(sqlStr, 2)
				args = append(args, studentID)
				args = append(args, extra...)
				args = append(args, limit, offset)
				sqlStr += ` ORDER BY c.created_at DESC LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))

			default:
				// "admin but no special filters" – either mimic teacher view or return all.
				// To keep it least-surprising, return *all*:
				sqlStr = `
					SELECT c.id, c.name
					  FROM courses c
					 WHERE 1=1`
				var extra []any
				sqlStr, extra = addNameFilter(sqlStr, 1)
				args = append(args, extra...)
				args = append(args, limit, offset)
				sqlStr += ` ORDER BY c.created_at DESC LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
			}

		case "teacher":
			sqlStr = `
				SELECT c.id, c.name
				  FROM courses c
				  JOIN course_teachers t ON t.course_id=c.id
				 WHERE t.teacher_id=$1`
			var extra []any
			sqlStr, extra = addNameFilter(sqlStr, 2)
			args = append(args, sub)
			args = append(args, extra...)
			args = append(args, limit, offset)
			sqlStr += ` ORDER BY c.created_at DESC LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))

		default: // student
			sqlStr = `
				SELECT c.id, c.name
				  FROM courses c
				  JOIN course_students s ON s.course_id=c.id
				 WHERE s.student_id=$1 AND s.status='active'`
			var extra []any
			sqlStr, extra = addNameFilter(sqlStr, 2)
			args = append(args, sub)
			args = append(args, extra...)
			args = append(args, limit, offset)
			sqlStr += ` ORDER BY c.created_at DESC LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
		}

		rows, err := db.QueryContext(r.Context(), sqlStr, args...)
		if err != nil {
			nethttp.Error(w, "db error", nethttp.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out := []Course{}
		for rows.Next() {
			var c Course
			if err := rows.Scan(&c.ID, &c.Name); err == nil {
				out = append(out, c)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

func AddCoTeachersHandler(dbh *sql.DB, authSvc *authmw.AuthService) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		courseID := chi.URLParam(r, "courseID")
		sub, _ := subjectFromBearer(authSvc, r)
		if sub == "" {
			nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
			return
		}
		if !isCourseTeacher(dbh, sub, courseID) {
			nethttp.Error(w, "forbidden", nethttp.StatusForbidden)
			return
		}
		var req struct {
			UserIDs []string `json:"user_ids"`
			Role    string   `json:"role"` // "co" or "owner"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.UserIDs) == 0 {
			nethttp.Error(w, "bad json", nethttp.StatusBadRequest)
			return
		}
		role := "co"
		if req.Role == "owner" {
			role = "owner"
		}
		for _, uid := range req.UserIDs {
			uid = strings.TrimSpace(uid)
			if uid == "" {
				continue
			}
			_, _ = dbh.Exec(`INSERT INTO course_teachers (course_id, teacher_id, role) VALUES ($1, $2, $3)
                       ON CONFLICT (course_id, teacher_id) DO UPDATE SET role=EXCLUDED.role`, courseID, uid, role)
		}
		w.WriteHeader(nethttp.StatusNoContent)
	}
}

func EnrollStudentsHandler(dbh *sql.DB, authSvc *authmw.AuthService) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		courseID := chi.URLParam(r, "courseID")
		sub, _ := subjectFromBearer(authSvc, r)
		if sub == "" {
			nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
			return
		}
		if !isCourseTeacher(dbh, sub, courseID) {
			nethttp.Error(w, "forbidden", nethttp.StatusForbidden)
			return
		}
		var req struct {
			UserIDs []string `json:"user_ids"`
			Status  string   `json:"status"` // default active
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.UserIDs) == 0 {
			nethttp.Error(w, "bad json", nethttp.StatusBadRequest)
			return
		}
		status := "active"
		if s := strings.ToLower(strings.TrimSpace(req.Status)); s == "invited" || s == "dropped" {
			status = s
		}
		for _, uid := range req.UserIDs {
			uid = strings.TrimSpace(uid)
			if uid == "" {
				continue
			}
			_, _ = dbh.Exec(`INSERT INTO course_students (course_id, student_id, status) VALUES ($1, $2, $3)
                       ON CONFLICT (course_id, student_id) DO UPDATE SET status=EXCLUDED.status`, courseID, uid, status)
		}
		w.WriteHeader(nethttp.StatusNoContent)
	}
}

func CreateOfferingHandler(dbh *sql.DB, authSvc *authmw.AuthService) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		courseID := chi.URLParam(r, "courseID")
		sub, _ := subjectFromBearer(authSvc, r)
		if sub == "" {
			nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
			return
		}
		if !isCourseTeacher(dbh, sub, courseID) {
			nethttp.Error(w, "forbidden", nethttp.StatusForbidden)
			return
		}
		var req struct {
			ExamID       string  `json:"exam_id"`
			StartAt      *int64  `json:"start_at,omitempty"` // unix seconds
			EndAt        *int64  `json:"end_at,omitempty"`
			TimeLimitSec *int    `json:"time_limit_sec,omitempty"`
			MaxAttempts  *int    `json:"max_attempts,omitempty"`
			Visibility   *string `json:"visibility,omitempty"`
			AccessToken  *string `json:"access_token,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.ExamID) == "" {
			nethttp.Error(w, "bad json", nethttp.StatusBadRequest)
			return
		}

		offID := "o-" + strconv.FormatInt(time.Now().UnixNano(), 10)

		// ✅ Keep Unix seconds as *int64 to match BIGINT columns
		var startAt, endAt *int64
		if req.StartAt != nil {
			v := *req.StartAt
			startAt = &v
		}
		if req.EndAt != nil {
			v := *req.EndAt
			endAt = &v
		}

		timeLimit := sql.NullInt64{}
		if req.TimeLimitSec != nil {
			timeLimit.Valid = true
			timeLimit.Int64 = int64(*req.TimeLimitSec)
		}
		maxAttempts := 1
		if req.MaxAttempts != nil && *req.MaxAttempts > 0 {
			maxAttempts = *req.MaxAttempts
		}
		visibility := "course"
		if req.Visibility != nil && (*req.Visibility == "public" || *req.Visibility == "link") {
			visibility = *req.Visibility
		}
		var accTok sql.NullString
		if req.AccessToken != nil && strings.TrimSpace(*req.AccessToken) != "" {
			accTok.Valid = true
			accTok.String = strings.TrimSpace(*req.AccessToken)
		}

		if _, err := dbh.Exec(`
            INSERT INTO exam_offerings
                (id, exam_id, course_id, assigned_by, start_at, end_at, time_limit_sec, max_attempts, visibility, access_token)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        `, offID, req.ExamID, courseID, sub, startAt, endAt, timeLimit, maxAttempts, visibility, accTok); err != nil {
			nethttp.Error(w, "db error", nethttp.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": offID})
	}
}

func ListOfferingsHandler(dbh *sql.DB, authSvc *authmw.AuthService) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		courseID := chi.URLParam(r, "courseID")
		sub, role := subjectFromBearer(authSvc, r)
		if sub == "" {
			nethttp.Error(w, "unauthorized", nethttp.StatusUnauthorized)
			return
		}
		if role == "student" && !isCourseStudent(dbh, sub, courseID) {
			nethttp.Error(w, "forbidden", nethttp.StatusForbidden)
			return
		}
		if role != "student" && !isCourseTeacher(dbh, sub, courseID) && role != "admin" {
			nethttp.Error(w, "forbidden", nethttp.StatusForbidden)
			return
		}

		rows, err := dbh.Query(`
			SELECT id, exam_id, start_at, end_at, time_limit_sec, max_attempts, visibility
			FROM exam_offerings
			WHERE course_id=$1
			ORDER BY start_at NULLS FIRST, id
		`, courseID)
		if err != nil {
			nethttp.Error(w, "db error", nethttp.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type off struct {
			ID           string     `json:"id"`
			ExamID       string     `json:"exam_id"`
			StartAt      *time.Time `json:"start_at,omitempty"` // RFC3339 in JSON
			EndAt        *time.Time `json:"end_at,omitempty"`
			TimeLimitSec *int       `json:"time_limit_sec,omitempty"`
			MaxAttempts  int        `json:"max_attempts"`
			Visibility   string     `json:"visibility"`
		}

		out := make([]off, 0, 8) // ensures [] not null

		for rows.Next() {
			var o off
			var start, end sql.NullInt64
			var tls sql.NullInt64

			if err := rows.Scan(&o.ID, &o.ExamID, &start, &end, &tls, &o.MaxAttempts, &o.Visibility); err != nil {
				// optionally log the scan error
				continue
			}
			if start.Valid {
				t := time.Unix(start.Int64, 0).UTC()
				o.StartAt = &t
			}
			if end.Valid {
				t := time.Unix(end.Int64, 0).UTC()
				o.EndAt = &t
			}
			if tls.Valid {
				v := int(tls.Int64)
				o.TimeLimitSec = &v
			}
			out = append(out, o)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out) // [] when empty
	}
}

// ---------- Local helpers (moved from main.go) ----------

func subjectFromBearer(a *authmw.AuthService, r *nethttp.Request) (sub, role string) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", ""
	}
	claims, err := a.Parse(strings.TrimPrefix(h, "Bearer "))
	if err != nil {
		return "", ""
	}
	return claims.Sub, claims.Role
}

func isCourseTeacher(db *sql.DB, userID, courseID string) bool {
	var ok bool
	_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM course_teachers WHERE course_id=$1 AND teacher_id=$2)`, courseID, userID).Scan(&ok)
	return ok
}

func isCourseStudent(db *sql.DB, userID, courseID string) bool {
	var ok bool
	_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM course_students WHERE course_id=$1 AND student_id=$2 AND status='active')`, courseID, userID).Scan(&ok)
	return ok
}
