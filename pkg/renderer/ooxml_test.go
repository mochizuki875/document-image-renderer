package renderer

import (
	"archive/zip"
	"bytes"
	"errors"
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

func TestNormalizePPTXIgnoresNonSlideParts(t *testing.T) {
	xml := []byte(`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:spPr><a:xfrm><a:off x="1" y="2"/><a:ext cx="-3" cy="4"/></a:xfrm><a:prstGeom prst="line"/></p:spPr></p:sp></p:spTree></p:cSld></p:sld>`)
	updated, changed, err := normalizePPTXPart("ppt/slideLayouts/slideLayout1.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	if changed || !bytes.Equal(updated, xml) {
		t.Fatalf("non-slide part must not be modified: changed=%t", changed)
	}
}

func TestNormalizePPTXIgnoresNonLineShapes(t *testing.T) {
	xml := []byte(`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:spPr><a:xfrm><a:off x="1" y="2"/><a:ext cx="-3" cy="4"/></a:xfrm><a:prstGeom prst="rect"/></p:spPr></p:sp></p:spTree></p:cSld></p:sld>`)
	updated, changed, err := normalizePPTXPart("ppt/slides/slide1.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	if changed || !bytes.Equal(updated, xml) {
		t.Fatalf("non-line shape must not be modified: changed=%t", changed)
	}
}

func TestNormalizePPTXHandlesOnlyNegativeAxis(t *testing.T) {
	xml := []byte(`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:spPr><a:xfrm><a:off x="100" y="200"/><a:ext cx="50" cy="-30"/></a:xfrm><a:prstGeom prst="line"/></p:spPr></p:sp></p:spTree></p:cSld></p:sld>`)
	updated, changed, err := normalizePPTXPart("ppt/slides/slide1.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected a change")
	}
	if !bytes.Contains(updated, []byte(`y="170"`)) || !bytes.Contains(updated, []byte(`cy="30"`)) {
		t.Fatalf("only the negative axis must be normalized: %s", updated)
	}
	if !bytes.Contains(updated, []byte(`x="100"`)) || !bytes.Contains(updated, []byte(`cx="50"`)) {
		t.Fatalf("positive axis must be unchanged: %s", updated)
	}
}

func TestNormalizePPTXRejectsMalformedExtent(t *testing.T) {
	xml := []byte(`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:spPr><a:xfrm><a:off x="1" y="2"/><a:ext cx="not-a-number" cy="4"/></a:xfrm><a:prstGeom prst="line"/></p:spPr></p:sp></p:spTree></p:cSld></p:sld>`)
	if _, _, err := normalizePPTXPart("ppt/slides/slide1.xml", xml); err == nil {
		t.Fatal("expected parse error for malformed extent")
	}
}

func TestNormalizePPTXRejectsMalformedXML(t *testing.T) {
	if _, _, err := normalizePPTXPart("ppt/slides/slide1.xml", []byte("<not-xml")); err == nil {
		t.Fatal("expected parse error for malformed XML")
	}
}

func TestFitWorksheetCreatesMissingElements(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/></worksheet>`)
	updated, changed, err := fitWorksheetPart("xl/worksheets/sheet1.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected a change")
	}
	text := string(updated)
	for _, expected := range []string{`fitToPage="1"`, `orientation="landscape"`, `fitToWidth="1"`, `fitToHeight="1"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %s in %s", expected, text)
		}
	}
}

func TestFitWorksheetInsertsPageSetupAfterPageMargins(t *testing.T) {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/><pageMargins left="0.7"/></worksheet>`)
	updated, _, err := fitWorksheetPart("xl/worksheets/sheet1.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	marginsIndex := strings.Index(text, "<pageMargins")
	setupIndex := strings.Index(text, "<pageSetup")
	if marginsIndex == -1 || setupIndex == -1 || setupIndex < marginsIndex {
		t.Fatalf("pageSetup must follow pageMargins: %s", text)
	}
}

func TestFitWorksheetIgnoresNonWorksheetParts(t *testing.T) {
	xml := []byte(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/></worksheet>`)
	updated, changed, err := fitWorksheetPart("xl/workbook.xml", xml)
	if err != nil {
		t.Fatal(err)
	}
	if changed || !bytes.Equal(updated, xml) {
		t.Fatalf("non-worksheet part must not be modified: changed=%t", changed)
	}
}

func TestFitWorksheetRejectsMalformedXML(t *testing.T) {
	if _, _, err := fitWorksheetPart("xl/worksheets/sheet1.xml", []byte("<not-xml")); err == nil {
		t.Fatal("expected parse error for malformed XML")
	}
}

func TestRewriteOOXMLAppliesTransformAndPreservesMembers(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/></worksheet>`,
		"xl/workbook.xml":          `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"/>`,
	})
	workingDirectory := t.TempDir()
	prepared, err := rewriteOOXML(source, workingDirectory, false, fitWorksheetPart, ooxmlLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if prepared == source {
		t.Fatal("rewrite must produce a new file when onlyWhenChanged is false")
	}
	archive, err := zip.OpenReader(prepared)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	found := map[string]bool{}
	for _, file := range archive.File {
		found[file.Name] = true
	}
	if !found["xl/worksheets/sheet1.xml"] || !found["xl/workbook.xml"] {
		t.Fatalf("members were not preserved: %v", found)
	}
}

func TestRewriteOOXMLReturnsSourceWhenUnchanged(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{
		"xl/workbook.xml": `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"/>`,
	})
	workingDirectory := t.TempDir()
	prepared, err := rewriteOOXML(source, workingDirectory, true, fitWorksheetPart, ooxmlLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if prepared != source {
		t.Fatalf("rewrite must return the original source when nothing changed: %s", prepared)
	}
}

func TestRewriteOOXMLRejectsInvalidArchive(t *testing.T) {
	source := filepath.Join(t.TempDir(), "invalid.zip")
	if err := os.WriteFile(source, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := rewriteOOXML(source, t.TempDir(), false, fitWorksheetPart, ooxmlLimits{}); err == nil {
		t.Fatal("expected error for invalid archive")
	}
}

func TestRewriteOOXMLEnforcesLimits(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"one.xml": "1234"})
	_, err := rewriteOOXML(source, t.TempDir(), false, fitWorksheetPart, ooxmlLimits{maxMemberBytes: 3})
	var exceeded *OOXMLLimitExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("expected OOXMLLimitExceededError, got %v", err)
	}
}

func TestPrepareOfficeSourceRewritesPPTX(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:spPr><a:xfrm><a:off x="100" y="200"/><a:ext cx="-50" cy="30"/></a:xfrm><a:prstGeom prst="line"/></p:spPr></p:sp></p:spTree></p:cSld></p:sld>`,
	})
	prepared, err := prepareOfficeSource(source, ".pptx", t.TempDir(), ooxmlLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if prepared == source {
		t.Fatal("PPTX with negative extents must be rewritten")
	}
}

func TestPrepareOfficeSourceRewritesWorkbook(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{
		"xl/worksheets/sheet1.xml": `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData/></worksheet>`,
	})
	prepared, err := prepareOfficeSource(source, ".xlsx", t.TempDir(), ooxmlLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if prepared == source {
		t.Fatal("workbook must be rewritten")
	}
}

func TestPrepareOfficeSourceValidatesDOCX(t *testing.T) {
	source := writeOOXMLFixture(t, map[string]string{"word/document.xml": "<w:document/>"})
	prepared, err := prepareOfficeSource(source, ".docx", t.TempDir(), ooxmlLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if prepared != source {
		t.Fatal("DOCX must be returned unchanged after validation")
	}
}

func TestPrepareOfficeSourceReturnsUnsupportedFormat(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.pdf")
	if err := os.WriteFile(source, []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareOfficeSource(source, ".pdf", t.TempDir(), ooxmlLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if prepared != source {
		t.Fatal("unsupported format must be returned unchanged")
	}
}

func TestPrepareOfficeSourcePropagatesRewriteError(t *testing.T) {
	source := filepath.Join(t.TempDir(), "input.pptx")
	if err := os.WriteFile(source, []byte("not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareOfficeSource(source, ".pptx", t.TempDir(), ooxmlLimits{}); err == nil {
		t.Fatal("expected error for invalid PPTX archive")
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
