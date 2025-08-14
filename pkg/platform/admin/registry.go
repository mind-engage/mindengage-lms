// pkg/platform/admin/registry.go
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mind-engage/mindengage-lms/pkg/platform/tenants"
)

/*
Package admin exposes a minimal, multi-tenant-aware HTTP API to manage:
  - Tools (client_id, JWKS URL, redirect URIs, allowed scopes, auth methods)
  - Deployments (deployment_id bound to tenant + client_id + context_id)

It is intentionally thin and delegates persistence to a Store interface,
so you can provide a SQL or other implementation elsewhere.

Route prefix (suggested): /admin
All endpoints are scoped by {tenantID} path param.
*/

// Store is the persistence interface used by the admin registry API.
type Store interface {
	// Tools
	CreateTool(ctx context.Context, t tenants.Tool) error
	GetTool(ctx context.Context, tenantID, clientID string) (tenants.Tool, error)
	ListTools(ctx context.Context, tenantID string, offset, limit int) ([]tenants.Tool, error)
	UpdateTool(ctx context.Context, t tenants.Tool) error
	DeleteTool(ctx context.Context, tenantID, clientID string) error

	// Deployments
	CreateDeployment(ctx context.Context, d tenants.Deployment) error
	GetDeployment(ctx context.Context, tenantID, id string) (tenants.Deployment, error)
	ListDeployments(ctx context.Context, tenantID string, filter DeploymentFilter, offset, limit int) ([]tenants.Deployment, error)
	DeleteDeployment(ctx context.Context, tenantID, id string) error
}

// NotFound is a sentinel error that Store implementations can return to signal 404s.
var NotFound = errors.New("admin: not found")

// DeploymentFilter narrows ListDeployments results.
type DeploymentFilter struct {
	ClientID  string
	ContextID string
}

// Routes returns an http.Handler with CRUD endpoints for tools and deployments.
// Mount it under something like: r.Mount("/admin", admin.Routes(store))
func Routes(store Store) http.Handler {
	r := chi.NewRouter()

	// Tools
	r.Post("/tenants/{tenantID}/tools", createTool(store))
	r.Get("/tenants/{tenantID}/tools", listTools(store))
	r.Get("/tenants/{tenantID}/tools/{clientID}", getTool(store))
	r.Put("/tenants/{tenantID}/tools/{clientID}", updateTool(store))
	r.Delete("/tenants/{tenantID}/tools/{clientID}", deleteTool(store))

	// Deployments
	r.Post("/tenants/{tenantID}/deployments", createDeployment(store))
	r.Get("/tenants/{tenantID}/deployments", listDeployments(store))
	r.Get("/tenants/{tenantID}/deployments/{id}", getDeployment(store))
	r.Delete("/tenants/{tenantID}/deployments/{id}", deleteDeployment(store))

	return r
}

/* ------------------------------- Tools ------------------------------------ */

func createTool(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")

		var req CreateToolReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if msg := validateCreateToolReq(req); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}

		t := tenants.Tool{
			ClientID:      strings.TrimSpace(req.ClientID),
			TenantID:      tenantID,
			Name:          strings.TrimSpace(req.Name),
			JWKSURL:       strings.TrimSpace(req.JWKSURL),
			RedirectURIs:  trimAll(req.RedirectURIs),
			AllowedScopes: trimAll(req.AllowedScopes),
			AuthMethods:   trimAll(req.AuthMethods),
		}

		if err := store.CreateTool(r.Context(), t); err != nil {
			if errors.Is(err, NotFound) {
				writeErr(w, http.StatusNotFound, "tenant not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, t)
	}
}

func getTool(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		clientID := chi.URLParam(r, "clientID")
		t, err := store.GetTool(r.Context(), tenantID, clientID)
		if err != nil {
			if errors.Is(err, NotFound) {
				writeErr(w, http.StatusNotFound, "tool not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, t)
	}
}

func listTools(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		offset, limit := parsePage(r, 0, 100)
		items, err := store.ListTools(r.Context(), tenantID, offset, limit)
		if err != nil {
			if errors.Is(err, NotFound) {
				// Treat missing tenant as empty list to avoid leaking existence
				writeJSON(w, http.StatusOK, []tenants.Tool{})
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func updateTool(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		clientID := chi.URLParam(r, "clientID")

		var req CreateToolReq // full replacement for simplicity
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		// ClientID in path is canonical; body may omit or must match if present.
		if req.ClientID != "" && strings.TrimSpace(req.ClientID) != clientID {
			writeErr(w, http.StatusBadRequest, "client_id in body must match path")
			return
		}
		req.ClientID = clientID

		if msg := validateCreateToolReq(req); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		t := tenants.Tool{
			ClientID:      clientID,
			TenantID:      tenantID,
			Name:          strings.TrimSpace(req.Name),
			JWKSURL:       strings.TrimSpace(req.JWKSURL),
			RedirectURIs:  trimAll(req.RedirectURIs),
			AllowedScopes: trimAll(req.AllowedScopes),
			AuthMethods:   trimAll(req.AuthMethods),
		}
		if err := store.UpdateTool(r.Context(), t); err != nil {
			if errors.Is(err, NotFound) {
				writeErr(w, http.StatusNotFound, "tool not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, t)
	}
}

func deleteTool(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		clientID := chi.URLParam(r, "clientID")
		if err := store.DeleteTool(r.Context(), tenantID, clientID); err != nil {
			if errors.Is(err, NotFound) {
				writeErr(w, http.StatusNotFound, "tool not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

/* ---------------------------- Deployments --------------------------------- */

func createDeployment(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")

		var req CreateDeploymentReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if msg := validateCreateDeploymentReq(req); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}

		d := tenants.Deployment{
			ID:        strings.TrimSpace(req.ID),
			TenantID:  tenantID,
			ClientID:  strings.TrimSpace(req.ClientID),
			ContextID: strings.TrimSpace(req.ContextID),
			Title:     strings.TrimSpace(req.Title),
		}
		if err := store.CreateDeployment(r.Context(), d); err != nil {
			if errors.Is(err, NotFound) {
				writeErr(w, http.StatusNotFound, "tenant or tool not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, d)
	}
}

func getDeployment(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		id := chi.URLParam(r, "id")
		d, err := store.GetDeployment(r.Context(), tenantID, id)
		if err != nil {
			if errors.Is(err, NotFound) {
				writeErr(w, http.StatusNotFound, "deployment not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

func listDeployments(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		offset, limit := parsePage(r, 0, 100)
		filter := DeploymentFilter{
			ClientID:  strings.TrimSpace(r.URL.Query().Get("client_id")),
			ContextID: strings.TrimSpace(r.URL.Query().Get("context_id")),
		}
		items, err := store.ListDeployments(r.Context(), tenantID, filter, offset, limit)
		if err != nil {
			if errors.Is(err, NotFound) {
				writeJSON(w, http.StatusOK, []tenants.Deployment{})
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func deleteDeployment(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := chi.URLParam(r, "tenantID")
		id := chi.URLParam(r, "id")
		if err := store.DeleteDeployment(r.Context(), tenantID, id); err != nil {
			if errors.Is(err, NotFound) {
				writeErr(w, http.StatusNotFound, "deployment not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

/* ------------------------------ Validation -------------------------------- */

func validateCreateToolReq(req CreateToolReq) string {
	if strings.TrimSpace(req.ClientID) == "" {
		return "client_id is required"
	}
	if strings.TrimSpace(req.Name) == "" {
		return "name is required"
	}
	if strings.TrimSpace(req.JWKSURL) == "" {
		return "jwks_url is required"
	}
	if !isHTTPURL(req.JWKSURL) {
		return "jwks_url must be http(s) URL"
	}
	if len(req.RedirectURIs) == 0 {
		return "redirect_uris is required"
	}
	for _, u := range req.RedirectURIs {
		if !isHTTPURL(u) {
			return "redirect_uris must contain only http(s) URLs"
		}
	}
	if len(req.AuthMethods) == 0 {
		return "auth_methods is required (e.g., [\"private_key_jwt\"])"
	}
	return ""
}

func validateCreateDeploymentReq(req CreateDeploymentReq) string {
	if strings.TrimSpace(req.ID) == "" {
		return "id (deployment_id) is required"
	}
	if strings.TrimSpace(req.ClientID) == "" {
		return "client_id is required"
	}
	if strings.TrimSpace(req.ContextID) == "" {
		return "context_id is required"
	}
	return ""
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

/* ------------------------------ Utilities --------------------------------- */

func parsePage(r *http.Request, defOffset, defLimit int) (offset, limit int) {
	q := r.URL.Query()
	offset = defOffset
	limit = defLimit

	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	return
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errResp struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errResp{Error: msg})
}

// trimAll trims whitespace from every string in the slice and removes empties.
func trimAll(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, s := range xs {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
