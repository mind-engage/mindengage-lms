package exam

import "context"

type ListOpts struct {
	Q          string
	Limit      int
	Offset     int
	ViewerID   string // <- NEW
	ViewerRole string // <- NEW: "student" | "teacher" | "admin"
}

type AttemptListOpts struct {
	ExamID string // filter by exam/course
	UserID string // filter by student
	Status string // optional: in_progress|submitted
	Limit  int
	Offset int
	Sort   string // started_at|submitted_at desc (default: started_at desc)
}

type ManualGradeInput struct {
	ManualPoints float64 `json:"manual_points"`
	Comment      string  `json:"comment,omitempty"`
}

type Store interface {
	PutExam(e Exam) error
	GetExam(id string) (Exam, error)                           // student-safe (no answer keys)
	GetExamAdmin(ctx context.Context, id string) (Exam, error) // full exam, for export/teachers
	NewAttempt(examID, userID string) (Attempt, error)
	SaveResponses(attemptID string, resp map[string]interface{}) (Attempt, error)
	Submit(attemptID string) (Attempt, error)
	GetAttempt(id string) (Attempt, error)

	ListExams(ctx context.Context, opts ListOpts) ([]ExamSummary, error)
	AdvanceModule(attemptID string) (Attempt, error)

	// NEW: list attempts with filters for teacher/admin dashboards (and student “my attempts”)
	ListAttempts(ctx context.Context, opts AttemptListOpts) ([]Attempt, error)
	Navigate(attemptID string, target int) (Attempt, error)

	GetAttemptItems(ctx context.Context, attemptID string) ([]AttemptItem, error)
	ApplyManualGrades(ctx context.Context, attemptID string, updates map[string]ManualGradeInput, gradedBy string, finalize bool) (Attempt, error)
}
