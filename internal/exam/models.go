package exam

type Exam struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	TimeLimitSec int        `json:"time_limit_sec"`
	Questions    []Question `json:"questions"`
}

type Question struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"` // mcq_single, mcq_multi, true_false, short_word, numeric, essay
	Prompt    string   `json:"prompt"`
	Choices   []string `json:"choices,omitempty"`
	AnswerKey []string `json:"answer_key,omitempty"` // objective keys
	Points    float64  `json:"points"`
}

type Attempt struct {
	ID        string                 `json:"id"`
	ExamID    string                 `json:"exam_id"`
	UserID    string                 `json:"user_id"`
	Status    string                 `json:"status"` // in_progress|submitted
	Score     float64                `json:"score"`
	Responses map[string]interface{} `json:"responses"` // questionID -> response payload
}
