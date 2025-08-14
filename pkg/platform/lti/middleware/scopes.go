// pkg/platform/lti/middleware/scopes.go
package middleware

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// RequireScopes enforces that the caller's access token includes ALL of the
// required scopes. It accepts either:
//
//  1. Scopes attached to the request context by a prior authn middleware via
//     WithScopesCtx / SetScopesOnContext, OR
//  2. Scopes decoded from the Bearer JWT's payload "scope" (space-separated)
//     or "scp" (array) claims (best-effort read; signature is assumed to be
//     validated by your authn middleware).
//
// Special-case: if a handler requires ".../lineitem.readonly", the presence of
// the write scope ".../lineitem" also satisfies it.
func RequireScopes(required ...string) func(http.Handler) http.Handler {
	req := uniqueScopes(required)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			have := haveScopesSet(r)
			if satisfiesAll(have, req) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "insufficient_scope",
				"required_scopes":   req,
				"granted_scopes":    setToSlice(have),
				"error_description": "caller does not have all required scopes",
			})
		})
	}
}

/* ---------------------------- Context helpers ---------------------------- */

// context key for scopes
type ctxScopesKey struct{}

// WithScopesCtx attaches scopes to the request context. Useful from your
// authn middleware after validating the access token.
func WithScopesCtx(scopes []string) func(http.Handler) http.Handler {
	ss := uniqueScopes(scopes)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxScopesKey{}, ss)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SetScopesOnContext is a utility if you prefer to set scopes inside an
// existing middleware without wrapping the handler.
func SetScopesOnContext(r *http.Request, scopes []string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxScopesKey{}, uniqueScopes(scopes))
	return r.WithContext(ctx)
}

/* ---------------------------- Internal logic ----------------------------- */

func haveScopesSet(r *http.Request) map[string]struct{} {
	// 1) Prefer scopes already on context (set by authn middleware)
	if v := r.Context().Value(ctxScopesKey{}); v != nil {
		if ss, ok := v.([]string); ok {
			return toSet(ss)
		}
	}

	// 2) Fallback: best-effort read from Bearer token payload (already validated upstream)
	// NOTE: This does NOT validate the signature; it's only used if authn
	// didn't already attach scopes.
	if tok := strings.TrimSpace(bearerToken(r.Header.Get("Authorization"))); tok != "" {
		if scopes := parseScopesFromJWT(tok); len(scopes) > 0 {
			return toSet(scopes)
		}
	}

	return map[string]struct{}{}
}

func satisfiesAll(have map[string]struct{}, required []string) bool {
	if len(required) == 0 {
		return true
	}
	for _, need := range required {
		if _, ok := have[need]; ok {
			continue
		}
		// Allow write scope to satisfy read-only for AGS lineitem
		if strings.HasSuffix(need, "/lineitem.readonly") {
			write := strings.TrimSuffix(need, ".readonly")
			if _, ok := have[write]; ok {
				continue
			}
		}
		return false
	}
	return true
}

func bearerToken(authorization string) string {
	if authorization == "" {
		return ""
	}
	parts := strings.Fields(authorization)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

func parseScopesFromJWT(jwt string) []string {
	// JWT is header.payload.signature (base64url). We only need payload.
	dot1 := strings.IndexByte(jwt, '.')
	dot2 := strings.LastIndexByte(jwt, '.')
	if dot1 <= 0 || dot2 <= dot1 {
		return nil
	}
	payloadB64 := jwt[dot1+1 : dot2]
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	// "scope": space-separated string
	if s, ok := claims["scope"].(string); ok && strings.TrimSpace(s) != "" {
		return uniqueScopes(strings.Fields(s))
	}
	// "scp": array of strings
	if arr, ok := claims["scp"].([]any); ok {
		var out []string
		for _, v := range arr {
			if str, ok := v.(string); ok {
				out = append(out, str)
			}
		}
		return uniqueScopes(out)
	}
	return nil
}

func uniqueScopes(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			set[s] = struct{}{}
		}
	}
	return setToSlice(set)
}

func toSet(in []string) map[string]struct{} {
	m := make(map[string]struct{}, len(in))
	for _, s := range in {
		m[s] = struct{}{}
	}
	return m
}

func setToSlice(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	return out
}
