// pkg/platform/tenants/resolver.go
package tenants

import (
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// Resolver resolves the current tenant and its issuer URL from an HTTP request.
type Resolver interface {
	// Resolve extracts the tenant identifier and the issuer URL for that tenant.
	// Issuer must be a stable URL you publish in metadata/JWKS (per 1EdTech LTI).
	Resolve(r *http.Request) (tenantID string, issuer string, err error)
}

// Options controls multi-tenant resolution.
//
// Typical host-based setup (recommended):
//
//	BaseDomain:    "lti.mindengage.com"
//	HostIsTenant:  true               // {tenant}.lti.mindengage.com
//	ForceHTTPS:    true
//
// Path-based alternative (single host):
//
//	BaseDomain:    "lti.mindengage.com"  // or leave empty to use r.Host
//	HostIsTenant:  false
//	PathPrefix:    "/t"                  // issuer => https://{BaseDomain}/t/{tenant}
//	ForceHTTPS:    true
//
// Header override (for tests/internal routing):
//
//	HeaderKey:     "X-ME-Tenant"         // if present, takes precedence
type Options struct {
	BaseDomain    string // e.g. "lti.mindengage.com" (no scheme)
	HostIsTenant  bool   // true => {tenant}.{BaseDomain}
	PathPrefix    string // e.g. "/t" when HostIsTenant == false
	HeaderKey     string // optional override header for tenant id, e.g. "X-ME-Tenant"
	DefaultTenant string // optional fallback when tenant could not be inferred
	ForceHTTPS    bool   // if true, issuer scheme is always https
}

// NewResolver returns a Resolver that can resolve tenant from header (if set),
// host (subdomain), or path prefix, depending on Options.
func NewResolver(opts Options) Resolver {
	// Normalize PathPrefix
	if opts.PathPrefix != "" && !strings.HasPrefix(opts.PathPrefix, "/") {
		opts.PathPrefix = "/" + opts.PathPrefix
	}
	return &universalResolver{opts: opts}
}

type universalResolver struct {
	opts Options
}

// Resolve implements the Resolver interface.
func (u *universalResolver) Resolve(r *http.Request) (string, string, error) {
	// 1) Header override (highest priority)
	if u.opts.HeaderKey != "" {
		if v := strings.TrimSpace(r.Header.Get(u.opts.HeaderKey)); v != "" {
			tenant := sanitizeTenant(v)
			if tenant == "" {
				return "", "", errBadTenantToken
			}
			return tenant, u.issuerFor(tenant, r), nil
		}
	}

	// 2) Host-based tenant, e.g., {tenant}.lti.mindengage.com
	if u.opts.HostIsTenant {
		if tenant := u.tenantFromHost(r); tenant != "" {
			return tenant, u.issuerFor(tenant, r), nil
		}
	}

	// 3) Path-based tenant, e.g., /t/{tenant}/...
	if u.opts.PathPrefix != "" {
		if tenant := u.tenantFromPath(r); tenant != "" {
			return tenant, u.issuerFor(tenant, r), nil
		}
	}

	// 4) Fallback default
	if u.opts.DefaultTenant != "" {
		tenant := sanitizeTenant(u.opts.DefaultTenant)
		if tenant == "" {
			return "", "", errBadTenantToken
		}
		return tenant, u.issuerFor(tenant, r), nil
	}

	return "", "", errNoTenant
}

// tenantFromHost extracts {tenant} from {tenant}.{BaseDomain}.
func (u *universalResolver) tenantFromHost(r *http.Request) string {
	host := hostWithoutPort(r.Host)
	base := strings.ToLower(strings.TrimSpace(u.opts.BaseDomain))
	if host == "" || base == "" {
		return ""
	}
	// Exact base domain => no subdomain (ambiguous)
	if strings.EqualFold(host, base) {
		return ""
	}
	// Must end with ".{BaseDomain}"
	suffix := "." + base
	if !strings.HasSuffix(strings.ToLower(host), suffix) {
		return ""
	}
	// Strip suffix and take the leftmost label as tenant.
	rest := host[:len(host)-len(suffix)]
	if rest == "" {
		return ""
	}
	labels := strings.Split(rest, ".")
	tenant := sanitizeTenant(labels[0])
	return tenant
}

// tenantFromPath extracts {tenant} from /{PathPrefix}/{tenant}/...
func (u *universalResolver) tenantFromPath(r *http.Request) string {
	if u.opts.PathPrefix == "" {
		return ""
	}
	path := r.URL.Path
	if !strings.HasPrefix(path, u.opts.PathPrefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, u.opts.PathPrefix)
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return ""
	}
	segEnd := strings.IndexByte(rest, '/')
	if segEnd == -1 {
		segEnd = len(rest)
	}
	raw := rest[:segEnd]
	return sanitizeTenant(raw)
}

// issuerFor builds the issuer URL for a given tenant, based on Options.
// - HostIsTenant: https://{tenant}.{BaseDomain}
// - PathPrefix:   https://{BaseDomain}{PathPrefix}/{tenant}
// If BaseDomain is empty in path mode, uses r.Host (without port).
func (u *universalResolver) issuerFor(tenant string, r *http.Request) string {
	scheme := schemeFromRequest(r, u.opts.ForceHTTPS)

	if u.opts.HostIsTenant {
		// require BaseDomain
		base := strings.TrimSpace(u.opts.BaseDomain)
		if base == "" {
			// Fallback: use request host's base domain (unsafe; avoid in prod)
			base = hostWithoutPort(r.Host)
		}
		return scheme + "://" + tenant + "." + base
	}

	// Path-based issuer
	base := strings.TrimSpace(u.opts.BaseDomain)
	host := base
	if host == "" {
		host = hostWithoutPort(r.Host)
	}
	return scheme + "://" + host + u.opts.PathPrefix + "/" + tenant
}

/* ------------------------------ helpers ----------------------------------- */

var (
	errNoTenant       = errors.New("tenants: could not resolve tenant from request")
	errBadTenantToken = errors.New("tenants: invalid tenant token")
)

// sanitizeTenant lowercases and validates the tenant token.
// Allowed: [a-z0-9] first char, then [a-z0-9-]{0,61}, total up to 62 chars.
// Adjust if you need underscores or longer IDs.
func sanitizeTenant(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	// Strict but DNS-friendly pattern.
	var re = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,61}$`)
	if !re.MatchString(s) {
		return ""
	}
	return s
}

// schemeFromRequest attempts to infer the external scheme.
// If forceHTTPS is true, always returns "https".
func schemeFromRequest(r *http.Request, forceHTTPS bool) string {
	if forceHTTPS {
		return "https"
	}
	// Respect common proxy headers first.
	if xfproto := r.Header.Get("X-Forwarded-Proto"); xfproto != "" {
		// could be "https,http"; take first token
		if i := strings.IndexByte(xfproto, ','); i >= 0 {
			return strings.TrimSpace(xfproto[:i])
		}
		return strings.TrimSpace(xfproto)
	}
	// Fallback to TLS presence.
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func hostWithoutPort(h string) string {
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}
