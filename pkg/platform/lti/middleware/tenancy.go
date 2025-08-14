// pkg/platform/lti/middleware/tenancy.go
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/mind-engage/mindengage-lms/pkg/platform/tenants"
)

/*
Tenancy middleware

This middleware resolves the current tenant (and its issuer URL) from each
incoming request using a tenants.Resolver, then attaches that information to
the request context for downstream handlers (e.g., AGS/NRPS servers, /oauth).

Typical wiring:

    tr := tenants.NewResolver(tenants.Options{
        BaseDomain:   "lti.mindengage.com",
        HostIsTenant: true,   // {tenant}.lti.mindengage.com
        ForceHTTPS:   true,
    })

    // Resolve tenant early and put into context for all platform routes.
    r := chi.NewRouter()
    r.Use(middleware.Tenancy(tr))

    // If you also use BearerAuthWithOptions, you can enforce tenant match:
    authMW := middleware.BearerAuthWithOptions(verifier, middleware.AuthOptions{
        Realm:              "LTI Platform",
        EnforceTenantMatch: true,
        ResolveTenantID:    middleware.ResolveTenantIDFromContext,
    })

Notes:
- If resolution fails, Tenancy() returns 400 by default. Use TenancyOptional()
  if you prefer to proceed without a tenant (context will not carry tenant info).
*/

// TenantInfo carries the resolved tenant identifier and its issuer URL.
type TenantInfo struct {
	ID     string // e.g., "school-a"
	Issuer string // e.g., "https://school-a.lti.mindengage.com"
}

// tenancyCtxKey is the private context key under which we store TenantInfo.
type tenancyCtxKey struct{}

// Tenancy creates middleware that resolves the tenant for each request.
// On failure to resolve, it returns HTTP 400 (Bad Request).
func Tenancy(res tenants.Resolver) func(http.Handler) http.Handler {
	return tenancy(res, false)
}

// TenancyOptional resolves the tenant but does NOT fail the request if it
// cannot be resolved. Downstream code can detect absence via TenancyInfo().
func TenancyOptional(res tenants.Resolver) func(http.Handler) http.Handler {
	return tenancy(res, true)
}

func tenancy(res tenants.Resolver, optional bool) func(http.Handler) http.Handler {
	if res == nil {
		panic("middleware.Tenancy: nil tenants.Resolver")
	}
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			tenantID, issuer, err := res.Resolve(r)
			if err != nil || strings.TrimSpace(tenantID) == "" {
				if optional {
					next.ServeHTTP(w, r) // proceed without tenant in context
					return
				}
				http.Error(w, "bad request: could not resolve tenant", http.StatusBadRequest)
				return
			}
			ti := TenantInfo{ID: tenantID, Issuer: issuer}
			ctx := context.WithValue(r.Context(), tenancyCtxKey{}, ti)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}

/* ----------------------------- Accessors ----------------------------------- */

// TenancyInfo extracts the resolved tenant information from context.
// The second return value is false when the middleware did not set tenancy.
func TenancyInfo(ctx context.Context) (TenantInfo, bool) {
	if ctx == nil {
		return TenantInfo{}, false
	}
	v := ctx.Value(tenancyCtxKey{})
	if v == nil {
		return TenantInfo{}, false
	}
	ti, ok := v.(TenantInfo)
	return ti, ok
}

// TenantIDFromRequest is a convenience that returns the tenant id from
// the request context (or an empty string if missing).
func TenantIDFromRequest(r *http.Request) string {
	if ti, ok := TenancyInfo(r.Context()); ok {
		return ti.ID
	}
	return ""
}

// IssuerFromRequest returns the issuer URL from the request context (or empty).
func IssuerFromRequest(r *http.Request) string {
	if ti, ok := TenancyInfo(r.Context()); ok {
		return ti.Issuer
	}
	return ""
}

// ResolveTenantIDFromContext adapts tenancy context into a function compatible
// with AuthOptions.ResolveTenantID in BearerAuthWithOptions.
func ResolveTenantIDFromContext(r *http.Request) (string, error) {
	if ti, ok := TenancyInfo(r.Context()); ok && strings.TrimSpace(ti.ID) != "" {
		return ti.ID, nil
	}
	return "", errors.New("tenant not resolved in context")
}
