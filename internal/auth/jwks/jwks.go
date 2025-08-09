package jwks

import (
	"encoding/json"
	"net/http"
)

type JWK struct {
	Kty string `json:"kty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
	Use string `json:"use,omitempty"`
	Kid string `json:"kid,omitempty"`
	Alg string `json:"alg,omitempty"`
}
type JWKS struct {
	Keys []JWK `json:"keys"`
}

func Handler(static JWKS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(static)
	}
}
