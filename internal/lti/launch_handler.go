// internal/lti/launch_handler.go
package lti

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	auth "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"github.com/mind-engage/mindengage-lms/internal/config"
)

// LTIClaims embeds RegisteredClaims so it satisfies jwt.Claims in v5.
type LTIClaims struct {
	jwt.RegisteredClaims
	Email string   `json:"email"`
	Name  string   `json:"name"`
	Roles []string `json:"https://purl.imsglobal.org/spec/lti/claim/roles"`
}

// Receives id_token POST, extracts user & role, upserts DB user, and mints internal JWT.
// NOTE: Signature/claims verification is still TODO; this parses the token without verification
// for dev purposes. In production, verify with Platform issuer and JWKS.
func LaunchHandler(a *auth.AuthService, db *sql.DB, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		idtok := r.PostFormValue("id_token")
		if idtok == "" {
			http.Error(w, "missing id_token", http.StatusBadRequest)
			return
		}

		// DEV ONLY: parse without verifying
		var claims LTIClaims
		parser := jwt.NewParser()
		if _, _, err := parser.ParseUnverified(idtok, &claims); err != nil {
			http.Error(w, "bad id_token", http.StatusBadRequest)
			return
		}

		// Map LTI roles -> internal role
		role := "student"
		for _, r := range claims.Roles {
			lr := strings.ToLower(r)
			if strings.Contains(lr, "instructor") || strings.Contains(lr, "teacher") ||
				strings.Contains(lr, "faculty") || strings.Contains(lr, "administrator") {
				role = "teacher"
				break
			}
		}

		// Choose username and userID
		username := claims.Email
		if username == "" {
			username = claims.Subject
		}
		if username == "" {
			http.Error(w, "invalid claims: missing sub/email", http.StatusBadRequest)
			return
		}
		userID := claims.Subject
		if claims.Issuer != "" && claims.Subject != "" {
			userID = claims.Issuer + "|" + claims.Subject
		}

		// Upsert user for DB-backed role resolution via auth.AttachRoleFromDB
		if db != nil {
			var existingID string
			err := db.QueryRow(`SELECT id FROM users WHERE username=$1`, username).Scan(&existingID)
			switch {
			case err == sql.ErrNoRows:
				_, _ = db.Exec(`INSERT INTO users (id, username, role) VALUES ($1, $2, $3)`, userID, username, role)
			case err == nil:
				_, _ = db.Exec(`UPDATE users SET role=$1 WHERE id=$2`, role, existingID)
				userID = existingID
			default:
				// On DB error, continue; RBAC may rely on claims fallback depending on config.
			}
		}

		// Mint internal JWT for our API
		tok, err := a.IssueJWT(userID, role)
		if err != nil {
			http.Error(w, "issue token", http.StatusInternalServerError)
			return
		}

		// Send token as HttpOnly cookie and redirect to SPA
		http.SetCookie(w, &http.Cookie{
			Name:     "me_access_token",
			Value:    tok,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
		})

		target := cfg.PublicURL
		if target == "" {
			target = "/"
		}
		target = strings.TrimRight(target, "/") + "/exam/"
		http.Redirect(w, r, target, http.StatusFound)
	}
}
