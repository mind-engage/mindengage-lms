// internal/api/http/offerings_public.go
package http

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func ListPublicOfferingsHandler(db *sql.DB) http.HandlerFunc {
	type off struct {
		ID           string     `json:"id"`
		CourseID     string     `json:"course_id"`
		ExamID       string     `json:"exam_id"`
		StartAt      *time.Time `json:"start_at,omitempty"`
		EndAt        *time.Time `json:"end_at,omitempty"`
		TimeLimitSec *int       `json:"time_limit_sec,omitempty"`
		MaxAttempts  int        `json:"max_attempts"`
		Visibility   string     `json:"visibility"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().Unix()
		rows, err := db.Query(`
			SELECT id, course_id, exam_id, start_at, end_at, time_limit_sec, max_attempts, visibility
			  FROM exam_offerings
			 WHERE visibility='public'
			   AND (start_at IS NULL OR start_at <= $1)
			   AND (end_at   IS NULL OR end_at   >= $1)
			 ORDER BY start_at NULLS FIRST, id
		`, now)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out := make([]off, 0, 8)
		for rows.Next() {
			var o off
			var start, end, tls sql.NullInt64
			if err := rows.Scan(&o.ID, &o.CourseID, &o.ExamID, &start, &end, &tls, &o.MaxAttempts, &o.Visibility); err != nil {
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
		_ = json.NewEncoder(w).Encode(out)
	}
}
