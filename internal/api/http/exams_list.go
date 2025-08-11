package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/mind-engage/mindengage-lms/internal/exam"
)

func ListExamsHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
		offset := parseIntDefault(r.URL.Query().Get("offset"), 0)

		list, err := store.ListExams(r.Context(), exam.ListOpts{
			Q:      q,
			Limit:  limit,
			Offset: offset,
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
