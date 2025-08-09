package lti

import (
	"fmt"
	"net/http"
)

// Receives id_token POST. Dev-only: accept and show 200.
func LaunchHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", 400)
			return
		}
		idtok := r.PostFormValue("id_token")
		if idtok == "" {
			http.Error(w, "missing id_token", 400)
			return
		}
		// TODO: verify signature using platform JWKS, validate claims, mint local session
		fmt.Fprintln(w, "LTI launch received (dev stub)")
	}
}
