package exam

import (
	"encoding/json"
)

type Choice struct {
	ID        string `json:"id,omitempty"`
	LabelHTML string `json:"label_html,omitempty"`
}

type Question struct {
	ID         string `json:"id"`
	Type       string `json:"type"`                  // mcq_single, mcq_multi, true_false, short_word, numeric, essay, ...
	PromptHTML string `json:"prompt_html,omitempty"` // NEW: QTI import/export uses this
	// If you already had a plain-text Prompt, keep it too:
	// Prompt     string   `json:"prompt,omitempty"`

	Choices   []Choice `json:"choices,omitempty"` // NEW: for MCQ rendered from QTI
	AnswerKey []string `json:"answer_key,omitempty"`
	Points    float64  `json:"points"`
	SectionID string   `json:"section_id,omitempty"`
	ModuleID  string   `json:"module_id,omitempty"`
}

type Attempt struct {
	ID        string                 `json:"id"`
	ExamID    string                 `json:"exam_id"`
	UserID    string                 `json:"user_id"`
	Status    string                 `json:"status"` // in_progress|submitted
	Score     float64                `json:"score"`
	Responses map[string]interface{} `json:"responses"` // questionID -> response payload

	ModuleIndex     int   `json:"module_index"`
	ModuleStartedAt int64 `json:"module_started_at,omitempty"`
	ModuleDeadline  int64 `json:"module_deadline,omitempty"`
	OverallDeadline int64 `json:"overall_deadline,omitempty"`

	// Timestamps (useful for teacher/admin list views)
	StartedAt   int64 `json:"started_at,omitempty"`
	SubmittedAt int64 `json:"submitted_at,omitempty"`

	RemainingSeconds int    `json:"remaining_seconds,omitempty"`
	CurrentIndex     int    `json:"current_index,omitempty"`
	MaxReachedIndex  int    `json:"max_reached_index,omitempty"`
	CurrentModuleID  string `json:"current_module_id,omitempty"`
}

type AttemptItem struct {
	AttemptID    string          `json:"attempt_id"`
	QuestionID   string          `json:"question_id"`
	QType        string          `json:"q_type"`
	PointsMax    float64         `json:"points_max"`
	AutoPoints   float64         `json:"auto_points"`
	ManualPoints float64         `json:"manual_points"`
	NeedsManual  bool            `json:"needs_manual"`
	Comment      string          `json:"comment,omitempty"`
	ResponseJSON json.RawMessage `json:"response_json,omitempty"`
	GradedBy     string          `json:"graded_by,omitempty"`
	GradedAt     int64           `json:"graded_at,omitempty"`
}

type Exam struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	TimeLimitSec int        `json:"time_limit_sec"`
	Questions    []Question `json:"questions"`

	// NEW:
	Profile   string          `json:"profile,omitempty"` // e.g., "sat.v1", "act.v1", "jee.v1"
	PolicyRaw json.RawMessage `json:"policy,omitempty"`

	CreatedAt int64 `json:"created_at,omitempty"` // NEW: aligns with DB schema
}

type ExamSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	TimeLimitSec int    `json:"time_limit_sec"`
	CreatedAt    int64  `json:"created_at,omitempty"`
	Profile      string `json:"profile,omitempty"`
}
