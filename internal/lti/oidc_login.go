// internal/lti/oidc_login.go
package lti

import (
	"net/http"
	"net/url"

	"github.com/mind-engage/mindengage-lms/internal/config"
)

// Accepts login initiation and bounces to platform auth endpoint with state/nonce placeholders.
func OIDCLoginHandler(platformAuthURL string) http.HandlerFunc {
	cfg := config.FromEnv()
	if platformAuthURL == "" {
		platformAuthURL = cfg.LTIPlatformAuthURL
	}
	clientID := cfg.LTIToolClientID
	redirectURI := cfg.LTIToolRedirectURI

	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: validate iss/login_hint/target_link_uri, generate state/nonce, persist state
		q := url.Values{}
		q.Set("response_type", "id_token")
		q.Set("response_mode", "form_post")
		q.Set("client_id", clientID)       // from config/env
		q.Set("redirect_uri", redirectURI) // from config/env (e.g., PUBLIC_URL + /api/lti/launch)
		q.Set("scope", "openid")
		q.Set("state", "dev-state")
		q.Set("nonce", "dev-nonce")
		http.Redirect(w, r, platformAuthURL+"?"+q.Encode(), http.StatusFound)
	}
}
