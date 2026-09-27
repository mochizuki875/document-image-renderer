package renderer

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizePPTXNegativeLineExtents(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <p:cSld><p:spTree><p:sp><p:spPr><a:xfrm><a:off x="6400800" y="3547872"/><a:ext cx="-960120" cy="886968"/></a:xfrm><a:prstGeom prst="line"/></p:spPr></p:sp></p:spTree></p:cSld>
</p:sld>`)

	updated, changed, err := normalizePPTXPart("ppt/slides/slide1.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !bytes.Contains(updated, []byte(`x="5440680"`)) || !bytes.Contains(updated, []byte(`cx="960120"`)) {
		t.Fatalf("negative extent was not normalized: %s", updated)
	}
}

func TestFitWorksheetToOneLandscapePage(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/><pageMargins left="0.7"/><pageSetup scale="75"/></worksheet>`)

	updated, changed, err := fitWorksheetPart("xl/worksheets/sheet1.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	for _, expected := range []string{`fitToPage="1"`, `orientation="landscape"`, `fitToWidth="1"`, `fitToHeight="1"`} {
		if !changed || !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
	if strings.Contains(text, `scale=`) {
		t.Fatalf("scale attribute must be removed: %s", text)
	}
}

func TestOOXMLArchiveLimits(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"one.xml": "1234", "two.xml": "5678"})
	tests := []struct {
		name   string
		limits ooxmlLimits
		fails  bool
	}{
		{name: "member count boundary", limits: ooxmlLimits{maxMembers: 2}},
		{name: "member count exceeded", limits: ooxmlLimits{maxMembers: 1}, fails: true},
		{name: "member size boundary", limits: ooxmlLimits{maxMemberBytes: 4}},
		{name: "member size exceeded", limits: ooxmlLimits{maxMemberBytes: 3}, fails: true},
		{name: "total size boundary", limits: ooxmlLimits{maxTotalBytes: 8}},
		{name: "total size exceeded", limits: ooxmlLimits{maxTotalBytes: 7}, fails: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOOXMLArchive(source, test.limits)
			var exceeded *OOXMLLimitExceededError
			if errors.As(err, &exceeded) != test.fails {
				t.Fatalf("error = %v, expected limit failure %t", err, test.fails)
			}
		})
	}
}

func TestOOXMLStreamingLimitDoesNotTrustHeader(t *testing.T) {
	_, err := copyLimitedOOXML(io.Discard, strings.NewReader("12345"), "part.xml", 0, ooxmlLimits{maxMemberBytes: 4})
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
}

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

func slideXML(text string) string {
	return `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="` + drawingNamespace + `"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
}

func writeOOXMLFixture(t *testing.T, members map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.zip")
	output, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(output)
	for name, content := range members {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
