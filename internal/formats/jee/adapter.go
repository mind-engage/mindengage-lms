package jee

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/mind-engage/mindengage-lms/internal/formats"
)

func init() {
	formats.Register("jee.v1", New())
}

type AdapterJEE struct{}

func New() *AdapterJEE { return &AdapterJEE{} }

func (a *AdapterJEE) Import(ctx context.Context, r io.Reader) (formats.ExamLike, formats.Policy, error) {
	_, _ = ioutil.ReadAll(r)
	return nil, formats.Policy{}, errors.New("jee.v1 import not implemented")
}

func (a *AdapterJEE) Export(ctx context.Context, ex formats.ExamLike, pol formats.Policy) (io.ReadCloser, error) {
	content := fmt.Sprintf("JEE export placeholder\nexam=%s title=%s\n", ex.GetID(), ex.GetTitle())
	return io.NopCloser(stringsNewReader(content)), nil
}

func (a *AdapterJEE) Validate(ex formats.ExamLike, pol formats.Policy) error {
	if err := formats.ValidatePolicy("jee.v1", &pol); err != nil {
		return err
	}

	// JEE allows multi-correct MCQ, numeric/integer, and negative marking.
	// - If scoring.penalty is set (e.g., -0.25), allow it.
	// - Ensure types include allowed ones; reject unsupported types.
	for _, q := range ex.GetQuestions() {
		switch q.GetType() {
		case "mcq_single", "mcq_multi", "numeric", "integer", "short_word", "essay":
			// OK (integer is optional extension; treat like numeric in your grader)
		default:
			return fmt.Errorf("jee.v1: unsupported question type %s", q.GetType())
		}
	}

	// No strict choice count; but if choices exist, ensure >= 2
	for _, q := range ex.GetQuestions() {
		if len(q.GetChoices()) > 0 && len(q.GetChoices()) < 2 {
			return fmt.Errorf("jee.v1: question %s must have at least 2 choices", q.GetID())
		}
	}

	return nil
}

// mini reader helper
func stringsNewReader(s string) io.ReadCloser { return io.NopCloser(stringsReader(s)) }
func stringsReader(s string) *stringsReaderT  { return &stringsReaderT{s: []byte(s)} }

type stringsReaderT struct {
	s []byte
	i int
}

func (r *stringsReaderT) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}
