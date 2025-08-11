package exam

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
}

type Attempt struct {
	ID        string                 `json:"id"`
	ExamID    string                 `json:"exam_id"`
	UserID    string                 `json:"user_id"`
	Status    string                 `json:"status"` // in_progress|submitted
	Score     float64                `json:"score"`
	Responses map[string]interface{} `json:"responses"` // questionID -> response payload
}

type Exam struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	TimeLimitSec int        `json:"time_limit_sec"`
	Questions    []Question `json:"questions"`

	CreatedAt int64 `json:"created_at,omitempty"` // NEW: aligns with DB schema
}
