package rbac

import (
	"net/http"
)

var defaultChecker = NewChecker(nil)

// Require enforces a single permission.
func Require(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromContext(r.Context())
			if role == "" || !defaultChecker.Has(role, perm) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAny enforces that the role has at least one of the permissions.
func RequireAny(perms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromContext(r.Context())
			if role == "" || !defaultChecker.Any(role, perms...) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAll enforces that the role has all of the permissions.
func RequireAll(perms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromContext(r.Context())
			if role == "" || !defaultChecker.All(role, perms...) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireOwnerOr(perm string, isOwner func(r *http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromContext(r.Context())
			if isOwner(r) || defaultChecker.Has(role, perm) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "forbidden", http.StatusForbidden)
		})
	}
}
