package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// -----------------------------
// Admin: Compliance & Audit
// -----------------------------

// handleAdminPIIExport returns all PII for a given user (admin-only).
// handleAdminPIIExport returns all PII for a given user (admin-only) as a downloadable JSON file.
func HandleAdminPIIExport(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}

		row := db.QueryRowContext(r.Context(),
			`SELECT id, username, role, created_at FROM users WHERE id=$1 OR username=$1`,
			req.UserID)

		var id, username, role string
		var createdAt int64
		if err := row.Scan(&id, &username, &role, &createdAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "user not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		resp := map[string]any{
			"id":         id,
			"username":   username,
			"role":       role,
			"created_at": createdAt,
		}

		filename := fmt.Sprintf("pii_%s.json", id)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// handleAdminPIIDelete deletes (or anonymizes) all user data for GDPR-style compliance.
func HandleAdminPIIDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// Remove attempts
		if _, err := tx.ExecContext(r.Context(),
			`DELETE FROM attempts WHERE user_id=$1`, req.UserID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Remove from course enrollments
		if _, err := tx.ExecContext(r.Context(),
			`DELETE FROM course_students WHERE student_id=$1`, req.UserID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := tx.ExecContext(r.Context(),
			`DELETE FROM course_teachers WHERE teacher_id=$1`, req.UserID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Finally remove the user record
		if _, err := tx.ExecContext(r.Context(),
			`DELETE FROM users WHERE id=$1 OR username=$1`, req.UserID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

// handleAdminAuditSearch queries the event_log for recent events, filtered by q.
func HandleAdminAuditSearch(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")

		rows, err := db.QueryContext(r.Context(),
			`SELECT typ, key, data, created_at FROM event_log
			 WHERE typ LIKE '%'||$1||'%' OR key LIKE '%'||$1||'%'
			 ORDER BY created_at DESC LIMIT 100`, q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var out []map[string]any
		for rows.Next() {
			var typ, key, data string
			var createdAt int64
			if err := rows.Scan(&typ, &key, &data, &createdAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out = append(out, map[string]any{
				"typ":        typ,
				"key":        key,
				"data":       data,
				"created_at": time.Unix(createdAt, 0),
			})
		}

		respondJSON(w, http.StatusOK, out)
	}
}

// shared JSON helper (same as in admin_api_stubs.go)
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
