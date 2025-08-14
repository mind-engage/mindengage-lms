// pkg/platform/lti/metadata.go
package lti

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

/*
OpenID Provider Discovery (".well-known/openid-configuration") for the
MindEngage LTI Platform.

Tools use this document to learn:
  • the Platform issuer
  • authorization, token and JWKS endpoints
  • supported response modes/types and signing algorithms
  • (LTI) the platform configuration extension

This handler is multi-tenant aware. You must provide:
  - ResolveTenantID: how to map an incoming request to a tenant
  - Issuers:         how to obtain the issuer URL for that tenant

Typical wiring:

    ms := &lti.MetadataServer{
        ResolveTenantID: resolveTenantFromHost, // e.g., "{tenant}.lti.example.com"
        Issuers:         yourIssuerResolver,
        ExternalBasePath: "/api",               // optional, if behind a reverse proxy prefix
        ProductName:     "MindEngage",
        ProductVersion:  "1.0",
        PlatformGUID:    "urn:mindengage:platform:prod",
        AllowCORS:       true,
    }

    r := chi.NewRouter()
    r.Get("/.well-known/openid-configuration", ms.OpenIDConfiguration())

Notes:
• JWKS is served by JWKSHandler in jwks.go (mount at "/.well-known/jwks.json").
• This file only emits discovery metadata; it does not implement endpoints.
*/

type MetadataServer struct {
	// ResolveTenantID must extract a stable tenant identifier from the request.
	ResolveTenantID func(*http.Request) (string, error)

	// Issuers must return the absolute issuer URL (https/http) for a tenant.
	Issuers IssuerResolver

	// Optional public prefix (if the service is exposed behind a reverse proxy).
	// Example: "/api"
	ExternalBasePath string

	// Which ID Token algorithms this platform will sign with (default: RS256).
	IDTokenAlgs []string

	// Advertise supported token endpoint auth methods (default: private_key_jwt + client_secret_post).
	TokenAuthMethods []string

	// Optional: enable dynamic client registration by advertising an absolute registration endpoint.
	// Leave empty to omit.
	RegistrationAbsoluteURL string

	// Optional presentation details reported via the LTI platform configuration extension.
	ProductName      string // e.g., "MindEngage"
	ProductVersion   string // e.g., "1.0"
	PlatformGUID     string // stable identifier for this platform deployment
	LogoURI          string // absolute https URL to a logo (optional)
	SupportEmail     string // "service_documentation" style hint (optional)
	DocumentationURL string // optional absolute URL with docs

	// Caching and CORS
	CacheMaxAge time.Duration // default 1h
	AllowCORS   bool
}

// OpenIDConfiguration returns a handler for /.well-known/openid-configuration.
func (s *MetadataServer) OpenIDConfiguration() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.ResolveTenantID == nil || s.Issuers == nil {
			http.Error(w, "metadata: not configured", http.StatusInternalServerError)
			return
		}
		tenantID, err := s.ResolveTenantID(r)
		if err != nil || strings.TrimSpace(tenantID) == "" {
			http.Error(w, "metadata: unable to resolve tenant", http.StatusBadRequest)
			return
		}
		iss, err := s.Issuers.IssuerForTenant(r.Context(), tenantID)
		if err != nil || !isHTTPURL(iss) {
			http.Error(w, "metadata: issuer resolution failed", http.StatusInternalServerError)
			return
		}

		base := strings.TrimSuffix(s.ExternalBasePath, "/")
		scheme := schemeFromRequest(r)
		host := hostWithoutPort(r.Host)

		authorizationEndpoint := joinURL(scheme, host, base, "/oauth/authorize")
		tokenEndpoint := joinURL(scheme, host, base, "/oauth/token")
		jwksURI := joinURL(scheme, host, "", "/.well-known/jwks.json")

		cfg := map[string]any{
			"issuer":                                iss,
			"authorization_endpoint":                authorizationEndpoint,
			"token_endpoint":                        tokenEndpoint,
			"jwks_uri":                              jwksURI,
			"response_modes_supported":              []string{"form_post"},
			"response_types_supported":              []string{"id_token"},
			"scopes_supported":                      []string{"openid"}, // OIDC scope list (LTI scopes are advertised in extension below)
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": s.idTokenAlgs(),
			"token_endpoint_auth_methods_supported": s.tokenAuthMethods(),
			"claims_supported": []string{
				"iss", "sub", "aud", "exp", "iat", "nonce", "azp",
				// Common LTI claims (non-exhaustive)
				ltiClaimMessageType, ltiClaimVersion, ltiClaimDeployment, ltiClaimTarget,
				ltiClaimResource, ltiClaimContext, ltiClaimRoles, ltiClaimToolPlat,
				agsClaimEndpoint, nrpsClaim, dlClaimSettings,
			},
		}
		if s.RegistrationAbsoluteURL != "" && isHTTPURL(s.RegistrationAbsoluteURL) {
			cfg["registration_endpoint"] = s.RegistrationAbsoluteURL
		}
		if s.DocumentationURL != "" && isHTTPURL(s.DocumentationURL) {
			cfg["service_documentation"] = s.DocumentationURL
		}

		// LTI Platform Configuration extension per 1EdTech
		cfg["https://purl.imsglobal.org/spec/lti-platform-configuration"] = map[string]any{
			"product_family_code": "mindengage",
			"version":             orDefault(s.ProductVersion, "1.0"),
			"guid":                s.PlatformGUID,
			"messages_supported": []map[string]any{
				{"type": msgTypeResourceLink},
				{"type": msgTypeDeepLink},
			},
			"variables": []string{
				"Context.id", "Context.label", "Context.title",
				"ResourceLink.id",
				"User.id", "User.username", "User.email", "User.locale",
			},
			// Optional branding info
			"presentation_document_target": []string{"iframe", "window"},
			"logo_uri":                     s.logoOrEmpty(),
		}

		payload, _ := json.Marshal(cfg)
		maxAge := int(s.cacheAge().Seconds())

		if s.AllowCORS {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(maxAge))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}
}

/* ------------------------------ helpers ----------------------------------- */

func (s *MetadataServer) idTokenAlgs() []string {
	if len(s.IDTokenAlgs) == 0 {
		return []string{"RS256"}
	}
	out := make([]string, 0, len(s.IDTokenAlgs))
	for _, a := range s.IDTokenAlgs {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

func (s *MetadataServer) tokenAuthMethods() []string {
	if len(s.TokenAuthMethods) == 0 {
		return []string{"private_key_jwt", "client_secret_post"}
	}
	out := make([]string, 0, len(s.TokenAuthMethods))
	for _, m := range s.TokenAuthMethods {
		m = strings.TrimSpace(m)
		if m != "" {
			out := append(out, m)
			_ = out // shadow fix (see below)
		}
	}
	// correct append usage (no shadow)
	out = make([]string, 0, len(s.TokenAuthMethods))
	for _, m := range s.TokenAuthMethods {
		m = strings.TrimSpace(m)
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}

func (s *MetadataServer) cacheAge() time.Duration {
	if s.CacheMaxAge > 0 {
		return s.CacheMaxAge
	}
	return time.Hour
}

func (s *MetadataServer) logoOrEmpty() string {
	if isHTTPURL(s.LogoURI) {
		return s.LogoURI
	}
	return ""
}

func joinURL(scheme, host, base, path string) string {
	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   strings.TrimSuffix(base, "/") + path,
	}
	return u.String()
}

func orDefault(sv, d string) string {
	if strings.TrimSpace(sv) != "" {
		return sv
	}
	return d
}
