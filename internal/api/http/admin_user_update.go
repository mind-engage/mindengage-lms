package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type updateUserRoleReq struct {
	Role string `json:"role"`
}

func AdminUpdateUserRoleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := chi.URLParam(r, "userID") // may be id or username; frontend encodes it
		if target == "" {
			http.Error(w, "missing userID", http.StatusBadRequest)
			return
		}

		var req updateUserRoleReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		role := strings.ToLower(strings.TrimSpace(req.Role))
		if role != "student" && role != "teacher" && role != "admin" {
			http.Error(w, "invalid role", http.StatusBadRequest)
			return
		}

		// Ensure user exists & guard against demoting the last admin
		var id, curRole string
		err := db.QueryRowContext(r.Context(),
			`SELECT id, role FROM users WHERE id=$1 OR username=$1`, target).Scan(&id, &curRole)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if curRole == "admin" && role != "admin" {
			var adminCount int
			if err := db.QueryRowContext(r.Context(),
				`SELECT COUNT(1) FROM users WHERE role='admin'`).Scan(&adminCount); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if adminCount <= 1 {
				http.Error(w, "cannot demote the last admin", http.StatusBadRequest)
				return
			}
		}

		// Update role
		if _, err := db.ExecContext(r.Context(),
			`UPDATE users SET role=$2 WHERE id=$1 OR username=$1`, target, role); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
