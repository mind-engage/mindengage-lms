package exam

import "context"

type Store interface {
	PutExam(e Exam) error
	GetExam(id string) (Exam, error)                           // student-safe (no answer keys)
	GetExamAdmin(ctx context.Context, id string) (Exam, error) // full exam, for export/teachers
	NewAttempt(examID, userID string) (Attempt, error)
	SaveResponses(attemptID string, resp map[string]interface{}) (Attempt, error)
	Submit(attemptID string) (Attempt, error)
	GetAttempt(id string) (Attempt, error)
}
