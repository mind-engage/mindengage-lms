// pkg/platform/lti/helpers.go
package lti

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
)

// b64url encodes bytes using base64url without padding.
func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// randHex returns n random bytes hex-encoded (len=2n).
func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// schemeFromRequest returns "https" when behind a proxy that sets X-Forwarded-Proto,
// otherwise falls back to r.URL.Scheme or "http".
func schemeFromRequest(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		// may be "https,http"; take first
		if i := strings.IndexByte(xf, ','); i >= 0 {
			return strings.TrimSpace(xf[:i])
		}
		return strings.TrimSpace(xf)
	}
	if r.URL != nil && r.URL.Scheme != "" {
		return r.URL.Scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// hostWithoutPort strips :port if present.
func hostWithoutPort(h string) string {
	if i := strings.IndexByte(h, ':'); i >= 0 {
		return h[:i]
	}
	return h
}
