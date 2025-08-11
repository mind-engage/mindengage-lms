package formats

import (
	"context"
	"io"
)

// Adapter defines import/export/validation for a given test profile (SAT/ACT/JEE).
type Adapter interface {
	// Import parses a format-specific package (zip/csv/etc.) into an internal Exam + Policy.
	Import(ctx context.Context, r io.Reader) (ExamLike, Policy, error)
	// Export creates a format-specific package from an internal Exam + Policy.
	Export(ctx context.Context, ex ExamLike, pol Policy) (io.ReadCloser, error)
	// Validate enforces profile-specific constraints on the exam+policy.
	Validate(ex ExamLike, pol Policy) error
}

// ExamLike is the minimal surface we need from your internal exam model.
// (Prevents import cycles: the formats layer doesn’t depend on the full exam pkg.)
type ExamLike interface {
	GetID() string
	GetTitle() string
	GetQuestions() []QuestionLike
}

type QuestionLike interface {
	GetID() string
	GetType() string          // mcq_single, mcq_multi, true_false, short_word, numeric, essay, ...
	GetChoices() []ChoiceLike // for MCQ; may be nil
}

type ChoiceLike interface {
	GetID() string
}

// Registry of adapters by profile key (e.g., "sat.v1", "act.v1", "jee.v1")
var registry = map[string]Adapter{}

// Register a profile adapter. Call from init() in subpackages.
func Register(profile string, a Adapter) { registry[profile] = a }

// Lookup returns a registered adapter for a profile.
func Lookup(profile string) (Adapter, bool) { a, ok := registry[profile]; return a, ok }
