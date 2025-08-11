package qti

import (
	"fmt"
	"html"
	"path/filepath"
	"strings"

	"github.com/mind-engage/mindengage-lms/internal/exam"
	"github.com/mind-engage/mindengage-lms/internal/qti/parser"
)

// Map parsed QTI items to internal exam.Exam
func MapToExam(mf parser.Manifest, items []parser.ParsedItem, rewriteMedia func(htmlIn string) string) (exam.Exam, error) {
	q := make([]exam.Question, 0, len(items))
	for _, it := range items {
		var t string
		switch it.Kind {
		case parser.InteractionChoiceSingle:
			t = "mcq_single"
		case parser.InteractionChoiceMulti:
			t = "mcq_multi"
		case parser.InteractionTextEntry:
			t = "short_word"
		case parser.InteractionExtendedText:
			t = "essay"
		default:
			t = "essay"
		}
		var choices []exam.Choice
		for _, c := range it.Choices {
			choices = append(choices, exam.Choice{ID: c.ID, LabelHTML: c.Label})
		}
		q = append(q, exam.Question{
			ID:         it.ID,
			Type:       t,
			PromptHTML: rewriteMedia(it.PromptHTML),
			Choices:    choices,
			AnswerKey:  it.AnswerKey,
			Points:     it.Points,
		})
	}
	return exam.Exam{
		ID:           safeIDFromTitle(mf, "exam"),
		Title:        titleFromManifest(mf),
		TimeLimitSec: 1800,
		Questions:    q,
	}, nil
}

func titleFromManifest(m parser.Manifest) string {
	for _, r := range m.Resources {
		base := filepath.Base(r.Href)
		if base != "" {
			return strings.TrimSuffix(base, filepath.Ext(base))
		}
	}
	return "Imported Exam"
}
func safeIDFromTitle(m parser.Manifest, prefix string) string {
	t := strings.ToLower(titleFromManifest(m))
	t = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == '-' || r == '_' {
			return r
		}
		return '-'
	}, t)
	if t == "" {
		t = "exam"
	}
	return fmt.Sprintf("%s-%s", prefix, t)
}

// Basic media rewriter default (no-op)
func NoopRewrite(in string) string { return html.UnescapeString(in) }
