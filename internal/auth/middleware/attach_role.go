// internal/auth/middleware/attach_role.go
package auth

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/mind-engage/mindengage-lms/internal/rbac"
)

// allowClaimFallback=true in dev/offline; false in prod.
func AttachRoleFromDB(db *sql.DB, allowClaimFallback bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			sub := rbac.SubjectFromContext(ctx)
			claimRole := rbac.RoleFromContext(ctx) // set by JWTMiddleware

			// Try DB by id or username (dev tokens often use username as sub)
			var role string
			err := db.QueryRowContext(ctx,
				`SELECT role FROM users WHERE id=$1 OR username=$1`,
				sub,
			).Scan(&role)

			switch {
			case err == nil && role != "":
				// Authoritative DB role
				next.ServeHTTP(w, r.WithContext(rbac.WithRole(ctx, role)))
				return

			case errors.Is(err, sql.ErrNoRows) || isUsersTableMissing(err):
				// Dev fallback to claim
				if claimRole == "admin" || (allowClaimFallback && claimRole != "") {
					next.ServeHTTP(w, r) // keep whatever JWTMiddleware set
					return
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return

			case err != nil:
				// Unknown DB error: in dev, be lenient; in prod, deny
				if allowClaimFallback && claimRole != "" {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		})
	}
}

func isUsersTableMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table: users") || // sqlite
		strings.Contains(msg, `relation "users" does not exist`) // postgres
}
