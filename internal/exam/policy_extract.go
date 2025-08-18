// internal/exam/policy_extract.go
package exam

import (
	"encoding/json"
	"strings"
)

type ModuleInfo struct {
	SectionID    string
	ModuleID     string
	TimeLimitSec int
}

// extractModules flattens sections -> modules, preserving section membership.
// NOTE: This reads placeholder modules only. If you later introduce variants
// (e.g., "rw-m2-easy"), you’ll choose a concrete module via routing and store it
// in Attempt.CurrentModuleID; this function still remains correct for baseline timing/order.
func extractModules(policyRaw json.RawMessage) []ModuleInfo {
	if len(policyRaw) == 0 {
		return nil
	}
	var pol struct {
		Sections []struct {
			ID      string `json:"id"`
			Modules []struct {
				ID           string `json:"id"`
				TimeLimitSec int    `json:"time_limit_sec"`
			} `json:"modules"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(policyRaw, &pol); err != nil {
		return nil
	}
	out := make([]ModuleInfo, 0, 8)
	for _, s := range pol.Sections {
		secID := strings.TrimSpace(s.ID)
		for _, m := range s.Modules {
			out = append(out, ModuleInfo{
				SectionID:    secID,
				ModuleID:     strings.TrimSpace(m.ID),
				TimeLimitSec: m.TimeLimitSec,
			})
		}
	}
	return out
}
