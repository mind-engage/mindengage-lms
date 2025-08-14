// pkg/platform/lti/nrps/routes.go
// NOTE: This file implements the NRPS (Names & Role Provisioning Service) HTTP route,
// but follows the folder/package name you requested: "nrps". If you prefer the
// canonical spelling, rename the folder/package to "nrps".
package nrps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

/*
NRPS (Names & Role Provisioning Service) – route scaffolding

Exposes:
  GET /contexts/{contextId}/memberships

Behavior:
- Requires an OAuth2 Bearer token with the scope:
    https://purl.imsglobal.org/spec/lti-nrps/scope/contextmembership.readonly
  (Enforce this with your auth middleware outside of this package.)
- Supports optional filtering by `role` query param (exact URI or short alias).
- Supports pagination with `limit` (default 50, max 200) and `page_token`.
  If the storage indicates there is another page (non-empty next token),
  the response will include a RFC-5988 `Link: <...>; rel="next"` header.
- Returns a Membership Container per 1EdTech NRPS v2 media type:
    application/vnd.ims.lti-nrps.v2.membershipcontainer+json

Wire-up example:

  srv := &nrps.Server{
      Store:           myNRPSStore,
      ResolveTenantID: middleware.ResolveTenantIDFromContext, // or your own
      ExternalBasePath: "/api", // optional
      Now: func() time.Time { return time.Now().UTC() },
  }

  r := chi.NewRouter()
  // r.Use(middleware.Tenancy(tenantsResolver))
  // r.Use(middleware.BearerAuthWithOptions(...))
  r.Mount("/lti/nrps", nrps.Routes(srv))

*/

// --------- Storage contract (implement this in your platform storage) --------

// Storage abstracts persistence for NRPS data.
type Storage interface {
	// ListMemberships returns a page of memberships for a context. If roleFilter
	// is non-empty, results should be filtered to members that have the role.
	// pageToken is an opaque implementation-defined cursor for pagination.
	// Returns: items, nextPageToken ("" when no more), error.
	ListMemberships(ctx context.Context, tenantID, contextID, roleFilter, pageToken string, limit int) ([]Membership, string, error)

	// GetContextMeta returns minimal context metadata for container "context".
	// If your platform doesn't track label/title, return empty strings.
	GetContextMeta(ctx context.Context, tenantID, contextID string) (ContextMeta, error)
}

// --------- Types returned by Storage and serialized to JSON container --------

// Membership reflects a single user in a context with their roles.
// Field names match NRPS v2 JSON where applicable.
type Membership struct {
	UserID               string         `json:"user_id"`          // required
	Roles                []string       `json:"roles"`            // role URIs
	Status               string         `json:"status,omitempty"` // Active|Inactive
	Name                 string         `json:"name,omitempty"`   // display name
	GivenName            string         `json:"given_name,omitempty"`
	FamilyName           string         `json:"family_name,omitempty"`
	MiddleName           string         `json:"middle_name,omitempty"`
	Email                string         `json:"email,omitempty"`
	Picture              string         `json:"picture,omitempty"`
	LISPersonSourcedID   string         `json:"lis_person_sourcedid,omitempty"`
	LTI11LegacyUserID    string         `json:"lti11_legacy_user_id,omitempty"` // optional legacy
	AdditionalProperties map[string]any `json:"-"`                              // storage-only, not serialized
}

// ContextMeta populates the container's "context" object.
type ContextMeta struct {
	ID    string // required in container
	Label string // optional
	Title string // optional
}

// --------- Server and routes -------------------------------------------------

// Server holds dependencies for the NRPS routes.
type Server struct {
	Store            Storage
	ResolveTenantID  func(*http.Request) (string, error) // required
	ExternalBasePath string                              // optional prefix before /lti/nrps
	Now              func() time.Time                    // optional clock (defaults to UTC now)
}

// Routes returns a chi.Router with NRPS endpoints mounted at the router root.
func Routes(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Get("/contexts/{contextId}/memberships", s.GetMemberships)
	return r
}

// GetMemberships handles GET /contexts/{contextId}/memberships.
func (s *Server) GetMemberships(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil || s.ResolveTenantID == nil {
		writeErr(w, http.StatusInternalServerError, "server not configured")
		return
	}

	tenantID, err := s.ResolveTenantID(r)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		writeErr(w, http.StatusBadRequest, "could not resolve tenant")
		return
	}
	contextID := chi.URLParam(r, "contextId")
	if strings.TrimSpace(contextID) == "" {
		writeErr(w, http.StatusBadRequest, "contextId is required")
		return
	}

	q := r.URL.Query()
	roleFilter := normalizeRole(q.Get("role")) // accepts full URI or friendly alias
	limit := clamp(parseInt(q.Get("limit"), 50), 1, 200)
	pageToken := strings.TrimSpace(q.Get("page_token"))

	meta, err := s.Store.GetContextMeta(r.Context(), tenantID, contextID)
	if err != nil {
		writeStorageErr(w, err)
		return
	}
	if meta.ID == "" {
		meta.ID = contextID
	}

	items, nextToken, err := s.Store.ListMemberships(r.Context(), tenantID, contextID, roleFilter, pageToken, limit)
	if err != nil {
		writeStorageErr(w, err)
		return
	}

	// RFC-5988 "Link: <...>; rel=next" if we have another page
	if nextToken != "" {
		next := cloneURL(r.URL)
		nq := next.Query()
		nq.Set("limit", strconv.Itoa(limit))
		nq.Set("page_token", nextToken)
		if roleFilter != "" {
			nq.Set("role", roleFilter)
		}
		next.RawQuery = nq.Encode()
		w.Header().Add("Link", fmt.Sprintf("<%s>; rel=\"next\"", next.String()))
	}

	// Build membership container
	selfURL := absoluteURL(r)
	container := map[string]any{
		"id": selfURL,
		"context": map[string]any{
			"id":    meta.ID,
			"label": meta.Label,
			"title": meta.Title,
		},
		"members": mapMembers(items),
		// "next": optional per spec (we prefer the Link header; include when helpful)
	}
	if nextToken != "" {
		// Include "next" as a convenience for clients that don't read Link headers
		next := cloneURL(r.URL)
		nq := next.Query()
		nq.Set("limit", strconv.Itoa(limit))
		nq.Set("page_token", nextToken)
		if roleFilter != "" {
			nq.Set("role", roleFilter)
		}
		next.RawQuery = nq.Encode()
		container["next"] = next.String()
	}

	w.Header().Set("Content-Type", "application/vnd.ims.lti-nrps.v2.membershipcontainer+json")
	_ = json.NewEncoder(w).Encode(container)
}

/* ------------------------------- Helpers ----------------------------------- */

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func mapMembers(in []Membership) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, m := range in {
		out = append(out, map[string]any{
			"status":               emptyAs(m.Status, "Active"),
			"name":                 m.Name,
			"given_name":           m.GivenName,
			"family_name":          m.FamilyName,
			"middle_name":          m.MiddleName,
			"email":                m.Email,
			"user_id":              m.UserID,
			"roles":                normalizeRoles(m.Roles),
			"lis_person_sourcedid": m.LISPersonSourcedID,
			"lti11_legacy_user_id": m.LTI11LegacyUserID,
			"picture":              m.Picture,
		})
	}
	return out
}

func normalizeRoles(rs []string) []string {
	if len(rs) == 0 {
		return rs
	}
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, normalizeRole(r))
	}
	return out
}

// normalizeRole accepts full URIs or common short aliases and yields a URI.
func normalizeRole(in string) string {
	s := strings.TrimSpace(strings.ToLower(in))
	switch s {
	case "", "any":
		return ""
	case "learner", "student":
		return "http://purl.imsglobal.org/vocab/lis/v2/membership#Learner"
	case "instructor", "teacher":
		return "http://purl.imsglobal.org/vocab/lis/v2/membership#Instructor"
	case "ta", "teachingassistant":
		return "http://purl.imsglobal.org/vocab/lis/v2/membership#TeachingAssistant"
	case "contentdeveloper":
		return "http://purl.imsglobal.org/vocab/lis/v2/membership#ContentDeveloper"
	case "manager":
		return "http://purl.imsglobal.org/vocab/lis/v2/membership#Manager"
	case "administrator", "admin":
		return "http://purl.imsglobal.org/vocab/lis/v2/membership#Administrator"
	}
	// If caller already provided a URI, return as-is
	if strings.HasPrefix(in, "http://") || strings.HasPrefix(in, "https://") {
		return in
	}
	return in
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func clamp(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func emptyAs(s, def string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	return s
}

func cloneURL(in *url.URL) *url.URL {
	out := *in
	return &out
}

func absoluteURL(r *http.Request) string {
	scheme := schemeFromRequest(r)
	host := hostWithoutPort(r.Host)

	var path string
	if base := strings.TrimSuffix("", "/"); base != "" {
		path = base + r.URL.Path
	} else {
		path = r.URL.Path
	}
	u := url.URL{
		Scheme:   scheme,
		Host:     host,
		Path:     path,
		RawQuery: r.URL.RawQuery,
	}
	return u.String()
}

func schemeFromRequest(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		if i := strings.IndexByte(xf, ','); i >= 0 {
			return strings.TrimSpace(xf[:i])
		}
		return strings.TrimSpace(xf)
	}
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

type errPayload struct {
	Error string `json:"error"`
}

var errNotFound = errors.New("nrps: not found")

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errPayload{Error: msg})
}

func writeStorageErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}
