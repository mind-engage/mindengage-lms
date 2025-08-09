// internal/api/http/assets.go
package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mind-engage/mindengage-lms/internal/storage"
)

func MountAssets(r chi.Router, bs storage.BlobStore) {
	// POST /assets/{attemptID}
	r.Post("/{attemptID}", func(w http.ResponseWriter, r *http.Request) {
		attemptID := chi.URLParam(r, "attemptID")
		f, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file required", http.StatusBadRequest)
			return
		}
		defer f.Close()

		key := "attempts/" + attemptID + "/upload.bin"
		if _, err := bs.Put(key, f); err != nil {
			http.Error(w, "store error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"key": key})
	})

	// GET /assets/*   -> returns the blob at whatever follows /assets/
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		key := chi.URLParam(r, "*")        // everything after /assets/
		key = strings.TrimPrefix(key, "/") // normalize
		rc, err := bs.Get(key)
		if err != nil {
			http.Error(w, "not found: "+err.Error(), http.StatusNotFound)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.Copy(w, rc)
	})
}
