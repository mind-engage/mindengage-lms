package sat

// SATScale is a placeholder raw→scaled mapper.
// It demonstrates the mechanism:
// - expects raw buckets: {"rw_raw": x, "math_raw": y}
// - returns scaled:      {"rw_scaled": 200..800, "math_scaled": 200..800, "total": 400..1600}
type SATScale struct{}

func (SATScale) Scale(raw map[string]float64) map[string]float64 {
	// Linear-ish placeholder: map [0..40] → [200..800]
	// Clamp and simple linear transform; replace with a table later.
	scaleOne := func(x float64) float64 {
		if x < 0 {
			x = 0
		}
		if x > 40 {
			x = 40
		}
		return 200 + (x/40.0)*600.0
	}
	rw := scaleOne(raw["rw_raw"])
	math := scaleOne(raw["math_raw"])

	return map[string]float64{
		"rw_scaled":   float64(int(rw + 0.5)),
		"math_scaled": float64(int(math + 0.5)),
		"total":       float64(int(rw + math + 0.5)),
	}
}
