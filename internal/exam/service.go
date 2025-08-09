package exam

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/mind-engage/mindengage-lms/internal/grading"
)

type Store interface {
	PutExam(e Exam) error
	GetExam(id string) (Exam, error)
	NewAttempt(examID, userID string) (Attempt, error)
	SaveResponses(attemptID string, resp map[string]interface{}) (Attempt, error)
	Submit(attemptID string) (Attempt, error)
	GetAttempt(id string) (Attempt, error)
}

type memoryStore struct {
	mu       sync.RWMutex
	exams    map[string]Exam
	attempts map[string]Attempt
	grader   grading.Grader
}

func NewInMemoryStore() Store {
	rand.Seed(time.Now().UnixNano())
	return &memoryStore{
		exams:    map[string]Exam{},
		attempts: map[string]Attempt{},
		grader:   grading.NewDefaultGrader(), // default strategies
	}
}

func (m *memoryStore) PutExam(e Exam) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exams[e.ID] = e
	return nil
}

func (m *memoryStore) GetExam(id string) (Exam, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.exams[id]
	if !ok {
		return Exam{}, errors.New("exam not found")
	}
	// hide answers from students in this minimal store
	for i := range e.Questions {
		e.Questions[i].AnswerKey = nil
	}
	return e, nil
}

func (m *memoryStore) NewAttempt(examID, userID string) (Attempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.exams[examID]; !ok {
		return Attempt{}, errors.New("exam not found")
	}
	id := randID()
	a := Attempt{ID: id, ExamID: examID, UserID: userID, Status: "in_progress", Responses: map[string]interface{}{}}
	m.attempts[id] = a
	return a, nil
}

func (m *memoryStore) SaveResponses(attemptID string, resp map[string]interface{}) (Attempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.attempts[attemptID]
	if !ok {
		return Attempt{}, errors.New("attempt not found")
	}
	if a.Responses == nil {
		a.Responses = map[string]interface{}{}
	}
	for k, v := range resp {
		a.Responses[k] = v
	}
	m.attempts[attemptID] = a
	return a, nil
}

func (m *memoryStore) Submit(attemptID string) (Attempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.attempts[attemptID]
	if !ok {
		return Attempt{}, errors.New("attempt not found")
	}
	if a.Status == "submitted" {
		return a, nil
	}

	e, ok := m.exams[a.ExamID]
	if !ok {
		return Attempt{}, errors.New("exam not found")
	}

	ctx := context.Background()
	score := 0.0

	for _, q := range e.Questions {
		resp, has := a.Responses[q.ID]
		if !has {
			continue
		}
		gq := grading.Q{
			Type:      q.Type,
			Points:    q.Points,
			AnswerKey: q.AnswerKey,
		}
		res, err := m.grader.Grade(ctx, gq, resp)
		if err != nil {
			continue
		}
		score += res.AutoPoints
	}

	a.Score = score
	a.Status = "submitted"
	m.attempts[attemptID] = a
	return a, nil
}

func (m *memoryStore) GetAttempt(id string) (Attempt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.attempts[id]
	if !ok {
		return Attempt{}, errors.New("attempt not found")
	}
	return a, nil
}

func randID() string { return time.Now().Format("20060102150405") }
