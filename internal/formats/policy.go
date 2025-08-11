package formats

import (
	"errors"
	"fmt"
)

// Policy holds timing, navigation, scoring rules, etc., independent of the item content.
type Policy struct {
	Sections    []Section      `json:"sections,omitempty"`
	Navigation  Navigation     `json:"navigation,omitempty"`
	Calculator  Calculator     `json:"calculator,omitempty"`
	Scoring     Scoring        `json:"scoring,omitempty"`
	Constraints Constraints    `json:"item_constraints,omitempty"`
	Proctor     Proctor        `json:"proctor,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"` // free-form e.g. versioning, locale
}

type Section struct {
	ID      string   `json:"id"`
	Title   string   `json:"title,omitempty"`
	Modules []Module `json:"modules,omitempty"`
}

type Module struct {
	ID           string `json:"id"`
	TimeLimitSec int    `json:"time_limit_sec,omitempty"`
}

type Navigation struct {
	AllowBack    bool `json:"allow_back,omitempty"`
	ModuleLocked bool `json:"module_locked,omitempty"`
}

type Calculator struct {
	AllowedSections []string `json:"allowed_sections,omitempty"`
	Policy          string   `json:"policy,omitempty"` // e.g., "desmos", "basic", "none"
	AllowKeypad     bool     `json:"allow_keypad,omitempty"`
}

type Scoring struct {
	RawToScale    string            `json:"raw_to_scale,omitempty"` // key for scale mapper, e.g., "sat.v1.scale"
	Penalty       float64           `json:"penalty,omitempty"`      // negative marking (e.g., JEE)
	PartialCredit map[string]string `json:"partial_credit,omitempty"`
}

type Constraints struct {
	// per section constraints e.g., {"rw":{"mcq_single":{"choices":4}}}
	BySection map[string]map[string]map[string]float64 `json:"by_section,omitempty"`
}

type Proctor struct {
	Screenshot bool `json:"screenshot,omitempty"`
	Camera     bool `json:"camera,omitempty"`
}

// ValidatePolicy runs basic consistency checks.
func ValidatePolicy(profile string, pol *Policy) error {
	if pol == nil {
		return errors.New("policy is required")
	}
	seen := map[string]bool{}
	for _, s := range pol.Sections {
		if s.ID == "" {
			return errors.New("section.id is required")
		}
		if seen[s.ID] {
			return fmt.Errorf("duplicate section id: %s", s.ID)
		}
		seen[s.ID] = true
		modSeen := map[string]bool{}
		for _, m := range s.Modules {
			if m.ID == "" {
				return fmt.Errorf("module.id required in section %s", s.ID)
			}
			if modSeen[m.ID] {
				return fmt.Errorf("duplicate module id %s in section %s", m.ID, s.ID)
			}
			modSeen[m.ID] = true
			if m.TimeLimitSec < 0 {
				return fmt.Errorf("negative time_limit_sec in %s/%s", s.ID, m.ID)
			}
		}
	}
	// Additional profile-specific checks are enforced by Adapter.Validate.
	return nil
}
