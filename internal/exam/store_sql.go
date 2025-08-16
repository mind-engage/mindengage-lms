package exam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mind-engage/mindengage-lms/internal/grading"
	syncx "github.com/mind-engage/mindengage-lms/internal/sync"
)

// SQLStore persists exams/attempts in SQL (SQLite or Postgres).
type SQLStore struct {
	db     *sql.DB
	driver string // "sqlite" or "postgres"
	grader grading.Grader
}

func NewSQLStore(db *sql.DB, driver string, grader grading.Grader) *SQLStore {
	return &SQLStore{db: db, driver: driver, grader: grader}
}

/* ------------------------- Exams ------------------------- */

func (s *SQLStore) PutExam(e Exam) error {
	// sanitize
	if e.TimeLimitSec < 0 {
		e.TimeLimitSec = 0
	}
	qj, err := json.Marshal(e.Questions)
	if err != nil {
		return err
	}
	// Persist profile + policy_json as well
	var pjson string
	if len(e.PolicyRaw) > 0 {
		pjson = string(e.PolicyRaw)
	}
	_, err = s.db.Exec(`
		INSERT INTO exams (id,title,time_limit_sec,questions_json,created_at,profile,policy_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET
			title=EXCLUDED.title,
			time_limit_sec=EXCLUDED.time_limit_sec,
			questions_json=EXCLUDED.questions_json,
			profile=EXCLUDED.profile,
			policy_json=EXCLUDED.policy_json
	`,
		e.ID, e.Title, e.TimeLimitSec, string(qj), time.Now().Unix(), e.Profile, pjson)
	return err
}

func (s *SQLStore) GetExam(id string) (Exam, error) {
	row := s.db.QueryRow(`
		SELECT id, title, time_limit_sec, questions_json, created_at, profile, policy_json
		FROM exams WHERE id = $1
	`, id)

	var e Exam
	var qjson, pjson string

	if err := row.Scan(&e.ID, &e.Title, &e.TimeLimitSec, &qjson, &e.CreatedAt, &e.Profile, &pjson); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Exam{}, errors.New("exam not found")
		}
		return Exam{}, err
	}

	if err := json.Unmarshal([]byte(qjson), &e.Questions); err != nil {
		return Exam{}, err
	}

	// Include policy for the client (e.g., module_locked), if present (non-empty)
	if strings.TrimSpace(pjson) != "" {
		e.PolicyRaw = json.RawMessage(pjson)
	}

	// Strip answer keys for student response
	for i := range e.Questions {
		e.Questions[i].AnswerKey = nil
	}

	return e, nil
}

// Admin fetch: returns full exam (including answer keys), plus profile/policy for exports/timing logic.
func (s *SQLStore) GetExamAdmin(ctx context.Context, id string) (Exam, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, time_limit_sec, questions_json, created_at, profile, policy_json
		FROM exams WHERE id=$1`, id)
	var e Exam
	var qjson, pjson string
	if err := row.Scan(&e.ID, &e.Title, &e.TimeLimitSec, &qjson, &e.CreatedAt, &e.Profile, &pjson); err != nil {
		return Exam{}, err
	}
	if err := json.Unmarshal([]byte(qjson), &e.Questions); err != nil {
		return Exam{}, err
	}
	if pjson != "" {
		e.PolicyRaw = json.RawMessage(pjson)
	}
	return e, nil
}

// ListExams returns student-safe summaries. Title filter optional.
func (s *SQLStore) ListExams(ctx context.Context, opts ListOpts) ([]ExamSummary, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, time_limit_sec, created_at, profile
		FROM exams
		WHERE ($1 = '' OR LOWER(title) LIKE LOWER('%' || $1 || '%'))
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, strings.TrimSpace(opts.Q), opts.Limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ExamSummary{}
	for rows.Next() {
		var e ExamSummary
		if err := rows.Scan(&e.ID, &e.Title, &e.TimeLimitSec, &e.CreatedAt, &e.Profile); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

/* ------------------------ Attempts ------------------------ */

func (s *SQLStore) NewAttempt(examID, userID string) (Attempt, error) {
	// Ensure exam exists & load admin for policy/timing.
	ex, err := s.GetExamAdmin(context.Background(), examID)
	if err != nil {
		// Normalize "not found"
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("exam not found")
		}
		return Attempt{}, err
	}

	// Compute module timings from policy (if any)
	modules := extractModuleTimes(ex.PolicyRaw) // []int (seconds)
	now := time.Now().Unix()
	overall := int64(0)
	for _, sec := range modules {
		if sec > 0 {
			overall += int64(sec)
		}
	}
	var firstMod int64
	if len(modules) > 0 && modules[0] > 0 {
		firstMod = int64(modules[0])
	}

	id := time.Now().Format("20060102150405")
	resp := map[string]interface{}{}
	respJSON, _ := json.Marshal(resp)

	_, err = s.db.Exec(`
		INSERT INTO attempts (
			id, exam_id, user_id, status, score, responses_json, started_at,
			module_index, module_started_at, module_deadline, overall_deadline
		)
		VALUES ($1,$2,$3,'in_progress',0,$4,$5,$6,$7,$8,$9)
	`,
		id, examID, userID, string(respJSON), now,
		0, now, nullableDeadline(now, firstMod), nullableDeadline(now, overall),
	)
	if err != nil {
		return Attempt{}, err
	}
	return Attempt{
		ID:        id,
		ExamID:    examID,
		UserID:    userID,
		Status:    "in_progress",
		Score:     0,
		Responses: resp,
		StartedAt: now,
	}, nil
}

func (s *SQLStore) SaveResponses(attemptID string, resp map[string]interface{}) (Attempt, error) {
	// Load attempt (with timing columns for enforcement)
	var a Attempt
	var rjson string
	var moduleIdx int
	var moduleStarted, moduleDeadline, overallDeadline sql.NullInt64

	row := s.db.QueryRow(`
		SELECT id, exam_id, user_id, status, score, responses_json,
		       module_index, module_started_at, module_deadline, overall_deadline
		FROM attempts WHERE id=$1`, attemptID)
	if err := row.Scan(&a.ID, &a.ExamID, &a.UserID, &a.Status, &a.Score, &rjson,
		&moduleIdx, &moduleStarted, &moduleDeadline, &overallDeadline); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("attempt not found")
		}
		return Attempt{}, err
	}
	if err := json.Unmarshal([]byte(rjson), &a.Responses); err != nil || a.Responses == nil {
		a.Responses = map[string]interface{}{}
	}

	// Enforce timing (server-side source of truth)
	now := time.Now().Unix()
	if overallDeadline.Valid && now > overallDeadline.Int64 {
		return Attempt{}, errors.New("time over: overall deadline reached")
	}
	if moduleDeadline.Valid && now > moduleDeadline.Int64 {
		return Attempt{}, errors.New("time over: module deadline reached")
	}

	if a.Status == "submitted" {
		return Attempt{}, errors.New("attempt already submitted")
	}

	// merge responses
	for k, v := range resp {
		a.Responses[k] = v
	}
	buf, _ := json.Marshal(a.Responses)

	_, err := s.db.Exec(`UPDATE attempts SET responses_json=$1 WHERE id=$2`, string(buf), attemptID)
	if err != nil {
		return Attempt{}, err
	}
	return s.GetAttempt(attemptID)
}

func (s *SQLStore) Submit(attemptID string) (Attempt, error) {
	a, err := s.GetAttempt(attemptID)
	if err != nil {
		return Attempt{}, err
	}
	if a.Status == "submitted" {
		return a, nil
	}

	// load full exam WITH keys for grading
	row := s.db.QueryRow(`SELECT questions_json FROM exams WHERE id=$1`, a.ExamID)
	var qjson string
	if err := row.Scan(&qjson); err != nil {
		return Attempt{}, err
	}
	var questions []Question
	if err := json.Unmarshal([]byte(qjson), &questions); err != nil {
		return Attempt{}, err
	}

	ctx := context.Background()
	score := 0.0
	for _, q := range questions {
		resp, has := a.Responses[q.ID]
		if !has {
			continue
		}
		gq := grading.Q{Type: q.Type, Points: q.Points, AnswerKey: q.AnswerKey}
		res, err := s.grader.Grade(ctx, gq, resp)
		if err != nil {
			continue
		}
		score += res.AutoPoints
	}

	a.Score = score
	a.Status = "submitted"
	buf, _ := json.Marshal(a.Responses)
	now := time.Now().Unix()
	_, err = s.db.Exec(`UPDATE attempts SET status='submitted', score=$1, responses_json=$2, submitted_at=$3 WHERE id=$4`,
		a.Score, string(buf), now, attemptID)
	if err != nil {
		return Attempt{}, err
	}

	_ = syncx.NewEventRepo(s.db).Append(context.Background(), syncx.Event{
		SiteID:   "local", // later: cfg.SiteID
		Type:     "AttemptSubmitted",
		Key:      attemptID,
		DataJSON: string(buf), // responses; include more if desired
	})

	return s.GetAttempt(attemptID)
}

func (s *SQLStore) GetAttempt(id string) (Attempt, error) {
	row := s.db.QueryRow(`SELECT id,exam_id,user_id,status,score,responses_json,started_at,submitted_at FROM attempts WHERE id=$1`, id)
	var a Attempt
	var rjson string
	if err := row.Scan(&a.ID, &a.ExamID, &a.UserID, &a.Status, &a.Score, &rjson, &a.StartedAt, &a.SubmittedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("attempt not found")
		}
		return Attempt{}, err
	}
	if err := json.Unmarshal([]byte(rjson), &a.Responses); err != nil {
		a.Responses = map[string]interface{}{}
	}
	return a, nil
}

/* ------------------ Multi-module support ------------------ */

func (s *SQLStore) AdvanceModule(attemptID string) (Attempt, error) {
	// load attempt (need exam_id and current module_index + deadlines)
	var a Attempt
	var rjson string
	var moduleIdx int
	row := s.db.QueryRow(`SELECT exam_id, responses_json, module_index FROM attempts WHERE id=$1`, attemptID)
	if err := row.Scan(&a.ExamID, &rjson, &moduleIdx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("attempt not found")
		}
		return Attempt{}, err
	}
	_ = json.Unmarshal([]byte(rjson), &a.Responses)

	// load exam policy
	ex, err := s.GetExamAdmin(context.Background(), a.ExamID)
	if err != nil {
		return Attempt{}, err
	}
	modules := extractModuleTimes(ex.PolicyRaw)
	if len(modules) == 0 {
		return Attempt{}, errors.New("no modules in policy")
	}
	if moduleIdx+1 >= len(modules) {
		return Attempt{}, errors.New("already at last module")
	}

	// advance
	nextIdx := moduleIdx + 1
	now := time.Now().Unix()
	nextDur := int64(0)
	if modules[nextIdx] > 0 {
		nextDur = int64(modules[nextIdx])
	}
	_, err = s.db.Exec(`
		UPDATE attempts
		SET module_index=$1, module_started_at=$2, module_deadline=$3
		WHERE id=$4`,
		nextIdx, now, nullableDeadline(now, nextDur), attemptID)
	if err != nil {
		return Attempt{}, err
	}
	// return fresh view (without timing fields in struct)
	return s.GetAttempt(attemptID)
}

/* ---------------------- Attempt listing ------------------- */

func (s *SQLStore) ListAttempts(ctx context.Context, opts AttemptListOpts) ([]Attempt, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	where := []string{"1=1"}
	args := []any{}
	i := 1
	if strings.TrimSpace(opts.ExamID) != "" {
		where = append(where, fmt.Sprintf("exam_id=$%d", i))
		args = append(args, strings.TrimSpace(opts.ExamID))
		i++
	}
	if strings.TrimSpace(opts.UserID) != "" {
		where = append(where, fmt.Sprintf("user_id=$%d", i))
		args = append(args, strings.TrimSpace(opts.UserID))
		i++
	}
	if strings.TrimSpace(opts.Status) != "" {
		where = append(where, fmt.Sprintf("status=$%d", i))
		args = append(args, strings.TrimSpace(opts.Status))
		i++
	}
	order := "started_at DESC"
	switch strings.ToLower(strings.TrimSpace(opts.Sort)) {
	case "submitted_at asc":
		order = "submitted_at ASC NULLS LAST"
	case "submitted_at desc":
		order = "submitted_at DESC NULLS LAST"
	case "started_at asc":
		order = "started_at ASC"
	case "started_at desc", "":
		order = "started_at DESC"
	}

	q := fmt.Sprintf(`
		SELECT id, exam_id, user_id, status, score, responses_json, started_at, submitted_at
		FROM attempts
		WHERE %s
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, strings.Join(where, " AND "), order, opts.Limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Attempt{}
	for rows.Next() {
		var a Attempt
		var rjson string
		if err := rows.Scan(&a.ID, &a.ExamID, &a.UserID, &a.Status, &a.Score, &rjson, &a.StartedAt, &a.SubmittedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(rjson), &a.Responses); err != nil {
			a.Responses = map[string]interface{}{}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

/* ------------------------- Helpers ------------------------ */

func extractModuleTimes(policyRaw json.RawMessage) []int {
	if len(policyRaw) == 0 {
		return nil
	}
	// Minimal inline struct to avoid importing formats package here.
	var pol struct {
		Sections []struct {
			Modules []struct {
				TimeLimitSec int `json:"time_limit_sec"`
			} `json:"modules"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(policyRaw, &pol); err != nil {
		return nil
	}
	out := make([]int, 0, 8)
	for _, s := range pol.Sections {
		for _, m := range s.Modules {
			out = append(out, m.TimeLimitSec)
		}
	}
	return out
}

func nullableDeadline(start int64, dur int64) *int64 {
	if dur <= 0 {
		return nil // stored as NULL
	}
	sum := start + dur
	return &sum
}
