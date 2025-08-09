package lti

import (
	"net/http"
	"net/url"
)

// Accepts login initiation and bounces to platform auth endpoint with state/nonce placeholders.
func OIDCLoginHandler(platformAuthURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: validate iss/login_hint/target_link_uri, generate state/nonce, persist state
		q := url.Values{}
		q.Set("response_type", "id_token")
		q.Set("response_mode", "form_post")
		q.Set("client_id", "TOOL_CLIENT_ID")                      // TODO: load from registry
		q.Set("redirect_uri", "https://your-lms-host/lti/launch") // TODO: cfg.PublicURL + /lti/launch
		q.Set("scope", "openid")
		q.Set("state", "dev-state")
		q.Set("nonce", "dev-nonce")
		http.Redirect(w, r, platformAuthURL+"?"+q.Encode(), http.StatusFound)
	}
}
