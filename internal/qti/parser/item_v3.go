package parser

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

type assessmentItem struct {
	XMLName      xml.Name            `xml:"assessmentItem"`
	Identifier   string              `xml:"identifier,attr"`
	Title        string              `xml:"title,attr"`
	Body         itemBody            `xml:"itemBody"`
	ResponseDecl responseDeclaration `xml:"responseDeclaration"`
	OutcomeDecl  outcomeDeclaration  `xml:"outcomeDeclaration"`
}
type itemBody struct {
	// We extract just enough: prompt html-ish and interaction type
	RawXML string `xml:",innerxml"`
}
type responseDeclaration struct {
	Identifier  string `xml:"identifier,attr"`
	Cardinality string `xml:"cardinality,attr"` // single|multiple
	Correct     struct {
		Values []string `xml:"value"`
	} `xml:"correctResponse"`
	// numeric tolerance extension (non-standard) via mapping or <value> forms
}
type outcomeDeclaration struct {
	Identifier string `xml:"identifier,attr"`
	BaseType   string `xml:"baseType,attr"`
}

type InteractionType string

const (
	InteractionChoiceSingle InteractionType = "choice_single"
	InteractionChoiceMulti  InteractionType = "choice_multi"
	InteractionTextEntry    InteractionType = "text_entry"
	InteractionExtendedText InteractionType = "extended_text"
)

type ParsedItem struct {
	ID         string
	Title      string
	PromptHTML string
	Kind       InteractionType
	Choices    []Choice // for choice
	AnswerKey  []string // correct ids or strings
	Points     float64
}

type Choice struct {
	ID    string
	Label string // HTML
}

// NOTE: We don't fully parse <itemBody> interactions; a robust parser is larger.
// For MVP we infer from body content (presence of <choiceInteraction>, etc.) and extract labels heuristically.
func ParseItemFile(baseDir, rel string) (ParsedItem, error) {
	b, err := os.ReadFile(filepath.Join(baseDir, rel))
	if err != nil {
		return ParsedItem{}, err
	}
	var it assessmentItem
	if err := xml.Unmarshal(b, &it); err != nil {
		return ParsedItem{}, err
	}

	pi := ParsedItem{
		ID:         it.Identifier,
		Title:      it.Title,
		PromptHTML: extractPrompt(it.Body.RawXML),
		Points:     1, // default, can be extended reading outcomeDecl
	}

	body := strings.ToLower(it.Body.RawXML)
	switch {
	case strings.Contains(body, "<choiceinteraction"):
		if it.ResponseDecl.Cardinality == "multiple" {
			pi.Kind = InteractionChoiceMulti
		} else {
			pi.Kind = InteractionChoiceSingle
		}
		pi.Choices = extractChoices(it.Body.RawXML) // IDs + labels
		pi.AnswerKey = it.ResponseDecl.Correct.Values
	case strings.Contains(body, "<textentryinteraction"):
		pi.Kind = InteractionTextEntry
		pi.AnswerKey = it.ResponseDecl.Correct.Values
	case strings.Contains(body, "<extendedtextinteraction"):
		pi.Kind = InteractionExtendedText
	default:
		// fallback: treat as extended text
		pi.Kind = InteractionExtendedText
	}
	return pi, nil
}

// --- very small HTML-ish extraction helpers (heuristic) ---

func extractPrompt(inner string) string {
	// remove interaction tags, keep preceding text as prompt
	l := strings.ToLower(inner)
	idx := strings.Index(l, "<choiceinteraction")
	if idx == -1 {
		idx = strings.Index(l, "<textentryinteraction")
	}
	if idx == -1 {
		idx = strings.Index(l, "<extendedtextinteraction")
	}
	if idx == -1 {
		return inner
	}
	return strings.TrimSpace(inner[:idx])
}

// Extract choices as <simpleChoice identifier="A">Label</simpleChoice>
func extractChoices(inner string) []Choice {
	out := []Choice{}
	dec := xml.NewDecoder(strings.NewReader(inner))
	for {
		t, err := dec.Token()
		if err != nil {
			break
		}
		switch se := t.(type) {
		case xml.StartElement:
			if strings.EqualFold(se.Name.Local, "simpleChoice") {
				var id string
				for _, a := range se.Attr {
					if strings.EqualFold(a.Name.Local, "identifier") {
						id = a.Value
						break
					}
				}
				var text struct {
					Inner string `xml:",innerxml"`
				}
				if err := dec.DecodeElement(&text, &se); err == nil {
					out = append(out, Choice{ID: id, Label: strings.TrimSpace(text.Inner)})
				}
			}
		}
	}
	return out
}
