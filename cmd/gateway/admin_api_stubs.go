package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	authmw "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	rbac "github.com/mind-engage/mindengage-lms/internal/rbac"
)

// mountAdminRoutes wires governance-focused Admin APIs under /api/admin.
// All handlers are *stubs* that validate input and return placeholder JSON.
// Replace bodies with real implementations incrementally.
func mountAdminRoutes(api chi.Router, dbh *sql.DB, authSvc *authmw.AuthService) {
	_ = dbh
	_ = authSvc
	api.Route("/admin", func(r chi.Router) {
		// ---- Tenants & Feature Flags ----
		r.With(rbac.Require("admin:tenants")).Get("/tenants", handleAdminListTenants)
		r.With(rbac.Require("admin:tenants")).Post("/tenants", handleAdminCreateTenant)
		r.With(rbac.Require("admin:tenants")).Post("/tenants/{tenantID}/flags", handleAdminUpdateTenantFlags)

		// ---- Identity, Roles, API Keys ----
		r.With(rbac.Require("admin:identity")).Get("/identity/providers", handleAdminListIdentityProviders)
		r.With(rbac.Require("admin:identity")).Post("/identity/providers", handleAdminAddIdentityProvider)

		r.With(rbac.Require("admin:apikeys")).Get("/api-keys", handleAdminListAPIKeys)
		r.With(rbac.Require("admin:apikeys")).Post("/api-keys", handleAdminCreateAPIKey)
		r.With(rbac.Require("admin:apikeys")).Delete("/api-keys/{keyID}", handleAdminRevokeAPIKey)

		r.With(rbac.Require("admin:identity")).Patch("/users/{userID}", handleAdminUpdateUserRole)

		// ---- Content Governance ----
		r.With(rbac.Require("admin:content")).Post("/exams/{examID}/approve", handleAdminApproveExam)
		r.With(rbac.Require("admin:content")).Post("/exams/{examID}/archive", handleAdminArchiveExam)
		r.With(rbac.Require("admin:content")).Post("/policy-templates", handleAdminSavePolicyTemplate)

		// ---- Attempts Oversight ----
		r.With(rbac.Require("admin:attempts")).Post("/attempts/{attemptID}/{action}", handleAdminAttemptAction)

		// ---- Compliance & Audit ----
		r.With(rbac.Require("admin:compliance")).Post("/pii/export", handleAdminPIIExport)
		r.With(rbac.Require("admin:compliance")).Post("/pii/delete", handleAdminPIIDelete)
		r.With(rbac.Require("admin:compliance")).Get("/audit", handleAdminAuditSearch)

		// ---- Settings (CORS, IP allowlist, Branding) ----
		r.With(rbac.Require("admin:settings")).Get("/cors", handleAdminGetCORS)
		r.With(rbac.Require("admin:settings")).Post("/cors", handleAdminSetCORS)
		r.With(rbac.Require("admin:settings")).Get("/ip-allowlist", handleAdminGetIPAllowlist)
		r.With(rbac.Require("admin:settings")).Post("/ip-allowlist", handleAdminSetIPAllowlist)
		r.With(rbac.Require("admin:settings")).Post("/branding", handleAdminSetBranding)
	})
}

// -----------------------------
// Handlers (stubs)
// -----------------------------

func handleAdminListTenants(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, []map[string]any{})
}

func handleAdminCreateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	resp := map[string]any{
		"id":     fmt.Sprintf("tnt_%d", time.Now().UnixNano()),
		"name":   req.Name,
		"domain": req.Domain,
		"flags":  map[string]bool{},
	}
	respondJSON(w, http.StatusCreated, resp)
}

func handleAdminUpdateTenantFlags(w http.ResponseWriter, r *http.Request) {
	var flags map[string]bool
	if err := json.NewDecoder(r.Body).Decode(&flags); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"tenant_id": chi.URLParam(r, "tenantID"), "updated": flags})
}

func handleAdminListIdentityProviders(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, []map[string]any{})
}

func handleAdminAddIdentityProvider(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	body["id"] = fmt.Sprintf("idp_%d", time.Now().UnixNano())
	respondJSON(w, http.StatusCreated, body)
}

func handleAdminListAPIKeys(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, []map[string]any{})
}

func handleAdminCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	respondJSON(w, http.StatusCreated, map[string]any{
		"id":         fmt.Sprintf("key_%d", time.Now().UnixNano()),
		"prefix":     "ak_test_",
		"note":       req.Note,
		"created_at": time.Now().Unix(),
	})
}

func handleAdminRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusNoContent, nil)
}

func handleAdminUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"user_id": chi.URLParam(r, "userID"), "role": req.Role})
}

func handleAdminApproveExam(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{"exam_id": chi.URLParam(r, "examID"), "status": "approved"})
}

func handleAdminArchiveExam(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{"exam_id": chi.URLParam(r, "examID"), "status": "archived"})
}

func handleAdminSavePolicyTemplate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if _, ok := body["id"]; !ok {
		body["id"] = fmt.Sprintf("policy_%d", time.Now().UnixNano())
	}
	respondJSON(w, http.StatusCreated, body)
}

func handleAdminAttemptAction(w http.ResponseWriter, r *http.Request) {
	action := chi.URLParam(r, "action")
	switch action {
	case "force-submit", "unlock", "invalidate":
		// ok
	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Reason == "" {
		http.Error(w, "reason required", http.StatusBadRequest)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"attempt_id": chi.URLParam(r, "attemptID"),
		"action":     action,
		"status":     "queued",
	})
}

func handleAdminPIIExport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}
	respondJSON(w, http.StatusAccepted, map[string]any{"job_id": fmt.Sprintf("job_%d", time.Now().UnixNano())})
}

func handleAdminPIIDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}
	respondJSON(w, http.StatusAccepted, map[string]any{"job_id": fmt.Sprintf("job_%d", time.Now().UnixNano())})
}

func handleAdminAuditSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	rows := []map[string]any{
		{"at": time.Now().Add(-2 * time.Hour), "actor": "admin:stub", "action": "search", "target": "audit", "reason": q, "ip": "0.0.0.0"},
	}
	respondJSON(w, http.StatusOK, rows)
}

func handleAdminGetCORS(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{"origins": []string{}})
}

func handleAdminSetCORS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Origins []string `json:"origins"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	respondJSON(w, http.StatusNoContent, nil)
}

func handleAdminGetIPAllowlist(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{"ips": []string{}})
}

func handleAdminSetIPAllowlist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IPs []string `json:"ips"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	respondJSON(w, http.StatusNoContent, nil)
}

func handleAdminSetBranding(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		PrimaryColor string `json:"primary_color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	respondJSON(w, http.StatusNoContent, nil)
}

// -----------------------------
// helpers
// -----------------------------
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
