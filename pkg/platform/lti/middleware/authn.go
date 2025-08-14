// pkg/platform/lti/middleware/authn.go
package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

/*
Package middleware provides HTTP middleware for authenticating and authorizing
requests to the MindEngage LTI Platform services (AGS, NRPS, etc.).

It expects callers (Tools) to present an OAuth 2.0 Bearer access token in the
"Authorization" header, minted by the Platform’s /oauth/token endpoint.

You need to provide a TokenVerifier implementation that validates a raw token
string and returns normalized AccessClaims (tenant, client, scopes, expiry).

Typical wiring:

    // in your server bootstrap
    authMW := middleware.BearerAuthWithOptions(verifier, middleware.AuthOptions{
        Realm: "LTI Platform",
        // Optionally enforce that the token tenant matches the tenant resolved from host/path:
        EnforceTenantMatch:  true,
        ResolveTenantID:     yourTenantResolverFunc,
    })

    r := chi.NewRouter()
    r.Route("/api/lti/ags", func(ags chi.Router) {
        ags.Use(authMW)
        ags.With(middleware.RequireAnyScope(middleware.ScopeLineItem, middleware.ScopeLineItemRead)).
            Get("/contexts/{contextId}/line_items", srv.GetLineItems)
        ags.With(middleware.RequireAllScopes(middleware.ScopeLineItem)).
            Post("/contexts/{contextId}/line_items", srv.PostLineItem)
        ags.With(middleware.RequireAllScopes(middleware.ScopeLineItem)).
            Put("/line_items/{id}", srv.PutLineItem)
        ags.With(middleware.RequireAllScopes(middleware.ScopeLineItem)).
            Delete("/line_items/{id}", srv.DeleteLineItem)
        ags.With(middleware.RequireAllScopes(middleware.ScopeScore)).
            Post("/line_items/{id}/scores", srv.PostScore)
        ags.With(middleware.RequireAnyScope(middleware.ScopeResultRead)).
            Get("/line_items/{id}/results", srv.GetResults)
    })

Downstream handlers can read claims via middleware.FromContext(ctx).
*/

// --- Common IMS scopes (helpers for callers) --------------------------------

const (
	// AGS (Assignment & Grade Service)
	ScopeLineItem     = "https://purl.imsglobal.org/spec/lti-ags/scope/lineitem"
	ScopeLineItemRead = "https://purl.imsglobal.org/spec/lti-ags/scope/lineitem.readonly"
	ScopeScore        = "https://purl.imsglobal.org/spec/lti-ags/scope/score"
	ScopeResultRead   = "https://purl.imsglobal.org/spec/lti-ags/scope/result.readonly"

	// NRPS (Names & Role Provisioning Service)
	ScopeNRPSContextMembershipRead = "https://purl.imsglobal.org/spec/lti-nrps/scope/contextmembership.readonly"
)

// --- Claims model & verifier interface --------------------------------------

type AccessClaims struct {
	// Token subject (usually the tool client_id, but verifier decides)
	Subject string

	// Tenant (school) that the token was issued for. Required for multi-tenant platforms.
	TenantID string

	// The registered tool client_id this token was issued to/for.
	ClientID string

	// Granted scopes. Either copied from token "scope" or derived by verifier.
	Scopes []string

	// Optional time-based fields, useful for logging/metrics.
	IssuedAt  time.Time
	ExpiresAt time.Time

	// Opaque token id (jti) if available – useful for replay tracking.
	JTI string
}

// TokenVerifier validates incoming OAuth2 Bearer tokens and returns claims.
// It should validate signature / introspect, "audience", expiration, etc.
type TokenVerifier interface {
	VerifyAccessToken(ctx context.Context, rawToken string) (AccessClaims, error)
}

// --- Middleware options & constructor ---------------------------------------

type AuthOptions struct {
	// Realm string used in WWW-Authenticate header on 401s.
	Realm string

	// If true, the request tenant (from ResolveTenantID) must match claims.TenantID.
	// Useful when you route tenants using host/path and want strict alignment.
	EnforceTenantMatch bool

	// ResolveTenantID determines the tenant implied by the request URL (host/path).
	// Only used when EnforceTenantMatch is true.
	ResolveTenantID func(*http.Request) (string, error)
}

// BearerAuth returns middleware that:
//   - extracts and verifies a Bearer token
//   - attaches AccessClaims to the request context
//   - optionally enforces tenant match (see AuthOptions)
func BearerAuth(verifier TokenVerifier) func(http.Handler) http.Handler {
	return BearerAuthWithOptions(verifier, AuthOptions{})
}

func BearerAuthWithOptions(verifier TokenVerifier, opts AuthOptions) func(http.Handler) http.Handler {
	realm := opts.Realm
	if realm == "" {
		realm = "protected"
	}
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			raw, ok := extractBearer(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w, realm, "invalid_request", "missing bearer token")
				return
			}
			claims, err := verifier.VerifyAccessToken(r.Context(), raw)
			if err != nil {
				unauthorized(w, realm, "invalid_token", err.Error())
				return
			}
			// Optional tenant alignment
			if opts.EnforceTenantMatch && opts.ResolveTenantID != nil {
				reqTenant, err := opts.ResolveTenantID(r)
				if err != nil || strings.TrimSpace(reqTenant) == "" {
					unauthorized(w, realm, "invalid_request", "unable to resolve tenant")
					return
				}
				if !tenantEqual(reqTenant, claims.TenantID) {
					forbidden(w, "tenant mismatch")
					return
				}
			}

			ctx := withClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}

// --- Authorization helpers (scopes) -----------------------------------------

// RequireAllScopes ensures ALL provided scopes are present in token claims.
func RequireAllScopes(required ...string) func(http.Handler) http.Handler {
	required = normScopes(required)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cl, ok := FromContext(r.Context())
			if !ok {
				forbidden(w, "missing auth context")
				return
			}
			if !hasAllScopes(cl.Scopes, required) {
				forbidden(w, fmt.Sprintf("missing required scopes: %s", strings.Join(missingScopes(cl.Scopes, required), " ")))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyScope ensures at least ONE of the provided scopes is present.
func RequireAnyScope(anyOf ...string) func(http.Handler) http.Handler {
	anyOf = normScopes(anyOf)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cl, ok := FromContext(r.Context())
			if !ok {
				forbidden(w, "missing auth context")
				return
			}
			if !hasAnyScope(cl.Scopes, anyOf) {
				forbidden(w, fmt.Sprintf("requires one of scopes: %s", strings.Join(anyOf, " ")))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func hasAllScopes(have, need []string) bool {
	hset := make(map[string]struct{}, len(have))
	for _, s := range have {
		hset[strings.TrimSpace(s)] = struct{}{}
	}
	for _, s := range need {
		if _, ok := hset[strings.TrimSpace(s)]; !ok {
			return false
		}
	}
	return true
}

func hasAnyScope(have, anyOf []string) bool {
	hset := make(map[string]struct{}, len(have))
	for _, s := range have {
		hset[strings.TrimSpace(s)] = struct{}{}
	}
	for _, s := range anyOf {
		if _, ok := hset[strings.TrimSpace(s)]; ok {
			return true
		}
	}
	return false
}

func missingScopes(have, need []string) []string {
	hset := make(map[string]struct{}, len(have))
	for _, s := range have {
		hset[strings.TrimSpace(s)] = struct{}{}
	}
	var miss []string
	for _, s := range need {
		if _, ok := hset[strings.TrimSpace(s)]; !ok {
			miss = append(miss, s)
		}
	}
	return miss
}

func normScopes(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, s := range xs {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// --- Context plumbing --------------------------------------------------------

type ctxKey int

const claimsKey ctxKey = 1

func withClaims(ctx context.Context, c AccessClaims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// FromContext extracts AccessClaims that BearerAuth placed in the context.
func FromContext(ctx context.Context) (AccessClaims, bool) {
	if ctx == nil {
		return AccessClaims{}, false
	}
	v := ctx.Value(claimsKey)
	if v == nil {
		return AccessClaims{}, false
	}
	c, ok := v.(AccessClaims)
	return c, ok
}

// TenantID helper returns claims.TenantID (empty if not present).
func TenantID(ctx context.Context) string {
	if c, ok := FromContext(ctx); ok {
		return c.TenantID
	}
	return ""
}

// ClientID helper returns claims.ClientID (empty if not present).
func ClientID(ctx context.Context) string {
	if c, ok := FromContext(ctx); ok {
		return c.ClientID
	}
	return ""
}

// --- Wire helpers (headers & responses) -------------------------------------

func extractBearer(hdr string) (string, bool) {
	if hdr == "" {
		return "", false
	}
	// Case-insensitive per RFC6750; allow extra spaces
	prefix := "bearer "
	if len(hdr) < len(prefix) {
		return "", false
	}
	if !strings.EqualFold(hdr[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(hdr[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

func unauthorized(w http.ResponseWriter, realm, code, desc string) {
	// RFC 6750: WWW-Authenticate: Bearer realm="...", error="invalid_token", error_description="..."
	w.Header().Set("WWW-Authenticate",
		fmt.Sprintf(`Bearer realm="%s", error="%s", error_description="%s"`, escape(realm), escape(code), escape(desc)))
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func forbidden(w http.ResponseWriter, msg string) {
	http.Error(w, "forbidden: "+msg, http.StatusForbidden)
}

func escape(s string) string {
	// very small sanitizer for header values
	s = strings.ReplaceAll(s, `"`, `'`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func tenantEqual(a, b string) bool {
	return strings.TrimSpace(strings.ToLower(a)) == strings.TrimSpace(strings.ToLower(b))
}

// --- Simple in-memory verifier (optional reference) -------------------------
// You can delete this if you wire a real verifier. It can be useful for tests.

var ErrInvalidToken = errors.New("invalid token")

// StaticTokenVerifier verifies tokens against an in-memory map.
// Map key = token string; value = claims.
type StaticTokenVerifier struct {
	// Tokens maps raw bearer token -> claims.
	Tokens map[string]AccessClaims
}

func (v *StaticTokenVerifier) VerifyAccessToken(_ context.Context, raw string) (AccessClaims, error) {
	if v == nil || v.Tokens == nil {
		return AccessClaims{}, ErrInvalidToken
	}
	claims, ok := v.Tokens[raw]
	if !ok {
		return AccessClaims{}, ErrInvalidToken
	}
	// Expiry check if provided
	if !claims.ExpiresAt.IsZero() && time.Now().After(claims.ExpiresAt) {
		return AccessClaims{}, ErrInvalidToken
	}
	return claims, nil
}
