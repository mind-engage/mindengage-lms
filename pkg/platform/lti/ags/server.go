// pkg/platform/lti/ags/server.go
package ags

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
Server implements the LTI AGS endpoints:

  GET  /contexts/{contextId}/line_items
  POST /contexts/{contextId}/line_items
  GET  /line_items/{id}
  PUT  /line_items/{id}
  DELETE /line_items/{id}
  POST /line_items/{id}/scores
  GET  /line_items/{id}/results

This file focuses on HTTP handling, JSON/media types, and basic pagination.
Storage (DB) operations are abstracted behind the Storage interface below.
*/

// Storage abstracts persistence for AGS data owned by the Platform.
type Storage interface {
	// Line items
	CreateLineItem(ctx context.Context, tenantID string, li LineItem) (LineItem, error)
	GetLineItem(ctx context.Context, tenantID, lineItemID string) (LineItem, error)
	UpdateLineItem(ctx context.Context, tenantID string, li LineItem) (LineItem, error)
	DeleteLineItem(ctx context.Context, tenantID, lineItemID string) error
	// List line items for a context/course with optional filters.
	ListLineItems(ctx context.Context, tenantID, contextID string, filter ListFilter, offset, limit int) ([]LineItem, error)

	// Scores/Results
	// Upsert the latest result for a user on a line item (server computes semantics).
	UpsertScore(ctx context.Context, tenantID, lineItemID string, in Score) (Result, error)
	// List results for a line item (optionally for a single user).
	ListResults(ctx context.Context, tenantID, lineItemID, userID string, offset, limit int) ([]Result, error)
}

// NotFound sentinel to allow 404 mapping.
var NotFound = errors.New("ags: not found")

// ListFilter narrows line item collection queries.
type ListFilter struct {
	ResourceID     string
	ResourceLinkID string
}

// Result matches IMS AGS v2 result container (trimmed to fields we return).
type Result struct {
	ID            string    `json:"id,omitempty"` // optional URL to this result
	UserID        string    `json:"userId"`
	ResultScore   *float64  `json:"resultScore,omitempty"`
	ResultMaximum *float64  `json:"resultMaximum,omitempty"`
	Comment       string    `json:"comment,omitempty"`
	Timestamp     time.Time `json:"timestamp,omitempty"`
}

// IDGen creates opaque IDs for new line items (path segment, not full URL).
type IDGen interface{ NewID() string }

type randIDGen struct{}

func (randIDGen) NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Server holds dependencies for AGS handlers.
type Server struct {
	Store            Storage
	Now              func() time.Time
	IDs              IDGen
	ResolveTenantID  func(*http.Request) (string, error) // required: resolve tenant from request (host/path/header)
	ExternalBasePath string                              // prefix before /api/lti/ags, optional (e.g., behind a reverse proxy)
}

// NewServer creates a Server with sane defaults.
// You MUST set Store and ResolveTenantID before using the handlers.
func NewServer(_ any) *Server {
	return &Server{
		Now: func() time.Time { return time.Now().UTC() },
		IDs: randIDGen{},
	}
}

/* ----------------------------- Handlers ----------------------------------- */

func (s *Server) PostLineItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := s.requireTenant(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	contextID := chi.URLParam(r, "contextId")
	if contextID == "" {
		writeErr(w, http.StatusBadRequest, "contextId is required")
		return
	}

	var in struct {
		Label          string   `json:"label"`
		ScoreMaximum   float64  `json:"scoreMaximum"`
		ResourceID     string   `json:"resourceId,omitempty"`
		ResourceLinkID string   `json:"resourceLinkId,omitempty"`
		StartDateTime  *string  `json:"startDateTime,omitempty"` // accepted but ignored in v0
		EndDateTime    *string  `json:"endDateTime,omitempty"`
		BestScore      *float64 `json:"bestScore,omitempty"` // non-standard; ignored
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(in.Label) == "" {
		writeErr(w, http.StatusBadRequest, "label is required")
		return
	}
	if in.ScoreMaximum <= 0 {
		writeErr(w, http.StatusBadRequest, "scoreMaximum must be > 0")
		return
	}

	// Build absolute line item URL id
	itemKey := s.IDs.NewID()
	itemURL := s.lineItemURL(r, itemKey)

	li := LineItem{
		ID:             itemURL,
		ContextID:      contextID,
		ResourceLinkID: strings.TrimSpace(in.ResourceLinkID),
		ResourceID:     strings.TrimSpace(in.ResourceID),
		Label:          strings.TrimSpace(in.Label),
		ScoreMaximum:   in.ScoreMaximum,
		CreatedAt:      s.Now(),
		UpdatedAt:      s.Now(),
	}
	created, err := s.Store.CreateLineItem(r.Context(), tenantID, li)
	if err != nil {
		writeStorageErr(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.ims.lis.v2.lineitem+json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(mapLineItemOut(created))
}

func (s *Server) GetLineItems(w http.ResponseWriter, r *http.Request) {
	tenantID, err := s.requireTenant(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	contextID := chi.URLParam(r, "contextId")
	if contextID == "" {
		writeErr(w, http.StatusBadRequest, "contextId is required")
		return
	}

	q := r.URL.Query()
	filter := ListFilter{
		ResourceID:     strings.TrimSpace(q.Get("resource_id")),
		ResourceLinkID: strings.TrimSpace(q.Get("resource_link_id")),
	}
	limit, page := parseLimitPage(q, 50, 1, 100)
	offset := (page - 1) * limit

	items, err := s.Store.ListLineItems(r.Context(), tenantID, contextID, filter, offset, limit)
	if err != nil {
		writeStorageErr(w, err)
		return
	}

	// Pagination "Link: <...>; rel=next" if we likely have more (naive heuristic)
	if len(items) == limit {
		nextURL := cloneURL(r.URL)
		nq := nextURL.Query()
		nq.Set("limit", strconv.Itoa(limit))
		nq.Set("page", strconv.Itoa(page+1))
		nextURL.RawQuery = nq.Encode()
		w.Header().Add("Link", fmt.Sprintf("<%s>; rel=\"next\"", nextURL.String()))
	}

	out := make([]any, 0, len(items))
	for _, it := range items {
		out = append(out, mapLineItemOut(it))
	}

	w.Header().Set("Content-Type", "application/vnd.ims.lis.v2.lineitemcontainer+json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) GetLineItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := s.requireTenant(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	liID := s.absoluteLineItemIDFromPath(r)
	item, err := s.Store.GetLineItem(r.Context(), tenantID, liID)
	if err != nil {
		writeStorageErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.ims.lis.v2.lineitem+json")
	_ = json.NewEncoder(w).Encode(mapLineItemOut(item))
}

func (s *Server) PutLineItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := s.requireTenant(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	liID := s.absoluteLineItemIDFromPath(r)

	var in struct {
		Label          *string  `json:"label,omitempty"`
		ScoreMaximum   *float64 `json:"scoreMaximum,omitempty"`
		ResourceID     *string  `json:"resourceId,omitempty"`
		ResourceLinkID *string  `json:"resourceLinkId,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Load existing, apply changes
	existing, err := s.Store.GetLineItem(r.Context(), tenantID, liID)
	if err != nil {
		writeStorageErr(w, err)
		return
	}
	if in.Label != nil {
		existing.Label = strings.TrimSpace(*in.Label)
	}
	if in.ScoreMaximum != nil {
		if *in.ScoreMaximum <= 0 {
			writeErr(w, http.StatusBadRequest, "scoreMaximum must be > 0")
			return
		}
		existing.ScoreMaximum = *in.ScoreMaximum
	}
	if in.ResourceID != nil {
		existing.ResourceID = strings.TrimSpace(*in.ResourceID)
	}
	if in.ResourceLinkID != nil {
		existing.ResourceLinkID = strings.TrimSpace(*in.ResourceLinkID)
	}
	existing.UpdatedAt = s.Now()

	updated, err := s.Store.UpdateLineItem(r.Context(), tenantID, existing)
	if err != nil {
		writeStorageErr(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.ims.lis.v2.lineitem+json")
	_ = json.NewEncoder(w).Encode(mapLineItemOut(updated))
}

func (s *Server) DeleteLineItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := s.requireTenant(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	liID := s.absoluteLineItemIDFromPath(r)
	if err := s.Store.DeleteLineItem(r.Context(), tenantID, liID); err != nil {
		writeStorageErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) PostScore(w http.ResponseWriter, r *http.Request) {
	tenantID, err := s.requireTenant(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	liID := s.absoluteLineItemIDFromPath(r)

	var in struct {
		UserID           string   `json:"userId"`
		ScoreGiven       *float64 `json:"scoreGiven,omitempty"`
		ScoreMaximum     *float64 `json:"scoreMaximum,omitempty"`
		ActivityProgress string   `json:"activityProgress,omitempty"`
		GradingProgress  string   `json:"gradingProgress,omitempty"`
		Comment          string   `json:"comment,omitempty"`
		Timestamp        string   `json:"timestamp,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(in.UserID) == "" {
		writeErr(w, http.StatusBadRequest, "userId is required")
		return
	}

	ts := s.Now()
	if in.Timestamp != "" {
		tp, err := time.Parse(time.RFC3339Nano, in.Timestamp)
		if err != nil {
			// try RFC3339
			if tp2, err2 := time.Parse(time.RFC3339, in.Timestamp); err2 == nil {
				tp = tp2
			} else {
				writeErr(w, http.StatusBadRequest, "invalid timestamp")
				return
			}
		}
		ts = tp.UTC()
	}

	score := Score{
		UserID:           strings.TrimSpace(in.UserID),
		ScoreGiven:       in.ScoreGiven,
		ScoreMaximum:     in.ScoreMaximum,
		ActivityProgress: defaultIfEmpty(in.ActivityProgress, "Completed"),
		GradingProgress:  defaultIfEmpty(in.GradingProgress, "FullyGraded"),
		Timestamp:        ts,
		Comment:          in.Comment,
	}

	if _, err := s.Store.UpsertScore(r.Context(), tenantID, liID, score); err != nil {
		writeStorageErr(w, err)
		return
	}
	// Valid statuses include 200/201/202/204. We return 200 OK.
	w.WriteHeader(http.StatusOK)
}

func (s *Server) GetResults(w http.ResponseWriter, r *http.Request) {
	tenantID, err := s.requireTenant(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	liID := s.absoluteLineItemIDFromPath(r)

	q := r.URL.Query()
	userID := strings.TrimSpace(q.Get("user_id"))
	limit, page := parseLimitPage(q, 50, 1, 100)
	offset := (page - 1) * limit

	results, err := s.Store.ListResults(r.Context(), tenantID, liID, userID, offset, limit)
	if err != nil {
		writeStorageErr(w, err)
		return
	}

	// pagination next link if likely more
	if len(results) == limit {
		nextURL := cloneURL(r.URL)
		nq := nextURL.Query()
		nq.Set("limit", strconv.Itoa(limit))
		nq.Set("page", strconv.Itoa(page+1))
		if userID != "" {
			nq.Set("user_id", userID)
		}
		nextURL.RawQuery = nq.Encode()
		w.Header().Add("Link", fmt.Sprintf("<%s>; rel=\"next\"", nextURL.String()))
	}

	type outRes struct {
		ID            string    `json:"id,omitempty"`
		UserID        string    `json:"userId"`
		ResultScore   *float64  `json:"resultScore,omitempty"`
		ResultMaximum *float64  `json:"resultMaximum,omitempty"`
		Comment       string    `json:"comment,omitempty"`
		Timestamp     time.Time `json:"timestamp,omitempty"`
	}

	out := make([]outRes, 0, len(results))
	for _, r := range results {
		out = append(out, outRes{
			ID:            r.ID,
			UserID:        r.UserID,
			ResultScore:   r.ResultScore,
			ResultMaximum: r.ResultMaximum,
			Comment:       r.Comment,
			Timestamp:     r.Timestamp,
		})
	}

	w.Header().Set("Content-Type", "application/vnd.ims.lis.v2.resultcontainer+json")
	_ = json.NewEncoder(w).Encode(out)
}

/* ----------------------------- Helpers ------------------------------------ */

func (s *Server) requireTenant(r *http.Request) (string, error) {
	if s.ResolveTenantID == nil {
		return "", errors.New("tenant resolver not configured")
	}
	return s.ResolveTenantID(r)
}

func (s *Server) absoluteLineItemIDFromPath(r *http.Request) string {
	opaqueID := chi.URLParam(r, "id")
	return s.lineItemURL(r, opaqueID)
}

func (s *Server) lineItemURL(r *http.Request, opaqueID string) string {
	// External scheme/host
	scheme := schemeFromRequest(r)
	host := hostWithoutPort(r.Host)

	// Optional base path (if the server is mounted under a prefix at a proxy)
	base := strings.TrimSuffix(s.ExternalBasePath, "/")

	// Our routes expect /api/lti/ags/line_items/{id}
	return fmt.Sprintf("%s://%s%s/api/lti/ags/line_items/%s", scheme, host, base, url.PathEscape(opaqueID))
}

func mapLineItemOut(it LineItem) map[string]any {
	return map[string]any{
		"id":             it.ID,
		"label":          it.Label,
		"scoreMaximum":   it.ScoreMaximum,
		"resourceId":     it.ResourceID,
		"resourceLinkId": it.ResourceLinkID,
	}
}

func parseLimitPage(q url.Values, defLimit, defPage, maxLimit int) (limit, page int) {
	limit = defLimit
	page = defPage

	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= maxLimit {
			limit = n
		}
	}
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	return
}

func cloneURL(in *url.URL) *url.URL {
	out := *in
	return &out
}

func defaultIfEmpty(s, d string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return d
	}
	return s
}

func writeStorageErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, NotFound):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

type errPayload struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errPayload{Error: msg})
}

func schemeFromRequest(r *http.Request) string {
	// Prefer proxy header if present
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		// Could be "https,http"
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
