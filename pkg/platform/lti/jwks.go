// pkg/platform/lti/jwks.go
package lti

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

/*
JWKS endpoint (Platform side)

This file exposes a small, dependency-free HTTP handler that serves a tenant’s
public keys in JWKS (RFC 7517) format. Tools fetch this at
  https://<tenant-issuer>/.well-known/jwks.json
to verify the Platform’s LTI id_tokens.

You provide:
  - ResolveTenantID: how to map the request to a tenant (host/path-based, etc.)
  - Provider: how to load that tenant’s current key set

It also includes helpers to build RSA/EC JWKs from Go public keys, and a
StaticJWKS provider useful for tests and single-tenant setups.
*/

// JWKS is a JSON Web Key Set, i.e. { "keys": [ JWK, ... ] }.
type JWKS struct {
	Keys []map[string]any `json:"keys"`
}

// JWKSProvider loads the public key set for a tenant.
type JWKSProvider interface {
	// PublicJWKS returns only public material; NEVER return private fields.
	PublicJWKS(ctx context.Context, tenantID string) (JWKS, error)
}

// JWKSHandler serves /.well-known/jwks.json for the Platform.
type JWKSHandler struct {
	// ResolveTenantID returns the tenant identifier for this request.
	ResolveTenantID func(*http.Request) (string, error)
	// Provider returns the tenant’s JWKS.
	Provider JWKSProvider

	// Optional: cache control for responses (default: 10 minutes).
	CacheMaxAge time.Duration
	// Optional: if true, adds Access-Control-Allow-Origin: * (many tools fetch
	// JWKS server-side and don’t need CORS, but enabling doesn’t hurt).
	AllowCORS bool
	// Optional: override the clock (useful in tests).
	Now func() time.Time
}

// ServeHTTP implements http.Handler for the JWKS endpoint.
//
// Mount it like:
//
//	r := chi.NewRouter()
//	r.Get("/.well-known/jwks.json", jwksHandler.ServeHTTP)
func (h *JWKSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.ResolveTenantID == nil || h.Provider == nil {
		http.Error(w, "jwks: not configured", http.StatusInternalServerError)
		return
	}
	tenantID, err := h.ResolveTenantID(r)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		http.Error(w, "jwks: unable to resolve tenant", http.StatusBadRequest)
		return
	}
	set, err := h.Provider.PublicJWKS(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "jwks: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Empty key set is unusual but allowed; return 200 with {"keys":[]}
	if set.Keys == nil {
		set.Keys = []map[string]any{}
	}

	// Marshal once to compute ETag and to write the body.
	payload, err := json.Marshal(set)
	if err != nil {
		http.Error(w, "jwks: marshal error", http.StatusInternalServerError)
		return
	}

	// Caching headers
	now := h.now()
	maxAge := int(h.cacheAge().Seconds())
	etag := computeETag(payload)
	w.Header().Set("Content-Type", "application/jwk-set+json")
	w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAge))
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", now.UTC().Format(http.TimeFormat))
	if h.AllowCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	// Conditional GET
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// HEAD support
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Body
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (h *JWKSHandler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *JWKSHandler) cacheAge() time.Duration {
	if h.CacheMaxAge > 0 {
		return h.CacheMaxAge
	}
	return 10 * time.Minute
}

func computeETag(b []byte) string {
	sum := sha256.Sum256(b)
	// weak ETag is fine here
	return `W/"` + b64url(sum[:]) + `"`
}

// ------------------------------------------------------------------------------------
// Helpers to construct public JWKs from *rsa.PublicKey / *ecdsa.PublicKey.
// These return only PUBLIC parameters as per RFC 7517 and set typical metadata:
//   - "use": "sig"
//   - "key_ops": ["verify"]
// Caller should provide a stable "kid" and an "alg" (e.g., "RS256", "ES256").
// ------------------------------------------------------------------------------------

// RSAPublicJWK builds a minimal RSA JWK map (n,e) for the given key.
func RSAPublicJWK(pub *rsa.PublicKey, kid, alg string) map[string]any {
	if pub == nil || pub.N == nil || pub.E == 0 {
		return nil
	}
	return map[string]any{
		"kty":     "RSA",
		"kid":     kid,
		"alg":     alg,
		"use":     "sig",
		"key_ops": []string{"verify"},
		"n":       bigIntToB64(pub.N),
		"e":       intToB64(pub.E),
	}
}

// ECPublicJWK builds a minimal EC JWK map (crv,x,y) for the given key.
// The caller is responsible for the right "alg" according to curve:
//   - P-256 => ES256, P-384 => ES384, P-521 => ES512
func ECPublicJWK(pub *ecdsa.PublicKey, kid, alg string) map[string]any {
	if pub == nil || pub.X == nil || pub.Y == nil || pub.Curve == nil {
		return nil
	}
	crv := curveName(pub)
	if crv == "" {
		return nil
	}
	return map[string]any{
		"kty":     "EC",
		"kid":     kid,
		"alg":     alg,
		"use":     "sig",
		"key_ops": []string{"verify"},
		"crv":     crv,
		"x":       bigIntToB64(pub.X),
		"y":       bigIntToB64(pub.Y),
	}
}

func curveName(pk *ecdsa.PublicKey) string {
	switch pk.Curve.Params().Name {
	case "P-256", "prime256v1", "secp256r1":
		return "P-256"
	case "P-384", "secp384r1":
		return "P-384"
	case "P-521", "secp521r1":
		return "P-521"
	default:
		return ""
	}
}

func bigIntToB64(n *big.Int) string {
	if n == nil {
		return ""
	}
	return b64url(n.FillBytes(make([]byte, (n.BitLen()+7)/8)))
}

func intToB64(e int) string {
	return b64url(big.NewInt(int64(e)).FillBytes(make([]byte, intByteLen(e))))
}

func intByteLen(v int) int {
	switch {
	case v <= 0xff:
		return 1
	case v <= 0xffff:
		return 2
	case v <= 0xffffff:
		return 3
	case v <= 0xffffffff:
		return 4
	default:
		// large public exponent is extremely rare; adapt as needed
		return 8
	}
}

func stripLeadingZeros(b []byte) []byte {
	for len(b) > 0 && b[0] == 0x00 {
		b = b[1:]
	}
	return b
}

// ------------------------------------------------------------------------------------
// Static / test provider implementations
// ------------------------------------------------------------------------------------

// StaticJWKS is a simple in-memory JWKSProvider (useful for tests and dev).
type StaticJWKS struct {
	Set JWKS
}

func (p StaticJWKS) PublicJWKS(_ context.Context, _ string) (JWKS, error) {
	if p.Set.Keys == nil {
		return JWKS{Keys: []map[string]any{}}, nil
	}
	return p.Set, nil
}

// MapJWKSProvider lets you configure a per-tenant JWKS map.
//
//	prov := MapJWKSProvider{"school-a": jwksA, "school-b": jwksB}
type MapJWKSProvider map[string]JWKS

func (m MapJWKSProvider) PublicJWKS(_ context.Context, tenantID string) (JWKS, error) {
	set, ok := m[tenantID]
	if !ok {
		return JWKS{}, errors.New("jwks: tenant not found")
	}
	return set, nil
}
