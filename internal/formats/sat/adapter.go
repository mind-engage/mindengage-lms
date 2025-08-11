package sat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/mind-engage/mindengage-lms/internal/formats"
)

// Register adapter at init
func init() {
	formats.Register("sat.v1", New())
	formats.RegisterScale("sat.v1.scale", SATScale{}) // attach scale mapper
}

type AdapterSAT struct{}

func New() *AdapterSAT { return &AdapterSAT{} }

// Import: stub that just rejects for now (wire later if you want).
func (a *AdapterSAT) Import(ctx context.Context, r io.Reader) (formats.ExamLike, formats.Policy, error) {
	_, _ = ioutil.ReadAll(r) // consume to avoid unused
	return nil, formats.Policy{}, errors.New("sat.v1 import not implemented")
}

// Export: returns a tiny placeholder package so piping works end-to-end.
func (a *AdapterSAT) Export(ctx context.Context, ex formats.ExamLike, pol formats.Policy) (io.ReadCloser, error) {
	content := fmt.Sprintf("SAT export placeholder\nexam=%s title=%s\n", ex.GetID(), ex.GetTitle())
	return io.NopCloser(strings.NewReader(content)), nil
}

// Validate: enforce key SAT constraints (minimal, extensible)
func (a *AdapterSAT) Validate(ex formats.ExamLike, pol formats.Policy) error {
	if err := formats.ValidatePolicy("sat.v1", &pol); err != nil {
		return err
	}

	// Check module timing (39min R&W, 43min Math) if provided.
	// We only warn/enforce when module ids look like expected names.
	want := map[string]int{"rw": 39 * 60, "math": 43 * 60}
	for _, s := range pol.Sections {
		if s.ID == "rw" || s.ID == "math" {
			exp := want[s.ID]
			for _, m := range s.Modules {
				if m.TimeLimitSec != 0 && m.TimeLimitSec != exp {
					return fmt.Errorf("sat.v1: section %s module %s must be %ds", s.ID, m.ID, exp)
				}
			}
		}
	}

	// R&W: multiple-choice must have exactly 4 choices.
	for _, q := range ex.GetQuestions() {
		if q.GetType() == "mcq_single" {
			if len(q.GetChoices()) > 0 && len(q.GetChoices()) != 4 {
				return fmt.Errorf("sat.v1: question %s must have exactly 4 choices", q.GetID())
			}
		}
	}

	// Navigation: typical SAT - no back within module, modules locked.
	if pol.Navigation.AllowBack {
		return errors.New("sat.v1: navigation.allow_back must be false")
	}
	if !pol.Navigation.ModuleLocked {
		return errors.New("sat.v1: navigation.module_locked must be true")
	}
	return nil
}
