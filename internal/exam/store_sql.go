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

var (
	ErrAttemptSubmitted   = errors.New("attempt already submitted")
	ErrOutsideModule      = errors.New("outside current module window")
	ErrBackwardNavBlocked = errors.New("backward navigation blocked")
	ErrEditBackBlocked    = errors.New("editing a locked (past) question")
	ErrTimeOver           = errors.New("time over")
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
	// --- unchanged prelude: load exam (admin view) for policy/timing ---
	ex, err := s.GetExamAdmin(context.Background(), examID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("exam not found")
		}
		return Attempt{}, err
	}

	// Compute module timings from policy (if any), with fallback to overall time_limit_sec
	modules := extractModuleTimes(ex.PolicyRaw) // []int (seconds)
	if len(modules) == 0 && ex.TimeLimitSec > 0 {
		modules = []int{ex.TimeLimitSec}
	}

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

	// --- nav defaults (NEW) ---
	startIdx := 0
	nav := parseNavPolicy(ex.PolicyRaw)
	modIDs := extractModuleIDs(ex.PolicyRaw)
	if nav.ModuleLocked && len(modIDs) > 0 {
		win := moduleWindowFor(ex, modIDs[0])
		if win.hasAny {
			startIdx = win.firstIdx
		}
	}

	// --- persist attempt ---
	id := time.Now().Format("20060102150405")
	resp := map[string]interface{}{}
	respJSON, _ := json.Marshal(resp)

	_, err = s.db.Exec(`
		INSERT INTO attempts (
			id, exam_id, user_id, status, score, responses_json, started_at,
			module_index, module_started_at, module_deadline, overall_deadline,
			current_index, max_reached_index
		)
		VALUES ($1,$2,$3,'in_progress',0,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		id, examID, userID, string(respJSON), now,
		0, now, nullableDeadline(now, firstMod), nullableDeadline(now, overall),
		startIdx, startIdx,
	)
	if err != nil {
		return Attempt{}, err
	}

	// Return a basic view; clients can call GetAttempt to fetch full timing fields
	return Attempt{
		ID:              id,
		ExamID:          examID,
		UserID:          userID,
		Status:          "in_progress",
		Score:           0,
		Responses:       resp,
		StartedAt:       now,
		ModuleIndex:     0,
		ModuleStartedAt: now,
	}, nil
}

func (s *SQLStore) SaveResponses(attemptID string, resp map[string]interface{}) (Attempt, error) {
	// Load attempt (with timing columns for enforcement)
	var a Attempt
	var rjson string
	var moduleIdx, curIdx, maxIdx int // NEW: cur/max
	var moduleStarted, moduleDeadline, overallDeadline sql.NullInt64

	row := s.db.QueryRow(`
	  SELECT id, exam_id, user_id, status, score, responses_json,
			 module_index, module_started_at, module_deadline, overall_deadline,
			 current_index, max_reached_index
	  FROM attempts WHERE id=$1`, attemptID)
	if err := row.Scan(&a.ID, &a.ExamID, &a.UserID, &a.Status, &a.Score, &rjson,
		&moduleIdx, &moduleStarted, &moduleDeadline, &overallDeadline,
		&curIdx, &maxIdx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("attempt not found")
		}
		return Attempt{}, err
	}
	if err := json.Unmarshal([]byte(rjson), &a.Responses); err != nil || a.Responses == nil {
		a.Responses = map[string]interface{}{}
	}

	// timing guards (unchanged)
	now := time.Now().Unix()
	if overallDeadline.Valid && now > overallDeadline.Int64 {
		return Attempt{}, ErrTimeOver
	}
	if moduleDeadline.Valid && now > moduleDeadline.Int64 {
		return Attempt{}, ErrTimeOver
	}
	if a.Status == "submitted" {
		return Attempt{}, ErrAttemptSubmitted
	}

	// Load exam/policy for enforcement
	ex, err := s.GetExamAdmin(context.Background(), a.ExamID)
	if err != nil {
		return Attempt{}, err
	}
	nav := parseNavPolicy(ex.PolicyRaw)

	// Module lock (as you already had)
	if nav.ModuleLocked {
		allowed := allowedQIDsForModule(ex, moduleIdx)
		if allowed != nil {
			for k := range resp {
				if _, ok := allowed[k]; !ok {
					return Attempt{}, ErrOutsideModule
				}
			}
		}
	}

	// NEW: forward-only editing guard when allow_back=false
	if !nav.AllowBack {
		qidToIdx, _, _ := buildIndexMaps(ex.Questions)
		for k := range resp {
			if idx, ok := qidToIdx[k]; ok && idx < curIdx {
				return Attempt{}, ErrEditBackBlocked
			}
		}
	}

	// merge + save (unchanged)
	for k, v := range resp {
		a.Responses[k] = v
	}
	buf, _ := json.Marshal(a.Responses)
	if _, err := s.db.Exec(`UPDATE attempts SET responses_json=$1 WHERE id=$2`, string(buf), attemptID); err != nil {
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
	row := s.db.QueryRow(`SELECT id,exam_id,user_id,status,score,responses_json,started_at,submitted_at,
	  module_index, COALESCE(module_started_at,0), COALESCE(module_deadline,0), COALESCE(overall_deadline,0),
	  current_index, max_reached_index
	  FROM attempts WHERE id=$1`, id)

	var a Attempt
	var rjson string
	var moduleStarted, moduleDeadline, overallDeadline int64
	if err := row.Scan(&a.ID, &a.ExamID, &a.UserID, &a.Status, &a.Score, &rjson, &a.StartedAt, &a.SubmittedAt,
		&a.ModuleIndex, &moduleStarted, &moduleDeadline, &overallDeadline,
		&a.CurrentIndex, &a.MaxReachedIndex); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("attempt not found")
		}
		return Attempt{}, err
	}
	if err := json.Unmarshal([]byte(rjson), &a.Responses); err != nil {
		a.Responses = map[string]interface{}{}
	}
	if moduleStarted > 0 {
		a.ModuleStartedAt = moduleStarted
	}
	if moduleDeadline > 0 {
		a.ModuleDeadline = moduleDeadline
	}
	if overallDeadline > 0 {
		a.OverallDeadline = overallDeadline
	}

	// remaining seconds (unchanged logic)
	now := time.Now().Unix()
	rem := 0
	if a.ModuleDeadline > 0 {
		if d := int(a.ModuleDeadline - now); d > 0 {
			rem = d
		}
	}
	if a.OverallDeadline > 0 {
		if d := int(a.OverallDeadline - now); d > 0 {
			if rem == 0 || d < rem {
				rem = d
			}
		}
	}
	a.RemainingSeconds = rem
	return a, nil
}

/* ------------------ Multi-module support ------------------ */

func (s *SQLStore) AdvanceModule(attemptID string) (Attempt, error) {
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

	ex, err := s.GetExamAdmin(context.Background(), a.ExamID)
	if err != nil {
		return Attempt{}, err
	}
	modules := extractModuleTimes(ex.PolicyRaw)
	if len(modules) == 0 && ex.TimeLimitSec > 0 {
		modules = []int{ex.TimeLimitSec}
	}
	if len(modules) == 0 {
		return Attempt{}, errors.New("no modules in policy")
	}
	if moduleIdx+1 >= len(modules) {
		return Attempt{}, errors.New("already at last module")
	}

	nextIdx := moduleIdx + 1
	now := time.Now().Unix()
	nextDur := int64(0)
	if modules[nextIdx] > 0 {
		nextDur = int64(modules[nextIdx])
	}

	// Compute first question index of next module (if any)
	modIDs := extractModuleIDs(ex.PolicyRaw)
	cur := 0
	if nextIdx >= 0 && nextIdx < len(modIDs) {
		win := moduleWindowFor(ex, modIDs[nextIdx])
		if win.hasAny {
			cur = win.firstIdx
		}
	}

	_, err = s.db.Exec(`
	  UPDATE attempts
	  SET module_index=$1, module_started_at=$2, module_deadline=$3,
		  current_index=$4, max_reached_index=$4
	  WHERE id=$5`,
		nextIdx, now, nullableDeadline(now, nextDur),
		cur, attemptID,
	)
	if err != nil {
		return Attempt{}, err
	}
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

// NEW: extract ordered module IDs from policy to align with Question.ModuleID
func extractModuleIDs(policyRaw json.RawMessage) []string {
	if len(policyRaw) == 0 {
		return nil
	}
	var pol struct {
		Sections []struct {
			Modules []struct {
				ID string `json:"id"`
			} `json:"modules"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(policyRaw, &pol); err != nil {
		return nil
	}
	out := make([]string, 0, 8)
	for _, s := range pol.Sections {
		for _, m := range s.Modules {
			out = append(out, strings.TrimSpace(m.ID))
		}
	}
	return out
}

// NEW: set of question IDs allowed in the given module index (nil => no restriction)
func allowedQIDsForModule(ex Exam, moduleIdx int) map[string]struct{} {
	modIDs := extractModuleIDs(ex.PolicyRaw)
	if len(modIDs) == 0 || moduleIdx < 0 || moduleIdx >= len(modIDs) {
		return nil // nothing to enforce
	}
	target := strings.TrimSpace(modIDs[moduleIdx])
	if target == "" {
		return nil // cannot enforce without an ID
	}
	set := map[string]struct{}{}
	for _, q := range ex.Questions {
		if strings.TrimSpace(q.ModuleID) == target {
			set[q.ID] = struct{}{}
		}
	}
	if len(set) == 0 {
		// If nothing matches, be permissive (avoid accidental lockout due to bad authoring)
		return nil
	}
	return set
}

func nullableDeadline(start int64, dur int64) *int64 {
	if dur <= 0 {
		return nil // stored as NULL
	}
	sum := start + dur
	return &sum
}

type navPolicy struct {
	AllowBack    bool `json:"allow_back"`
	ModuleLocked bool `json:"module_locked"`
}

func parseNavPolicy(policyRaw json.RawMessage) navPolicy {
	if len(policyRaw) == 0 {
		return navPolicy{AllowBack: true, ModuleLocked: false}
	}
	var p struct {
		Navigation struct {
			AllowBack    *bool `json:"allow_back"`
			ModuleLocked *bool `json:"module_locked"`
		} `json:"navigation"`
	}
	_ = json.Unmarshal(policyRaw, &p)
	np := navPolicy{AllowBack: true, ModuleLocked: false}
	if p.Navigation.AllowBack != nil {
		np.AllowBack = *p.Navigation.AllowBack
	}
	if p.Navigation.ModuleLocked != nil {
		np.ModuleLocked = *p.Navigation.ModuleLocked
	}
	return np
}

// Map question -> absolute index, question -> moduleID, and reverse index->qid.
func buildIndexMaps(questions []Question) (qidToIdx map[string]int, qidToMod map[string]string, idxToQID []string) {
	qidToIdx = make(map[string]int, len(questions))
	qidToMod = make(map[string]string, len(questions))
	idxToQID = make([]string, 0, len(questions))
	for i, q := range questions {
		qidToIdx[q.ID] = i
		qidToMod[q.ID] = q.ModuleID
		idxToQID = append(idxToQID, q.ID)
	}
	return
}

// Compute the absolute index set (and min/max) for the current module.
type moduleWindow struct {
	indices  map[int]struct{}
	firstIdx int
	lastIdx  int
	hasAny   bool
}

func moduleWindowFor(ex Exam, moduleID string) moduleWindow {
	qidToIdx, qidToMod, _ := buildIndexMaps(ex.Questions)
	win := moduleWindow{indices: map[int]struct{}{}, firstIdx: 0, lastIdx: 0, hasAny: false}
	for qid, mid := range qidToMod {
		if mid == moduleID {
			i := qidToIdx[qid]
			win.indices[i] = struct{}{}
			if !win.hasAny {
				win.firstIdx, win.lastIdx, win.hasAny = i, i, true
			} else {
				if i < win.firstIdx {
					win.firstIdx = i
				}
				if i > win.lastIdx {
					win.lastIdx = i
				}
			}
		}
	}
	return win
}

// Navigate moves the attempt cursor to target absolute question index.
func (s *SQLStore) Navigate(attemptID string, target int) (Attempt, error) {
	// load attempt core + nav
	var examID string
	var status string
	var moduleIdx, curIdx, maxIdx int
	var moduleDeadline, overallDeadline sql.NullInt64

	row := s.db.QueryRow(`
		SELECT exam_id, status, module_index, current_index, max_reached_index,
		       module_deadline, overall_deadline
		FROM attempts WHERE id=$1`, attemptID)
	if err := row.Scan(&examID, &status, &moduleIdx, &curIdx, &maxIdx, &moduleDeadline, &overallDeadline); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("attempt not found")
		}
		return Attempt{}, err
	}
	if status == "submitted" {
		return Attempt{}, ErrAttemptSubmitted
	}

	now := time.Now().Unix()
	if (moduleDeadline.Valid && now > moduleDeadline.Int64) || (overallDeadline.Valid && now > overallDeadline.Int64) {
		return Attempt{}, ErrTimeOver
	}

	// exam + policy
	ex, err := s.GetExamAdmin(context.Background(), examID)
	if err != nil {
		return Attempt{}, err
	}
	nav := parseNavPolicy(ex.PolicyRaw)
	modIDs := extractModuleIDs(ex.PolicyRaw)

	// window
	var curModID string
	if moduleIdx >= 0 && moduleIdx < len(modIDs) {
		curModID = modIDs[moduleIdx]
	}
	win := moduleWindowFor(ex, curModID)

	// Validate target inside window if locked
	if nav.ModuleLocked && win.hasAny {
		if _, ok := win.indices[target]; !ok {
			return Attempt{}, ErrOutsideModule
		}
	}
	// Forward-only validation
	if !nav.AllowBack && target < maxIdx {
		return Attempt{}, ErrBackwardNavBlocked
	}

	// persist
	newMax := maxIdx
	if target > newMax {
		newMax = target
	}
	if _, err := s.db.Exec(`UPDATE attempts SET current_index=$1, max_reached_index=$2 WHERE id=$3`, target, newMax, attemptID); err != nil {
		return Attempt{}, err
	}
	return s.GetAttempt(attemptID)
}

// internal/exam/store_sql.go (helpers)
func extractModuleTimes(policyRaw json.RawMessage) []int {
	ms := extractModules(policyRaw)
	if len(ms) == 0 {
		return nil
	}
	out := make([]int, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.TimeLimitSec)
	}
	return out
}
