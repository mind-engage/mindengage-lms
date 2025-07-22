// === pkg/qti/qti.go ===
package qti

import "github.com/google/uuid"

// Item represents a simple multiple-choice question
type Item struct {
	ID        uuid.UUID `json:"id"`
	Question  string    `json:"question"`
	Options   []string  `json:"options"`
	AnswerKey int       `json:"-"` // index of correct option
}

// CreateItemRequest is used to add new items
type CreateItemRequest struct {
	Question  string   `json:"question"`
	Options   []string `json:"options"`
	AnswerKey int      `json:"answer_key"`
}

// SubmitRequest is sent by clients with their answer
type SubmitRequest struct {
	SelectedOption int `json:"selected_option"`
}

// SubmitResponse returns the score
type SubmitResponse struct {
	Score int `json:"score"` // 1 for correct, 0 for incorrect
}

// Engine handles scoring logic
type Engine interface {
	Score(Item, int) int
}

// engineImpl is a basic implementation of Engine
type engineImpl struct{}

// NewEngine creates a new QTI engine
func NewEngine() Engine {
	return &engineImpl{}
}

// Score compares the selected option to the answer key
func (e *engineImpl) Score(item Item, selected int) int {
	if selected == item.AnswerKey {
		return 1
	}
	return 0
}
