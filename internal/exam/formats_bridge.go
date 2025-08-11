//go:build !formats_no_bridge
// +build !formats_no_bridge

package exam

// --- bridge to formats.ExamLike ---

func (e Exam) GetID() string            { return e.ID }
func (e Exam) GetTitle() string         { return e.Title }
func (e Exam) GetQuestions() []Question { return e.Questions }

func (q Question) GetID() string        { return q.ID }
func (q Question) GetType() string      { return q.Type }
func (q Question) GetChoices() []Choice { return q.Choices }

func (c Choice) GetID() string { return c.ID }
