// pkg/platform/lti/deeplinking/request.go
package deeplinking

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

/*
Package deeplinking initiates an LTI 1.3 Deep Linking flow by building the
Tool’s OIDC Login Initiation URL.

Flow (high level):
1) Instructor clicks “Add external content” in the Platform UI.
2) Platform sends the user to the Tool’s *login initiation endpoint* with:
   - iss (Platform issuer)
   - login_hint (opaque hint the Platform will validate later)
   - target_link_uri (Tool’s redirect/launch endpoint)
   - client_id (tool registration id at the Platform)
   - lti_message_hint (opaque, e.g., context info)
   - optional lti_deployment_id
3) Tool redirects the user to the Platform’s /authorize with the right params.
4) Platform returns an id_token with message_type=LtiDeepLinkingRequest.

This file builds step (2). It does NOT sign anything; signing happens when the
Platform later returns the id_token at /authorize.
*/

// IssuerResolver returns the fully-qualified issuer URL for a tenant.
// Example: "https://schoolA.lti.mindengage.com"
type IssuerResolver interface {
	IssuerForTenant(ctx context.Context, tenantID string) (issuer string, err error)
}

// Tool holds the minimal data we need for deep-linking initiation.
type Tool struct {
	ClientID         string
	InitiateLoginURL string   // Tool’s OIDC login initiation endpoint (required)
	RedirectURIs     []string // Registered redirect URIs (pick one for target_link_uri)
}

// Registry exposes tool lookup for a given tenant + client id.
type Registry interface {
	GetTool(ctx context.Context, tenantID, clientID string) (Tool, error)
}

// Client builds Deep Linking login initiation URLs for Tools.
type Client struct {
	Issuers IssuerResolver
	Tools   Registry
	// Optional: select which tool redirect URI to use as target_link_uri.
	// If nil, the first registered URI is used.
	PickRedirect func(redirects []string) (string, error)
	// Now allows deterministic tests
	Now func() time.Time
}

// StartParams contains Platform context for a deep-link initiation.
type StartParams struct {
	TenantID     string
	ClientID     string
	DeploymentID string
	ContextID    string
	// ReturnURL is where your Platform UI should land AFTER a successful deep-link
	// response from the Tool (non-standard, carried inside lti_message_hint).
	ReturnURL string

	// Optional: if empty, a random hash will be generated (prefixed with "dl-").
	// In production, pass a session-bound, single-use token you can verify later.
	LoginHint string

	// Optional override; if empty we pick from the Tool’s registered RedirectURIs.
	TargetLinkURI string

	// Optional extra key/values to include in lti_message_hint JSON (string values only).
	// Use sparingly; keep the hint small. Anything sensitive should NOT be placed here.
	Extra map[string]string
}

// StartRequest returns the Tool’s OIDC Login Initiation URL for Deep Linking.
//
// Typical usage from an HTTP handler:
//
//	url, err := client.StartRequest(r.Context(), StartParams{...})
//	if err != nil { /* 400/500 */ }
//	http.Redirect(w, r, url, http.StatusFound)
func (c *Client) StartRequest(ctx context.Context, p StartParams) (string, error) {
	if c.Issuers == nil || c.Tools == nil {
		return "", errors.New("deeplinking: Client not configured (Issuers/Tools)")
	}
	if strings.TrimSpace(p.TenantID) == "" {
		return "", errors.New("deeplinking: tenant_id is required")
	}
	if strings.TrimSpace(p.ClientID) == "" {
		return "", errors.New("deeplinking: client_id is required")
	}

	tool, err := c.Tools.GetTool(ctx, p.TenantID, p.ClientID)
	if err != nil {
		return "", fmt.Errorf("deeplinking: get tool: %w", err)
	}
	if !isHTTPURL(tool.InitiateLoginURL) {
		return "", fmt.Errorf("deeplinking: tool initiate_login_url is invalid")
	}

	target := strings.TrimSpace(p.TargetLinkURI)
	if target == "" {
		target, err = c.pickRedirect(tool.RedirectURIs)
		if err != nil {
			return "", err
		}
	}
	if !isHTTPURL(target) {
		return "", fmt.Errorf("deeplinking: target_link_uri must be http(s) absolute URL")
	}

	iss, err := c.Issuers.IssuerForTenant(ctx, p.TenantID)
	if err != nil || !isHTTPURL(iss) {
		if err == nil {
			err = fmt.Errorf("invalid issuer for tenant %q", p.TenantID)
		}
		return "", fmt.Errorf("deeplinking: resolve issuer: %w", err)
	}

	loginHint := strings.TrimSpace(p.LoginHint)
	if loginHint == "" {
		loginHint = "dl-" + randHex(16)
	}

	msgHint, err := buildMessageHint(messageHint{
		TenantID:     p.TenantID,
		ClientID:     p.ClientID,
		DeploymentID: p.DeploymentID,
		ContextID:    p.ContextID,
		ReturnURL:    p.ReturnURL,
		Extra:        p.Extra,
	})
	if err != nil {
		return "", fmt.Errorf("deeplinking: lti_message_hint: %w", err)
	}

	// Build the final initiation URL with query parameters
	u, _ := url.Parse(tool.InitiateLoginURL)
	q := u.Query()
	q.Set("iss", iss)
	q.Set("login_hint", loginHint)
	q.Set("target_link_uri", target)
	q.Set("client_id", tool.ClientID)
	if p.DeploymentID != "" {
		q.Set("lti_deployment_id", p.DeploymentID)
	}
	if msgHint != "" {
		q.Set("lti_message_hint", msgHint)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

/* -------------------------------- helpers --------------------------------- */

func (c *Client) pickRedirect(uris []string) (string, error) {
	if c.PickRedirect != nil {
		return c.PickRedirect(uris)
	}
	if len(uris) == 0 {
		return "", errors.New("deeplinking: tool has no redirect_uris")
	}
	return strings.TrimSpace(uris[0]), nil
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
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

// randHex returns n random bytes hex-encoded (length 2n).
func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

/* ------------------------- lti_message_hint format ------------------------- */

// messageHint is an internal, compact structure we encode as Base64URL(JSON).
// It is opaque to the Tool but helps the Platform correlate the context when
// handling the later /authorize request and Deep Linking Response.
type messageHint struct {
	Type         string            `json:"type"` // always "deep_link"
	TenantID     string            `json:"tenant_id,omitempty"`
	ClientID     string            `json:"client_id,omitempty"`
	DeploymentID string            `json:"deployment_id,omitempty"`
	ContextID    string            `json:"context_id,omitempty"`
	ReturnURL    string            `json:"return_url,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
	IssuedAt     int64             `json:"iat"` // seconds
}

func buildMessageHint(m messageHint) (string, error) {
	if m.Type == "" {
		m.Type = "deep_link"
	}
	if m.IssuedAt == 0 {
		m.IssuedAt = time.Now().UTC().Unix()
	}
	buf, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	// Base64 URL without padding (as commonly used in LTI ecosystems).
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
