// pkg/platform/lti/authorize.go
package lti

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

/*
OIDC Authorization Endpoint (Platform side) for LTI 1.3

This endpoint accepts an authorization request from a Tool and returns
an `id_token` (via `response_mode=form_post`) that contains the required
LTI 1.3 launch claims.

Supported request parameters (subset):
  - response_type=id_token (required)
  - response_mode=form_post (required)
  - scope=openid (ignored, but typical)
  - client_id (required; must be a registered Tool)
  - redirect_uri (required; must match a registered redirect)
  - login_hint (opaque, Tool-provided; used by your resolver)
  - lti_message_hint (opaque, Tool-provided; guides message type & context)
  - nonce (required; copied to id_token)
  - state (strongly recommended; echoed back in form_post)

This file focuses on request validation, claim construction, and JWT minting.
It delegates the following to interfaces you provide:

  - IssuerResolver:  returns the issuer URL for a tenant (school)
  - ToolRegistry:    validates the Tool and its allowed redirect URIs
  - LaunchResolver:  resolves deployment/context/resource link, roles, user, etc.
  - Signer:          signs the JWT (RS256/ES256, etc.)
  - ResolveTenantID: extracts tenant id from the HTTP request (host/path)

Mount it under your public base, e.g.:
  r := chi.NewRouter()
  r.Get("/oauth/authorize", platform.AuthorizeHandler())
*/

const (
	ltiClaimMessageType = "https://purl.imsglobal.org/spec/lti/claim/message_type"
	ltiClaimVersion     = "https://purl.imsglobal.org/spec/lti/claim/version"
	ltiClaimDeployment  = "https://purl.imsglobal.org/spec/lti/claim/deployment_id"
	ltiClaimTarget      = "https://purl.imsglobal.org/spec/lti/claim/target_link_uri"
	ltiClaimContext     = "https://purl.imsglobal.org/spec/lti/claim/context"
	ltiClaimResource    = "https://purl.imsglobal.org/spec/lti/claim/resource_link"
	ltiClaimRoles       = "https://purl.imsglobal.org/spec/lti/claim/roles"
	ltiClaimToolPlat    = "https://purl.imsglobal.org/spec/lti/claim/tool_platform"

	// AGS & NRPS
	agsClaimEndpoint = "https://purl.imsglobal.org/spec/lti-ags/claim/endpoint"
	nrpsClaim        = "https://purl.imsglobal.org/spec/lti-nrps/claim/namesroleservice"

	// Deep linking
	dlClaimSettings = "https://purl.imsglobal.org/spec/lti-dl/claim/deep_linking_settings"

	// Message types
	msgTypeResourceLink = "LtiResourceLinkRequest"
	msgTypeDeepLink     = "LtiDeepLinkingRequest"
)

// ---------- Dependencies (provide real implementations in your service) -------

// IssuerResolver returns the platform issuer URL for a tenant (must be https/http absolute).
type IssuerResolver interface {
	IssuerForTenant(ctx context.Context, tenantID string) (issuer string, err error)
}

// Tool describes a registered Tool.
type Tool struct {
	ClientID     string
	Name         string
	RedirectURIs []string
}

// ToolRegistry looks up a Tool by (tenantID, clientID).
type ToolRegistry interface {
	GetTool(ctx context.Context, tenantID, clientID string) (Tool, error)
}

// LaunchInfo is returned by your LaunchResolver with all details needed for claims.
type LaunchInfo struct {
	// Required for a standard resource link launch:
	UserID         string   // platform subject for the user
	UserRoles      []string // IMS role URIs (Instructor/Learner/etc.)
	DeploymentID   string
	ContextID      string
	ContextLabel   string
	ContextTitle   string
	ResourceLinkID string // required for resource link launches

	// Services (absolute URLs)
	LineItemsURL string   // AGS collection URL for this context (optional but recommended)
	AGSScope     []string // granted AGS scopes for this Tool/user (subset)
	NRPSURL      string   // NRPS memberships URL for this context (optional)

	// Deep linking request (when message type is deep link)
	DeepLinking bool
	// If deep linking, return URL that the Tool should POST its response to.
	DeepLinkReturnURL string
	// Optional opaque data you want echoed back in the DL response.
	DeepLinkData string
}

// LaunchResolver maps the incoming request (tenant, client, login/message hints)
// and the currently authenticated platform user to a LaunchInfo.
type LaunchResolver interface {
	Resolve(ctx context.Context, tenantID, clientID, loginHint, messageHint string) (LaunchInfo, error)
}

// Signer signs a JWT with the platform private key for the tenant.
type Signer interface {
	// Sign returns a compact JWS with the provided claims. You may use tenantID
	// to select a tenant-specific key (KID) and set the appropriate "kid" header.
	Sign(ctx context.Context, tenantID string, claims map[string]any) (string, error)
}

// ---------- Server ------------------------------------------------------------

type AuthorizeServer struct {
	Issuers         IssuerResolver
	Registry        ToolRegistry
	Launches        LaunchResolver
	Signer          Signer
	ResolveTenantID func(*http.Request) (string, error)

	// Optional knobs
	Now              func() time.Time
	TokenTTL         time.Duration // default 5 minutes
	ExternalBasePath string        // if behind a proxy prefix (e.g., "/api")
	// tool_platform claim
	ProductName    string // e.g., "MindEngage"
	ProductVersion string // e.g., "1.0"
	PlatformGUID   string // stable GUID/URN for your platform deployment
}

// AuthorizeHandler returns the http.Handler for GET /oauth/authorize.
func (s *AuthorizeServer) AuthorizeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Basic wiring checks
		if s.Issuers == nil || s.Registry == nil || s.Launches == nil || s.Signer == nil || s.ResolveTenantID == nil {
			http.Error(w, "server not configured", http.StatusInternalServerError)
			return
		}

		tenantID, err := s.ResolveTenantID(r)
		if err != nil || strings.TrimSpace(tenantID) == "" {
			writeErr(w, http.StatusBadRequest, "unable to resolve tenant")
			return
		}

		// Parse/validate OIDC params
		q := r.URL.Query()
		if v := q.Get("response_type"); !eqFold(v, "id_token") {
			writeErr(w, http.StatusBadRequest, "response_type must be id_token")
			return
		}
		if v := q.Get("response_mode"); !eqFold(v, "form_post") {
			writeErr(w, http.StatusBadRequest, "response_mode must be form_post")
			return
		}

		clientID := strings.TrimSpace(q.Get("client_id"))
		if clientID == "" {
			writeErr(w, http.StatusBadRequest, "client_id required")
			return
		}
		redirectURI := strings.TrimSpace(q.Get("redirect_uri"))
		if !isHTTPURL(redirectURI) {
			writeErr(w, http.StatusBadRequest, "redirect_uri must be http(s)")
			return
		}
		nonce := strings.TrimSpace(q.Get("nonce"))
		if nonce == "" {
			writeErr(w, http.StatusBadRequest, "nonce required")
			return
		}
		state := strings.TrimSpace(q.Get("state")) // echoed back if present
		loginHint := strings.TrimSpace(q.Get("login_hint"))
		messageHintRaw := strings.TrimSpace(q.Get("lti_message_hint"))

		// Validate the Tool & redirect
		tool, err := s.Registry.GetTool(r.Context(), tenantID, clientID)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unknown tool/client")
			return
		}
		if !redirectAllowed(redirectURI, tool.RedirectURIs) {
			writeErr(w, http.StatusUnauthorized, "redirect_uri not allowed for client")
			return
		}

		// Build LaunchInfo (resolve user, roles, context, services)
		li, err := s.Launches.Resolve(r.Context(), tenantID, clientID, loginHint, messageHintRaw)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unable to resolve launch: "+err.Error())
			return
		}

		// Resolve issuer
		iss, err := s.Issuers.IssuerForTenant(r.Context(), tenantID)
		if err != nil || !isHTTPURL(iss) {
			writeErr(w, http.StatusInternalServerError, "issuer resolution failed")
			return
		}

		now := s.now()
		ttl := s.ttl()
		exp := now.Add(ttl)

		// Base OIDC id_token claims
		claims := map[string]any{
			"iss":   iss,
			"aud":   clientID,
			"sub":   nonEmpty(li.UserID, "user-"+randHex(8)), // resolver SHOULD set
			"iat":   now.Unix(),
			"exp":   exp.Unix(),
			"nonce": nonce,
			"azp":   clientID,
		}

		// tool_platform claim (recommended)
		if s.ProductName != "" || s.PlatformGUID != "" {
			claims[ltiClaimToolPlat] = map[string]any{
				"name":                s.ProductName,
				"version":             s.ProductVersion,
				"product_family_code": "mindengage",
				"guid":                s.PlatformGUID,
			}
		}

		// LTI claims (resource link or deep link)
		if li.DeepLinking {
			// Deep Linking Request
			claims[ltiClaimMessageType] = msgTypeDeepLink
			claims[ltiClaimVersion] = "1.3.0"
			if li.DeploymentID != "" {
				claims[ltiClaimDeployment] = li.DeploymentID
			}
			claims[dlClaimSettings] = map[string]any{
				"deep_link_return_url": li.DeepLinkReturnURL,
				"data":                 li.DeepLinkData,
				"accept_types":         []string{"ltiResourceLink"},
				"accept_presentation_document_targets": []string{
					"iframe", "window",
				},
			}
			// target_link_uri (where the Tool expects to receive it) is the same as redirect_uri
			claims[ltiClaimTarget] = redirectURI
		} else {
			// Resource Link Request
			claims[ltiClaimMessageType] = msgTypeResourceLink
			claims[ltiClaimVersion] = "1.3.0"
			claims[ltiClaimTarget] = redirectURI
			if li.DeploymentID != "" {
				claims[ltiClaimDeployment] = li.DeploymentID
			}
			if li.ContextID != "" {
				claims[ltiClaimContext] = map[string]any{
					"id":    li.ContextID,
					"label": li.ContextLabel,
					"title": li.ContextTitle,
				}
			}
			if li.ResourceLinkID != "" {
				claims[ltiClaimResource] = map[string]any{
					"id": li.ResourceLinkID,
				}
			}
			if len(li.UserRoles) > 0 {
				claims[ltiClaimRoles] = sortedCopy(li.UserRoles)
			}
			// Services
			if li.LineItemsURL != "" {
				scope := li.AGSScope
				if len(scope) == 0 {
					scope = []string{
						"https://purl.imsglobal.org/spec/lti-ags/scope/lineitem",
						"https://purl.imsglobal.org/spec/lti-ags/scope/score",
						"https://purl.imsglobal.org/spec/lti-ags/scope/result.readonly",
						"https://purl.imsglobal.org/spec/lti-ags/scope/lineitem.readonly",
					}
				}
				claims[agsClaimEndpoint] = map[string]any{
					"lineitems": li.LineItemsURL,
					"scope":     scope,
				}
			}
			if li.NRPSURL != "" {
				claims[nrpsClaim] = map[string]any{
					"context_memberships_url": li.NRPSURL,
					"service_versions":        []string{"2.0"},
				}
			}
		}

		// Sign the JWT
		jwt, err := s.Signer.Sign(r.Context(), tenantID, claims)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "signing failed")
			return
		}

		// Respond via form_post to redirect_uri (echo state if provided)
		writeFormPost(w, redirectURI, jwt, state)
	}
}

// ---------- Helpers ----------------------------------------------------------

func (s *AuthorizeServer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *AuthorizeServer) ttl() time.Duration {
	if s.TokenTTL > 0 {
		return s.TokenTTL
	}
	return 5 * time.Minute
}

func eqFold(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }

func nonEmpty(s, d string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return d
}

func sortedCopy(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, s := range xs {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

func redirectAllowed(uri string, allowed []string) bool {
	for _, a := range allowed {
		if strings.TrimSpace(a) == uri {
			return true
		}
	}
	return false
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeFormPost(w http.ResponseWriter, actionURL, idToken, state string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	const tpl = `<!doctype html>
<html><head><meta charset="utf-8"><title>LTI Launch</title></head>
<body onload="document.forms[0].submit()">
<form method="post" action="{{.Action}}">
  <input type="hidden" name="id_token" value="{{.JWT}}">
  {{if .State}}<input type="hidden" name="state" value="{{.State}}">{{end}}
  <noscript><button type="submit">Continue</button></noscript>
</form>
</body></html>`
	t := template.Must(template.New("fp").Parse(tpl))
	_ = t.Execute(w, map[string]string{
		"Action": actionURL,
		"JWT":    idToken,
		"State":  state,
	})
}

// ---------- Optional: helper for building service URLs (if you need it) ------

// BuildServiceURLs is a convenience that constructs AGS/NRPS URLs for a context
// based on the current request and an optional ExternalBasePath (e.g., "/api").
// You can call this from your LaunchResolver to fill LaunchInfo.LineItemsURL / NRPSURL.
func (s *AuthorizeServer) BuildServiceURLs(r *http.Request, contextID string) (lineItemsURL, nrpsURL string) {
	base := strings.TrimSuffix(s.ExternalBasePath, "/")
	scheme := schemeFromRequest(r)
	host := hostWithoutPort(r.Host)

	if contextID != "" {
		lineItemsURL = fmt.Sprintf("%s://%s%s/lti/ags/contexts/%s/line_items", scheme, host, base, url.PathEscape(contextID))
		nrpsURL = fmt.Sprintf("%s://%s%s/lti/nrps/contexts/%s/memberships", scheme, host, base, url.PathEscape(contextID))
	}
	return
}

// ---------- Utilities: message_hint decode (optional for resolvers) ----------

// MessageHint is the opaque structure we sometimes encode as base64url(JSON)
// in lti_message_hint (see deeplinking/request.go). This type is provided here
// as a convenience if your LaunchResolver wants to decode it.
type MessageHint struct {
	Type         string            `json:"type"` // "deep_link" or "launch"
	TenantID     string            `json:"tenant_id,omitempty"`
	ClientID     string            `json:"client_id,omitempty"`
	DeploymentID string            `json:"deployment_id,omitempty"`
	ContextID    string            `json:"context_id,omitempty"`
	ReturnURL    string            `json:"return_url,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
	IssuedAt     int64             `json:"iat,omitempty"`
}

// DecodeMessageHint decodes base64url(JSON) message hints. It returns ok=false
// when input is empty.
func DecodeMessageHint(s string) (mh MessageHint, ok bool, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return MessageHint{}, false, nil
	}
	buf, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		// some senders may include padding; try a different decoder
		buf, err = base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(s)
		if err != nil {
			return MessageHint{}, false, err
		}
	}
	err = json.Unmarshal(buf, &mh)
	return mh, true, err
}
