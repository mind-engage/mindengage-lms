// internal/exam/store_sql.go
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

	base := `
SELECT e.id, e.title, e.time_limit_sec, e.created_at, e.profile
FROM exams e
`
	where := []string{}
	args := []any{}
	i := 1

	role := strings.ToLower(strings.TrimSpace(opts.ViewerRole))
	uid := strings.TrimSpace(opts.ViewerID)

	switch role {
	case "teacher":
		// Teacher: exams they own
		base += ` JOIN exam_owners eo ON eo.exam_id = e.id `
		where = append(where, fmt.Sprintf("eo.teacher_id = $%d", i))
		args = append(args, uid)
		i++
	case "student":
		// Student: exams offered in courses they are enrolled in (active)
		where = append(where, fmt.Sprintf(`
EXISTS (
  SELECT 1
    FROM exam_offerings ofr
    JOIN course_students cs
      ON cs.course_id = ofr.course_id
   WHERE ofr.exam_id = e.id
     AND cs.student_id = $%d
     AND cs.status = 'active'
)`, i))
		args = append(args, uid)
		i++
	default:
		// Admin or unknown: no extra predicate (admins see all)
	}

	// Optional title search
	if q := strings.TrimSpace(opts.Q); q != "" {
		where = append(where, fmt.Sprintf("LOWER(e.title) LIKE LOWER('%%' || $%d || '%%')", i))
		args = append(args, q)
		i++
	}
	if len(where) == 0 {
		where = append(where, "1=1")
	}

	q := fmt.Sprintf(`
%s
WHERE %s
ORDER BY e.created_at DESC
LIMIT %d OFFSET %d
`, base, strings.Join(where, " AND "), opts.Limit, opts.Offset)

	rows, err := s.db.QueryContext(ctx, q, args...)
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

	// Initialize concrete module id for module 0 (usually placeholder id)
	firstConcrete := ""
	if len(modIDs) > 0 {
		firstConcrete = modIDs[0]
	}

	// --- persist attempt ---
	id := time.Now().Format("20060102150405")
	resp := map[string]interface{}{}
	respJSON, _ := json.Marshal(resp)

	_, err = s.db.Exec(`
		INSERT INTO attempts (
			id, exam_id, user_id, status, score, responses_json, started_at,
			module_index, module_started_at, module_deadline, overall_deadline,
			current_index, max_reached_index, current_module_id
		)
		VALUES ($1,$2,$3,'in_progress',0,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`,
		id, examID, userID, string(respJSON), now,
		0, now, nullableDeadline(now, firstMod), nullableDeadline(now, overall),
		startIdx, startIdx, firstConcrete,
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
		CurrentModuleID: firstConcrete,
	}, nil
}

func (s *SQLStore) SaveResponses(attemptID string, resp map[string]interface{}) (Attempt, error) {
	// Load attempt (with timing columns for enforcement)
	var a Attempt
	var rjson string
	var moduleIdx, curIdx, maxIdx int // NEW: cur/max
	var moduleStarted, moduleDeadline, overallDeadline sql.NullInt64
	var curModID sql.NullString

	row := s.db.QueryRow(`
	  SELECT id, exam_id, user_id, status, score, responses_json,
			 module_index, module_started_at, module_deadline, overall_deadline,
			 current_index, max_reached_index, current_module_id
	  FROM attempts WHERE id=$1`, attemptID)
	if err := row.Scan(&a.ID, &a.ExamID, &a.UserID, &a.Status, &a.Score, &rjson,
		&moduleIdx, &moduleStarted, &moduleDeadline, &overallDeadline,
		&curIdx, &maxIdx, &curModID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("attempt not found")
		}
		return Attempt{}, err
	}
	if err := json.Unmarshal([]byte(rjson), &a.Responses); err != nil || a.Responses == nil {
		a.Responses = map[string]interface{}{}
	}
	if curModID.Valid {
		a.CurrentModuleID = curModID.String
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

	// Module lock (prefer the concrete current_module_id)
	if nav.ModuleLocked {
		targetID := strings.TrimSpace(a.CurrentModuleID)
		if targetID == "" {
			// fallback to placeholder by index
			modIDs := extractModuleIDs(ex.PolicyRaw)
			if moduleIdx >= 0 && moduleIdx < len(modIDs) {
				targetID = strings.TrimSpace(modIDs[moduleIdx])
			}
		}
		allowed := allowedQIDsForModuleID(ex, targetID)
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
		// still recompute scores (idempotent) to ensure item rows exist
	} // else proceed

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
	autoTotal := 0.0

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Attempt{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// For manual sum we look at persisted rows (may have pre-existing manual points)
	for _, q := range questions {
		resp, has := a.Responses[q.ID]
		// grade what we can automatically
		auto := 0.0
		if has {
			gq := grading.Q{Type: q.Type, Points: q.Points, AnswerKey: q.AnswerKey}
			res, err := s.grader.Grade(ctx, gq, resp)
			if err == nil {
				auto = res.AutoPoints
			}
		}
		autoTotal += auto

		// upsert attempt_items
		respJSON, _ := json.Marshal(resp)
		needMan := needsManualForType(q.Type, q)
		_, err := tx.Exec(`
			INSERT INTO attempt_items (attempt_id, question_id, q_type, points_max, auto_points, manual_points, needs_manual, response_json)
			VALUES ($1,$2,$3,$4,$5,
			        COALESCE((SELECT manual_points FROM attempt_items WHERE attempt_id=$1 AND question_id=$2), 0),
			        $6,$7)
			ON CONFLICT (attempt_id, question_id) DO UPDATE SET
			  q_type=EXCLUDED.q_type,
			  points_max=EXCLUDED.points_max,
			  auto_points=EXCLUDED.auto_points,
			  needs_manual=EXCLUDED.needs_manual,
			  response_json=EXCLUDED.response_json
		`, attemptID, q.ID, q.Type, q.Points, auto, needMan, string(respJSON))
		if err != nil {
			return Attempt{}, err
		}
	}

	// sum manual points currently on items
	var manualSum float64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(manual_points),0) FROM attempt_items WHERE attempt_id=$1`, attemptID).Scan(&manualSum); err != nil {
		return Attempt{}, err
	}

	now := time.Now().Unix()
	// status becomes submitted (or stays submitted), and score is auto+manual
	_, err = tx.Exec(`
	  UPDATE attempts
	     SET status='submitted',
	         auto_score=$1,
	         manual_score=$2,
	         score=$3,
	         submitted_at=COALESCE(submitted_at, $4)
	   WHERE id=$5`,
		autoTotal, manualSum, autoTotal+manualSum, now, attemptID)
	if err != nil {
		return Attempt{}, err
	}

	if err := tx.Commit(); err != nil {
		return Attempt{}, err
	}

	_ = syncx.NewEventRepo(s.db).Append(context.Background(), syncx.Event{
		SiteID:   "local",
		Type:     "AttemptSubmitted",
		Key:      attemptID,
		DataJSON: "{}", // keep minimal; responses already stored
	})

	return s.GetAttempt(attemptID)
}

func (s *SQLStore) GetAttempt(id string) (Attempt, error) {
	row := s.db.QueryRow(`SELECT id,exam_id,user_id,status,score,responses_json,started_at,submitted_at,
	  module_index, COALESCE(module_started_at,0), COALESCE(module_deadline,0), COALESCE(overall_deadline,0),
	  current_index, max_reached_index, current_module_id
	  FROM attempts WHERE id=$1`, id)

	var a Attempt
	var rjson string
	var moduleStarted, moduleDeadline, overallDeadline int64
	var curModID sql.NullString
	if err := row.Scan(&a.ID, &a.ExamID, &a.UserID, &a.Status, &a.Score, &rjson, &a.StartedAt, &a.SubmittedAt,
		&a.ModuleIndex, &moduleStarted, &moduleDeadline, &overallDeadline,
		&a.CurrentIndex, &a.MaxReachedIndex, &curModID); err != nil {
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
	if curModID.Valid {
		a.CurrentModuleID = curModID.String
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
	var curModID sql.NullString

	row := s.db.QueryRow(`SELECT exam_id, responses_json, module_index, current_module_id FROM attempts WHERE id=$1`, attemptID)
	if err := row.Scan(&a.ExamID, &rjson, &moduleIdx, &curModID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("attempt not found")
		}
		return Attempt{}, err
	}
	_ = json.Unmarshal([]byte(rjson), &a.Responses)
	if curModID.Valid {
		a.CurrentModuleID = curModID.String
	}
	a.ModuleIndex = moduleIdx

	ex, err := s.GetExamAdmin(context.Background(), a.ExamID)
	if err != nil {
		return Attempt{}, err
	}

	// Placeholder-derived times and ids
	modules := extractModuleTimes(ex.PolicyRaw)
	modIDs := extractModuleIDs(ex.PolicyRaw)
	if len(modules) == 0 && ex.TimeLimitSec > 0 {
		modules = []int{ex.TimeLimitSec}
	}
	if len(modules) == 0 || len(modIDs) == 0 {
		return Attempt{}, errors.New("no modules in policy")
	}
	if moduleIdx+1 >= len(modules) {
		return Attempt{}, errors.New("already at last module")
	}

	nextIdx := moduleIdx + 1
	nextPlaceholderID := modIDs[nextIdx]

	// Build performance on the module that just finished (prefer concrete id)
	prevID := strings.TrimSpace(a.CurrentModuleID)
	if prevID == "" && moduleIdx >= 0 && moduleIdx < len(modIDs) {
		prevID = strings.TrimSpace(modIDs[moduleIdx])
	}
	perfRaw := s.moduleRawPerf(ex, a, prevID)

	// Route to a concrete next module id (variant) if router exists
	concreteNextID := nextPlaceholderID
	if r := RouterForProfile(ex.Profile); r != nil {
		if chosen, _ := r.NextModule(context.Background(), ex, a, Perf{RawPoints: perfRaw}); strings.TrimSpace(chosen) != "" {
			concreteNextID = strings.TrimSpace(chosen)
		}
	}

	now := time.Now().Unix()
	nextDur := int64(0)
	if modules[nextIdx] > 0 {
		nextDur = int64(modules[nextIdx])
	}

	// Compute first question index of the concrete next module (if any)
	cur := 0
	if concreteNextID != "" {
		win := moduleWindowFor(ex, concreteNextID)
		if win.hasAny {
			cur = win.firstIdx
		}
	}

	_, err = s.db.Exec(`
	  UPDATE attempts
	  SET module_index=$1, module_started_at=$2, module_deadline=$3,
		  current_index=$4, max_reached_index=$4, current_module_id=$5
	  WHERE id=$6`,
		nextIdx, now, nullableDeadline(now, nextDur),
		cur, concreteNextID, attemptID,
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

// extract ordered module IDs from policy to align with Question.ModuleID
func extractModuleIDs(policyRaw json.RawMessage) []string {
	ms := extractModules(policyRaw)
	if len(ms) == 0 {
		return nil
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ModuleID)
	}
	return out
}

// set of question IDs allowed in the given module index (nil => no restriction)
// (kept for backward-compat; variant-aware code should prefer allowedQIDsForModuleID)
func allowedQIDsForModule(ex Exam, moduleIdx int) map[string]struct{} {
	modIDs := extractModuleIDs(ex.PolicyRaw)
	if len(modIDs) == 0 || moduleIdx < 0 || moduleIdx >= len(modIDs) {
		return nil // nothing to enforce
	}
	target := strings.TrimSpace(modIDs[moduleIdx])
	if target == "" {
		return nil // cannot enforce without an ID
	}
	return allowedQIDsForModuleID(ex, target)
}

// variant-aware: set of question IDs allowed for the given concrete module id
func allowedQIDsForModuleID(ex Exam, moduleID string) map[string]struct{} {
	moduleID = strings.TrimSpace(moduleID)
	if moduleID == "" {
		return nil
	}
	set := map[string]struct{}{}
	for _, q := range ex.Questions {
		if strings.TrimSpace(q.ModuleID) == moduleID {
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
	var curModID sql.NullString

	row := s.db.QueryRow(`
		SELECT exam_id, status, module_index, current_index, max_reached_index,
		       module_deadline, overall_deadline, current_module_id
		FROM attempts WHERE id=$1`, attemptID)
	if err := row.Scan(&examID, &status, &moduleIdx, &curIdx, &maxIdx, &moduleDeadline, &overallDeadline, &curModID); err != nil {
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

	// window: prefer concrete current_module_id
	activeID := strings.TrimSpace(curModID.String)
	if activeID == "" && moduleIdx >= 0 && moduleIdx < len(modIDs) {
		activeID = strings.TrimSpace(modIDs[moduleIdx])
	}
	win := moduleWindowFor(ex, activeID)

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

// Compute raw performance for a module (simple correct-count; tweak as needed).
func (s *SQLStore) moduleRawPerf(ex Exam, a Attempt, moduleID string) float64 {
	moduleID = strings.TrimSpace(moduleID)
	if moduleID == "" {
		return 0
	}
	raw := 0.0
	for _, q := range ex.Questions {
		if strings.TrimSpace(q.ModuleID) != moduleID {
			continue
		}
		if resp, ok := a.Responses[q.ID]; ok {
			res, err := s.grader.Grade(context.Background(),
				grading.Q{Type: q.Type, Points: 1, AnswerKey: q.AnswerKey}, resp)
			if err == nil && res.AutoPoints > 0 {
				raw += 1
			}
		}
	}
	return raw
}

// helpers
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

func needsManualForType(t string, q Question) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "essay":
		return true
	case "short_word":
		// treat short_word as manual if no answer_key is provided
		return len(q.AnswerKey) == 0
	default:
		return false
	}
}

func (s *SQLStore) GetAttemptItems(ctx context.Context, attemptID string) ([]AttemptItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT attempt_id, question_id, q_type, points_max, auto_points, manual_points,
		       needs_manual, response_json, graded_by, graded_at
		FROM attempt_items
		WHERE attempt_id = $1
	`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AttemptItem, 0, 64)
	for rows.Next() {
		var it AttemptItem
		var respRaw any             // []byte on pg, string on sqlite
		var gradedBy sql.NullString // nullable
		var gradedAt sql.NullInt64  // nullable

		if err := rows.Scan(
			&it.AttemptID,
			&it.QuestionID,
			&it.QType,
			&it.PointsMax,
			&it.AutoPoints,
			&it.ManualPoints,
			&it.NeedsManual,
			&respRaw,  // response_json (nullable JSON)
			&gradedBy, // nullable TEXT
			&gradedAt, // nullable BIGINT
		); err != nil {
			return nil, err
		}

		it.ResponseJSON = normalizeRawJSON(respRaw)
		if gradedBy.Valid {
			it.GradedBy = gradedBy.String
		}
		if gradedAt.Valid {
			it.GradedAt = gradedAt.Int64
		}

		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func normalizeRawJSON(v any) json.RawMessage {
	switch t := v.(type) {
	case nil:
		return json.RawMessage("null")
	case []byte:
		if len(t) == 0 {
			return json.RawMessage("null")
		}
		b := make([]byte, len(t))
		copy(b, t)
		return json.RawMessage(b)
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return json.RawMessage("null")
		}
		return json.RawMessage([]byte(s))
	default:
		// Fallback: marshal whatever the driver returned.
		b, _ := json.Marshal(t)
		return b
	}
}

func (s *SQLStore) ApplyManualGrades(ctx context.Context, attemptID string, updates map[string]ManualGradeInput, gradedBy string, finalize bool) (Attempt, error) {
	if len(updates) == 0 {
		return s.GetAttempt(attemptID)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Attempt{}, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	for qid, u := range updates {
		if _, err := tx.ExecContext(ctx, `
			UPDATE attempt_items
			   SET manual_points=$1,
			       comment=$2,
				   graded_by=$3,
				   graded_at=$4
			 WHERE attempt_id=$5 AND question_id=$6`,
			u.ManualPoints, u.Comment, gradedBy, now, attemptID, qid); err != nil {
			return Attempt{}, err
		}
	}

	var autoSum, manualSum float64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(auto_points),0) FROM attempt_items WHERE attempt_id=$1`, attemptID).Scan(&autoSum); err != nil {
		return Attempt{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(manual_points),0) FROM attempt_items WHERE attempt_id=$1`, attemptID).Scan(&manualSum); err != nil {
		return Attempt{}, err
	}

	// mark attempts.graded_at if finalize OR if no remaining needs_manual without manual score
	gradedAtExpr := "graded_at"
	if finalize {
		gradedAtExpr = fmt.Sprintf("%d", now)
	} else {
		// if all items that need manual have >0 or >=0? We just set when there is no item left with needs_manual=true AND manual_points IS NULL? We used REAL default 0
		// Keep graded_at untouched unless finalize=true; simple & predictable.
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		UPDATE attempts
		   SET manual_score=$1,
		       auto_score=$2,
		       score=$3,
		       %s=%s
		 WHERE id=$4`, gradedAtExpr, gradedAtExpr),
		manualSum, autoSum, autoSum+manualSum, attemptID); err != nil {
		return Attempt{}, err
	}

	if err := tx.Commit(); err != nil {
		return Attempt{}, err
	}
	return s.GetAttempt(attemptID)
}
