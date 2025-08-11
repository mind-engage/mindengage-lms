package formats

// ScaleMapper converts raw scores (and/or subscores) to scaled composites per profile.
type ScaleMapper interface {
	Scale(raw map[string]float64) map[string]float64
}

var scaleRegistry = map[string]ScaleMapper{}

// RegisterScale binds a mapper to a key like "sat.v1.scale".
func RegisterScale(key string, m ScaleMapper) { scaleRegistry[key] = m }

// ApplyScaling applies a registered scale mapper; returns raw if not found.
func ApplyScaling(key string, raw map[string]float64) map[string]float64 {
	if m, ok := scaleRegistry[key]; ok && m != nil {
		return m.Scale(raw)
	}
	// default: passthrough
	out := make(map[string]float64, len(raw))
	for k, v := range raw {
		out[k] = v
	}
	return out
}
