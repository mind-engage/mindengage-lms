// internal/auth/google_oauth.go
package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	authmw "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"github.com/mind-engage/mindengage-lms/internal/config"
)

// /api/auth/google/login → redirect to Google OAuth
// internal/auth/google_oauth.go (excerpt)
func GoogleLoginHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Caller can pass their current page (e.g., /exam/, /teacher/, /admin/)
		next := r.URL.Query().Get("redirect")
		if next == "" && r.Referer() != "" {
			next = r.Referer()
		}
		if next == "" {
			base := strings.TrimRight(cfg.PublicURL, "/")
			if base == "" {
				base = "/"
			}
			next = base + "/"
		}

		// VERY simple origin check: only allow same-origin as PUBLIC_URL or localhost (dev)
		if u, err := url.Parse(next); err == nil {
			if base, err2 := url.Parse(cfg.PublicURL); err2 == nil && base.Host != "" {
				if !(u.Host == "" || (u.Scheme == base.Scheme && u.Host == base.Host) || strings.HasPrefix(u.Host, "localhost")) {
					http.Error(w, "bad redirect", http.StatusBadRequest)
					return
				}
			}
		}

		// Persist redirect + state in short-lived cookies
		state := fmt.Sprintf("s-%d", time.Now().UnixNano())
		http.SetCookie(w, &http.Cookie{
			Name:     "me_oauth_state",
			Value:    state,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			Expires:  time.Now().Add(10 * time.Minute),
		})
		http.SetCookie(w, &http.Cookie{
			Name:     "me_post_auth_redirect",
			Value:    url.QueryEscape(next),
			Path:     "/",
			HttpOnly: false,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			Expires:  time.Now().Add(10 * time.Minute),
		})

		// Build Google Auth URL
		q := url.Values{}
		q.Set("client_id", cfg.GoogleClientID)
		q.Set("redirect_uri", cfg.GoogleRedirectURI)
		q.Set("response_type", "code")
		q.Set("scope", "openid email profile")
		q.Set("access_type", "offline")
		q.Set("include_granted_scopes", "true")
		q.Set("state", state)
		if cfg.GoogleAllowedHD != "" {
			q.Set("hd", cfg.GoogleAllowedHD)
		}
		http.Redirect(w, r, "https://accounts.google.com/o/oauth2/v2/auth?"+q.Encode(), http.StatusFound)
	}
}

// /api/auth/google/callback → exchange code, verify id_token, upsert, mint internal JWT, set cookie
func GoogleCallbackHandler(a *authmw.AuthService, db *sql.DB, cfg config.Config) http.HandlerFunc {
	type tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		IdToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
	}
	type tokenInfo struct {
		Iss           string `json:"iss"`
		Aud           string `json:"aud"`
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified string `json:"email_verified"`
		Hd            string `json:"hd"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		Exp           string `json:"exp"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// 1) Validate state (TODO: dev stub)
		if r.URL.Query().Get("state") == "" {
			http.Error(w, "missing state", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		// 2) Exchange code for tokens
		form := url.Values{}
		form.Set("code", code)
		form.Set("client_id", cfg.GoogleClientID)
		form.Set("client_secret", cfg.GoogleClientSecret)
		form.Set("redirect_uri", cfg.GoogleRedirectURI)
		form.Set("grant_type", "authorization_code")

		resp, err := http.PostForm("https://oauth2.googleapis.com/token", form)
		if err != nil {
			http.Error(w, "token exchange error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		var tr tokenResp
		if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil || tr.IdToken == "" {
			http.Error(w, "bad token response", http.StatusBadGateway)
			return
		}

		// 3) Verify id_token via Google tokeninfo (simple server-side verification)
		// NOTE: For production, prefer verifying the JWT signature with Google's JWKS and checking nonce.
		tiResp, err := http.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(tr.IdToken))
		if err != nil {
			http.Error(w, "tokeninfo fetch error", http.StatusBadGateway)
			return
		}
		defer tiResp.Body.Close()
		var ti tokenInfo
		if err := json.NewDecoder(tiResp.Body).Decode(&ti); err != nil {
			http.Error(w, "tokeninfo parse error", http.StatusBadGateway)
			return
		}
		if ti.Aud != cfg.GoogleClientID {
			http.Error(w, "invalid aud", http.StatusUnauthorized)
			return
		}
		if ti.Iss != "accounts.google.com" && ti.Iss != "https://accounts.google.com" {
			http.Error(w, "invalid iss", http.StatusUnauthorized)
			return
		}
		if cfg.GoogleAllowedHD != "" && !strings.EqualFold(ti.Hd, cfg.GoogleAllowedHD) {
			http.Error(w, "unauthorized domain", http.StatusUnauthorized)
			return
		}

		// 4) Determine role (default student; elevate if user exists in DB with role, or add your own rule)
		role := "student"
		username := ti.Email
		userID := "google|" + ti.Sub

		if db != nil {
			// upsert user; keep existing role if present
			var existingID, existingRole string
			err := db.QueryRow(`SELECT id, role FROM users WHERE username=$1`, username).Scan(&existingID, &existingRole)
			switch {
			case err == sql.ErrNoRows:
				_, _ = db.Exec(`INSERT INTO users (id, username, role) VALUES ($1, $2, $3)`, userID, username, role)
			case err == nil:
				if existingRole != "" {
					role = existingRole
				}
				userID = existingID
			default:
				// on DB error, continue; RBAC may fallback if configured
			}
		}

		// 5) Mint your internal JWT and set cookie (same as LTI)
		tok, err := a.IssueJWT(userID, role)
		if err != nil {
			http.Error(w, "issue token", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "me_access_token",
			Value:    tok,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteNoneMode,
			Expires:  time.Now().Add(8 * time.Hour),
		})

		// --- pick target from cookie (set at /auth/google/login) ---
		target := ""
		if c, err := r.Cookie("me_post_auth_redirect"); err == nil {
			if raw, _ := url.QueryUnescape(c.Value); raw != "" {
				target = raw
			}
		}
		if target == "" {
			target = cfg.PublicURL
			if target == "" {
				target = "/"
			}
		}

		// Optional: validate same-origin again (defense-in-depth)
		if u, err := url.Parse(target); err == nil {
			if base, err2 := url.Parse(cfg.PublicURL); err2 == nil && base.Host != "" {
				if !(u.Host == "" || (u.Scheme == base.Scheme && u.Host == base.Host) || strings.HasPrefix(u.Host, "localhost")) {
					target = strings.TrimRight(cfg.PublicURL, "/") + "/"
				}
			}
		}

		// Clean up cookies
		http.SetCookie(w, &http.Cookie{Name: "me_oauth_state", Value: "", Path: "/", Expires: time.Unix(0, 0), MaxAge: -1})
		http.SetCookie(w, &http.Cookie{Name: "me_post_auth_redirect", Value: "", Path: "/", Expires: time.Unix(0, 0), MaxAge: -1})

		// --- append ?access_token= so the SPA can store it (your app already parses it) ---
		u, _ := url.Parse(target)
		q := u.Query()
		q.Set("access_token", tok)
		u.RawQuery = q.Encode()

		http.Redirect(w, r, u.String(), http.StatusFound)
	}
}
