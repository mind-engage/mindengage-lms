package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mind-engage/mindengage-lms/internal/exam"
	"github.com/mind-engage/mindengage-lms/internal/qti"
	"github.com/mind-engage/mindengage-lms/internal/qti/export"
	"github.com/mind-engage/mindengage-lms/internal/qti/parser"
	"github.com/mind-engage/mindengage-lms/internal/storage"
)

// POST /qti/import (multipart: file=package.zip)
func ImportQTIHandler(store exam.Store, bs storage.BlobStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f, hdr, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file required", 400)
			return
		}
		defer f.Close()

		// read into temp to get ReaderAt+size for unzip
		tmp, err := os.CreateTemp("", "qti-upload-*")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer os.Remove(tmp.Name())
		if _, err := io.Copy(tmp, f); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		info, _ := tmp.Stat()
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		base, err := parser.UnzipToTemp(tmp, info.Size())
		if err != nil {
			http.Error(w, "unzip: "+err.Error(), 400)
			return
		}
		defer os.RemoveAll(base)

		mf, itemFiles, err := parser.ParseManifest(base)
		if err != nil {
			http.Error(w, "manifest: "+err.Error(), 400)
			return
		}

		parsed := []parser.ParsedItem{}
		for _, rel := range itemFiles {
			it, err := parser.ParseItemFile(base, rel)
			if err != nil {
				continue
			} // skip unsupported for MVP
			parsed = append(parsed, it)
		}

		// media rewrite: in MVP we just pass through prompt HTML (assets stay inside package)
		ex, err := qti.MapToExam(mf, parsed, qti.NoopRewrite)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// give imported exam a stable ID if none supplied
		if ex.ID == "" {
			ex.ID = "exam-" + time.Now().Format("20060102150405")
		}

		if err := store.PutExam(ex); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"exam_id": ex.ID, "filename": hdr.Filename})
	}
}

// GET /exams/{id}/export?format=qti
func ExportQTIHandler(store exam.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		format := strings.ToLower(r.URL.Query().Get("format"))
		if format == "" {
			format = "qti"
		}

		// We need answer keys; use admin fetch when available.
		ex, err := store.GetExamAdmin(context.Background(), id)
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}

		pkg, err := export.BuildPackage(ex, func(path string) (io.ReadCloser, error) {
			// For future media inclusion; not used in MVP.
			return nil, os.ErrNotExist
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+id+".zip\"")
		http.ServeContent(w, r, id+".zip", time.Now(), bytesReader(pkg))
	}
}

func bytesReader(b []byte) io.ReadSeeker {
	return nopCloserSeeker{r: bytes.NewReader(b)}
}

type nopCloserSeeker struct{ r *bytes.Reader }

func (n nopCloserSeeker) Read(p []byte) (int, error)         { return n.r.Read(p) }
func (n nopCloserSeeker) Seek(o int64, w int) (int64, error) { return n.r.Seek(o, w) }
