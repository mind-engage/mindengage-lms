package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/mind-engage/mindengage-lms/internal/exam"
)

// Very small exporter that writes a manifest and simple items for single/multi/text/essay.

func BuildPackage(ex exam.Exam, fetchMedia func(path string) (io.ReadCloser, error)) ([]byte, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// manifest
	mf := imsManifest{
		Resources: []imsResource{},
	}
	for _, q := range ex.Questions {
		itemName := fmt.Sprintf("%s.xml", q.ID)
		mf.Resources = append(mf.Resources, imsResource{
			Identifier: q.ID,
			Type:       "imsqti_item_xmlv2p1",
			Href:       itemName,
			Files:      []imsFile{{Href: itemName}},
		})
		// write item file
		w, _ := zw.Create(itemName)
		io.WriteString(w, buildItemXML(q))
	}
	// write manifest
	mfw, _ := zw.Create("imsmanifest.xml")
	b, _ := xml.MarshalIndent(mf, "", "  ")
	mfw.Write([]byte(xml.Header))
	mfw.Write(b)

	// (Optional) include media in the future: iterate prompts to find src= and add to zip using fetchMedia

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- mini XML model for manifest (export only) ---
type imsManifest struct {
	XMLName   xml.Name      `xml:"manifest"`
	Xmlns     string        `xml:"xmlns,attr,omitempty"`
	Resources []imsResource `xml:"resources>resource"`
}
type imsResource struct {
	Identifier string    `xml:"identifier,attr"`
	Type       string    `xml:"type,attr"`
	Href       string    `xml:"href,attr"`
	Files      []imsFile `xml:"file"`
}
type imsFile struct {
	Href string `xml:"href,attr"`
}

// Build a tiny QTI-2.x-ish item (minimal)
func buildItemXML(q exam.Question) string {
	switch q.Type {
	case "mcq_single", "mcq_multi":
		card := "single"
		if q.Type == "mcq_multi" {
			card = "multiple"
		}
		var choices strings.Builder
		for _, c := range q.Choices {
			choices.WriteString(fmt.Sprintf(`<simpleChoice identifier="%s">%s</simpleChoice>`, c.ID, c.LabelHTML))
		}
		var correct strings.Builder
		for _, v := range q.AnswerKey {
			correct.WriteString(fmt.Sprintf("<value>%s</value>", v))
		}
		return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<assessmentItem identifier="%s" title="%s" xmlns="http://www.imsglobal.org/xsd/imsqti_v2p1">
  <responseDeclaration identifier="RESPONSE" cardinality="%s">
    <correctResponse>%s</correctResponse>
  </responseDeclaration>
  <itemBody>
    %s
    <choiceInteraction responseIdentifier="RESPONSE" maxChoices="%d">
      %s
    </choiceInteraction>
  </itemBody>
</assessmentItem>`,
			q.ID, q.ID, card, correct.String(), q.PromptHTML, maxChoices(card), choices.String(),
		)
	case "short_word":
		// treat as textEntry
		var correct strings.Builder
		for _, v := range q.AnswerKey {
			correct.WriteString(fmt.Sprintf("<value>%s</value>", v))
		}
		return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<assessmentItem identifier="%s" title="%s" xmlns="http://www.imsglobal.org/xsd/imsqti_v2p1">
  <responseDeclaration identifier="RESPONSE" cardinality="single">
    <correctResponse>%s</correctResponse>
  </responseDeclaration>
  <itemBody>
    %s
    <textEntryInteraction responseIdentifier="RESPONSE"/>
  </itemBody>
</assessmentItem>`,
			q.ID, q.ID, correct.String(), q.PromptHTML,
		)
	default: // essay
		return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<assessmentItem identifier="%s" title="%s" xmlns="http://www.imsglobal.org/xsd/imsqti_v2p1">
  <itemBody>
    %s
    <extendedTextInteraction responseIdentifier="RESPONSE"/>
  </itemBody>
</assessmentItem>`, q.ID, q.ID, q.PromptHTML)
	}
}
func maxChoices(card string) int {
	if card == "multiple" {
		return 99
	}
	return 1
}
