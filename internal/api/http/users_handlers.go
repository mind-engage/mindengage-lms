package http

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type userRow struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`               // usually "student"
	Password string `json:"password,omitempty"` // plaintext optional (LAN-only)
}

func BulkUpsertUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Accept either multipart file= (CSV/JSON) OR raw JSON array in body
		var rows []userRow
		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/form-data") {
			f, _, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "file required", 400)
				return
			}
			defer f.Close()
			// sniff simple CSV vs JSON by first non-space byte
			buf := make([]byte, 1)
			if _, err := f.Read(buf); err != nil {
				http.Error(w, "empty file", 400)
				return
			}
			if _, err := f.(io.Seeker).Seek(0, io.SeekStart); err != nil {
			}
			if buf[0] == '[' || buf[0] == '{' {
				if err := json.NewDecoder(f).Decode(&rows); err != nil {
					http.Error(w, "bad json", 400)
					return
				}
			} else {
				rs, err := parseCSV(f)
				if err != nil {
					http.Error(w, "bad csv: "+err.Error(), 400)
					return
				}
				rows = rs
			}
		} else {
			if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
				http.Error(w, "expected JSON array or multipart file", 400)
				return
			}
		}
		if len(rows) == 0 {
			_ = json.NewEncoder(w).Encode(map[string]any{"inserted": 0, "updated": 0})
			return
		}

		ins, upd, err := upsertUsers(r.Context(), db, rows)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"inserted": ins, "updated": upd})
	}
}

func ListUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := r.URL.Query().Get("role")
		var rows *sql.Rows
		var err error
		if role == "" {
			rows, err = db.QueryContext(r.Context(), `SELECT id,username,role FROM users ORDER BY username`)
		} else {
			rows, err = db.QueryContext(r.Context(), `SELECT id,username,role FROM users WHERE role=$1 ORDER BY username`, role)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()
		out := []map[string]string{}
		for rows.Next() {
			var id, u, role string
			if err := rows.Scan(&id, &u, &role); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			out = append(out, map[string]string{"id": id, "username": u, "role": role})
		}
		_ = json.NewEncoder(w).Encode(out)
	}
}

func parseCSV(r io.Reader) ([]userRow, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	hdr, err := cr.Read()
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	for i, h := range hdr {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	required := []string{"id", "username", "role"}
	for _, k := range required {
		if _, ok := idx[k]; !ok {
			return nil, errors.New("missing column: " + k)
		}
	}
	var rows []userRow
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		row := userRow{
			ID:       rec[idx["id"]],
			Username: rec[idx["username"]],
			Role:     strings.ToLower(rec[idx["role"]]),
		}
		if i, ok := idx["password"]; ok {
			row.Password = rec[i]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func upsertUsers(ctx context.Context, db *sql.DB, rows []userRow) (inserted, updated int, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	now := time.Now().Unix()
	for _, r := range rows {
		if r.Role == "" {
			r.Role = "student"
		}
		if r.Role != "student" && r.Role != "teacher" && r.Role != "admin" {
			return inserted, updated, errors.New("invalid role: " + r.Role)
		}
		// Hash password if provided (LAN-only flow). If empty, keep existing hash or reject if new.
		var phash string
		if r.Password != "" {
			b, e := bcrypt.GenerateFromPassword([]byte(r.Password), 12)
			if e != nil {
				return inserted, updated, e
			}
			phash = string(b)
		}

		// Upsert: if exists update (optionally password), else insert (password required)
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT 1 FROM users WHERE id=$1 OR username=$2`, r.ID, r.Username).Scan(new(int)); err == nil {
			exists = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return inserted, updated, err
		}
		if exists {
			if phash != "" {
				_, err = tx.ExecContext(ctx, `UPDATE users SET username=$1, role=$2, password_hash=$3 WHERE id=$4`,
					r.Username, r.Role, phash, r.ID)
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE users SET username=$1, role=$2 WHERE id=$3`,
					r.Username, r.Role, r.ID)
			}
			if err != nil {
				return inserted, updated, err
			}
			updated++
		} else {
			if phash == "" {
				return inserted, updated, errors.New("password required for new user: " + r.Username)
			}
			_, err = tx.ExecContext(ctx,
				`INSERT INTO users (id, username, password_hash, role, created_at) VALUES ($1,$2,$3,$4,$5)`,
				r.ID, r.Username, phash, r.Role, now)
			if err != nil {
				return inserted, updated, err
			}
			inserted++
		}
	}
	return
}

// Optional: enforce that FormFile is present (nice UX)
func requireFile(r *http.Request, field string) (multipart.File, *multipart.FileHeader, error) {
	f, h, err := r.FormFile(field)
	if err != nil {
		return nil, nil, errors.New("missing file field: " + field)
	}
	return f, h, nil
}
