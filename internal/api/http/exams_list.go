// internal/api/http/exams_list.go
package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	authmw "github.com/mind-engage/mindengage-lms/internal/auth/middleware"
	"github.com/mind-engage/mindengage-lms/internal/exam"
)

func ListExamsHandler(store exam.Store, authSvc *authmw.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
		offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

		// Extract viewer from the already-validated JWT
		var viewerID, viewerRole string
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			if claims, err := authSvc.Parse(strings.TrimPrefix(h, "Bearer ")); err == nil {
				viewerID = claims.Sub
				viewerRole = claims.Role
			}
		}

		list, err := store.ListExams(r.Context(), exam.ListOpts{
			Q:          q,
			Limit:      limit,
			Offset:     offset,
			ViewerID:   strings.TrimSpace(viewerID),
			ViewerRole: strings.TrimSpace(viewerRole),
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	}
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil && v >= 0 {
		return v
	}
	return def
}
