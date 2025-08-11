// internal/api/http/user_password.go
package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	authmw "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"golang.org/x/crypto/bcrypt"
)

type changePasswordReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func ChangePasswordHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := authmw.SubjectFromContext(r.Context())
		if userID == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req changePasswordReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if req.NewPassword == "" {
			http.Error(w, "new password required", http.StatusBadRequest)
			return
		}

		var storedHash string
		err := db.QueryRow(`SELECT password_hash FROM users WHERE id=$1`, userID).Scan(&storedHash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.OldPassword)) != nil {
			http.Error(w, "incorrect old password", http.StatusForbidden)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = db.Exec(`UPDATE users SET password_hash=$1 WHERE id=$2`, hash, userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
