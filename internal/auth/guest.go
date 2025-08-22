package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	authmw "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"github.com/mind-engage/mindengage-lms/internal/config"
)

func GuestLoginHandler(a *authmw.AuthService, db *sql.DB, cfg config.Config) http.HandlerFunc {
	type out struct {
		AccessToken string `json:"access_token"`
		Username    string `json:"username"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.EnableGuestAuth {
			http.Error(w, "guest auth disabled", http.StatusForbidden)
			return
		}

		// 1) Try to reuse existing guest from cookie
		if c, err := r.Cookie("me_guest_id"); err == nil && c.Value != "" {
			var username, role string
			err := db.QueryRow(`SELECT username, role FROM users WHERE id=$1`, c.Value).Scan(&username, &role)
			if err == nil && role == "student" && strings.HasPrefix(c.Value, "guest|") {
				tok, _ := a.IssueJWT(c.Value, role)
				// Refresh cookie TTL
				http.SetCookie(w, &http.Cookie{
					Name:     "me_guest_id",
					Value:    c.Value,
					Path:     "/",
					HttpOnly: true,
					Secure:   true,
					SameSite: http.SameSiteNoneMode,
					Expires:  time.Now().Add(30 * 24 * time.Hour),
				})
				_ = json.NewEncoder(w).Encode(out{AccessToken: tok, Username: username})
				return
			}
		}

		// 2) Create a new guest
		sfx := strconv.FormatInt(time.Now().UnixNano(), 36)
		userID := "guest|" + sfx
		username := "guest-" + sfx[len(sfx)-6:]
		role := "student"

		_, _ = db.Exec(`INSERT INTO users (id, username, role, created_at)
		                VALUES ($1,$2,$3,$4)`, userID, username, role, time.Now().Unix())

		tok, err := a.IssueJWT(userID, role)
		if err != nil {
			http.Error(w, "issue token", http.StatusInternalServerError)
			return
		}
		// Persist guest identity for this browser
		http.SetCookie(w, &http.Cookie{
			Name:     "me_guest_id",
			Value:    userID,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			Expires:  time.Now().Add(30 * 24 * time.Hour),
		})
		_ = json.NewEncoder(w).Encode(out{AccessToken: tok, Username: username})
	}
}
