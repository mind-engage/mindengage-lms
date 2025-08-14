// pkg/platform/lti/middleware/replay.go
package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

/*
Replay protection utilities and HTTP middleware.

This package provides a small, dependency-free in-memory replay cache and
helpers to protect endpoints that rely on single-use tokens/values such as:

  - OAuth/OIDC "state" or "nonce" parameters (e.g., LTI OIDC login, Deep Linking)
  - JWT "jti" (token id) attached to AccessClaims by your TokenVerifier
  - Idempotency keys supplied by clients: "Idempotency-Key" header

You can also implement your own store by satisfying the ReplayProtector
interface and pass it to the middleware below.
*/

// ReplayProtector is the interface used by the middleware to enforce
// single-use semantics on a (kind, value) pair for a given TTL.
type ReplayProtector interface {
	// Use marks (kind,value) as consumed for the provided TTL and returns true
	// if this is the first time it is seen (or the previous entry expired).
	// It returns false when the same (kind,value) is reused before it expires.
	Use(kind, value string, ttl time.Duration) (bool, error)
}

// InMemoryReplay is a simple process-local implementation of ReplayProtector.
// It is safe for concurrent use and performs opportunistic purging on writes.
type InMemoryReplay struct {
	mu      sync.Mutex
	entries map[string]time.Time
	// optional: every N uses we will purge expired entries
	useCount uint64
	purgeN   uint64
}

// NewInMemoryReplay creates an in-memory replay cache.
// purgeEvery controls how often (every N calls to Use) the cache purges
// expired entries. A value like 512 or 1024 is usually fine.
// If purgeEvery <= 0, a default of 1024 is used.
func NewInMemoryReplay(purgeEvery int) *InMemoryReplay {
	if purgeEvery <= 0 {
		purgeEvery = 1024
	}
	return &InMemoryReplay{
		entries: make(map[string]time.Time, 1024),
		purgeN:  uint64(purgeEvery),
	}
}

func (m *InMemoryReplay) Use(kind, value string, ttl time.Duration) (bool, error) {
	kind = strings.TrimSpace(strings.ToLower(kind))
	value = strings.TrimSpace(value)
	if kind == "" || value == "" {
		return false, fmt.Errorf("replay: kind and value are required")
	}
	now := time.Now()
	exp := now.Add(ttl)
	k := kind + "|" + value

	m.mu.Lock()
	defer m.mu.Unlock()

	// Opportunistic purge
	m.useCount++
	if m.useCount%m.purgeN == 0 {
		m.purgeLocked(now)
	}

	if until, ok := m.entries[k]; ok && until.After(now) {
		// seen and not expired -> replay
		return false, nil
	}
	m.entries[k] = exp
	return true, nil
}

func (m *InMemoryReplay) purgeLocked(now time.Time) {
	for k, until := range m.entries {
		if !until.After(now) {
			delete(m.entries, k)
		}
	}
}

/* ------------------------------- Middleware -------------------------------- */

// RequireNonce enforces that a request contains a single-use nonce provided
// via either a query parameter (?nonce=...) or a form field (POSTed nonce).
// The (kind,value) stored is ("nonce", <value>).
// If the nonce is missing, the request is rejected with 400.
// If the nonce was already used (and not yet expired), the request is rejected
// with 401 (unauthorized) to mirror typical OIDC behavior.
func RequireNonce(cache ReplayProtector, ttl time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			nonce := r.Form.Get("nonce")
			if nonce == "" {
				nonce = r.URL.Query().Get("nonce")
			}
			nonce = strings.TrimSpace(nonce)
			if nonce == "" {
				http.Error(w, "bad request: missing nonce", http.StatusBadRequest)
				return
			}
			ok, err := cache.Use("nonce", nonce, ttl)
			if err != nil {
				http.Error(w, "server error: replay check", http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, "unauthorized: nonce replay detected", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

// RequireIdempotencyKey enforces a unique header value (default: "Idempotency-Key")
// for a given TTL. If the header is missing, the request is rejected with 400.
// If the value was already used, the request is rejected with 409 Conflict.
func RequireIdempotencyKey(cache ReplayProtector, headerName string, ttl time.Duration) func(http.Handler) http.Handler {
	if headerName == "" {
		headerName = "Idempotency-Key"
	}
	hdr := headerName
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			key := strings.TrimSpace(r.Header.Get(hdr))
			if key == "" {
				http.Error(w, "bad request: missing "+hdr, http.StatusBadRequest)
				return
			}
			ok, err := cache.Use("idempotency", key, ttl)
			if err != nil {
				http.Error(w, "server error: replay check", http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, "conflict: idempotency key reuse", http.StatusConflict)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

// RequireUniqueJTI checks AccessClaims (populated by BearerAuth middleware) and,
// if a non-empty JTI is present, ensures it has not been seen within TTL.
// If JTI is absent, the request is allowed (no-op) to avoid breaking access
// tokens that don't carry jti. If you want strict enforcement, use
// RequireJTIStrict instead.
func RequireUniqueJTI(cache ReplayProtector, ttl time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok {
				http.Error(w, "forbidden: missing auth context", http.StatusForbidden)
				return
			}
			jti := strings.TrimSpace(claims.JTI)
			if jti == "" {
				// allow if not provided
				next.ServeHTTP(w, r)
				return
			}
			ok2, err := cache.Use("jti", jti, ttl)
			if err != nil {
				http.Error(w, "server error: replay check", http.StatusInternalServerError)
				return
			}
			if !ok2 {
				http.Error(w, "unauthorized: token replay detected", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}

// RequireJTIStrict enforces presence AND single-use of AccessClaims.JTI.
// If missing, it returns 400. If reused, it returns 401.
func RequireJTIStrict(cache ReplayProtector, ttl time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			claims, ok := FromContext(r.Context())
			if !ok {
				http.Error(w, "forbidden: missing auth context", http.StatusForbidden)
				return
			}
			jti := strings.TrimSpace(claims.JTI)
			if jti == "" {
				http.Error(w, "bad request: missing jti", http.StatusBadRequest)
				return
			}
			ok2, err := cache.Use("jti", jti, ttl)
			if err != nil {
				http.Error(w, "server error: replay check", http.StatusInternalServerError)
				return
			}
			if !ok2 {
				http.Error(w, "unauthorized: token replay detected", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
