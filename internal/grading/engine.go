package grading

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

type OCR interface {
	Extract(ctx context.Context, r io.Reader) (string, error)
	ExtractPath(ctx context.Context, path string) (string, error)
}

// Q is a minimal view of a question needed for grading.
// Keep this in sync with whatever fields your store uses.
type Q struct {
	Type      string
	Points    float64
	AnswerKey []string
}

// Result is the outcome of grading a single question response.
type Result struct {
	AutoPoints  float64  // points awarded automatically
	MaxPoints   float64  // the question's max points
	NeedsManual bool     // true if teacher review is required
	Feedback    []string // optional notes
}

// Strategy grades a single question.
type Strategy interface {
	Grade(ctx context.Context, q Q, response interface{}) (Result, error)
}

// Grader routes by question type to the correct Strategy.
type Grader interface {
	Grade(ctx context.Context, q Q, response interface{}) (Result, error)
}

type defaultGrader struct {
	strategies map[string]Strategy
}

func (g *defaultGrader) Grade(ctx context.Context, q Q, response interface{}) (Result, error) {
	s, ok := g.strategies[q.Type]
	if !ok {
		return Result{MaxPoints: q.Points, NeedsManual: true, Feedback: []string{"no strategy available"}}, nil
	}
	return s.Grade(ctx, q, response)
}

// Engine options

type Option func(*config)

type config struct {
	MaxEditDistance   int  // for short-word fuzzy
	AllowPartialMulti bool // partial credit for mcq_multi without FP
	OCR               OCR  // optional OCR for "scan"
}

func WithMaxEditDistance(n int) Option { return func(c *config) { c.MaxEditDistance = n } }
func WithPartialMulti(b bool) Option   { return func(c *config) { c.AllowPartialMulti = b } }
func WithOCR(o OCR) Option             { return func(c *config) { c.OCR = o } }

// NewDefaultGrader installs built-in strategies.
func NewDefaultGrader(opts ...Option) Grader {
	cfg := &config{
		MaxEditDistance:   1,
		AllowPartialMulti: true,
	}
	for _, o := range opts {
		o(cfg)
	}
	return &defaultGrader{
		strategies: map[string]Strategy{
			"mcq_single": mcqSingleStrategy{},
			"true_false": mcqSingleStrategy{},
			"mcq_multi":  mcqMultiStrategy{allowPartial: cfg.AllowPartialMulti},
			"short_word": shortWordStrategy{maxEdit: cfg.MaxEditDistance},
			"numeric":    numericStrategy{},
			"essay":      essayStrategy{},
			"scan":       scanStrategy{ocr: cfg.OCR},
		},
	}
}

// --- Strategies ---

type mcqSingleStrategy struct{}

func (mcqSingleStrategy) Grade(_ context.Context, q Q, response interface{}) (Result, error) {
	res := Result{MaxPoints: q.Points}
	resp, ok := response.(string)
	if !ok {
		return res, errors.New("response must be string")
	}
	for _, k := range q.AnswerKey {
		if resp == k {
			res.AutoPoints = q.Points
			return res, nil
		}
	}
	return res, nil
}

type mcqMultiStrategy struct{ allowPartial bool }

func (s mcqMultiStrategy) Grade(_ context.Context, q Q, response interface{}) (Result, error) {
	res := Result{MaxPoints: q.Points}
	respSlice, ok := toStringSlice(response)
	if !ok {
		return res, errors.New("response must be []string")
	}
	correct := toSet(q.AnswerKey)
	resp := toSet(respSlice)

	if setEqual(correct, resp) {
		res.AutoPoints = q.Points
		return res, nil
	}
	hasFalsePositive := false
	for r := range resp {
		if _, ok := correct[r]; !ok {
			hasFalsePositive = true
			break
		}
	}
	if s.allowPartial && !hasFalsePositive && len(correct) > 0 {
		inter := 0
		for k := range resp {
			if _, ok := correct[k]; ok {
				inter++
			}
		}
		res.AutoPoints = q.Points * (float64(inter) / float64(len(correct)))
	}
	return res, nil
}

type shortWordStrategy struct{ maxEdit int }

func (s shortWordStrategy) Grade(_ context.Context, q Q, response interface{}) (Result, error) {
	res := Result{MaxPoints: q.Points}
	resp, ok := response.(string)
	if !ok {
		return res, errors.New("response must be string")
	}
	normResp := normalize(resp)

	best := 0
	for _, k := range q.AnswerKey {
		nk := normalize(k)
		if nk == normResp {
			res.AutoPoints = q.Points
			return res, nil
		}
		if s.maxEdit > 0 && levenshtein(nk, normResp) <= s.maxEdit {
			if best < 1 {
				best = 1
			}
		}
	}
	if best == 1 {
		res.AutoPoints = q.Points * 0.5
		res.Feedback = append(res.Feedback, "close match (fuzzy)")
	}
	return res, nil
}

type essayStrategy struct{}

func (essayStrategy) Grade(_ context.Context, q Q, response interface{}) (Result, error) {
	return Result{MaxPoints: q.Points, NeedsManual: true, Feedback: []string{"manual grading required"}}, nil
}

type scanStrategy struct{ ocr OCR }

func (s scanStrategy) Grade(ctx context.Context, q Q, response interface{}) (Result, error) {
	res := Result{MaxPoints: q.Points, NeedsManual: true}
	if s.ocr == nil {
		res.Feedback = append(res.Feedback, "OCR not configured")
		return res, nil
	}
	switch v := response.(type) {
	case []byte:
		text, err := s.ocr.Extract(ctx, bytesReader(v))
		if err != nil {
			res.Feedback = append(res.Feedback, "OCR failed: "+err.Error())
			return res, nil
		}
		score, fb := keywordHeuristic(text, q.AnswerKey, q.Points)
		res.AutoPoints = score
		res.Feedback = append(res.Feedback, fb...)
		return res, nil
	case string:
		text, err := s.ocr.ExtractPath(ctx, v)
		if err != nil {
			res.Feedback = append(res.Feedback, "OCR failed: "+err.Error())
			return res, nil
		}
		score, fb := keywordHeuristic(text, q.AnswerKey, q.Points)
		res.AutoPoints = score
		res.Feedback = append(res.Feedback, fb...)
		return res, nil
	default:
		return res, errors.New("scan response must be []byte or string path")
	}
}

// helpers

func toStringSlice(v interface{}) ([]string, bool) {
	switch t := v.(type) {
	case []string:
		return t, true
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func toSet(arr []string) map[string]struct{} {
	m := make(map[string]struct{}, len(arr))
	for _, s := range arr {
		m[s] = struct{}{}
	}
	return m
}

func setEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func keywordHeuristic(text string, required []string, max float64) (float64, []string) {
	if len(required) == 0 || strings.TrimSpace(text) == "" {
		return 0, []string{"no keywords or empty OCR"}
	}
	found := 0
	low := strings.ToLower(text)
	for _, k := range required {
		if k == "" {
			continue
		}
		if strings.Contains(low, strings.ToLower(k)) {
			found++
		}
	}
	score := max * (float64(found) / float64(len(required)))
	fb := []string{fmt.Sprintf("keyword hits: %d/%d", found, len(required))}
	return score, fb
}

func bytesReader(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}
