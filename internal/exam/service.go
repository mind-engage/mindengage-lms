package exam

import (
	"errors"
	"math/rand"
	"sync"
	"time"
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
}

func NewInMemoryStore() Store {
	rand.Seed(time.Now().UnixNano())
	return &memoryStore{
		exams:    map[string]Exam{},
		attempts: map[string]Attempt{},
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
	e := m.exams[a.ExamID]
	score := 0.0
	for _, q := range e.Questions {
		resp, ok := a.Responses[q.ID]
		if !ok {
			continue
		}
		switch q.Type {
		case "mcq_single", "true_false", "short_word", "numeric":
			// compare string-equal against first answer_key (super-minimal)
			if s, ok := resp.(string); ok && len(q.AnswerKey) > 0 && s == q.AnswerKey[0] {
				score += q.Points
			}
		case "mcq_multi":
			// resp and key must match as string slices, order-insensitive (very basic)
			if arr, ok := toStringSlice(resp); ok && equalStringSets(arr, q.AnswerKey) {
				score += q.Points
			}
		case "essay":
			// no autograde in minimal; 0 until manual grading
		}
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

func toStringSlice(v interface{}) ([]string, bool) {
	x, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(x))
	for _, e := range x {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out, true
}
func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}
