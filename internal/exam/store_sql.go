package exam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/mind-engage/mindengage-lms/internal/grading"
)

type SQLStore struct {
	db     *sql.DB
	driver string // "sqlite" or "postgres"
	grader grading.Grader
}

func NewSQLStore(db *sql.DB, driver string) *SQLStore {
	return &SQLStore{db: db, driver: driver}
}

func (s *SQLStore) PutExam(e Exam) error {
	qj, err := json.Marshal(e.Questions)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO exams (id,title,time_limit_sec,questions_json,created_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, time_limit_sec=EXCLUDED.time_limit_sec, questions_json=EXCLUDED.questions_json`,
		e.ID, e.Title, e.TimeLimitSec, string(qj), time.Now().Unix())
	return err
}

func (s *SQLStore) GetExam(id string) (Exam, error) {
	row := s.db.QueryRow(`SELECT id,title,time_limit_sec,questions_json FROM exams WHERE id=$1`, id)
	var e Exam
	var qjson string
	if err := row.Scan(&e.ID, &e.Title, &e.TimeLimitSec, &qjson); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Exam{}, errors.New("exam not found")
		}
		return Exam{}, err
	}
	if err := json.Unmarshal([]byte(qjson), &e.Questions); err != nil {
		return Exam{}, err
	}
	// Strip answer keys when serving to students (parity with in-memory behavior)
	for i := range e.Questions {
		e.Questions[i].AnswerKey = nil
	}
	return e, nil
}

func (s *SQLStore) NewAttempt(examID, userID string) (Attempt, error) {
	// ensure exam exists
	var exist int
	if err := s.db.QueryRow(`SELECT 1 FROM exams WHERE id=$1`, examID).Scan(&exist); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Attempt{}, errors.New("exam not found")
		}
		if err != nil {
			return Attempt{}, err
		}
	}
	id := time.Now().Format("20060102150405")
	resp := map[string]interface{}{}
	respJSON, _ := json.Marshal(resp)
	_, err := s.db.Exec(`INSERT INTO attempts (id,exam_id,user_id,status,score,responses_json,started_at)
		VALUES ($1,$2,$3,'in_progress',0,$4,$5)`,
		id, examID, userID, string(respJSON), time.Now().Unix())
	if err != nil {
		return Attempt{}, err
	}
	return Attempt{ID: id, ExamID: examID, UserID: userID, Status: "in_progress", Score: 0, Responses: resp}, nil
}

func (s *SQLStore) SaveResponses(attemptID string, resp map[string]interface{}) (Attempt, error) {
	a, err := s.GetAttempt(attemptID)
	if err != nil {
		return Attempt{}, err
	}
	if a.Status == "submitted" {
		return Attempt{}, errors.New("attempt already submitted")
	}
	// merge
	if a.Responses == nil {
		a.Responses = map[string]interface{}{}
	}
	for k, v := range resp {
		a.Responses[k] = v
	}
	buf, _ := json.Marshal(a.Responses)
	_, err = s.db.Exec(`UPDATE attempts SET responses_json=$1 WHERE id=$2`, string(buf), attemptID)
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
	_, err = s.db.Exec(`UPDATE attempts SET status='submitted', score=$1, responses_json=$2, submitted_at=$3 WHERE id=$4`,
		a.Score, string(buf), time.Now().Unix(), attemptID)
	if err != nil {
		return Attempt{}, err
	}
	return s.GetAttempt(attemptID)
}

func (s *SQLStore) GetAttempt(id string) (Attempt, error) {
	row := s.db.QueryRow(`SELECT id,exam_id,user_id,status,score,responses_json FROM attempts WHERE id=$1`, id)
	var a Attempt
	var rjson string
	if err := row.Scan(&a.ID, &a.ExamID, &a.UserID, &a.Status, &a.Score, &rjson); err != nil {
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
