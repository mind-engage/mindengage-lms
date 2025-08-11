package stem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/mind-engage/mindengage-lms/internal/formats"
)

func init() {
	formats.Register("stem.v1", New())
}

type AdapterSTEM struct{}

func New() *AdapterSTEM { return &AdapterSTEM{} }

func (a *AdapterSTEM) Import(ctx context.Context, r io.Reader) (formats.ExamLike, formats.Policy, error) {
	_, _ = ioutil.ReadAll(r)
	return nil, formats.Policy{}, errors.New("stem.v1 import not implemented")
}

func (a *AdapterSTEM) Export(ctx context.Context, ex formats.ExamLike, pol formats.Policy) (io.ReadCloser, error) {
	// Placeholder
	return io.NopCloser(stringsNewReader(
		fmt.Sprintf("STEM export placeholder\nexam=%s title=%s\n", ex.GetID(), ex.GetTitle()),
	)), nil
}

func (a *AdapterSTEM) Validate(ex formats.ExamLike, pol formats.Policy) error {
	if err := formats.ValidatePolicy("stem.v1", &pol); err != nil {
		return err
	}

	// Core constraints:
	// - 4-choice single-answer MC is standard; enforce if choices present.
	for _, q := range ex.GetQuestions() {
		if q.GetType() == "mcq_single" && len(q.GetChoices()) > 0 && len(q.GetChoices()) != 4 {
			return fmt.Errorf("stem.v1: question %s must have exactly 4 choices", q.GetID())
		}
		// No SPR typically; if you use "numeric" as SPR, discourage here.
		//if q.GetType() == "numeric" && len(q.GetChoices()) == 0 {
		//	return fmt.Errorf("stem.v1: numeric SPR-style items not supported in MVP")
		//}
	}

	// Section timing may be per-section not module; we don't enforce exact minutes here.
	return nil
}

// tiny helper without importing fmt/strings everywhere
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
