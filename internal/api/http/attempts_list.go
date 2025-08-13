package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mind-engage/mindengage-lms/internal/exam"
	"github.com/mind-engage/mindengage-lms/internal/rbac"
)

// GET /attempts?exam_id=...&user_id=...&status=...&limit=50&offset=0&sort=started_at+desc
// RBAC:
// - role with attempt:view-all can list any filters
// - role with attempt:view-own can only see their own attempts (user_id is forced to subject)
func ListAttemptsHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := rbac.RoleFromContext(r.Context())
		sub := rbac.SubjectFromContext(r.Context())

		// parse inputs
		examID := strings.TrimSpace(r.URL.Query().Get("exam_id"))
		userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		sort := strings.TrimSpace(r.URL.Query().Get("sort"))
		limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
		offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

		// enforce RBAC scoping
		if role == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// teachers/admins: attempt:view-all (handled by router). Students: attempt:view-own only.
		// If caller does NOT have attempt:view-all, force user_id to their own subject.
		if role != "admin" && role != "teacher" {
			userID = sub
		}

		list, err := store.ListAttempts(r.Context(), exam.AttemptListOpts{
			ExamID: examID,
			UserID: userID,
			Status: status,
			Limit:  limit,
			Offset: offset,
			Sort:   sort,
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	}
}
