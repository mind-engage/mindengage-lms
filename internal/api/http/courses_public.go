// internal/api/http/courses_public.go
package http

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func ListPublicCoursesHandler(db *sql.DB) http.HandlerFunc {
	type row struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		OpenPublicCount int    `json:"open_public_count"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().Unix()
		rows, err := db.Query(`
			SELECT c.id, c.name, COUNT(o.id) AS open_public_count
			  FROM courses c
			  JOIN exam_offerings o ON o.course_id = c.id
			 WHERE o.visibility = 'public'
			   AND (o.start_at IS NULL OR o.start_at <= $1)
			   AND (o.end_at   IS NULL OR o.end_at   >= $1)
			 GROUP BY c.id, c.name
			 ORDER BY open_public_count DESC, c.name
		`, now)
		if err != nil {
			http.Error(w, "db error", 500)
			return
		}
		defer rows.Close()

		var out []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.ID, &r.Name, &r.OpenPublicCount); err == nil {
				out = append(out, r)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}
