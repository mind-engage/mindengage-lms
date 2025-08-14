// pkg/platform/lti/deeplinking/response.go
package deeplinking

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

/*
Deep Linking Response handler (Platform side)

This handler accepts the Tool’s Deep Linking Response (a signed JWT),
verifies it against the Tool’s JWKS, extracts content items, persists
resource links (and optional AGS line items), and then redirects the
instructor back to your UI (or returns JSON if no return URL is provided).

Expected request (per 1EdTech LTI DL 1.3):
  POST content-type: application/x-www-form-urlencoded
  Form field: JWT=<tool-signed-JWT>
Some tools may send `id_token` instead of `JWT`; we accept both.

Verification steps (performed via Verifier interface you supply):
  - Check signature using Tool’s JWKS (looked up by tenant + tool client_id)
  - Validate `aud` (should be this Platform’s issuer/client identifier)
  - Validate `exp`, `iat`, (optionally) `nonce`/`jti` if you have a replay store
  - Validate LTI `message_type` == "LtiDeepLinkingResponse"

Persistence (via Store interface you supply):
  - Upsert a Placement/Resource Link in the specified `context_id`
  - If the content item contains a `lineItem` hint, also upsert a platform line item

Context resolution:
  - You must resolve `tenantID` from the HTTP request (host or path)
  - `deployment_id` and `context_id` can be found in the response claims,
    but some Tools omit them; we also accept optional query params:
      ?deployment_id=...&context_id=...
  - If a `return` query param is present, we redirect there after success
    (add your own CSRF/state tracking as needed).

This is a complete, compile-light handler with interfaces for verification and storage.
Wire these to your actual implementations in your server bootstrap.
*/

// ------------------------------ Interfaces ----------------------------------

// Verifier validates the Tool-signed Deep Linking JWT and returns its claims.
type Verifier interface {
	// VerifyToolJWT validates sig (using Tool's JWKS), time claims, audience,
	// and returns claims if valid. expectedAud is usually the Platform issuer URL.
	VerifyToolJWT(ctx context.Context, tenantID, toolClientID, rawJWT, expectedAud string) (map[string]any, error)
}

// Store persists placements and optional platform line items based on content items.
type Store interface {
	// UpsertResourceLink stores/updates a placement for a tool link in a context.
	// resourceLinkID is the LTI resource_link_id used by the Platform; if you pass
	// an empty id, the store can generate a new one.
	UpsertResourceLink(ctx context.Context, tenantID, clientID, deploymentID, contextID, resourceLinkID, title, targetURL string, custom map[string]string) (string, error)

	// UpsertPlatformLineItem creates or updates a line item associated with the placement.
	// Returns the absolute line item id (URL) if created/updated.
	UpsertPlatformLineItem(ctx context.Context, tenantID, contextID, resourceLinkID, resourceID, label string, scoreMax float64) (string, error)
}

// ------------------------------- Types --------------------------------------

type Server struct {
	Issuers         IssuerResolver
	Tools           Registry
	Verify          Verifier
	Store           Store
	ResolveTenantID func(*http.Request) (string, error) // required

	// Optional: a replay checker to prevent reuse of response JWTs (by jti/nonce).
	Replay Replay
}

type Replay interface {
	// Use should atomically mark the value consumed (return true if this is first use).
	Use(kind, value string) (bool, error)
}

// Output represents a minimal JSON result when no redirect is specified.
type Output struct {
	Status           string   `json:"status"`
	PlacementCount   int      `json:"placementCount"`
	CreatedLineItems []string `json:"createdLineItems,omitempty"`
}

// ------------------------------ Handler -------------------------------------

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ResponseHandler verifies the Deep Linking Response and persists placements.
func (s *Server) ResponseHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, err := s.requireTenant(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		// Parse form (for x-www-form-urlencoded POST) but also allow query fallback
		_ = r.ParseForm()
		rawJWT := firstNonEmpty(
			r.PostFormValue("JWT"),
			r.PostFormValue("jwt"),
			r.PostFormValue("id_token"),
			r.URL.Query().Get("JWT"),
			r.URL.Query().Get("jwt"),
			r.URL.Query().Get("id_token"),
		)
		if strings.TrimSpace(rawJWT) == "" {
			writeErr(w, http.StatusBadRequest, "missing Deep Linking JWT (JWT/id_token)")
			return
		}

		// Extract unverified claims to discover tool client_id (iss) quickly
		uv, err := unverifiedClaims(rawJWT)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "malformed JWT")
			return
		}
		toolClientID := asString(uv["iss"])
		if toolClientID == "" {
			writeErr(w, http.StatusBadRequest, "missing iss in JWT")
			return
		}

		// Resolve expected audience (this Platform issuer URL)
		expAud, err := s.Issuers.IssuerForTenant(r.Context(), tenantID)
		if err != nil || !isHTTPURL(expAud) {
			writeErr(w, http.StatusInternalServerError, "issuer resolution failed")
			return
		}

		// Verify JWT (signature, aud, exp/iat, etc.) using platform-provided Verifier.
		claims, err := s.Verify.VerifyToolJWT(r.Context(), tenantID, toolClientID, rawJWT, expAud)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "jwt verification failed: "+err.Error())
			return
		}

		// Optional replay protection using jti or nonce claims
		if s.Replay != nil {
			if jti := asString(claims["jti"]); jti != "" {
				if ok, _ := s.Replay.Use("jti", jti); !ok {
					writeErr(w, http.StatusUnauthorized, "jwt replay detected")
					return
				}
			} else if nonce := asString(claims["nonce"]); nonce != "" {
				if ok, _ := s.Replay.Use("nonce", nonce); !ok {
					writeErr(w, http.StatusUnauthorized, "nonce replay detected")
					return
				}
			}
		}

		// Validate LTI message type
		if mt := asString(getNested(claims, "https://purl.imsglobal.org/spec/lti/claim/message_type")); mt != "LtiDeepLinkingResponse" {
			writeErr(w, http.StatusBadRequest, "not a Deep Linking Response")
			return
		}

		// Deployment/context (prefer claims, else query params fallback)
		deploymentID := asString(getNested(claims, "https://purl.imsglobal.org/spec/lti/claim/deployment_id"))
		contextID := ""
		if ctxObj, ok := getNested(claims, "https://purl.imsglobal.org/spec/lti/claim/context").(map[string]any); ok {
			contextID = asString(ctxObj["id"])
		}
		// Fallbacks from URL if not present in claims
		if deploymentID == "" {
			deploymentID = strings.TrimSpace(r.URL.Query().Get("deployment_id"))
		}
		if contextID == "" {
			contextID = strings.TrimSpace(r.URL.Query().Get("context_id"))
		}
		if deploymentID == "" || contextID == "" {
			writeErr(w, http.StatusBadRequest, "deployment_id and context_id are required (claims or query)")
			return
		}

		// Content items
		items, err := parseContentItems(getNested(claims, "https://purl.imsglobal.org/spec/lti-dl/claim/content_items"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid content_items: "+err.Error())
			return
		}
		if len(items) == 0 {
			writeErr(w, http.StatusBadRequest, "no content items provided")
			return
		}

		// Persist placements (+ optional line items)
		createdLineItems := make([]string, 0, len(items))
		for _, it := range items {
			if it.Type != "ltiResourceLink" {
				// For now we only persist LTI resource links. Ignore other types.
				continue
			}
			// Resource link id is optional in the response; generate if absent.
			resourceLinkID := it.ResourceLinkID
			if resourceLinkID == "" {
				resourceLinkID = "rl-" + randHex(8)
			}

			if _, err := s.Store.UpsertResourceLink(
				r.Context(),
				tenantID,
				toolClientID,
				deploymentID,
				contextID,
				resourceLinkID,
				it.Title,
				it.URL,
				it.Custom,
			); err != nil {
				writeErr(w, http.StatusInternalServerError, "persist resource link: "+err.Error())
				return
			}

			// If the item includes an AGS lineItem hint, create/update it now.
			if it.LineItem != nil {
				lbl := strings.TrimSpace(it.LineItem.Label)
				if lbl == "" {
					lbl = it.Title
				}
				scoreMax := it.LineItem.ScoreMaximum
				if scoreMax <= 0 {
					// Default if not provided
					scoreMax = 100
				}
				lineID, err := s.Store.UpsertPlatformLineItem(
					r.Context(),
					tenantID,
					contextID,
					resourceLinkID,
					it.LineItem.ResourceID,
					lbl,
					scoreMax,
				)
				if err != nil {
					writeErr(w, http.StatusInternalServerError, "persist line item: "+err.Error())
					return
				}
				if lineID != "" {
					createdLineItems = append(createdLineItems, lineID)
				}
			}
		}

		// Success: redirect or return JSON
		if ret := strings.TrimSpace(r.URL.Query().Get("return")); ret != "" && isHTTPURL(ret) {
			// Optionally add a small status to the return URL
			u, _ := url.Parse(ret)
			q := u.Query()
			q.Set("dl_status", "ok")
			q.Set("dl_count", strconv.Itoa(len(items)))
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}

		writeJSON(w, http.StatusOK, Output{
			Status:           "ok",
			PlacementCount:   len(items),
			CreatedLineItems: createdLineItems,
		})
	}
}

// ------------------------------- Helpers ------------------------------------

func (s *Server) requireTenant(r *http.Request) (string, error) {
	if s.ResolveTenantID == nil {
		return "", errors.New("tenant resolver not configured")
	}
	return s.ResolveTenantID(r)
}

// unverifiedClaims decodes the JWT payload without verifying the signature.
// Only used to extract "iss" to locate the correct JWKS for full verification.
func unverifiedClaims(jwt string) (map[string]any, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid JWT format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// some libs may use padded encoding; try regular
		payload, err = base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(parts[1])
		if err != nil {
			return nil, err
		}
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case []any:
		if len(t) > 0 {
			return asString(t[0])
		}
	}
	return ""
}

func getNested(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errPayload struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errPayload{Error: msg})
}

/* ---------------------- Deep Linking content items ------------------------- */

// ContentItem represents a single item from the Deep Linking Response.
type ContentItem struct {
	Type           string // expect "ltiResourceLink"
	Title          string
	URL            string
	ResourceLinkID string
	Custom         map[string]string
	LineItem       *LineItemHint
}

type LineItemHint struct {
	Label        string
	ScoreMaximum float64
	ResourceID   string
}

// parseContentItems accepts the raw claim value and returns normalized items.
func parseContentItems(raw any) ([]ContentItem, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, errors.New("content_items must be an array")
	}
	out := make([]ContentItem, 0, len(arr))
	for _, el := range arr {
		obj, ok := el.(map[string]any)
		if !ok {
			return nil, errors.New("content item must be an object")
		}
		item := ContentItem{
			Type:           asString(obj["type"]),
			Title:          asString(obj["title"]),
			URL:            asString(obj["url"]),
			ResourceLinkID: asString(obj["resourceLinkId"]),
			Custom:         toStringMap(obj["custom"]),
		}
		// Optional AGS line item hint
		if li, ok := obj["lineItem"].(map[string]any); ok {
			item.LineItem = &LineItemHint{
				Label:        asString(li["label"]),
				ScoreMaximum: toFloat(li["scoreMaximum"]),
				ResourceID:   asString(li["resourceId"]),
			}
		}
		// Basic validation for resource link
		if item.Type == "ltiResourceLink" {
			if item.URL == "" {
				return nil, errors.New("ltiResourceLink requires url")
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func toStringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s := asString(v); s != "" {
			out[k] = s
		}
	}
	return out
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		if t == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
	}
	return 0
}
