// pkg/platform/lti/token.go
package lti

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

/*
OAuth 2.0 Token Endpoint for the MindEngage LTI Platform

Implements a minimal OAuth 2.0 token endpoint used by LTI Tools to obtain
Bearer access tokens for calling Platform services (AGS, NRPS, etc).

Supported:
  - grant_type=client_credentials
  - client authentication:
      • client_secret_post
      • private_key_jwt  (RS256; JWKS must be registered for the Tool)

Issued tokens are JWTs signed by the Platform Signer. Claims include:
  iss (platform issuer), sub (client_id), aud (token endpoint URL),
  iat, exp, jti, tenant (custom), client_id (custom), scope (space string)

You must provide:
  - ResolveTenantID: map request -> tenant id
  - Issuers:         map tenant id -> issuer URL (absolute http(s))
  - Registry:        lookup client info (secret/JWKS/allowed scopes)
  - Signer:          sign JWTs (use KeyManager from keys.go)
  - Replay (opt):    protect client_assertion jti replays (recommended)

Mount:
    ts := &lti.TokenServer{...}
    r.Post("/oauth/token", ts.Handler())

Error responses use RFC 6749 fields: {"error":"...", "error_description":"..."}.
*/

const (
	errInvalidRequest          = "invalid_request"
	errInvalidClient           = "invalid_client"
	errUnauthorizedClient      = "unauthorized_client"
	errUnsupportedGrantType    = "unsupported_grant_type"
	errInvalidScope            = "invalid_scope"
	assertionTypePrivateKeyJWT = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)

// ---------------------- Registry (clients) -----------------------------------

// OAuthClient describes a registered Tool's OAuth client.
type OAuthClient struct {
	ClientID string

	// Either (or both) of these must be configured:
	// - SecretHash for client_secret_post (bcrypt or plain for dev)
	// - JWKS for private_key_jwt (RSA public keys; "kid" optional)
	SecretHash string
	JWKS       JWKS

	// AllowedScopes restricts scope grants; empty means "any known LTI scopes".
	AllowedScopes []string
}

// OAuthClientRegistry looks up the client by tenant+client_id.
type OAuthClientRegistry interface {
	GetOAuthClient(ctx context.Context, tenantID, clientID string) (OAuthClient, error)
}

// ---------------------- Token server -----------------------------------------

type TokenServer struct {
	ResolveTenantID func(*http.Request) (string, error)
	Issuers         IssuerResolver
	Registry        OAuthClientRegistry
	Signer          Signer

	// Optional replay protection for private_key_jwt (recommended)
	Replay Replay

	// Optional knobs
	AccessTokenTTL time.Duration // default 3600s
	Now            func() time.Time
	// If your platform sits behind a proxy prefix (e.g., "/api")
	ExternalBasePath string
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

// Handler returns http.HandlerFunc for POST /oauth/token
func (s *TokenServer) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeOAuthError(w, http.StatusMethodNotAllowed, errInvalidRequest, "use POST")
			return
		}
		if s.ResolveTenantID == nil || s.Issuers == nil || s.Registry == nil || s.Signer == nil {
			writeOAuthError(w, http.StatusInternalServerError, errInvalidRequest, "server not configured")
			return
		}

		if ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))); !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			writeOAuthError(w, http.StatusBadRequest, errInvalidRequest, "content-type must be application/x-www-form-urlencoded")
			return
		}
		if err := r.ParseForm(); err != nil {
			writeOAuthError(w, http.StatusBadRequest, errInvalidRequest, "bad form")
			return
		}

		tenantID, err := s.ResolveTenantID(r)
		if err != nil || strings.TrimSpace(tenantID) == "" {
			writeOAuthError(w, http.StatusBadRequest, errInvalidRequest, "unable to resolve tenant")
			return
		}
		issuer, err := s.Issuers.IssuerForTenant(r.Context(), tenantID)
		if err != nil || !isHTTPURL(issuer) {
			writeOAuthError(w, http.StatusInternalServerError, errInvalidRequest, "issuer resolution failed")
			return
		}

		grant := r.PostFormValue("grant_type")
		if grant != "client_credentials" {
			writeOAuthError(w, http.StatusBadRequest, errUnsupportedGrantType, "only client_credentials supported")
			return
		}

		// Client Authentication
		clientID := strings.TrimSpace(r.PostFormValue("client_id"))
		clientAssertionType := strings.TrimSpace(r.PostFormValue("client_assertion_type"))
		clientAssertion := strings.TrimSpace(r.PostFormValue("client_assertion"))
		clientSecret := r.PostFormValue("client_secret")

		switch {
		case clientAssertionType == assertionTypePrivateKeyJWT && clientAssertion != "":
			// ok; client_id may be in assertion (iss/sub)
		case clientSecret != "" && clientID != "":
			// ok; client_secret_post
		default:
			writeOAuthError(w, http.StatusUnauthorized, errInvalidClient, "missing client authentication")
			return
		}

		// Determine effective client_id (for private_key_jwt, prefer assertion's iss/sub)
		if clientAssertion != "" {
			iss, sub, err := peekJWTIssuerSubject(clientAssertion)
			if err != nil {
				writeOAuthError(w, http.StatusUnauthorized, errInvalidClient, "invalid client_assertion")
				return
			}
			// Per spec, iss==sub==client_id
			if iss == "" || sub == "" || iss != sub {
				writeOAuthError(w, http.StatusUnauthorized, errInvalidClient, "invalid client_assertion iss/sub")
				return
			}
			if clientID == "" {
				clientID = iss
			} else if clientID != iss {
				writeOAuthError(w, http.StatusUnauthorized, errInvalidClient, "client_id mismatch")
				return
			}
		}
		if clientID == "" {
			writeOAuthError(w, http.StatusUnauthorized, errInvalidClient, "client_id required")
			return
		}

		// Lookup client
		client, err := s.Registry.GetOAuthClient(r.Context(), tenantID, clientID)
		if err != nil {
			writeOAuthError(w, http.StatusUnauthorized, errInvalidClient, "unknown client")
			return
		}

		// Authenticate either by secret_post or private_key_jwt
		tokenURL := s.absoluteTokenURL(r)
		switch {
		case clientAssertion != "":
			if err := s.verifyClientAssertion(r.Context(), client, clientAssertion, tokenURL); err != nil {
				writeOAuthError(w, http.StatusUnauthorized, errInvalidClient, err.Error())
				return
			}
		case clientSecret != "":
			if err := verifySecret(client.SecretHash, clientSecret); err != nil {
				writeOAuthError(w, http.StatusUnauthorized, errInvalidClient, "invalid client_secret")
				return
			}
		default:
			writeOAuthError(w, http.StatusUnauthorized, errInvalidClient, "unsupported client authentication")
			return
		}

		// Scope negotiation: intersection(requested, allowed)
		requested := parseScopes(r.PostFormValue("scope"))
		granted := intersectScopes(requested, client.AllowedScopes)
		if len(granted) == 0 && len(requested) > 0 {
			writeOAuthError(w, http.StatusBadRequest, errInvalidScope, "requested scopes not allowed")
			return
		}
		// If no scopes requested, grant all allowed (or a safe default set)
		if len(granted) == 0 {
			if len(client.AllowedScopes) > 0 {
				granted = uniqueScopes(client.AllowedScopes)
			} else {
				// very permissive fallback (platform may narrow later via route-level checks)
				granted = []string{
					"https://purl.imsglobal.org/spec/lti-ags/scope/lineitem",
					"https://purl.imsglobal.org/spec/lti-ags/scope/lineitem.readonly",
					"https://purl.imsglobal.org/spec/lti-ags/scope/score",
					"https://purl.imsglobal.org/spec/lti-ags/scope/result.readonly",
					"https://purl.imsglobal.org/spec/lti-nrps/scope/contextmembership.readonly",
				}
			}
		}

		now := s.now()
		exp := now.Add(s.ttl())

		claims := map[string]any{
			"iss":       issuer,
			"sub":       clientID,
			"aud":       tokenURL,
			"iat":       now.Unix(),
			"exp":       exp.Unix(),
			"jti":       randHex(20),
			"tenant":    tenantID,
			"client_id": clientID,
			"scope":     strings.Join(granted, " "),
			"typ":       "access",
		}

		jwt, err := s.Signer.Sign(r.Context(), tenantID, claims)
		if err != nil {
			writeOAuthError(w, http.StatusInternalServerError, errInvalidRequest, "signing failed")
			return
		}

		resp := tokenResponse{
			AccessToken: jwt,
			TokenType:   "Bearer",
			ExpiresIn:   int64(s.ttl().Seconds()),
			Scope:       strings.Join(granted, " "),
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

/* ---------------------- client_secret_post verification -------------------- */

func verifySecret(storedHash, provided string) error {
	// Accept either bcrypt hash (prefix "$2") or raw equality (dev only).
	stored := strings.TrimSpace(storedHash)
	if stored == "" {
		return errors.New("no client_secret configured")
	}
	// bcrypt hash?
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(provided))
	}
	// constant-time compare for dev/plain
	if subtle.ConstantTimeCompare([]byte(stored), []byte(provided)) != 1 {
		return errors.New("secret mismatch")
	}
	return nil
}

/* ---------------------- private_key_jwt verification ----------------------- */

func (s *TokenServer) verifyClientAssertion(_ context.Context, client OAuthClient, assertion string, audience string) error {
	// Parse JWT (header.payload.signature)
	hdr, payload, sig, err := splitJWT(assertion)
	if err != nil {
		return errors.New("malformed client_assertion")
	}
	var h struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		KID string `json:"kid"`
	}
	if err := json.Unmarshal(hdr, &h); err != nil {
		return errors.New("invalid JWT header")
	}
	if h.Alg != "RS256" {
		return errors.New("unsupported alg (only RS256)")
	}

	var c struct {
		Iss string      `json:"iss"`
		Sub string      `json:"sub"`
		Aud interface{} `json:"aud"`
		Exp int64       `json:"exp"`
		Iat int64       `json:"iat"`
		Nbf int64       `json:"nbf,omitempty"`
		JTI string      `json:"jti"`
	}
	if err := json.Unmarshal(payload, &c); err != nil {
		return errors.New("invalid JWT claims")
	}

	// iss==sub==client_id
	if c.Iss == "" || c.Sub == "" || c.Iss != c.Sub || c.Sub != client.ClientID {
		return errors.New("iss/sub mismatch")
	}
	// aud must contain the token endpoint URL
	if !audContains(c.Aud, audience) {
		return errors.New("aud mismatch")
	}
	// exp / iat / nbf windows
	now := time.Now().UTC()
	if c.Exp == 0 || now.Unix() > c.Exp {
		return errors.New("assertion expired")
	}
	if c.Iat > 0 && (now.Unix()-c.Iat) > 600 {
		return errors.New("assertion too old")
	}
	if c.Nbf > 0 && now.Unix() < c.Nbf {
		return errors.New("assertion not yet valid")
	}
	// jti replay (15m TTL)
	if s.Replay != nil && strings.TrimSpace(c.JTI) != "" {
		if ok, _ := s.Replay.Use("pkjwt_jti:"+client.ClientID, c.JTI, 15*time.Minute); !ok {
			return errors.New("assertion replay detected")
		}
	}

	// Verify signature using client's JWKS
	if len(client.JWKS.Keys) == 0 {
		return errors.New("no client keys registered")
	}
	toVerify := signingInput(assertion) // "base64url(header).base64url(payload)"
	sum := sha256.Sum256([]byte(toVerify))

	pubKeys, err := rsaPublicKeysFromJWKS(client.JWKS, h.KID)
	if err != nil {
		return err
	}
	for _, pk := range pubKeys {
		if err := rsa.VerifyPKCS1v15(pk, crypto.SHA256, sum[:], sig); err == nil {
			return nil // verified by this key
		}
	}
	return errors.New("signature verification failed")
}

/* ---------------------- helpers: JWT, JWKS, scopes ------------------------- */

func signingInput(jwt string) string {
	// split into 3 parts and return "header.payload" portion
	i := strings.LastIndexByte(jwt, '.')
	if i <= 0 {
		return ""
	}
	return jwt[:i]
}

func splitJWT(jwt string) (hdr, payload, sig []byte, err error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return nil, nil, nil, errors.New("want 3 parts")
	}
	h, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, nil, err
	}
	p, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, nil, err
	}
	s, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, nil, err
	}
	return h, p, s, nil
}

func peekJWTIssuerSubject(jwt string) (iss, sub string, err error) {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return "", "", errors.New("malformed")
	}
	p, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", err
	}
	var c struct {
		Iss string `json:"iss"`
		Sub string `json:"sub"`
	}
	if e := json.Unmarshal(p, &c); e != nil {
		return "", "", e
	}
	return c.Iss, c.Sub, nil
}

func audContains(aud interface{}, want string) bool {
	switch v := aud.(type) {
	case string:
		return strings.TrimSpace(v) == want
	case []any:
		for _, it := range v {
			if s, ok := it.(string); ok && strings.TrimSpace(s) == want {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) == want {
				return true
			}
		}
	}
	return false
}

func rsaPublicKeysFromJWKS(set JWKS, kid string) ([]*rsa.PublicKey, error) {
	var out []*rsa.PublicKey
	for _, k := range set.Keys {
		if k == nil {
			continue
		}
		if t, _ := k["kty"].(string); t != "RSA" {
			continue
		}
		if kid != "" {
			if got, _ := k["kid"].(string); got != kid {
				continue
			}
		}
		nStr, _ := k["n"].(string)
		eStr, _ := k["e"].(string)
		if nStr == "" || eStr == "" {
			continue
		}
		nb, err := base64.RawURLEncoding.DecodeString(nStr)
		if err != nil {
			continue
		}
		eb, err := base64.RawURLEncoding.DecodeString(eStr)
		if err != nil {
			continue
		}
		n := new(big.Int).SetBytes(nb)
		e := 0
		for _, b := range eb {
			e = (e << 8) | int(b)
		}
		if e == 0 {
			continue
		}
		out = append(out, &rsa.PublicKey{N: n, E: e})
	}
	if len(out) == 0 {
		if kid != "" {
			return nil, fmt.Errorf("no RSA key with kid %q", kid)
		}
		return nil, errors.New("no RSA keys in client JWKS")
	}
	// If kid was not specified, we just return all RSA keys and try each.
	return out, nil
}

// scope helpers

func parseScopes(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return uniqueScopes(strings.Fields(s))
}

func uniqueScopes(in []string) []string {
	set := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			set[s] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func intersectScopes(requested, allowed []string) []string {
	if len(requested) == 0 {
		return nil
	}
	if len(allowed) == 0 {
		// no policy configured -> allow all requested
		return uniqueScopes(requested)
	}
	allow := map[string]struct{}{}
	for _, s := range allowed {
		allow[strings.TrimSpace(s)] = struct{}{}
	}
	var out []string
	for _, s := range requested {
		if _, ok := allow[strings.TrimSpace(s)]; ok {
			out = append(out, s)
		}
	}
	return uniqueScopes(out)
}

/* ---------------------- misc utils ----------------------------------------- */

func (s *TokenServer) ttl() time.Duration {
	if s.AccessTokenTTL > 0 {
		return s.AccessTokenTTL
	}
	return time.Hour
}

func (s *TokenServer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *TokenServer) absoluteTokenURL(r *http.Request) string {
	scheme := schemeFromRequest(r)
	host := hostWithoutPort(r.Host)
	base := strings.TrimSuffix(s.ExternalBasePath, "/")
	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   base + "/oauth/token",
	}
	return u.String()
}

type oauthErr struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(oauthErr{Error: code, Description: desc})
}
