package grading

import "fmt"

type Rubric struct {
	Criteria []Criterion `json:"criteria"`
	Max      float64     `json:"max_points"`
}

type Criterion struct {
	Key       string  `json:"key"`
	Desc      string  `json:"desc"`
	MaxPoints float64 `json:"max_points"`
}

func ScoreRubric(r Rubric, awarded map[string]float64) (float64, []string) {
	total := 0.0
	notes := make([]string, 0, len(r.Criteria))
	for _, c := range r.Criteria {
		v := awarded[c.Key]
		if v < 0 {
			v = 0
		}
		if v > c.MaxPoints {
			v = c.MaxPoints
		}
		total += v
		notes = append(notes, fmt.Sprintf("%s:%.2f", c.Key, v))
	}
	if r.Max > 0 && total > r.Max {
		total = r.Max
	}
	return total, notes
}
