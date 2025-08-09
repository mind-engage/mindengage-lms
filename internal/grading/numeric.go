package grading

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
)

// numericStrategy supports exact string match or numeric tolerance via AnswerKey.
// Examples:
//
//	AnswerKey: ["3.14159", "tol=0.01"]   // absolute tolerance
//	AnswerKey: ["100", "reltol=0.05"]    // 5% relative tolerance
type numericStrategy struct{}

func (numericStrategy) Grade(_ context.Context, q Q, response interface{}) (Result, error) {
	res := Result{MaxPoints: q.Points}
	str, ok := response.(string)
	if !ok {
		return res, errors.New("response must be string")
	}
	if len(q.AnswerKey) == 0 {
		return res, nil
	}
	target := q.AnswerKey[0]

	if str == target {
		res.AutoPoints = q.Points
		return res, nil
	}

	rv, rOK := parseFloatLoose(str)
	tv, tOK := parseFloatLoose(target)
	if !rOK || !tOK {
		return res, nil
	}

	absTol, relTol := parseTolerances(q.AnswerKey[1:])
	diff := math.Abs(rv - tv)
	pass := false
	if absTol >= 0 && diff <= absTol {
		pass = true
	}
	if !pass && relTol >= 0 && (diff <= relTol*math.Abs(tv)) {
		pass = true
	}
	if pass {
		res.AutoPoints = q.Points
	}
	return res, nil
}

func parseFloatLoose(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v, true
	}
	if sp := strings.Fields(s); len(sp) > 0 {
		if v, err := strconv.ParseFloat(sp[0], 64); err == nil {
			return v, true
		}
	}
	return 0, false
}

func parseTolerances(keys []string) (absTol float64, relTol float64) {
	absTol, relTol = -1, -1
	for _, k := range keys {
		k = strings.TrimSpace(strings.ToLower(k))
		if strings.HasPrefix(k, "tol=") {
			if v, err := strconv.ParseFloat(strings.TrimPrefix(k, "tol="), 64); err == nil {
				absTol = v
			}
		}
		if strings.HasPrefix(k, "reltol=") {
			if v, err := strconv.ParseFloat(strings.TrimPrefix(k, "reltol="), 64); err == nil {
				relTol = v
			}
		}
	}
	return
}
