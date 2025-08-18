package sat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mind-engage/mindengage-lms/internal/exam"
)

// Router implements exam.Router for SAT-style multistage adaptivity.
// Policy support (per placeholder module):
//
//	"variants": [{ "id": "rw-m2-easy" }, { "id": "rw-m2-hard" }],
//	"route": { "by_score": { "threshold": 18, "lte": "rw-m2-easy", "gt": "rw-m2-hard" } }
type Router struct{}

func NewRouter() *Router { return &Router{} }

// Self-register for "sat.v1". If you use a different profile string, change it here.
func init() {
	exam.RegisterRouter("sat.v1", NewRouter())
}

func (r *Router) NextModule(ctx context.Context, ex exam.Exam, a exam.Attempt, perf exam.Perf) (string, error) {
	// No policy => nothing to do; proceed sequentially.
	if len(ex.PolicyRaw) == 0 {
		return "", nil
	}

	var pol policy
	if err := json.Unmarshal(ex.PolicyRaw, &pol); err != nil {
		return "", fmt.Errorf("sat: bad policy json: %w", err)
	}

	flat := flattenModules(pol.Sections)
	if len(flat) == 0 {
		return "", errors.New("sat: policy has no modules")
	}

	nextIdx := a.ModuleIndex + 1
	if nextIdx < 0 || nextIdx >= len(flat) {
		// Already at last module or out of range.
		return "", nil
	}

	pm := flat[nextIdx] // placeholder module we'd deliver if sequential

	// If there are no variants or no route rule, fall back to sequential.
	if len(pm.Variants) == 0 || pm.Route.isZero() {
		return "", nil
	}

	// Route (currently supports by_score). Extend here for other rules (by_percent, etc).
	chosen := chooseVariant(pm, perf)
	if chosen == "" {
		// Failed to choose (e.g., bad route config) -> sequential fallback.
		return "", nil
	}
	// Validate the chosen variant actually exists in policy.
	if !hasVariant(pm, chosen) {
		// Misconfigured policy: chosen variant id not present.
		return "", nil
	}
	return chosen, nil
}

/* ---------------------------- Policy shapes ---------------------------- */

type policy struct {
	Sections []section `json:"sections"`
}

type section struct {
	ID      string           `json:"id"`
	Title   string           `json:"title,omitempty"`
	Modules []placeholderMod `json:"modules"`
}

type placeholderMod struct {
	ID           string    `json:"id"`
	TimeLimitSec int       `json:"time_limit_sec"`
	Variants     []variant `json:"variants,omitempty"`
	Route        route     `json:"route,omitempty"`
}

type variant struct {
	ID           string `json:"id"`
	TimeLimitSec int    `json:"time_limit_sec,omitempty"`
}

type route struct {
	ByScore *byScore `json:"by_score,omitempty"`
}

func (r route) isZero() bool { return r.ByScore == nil }

// byScore chooses a variant based on RawPoints compared to Threshold.
// You can provide any of LT/LTE/GT/GTE; the first matching key used below wins.
// "default" is optional as a fallback.
type byScore struct {
	Threshold float64 `json:"threshold"`
	LT        string  `json:"lt,omitempty"`
	LTE       string  `json:"lte,omitempty"`
	GT        string  `json:"gt,omitempty"`
	GTE       string  `json:"gte,omitempty"`
	Default   string  `json:"default,omitempty"`
}

/* ----------------------------- Helpers ----------------------------- */

func flattenModules(secs []section) []placeholderMod {
	var out []placeholderMod
	for _, s := range secs {
		out = append(out, s.Modules...)
	}
	return out
}

func hasVariant(pm placeholderMod, id string) bool {
	id = strings.TrimSpace(id)
	for _, v := range pm.Variants {
		if strings.TrimSpace(v.ID) == id {
			return true
		}
	}
	return false
}

func chooseVariant(pm placeholderMod, perf exam.Perf) string {
	r := pm.Route
	if r.ByScore != nil {
		return chooseByScore(*r.ByScore, perf.RawPoints)
	}
	return ""
}

func chooseByScore(rule byScore, score float64) string {
	// Prefer explicit boundary ops in this order.
	if rule.LT != "" && score < rule.Threshold {
		return rule.LT
	}
	if rule.LTE != "" && score <= rule.Threshold {
		return rule.LTE
	}
	if rule.GT != "" && score > rule.Threshold {
		return rule.GT
	}
	if rule.GTE != "" && score >= rule.Threshold {
		return rule.GTE
	}
	return rule.Default
}
