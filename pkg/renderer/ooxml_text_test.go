package renderer

import (
	"errors"
	"strings"
	"testing"
)

func TestExtractParagraphTextPreservesDocumentStructure(t *testing.T) {
	document := `<w:document xmlns:w="` + wordprocessingNamespace + `"><w:body>` +
		`<w:p><w:r><w:t>This is </w:t></w:r><w:r><w:t>important</w:t></w:r><w:tab/><w:r><w:t>text.</w:t></w:r><w:br/><w:r><w:t>next</w:t></w:r></w:p>` +
		`<w:p/><w:p><w:r><w:t>Second paragraph</w:t></w:r></w:p></w:body></w:document>`
	budget := &characterBudget{}
	text, err := extractParagraphText(strings.NewReader(document), wordprocessingNamespace, "p", budget)
	if err != nil {
		t.Fatal(err)
	}
	if text != "This is important\ttext.\nnext\nSecond paragraph" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestExtractParagraphTextStopsAtCharacterLimit(t *testing.T) {
	document := `<w:document xmlns:w="` + wordprocessingNamespace + `"><w:body><w:p><w:r><w:t>12345</w:t></w:r></w:p></w:body></w:document>`
	for _, maximum := range []int{5, 4} {
		budget := &characterBudget{max: maximum}
		_, err := extractParagraphText(strings.NewReader(document), wordprocessingNamespace, "p", budget)
		var exceeded *CharacterLimitExceededError
		if (maximum == 4) != errors.As(err, &exceeded) {
			t.Fatalf("maximum %d: unexpected error %v", maximum, err)
		}
	}
}

func TestExtractPPTXUsesPresentationOrderAndExcludesOrphans(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{
		"ppt/presentation.xml":            `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId r:id="rId3"/><p:sldId show="0" r:id="rId1"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="slides/slide1.xml"/><Relationship Id="rId3" Target="slides/slide3.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":           slideXML("first"),
		"ppt/slides/slide2.xml":           slideXML("orphan"),
		"ppt/slides/slide3.xml":           slideXML("third"),
	})
	options := DefaultExtractOptions()
	parts, err := extractPPTXText(source, options, &characterBudget{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || parts[0].Text != "third" || parts[1].Text != "first" {
		t.Fatalf("unexpected slide order: %#v", parts)
	}
}

func TestExtractPPTXRejectsMissingRelationship(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{
		"ppt/presentation.xml":            `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId r:id="missing"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`,
	})
	options := DefaultExtractOptions()
	if _, err := extractPPTXText(source, options, &characterBudget{}); err == nil || !strings.Contains(err.Error(), "relationship") {
		t.Fatalf("expected relationship error, got %v", err)
	}
}

func TestExtractPPTXRejectsMissingPresentation(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`,
	})
	options := DefaultExtractOptions()
	if _, err := extractPPTXText(source, options, &characterBudget{}); err == nil || !strings.Contains(err.Error(), "presentation.xml") {
		t.Fatalf("expected missing presentation error, got %v", err)
	}
}

func TestExtractPPTXRejectsMissingRelationships(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{
		"ppt/presentation.xml": `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst/></p:presentation>`,
	})
	options := DefaultExtractOptions()
	if _, err := extractPPTXText(source, options, &characterBudget{}); err == nil || !strings.Contains(err.Error(), "presentation.xml.rels") {
		t.Fatalf("expected missing relationships error, got %v", err)
	}
}

func TestExtractPPTXRejectsMissingSlidePart(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{
		"ppt/presentation.xml":            `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId r:id="rId1"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="slides/slide1.xml"/></Relationships>`,
	})
	options := DefaultExtractOptions()
	if _, err := extractPPTXText(source, options, &characterBudget{}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing slide part error, got %v", err)
	}
}

func TestOrderedSlideNamesRejectsMalformedPresentation(t *testing.T) {
	if _, err := orderedSlideNames([]byte("<not-xml"), []byte(`<Relationships/>`)); err == nil {
		t.Fatal("expected parse error for malformed presentation")
	}
}

func TestOrderedSlideNamesRejectsMalformedRelationships(t *testing.T) {
	if _, err := orderedSlideNames([]byte(`<p:presentation/>`), []byte("<not-xml")); err == nil {
		t.Fatal("expected parse error for malformed relationships")
	}
}

func TestOrderedSlideNamesRejectsExternalTarget(t *testing.T) {
	presentation := `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId r:id="rId1"/></p:sldIdLst></p:presentation>`
	relationships := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="http://example.com/slide.xml" TargetMode="External"/></Relationships>`
	if _, err := orderedSlideNames([]byte(presentation), []byte(relationships)); err == nil {
		t.Fatal("external targets must not be treated as slide relationships")
	}
}

func TestOrderedSlideNamesRejectsInvalidTarget(t *testing.T) {
	presentation := `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId r:id="rId1"/></p:sldIdLst></p:presentation>`
	for _, target := range []string{"../outside.xml", "/abs/slide.xml", "slides/slide1.xml?x=1", "slides/slide1.xml#frag", `slides\slide1.xml`} {
		relationships := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="` + target + `"/></Relationships>`
		if _, err := orderedSlideNames([]byte(presentation), []byte(relationships)); err == nil {
			t.Fatalf("target %q must be rejected", target)
		}
	}
}

func TestOrderedSlideNamesRejectsMissingRelationship(t *testing.T) {
	presentation := `<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId r:id="missing"/></p:sldIdLst></p:presentation>`
	relationships := `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`
	if _, err := orderedSlideNames([]byte(presentation), []byte(relationships)); err == nil {
		t.Fatal("expected missing relationship error")
	}
}

func TestExtractDOCXReturnsEmptyPartWithoutDocumentXML(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"word/styles.xml": "<w:styles/>"})
	options := DefaultExtractOptions()
	parts, err := extractDOCXText(source, options, &characterBudget{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].PartNumber != 1 || parts[0].Text != "" {
		t.Fatalf("unexpected parts: %#v", parts)
	}
}

func TestExtractDOCXRejectsMalformedDocumentXML(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"word/document.xml": "<not-xml"})
	options := DefaultExtractOptions()
	if _, err := extractDOCXText(source, options, &characterBudget{}); err == nil {
		t.Fatal("expected parse error for malformed document.xml")
	}
}

func TestExtractParagraphTextHandlesTabsAndBreaks(t *testing.T) {
	document := `<w:document xmlns:w="` + wordprocessingNamespace + `"><w:body>` +
		`<w:p><w:r><w:t>a</w:t></w:r><w:tab/><w:r><w:t>b</w:t></w:r><w:br/><w:r><w:t>c</w:t></w:r><w:cr/><w:r><w:t>d</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	budget := &characterBudget{}
	text, err := extractParagraphText(strings.NewReader(document), wordprocessingNamespace, "p", budget)
	if err != nil {
		t.Fatal(err)
	}
	if text != "a\tb\nc\nd" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestExtractParagraphTextSkipsEmptyParagraphs(t *testing.T) {
	document := `<w:document xmlns:w="` + wordprocessingNamespace + `"><w:body>` +
		`<w:p><w:r><w:t>first</w:t></w:r></w:p><w:p><w:r><w:t></w:t></w:r></w:p><w:p><w:r><w:t>second</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	budget := &characterBudget{}
	text, err := extractParagraphText(strings.NewReader(document), wordprocessingNamespace, "p", budget)
	if err != nil {
		t.Fatal(err)
	}
	if text != "first\nsecond" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestExtractParagraphTextRejectsMalformedXML(t *testing.T) {
	if _, err := extractParagraphText(strings.NewReader("<not-xml"), wordprocessingNamespace, "p", &characterBudget{}); err == nil {
		t.Fatal("expected parse error for malformed XML")
	}
}

func TestParagraphLimitReservesSeparator(t *testing.T) {
	budget := &characterBudget{max: 10, count: 9}
	max, limited := paragraphLimit(budget, 1)
	if !limited || max != 0 {
		t.Fatalf("expected remaining 0 after reserving separator, got max=%d limited=%t", max, limited)
	}
	budget.count = 10
	max, limited = paragraphLimit(budget, 0)
	if !limited || max != 0 {
		t.Fatalf("expected remaining 0 at budget, got max=%d limited=%t", max, limited)
	}
}

func slideXML(text string) string {
	return `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="` + drawingNamespace + `"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
}
