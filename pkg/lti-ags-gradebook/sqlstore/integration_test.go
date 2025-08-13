package sqlstore_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mind-engage/mindengage-lms/pkg/lti-ags-gradebook/agshttp"
	gb "github.com/mind-engage/mindengage-lms/pkg/lti-ags-gradebook/gradebook"
	"github.com/mind-engage/mindengage-lms/pkg/lti-ags-gradebook/sqlstore"

	"database/sql"

	_ "modernc.org/sqlite" // driver for "sqlite"
)

type Execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// create the minimal app tables (not part of gradebook migration) and seed content.
func ensureAppTablesAndSeed(t *testing.T, db Execer, lineItemsURL, tokenURL string) {
	t.Helper()

	// --- App tables (not part of gradebook migration) ---
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS exams (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  max_points REAL NOT NULL
);
CREATE TABLE IF NOT EXISTS attempts (
  id TEXT PRIMARY KEY,
  exam_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  score REAL NOT NULL,
  submitted_at DATETIME,
  platform_issuer TEXT,
  deployment_id TEXT,
  context_id TEXT,
  resource_link_id TEXT
);`); err != nil {
		t.Fatalf("create app tables: %v", err)
	}

	// --- Seed exam + attempt ---
	if _, err := db.Exec(`INSERT INTO exams (id,title,max_points) VALUES ('exam-1','Exam One',100)`); err != nil {
		t.Fatalf("seed exams: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO attempts
      (id, exam_id, user_id, score, submitted_at, platform_issuer, deployment_id, context_id, resource_link_id)
      VALUES ('attempt-1','exam-1','u1',80, CURRENT_TIMESTAMP, 'iss-1','dep-1','ctx-1','rl-1')`); err != nil {
		t.Fatalf("seed attempts: %v", err)
	}

	// --- Seed LTI platform FIRST (satisfies FK on lti_links.platform_issuer) ---
	if _, err := db.Exec(`
INSERT INTO lti_platforms (issuer, client_id, token_url, jwks_url, auth_url)
VALUES ('iss-1', 'test-client', ?, ?, ?)
ON CONFLICT(issuer) DO UPDATE SET
  client_id=excluded.client_id,
  token_url=excluded.token_url,
  jwks_url=excluded.jwks_url,
  auth_url=excluded.auth_url
`, tokenURL, "http://example.test/.well-known/jwks.json", "http://example.test/oauth/authorize"); err != nil {
		t.Fatalf("seed lti_platforms: %v", err)
	}

	// --- Seed link + user map ---
	if _, err := db.Exec(`
INSERT INTO lti_links
  (platform_issuer, deployment_id, context_id, resource_link_id, lineitems_url, scopes)
VALUES ('iss-1','dep-1','ctx-1','rl-1', ?, '["lineitem","score"]')
ON CONFLICT(platform_issuer, deployment_id, context_id, resource_link_id)
DO UPDATE SET lineitems_url=excluded.lineitems_url
`, lineItemsURL); err != nil {
		t.Fatalf("seed lti_links: %v", err)
	}

	if _, err := db.Exec(`
INSERT INTO lti_user_map (platform_issuer, platform_sub, local_user_id)
VALUES ('iss-1','platform-sub-123','u1')
ON CONFLICT(platform_issuer, platform_sub) DO NOTHING
`); err != nil {
		t.Fatalf("seed lti_user_map: %v", err)
	}
}

// Execer lets us pass *sql.DB or *sql.Tx

func Test_EndToEnd_SQLite_WithHTTPAGS(t *testing.T) {
	ctx := context.Background()

	// 1) DB with gradebook migrations applied
	db, err := gb.ConnectAndMigrate(ctx, "sqlite", "file:test.db?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	st := &sqlstore.Store{DB: db}

	// 2) Fake AGS server (token + lineitems + scores)
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// clientcredentials.Config sends application/x-www-form-urlencoded with grant_type
		if err := r.ParseForm(); err != nil {
			t.Fatalf("token: ParseForm: %v", err)
		}
		if r.PostForm.Get("grant_type") != "client_credentials" {
			t.Fatalf("token: unexpected grant_type=%q", r.PostForm.Get("grant_type"))
		}
		// (Optional) assert client_id/secret are present if you want stricter tests:
		// if r.PostForm.Get("client_id") != "x" || r.PostForm.Get("client_secret") != "y" { ... }

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
	})

	// collection
	mux.HandleFunc("/lti/lineitems", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.Header().Set("Content-Type", "application/vnd.ims.lis.v2.lineitem+json")
			_ = json.NewEncoder(w).Encode([]any{})
		case "POST":
			w.Header().Set("Content-Type", "application/vnd.ims.lis.v2.lineitem+json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             "http://" + r.Host + "/lti/lineitems/123", // <-- add scheme
				"label":          "Exam One",
				"scoreMaximum":   100,
				"resourceId":     "exam-1",
				"resourceLinkId": "rl-1",
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// item scores
	mux.HandleFunc("/lti/lineitems/123/scores", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct == "" {
			t.Fatalf("missing Content-Type for score post")
		}
		w.WriteHeader(200)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 3) Create app tables + seed rows (after we know the lineitems URL)
	lineItemsURL := ts.URL + "/lti/lineitems"
	tokenURL := ts.URL + "/oauth/token"
	ensureAppTablesAndSeed(t, st.DB, lineItemsURL, tokenURL)

	// 4) AGS client using the fake token endpoint
	ags := agshttp.New(agshttp.Config{
		TokenURL:     ts.URL + "/oauth/token",
		ClientID:     "x",
		ClientSecret: "y",
		Timeout:      5 * time.Second,
	})

	// 5) Syncer and run
	syncer := gb.New(st, ags, time.Now)
	if err := syncer.SyncAttempt("attempt-1"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}
