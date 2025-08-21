// internal/api/http/offerings_link.go
package http

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"net/http"
	nethttp "net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	ex "github.com/mind-engage/mindengage-lms/internal/exam"
	"github.com/mind-engage/mindengage-lms/internal/grading"
)

type EphemeralGradeReq struct {
	Responses map[string]any `json:"responses"`
}

type ItemResult struct {
	QuestionID    string   `json:"question_id"`
	Points        float64  `json:"points"`
	PointsMax     float64  `json:"points_max"`
	NeedsManual   bool     `json:"needs_manual,omitempty"`
	Feedback      []string `json:"feedback,omitempty"`
	Correct       bool     `json:"correct"`                  // true only if full credit
	CorrectAnswer []string `json:"correct_answer,omitempty"` // omitted unless ?show_answers=1
}

type EphemeralGradeResp struct {
	Score    float64      `json:"score"`
	ScoreMax float64      `json:"score_max"`
	Items    []ItemResult `json:"items"`
}

type EphemeralStatsResponse struct {
	OfferingID string  `json:"offering_id"`
	UpdatedAt  int64   `json:"updated_at"`
	Questions  []QStat `json:"questions"`
}
type QStat struct {
	QuestionID string   `json:"question_id"`
	Total      int64    `json:"total"`
	Avg        float64  `json:"avg_points"`
	Max        float64  `json:"max_points"`
	Buckets    []Bucket `json:"buckets"`
}
type Bucket struct {
	Key     string  `json:"key"`
	Count   int64   `json:"count"`
	Correct int64   `json:"correct"`
	Avg     float64 `json:"avg_points,omitempty"`
}

type offeringResolveResp struct {
	ID           string     `json:"id"`
	ExamID       string     `json:"exam_id"`
	CourseID     string     `json:"course_id"`
	StartAt      *time.Time `json:"start_at,omitempty"`
	EndAt        *time.Time `json:"end_at,omitempty"`
	TimeLimitSec *int       `json:"time_limit_sec,omitempty"`
	MaxAttempts  int        `json:"max_attempts"`
	Visibility   string     `json:"visibility"`
	State        string     `json:"state,omitempty"` // not_started | active | ended
	Exam         ex.Exam    `json:"exam"`            // student-safe (no answer_key)
}

// GetOfferingByTokenHandler returns offering metadata + student-safe exam via store.GetExam.
func GetOfferingByTokenHandler(db *sql.DB, store ex.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		offeringID := chi.URLParam(r, "offeringID")
		tok := strings.TrimSpace(r.URL.Query().Get("access_token"))
		if offeringID == "" || tok == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var out offeringResolveResp
		var start, end, tls sql.NullInt64
		var vis, dbTok string

		// Load offering + token
		err := db.QueryRowContext(r.Context(), `
			SELECT id, exam_id, course_id, start_at, end_at, time_limit_sec, max_attempts, visibility,
			       COALESCE(access_token,'')
			  FROM exam_offerings
			 WHERE id = $1
		`, offeringID).Scan(&out.ID, &out.ExamID, &out.CourseID, &start, &end, &tls, &out.MaxAttempts, &vis, &dbTok)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if vis != "link" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(dbTok)), []byte(tok)) != 1 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Timestamps / state
		if start.Valid {
			t := time.Unix(start.Int64, 0).UTC()
			out.StartAt = &t
		}
		if end.Valid {
			t := time.Unix(end.Int64, 0).UTC()
			out.EndAt = &t
		}
		if tls.Valid {
			v := int(tls.Int64)
			out.TimeLimitSec = &v
		}
		out.Visibility = vis
		now := time.Now().UTC().Unix()
		switch {
		case start.Valid && now < start.Int64:
			out.State = "not_started"
		case end.Valid && now > end.Int64:
			out.State = "ended"
		default:
			out.State = "active"
		}

		// Student-safe exam (no keys) + policy via store
		examSafe, err := store.GetExam(out.ExamID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		out.Exam = examSafe

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// Inject the SAME store and grader used elsewhere.
// If you support "scan", construct grader with grading.WithOCR(...).
func GradeEphemeralHandler(db *sql.DB, store ex.Store, grader grading.Grader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		offeringID := chi.URLParam(r, "offeringID")
		tok := strings.TrimSpace(r.URL.Query().Get("access_token"))
		if offeringID == "" || tok == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// 1) Token + window
		var vis, dbTok, examID string
		var start, end sql.NullInt64
		if err := db.QueryRow(`SELECT visibility, COALESCE(access_token,''), exam_id, start_at, end_at
		                        FROM exam_offerings WHERE id=$1`, offeringID).
			Scan(&vis, &dbTok, &examID, &start, &end); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if vis != "link" || subtle.ConstantTimeCompare([]byte(dbTok), []byte(tok)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		now := time.Now().UTC().Unix()
		if start.Valid && now < start.Int64 {
			http.Error(w, "not started", http.StatusForbidden)
			return
		}
		if end.Valid && now > end.Int64 {
			http.Error(w, "ended", http.StatusForbidden)
			return
		}

		// 2) Exam WITH keys for grading (admin view)
		exam, err := store.GetExamAdmin(r.Context(), examID)
		if err != nil {
			http.Error(w, "exam not found", http.StatusNotFound)
			return
		}

		// 3) Parse body
		var req EphemeralGradeReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if req.Responses == nil {
			req.Responses = map[string]any{}
		}

		showAnswers := r.URL.Query().Get("show_answers") == "1"

		// 4) Grade using same engine; normalize response types per strategy
		var out EphemeralGradeResp
		out.Items = make([]ItemResult, 0, len(exam.Questions))

		for _, q := range exam.Questions {
			gq := grading.Q{Type: q.Type, Points: q.Points, AnswerKey: q.AnswerKey}
			raw := req.Responses[q.ID]
			norm := normalizeForType(q.Type, raw) // <-- key difference vs earlier sketch

			res, _ := grader.Grade(context.Background(), gq, norm) // ignore error -> 0 points, like Submit()

			item := ItemResult{
				QuestionID:  q.ID,
				Points:      res.AutoPoints,
				PointsMax:   q.Points,
				NeedsManual: res.NeedsManual,
				Feedback:    res.Feedback,
				// full credit only -> Correct=true (partial credit remains false here)
				Correct: q.Points > 0 && res.AutoPoints >= q.Points,
			}
			if showAnswers {
				item.CorrectAnswer = q.AnswerKey
			}

			out.Score += item.Points
			out.ScoreMax += item.PointsMax
			out.Items = append(out.Items, item)

			isCorrect := q.Points > 0 && res.AutoPoints >= q.Points
			maxPts := q.Points

			// 1) always bump totals ("*")
			_ = bumpEphemeral(db, offeringID, q.ID, "*", isCorrect, res.AutoPoints, maxPts)

			// 2) optionally bump answer buckets — use the NORMALIZED response
			for _, k := range bucketKeys(q.Type, norm) {
				_ = bumpEphemeral(db, offeringID, q.ID, k, isCorrect, res.AutoPoints, maxPts)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// normalizeForType coerces incoming JSON to what each grading strategy expects.
// Matches what your persisted path effectively tolerates (Submit ignores type errors).
func normalizeForType(qType string, v any) any {
	switch strings.ToLower(strings.TrimSpace(qType)) {
	case "mcq_single", "true_false":
		// prefer string; if []string came in, pick first; numbers -> string
		switch t := v.(type) {
		case string:
			return t
		case []any:
			for _, e := range t {
				if s, ok := e.(string); ok {
					return s
				}
			}
		case []string:
			if len(t) > 0 {
				return t[0]
			}
		case float64:
			return trimFloat(t)
		case int, int64:
			return toString(t)
		}
		return v
	case "mcq_multi":
		// prefer []string; if string came in, split on commas; if []any, keep strings
		switch t := v.(type) {
		case []string:
			return t
		case []any:
			out := make([]string, 0, len(t))
			for _, e := range t {
				if s, ok := e.(string); ok {
					out = append(out, s)
				}
			}
			return out
		case string:
			parts := strings.Split(t, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				if s := strings.TrimSpace(p); s != "" {
					out = append(out, s)
				}
			}
			return out
		default:
			return v
		}
	case "numeric":
		// strategy expects string; coerce numbers
		switch t := v.(type) {
		case string:
			return t
		case float64:
			return trimFloat(t)
		case int, int64:
			return toString(t)
		default:
			return v
		}
	case "short_word":
		// expects string
		switch t := v.(type) {
		case string:
			return t
		case []any:
			for _, e := range t {
				if s, ok := e.(string); ok {
					return s
				}
			}
			return ""
		default:
			return toString(t)
		}
	case "scan":
		// pass-through: []byte or file path string
		return v
	default:
		return v
	}
}

func trimFloat(f float64) string {
	// nicer string for integers 3.0 -> "3"
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return trimFloat(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func GetEphemeralStatsHandler(db *sql.DB) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		offID := chi.URLParam(r, "offeringID")
		tok := strings.TrimSpace(r.URL.Query().Get("access_token"))
		if offID == "" || tok == "" {
			nethttp.Error(w, "bad request", nethttp.StatusBadRequest)
			return
		}

		// token + visibility check (same as /resolve)
		var vis, dbTok string
		if err := db.QueryRowContext(r.Context(),
			`SELECT visibility, COALESCE(access_token,'') FROM exam_offerings WHERE id=$1`, offID).
			Scan(&vis, &dbTok); err != nil || vis != "link" ||
			subtle.ConstantTimeCompare([]byte(strings.TrimSpace(dbTok)), []byte(tok)) != 1 {
			nethttp.Error(w, "not found", nethttp.StatusNotFound)
			return
		}

		var since int64
		if s := strings.TrimSpace(r.URL.Query().Get("since")); s != "" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil {
				since = v
			}
		}

		var rows *sql.Rows
		var err error
		if since > 0 {
			rows, err = db.QueryContext(r.Context(), `
				SELECT question_id, bucket, count, correct, sum_points, max_points, updated_at
				  FROM ephemeral_stats
				 WHERE offering_id=$1 AND updated_at>$2
				ORDER BY question_id, bucket`, offID, since)
		} else {
			rows, err = db.QueryContext(r.Context(), `
				SELECT question_id, bucket, count, correct, sum_points, max_points, updated_at
				  FROM ephemeral_stats
				 WHERE offering_id=$1
				ORDER BY question_id, bucket`, offID)
		}
		if err != nil {
			nethttp.Error(w, "db error", nethttp.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type acc struct {
			total int64
			sum   float64
			max   float64
			bks   []Bucket
		}
		out := EphemeralStatsResponse{OfferingID: offID, UpdatedAt: time.Now().Unix()}
		accs := map[string]*acc{}

		for rows.Next() {
			var qid, bucket string
			var cnt, cor int64
			var sum, max float64
			var upd int64
			if err := rows.Scan(&qid, &bucket, &cnt, &cor, &sum, &max, &upd); err != nil {
				continue
			}
			if upd > out.UpdatedAt {
				out.UpdatedAt = upd
			}
			a := accs[qid]
			if a == nil {
				a = &acc{}
				accs[qid] = a
			}
			if bucket == "*" {
				a.total += cnt
				a.sum += sum
				if max > a.max {
					a.max = max
				}
				continue
			}
			b := Bucket{Key: bucket, Count: cnt, Correct: cor}
			if cnt > 0 {
				b.Avg = sum / float64(cnt)
			}
			a.bks = append(a.bks, b)
			if max > a.max {
				a.max = max
			}
		}

		for qid, a := range accs {
			q := QStat{QuestionID: qid, Total: a.total, Max: a.max, Buckets: a.bks}
			if a.total > 0 {
				q.Avg = a.sum / float64(a.total)
			}
			out.Questions = append(out.Questions, q)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// Portable UPSERT for SQLite/Postgres (no GREATEST)
func bumpEphemeral(db *sql.DB, offID, qid, bucket string, correct bool, auto, max float64) error {
	now := time.Now().Unix()
	cor := int64(0)
	if correct {
		cor = 1
	}
	_, err := db.Exec(`
		INSERT INTO ephemeral_stats (offering_id, question_id, bucket, count, correct, sum_points, max_points, updated_at)
		VALUES ($1,$2,$3,1,$4,$5,$6,$7)
		ON CONFLICT (offering_id, question_id, bucket) DO UPDATE SET
		  count      = ephemeral_stats.count + 1,
		  correct    = ephemeral_stats.correct + EXCLUDED.correct,
		  sum_points = ephemeral_stats.sum_points + EXCLUDED.sum_points,
		  max_points = CASE WHEN ephemeral_stats.max_points > EXCLUDED.max_points
		                    THEN ephemeral_stats.max_points ELSE EXCLUDED.max_points END,
		  updated_at = EXCLUDED.updated_at
	`, offID, qid, bucket, cor, auto, max, now)
	return err
}

func normText(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 64 {
		s = s[:64]
	} // keep buckets tight
	return s
}

func bucketKeys(qType string, resp interface{}) []string {
	switch strings.ToLower(qType) {
	case "mcq_single", "true_false":
		if v, ok := resp.(string); ok && v != "" {
			return []string{v}
		}
	case "mcq_multi":
		// record both per-option and combo
		var arr []string
		switch t := resp.(type) {
		case []string:
			arr = t
		case []interface{}:
			for _, e := range t {
				if s, ok := e.(string); ok {
					arr = append(arr, s)
				}
			}
		}
		if len(arr) > 0 {
			sort.Strings(arr)
			keys := make([]string, 0, 1+len(arr))
			keys = append(keys, "set:"+strings.Join(arr, ","))
			for _, o := range arr {
				keys = append(keys, "opt:"+o)
			}
			return keys
		}
	case "short_word", "numeric":
		if v, ok := resp.(string); ok && v != "" {
			return []string{"text:" + normText(v)}
		}
	case "essay", "scan":
		// too free-form; only totals will be useful
	}
	return nil
}
